package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
		case r.Method == http.MethodPost && r.URL.Path == "/files/upload-url":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"uploadUrl":"https://example.com/upload"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/dir":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"name":"root","path":"s3://managed-bucket/customer/root"}`)
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
		"--upload-path", "s3://override-bucket/custom/prefix",
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
		case r.Method == http.MethodPost && r.URL.Path == "/files/upload-url":
			_, _ = io.WriteString(w, `{"uploadUrl":"https://example.com/upload"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/dir":
			_, _ = io.WriteString(w, `{"name":"root","path":"s3://managed-bucket/customer/root"}`)
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

func TestJobsRunAutoUploadLocalScript(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := dir + "/script.py"
	if err := os.WriteFile(script, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var sawUpload bool
	var sawCreateRun bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/files/upload-url":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"uploadUrl":%q}`, serverURLWithPath(serverURLFromRequest(r), "/upload")))
		case r.Method == http.MethodGet && r.URL.Path == "/files/dir":
			_, _ = io.WriteString(w, `{"name":"root","path":"s3://managed-bucket/customer/root"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			sawUpload = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			sawCreateRun = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "s3://managed-bucket/customer/root/test-job-001/script.py") {
				t.Fatalf("expected auto-uploaded s3 URI in payload, got %s", string(body))
			}
			_, _ = io.WriteString(w, `{"id":"run-auto","status":"PENDING"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "run", script, "--name", "test-job-001"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !sawUpload || !sawCreateRun {
		t.Fatalf("expected upload and create-run calls; upload=%v create=%v", sawUpload, sawCreateRun)
	}
}

func TestJobsRunNoUploadWithLocalScriptFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := dir + "/script.py"
	if err := os.WriteFile(script, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/files/dir" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	root := buildJobsTestRootWithConfig(server.URL, func(cfg *config.Config) {
		cfg.S3Bucket = "bucket-test"
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "run", script, "--name", "test-job-001", "--no-upload"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "remove --no-upload") {
		t.Fatalf("expected no-upload validation error, got %v", err)
	}
}

func TestJobsRunUsesUploadPathFlagOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := dir + "/script.py"
	if err := os.WriteFile(script, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var sawDirLookup bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/files/dir":
			sawDirLookup = true
			_, _ = io.WriteString(w, `{"name":"root","path":"s3://managed-bucket/customer/root"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/files/upload-url":
			if got := r.URL.Query().Get("key"); !strings.HasPrefix(got, "flag-prefix/") {
				t.Fatalf("expected key from upload-path flag, got %q", got)
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"uploadUrl":%q}`, serverURLWithPath(serverURLFromRequest(r), "/upload")))
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "s3://flag-bucket/flag-prefix/test-job-001/script.py") {
				t.Fatalf("expected run payload to use upload-path flag, got %s", string(body))
			}
			_, _ = io.WriteString(w, `{"id":"run-flag","status":"PENDING"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := buildJobsTestRoot(server.URL)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "run", script, "--name", "test-job-001", "--upload-path", "s3://flag-bucket/flag-prefix"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if sawDirLookup {
		t.Fatalf("expected upload-path override to skip /files/dir lookup")
	}
}

func TestJobsRunUsesUploadPathEnvOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := dir + "/script.py"
	if err := os.WriteFile(script, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var sawDirLookup bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/files/dir":
			sawDirLookup = true
			_, _ = io.WriteString(w, `{"name":"root","path":"s3://managed-bucket/customer/root"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/files/upload-url":
			if got := r.URL.Query().Get("key"); !strings.HasPrefix(got, "env-prefix/") {
				t.Fatalf("expected key from upload-path env, got %q", got)
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"uploadUrl":%q}`, serverURLWithPath(serverURLFromRequest(r), "/upload")))
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "s3://env-bucket/env-prefix/test-job-001/script.py") {
				t.Fatalf("expected run payload to use upload-path env, got %s", string(body))
			}
			_, _ = io.WriteString(w, `{"id":"run-env","status":"PENDING"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := buildJobsTestRootWithConfig(server.URL, func(cfg *config.Config) {
		cfg.UploadPath = "s3://env-bucket/env-prefix"
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "run", script, "--name", "test-job-001"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if sawDirLookup {
		t.Fatalf("expected upload-path env override to skip /files/dir lookup")
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
	return buildJobsTestRootWithConfig(baseURL, nil)
}

func buildJobsTestRootWithConfig(baseURL string, mutate func(*config.Config)) *cobra.Command {
	cfg := config.Config{
		AppName:     "wherobots",
		APIKey:      "test-key",
		HTTPTimeout: time.Second,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	runtime := &spec.RuntimeSpec{
		BaseURL: baseURL,
		Operations: []*spec.Operation{
			{
				Method: "GET",
				Path:   "/files/dir",
				QueryParams: []spec.Parameter{
					{Name: "dir", Location: "query", Required: true, Type: "string"},
				},
			},
			{
				Method: "POST",
				Path:   "/files/upload-url",
				QueryParams: []spec.Parameter{
					{Name: "key", Location: "query", Required: true, Type: "string"},
				},
			},
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

func serverURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func serverURLWithPath(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

func gjsonValid(raw string) bool {
	var payload any
	return json.Unmarshal([]byte(raw), &payload) == nil
}
