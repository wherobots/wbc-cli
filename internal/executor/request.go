package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

type QueryPair struct {
	Key   string
	Value string
}

type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("request failed with HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("request failed with HTTP %d: %s", e.StatusCode, strings.TrimSpace(string(e.Body)))
}

func BuildRequest(
	ctx context.Context,
	cfg config.Config,
	runtimeSpec *spec.RuntimeSpec,
	op *spec.Operation,
	pathArgs []string,
	queryPairs []QueryPair,
	jsonBody string,
) (*http.Request, error) {
	if runtimeSpec == nil || op == nil {
		return nil, fmt.Errorf("missing runtime operation context")
	}
	if runtimeSpec.BaseURL == "" {
		return nil, fmt.Errorf("missing base URL (no OpenAPI servers and WHEROBOTS_API_URL has no resolvable host)")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("WHEROBOTS_API_KEY is required")
	}
	if len(pathArgs) != len(op.PathParamOrder) {
		return nil, fmt.Errorf("expected %d path arguments, got %d", len(op.PathParamOrder), len(pathArgs))
	}

	baseURL, err := url.Parse(runtimeSpec.BaseURL)
	if err != nil || !baseURL.IsAbs() {
		return nil, fmt.Errorf("invalid base URL %q", runtimeSpec.BaseURL)
	}

	resolvedPath := op.Path
	for idx, paramName := range op.PathParamOrder {
		resolvedPath = strings.ReplaceAll(resolvedPath, "{"+paramName+"}", url.PathEscape(pathArgs[idx]))
	}
	if strings.Contains(resolvedPath, "{") {
		return nil, fmt.Errorf("unresolved path parameters in %q", resolvedPath)
	}

	relativePath, err := url.Parse(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("parse operation path %q: %w", resolvedPath, err)
	}
	fullURL := baseURL.ResolveReference(relativePath)

	requiredQuery := make(map[string]struct{}, len(op.RequiredQueryParamNames()))
	for _, name := range op.RequiredQueryParamNames() {
		requiredQuery[name] = struct{}{}
	}

	seenQuery := make(map[string]struct{}, len(queryPairs))
	queryValues := fullURL.Query()
	for _, pair := range queryPairs {
		if pair.Key == "" {
			return nil, fmt.Errorf("query key cannot be empty")
		}
		queryValues.Set(pair.Key, pair.Value)
		seenQuery[pair.Key] = struct{}{}
	}
	for required := range requiredQuery {
		if _, exists := seenQuery[required]; !exists {
			return nil, fmt.Errorf("missing required query parameter %q", required)
		}
	}
	fullURL.RawQuery = queryValues.Encode()

	body := strings.TrimSpace(jsonBody)
	if body != "" && !gjson.Valid(body) {
		return nil, fmt.Errorf("--json must be valid JSON")
	}
	if op.RequestBody != nil && op.RequestBody.Required && body == "" {
		return nil, fmt.Errorf("request body is required")
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, fullURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != "" {
		contentType := "application/json"
		if op.RequestBody != nil && op.RequestBody.ContentType != "" {
			contentType = op.RequestBody.ContentType
		}
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("x-api-key", cfg.APIKey)

	return req, nil
}

func Do(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: body}
	}
	return body, nil
}
