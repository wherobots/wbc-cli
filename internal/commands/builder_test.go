package commands

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

func TestRootTreeOutput(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "wherobots", HTTPTimeout: time.Second}
	runtimeSpec := &spec.RuntimeSpec{
		BaseURL: "https://api.example.com",
		Operations: []*spec.Operation{
			{Method: "GET", Path: "/users"},
			{Method: "GET", Path: "/users/{id}", PathParamOrder: []string{"id"}},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--tree"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "wherobots\n") || !strings.Contains(got, "  users\n") || !strings.Contains(got, "    get\n") || !strings.Contains(got, "    list\n") {
		t.Fatalf("tree output missing expected nodes:\n%s", got)
	}
}

func TestDryRunOutputsCurl(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "wherobots", APIKey: "test-key", HTTPTimeout: time.Second}
	runtimeSpec := &spec.RuntimeSpec{
		BaseURL: "https://api.example.com",
		Operations: []*spec.Operation{
			{
				Method:         "GET",
				Path:           "/users/{id}",
				PathParamOrder: []string{"id"},
			},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"users", "get", "42", "--dry-run", "-q", "expand=true"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := strings.TrimSpace(out.String())
	if !strings.HasPrefix(got, "curl -X GET 'https://api.example.com/users/42?expand=true'") {
		t.Fatalf("unexpected dry-run output: %s", got)
	}
}

func TestInvalidArgsReturnsUsageHint(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "wherobots", HTTPTimeout: time.Second}
	runtimeSpec := &spec.RuntimeSpec{
		BaseURL: "https://api.example.com",
		Operations: []*spec.Operation{
			{
				Method:         "GET",
				Path:           "/users/{id}",
				PathParamOrder: []string{"id"},
				RequestBody: &spec.RequestBodyInfo{
					Required: true,
					RequiredFields: []spec.BodyField{
						{Name: "name", Type: "string"},
					},
				},
			},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"users", "get"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected Execute() error")
	}

	message := err.Error()
	if !strings.Contains(message, "Did you mean to use the body:") ||
		!strings.Contains(message, "Required Path Params: [id]") ||
		!strings.Contains(message, "Required Body Params: [name]") {
		t.Fatalf("expected hint in error, got: %s", message)
	}
}
