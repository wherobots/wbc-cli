package config

import (
	"fmt"
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
)

type Config struct {
	AppName     string
	OpenAPIURL  string
	APIKey      string
	CachePath   string
	CacheMeta   string
	CacheTTL    time.Duration
	HTTPTimeout time.Duration
	UploadPath  string
}

func Load() (Config, error) {
	appName := getenvDefault(envAppName, defaultAppName)
	openAPIURL, err := resolveOpenAPISpecURL(os.Getenv(envWherobotsAPIURL))
	if err != nil {
		return Config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv(envWherobotsAPIKey))
	if apiKey == "" {
		return Config{}, fmt.Errorf(
			"%s is required\n\nTo create an API key, visit: https://cloud.wherobots.com/apiKey\nThen export it:\n\n  export %s='<your-api-key>'",
			envWherobotsAPIKey, envWherobotsAPIKey,
		)
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve user cache dir: %w", err)
	}

	cacheDir := filepath.Join(cacheRoot, appName)
	ttl, err := parseTTL(os.Getenv(envOpenAPICacheTTL))
	if err != nil {
		return Config{}, err
	}

	timeout, err := parseDuration(os.Getenv(envHTTPTimeout), defaultHTTPTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", envHTTPTimeout, err)
	}

	uploadPath := strings.TrimSpace(os.Getenv(envWherobotsUploadPath))

	return Config{
		AppName:     appName,
		OpenAPIURL:  openAPIURL,
		APIKey:      apiKey,
		CachePath:   filepath.Join(cacheDir, "spec.json"),
		CacheMeta:   filepath.Join(cacheDir, "spec.meta.json"),
		CacheTTL:    ttl,
		HTTPTimeout: timeout,
		UploadPath:  uploadPath,
	}, nil
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

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
