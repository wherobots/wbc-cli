package config

import "testing"

func TestLoadDefaultsToWherobotsOpenAPISpec(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")

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

func TestLoadRequiresWherobotsAPIKey(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "https://api.example.com")
	t.Setenv("WHEROBOTS_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load() error")
	}
}
