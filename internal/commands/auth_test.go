package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"wherobots/cli/internal/auth"
	"wherobots/cli/internal/config"
)

// fakeAuthKit mocks the two AuthKit endpoints the device flow needs.
type fakeAuthKit struct {
	srv            *httptest.Server
	tokenResponses []func(w http.ResponseWriter)
	tokenCalls     int
}

func newFakeAuthKit(t *testing.T, accessToken string, pendingPolls int) *fakeAuthKit {
	t.Helper()
	f := &fakeAuthKit{}

	pending := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
	}
	success := func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": "refresh-1",
			"expires_in":    3600,
		})
	}
	for range pendingPolls {
		f.tokenResponses = append(f.tokenResponses, pending)
	}
	f.tokenResponses = append(f.tokenResponses, success)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "device-code-1",
			"user_code":                 "WDJB-MJHT",
			"verification_uri":          f.srv.URL + "/device",
			"verification_uri_complete": f.srv.URL + "/device?user_code=WDJB-MJHT",
			"expires_in":                299,
			"interval":                  1, // keep test polling fast; interval semantics are covered in internal/auth
		})
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := f.tokenResponses[min(f.tokenCalls, len(f.tokenResponses)-1)]
		f.tokenCalls++
		respond(w)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func testAccessToken(t *testing.T, email string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"sub":   "user_01ABC",
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func authTestConfig(t *testing.T, domain string) config.Config {
	t.Helper()
	return config.Config{
		AppName:         "wherobots",
		OpenAPIURL:      "https://api.staging.wherobots.com/openapi.json",
		OAuthDomain:     domain,
		OAuthClientID:   "client_test_123",
		CredentialsPath: filepath.Join(t.TempDir(), "credentials.json"),
		HTTPTimeout:     5 * time.Second,
	}
}

// stubBrowser replaces openBrowser for the test, recording opened URLs.
func stubBrowser(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	orig := openBrowser
	openBrowser = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openBrowser = orig })
	return &opened
}

func execAuth(t *testing.T, cfg config.Config, creds *auth.Resolver, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "wherobots", SilenceUsage: true, SilenceErrors: true}
	AddAuthCommand(root, cfg, creds)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestAuthLoginHappyPath(t *testing.T) {
	fake := newFakeAuthKit(t, testAccessToken(t, "clay@wherobots.com"), 1)
	cfg := authTestConfig(t, fake.srv.URL)
	opened := stubBrowser(t)
	creds := auth.NewResolver(cfg)

	out, err := execAuth(t, cfg, creds, "auth", "login")
	if err != nil {
		t.Fatalf("auth login error = %v\noutput:\n%s", err, out)
	}

	for _, want := range []string{
		"Confirmation code: WDJB-MJHT",
		"Opening your browser to confirm sign-in...",
		fake.srv.URL + "/device?user_code=WDJB-MJHT",
		"Waiting for confirmation... (expires in 4m59s)",
		"Signed in to https://api.staging.wherobots.com as clay@wherobots.com",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(*opened) != 1 || !strings.Contains((*opened)[0], "user_code=WDJB-MJHT") {
		t.Errorf("browser opened with %v, want verification_uri_complete", *opened)
	}

	session, err := creds.Store().Get(cfg.OAuthDomain)
	if err != nil || session == nil {
		t.Fatalf("stored session = %+v, %v", session, err)
	}
	if session.RefreshToken != "refresh-1" || session.Email != "clay@wherobots.com" {
		t.Errorf("session = %+v", session)
	}
	if session.TokenEndpoint != fake.srv.URL+"/oauth2/token" {
		t.Errorf("TokenEndpoint = %q", session.TokenEndpoint)
	}

	info, err := os.Stat(cfg.CredentialsPath)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials perm = %o, want 600", perm)
	}
}

func TestAuthLoginNoBrowserPrintsURLOnly(t *testing.T) {
	fake := newFakeAuthKit(t, testAccessToken(t, "clay@wherobots.com"), 0)
	cfg := authTestConfig(t, fake.srv.URL)
	opened := stubBrowser(t)
	creds := auth.NewResolver(cfg)

	out, err := execAuth(t, cfg, creds, "auth", "login", "--no-browser")
	if err != nil {
		t.Fatalf("auth login error = %v", err)
	}
	if len(*opened) != 0 {
		t.Errorf("browser should not be opened with --no-browser, got %v", *opened)
	}
	if !strings.Contains(out, "Visit this URL to confirm sign-in:") {
		t.Errorf("output missing manual URL instructions:\n%s", out)
	}
}

func TestAuthLoginNotesShadowingAPIKey(t *testing.T) {
	fake := newFakeAuthKit(t, testAccessToken(t, "clay@wherobots.com"), 0)
	cfg := authTestConfig(t, fake.srv.URL)
	cfg.APIKey = "key-1"
	stubBrowser(t)
	creds := auth.NewResolver(cfg)

	out, err := execAuth(t, cfg, creds, "auth", "login")
	if err != nil {
		t.Fatalf("auth login error = %v", err)
	}
	if !strings.Contains(out, "WHEROBOTS_API_KEY is set and takes precedence") {
		t.Errorf("output missing precedence note:\n%s", out)
	}
}

