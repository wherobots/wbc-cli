package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

func TestJobsRunNoWatchPrintsRunID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"run-123","status":"PENDING"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"jobs", "run", "s3://bucket/script.py",
		"--name", "test-job-001",
		"--runtime", "tiny",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "run-123" {
		t.Fatalf("expected run id output, got %q", got)
	}
}

func TestJobsRunWatchReturnsErrorOnFailedStatus(t *testing.T) {
	t.Parallel()

	var statusCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			_, _ = io.WriteString(w, `{"id":"run-xyz","status":"PENDING"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/runs/run-xyz/logs":
			if r.URL.Query().Get("size") == "1000" {
				_, _ = io.WriteString(w, `{"items":[{"raw":"final"}],"current_page":1,"next_page":null}`)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"raw":"line-1"}],"current_page":0,"next_page":1}`)
		case r.Method == http.MethodGet && r.URL.Path == "/runs/run-xyz":
			statusCalls++
			if statusCalls == 1 {
				_, _ = io.WriteString(w, `{"id":"run-xyz","status":"FAILED"}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"run-xyz","status":"FAILED"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{
		"jobs", "run", "s3://bucket/script.py",
		"--name", "test-job-001",
		"--watch",
	})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for failed run status")
	}
	if !strings.Contains(err.Error(), "run finished with status FAILED") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "line-1") {
		t.Fatalf("expected streamed log line, got %q", out.String())
	}
}

func TestJobsLogsJsonOutput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/runs/run-555/logs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"items":[{"raw":"a"},{"raw":"b"}],"current_page":0,"next_page":null}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "logs", "run-555", "--output", "json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := strings.TrimSpace(out.String())
	if !gjsonValid(got) {
		t.Fatalf("expected JSON output, got %q", got)
	}
}

func TestJobsListDefaultsToJson(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/runs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"items":[{"id":"run-1","name":"a","status":"RUNNING","createTime":"2026-01-01T00:00:00Z","payload":{"runtime":"tiny","region":"aws-us-west-2"}}],"total":1,"next_page":null}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !gjsonValid(strings.TrimSpace(out.String())) {
		t.Fatalf("expected json output by default, got %q", out.String())
	}
}

func TestJobsRunningAliasFiltersStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/runs" {
			if got := r.URL.Query().Get("status"); got != "RUNNING" {
				t.Fatalf("expected status RUNNING, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"items":[],"total":0,"next_page":null}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "running", "--output", "json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestBuildRunPayloadRejectsBadDependency(t *testing.T) {
	t.Parallel()

	_, err := buildRunPayload(
		"s3://bucket/script.py",
		"test-job-001",
		"tiny",
		3600,
		"",
		nil,
		nil,
		[]string{"s3://bucket/data.txt"},
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "supported extensions") {
		t.Fatalf("expected dependency validation error, got %v", err)
	}
}

func buildJobsTestRoot(baseURL string) *cobra.Command {
	cfg := config.Config{
		AppName:     "wherobots",
		APIKey:      "test-key",
		HTTPTimeout: time.Second,
	}
	runtime := &spec.RuntimeSpec{
		BaseURL: baseURL,
		Operations: []*spec.Operation{
			{
				Method: "POST",
				Path:   "/runs",
				QueryParams: []spec.Parameter{
					{Name: "region", Location: "query", Required: true, Type: "string"},
				},
				RequestBody: &spec.RequestBodyInfo{Required: true, ContentType: "application/json", SchemaType: "object"},
			},
			{
				Method:         "GET",
				Path:           "/runs/{run_id}",
				PathParamOrder: []string{"run_id"},
				PathParams:     []spec.Parameter{{Name: "run_id", Location: "path", Required: true, Type: "string"}},
			},
			{
				Method:         "GET",
				Path:           "/runs/{run_id}/logs",
				PathParamOrder: []string{"run_id"},
				PathParams:     []spec.Parameter{{Name: "run_id", Location: "path", Required: true, Type: "string"}},
			},
			{
				Method: "GET",
				Path:   "/runs",
			},
		},
	}
	return BuildRootCommand(cfg, runtime)
}

func gjsonValid(raw string) bool {
	var payload any
	return json.Unmarshal([]byte(raw), &payload) == nil
}
