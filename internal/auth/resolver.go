package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"wherobots/cli/internal/config"
)

// refreshSkew treats a token as expired shortly before its real expiry so an
// in-flight request never carries a token that dies mid-request. Mirrors the
// VS Code extension's REFRESH_SKEW_MS.
const refreshSkew = 2 * time.Minute

// Source identifies which credential a request will use.
type Source int

const (
	SourceNone Source = iota
	SourceAPIKey
	SourceOAuth
)

// Resolver picks the credential for outgoing requests. Precedence: the
// WHEROBOTS_API_KEY env var always wins over a stored OAuth session, so CI
// and scripts behave predictably (gh/aws convention — this intentionally
// differs from the VS Code extension, where OAuth wins).
type Resolver struct {
	cfg    config.Config
	store  *Store
	client *Client

	// now is a test seam for expiry checks.
	now func() time.Time
}

func NewResolver(cfg config.Config) *Resolver {
	return &Resolver{
		cfg:   cfg,
		store: &Store{Path: cfg.CredentialsPath},
		client: &Client{
			Domain:   cfg.OAuthDomain,
			ClientID: cfg.OAuthClientID,
			HTTP:     &http.Client{Timeout: cfg.HTTPTimeout},
		},
		now: time.Now,
	}
}

// Store exposes the session store for the auth commands.
func (r *Resolver) Store() *Store {
	return r.store
}

// OAuthClient exposes the device-flow client for the auth commands.
func (r *Resolver) OAuthClient() *Client {
	return r.client
}

// Source reports which credential Apply would use, without side effects.
func (r *Resolver) Source() Source {
	if r.cfg.APIKey != "" {
		return SourceAPIKey
	}
	session, err := r.store.Get(r.cfg.OAuthDomain)
	if err == nil && session != nil {
		return SourceOAuth
	}
	return SourceNone
}

// Apply sets the auth header on req, refreshing the stored session first
// when it is inside the expiry skew.
func (r *Resolver) Apply(ctx context.Context, req *http.Request) error {
	if r.cfg.APIKey != "" {
		req.Header.Set("x-api-key", r.cfg.APIKey)
		return nil
	}

	session, err := r.store.Get(r.cfg.OAuthDomain)
	if err != nil {
		if errors.Is(err, ErrCorruptStore) {
			return fmt.Errorf("%w — run `wherobots auth login` to sign in again", err)
		}
		return err
	}
	if session == nil {
		return r.noCredentialsError()
	}

	if r.now().After(session.ExpiresAt.Add(-refreshSkew)) {
		session, err = r.refreshSession(ctx, session)
		if err != nil {
			return err
		}
	}

	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	return nil
}

// ForceRefresh refreshes the stored session unconditionally — the backstop
// after a 401. It reports whether a replay with fresh credentials makes
// sense (always false for API-key/no-credential sources).
func (r *Resolver) ForceRefresh(ctx context.Context) (bool, error) {
	if r.cfg.APIKey != "" {
		return false, nil
	}
	session, err := r.store.Get(r.cfg.OAuthDomain)
	if err != nil || session == nil {
		return false, nil
	}
	if _, err := r.refreshSession(ctx, session); err != nil {
		return false, err
	}
	return true, nil
}

// refreshSession runs the refresh grant and persists the rotated tokens
// before returning. An invalid_grant means the session is dead: it is
// removed so the next attempt gives the sign-in hint instead of looping.
func (r *Resolver) refreshSession(ctx context.Context, session *Session) (*Session, error) {
	if session.RefreshToken == "" {
		_, _ = r.store.Delete(r.cfg.OAuthDomain)
		return nil, r.sessionExpiredError()
	}

	tokens, err := r.client.Refresh(ctx, session.TokenEndpoint, session.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidGrant) {
			_, _ = r.store.Delete(r.cfg.OAuthDomain)
			return nil, r.sessionExpiredError()
		}
		// Transient (network/5xx): keep the session so a retry can succeed.
		return nil, err
	}

	updated := *session
	updated.AccessToken = tokens.AccessToken
	updated.ExpiresAt = tokens.ExpiresAt
	if tokens.RefreshToken != "" {
		updated.RefreshToken = tokens.RefreshToken
	}
	if err := r.store.Put(r.cfg.OAuthDomain, updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *Resolver) sessionExpiredError() error {
	return fmt.Errorf("your session has expired — run `wherobots auth login` to sign in again")
}

func (r *Resolver) noCredentialsError() error {
	return NoCredentialsError(r.cfg)
}

// NoCredentialsError explains both sign-in routes; shared by the resolver
// and `auth status`.
func NoCredentialsError(cfg config.Config) error {
	return fmt.Errorf(
		"no credentials found\n\nSign in with your browser:\n\n  wherobots auth login\n\nOr create an API key at %s and export it:\n\n  export WHEROBOTS_API_KEY='<your-api-key>'",
		config.APIKeyURL(cfg.OpenAPIURL),
	)
}
