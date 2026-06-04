package executor

import (
	"context"
	"errors"
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
	if env, ok := parseAPIErrorEnvelope(e.Body); ok {
		return formatAPIError(e.StatusCode, env)
	}
	if len(e.Body) == 0 {
		return fmt.Sprintf("request failed with HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("request failed with HTTP %d: %s", e.StatusCode, strings.TrimSpace(string(e.Body)))
}

// apiErrorDetail mirrors one entry of the standard Wherobots API error
// envelope: {"errors":[{code,message,details,suggestion,...}],"requestId"}.
type apiErrorDetail struct {
	Code       string
	Message    string
	Details    string
	Suggestion string
	Field      string
	DocURL     string
}

type apiErrorEnvelope struct {
	Errors    []apiErrorDetail
	RequestID string
}

// parseAPIErrorEnvelope reports ok only when body is JSON containing a
// non-empty "errors" array in the standard envelope shape.
func parseAPIErrorEnvelope(body []byte) (apiErrorEnvelope, bool) {
	if !gjson.ValidBytes(body) {
		return apiErrorEnvelope{}, false
	}
	items := gjson.GetBytes(body, "errors")
	if !items.IsArray() || len(items.Array()) == 0 {
		return apiErrorEnvelope{}, false
	}

	env := apiErrorEnvelope{RequestID: strings.TrimSpace(gjson.GetBytes(body, "requestId").String())}
	for _, item := range items.Array() {
		env.Errors = append(env.Errors, apiErrorDetail{
			Code:       strings.TrimSpace(item.Get("code").String()),
			Message:    strings.TrimSpace(item.Get("message").String()),
			Details:    strings.TrimSpace(item.Get("details").String()),
			Suggestion: strings.TrimSpace(item.Get("suggestion").String()),
			Field:      strings.TrimSpace(item.Get("field").String()),
			DocURL:     strings.TrimSpace(item.Get("documentation_url").String()),
		})
	}
	return env, true
}

func formatAPIError(statusCode int, env apiErrorEnvelope) string {
	var b strings.Builder
	fmt.Fprintf(&b, "request failed with HTTP %d", statusCode)
	if len(env.Errors) == 1 && env.Errors[0].Code != "" {
		fmt.Fprintf(&b, " (%s)", env.Errors[0].Code)
	}

	appendLine := func(label, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "\n  %-11s %s", label+":", value)
	}

	for idx, detail := range env.Errors {
		if len(env.Errors) > 1 {
			fmt.Fprintf(&b, "\nerror %d (%s)", idx+1, detail.Code)
		}
		appendLine("message", detail.Message)
		appendLine("details", detail.Details)
		appendLine("suggestion", detail.Suggestion)
		appendLine("field", detail.Field)
		appendLine("docs", detail.DocURL)
	}
	appendLine("request id", env.RequestID)
	return b.String()
}

// APIErrorDetails returns the "details" of the first envelope error when err
// wraps an *HTTPError whose body is a standard API error envelope.
func APIErrorDetails(err error) (string, bool) {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return "", false
	}
	env, ok := parseAPIErrorEnvelope(httpErr.Body)
	if !ok {
		return "", false
	}
	return env.Errors[0].Details, true
}

// JSONError preserves the raw API error body so machine-oriented commands can
// emit the JSON envelope verbatim instead of the human-readable rendering.
type JSONError struct {
	httpErr *HTTPError
}

// NewJSONError wraps an *HTTPError so Error() returns the raw JSON body.
func NewJSONError(httpErr *HTTPError) *JSONError {
	return &JSONError{httpErr: httpErr}
}

func (e *JSONError) Error() string {
	if body := strings.TrimSpace(string(e.httpErr.Body)); body != "" && gjson.Valid(body) {
		return body
	}
	return e.httpErr.Error()
}

func (e *JSONError) Unwrap() error {
	return e.httpErr
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
	if err := cfg.RequireAPIKey(); err != nil {
		return nil, err
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
