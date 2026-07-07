package spec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tidwall/gjson"

	"wherobots/cli/internal/config"
)

// Credentials optionally authenticates the spec download; implemented by
// auth.Resolver.
type Credentials interface {
	Apply(ctx context.Context, req *http.Request) error
}

type Loader struct {
	cfg    config.Config
	creds  Credentials
	client *http.Client
}

func NewLoader(cfg config.Config, creds Credentials) *Loader {
	return &Loader{
		cfg:   cfg,
		creds: creds,
		client: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}
}

func (l *Loader) Load(ctx context.Context) ([]byte, error) {
	now := time.Now().UTC()
	cached, cacheErr := readCache(l.cfg.CachePath, l.cfg.CacheMeta)
	hasCache := cacheErr == nil

	if hasCache && isFresh(cached.Meta, l.cfg.CacheTTL, now) {
		return cached.SpecBytes, nil
	}

	if l.cfg.OpenAPIURL != "" {
		specBytes, err := l.download(ctx, l.cfg.OpenAPIURL)
		if err == nil {
			// An empty CachePath means no user cache dir resolved (e.g. HOME
			// unset); run uncached rather than failing.
			if l.cfg.CachePath != "" {
				if writeErr := writeCache(l.cfg.CachePath, l.cfg.CacheMeta, specBytes, l.cfg.OpenAPIURL, now); writeErr != nil {
					return nil, writeErr
				}
			}
			return specBytes, nil
		}
		if hasCache {
			return cached.SpecBytes, nil
		}
		return nil, fmt.Errorf("download spec and no cache fallback available: %w", err)
	}

	if hasCache {
		return cached.SpecBytes, nil
	}

	if cacheErr != nil && !isMissing(cacheErr) {
		return nil, fmt.Errorf("read cache: %w", cacheErr)
	}
	return nil, fmt.Errorf("failed to resolve OpenAPI spec URL")
}

func (l *Loader) download(ctx context.Context, sourceURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build spec request: %w", err)
	}
	// Best-effort auth: the spec endpoint is fetchable anonymously, and the
	// loader must never block credential-free commands like `auth login`.
	if l.creds != nil {
		_ = l.creds.Apply(ctx, req)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch spec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("fetch spec returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	specBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read spec body: %w", err)
	}

	if !gjson.ValidBytes(specBytes) {
		return nil, fmt.Errorf("spec is not valid JSON")
	}
	return specBytes, nil
}
