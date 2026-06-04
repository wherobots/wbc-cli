package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

const sampleErrorEnvelope = `{"errors":[{"code":"BAD_REQUEST_ERROR","message":"Bad Request","details":"InvalidInputException (No storage source found for bucket: qni7xwfc8m)","path":"/files/upload-url","suggestion":"Update your request and try again.","documentation_url":null,"field":null}],"requestId":"3629d5f8a10139a4867c043509678f05"}`

func TestHTTPErrorFormatsStandardEnvelope(t *testing.T) {
	t.Parallel()

	err := &HTTPError{StatusCode: 400, Body: []byte(sampleErrorEnvelope)}
	got := err.Error()

	if !strings.HasPrefix(got, "request failed with HTTP 400") {
		t.Fatalf("expected 'request failed with HTTP 400' prefix, got %q", got)
	}
	for _, want := range []string{
		"BAD_REQUEST_ERROR",
		"Bad Request",
		"InvalidInputException (No storage source found for bucket: qni7xwfc8m)",
		"Update your request and try again.",
		"3629d5f8a10139a4867c043509678f05",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected multi-line output, got %q", got)
	}
	if strings.Contains(got, "documentation") || strings.Contains(got, "field") {
		t.Fatalf("expected empty envelope fields to be omitted, got %q", got)
	}
}

func TestHTTPErrorFormatsMultipleEnvelopeErrors(t *testing.T) {
	t.Parallel()

	body := `{"errors":[{"code":"FIRST_ERROR","message":"first message"},{"code":"SECOND_ERROR","message":"second message","field":"name"}],"requestId":"req-1"}`
	got := (&HTTPError{StatusCode: 422, Body: []byte(body)}).Error()

	for _, want := range []string{"FIRST_ERROR", "first message", "SECOND_ERROR", "second message", "name", "req-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestHTTPErrorFallsBackOnNonJSONBody(t *testing.T) {
	t.Parallel()

	got := (&HTTPError{StatusCode: 502, Body: []byte("upstream timeout")}).Error()
	if got != "request failed with HTTP 502: upstream timeout" {
		t.Fatalf("expected raw fallback, got %q", got)
	}
}

func TestHTTPErrorFallsBackOnEmptyBody(t *testing.T) {
	t.Parallel()

	got := (&HTTPError{StatusCode: 500}).Error()
	if got != "request failed with HTTP 500" {
		t.Fatalf("expected bare fallback, got %q", got)
	}
}

func TestHTTPErrorFallsBackOnNonEnvelopeJSON(t *testing.T) {
	t.Parallel()

	got := (&HTTPError{StatusCode: 400, Body: []byte(`{"foo":"bar"}`)}).Error()
	if got != `request failed with HTTP 400: {"foo":"bar"}` {
		t.Fatalf("expected raw fallback, got %q", got)
	}
}

func TestAPIErrorDetailsExtractsDetailsFromWrappedError(t *testing.T) {
	t.Parallel()

	httpErr := &HTTPError{StatusCode: 400, Body: []byte(sampleErrorEnvelope)}
	wrapped := errors.Join(errors.New("context"), httpErr)

	detail, ok := APIErrorDetails(wrapped)
	if !ok {
		t.Fatal("expected details to be found")
	}
	if !strings.Contains(detail, "No storage source found for bucket: qni7xwfc8m") {
		t.Fatalf("unexpected detail %q", detail)
	}

	if _, ok := APIErrorDetails(errors.New("plain")); ok {
		t.Fatal("expected no details for non-HTTP error")
	}
	if _, ok := APIErrorDetails(&HTTPError{StatusCode: 502, Body: []byte("upstream timeout")}); ok {
		t.Fatal("expected no details for non-envelope body")
	}
}

func TestJSONErrorReturnsBodyAndUnwrapsHTTPError(t *testing.T) {
	t.Parallel()

	httpErr := &HTTPError{StatusCode: 400, Body: []byte(sampleErrorEnvelope)}
	jsonErr := NewJSONError(httpErr)

	if got := jsonErr.Error(); got != sampleErrorEnvelope {
		t.Fatalf("expected raw JSON body, got %q", got)
	}

	var unwrapped *HTTPError
	if !errors.As(jsonErr, &unwrapped) {
		t.Fatal("expected errors.As to recover *HTTPError")
	}
	if unwrapped.StatusCode != 400 {
		t.Fatalf("expected status 400, got %d", unwrapped.StatusCode)
	}
}

func TestJSONErrorFallsBackOnNonJSONBody(t *testing.T) {
	t.Parallel()

	jsonErr := NewJSONError(&HTTPError{StatusCode: 502, Body: []byte("upstream timeout")})
	if got := jsonErr.Error(); got != "request failed with HTTP 502: upstream timeout" {
		t.Fatalf("expected raw fallback, got %q", got)
	}
}

func TestBuildRequestInjectsPathQueryBodyAndAuth(t *testing.T) {
	t.Parallel()

	cfg := config.Config{APIKey: "abc123"}
	runtimeSpec := &spec.RuntimeSpec{BaseURL: "https://api.example.com"}
	op := &spec.Operation{
		Method:         "POST",
		Path:           "/users/{id}",
		PathParamOrder: []string{"id"},
		QueryParams: []spec.Parameter{
			{Name: "expand", Location: "query", Required: true},
		},
		RequestBody: &spec.RequestBodyInfo{
			Required:    true,
			ContentType: "application/json",
		},
	}

	req, err := BuildRequest(
		context.Background(),
		cfg,
		runtimeSpec,
		op,
		[]string{"u-1"},
		[]QueryPair{{Key: "expand", Value: "true"}},
		`{"name":"alice"}`,
	)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if req.URL.String() != "https://api.example.com/users/u-1?expand=true" {
		t.Fatalf("url = %s", req.URL.String())
	}
	if got := req.Header.Get("x-api-key"); got != "abc123" {
		t.Fatalf("x-api-key = %q, want %q", got, "abc123")
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestBuildRequestMissingRequiredQueryReturnsError(t *testing.T) {
	t.Parallel()

	cfg := config.Config{APIKey: "abc123"}
	runtimeSpec := &spec.RuntimeSpec{BaseURL: "https://api.example.com"}
	op := &spec.Operation{
		Method:      "GET",
		Path:        "/users",
		QueryParams: []spec.Parameter{{Name: "limit", Location: "query", Required: true}},
	}

	_, err := BuildRequest(context.Background(), cfg, runtimeSpec, op, nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), `missing required query parameter "limit"`) {
		t.Fatalf("expected required query error, got %v", err)
	}
}

func TestBuildRequestMissingAPIKeyReturnsError(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	runtimeSpec := &spec.RuntimeSpec{BaseURL: "https://api.example.com"}
	op := &spec.Operation{Method: "GET", Path: "/users"}

	_, err := BuildRequest(context.Background(), cfg, runtimeSpec, op, nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), "WHEROBOTS_API_KEY") {
		t.Fatalf("expected API key error, got %v", err)
	}
}
