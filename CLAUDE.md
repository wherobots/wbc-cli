# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

`wherobots` CLI — a Go command-line tool for the Wherobots Cloud API. It has two command groups:
- **`job-runs`** — curated commands for submitting Spark jobs, streaming logs, listing runs, and viewing metrics
- **Dynamic API commands** — generated at runtime from the Wherobots OpenAPI spec, so every API endpoint is available as a CLI command

## Build & Development Commands

```bash
make build          # compile to bin/wherobots
make test           # go test ./...
make fmt            # go fmt ./...
make tidy           # go mod tidy
make run ARGS='...' # run without building (e.g., make run ARGS='job-runs list')
make clean          # remove bin/
```

Run a single test:
```bash
go test -run TestName ./internal/commands/
```

Required env var for runtime: `WHEROBOTS_API_KEY`

## Architecture

### Dynamic Command Generation

The CLI builds its command tree at startup from a live OpenAPI spec. `internal/spec/loader.go` fetches and caches the spec (`~/.cache/wherobots/spec.json`, 15min TTL), `internal/spec/parser.go` extracts operations, and `internal/commands/builder.go` converts each operation into a Cobra command with flags for path params, query params, and request body fields.

### Curated `job-runs` Commands

`internal/commands/jobs.go` defines hand-written commands (`create`, `logs`, `list`, `running`, `failed`, `completed`, `metrics`) that layer workflow logic on top of the API: auto-uploading local scripts to S3 via presigned URLs, log streaming with polling, status watching, and formatted output.

### Request Execution Pipeline

`internal/executor/request.go` builds authenticated HTTP requests (API key in `x-api-key` header). `dryrun.go` outputs the equivalent curl command when `--dry-run` is used. `upload.go` handles S3 presigned-URL uploads with a 500MB limit.

### Key Packages

| Package | Role |
|---------|------|
| `internal/commands` | Cobra command builders — both dynamic (builder.go) and curated (jobs.go) |
| `internal/spec` | OpenAPI spec fetching, caching, and parsing |
| `internal/executor` | HTTP request construction, execution, dry-run, file upload |
| `internal/config` | Env-var-based configuration loading |
| `internal/hints` | Schema-aware error messages for invalid arguments |
| `internal/version` | Background update checking via `gh release view` |

### Version Injection

`main.go` has `buildVersion`, `commit`, `date` vars injected via ldflags at build time. Local builds show `dev`.

## CI/CD

- **PR validation** (`.github/workflows/pr-validate.yml`): runs `go test` and `go build` on every PR
- **Release** (`.github/workflows/release.yml`): on push to main, builds all 6 platform binaries (darwin/linux/windows x amd64/arm64), generates SHA-256 checksums, publishes as rolling `latest-prerelease` GitHub release
- **GoReleaser** (`.goreleaser.yaml`): configures cross-platform builds and archive formats
