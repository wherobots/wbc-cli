package spec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wherobots/cli/internal/config"
)

// fakeCreds sets a header, or fails when err is set.
type fakeCreds struct {
	header string
	value  string
	err    error
}

func (f *fakeCreds) Apply(_ context.Context, req *http.Request) error {
	if f.err != nil {
		return f.err
	}
	req.Header.Set(f.header, f.value)
	return nil
}

func TestLoaderDownloadAndCache(t *testing.T) {
	t.Parallel()

	specDoc := `{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{}}`
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(specDoc))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	cfg := config.Config{
		OpenAPIURL:  server.URL,
		CachePath:   filepath.Join(tempDir, "spec.json"),
		CacheMeta:   filepath.Join(tempDir, "spec.meta.json"),
		CacheTTL:    15 * time.Minute,
		HTTPTimeout: time.Second,
	}

	loader := NewLoader(cfg, &fakeCreds{header: "Authorization", value: "Bearer token-1"})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != specDoc {
		t.Fatalf("Load() = %s, want %s", string(got), specDoc)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q, want bearer from credentials", gotAuth)
	}

	cached, err := os.ReadFile(cfg.CachePath)
	if err != nil {
		t.Fatalf("expected cache file: %v", err)
	}
	if string(cached) != specDoc {
		t.Fatalf("cached spec = %s, want %s", string(cached), specDoc)
	}
}

func TestLoaderFallsBackToCacheWhenRefreshFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "spec.json")
	metaPath := filepath.Join(tempDir, "spec.meta.json")
	cachedSpec := []byte(`{"openapi":"3.0.3","info":{"title":"cached","version":"1"},"paths":{}}`)
	if err := writeCache(cachePath, metaPath, cachedSpec, "https://old.example/spec.json", time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatalf("writeCache() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.Config{
		OpenAPIURL:  server.URL,
		CachePath:   cachePath,
		CacheMeta:   metaPath,
		CacheTTL:    time.Minute,
		HTTPTimeout: time.Second,
	}

	loader := NewLoader(cfg, nil)
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != string(cachedSpec) {
		t.Fatalf("Load() = %s, want cached spec %s", string(got), string(cachedSpec))
	}
}

func TestLoaderDownloadsAnonymouslyWhenCredentialsFail(t *testing.T) {
	t.Parallel()

	specDoc := `{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{}}`
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != ""
		_, _ = w.Write([]byte(specDoc))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	cfg := config.Config{
		OpenAPIURL:  server.URL,
		CachePath:   filepath.Join(tempDir, "spec.json"),
		CacheMeta:   filepath.Join(tempDir, "spec.meta.json"),
		CacheTTL:    15 * time.Minute,
		HTTPTimeout: time.Second,
	}

	// No credentials must never block the spec fetch (e.g. `auth login` on a
	// fresh machine).
	loader := NewLoader(cfg, &fakeCreds{err: errors.New("no credentials found")})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != specDoc {
		t.Fatalf("Load() = %s", string(got))
	}
	if sawAuth {
		t.Fatalf("expected anonymous spec fetch when credentials fail")
	}
}
