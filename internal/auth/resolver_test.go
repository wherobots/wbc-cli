package auth

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wherobots/cli/internal/config"
)

func testResolverConfig(t *testing.T, domain string) config.Config {
	t.Helper()
	return config.Config{
		OpenAPIURL:      "https://api.staging.wherobots.com/openapi.json",
		OAuthDomain:     domain,
		OAuthClientID:   "client_test_123",
		CredentialsPath: filepath.Join(t.TempDir(), "credentials.json"),
		HTTPTimeout:     5 * time.Second,
	}
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://api.staging.wherobots.com/users/me", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestApplyEnvAPIKeyWinsOverSession(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	cfg.APIKey = "key-1"
	resolver := NewResolver(cfg)

	// A stored OAuth session exists but must be shadowed by the env key.
	if err := resolver.store.Put(cfg.OAuthDomain, testSession("access-1")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	req := newRequest(t)
	if err := resolver.Apply(context.Background(), req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := req.Header.Get("x-api-key"); got != "key-1" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}

func TestApplyUsesBearerFromStoredSession(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	resolver := NewResolver(cfg)

	if err := resolver.store.Put(cfg.OAuthDomain, testSession("access-1")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	req := newRequest(t)
	if err := resolver.Apply(context.Background(), req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer access-1" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestApplyNoCredentialsError(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	resolver := NewResolver(cfg)

	err := resolver.Apply(context.Background(), newRequest(t))
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "wherobots auth login") {
		t.Errorf("error should mention auth login, got: %v", msg)
	}
	if !strings.Contains(msg, "https://staging.wherobots.com/settings#api-keys") {
		t.Errorf("error should mention API key URL, got: %v", msg)
	}
	if !strings.Contains(msg, "WHEROBOTS_API_KEY") {
		t.Errorf("error should mention env var, got: %v", msg)
	}
}

// refreshableServer scripts a refresh endpoint returning rotated tokens.
func refreshableServer(t *testing.T, responses []mockResponse) *mockAuthKit {
	t.Helper()
	return newMockAuthKit(t, responses)
}

func TestApplyRefreshesInsideSkewAndPersistsRotation(t *testing.T) {
	t.Parallel()
	m := refreshableServer(t, []mockResponse{
		{http.StatusOK, map[string]any{
			"access_token":  "access-2",
			"refresh_token": "refresh-2",
			"expires_in":    3600,
		}},
	})
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	sess := testSession("access-1")
	sess.TokenEndpoint = m.srv.URL + "/oauth2/token"
	sess.ClientID = "client_stored_456"          // differs from the resolver's default
	sess.ExpiresAt = time.Now().Add(time.Minute) // inside the 2m skew
	if err := resolver.store.Put(cfg.OAuthDomain, sess); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	req := newRequest(t)
	if err := resolver.Apply(context.Background(), req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer access-2" {
		t.Errorf("Authorization = %q, want rotated token", got)
	}

	// The rotated refresh token must be persisted.
	stored, err := resolver.store.Get(cfg.OAuthDomain)
	if err != nil || stored == nil {
		t.Fatalf("Get() = %+v, %v", stored, err)
	}
	if stored.RefreshToken != "refresh-2" || stored.AccessToken != "access-2" {
		t.Errorf("stored session not rotated: %+v", stored)
	}
	if stored.Email != "clay@wherobots.com" || stored.TokenEndpoint == "" {
		t.Errorf("stored session lost metadata: %+v", stored)
	}

	// The refresh grant must carry the session's stored client id, not the
	// resolver's current default.
	if got := m.tokenForms[0].Get("client_id"); got != "client_stored_456" {
		t.Errorf("refresh client_id = %q, want the session's stored client id", got)
	}
}

func TestRefreshInvalidGrantAdoptsSessionRotatedByAnotherProcess(t *testing.T) {
	t.Parallel()
	// The refresh grant fails with invalid_grant because another CLI process
	// already redeemed (and rotated) the refresh token...
	m := refreshableServer(t, []mockResponse{
		{http.StatusBadRequest, map[string]any{"error": "invalid_grant"}},
	})
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	// ...and the store already holds the winner's fresh session.
	winner := testSession("access-2")
	winner.RefreshToken = "refresh-2"
	winner.ExpiresAt = time.Now().Add(time.Hour)
	if err := resolver.store.Put(cfg.OAuthDomain, winner); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	stale := testSession("access-1")
	stale.TokenEndpoint = m.srv.URL + "/oauth2/token"

	got, err := resolver.refreshSession(context.Background(), &stale)
	if err != nil {
		t.Fatalf("refreshSession() error = %v", err)
	}
	if got.AccessToken != "access-2" {
		t.Errorf("AccessToken = %q, want the winner's rotated token", got.AccessToken)
	}
	if stored, _ := resolver.store.Get(cfg.OAuthDomain); stored == nil {
		t.Errorf("the winner's session must not be deleted")
	}
}

func TestForceRefreshCorruptStoreMentionsRelogin(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	if err := os.WriteFile(cfg.CredentialsPath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	resolver := NewResolver(cfg)

	ok, err := resolver.ForceRefresh(context.Background())
	if ok {
		t.Errorf("ForceRefresh() ok = true, want false")
	}
	if err == nil || !strings.Contains(err.Error(), "wherobots auth login") {
		t.Fatalf("err = %v, want corrupt-store guidance", err)
	}
}

func TestApplyFreshTokenSkipsRefresh(t *testing.T) {
	t.Parallel()
	m := refreshableServer(t, nil) // any token call would fail the test
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	sess := testSession("access-1")
	sess.ExpiresAt = time.Now().Add(time.Hour)
	if err := resolver.store.Put(cfg.OAuthDomain, sess); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	req := newRequest(t)
	if err := resolver.Apply(context.Background(), req); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer access-1" {
		t.Errorf("Authorization = %q", got)
	}
	if len(m.tokenForms) != 0 {
		t.Errorf("unexpected refresh calls: %v", m.tokenForms)
	}
}

func TestApplyInvalidGrantDeletesSession(t *testing.T) {
	t.Parallel()
	m := refreshableServer(t, []mockResponse{
		{http.StatusBadRequest, map[string]any{"error": "invalid_grant"}},
	})
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	sess := testSession("access-1")
	sess.TokenEndpoint = m.srv.URL + "/oauth2/token"
	sess.ExpiresAt = time.Now().Add(-time.Minute)
	if err := resolver.store.Put(cfg.OAuthDomain, sess); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	err := resolver.Apply(context.Background(), newRequest(t))
	if err == nil || !strings.Contains(err.Error(), "wherobots auth login") {
		t.Fatalf("err = %v, want session-expired message", err)
	}

	if stored, _ := resolver.store.Get(cfg.OAuthDomain); stored != nil {
		t.Errorf("session should be deleted after invalid_grant")
	}
}

func TestApplyTransientRefreshErrorKeepsSession(t *testing.T) {
	t.Parallel()
	m := refreshableServer(t, []mockResponse{
		{http.StatusInternalServerError, map[string]any{"error": "server_error"}},
	})
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	sess := testSession("access-1")
	sess.TokenEndpoint = m.srv.URL + "/oauth2/token"
	sess.ExpiresAt = time.Now().Add(-time.Minute)
	if err := resolver.store.Put(cfg.OAuthDomain, sess); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if err := resolver.Apply(context.Background(), newRequest(t)); err == nil {
		t.Fatalf("expected transient error")
	}
	if stored, _ := resolver.store.Get(cfg.OAuthDomain); stored == nil {
		t.Errorf("session should survive a transient refresh failure")
	}
}

func TestForceRefreshWithAPIKeyIsNoop(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	cfg.APIKey = "key-1"
	resolver := NewResolver(cfg)

	ok, err := resolver.ForceRefresh(context.Background())
	if ok || err != nil {
		t.Fatalf("ForceRefresh() = %v, %v; want false, nil", ok, err)
	}
}

func TestForceRefreshRefreshesUnconditionally(t *testing.T) {
	t.Parallel()
	m := refreshableServer(t, []mockResponse{
		{http.StatusOK, map[string]any{
			"access_token":  "access-2",
			"refresh_token": "refresh-2",
			"expires_in":    3600,
		}},
	})
	cfg := testResolverConfig(t, m.srv.URL)
	resolver := NewResolver(cfg)

	sess := testSession("access-1")
	sess.TokenEndpoint = m.srv.URL + "/oauth2/token"
	sess.ExpiresAt = time.Now().Add(time.Hour) // fresh, but force anyway
	if err := resolver.store.Put(cfg.OAuthDomain, sess); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	ok, err := resolver.ForceRefresh(context.Background())
	if !ok || err != nil {
		t.Fatalf("ForceRefresh() = %v, %v; want true, nil", ok, err)
	}
	if stored, _ := resolver.store.Get(cfg.OAuthDomain); stored == nil || stored.AccessToken != "access-2" {
		t.Errorf("stored session not refreshed: %+v", stored)
	}
}

func TestApplyCorruptStoreMentionsRelogin(t *testing.T) {
	t.Parallel()
	cfg := testResolverConfig(t, "https://login.example")
	if err := os.WriteFile(cfg.CredentialsPath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	resolver := NewResolver(cfg)

	err := resolver.Apply(context.Background(), newRequest(t))
	if err == nil || !strings.Contains(err.Error(), "wherobots auth login") {
		t.Fatalf("err = %v, want corrupt-store guidance", err)
	}
}
