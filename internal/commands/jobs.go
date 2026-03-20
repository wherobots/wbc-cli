package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"

	"wherobots/cli/internal/config"
	"wherobots/cli/internal/executor"
	"wherobots/cli/internal/spec"
)

const (
	defaultRunRegion   = "aws-us-west-2"
	defaultRunTimeout  = 3600
	defaultListLimit   = 20
	defaultLogsPollSec = 2.0

	outputText = "text"
	outputJSON = "json"
)

var terminalStatuses = map[string]struct{}{
	"COMPLETED": {},
	"FAILED":    {},
	"CANCELLED": {},
}

type jobsRunner struct {
	cfg             config.Config
	runtime         *spec.RuntimeSpec
	client          *http.Client
	createRun       *spec.Operation
	createUploadURL *spec.Operation
	getDirectory    *spec.Operation
	getIntegration  *spec.Operation
	getOrganization *spec.Operation
	getRun          *spec.Operation
	getRunLogs      *spec.Operation
	getRunMetrics   *spec.Operation
	listRuns        *spec.Operation
}

func addJobsCustomCommands(root *cobra.Command, cfg config.Config, runtimeSpec *spec.RuntimeSpec, client *http.Client) {
	runner, ok := newJobsRunner(cfg, runtimeSpec, client)
	if !ok {
		return
	}

	jobsCmd := &cobra.Command{
		Use:           "job-runs",
		Short:         "Custom job-runs workflows",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	jobsCmd.AddCommand(runner.newCreateCommand())
	jobsCmd.AddCommand(runner.newLogsCommand())
	jobsCmd.AddCommand(runner.newListCommand())
	jobsCmd.AddCommand(runner.newRunningAliasCommand())
	jobsCmd.AddCommand(runner.newFailedAliasCommand())
	jobsCmd.AddCommand(runner.newCompletedAliasCommand())
	if runner.getRunMetrics != nil {
		jobsCmd.AddCommand(runner.newMetricsCommand())
	}
	root.AddCommand(jobsCmd)
}

func newJobsRunner(cfg config.Config, runtimeSpec *spec.RuntimeSpec, client *http.Client) (*jobsRunner, bool) {
	if runtimeSpec == nil {
		return nil, false
	}

	r := &jobsRunner{
		cfg:             cfg,
		runtime:         runtimeSpec,
		client:          client,
		createRun:       findOperation(runtimeSpec, "POST", "/runs"),
		createUploadURL: findOperation(runtimeSpec, "POST", "/files/upload-url"),
		getDirectory:    findOperation(runtimeSpec, "GET", "/files/dir"),
		getIntegration:  findOperation(runtimeSpec, "GET", "/files/integration-dir"),
		getOrganization: findOperation(runtimeSpec, "GET", "/organization"),
		getRun:          findOperation(runtimeSpec, "GET", "/runs/{run_id}"),
		getRunLogs:      findOperation(runtimeSpec, "GET", "/runs/{run_id}/logs"),
		getRunMetrics:   findOperation(runtimeSpec, "GET", "/runs/{run_id}/metrics"),
		listRuns:        findOperation(runtimeSpec, "GET", "/runs"),
	}

	if r.createRun == nil || r.getRun == nil || r.getRunLogs == nil || r.listRuns == nil || r.createUploadURL == nil || r.getDirectory == nil || r.getOrganization == nil {
		return nil, false
	}
	return r, true
}

func findOperation(runtimeSpec *spec.RuntimeSpec, method, path string) *spec.Operation {
	if runtimeSpec == nil {
		return nil
	}
	for _, op := range runtimeSpec.Operations {
		if op == nil {
			continue
		}
		if strings.EqualFold(op.Method, method) && op.Path == path {
			return op
		}
	}
	return nil
}

func (r *jobsRunner) newCreateCommand() *cobra.Command {
	var (
		name         string
		runtimeID    string
		runRegion    string
		timeoutSec   int
		argsRaw      string
		sparkConfigs []string
		depPypi      []string
		depFiles     []string
		jarMainClass string
		watch        bool
		noUpload     bool
		uploadPath   string
		output       string
	)

	cmd := &cobra.Command{
		Use:           "create <script>",
		Short:         "Create a job run",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			script := strings.TrimSpace(args[0])
			if script == "" {
				return fmt.Errorf("script is required")
			}
			if output != outputText && output != outputJSON {
				return fmt.Errorf("invalid --output %q (expected text|json)", output)
			}

			resolvedScript, err := r.prepareScript(cmd.Context(), script, name, noUpload, uploadPath)
			if err != nil {
				return err
			}

			payload, err := buildRunPayload(resolvedScript, name, runtimeID, timeoutSec, argsRaw, sparkConfigs, depPypi, depFiles, jarMainClass)
			if err != nil {
				return err
			}

			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("encode run payload: %w", err)
			}

			respBody, err := r.execOnce(cmd.Context(), r.createRun, nil, []executor.QueryPair{{Key: "region", Value: runRegion}}, string(payloadJSON))
			if err != nil {
				return err
			}

			runID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
			if runID == "" {
				return fmt.Errorf("create run response is missing id")
			}

			if !watch {
				if output == outputJSON {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
					return err
				}
				return writeRunSummary(cmd.OutOrStdout(), respBody)
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Submitted run %s\n", runID)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Streaming logs (Ctrl+C to detach)...\n\n")

			detached, err := r.followLogs(cmd, runID, defaultLogsPollSec)
			if err != nil {
				return err
			}
			if detached {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Detached.")
				return err
			}

			finalBody, err := r.execWithRetry(cmd.Context(), r.getRun, []string{runID}, nil, "")
			if err != nil {
				return err
			}

			finalStatus := strings.ToUpper(strings.TrimSpace(gjson.GetBytes(finalBody, "status").String()))
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nRun finished: %s\n", finalStatus)

			if output == outputJSON {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(finalBody))
				if err != nil {
					return err
				}
			}

			if finalStatus == "FAILED" || finalStatus == "CANCELLED" {
				return fmt.Errorf("run finished with status %s", finalStatus)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "job name (required)")
	cmd.Flags().StringVarP(&runtimeID, "runtime", "r", "tiny", "compute runtime size")
	cmd.Flags().StringVar(&runRegion, "run-region", defaultRunRegion, "region for this run")
	cmd.Flags().IntVar(&timeoutSec, "timeout", defaultRunTimeout, "job timeout in seconds")
	cmd.Flags().StringVar(&argsRaw, "args", "", "space-separated args for the script")
	cmd.Flags().StringArrayVarP(&sparkConfigs, "spark-config", "c", nil, "spark config as key=value, repeatable")
	cmd.Flags().StringArrayVar(&depPypi, "dep-pypi", nil, "PyPI dependency as name==version, repeatable")
	cmd.Flags().StringArrayVar(&depFiles, "dep-file", nil, "file dependency S3 URI, repeatable")
	cmd.Flags().StringVar(&jarMainClass, "jar-main-class", "", "main class (required for JAR files)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "stream logs until job completes")
	cmd.Flags().BoolVar(&noUpload, "no-upload", false, "disable auto-upload of local scripts")
	cmd.Flags().StringVar(&uploadPath, "upload-path", "", "override upload root as s3://bucket/prefix")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func buildRunPayload(script, name, runtimeID string, timeoutSec int, argsRaw string, sparkConfigs, depPypi, depFiles []string, jarMainClass string) (map[string]any, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("--name is required")
	}
	if timeoutSec <= 0 {
		return nil, fmt.Errorf("--timeout must be greater than 0")
	}

	parsedArgs, err := parseScriptArgs(argsRaw)
	if err != nil {
		return nil, err
	}

	isJar := strings.EqualFold(filepath.Ext(script), ".jar")
	if isJar && strings.TrimSpace(jarMainClass) == "" {
		return nil, fmt.Errorf("--jar-main-class is required for JAR files")
	}

	payload := map[string]any{
		"runtime":        runtimeID,
		"name":           name,
		"timeoutSeconds": timeoutSec,
	}

	if isJar {
		payload["runJar"] = map[string]any{
			"uri":       script,
			"mainClass": jarMainClass,
			"args":      parsedArgs,
		}
	} else {
		payload["runPython"] = map[string]any{
			"uri":  script,
			"args": parsedArgs,
		}
	}

	environment := map[string]any{}
	if len(sparkConfigs) > 0 {
		cfgs, err := parseSparkConfigs(sparkConfigs)
		if err != nil {
			return nil, err
		}
		environment["sparkConfigs"] = cfgs
	}

	dependencies, err := parseDependencies(depPypi, depFiles)
	if err != nil {
		return nil, err
	}
	if len(dependencies) > 0 {
		environment["dependencies"] = dependencies
	}

	if len(environment) > 0 {
		payload["environment"] = environment
	}

	return payload, nil
}

func parseSparkConfigs(entries []string) (map[string]string, error) {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid --spark-config %q (expected key=value)", entry)
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out, nil
}

func parseDependencies(depPypi, depFiles []string) ([]map[string]any, error) {
	deps := make([]map[string]any, 0, len(depPypi)+len(depFiles))
	for _, dep := range depPypi {
		parts := strings.SplitN(dep, "==", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --dep-pypi %q (expected name==version)", dep)
		}
		deps = append(deps, map[string]any{
			"sourceType":     "PYPI",
			"libraryName":    strings.TrimSpace(parts[0]),
			"libraryVersion": strings.TrimSpace(parts[1]),
		})
	}

	for _, dep := range depFiles {
		if !hasAllowedDependencyExt(dep) {
			return nil, fmt.Errorf("invalid --dep-file %q (supported extensions: .jar, .whl, .zip, .json)", dep)
		}
		deps = append(deps, map[string]any{
			"sourceType": "FILE",
			"filePath":   dep,
		})
	}
	return deps, nil
}

func hasAllowedDependencyExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jar", ".whl", ".zip", ".json":
		return true
	default:
		return false
	}
}

