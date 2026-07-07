package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadDefaultsToWherobotsOpenAPISpec(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")
	t.Setenv("WHEROBOTS_UPLOAD_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppName != "wherobots" {
		t.Fatalf("AppName = %q, want %q", cfg.AppName, "wherobots")
	}
	if cfg.OpenAPIURL != "https://api.cloud.wherobots.com/openapi.json" {
		t.Fatalf("OpenAPIURL = %q", cfg.OpenAPIURL)
	}
	if cfg.APIKey != "key-1" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "key-1")
	}
	if cfg.UploadPath != "" {
		t.Fatalf("UploadPath = %q, want empty", cfg.UploadPath)
	}
}

func TestLoadBuildsSpecURLFromWherobotsAPIURL(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "https://api.example.com")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OpenAPIURL != "https://api.example.com/openapi.json" {
		t.Fatalf("OpenAPIURL = %q, want %q", cfg.OpenAPIURL, "https://api.example.com/openapi.json")
	}
}

func TestLoadSucceedsWithoutAPIKey(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIKey != "" {
		t.Fatalf("APIKey = %q, want empty", cfg.APIKey)
	}
}

func TestAPIKeyURLDerivedFromDefaultURL(t *testing.T) {
	got := APIKeyURL("https://api.cloud.wherobots.com/openapi.json")
	if got != "https://cloud.wherobots.com/settings#api-keys" {
		t.Fatalf("APIKeyURL = %q", got)
	}
}

func TestAPIKeyURLDerivedFromCustomURL(t *testing.T) {
	got := APIKeyURL("https://api.staging.wherobots.com/openapi.json")
	if got != "https://staging.wherobots.com/settings#api-keys" {
		t.Fatalf("APIKeyURL = %q", got)
	}
}

func TestLoadDefaultsOAuthConfig(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "")
	t.Setenv("WHEROBOTS_OAUTH_DOMAIN", "")
	t.Setenv("WHEROBOTS_OAUTH_CLIENT_ID", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OAuthDomain != "https://login.cloud.wherobots.com" {
		t.Fatalf("OAuthDomain = %q", cfg.OAuthDomain)
	}
	if cfg.OAuthClientID == "" {
		t.Fatalf("OAuthClientID should have a baked-in default")
	}
}

func TestLoadOAuthEnvOverridesAreTrimmed(t *testing.T) {
	t.Setenv("WHEROBOTS_OAUTH_DOMAIN", "  https://staging.authkit.example/  ")
	t.Setenv("WHEROBOTS_OAUTH_CLIENT_ID", " client_staging_123 ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OAuthDomain != "https://staging.authkit.example" {
		t.Fatalf("OAuthDomain = %q", cfg.OAuthDomain)
	}
	if cfg.OAuthClientID != "client_staging_123" {
		t.Fatalf("OAuthClientID = %q", cfg.OAuthClientID)
	}
}

func TestLoadBlankOAuthOverridesFallBackToDefaults(t *testing.T) {
	t.Setenv("WHEROBOTS_OAUTH_DOMAIN", "   ")
	t.Setenv("WHEROBOTS_OAUTH_CLIENT_ID", "   ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OAuthDomain != "https://login.cloud.wherobots.com" {
		t.Fatalf("OAuthDomain = %q", cfg.OAuthDomain)
	}
}

func TestLoadResolvesCredentialsPath(t *testing.T) {
	t.Setenv("WHEROBOTS_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CredentialsPath == "" {
		t.Fatalf("CredentialsPath should be set")
	}
	if !strings.HasSuffix(cfg.CredentialsPath, filepath.Join("wherobots", "credentials.json")) {
		t.Fatalf("CredentialsPath = %q, want .../wherobots/credentials.json", cfg.CredentialsPath)
	}
}

func TestCachePathIsKeyedOnAPIURL(t *testing.T) {
	t.Setenv("WHEROBOTS_API_KEY", "key-1")

	t.Setenv("WHEROBOTS_API_URL", "https://api.cloud.wherobots.com")
	cfgProd, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	t.Setenv("WHEROBOTS_API_URL", "https://api.staging.wherobots.com")
	cfgStaging, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfgProd.CachePath == cfgStaging.CachePath {
		t.Errorf("different API URLs must produce different cache paths; both got %q", cfgProd.CachePath)
	}
	if !strings.Contains(cfgProd.CachePath, "spec-") {
		t.Errorf("cache path should contain URL key prefix, got %q", cfgProd.CachePath)
	}
}

func TestLoadReadsUploadPathConfig(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")
	t.Setenv("WHEROBOTS_UPLOAD_PATH", "s3://override-bucket/custom/root")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.UploadPath != "s3://override-bucket/custom/root" {
		t.Fatalf("UploadPath = %q", cfg.UploadPath)
	}
}

func TestLoadToleratesUnresolvableUserDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("user dirs resolve from the registry/env differently on windows")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want API-key-only usage to survive unresolvable user dirs", err)
	}
	if cfg.CredentialsPath != "" {
		t.Errorf("CredentialsPath = %q, want empty", cfg.CredentialsPath)
	}
	if cfg.CachePath != "" || cfg.CacheMeta != "" {
		t.Errorf("cache paths = %q/%q, want empty", cfg.CachePath, cfg.CacheMeta)
	}
}

func TestOAuthDomainLikelyMismatched(t *testing.T) {
	cases := []struct {
		name        string
		apiURL      string
		oauthDomain string
		want        bool
	}{
		{"custom API with default domain", "https://api.staging.wherobots.com", "", true},
		{"custom API with custom domain", "https://api.staging.wherobots.com", "https://login.staging.example", false},
		{"default API with default domain", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WHEROBOTS_API_URL", tc.apiURL)
			t.Setenv("WHEROBOTS_OAUTH_DOMAIN", tc.oauthDomain)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := cfg.OAuthDomainLikelyMismatched(); got != tc.want {
				t.Errorf("OAuthDomainLikelyMismatched() = %v, want %v", got, tc.want)
			}
		})
	}
}
