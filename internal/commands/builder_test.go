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
	if !strings.Contains(got, "wherobots\n") || !strings.Contains(got, "  api\n") || !strings.Contains(got, "    users\n") || !strings.Contains(got, "      get\n") || !strings.Contains(got, "      list\n") {
		t.Fatalf("tree output missing expected nodes:\n%s", got)
	}
}

func TestExcludedOperationsAbsentFromTree(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "wherobots", HTTPTimeout: time.Second}
	runtimeSpec := &spec.RuntimeSpec{
		BaseURL: "https://api.example.com",
		Operations: []*spec.Operation{
			{Method: "GET", Path: "/catalogs", Summary: "List catalogs"},
			{Method: "POST", Path: "/management/org", Summary: "Superuser action", Excluded: true},
			{Method: "POST", Path: "/files/upload-url", Summary: "Legacy upload", Excluded: true},
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

	tree := out.String()

	// Visible endpoint should be present
	if !strings.Contains(tree, "catalogs") {
		t.Errorf("tree should contain 'catalogs':\n%s", tree)
	}
	// Excluded endpoints must not appear
	if strings.Contains(tree, "management") {
		t.Errorf("tree must not contain excluded 'management':\n%s", tree)
	}
	if strings.Contains(tree, "files") {
		t.Errorf("tree must not contain excluded 'files':\n%s", tree)
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
				PathParams:     []spec.Parameter{{Name: "id", Location: "path", Required: true, Type: "string"}},
				PathParamOrder: []string{"id"},
				QueryParams:    []spec.Parameter{{Name: "expand", Location: "query", Required: false, Type: "boolean"}},
			},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"api", "users", "get", "--id", "42", "--expand", "true", "--dry-run"})

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
				PathParams:     []spec.Parameter{{Name: "id", Location: "path", Required: true, Type: "string"}},
				PathParamOrder: []string{"id"},
				RequestBody: &spec.RequestBodyInfo{
					Required:   true,
					SchemaType: "object",
					Fields: []spec.BodyField{
						{Name: "name", Type: "string", Required: true},
					},
				},
			},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"api", "users", "get"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected Execute() error")
	}

	message := err.Error()
	if !strings.Contains(message, "Did you mean to use the body:") ||
		!strings.Contains(message, "Required Path Params: [id]") ||
		!strings.Contains(message, "Required Body Params: [name]") ||
		!strings.Contains(message, "Expected Types:") {
		t.Fatalf("expected hint in error, got: %s", message)
	}
}

func TestHelpShowsTypedFlagSamples(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "wherobots", HTTPTimeout: time.Second}
	runtimeSpec := &spec.RuntimeSpec{
		BaseURL: "https://api.example.com",
		Operations: []*spec.Operation{
			{
				Method:         "PATCH",
				Path:           "/users/{id}",
				PathParams:     []spec.Parameter{{Name: "id", Location: "path", Required: true, Type: "string"}},
				PathParamOrder: []string{"id"},
				QueryParams:    []spec.Parameter{{Name: "limit", Location: "query", Required: false, Type: "integer"}},
				RequestBody: &spec.RequestBodyInfo{
					Required:   true,
					SchemaType: "object",
					Fields: []spec.BodyField{
						{Name: "enabled", Type: "boolean", Required: true},
						{Name: "metadata", Type: "object", Required: false},
					},
				},
			},
		},
	}

	root := BuildRootCommand(cfg, runtimeSpec)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"api", "users", "update", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := out.String()
	if !strings.Contains(help, "--limit string") ||
		!strings.Contains(help, "sample: 0") ||
		!strings.Contains(help, "--metadata-json string") ||
		!strings.Contains(help, "--json '{\"enabled\":false}'") {
		t.Fatalf("help missing expected typed guidance:\n%s", help)
	}
}