func parseScriptArgs(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{}, nil
	}
	parsed, err := shlex.Split(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid --args: %w", err)
	}
	return parsed, nil
}

func isLocalPath(script string) bool {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(trimmed), "s3://")
}

func (r *jobsRunner) prepareScript(ctx context.Context, script, name string, noUpload bool, uploadPathOverride string) (string, error) {
	if !isLocalPath(script) {
		return script, nil
	}
	if noUpload {
		return "", fmt.Errorf("local script path requires upload; remove --no-upload or provide s3:// URI")
	}

	bucket, prefix, err := r.resolveManagedUploadTarget(ctx, uploadPathOverride)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(script)
	if err != nil {
		return "", fmt.Errorf("script file not found: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("script path is not a file: %s", script)
	}

	key := fmt.Sprintf("%s/%s/%s", strings.Trim(prefix, "/"), name, filepath.Base(script))
	respBody, err := r.execWithRetry(ctx, r.createUploadURL, nil, []executor.QueryPair{{Key: "key", Value: key}}, "")
	if err != nil {
		return "", err
	}

	uploadURL := strings.TrimSpace(gjson.GetBytes(respBody, "uploadUrl").String())
	if uploadURL == "" {
		return "", fmt.Errorf("upload URL response missing uploadUrl")
	}

	if err := executor.UploadFileToPresignedURL(ctx, r.client, uploadURL, script); err != nil {
		return "", err
	}

	return fmt.Sprintf("s3://%s/%s", bucket, key), nil
}

func (r *jobsRunner) resolveManagedUploadTarget(ctx context.Context, uploadPathOverride string) (string, string, error) {
	if bucket, prefix, ok, err := resolveUploadPath(strings.TrimSpace(uploadPathOverride)); err != nil {
		return "", "", err
	} else if ok {
		return bucket, prefix, nil
	}

	if bucket, prefix, ok, err := resolveUploadPath(strings.TrimSpace(r.cfg.UploadPath)); err != nil {
		return "", "", err
	} else if ok {
		return bucket, prefix, nil
	}

	orgBody, err := r.execWithRetry(ctx, r.getOrganization, nil, nil, "")
	if err != nil {
		return "", "", fmt.Errorf("unable to resolve managed storage directory via API: failed to fetch organization: %w", err)
	}

	bucket := strings.TrimSpace(gjson.GetBytes(orgBody, "fileStore.bucketName").String())
	// Normalize bucket: if the API returns a full S3 URI (e.g. "s3://bucket-name"),
	// extract only the bucket name portion.
	if parsedBucket, _, ok := splitS3Path(bucket); ok && parsedBucket != "" {
		bucket = parsedBucket
	}
	if bucket == "" {
		return "", "", fmt.Errorf("unable to resolve managed storage directory via API: organization fileStore bucket not available")
	}

	integrationIDs := make([]string, 0, 4)
	storageIntegrations := gjson.GetBytes(orgBody, "storageIntegrations")
	if storageIntegrations.IsArray() {
		for _, item := range storageIntegrations.Array() {
			id := strings.TrimSpace(item.Get("id").String())
			if id == "" {
				continue
			}
			path := strings.TrimSpace(item.Get("path").String())
			if strings.HasPrefix(strings.ToLower(path), "s3://"+strings.ToLower(bucket)+"/") || strings.EqualFold(path, "s3://"+bucket) {
				integrationIDs = append([]string{id}, integrationIDs...)
				continue
			}
			integrationIDs = append(integrationIDs, id)
		}
	}
	if fallbackID := strings.TrimSpace(gjson.GetBytes(orgBody, "fileStore.id").String()); fallbackID != "" {
		integrationIDs = append(integrationIDs, fallbackID)
	}

	seenIntegrationID := map[string]struct{}{}
	for _, integrationID := range integrationIDs {
		if integrationID == "" {
			continue
		}
		if _, seen := seenIntegrationID[integrationID]; seen {
			continue
		}
		seenIntegrationID[integrationID] = struct{}{}
		if r.getIntegration == nil {
			break
		}
		integrationBody, integrationErr := r.execWithRetry(ctx, r.getIntegration, nil, []executor.QueryPair{{Key: "integration_id", Value: integrationID}, {Key: "dir", Value: "/"}}, "")
		if integrationErr != nil {
			continue
		}
		path := strings.TrimSpace(gjson.GetBytes(integrationBody, "path").String())
		if path != "" {
			parsedBucket, prefix, ok := splitS3Path(path)
			if ok && parsedBucket != "" {
				if prefix == "" {
					prefix = "wherobots-jobs"
				}
				return parsedBucket, prefix, nil
			}
		}
	}

	rootDir := fmt.Sprintf("s3://%s/", bucket)
	path := ""
	dirBody, err := r.execWithRetry(ctx, r.getDirectory, nil, []executor.QueryPair{{Key: "dir", Value: rootDir}}, "")
	if err != nil {
		return "", "", fmt.Errorf("unable to resolve managed storage directory via API: %w", err)
	}
	path = strings.TrimSpace(gjson.GetBytes(dirBody, "path").String())
	if path == "" {
		return "", "", fmt.Errorf("managed storage directory response missing path")
	}

	bucket, prefix, ok := splitS3Path(path)
	if !ok {
		return "", "", fmt.Errorf("managed storage directory is not a valid s3 path: %q", path)
	}

	if bucket == "" {
		return "", "", fmt.Errorf("managed storage path missing bucket")
	}
	if prefix == "" {
		prefix = "wherobots-jobs"
	}

	return bucket, prefix, nil
}

func resolveUploadPath(raw string) (bucket, prefix string, ok bool, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false, nil
	}
	bucket, prefix, valid := splitS3Path(trimmed)
	if !valid {
		return "", "", false, fmt.Errorf("invalid upload path %q (expected s3://bucket/prefix)", trimmed)
	}
	if prefix == "" {
		prefix = "wherobots-jobs"
	}
	return bucket, prefix, true, nil
}

