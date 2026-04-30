package executor

import (
	"context"
	"strings"
	"testing"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

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
