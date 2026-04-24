# wherobots CLI

A command-line interface for the [Wherobots](https://wherobots.com) Cloud API. Submit and manage Spark job runs, stream logs, and access the full Wherobots API surface — all from your terminal.

## Prerequisites

- A Wherobots Cloud account and API key
- **Go 1.25.7+** (only if building from source)

## Installation

### Quick install (curl | bash)

```bash
curl -fsSL https://raw.githubusercontent.com/wherobots/wbc-cli/main/scripts/install-release.sh | bash
```

This downloads the latest release binary for your OS/arch, verifies its SHA-256 checksum, and installs it to `~/.local/bin/wherobots`.

To pass options (e.g. a custom install directory or release tag), use `bash -s --`:

```bash
curl -fsSL https://raw.githubusercontent.com/wherobots/wbc-cli/main/scripts/install-release.sh \
  | bash -s -- --install-dir /usr/local/bin --tag latest-prerelease
```

Available flags: `--install-dir`, `--tag`, `--repo`, `--binary-name`, `--skip-checksum`.

### Build from source

```bash
git clone https://github.com/wherobots/wbc-cli.git
cd wbc-cli
make build        # produces bin/wherobots
```

## Getting started

1. **Set your API key** (required for all commands):

   ```bash
   export WHEROBOTS_API_KEY='<your-api-key>'
   ```

2. **Explore available commands:**

   ```bash
   wherobots --help
   wherobots --tree          # print the full command tree
   ```

3. **Submit a job run:**

   ```bash
   wherobots job-runs create s3://bucket/script.py --name my-job-001 --watch
   ```

## Commands

The CLI has two command groups:

| Group | Description |
|-------|-------------|
| `wherobots job-runs <subcommand>` | Purpose-built commands for creating, monitoring, and listing job runs. |
| `wherobots api <resource> ... <verb>` | Dynamically generated commands covering every Wherobots API endpoint. |

### `job-runs` — Job run management

Curated commands for the most common job-run workflows: submitting runs, streaming logs, listing by status, and viewing metrics.

#### `job-runs create`

Submit a new job run. Accepts an S3 URI or a local file path (which is auto-uploaded to your Wherobots managed directory).

```bash
# submit from an S3 path
wherobots job-runs create s3://bucket/script.py --name my-job-001

# submit a local script (auto-uploaded)
wherobots job-runs create ./script.py --name my-job-001

# submit and stream logs until the run reaches a terminal status
wherobots job-runs create s3://bucket/script.py --name my-job-001 --watch

# override the upload destination for local files
wherobots job-runs create ./script.py --name my-job-001 --upload-path s3://my-bucket/custom/root
```

| Flag | Description | Default |
|------|-------------|---------|
| `-n, --name` | **Required.** Name for the job run. | — |
| `-r, --runtime` | Wherobots runtime size. | `tiny` |
| `--run-region` | Region to run the job in. | `aws-us-west-2` |
| `--timeout` | Run timeout in seconds. | `3600` |
| `--args` | Arguments passed to the script. | — |
| `-c, --spark-config` | Repeatable Spark config key=value pairs. | — |
| `--dep-pypi` | PyPI dependency to install. | — |
| `--dep-file` | File dependency to include. | — |
| `--jar-main-class` | Main class for JAR jobs. | — |
| `-w, --watch` | Stream logs after submission until the run completes. | `false` |
| `--no-upload` | Skip auto-upload of local files. | `false` |
| `--upload-path` | Custom S3 root for uploading local files. | — |
| `--output` | Output format: `text` or `json`. | `json` |

#### `job-runs logs`

Fetch or stream logs for a job run.

```bash
# fetch logs once
wherobots job-runs logs <run-id>

# stream logs until the run completes
wherobots job-runs logs <run-id> --follow

# show only the last 50 lines
wherobots job-runs logs <run-id> --tail 50
```

| Flag | Description | Default |
|------|-------------|---------|
| `-f, --follow` | Stream logs continuously until the run finishes. | `false` |
| `-t, --tail` | Number of most recent lines to display. | — |
| `--interval` | Poll interval in seconds (used with `--follow`). | `2.0` |
| `--output` | Output format: `text` or `json`. | `text` |

#### `job-runs list`

List job runs, optionally filtered by status, name, or region.

```bash
# list recent runs (JSON output)
wherobots job-runs list

# human-readable table
wherobots job-runs list --output text

# filter by status
wherobots job-runs list --status RUNNING --status FAILED
```

| Flag | Description | Default |
|------|-------------|---------|
| `-s, --status` | Repeatable status filter (e.g. `RUNNING`, `FAILED`, `COMPLETED`). | — |
| `--name` | Filter by run name. | — |
| `--after` | Return runs created after this cursor/timestamp. | — |
| `-l, --limit` | Maximum number of results. | `20` |
| `--region` | Filter by region. | — |
| `--output` | Output format: `text` or `json`. | `json` |

#### `job-runs running` / `job-runs failed` / `job-runs completed`

Shorthand aliases equivalent to `job-runs list --status RUNNING`, `--status FAILED`, or `--status COMPLETED`. They accept the same flags as `job-runs list` except `--status`.

```bash
wherobots job-runs running
wherobots job-runs failed --output text
wherobots job-runs completed --limit 5
```

#### `job-runs metrics`

Display instant metrics (CPU, memory, etc.) for a running or recently completed job run.

```bash
wherobots job-runs metrics <run-id>
wherobots job-runs metrics <run-id> --output text
```

| Flag | Description | Default |
|------|-------------|---------|
| `--output` | Output format: `text` or `json`. | `json` |

### `api` — Full Wherobots API access

The `api` command group is **generated at runtime** from the Wherobots OpenAPI 3.x specification. Every endpoint Wherobots exposes is available as a CLI command — no CLI update required when new API endpoints are released.

> **Note:** Because these commands are generated dynamically from the API spec, the exact set of available resources and verbs may change as the Wherobots API evolves.

```bash
# discover all available api commands
wherobots api --tree

# general form
wherobots api <resource> [<sub-resource> ...] <verb> [flags]
```

**How it works:**

- Resource hierarchy is derived from API URL paths.
- Verbs are derived from `operationId` or inferred from the HTTP method (`GET` → `list`/`get`, `POST` → `create`, etc.).
- Path and query parameters become named flags (e.g. `--id`, `--limit`).
- Object/array request body fields become `*-json` flags (e.g. `--metadata-json '{"k":"v"}'`).
- Run `--help` on any generated command to see all available flags with types and examples.

**Common flags for api commands:**

| Flag | Description |
|------|-------------|
| `--json '<raw-json>'` | Raw JSON request body (overrides individual body-field flags). |
| `-q, --query key=value` | Repeatable query parameter (last value wins for duplicate keys). |
| `--dry-run` | Print the equivalent `curl` command instead of executing the request. |
| `--tree` | Print the command tree from this point. |

## Configuration

All configuration is done through environment variables.

| Variable | Required | Description | Default |
|----------|----------|-------------|---------|
| `WHEROBOTS_API_KEY` | **Yes** | Your Wherobots API key. Sent as `x-api-key` header. | — |
| `WHEROBOTS_API_URL` | No | Base URL for the Wherobots API. | `https://api.cloud.wherobots.com` |
| `WHEROBOTS_UPLOAD_PATH` | No | Default S3 root for local file uploads in `job-runs create`. | _(auto-resolved from your account)_ |
| `OPENAPI_CACHE_TTL` | No | How long to cache the OpenAPI spec (Go duration, e.g. `15m`). | `15m` |
| `OPENAPI_HTTP_TIMEOUT` | No | Timeout for fetching the OpenAPI spec (Go duration). | _(default)_ |

The OpenAPI spec is cached locally at `~/.cache/wherobots/spec.json`. If a fresh fetch fails, the cached version is used as a fallback.

## Output

- **Success:** raw JSON response body printed to `stdout`.
- **Failure:** error message printed to `stderr` with a non-zero exit code.
- `job-runs` commands support `--output text` for human-readable table output.
- For non-JSON API responses or endpoints requiring file upload/download, use `--dry-run` to get a `curl` command you can execute and customize.

## Dependencies

Built with:

| Dependency | Purpose |
|------------|---------|
| [cobra](https://github.com/spf13/cobra) | CLI framework and command routing |
| [libopenapi](https://github.com/pb33f/libopenapi) | OpenAPI 3.x spec parsing |
| [gjson](https://github.com/tidwall/gjson) / [sjson](https://github.com/tidwall/sjson) | JSON reading and mutation |
| [shlex](https://github.com/google/shlex) | Shell-safe argument quoting (for `--dry-run` curl output) |

## Development

```bash
make test          # run tests
make build         # compile to bin/wherobots
make fmt           # format source code
make tidy          # tidy go.mod dependencies
make run ARGS='--tree'   # run without building
```

- PR validation runs `go test` and `go build` automatically via GitHub Actions.
- Merges to `main` publish a rolling `latest-prerelease` release with binaries for Linux, macOS, and Windows (amd64 and arm64).
- To cut a stable `vX.Y.Z` release, see [RELEASE.md](./RELEASE.md).