func splitS3Path(path string) (string, string, bool) {
	trimmed := strings.TrimSpace(path)
	if !strings.HasPrefix(strings.ToLower(trimmed), "s3://") {
		return "", "", false
	}
	withoutScheme := strings.TrimPrefix(trimmed, "s3://")
	parts := strings.SplitN(withoutScheme, "/", 2)
	if len(parts) == 0 {
		return "", "", false
	}
	bucket := strings.TrimSpace(parts[0])
	if bucket == "" {
		return "", "", false
	}
	prefix := ""
	if len(parts) == 2 {
		prefix = strings.Trim(parts[1], "/")
	}
	return bucket, prefix, true
}

func (r *jobsRunner) newLogsCommand() *cobra.Command {
	var (
		follow   bool
		tail     int
		interval float64
		output   string
	)

	cmd := &cobra.Command{
		Use:           "logs <run-id>",
		Short:         "Fetch or stream run logs",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := strings.TrimSpace(args[0])
			if runID == "" {
				return fmt.Errorf("run-id is required")
			}
			if interval <= 0 {
				return fmt.Errorf("--interval must be greater than 0")
			}
			if tail < 0 {
				return fmt.Errorf("--tail must be non-negative")
			}
			if output != outputText && output != outputJSON {
				return fmt.Errorf("invalid --output %q (expected text|json)", output)
			}

			if !follow {
				size := 1000
				if tail > 0 {
					size = tail
				}
				respBody, err := r.execWithRetry(cmd.Context(), r.getRunLogs, []string{runID}, []executor.QueryPair{{Key: "cursor", Value: "0"}, {Key: "size", Value: fmt.Sprintf("%d", size)}}, "")
				if err != nil {
					return err
				}

				if output == outputJSON {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
					return err
				}

				lines := extractLogLines(respBody)
				if tail > 0 && len(lines) > tail {
					lines = lines[len(lines)-tail:]
				}
				for _, line := range lines {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
						return err
					}
				}
				return nil
			}

			if output == outputJSON {
				return fmt.Errorf("--output json is not supported with --follow")
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Following logs for %s (Ctrl+C to detach)...\n", runID)
			detached, err := r.followLogs(cmd, runID, interval)
			if err != nil {
				return err
			}
			if detached {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Detached.")
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow logs until run completes")
	cmd.Flags().IntVarP(&tail, "tail", "t", 0, "show only the last N log lines")
	cmd.Flags().Float64Var(&interval, "interval", defaultLogsPollSec, "poll interval in seconds")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func extractLogLines(body []byte) []string {
	items := gjson.GetBytes(body, "items")
	if !items.IsArray() {
		return nil
	}
	out := make([]string, 0, len(items.Array()))
	for _, item := range items.Array() {
		line := item.Get("raw").String()
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (r *jobsRunner) newListCommand() *cobra.Command {
	var (
		statuses []string
		name     string
		after    string
		limit    int
		region   string
		output   string
	)

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List job runs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.executeList(cmd, statuses, name, after, limit, region, output)
		},
	}

	cmd.Flags().StringArrayVarP(&statuses, "status", "s", nil, "filter by status, repeatable")
	cmd.Flags().StringVar(&name, "name", "", "filter by name pattern")
	cmd.Flags().StringVar(&after, "after", "", "filter runs created after ISO timestamp")
	cmd.Flags().IntVarP(&limit, "limit", "l", defaultListLimit, "max results")
	cmd.Flags().StringVar(&region, "region", "", "filter by region")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func writeRunSummary(out io.Writer, body []byte) error {
	id := gjson.GetBytes(body, "id").String()
	name := gjson.GetBytes(body, "name").String()
	status := gjson.GetBytes(body, "status").String()
	created := gjson.GetBytes(body, "createTime").String()
	runtimeID := gjson.GetBytes(body, "payload.runtime").String()
	region := gjson.GetBytes(body, "payload.region").String()

	pairs := [][2]string{
		{"ID", id},
		{"Name", name},
		{"Status", status},
		{"Created", created},
		{"Runtime", runtimeID},
		{"Region", region},
	}
	maxKeyLen := 0
	for _, pair := range pairs {
		if pair[1] != "" && len(pair[0]) > maxKeyLen {
			maxKeyLen = len(pair[0])
		}
	}
	for _, pair := range pairs {
		if pair[1] != "" {
			padding := strings.Repeat(" ", maxKeyLen-len(pair[0]))
			if _, err := fmt.Fprintf(out, "%s:%s  %s\n", pair[0], padding, pair[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeRunListTable(out io.Writer, body []byte) error {
	items := gjson.GetBytes(body, "items")
	if !items.IsArray() || len(items.Array()) == 0 {
		_, err := fmt.Fprintln(out, "No runs found.")
		return err
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tCREATED\tRUNTIME\tREGION"); err != nil {
		return err
	}
	for _, item := range items.Array() {
		id := item.Get("id").String()
		name := item.Get("name").String()
		status := item.Get("status").String()
		created := item.Get("createTime").String()
		runtimeID := item.Get("payload.runtime").String()
		region := item.Get("payload.region").String()
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", id, name, status, created, runtimeID, region); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func (r *jobsRunner) newRunningAliasCommand() *cobra.Command {
	var (
		name   string
		after  string
		limit  int
		region string
		output string
	)

	cmd := &cobra.Command{
		Use:           "running",
		Short:         "Alias for job-runs list --status RUNNING",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.executeList(cmd, []string{"RUNNING"}, name, after, limit, region, output)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filter by name pattern")
	cmd.Flags().StringVar(&after, "after", "", "filter runs created after ISO timestamp")
	cmd.Flags().IntVarP(&limit, "limit", "l", defaultListLimit, "max results")
	cmd.Flags().StringVar(&region, "region", "", "filter by region")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func (r *jobsRunner) newFailedAliasCommand() *cobra.Command {
	var (
		name   string
		after  string
		limit  int
		region string
		output string
	)

	cmd := &cobra.Command{
		Use:           "failed",
		Short:         "Alias for job-runs list --status FAILED",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.executeList(cmd, []string{"FAILED"}, name, after, limit, region, output)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filter by name pattern")
	cmd.Flags().StringVar(&after, "after", "", "filter runs created after ISO timestamp")
	cmd.Flags().IntVarP(&limit, "limit", "l", defaultListLimit, "max results")
	cmd.Flags().StringVar(&region, "region", "", "filter by region")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func (r *jobsRunner) newCompletedAliasCommand() *cobra.Command {
	var (
		name   string
		after  string
		limit  int
		region string
		output string
	)

	cmd := &cobra.Command{
		Use:           "completed",
		Short:         "Alias for job-runs list --status COMPLETED",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.executeList(cmd, []string{"COMPLETED"}, name, after, limit, region, output)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filter by name pattern")
	cmd.Flags().StringVar(&after, "after", "", "filter runs created after ISO timestamp")
	cmd.Flags().IntVarP(&limit, "limit", "l", defaultListLimit, "max results")
	cmd.Flags().StringVar(&region, "region", "", "filter by region")
	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func (r *jobsRunner) newMetricsCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:           "metrics <run-id>",
		Short:         "Display instant metrics for a job run",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := strings.TrimSpace(args[0])
			if runID == "" {
				return fmt.Errorf("run-id is required")
			}
			if output != outputText && output != outputJSON {
				return fmt.Errorf("invalid --output %q (expected text|json)", output)
			}

			respBody, err := r.execWithRetry(cmd.Context(), r.getRunMetrics, []string{runID}, nil, "")
			if err != nil {
				return err
			}

			if output == outputJSON {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
				return err
			}

			return writeInstantMetrics(cmd.OutOrStdout(), respBody)
		},
	}

	cmd.Flags().StringVar(&output, "output", outputText, "output format: text|json")
	return cmd
}

func writeInstantMetrics(out io.Writer, body []byte) error {
	metrics := gjson.GetBytes(body, "instant_metrics")
	if !metrics.Exists() || !metrics.IsObject() {
		_, err := fmt.Fprintln(out, "No instant metrics available.")
		return err
	}

	type metricEntry struct {
		displayName string
		formatted   string
	}

	entries := make([]metricEntry, 0)
	maxNameLen := 0

	metrics.ForEach(func(_, value gjson.Result) bool {
		displayName := value.Get("display_name").String()
		if displayName == "" {
			return true
		}

		rawValue := value.Get("metric.data.value")
		format := value.Get("metric.format").String()

		var formatted string
		if !rawValue.Exists() || rawValue.Type == gjson.Null {
			formatted = "N/A"
		} else {
			formatted = formatMetricValue(rawValue.Float(), format)
		}

		if len(displayName) > maxNameLen {
			maxNameLen = len(displayName)
		}
		entries = append(entries, metricEntry{displayName: displayName, formatted: formatted})
		return true
	})

	if len(entries) == 0 {
		_, err := fmt.Fprintln(out, "No instant metrics available.")
		return err
	}

	for _, entry := range entries {
		padding := strings.Repeat(" ", maxNameLen-len(entry.displayName))
		if _, err := fmt.Fprintf(out, "%s:%s  %s\n", entry.displayName, padding, entry.formatted); err != nil {
			return err
		}
	}
	return nil
}

func formatMetricValue(value float64, format string) string {
	switch strings.ToUpper(format) {
	case "PERCENTAGE":
		return fmt.Sprintf("%.1f%%", value)
	case "CURRENCY":
		return fmt.Sprintf("$%.2f", value)
	case "BYTES":
		return formatBytes(value)
	default:
		if value == math.Trunc(value) {
			return fmt.Sprintf("%.0f", value)
		}
		return fmt.Sprintf("%g", value)
	}
}

func formatBytes(bytes float64) string {
	if bytes < 0 {
		return fmt.Sprintf("%.0f B", bytes)
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	if bytes < 1 {
		return "0 B"
	}
	exp := int(math.Log(bytes) / math.Log(1024))
	if exp >= len(units) {
		exp = len(units) - 1
	}
	val := bytes / math.Pow(1024, float64(exp))
	if val == math.Trunc(val) {
		return fmt.Sprintf("%.0f %s", val, units[exp])
	}
	return fmt.Sprintf("%.1f %s", val, units[exp])
}

func (r *jobsRunner) executeList(cmd *cobra.Command, statuses []string, name, after string, limit int, region, output string) error {
	if limit <= 0 {
		return fmt.Errorf("--limit must be greater than 0")
	}
	if output != outputText && output != outputJSON {
		return fmt.Errorf("invalid --output %q (expected text|json)", output)
	}

	query := []executor.QueryPair{{Key: "size", Value: fmt.Sprintf("%d", limit)}}
	if strings.TrimSpace(name) != "" {
		query = append(query, executor.QueryPair{Key: "name", Value: name})
	}
	if strings.TrimSpace(after) != "" {
		query = append(query, executor.QueryPair{Key: "created_after", Value: after})
	}
	if strings.TrimSpace(region) != "" {
		query = append(query, executor.QueryPair{Key: "region", Value: region})
	}
	for _, status := range statuses {
		normalized := strings.ToUpper(strings.TrimSpace(status))
		if normalized == "" {
			continue
		}
		query = append(query, executor.QueryPair{Key: "status", Value: normalized})
	}

	respBody, err := r.execWithRetry(cmd.Context(), r.listRuns, nil, query, "")
	if err != nil {
		return err
	}

	if output == outputJSON {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
		return err
	}

	return writeRunListTable(cmd.OutOrStdout(), respBody)
}

func (r *jobsRunner) followLogs(cmd *cobra.Command, runID string, intervalSec float64) (bool, error) {
	watcher, stop := newInterruptWatcher()
	defer stop()

	cursor := 0
	for {
		if watcher.Interrupted() {
			return true, nil
		}

		respBody, err := r.execWithRetry(cmd.Context(), r.getRunLogs, []string{runID}, []executor.QueryPair{{Key: "cursor", Value: fmt.Sprintf("%d", cursor)}, {Key: "size", Value: "200"}}, "")
		if err != nil {
			return false, err
		}
		for _, line := range extractLogLines(respBody) {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
				return false, err
			}
		}

		nextCursor := gjson.GetBytes(respBody, "next_page")
		if nextCursor.Exists() && nextCursor.Type != gjson.Null {
			next := int(nextCursor.Int())
			if next != cursor {
				cursor = next
			}
		}

		runBody, err := r.execWithRetry(cmd.Context(), r.getRun, []string{runID}, nil, "")
		if err != nil {
			return false, err
		}
		status := strings.ToUpper(strings.TrimSpace(gjson.GetBytes(runBody, "status").String()))
		if _, terminal := terminalStatuses[status]; terminal {
			finalBody, err := r.execWithRetry(cmd.Context(), r.getRunLogs, []string{runID}, []executor.QueryPair{{Key: "cursor", Value: fmt.Sprintf("%d", cursor)}, {Key: "size", Value: "1000"}}, "")
			if err != nil {
				return false, err
			}
			for _, line := range extractLogLines(finalBody) {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return false, err
				}
			}
			return false, nil
		}

		if interrupted := sleepWithInterrupt(watcher, time.Duration(intervalSec*float64(time.Second))); interrupted {
			return true, nil
		}
	}
}

func (r *jobsRunner) execOnce(ctx context.Context, op *spec.Operation, pathArgs []string, query []executor.QueryPair, body string) ([]byte, error) {
	req, err := executor.BuildRequest(ctx, r.cfg, r.runtime, op, pathArgs, query, body)
	if err != nil {
		return nil, err
	}
	return executor.Do(r.client, req)
}

func (r *jobsRunner) execWithRetry(ctx context.Context, op *spec.Operation, pathArgs []string, query []executor.QueryPair, body string) ([]byte, error) {
	const maxAttempts = 6

	backoff := 300 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		respBody, err := r.execOnce(ctx, op, pathArgs, query, body)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}
	return nil, lastErr
}

func isRetryable(err error) bool {
	httpErr, ok := err.(*executor.HTTPError)
	if !ok {
		return true
	}
	if httpErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return httpErr.StatusCode >= http.StatusInternalServerError
}

type interruptWatcher struct {
	ctx context.Context
}

func newInterruptWatcher() (*interruptWatcher, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	return &interruptWatcher{ctx: ctx}, stop
}

func (w *interruptWatcher) Interrupted() bool {
	select {
	case <-w.ctx.Done():
		return true
	default:
		return false
	}
}

func sleepWithInterrupt(w *interruptWatcher, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-w.ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}
