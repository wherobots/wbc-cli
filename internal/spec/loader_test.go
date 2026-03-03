package spec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wherobots/cli/internal/config"
)

func TestLoaderDownloadAndCache(t *testing.T) {
	t.Parallel()

	specDoc := `{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	loader := NewLoader(cfg)
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != specDoc {
		t.Fatalf("Load() = %s, want %s", string(got), specDoc)
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

	loader := NewLoader(cfg)
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != string(cachedSpec) {
		t.Fatalf("Load() = %s, want cached spec %s", string(got), string(cachedSpec))
	}
}