func TestAuthStatusOAuthSession(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	creds := auth.NewResolver(cfg)
	if err := creds.Store().Put(cfg.OAuthDomain, auth.Session{
		AccessToken: "a",
		ExpiresAt:   time.Now().Add(43 * time.Minute),
		Email:       "clay@wherobots.com",
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	out, err := execAuth(t, cfg, creds, "auth", "status")
	if err != nil {
		t.Fatalf("auth status error = %v", err)
	}
	for _, want := range []string{
		"API host:    https://api.staging.wherobots.com",
		"Credential:  OAuth session (wherobots auth login)",
		"Account:     clay@wherobots.com",
		"(auto-refreshes)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAuthStatusAPIKeyShadowsSession(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	cfg.APIKey = "key-1"
	creds := auth.NewResolver(cfg)
	if err := creds.Store().Put(cfg.OAuthDomain, auth.Session{
		AccessToken: "a",
		ExpiresAt:   time.Now().Add(time.Hour),
		Email:       "clay@wherobots.com",
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	out, err := execAuth(t, cfg, creds, "auth", "status")
	if err != nil {
		t.Fatalf("auth status error = %v", err)
	}
	if !strings.Contains(out, "Credential:  WHEROBOTS_API_KEY environment variable") {
		t.Errorf("output missing env credential line:\n%s", out)
	}
	if !strings.Contains(out, "is ignored while WHEROBOTS_API_KEY is set") {
		t.Errorf("output missing shadowing note:\n%s", out)
	}
}

func TestAuthStatusNoCredentialsErrors(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	creds := auth.NewResolver(cfg)

	_, err := execAuth(t, cfg, creds, "auth", "status")
	if err == nil {
		t.Fatalf("expected error exit for no credentials")
	}
	for _, want := range []string{"wherobots auth login", "WHEROBOTS_API_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

func TestAuthLogout(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	creds := auth.NewResolver(cfg)
	if err := creds.Store().Put(cfg.OAuthDomain, auth.Session{AccessToken: "a", Email: "clay@wherobots.com"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := creds.Store().Put("https://login.other", auth.Session{AccessToken: "b"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	out, err := execAuth(t, cfg, creds, "auth", "logout")
	if err != nil {
		t.Fatalf("auth logout error = %v", err)
	}
	if !strings.Contains(out, "Signed out of https://login.example (clay@wherobots.com)") {
		t.Errorf("unexpected logout output:\n%s", out)
	}

	// Only the current domain's session is removed.
	if s, _ := creds.Store().Get(cfg.OAuthDomain); s != nil {
		t.Errorf("current domain session should be removed")
	}
	if s, _ := creds.Store().Get("https://login.other"); s == nil {
		t.Errorf("other domain session should survive")
	}

	// Second logout reports there is nothing to remove.
	out, err = execAuth(t, cfg, creds, "auth", "logout")
	if err != nil {
		t.Fatalf("auth logout error = %v", err)
	}
	if !strings.Contains(out, "No stored session for https://login.example") {
		t.Errorf("unexpected repeat logout output:\n%s", out)
	}
}

func TestAuthLogoutAll(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	creds := auth.NewResolver(cfg)
	_ = creds.Store().Put(cfg.OAuthDomain, auth.Session{AccessToken: "a"})
	_ = creds.Store().Put("https://login.other", auth.Session{AccessToken: "b"})

	out, err := execAuth(t, cfg, creds, "auth", "logout", "--all")
	if err != nil {
		t.Fatalf("auth logout --all error = %v", err)
	}
	if !strings.Contains(out, "Signed out of 2 session(s)") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if s, _ := creds.Store().Get("https://login.other"); s != nil {
		t.Errorf("all sessions should be removed")
	}
}

func TestAuthLogoutRemovesCorruptStore(t *testing.T) {
	cfg := authTestConfig(t, "https://login.example")
	if err := os.WriteFile(cfg.CredentialsPath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	creds := auth.NewResolver(cfg)

	out, err := execAuth(t, cfg, creds, "auth", "logout")
	if err != nil {
		t.Fatalf("auth logout error = %v", err)
	}
	if !strings.Contains(out, "Removed corrupt credentials file") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if _, err := os.Stat(cfg.CredentialsPath); !os.IsNotExist(err) {
		t.Errorf("corrupt file should be removed")
	}
}

func TestIsSpecFreeInvocation(t *testing.T) {
	t.Parallel()
	cfg := config.Config{AppName: "wherobots"}
	bare := BuildBareRootCommand(cfg)
	AddAuthCommand(bare, cfg, auth.NewResolver(cfg))
	AddUpgradeCommand(bare, "dev")

	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"auth", "login"}, true},
		{[]string{"auth", "login", "--no-browser"}, true},
		{[]string{"auth", "status"}, true},
		{[]string{"auth"}, true},
		{[]string{"upgrade"}, true},
		{[]string{"api", "users", "me", "get"}, false},
		{[]string{"job-runs", "list"}, false},
		{[]string{}, false},
		{[]string{"--tree"}, false},
	}
	for _, tc := range cases {
		if got := IsSpecFreeInvocation(bare, tc.args); got != tc.want {
			t.Errorf("IsSpecFreeInvocation(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
