package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"wherobots/cli/internal/auth"
	"wherobots/cli/internal/config"
)

// openBrowser opens a URL in the system browser. Package-level var so tests
// can stub it; failures are non-fatal because the URL is always printed.
var openBrowser = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// AddAuthCommand registers `auth login|logout|status`. These commands are
// spec-free: they work on a fresh machine with no API key, no cached spec,
// and no network access to the API host (see IsSpecFreeInvocation in main).
func AddAuthCommand(root *cobra.Command, cfg config.Config, creds *auth.Resolver) {
	authCmd := &cobra.Command{
		Use:           "auth",
		Short:         "Sign in to Wherobots and manage credentials",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	var noBrowser bool
	loginCmd := &cobra.Command{
		Use:           "login",
		Short:         "Sign in with your browser",
		Long:          "Sign in to Wherobots with your browser using a one-time confirmation code.\nThe session is stored in " + cfg.CredentialsPath + " and refreshed automatically.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, cfg, creds, noBrowser)
		},
	}
	loginCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the sign-in URL instead of opening a browser")

	var logoutAll bool
	logoutCmd := &cobra.Command{
		Use:           "logout",
		Short:         "Remove the stored sign-in session",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogout(cmd, cfg, creds, logoutAll)
		},
	}
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "remove stored sessions for every environment")

	statusCmd := &cobra.Command{
		Use:           "status",
		Short:         "Show which credential the CLI will use",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd, cfg, creds)
		},
	}

	authCmd.AddCommand(loginCmd, logoutCmd, statusCmd)
	root.AddCommand(authCmd)
}

func runLogin(cmd *cobra.Command, cfg config.Config, creds *auth.Resolver, noBrowser bool) error {
	out := cmd.OutOrStdout()
	client := creds.OAuthClient()

	da, err := client.StartDeviceAuthorization(cmd.Context())
	if err != nil {
		return err
	}

	target := da.VerificationURIComplete
	if target == "" {
		target = da.VerificationURI
	}

	fmt.Fprintf(out, "Confirmation code: %s\n\n", da.UserCode)
	if noBrowser {
		fmt.Fprintf(out, "Visit this URL to confirm sign-in:\n\n  %s\n\n", target)
	} else {
		fmt.Fprintf(out, "Opening your browser to confirm sign-in...\nIf the browser does not open, visit:\n\n  %s\n\n", target)
		_ = openBrowser(target) // non-fatal: the URL is printed above
	}
	if da.ExpiresIn > 0 {
		fmt.Fprintf(out, "Waiting for confirmation... (expires in %s)\n\n", (time.Duration(da.ExpiresIn) * time.Second).Round(time.Second))
	} else {
		fmt.Fprintf(out, "Waiting for confirmation...\n\n")
	}

	tokens, err := client.PollForToken(cmd.Context(), da)
	if err != nil {
		return err
	}

	claims := auth.DecodeClaims(tokens.AccessToken)
	endpoints, err := client.Endpoints(cmd.Context())
	if err != nil {
		return err
	}
	session := auth.Session{
		AccessToken:   tokens.AccessToken,
		RefreshToken:  tokens.RefreshToken,
		ExpiresAt:     tokens.ExpiresAt,
		TokenEndpoint: endpoints.Token,
		ClientID:      cfg.OAuthClientID,
		Email:         claims.Email,
		Sub:           claims.Sub,
	}
	if err := creds.Store().Put(cfg.OAuthDomain, session); err != nil {
		return err
	}

	fmt.Fprintf(out, "Signed in to %s as %s\n", apiBaseURL(cfg), accountLabel(session))
	if cfg.APIKey != "" {
		fmt.Fprintf(out, "\nNote: WHEROBOTS_API_KEY is set and takes precedence; unset it to use this session.\n")
	}
	return nil
}

func runLogout(cmd *cobra.Command, cfg config.Config, creds *auth.Resolver, all bool) error {
	out := cmd.OutOrStdout()
	store := creds.Store()

	if all {
		count, err := store.DeleteAll()
		if errors.Is(err, auth.ErrCorruptStore) {
			return removeCorruptStore(out, cfg)
		}
		if err != nil {
			return err
		}
		if count == 0 {
			fmt.Fprintln(out, "No stored sessions")
			return nil
		}
		fmt.Fprintf(out, "Signed out of %d session(s)\n", count)
		return nil
	}

	removed, err := store.Delete(cfg.OAuthDomain)
	if errors.Is(err, auth.ErrCorruptStore) {
		return removeCorruptStore(out, cfg)
	}
	if err != nil {
		return err
	}
	if removed == nil {
		fmt.Fprintf(out, "No stored session for %s\n", cfg.OAuthDomain)
		return nil
	}
	if label := accountLabel(*removed); label != "" {
		fmt.Fprintf(out, "Signed out of %s (%s)\n", cfg.OAuthDomain, label)
	} else {
		fmt.Fprintf(out, "Signed out of %s\n", cfg.OAuthDomain)
	}
	return nil
}

// removeCorruptStore honors the user's explicit intent to clear credentials
// even when the file cannot be parsed.
func removeCorruptStore(out io.Writer, cfg config.Config) error {
	if err := os.Remove(cfg.CredentialsPath); err != nil {
		return fmt.Errorf("remove corrupt credentials file: %w", err)
	}
	fmt.Fprintf(out, "Removed corrupt credentials file %s\n", cfg.CredentialsPath)
	return nil
}

func runStatus(cmd *cobra.Command, cfg config.Config, creds *auth.Resolver) error {
	out := cmd.OutOrStdout()

	session, storeErr := creds.Store().Get(cfg.OAuthDomain)

	fmt.Fprintf(out, "API host:    %s\n", apiBaseURL(cfg))

	if cfg.APIKey != "" {
		fmt.Fprintln(out, "Credential:  WHEROBOTS_API_KEY environment variable")
		if session != nil {
			label := accountLabel(*session)
			if label == "" {
				label = cfg.OAuthDomain
			}
			fmt.Fprintf(out, "\nNote: stored OAuth session (%s) is ignored while WHEROBOTS_API_KEY is set\n", label)
		}
		return nil
	}

	if storeErr != nil {
		if errors.Is(storeErr, auth.ErrCorruptStore) {
			return fmt.Errorf("%w — run `wherobots auth login` to sign in again", storeErr)
		}
		return storeErr
	}

	if session == nil {
		return auth.NoCredentialsError(cfg)
	}

	fmt.Fprintln(out, "Credential:  OAuth session (wherobots auth login)")
	if label := accountLabel(*session); label != "" {
		fmt.Fprintf(out, "Account:     %s\n", label)
	}
	remaining := time.Until(session.ExpiresAt)
	if remaining > 0 {
		fmt.Fprintf(out, "Token:       expires in %s (auto-refreshes)\n", remaining.Round(time.Minute))
	} else {
		fmt.Fprintln(out, "Token:       expired (refreshes on next use)")
	}
	return nil
}

func accountLabel(session auth.Session) string {
	if session.Email != "" {
		return session.Email
	}
	return session.Sub
}

// apiBaseURL is the API host as the user knows it (the OpenAPI URL without
// the spec path).
func apiBaseURL(cfg config.Config) string {
	return strings.TrimSuffix(cfg.OpenAPIURL, "/openapi.json")
}
