---
name: exploring-api
description: Discovers and calls Wherobots API endpoints using the CLI's dynamically generated commands. Covers files, storage integrations, organization details, and any endpoint beyond job runs. Use when a user needs API operations not covered by the job-runs commands.
---

# Exploring the Wherobots API

The `wherobots api` command tree is generated from the Wherobots OpenAPI spec. Every API endpoint has a corresponding CLI command.

## Discovery

```bash
wherobots api --tree                        # Full command hierarchy
wherobots api <resource> <verb> --help      # Flags and body template for a command
wherobots api <resource> <verb> --dry-run   # Preview as curl (API key sanitized)
```

## Common operations

### SQL session lifecycle
```bash
wherobots api sql --tree                    # Discover available SQL session operations
```

Use `api sql` commands to manage SQL sessions (create, stop, check status). Do not use these for query execution — use Python DB-API, JDBC, or the MCP server instead. See the `using-wherobots` skill for routing guidance.

### Files and storage
```bash
wherobots api files dir get -q dir=s3://my-bucket/path/
wherobots api files integration-dir get -q integration_id=<id> -q dir=/
wherobots api files upload-url create -q key=path/to/file.py
```

### Organization
```bash
wherobots api organization get    # Includes fileStore and storageIntegrations
```

### Runs (low-level)
```bash
wherobots api runs list -q size=10 -q status=RUNNING
wherobots api runs get --run-id <id>
wherobots api runs logs get --run-id <id> -q cursor=0 -q size=100
wherobots api runs metrics get --run-id <id>
```

## Flag conventions

- Path parameters → `--param-name`
- Query parameters → `--param-name` or `-q key=value`
- Body fields → `--field-name` (scalars) or `--field-name-json` (objects/arrays)
- `--json '{...}'` overrides all body field flags

## Guidance

- For job-run workflows, prefer the dedicated `job-runs` commands (better UX).
- For query execution, use Python DB-API, JDBC, or the MCP server — not the CLI.
- The CLI `api sql` commands are for session **lifecycle** (start, stop, status), not query execution.
- Suggest `--dry-run` first when the user is exploring or unsure.
- `WHEROBOTS_API_KEY` must be set. The `wherobots` CLI must be installed.
