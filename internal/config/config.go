package config

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAppName         = "wherobots"
	defaultOpenAPISpec     = "https://api.cloud.wherobots.com/openapi.json"
	defaultCacheTTL        = 15 * time.Minute
	defaultHTTPTimeout     = 30 * time.Second
	envAppName             = "APP_NAME"
	envWherobotsAPIURL     = "WHEROBOTS_API_URL"
	envWherobotsAPIKey     = "WHEROBOTS_API_KEY"
	envWherobotsUploadPath = "WHEROBOTS_UPLOAD_PATH"
	envOpenAPICacheTTL     = "OPENAPI_CACHE_TTL"
	envHTTPTimeout         = "OPENAPI_HTTP_TIMEOUT"

	// OAuth sign-in (WorkOS AuthKit device flow). The defaults target
	// production; point both env vars at another tenant (e.g. staging) to
	// sign in there, mirroring how WHEROBOTS_API_URL selects the API host.
	defaultOAuthDomain   = "https://login.cloud.wherobots.com"
	defaultOAuthClientID = "client_01KWJ9Z6GR1HQJCQZ71NWC28JM" // dedicated CLI Connect client; public, safe to ship
	envOAuthDomain       = "WHEROBOTS_OAUTH_DOMAIN"
	envOAuthClientID     = "WHEROBOTS_OAUTH_CLIENT_ID"
)

type Config struct {
	AppName         string
	OpenAPIURL      string
	APIKey          string
	CachePath       string
	CacheMeta       string
	CacheTTL        time.Duration
	HTTPTimeout     time.Duration
	UploadPath      string
	OAuthDomain     string
	OAuthClientID   string
	CredentialsPath string
}

func Load() (Config, error) {
	appName := getenvDefault(envAppName, defaultAppName)
	openAPIURL, err := resolveOpenAPISpecURL(os.Getenv(envWherobotsAPIURL))
	if err != nil {
		return Config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv(envWherobotsAPIKey))

	// Like the credentials path below, an unresolvable user cache dir (e.g.
	// HOME unset in a CI container) must not fail Load; the spec loader just
	// skips caching when the paths are empty.
	cachePath, cacheMeta := "", ""
	if cacheRoot, err := os.UserCacheDir(); err == nil {
		cacheDir := filepath.Join(cacheRoot, appName)
		cacheKey := urlCacheKey(openAPIURL)
		cachePath = filepath.Join(cacheDir, "spec-"+cacheKey+".json")
		cacheMeta = filepath.Join(cacheDir, "spec-"+cacheKey+".meta.json")
	}

	ttl, err := parseTTL(os.Getenv(envOpenAPICacheTTL))
	if err != nil {
		return Config{}, err
	}

	timeout, err := parseDuration(os.Getenv(envHTTPTimeout), defaultHTTPTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", envHTTPTimeout, err)
	}

	uploadPath := strings.TrimSpace(os.Getenv(envWherobotsUploadPath))

	// An unresolvable user config dir (e.g. HOME unset in a CI container)
	// must not fail Load — API-key-only usage never touches the credentials
	// file. CredentialsPath stays empty and the auth store reports the
	// problem when credentials are actually needed.
	credentialsPath := ""
	if configRoot, err := os.UserConfigDir(); err == nil {
		credentialsPath = filepath.Join(configRoot, appName, "credentials.json")
	}

	return Config{
		AppName:         appName,
		OpenAPIURL:      openAPIURL,
		APIKey:          apiKey,
		CachePath:       cachePath,
		CacheMeta:       cacheMeta,
		CacheTTL:        ttl,
		HTTPTimeout:     timeout,
		UploadPath:      uploadPath,
		OAuthDomain:     envDefaultTrimmed(envOAuthDomain, defaultOAuthDomain),
		OAuthClientID:   envDefaultTrimmed(envOAuthClientID, defaultOAuthClientID),
		CredentialsPath: credentialsPath,
	}, nil
}

// OAuthDomainLikelyMismatched reports a custom API URL paired with the
// default (production) OAuth domain: sign-in would target the production
// tenant while API requests go elsewhere, which typically ends in 401s.
func (c Config) OAuthDomainLikelyMismatched() bool {
	return c.OpenAPIURL != defaultOpenAPISpec && c.OAuthDomain == defaultOAuthDomain
}

// envDefaultTrimmed reads an env var, trims surrounding whitespace and any
// trailing slash, and falls back to the default when the result is blank.
func envDefaultTrimmed(key, fallback string) string {
	value := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/")
	if value == "" {
		return fallback
	}
	return value
}

// urlCacheKey returns a short hex string derived from the URL so that
// different API endpoints get separate cache files.
func urlCacheKey(rawURL string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(rawURL))
	return fmt.Sprintf("%08x", h.Sum32())
}

func resolveOpenAPISpecURL(baseURL string) (string, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return defaultOpenAPISpec, nil
	}
	raw = strings.TrimRight(raw, "/")
	if !strings.HasSuffix(raw, "/openapi.json") {
		raw += "/openapi.json"
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() {
		return "", fmt.Errorf("%s must be an absolute URL", envWherobotsAPIURL)
	}
	return parsed.String(), nil
}

func parseTTL(raw string) (time.Duration, error) {
	if raw == "" {
		return defaultCacheTTL, nil
	}
	d, err := parseDuration(raw, defaultCacheTTL)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", envOpenAPICacheTTL, err)
	}
	return d, nil
}

func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	if asInt, err := strconv.Atoi(raw); err == nil {
		if asInt <= 0 {
			return 0, fmt.Errorf("must be > 0, got %d", asInt)
		}
		return time.Duration(asInt) * time.Minute, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be > 0, got %s", d)
	}
	return d, nil
}

// APIKeyURL derives the console settings URL from the resolved OpenAPI spec URL.
// It strips the "api." prefix from the host (e.g. api.cloud.wherobots.com → cloud.wherobots.com)
// and appends /settings#api-keys.
func APIKeyURL(openAPISpecURL string) string {
	parsed, err := url.Parse(openAPISpecURL)
	if err != nil {
		return "https://cloud.wherobots.com/settings#api-keys"
	}
	host := parsed.Hostname()
	host = strings.TrimPrefix(host, "api.")
	return fmt.Sprintf("%s://%s/settings#api-keys", parsed.Scheme, host)
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
