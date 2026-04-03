---
name: monitoring-jobs
description: Lists, filters, and checks the status of Wherobots job runs. Includes shortcuts for running, failed, and completed jobs, plus instant metrics. Use when a user wants to see their jobs, check what's running, find failures, or get resource metrics.
---

# Monitoring Wherobots Job Runs

## Commands

### List jobs
```bash
wherobots job-runs list [flags]
```

**Filters:**
- `-s, --status` — `RUNNING`, `COMPLETED`, `FAILED`, `CANCELLED`, `PENDING` (repeatable)
- `--name` — filter by name pattern
- `--after` — ISO timestamp (e.g., `2025-01-15T00:00:00Z`)
- `-l, --limit` — max results (default: `20`)
- `--region` — filter by region
- `--output` — `text` (default, tabular) or `json`

### Status shortcuts
```bash
wherobots job-runs running       # RUNNING jobs only
wherobots job-runs failed        # FAILED jobs only
wherobots job-runs completed     # COMPLETED jobs only
```

Each accepts `--name`, `--after`, `-l`, `--region`, `--output`.

### Instant metrics
```bash
wherobots job-runs metrics <run-id>
```

Displays CPU, memory, and other metrics for an active run. Supports `--output json`.

## Examples

```bash
wherobots job-runs list                                          # Recent jobs
wherobots job-runs failed -l 5                                   # Last 5 failures
wherobots job-runs list --name nightly --after 2025-01-01T00:00:00Z
wherobots job-runs running --region aws-us-west-2
wherobots job-runs metrics abc-123-def
wherobots job-runs list --output json                            # For scripting
```

## Guidance

- Text output is a table: ID, NAME, STATUS, CREATED, RUNTIME, REGION.
- To dig into a specific job, suggest viewing logs or metrics by run ID.
- `WHEROBOTS_API_KEY` must be set. The `wherobots` CLI must be installed.
