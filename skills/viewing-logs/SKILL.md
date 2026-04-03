---
name: viewing-logs
description: Fetches or streams logs for a Wherobots job run. Supports following in real time and tailing the last N lines. Use when a user wants to see logs, debug a failed job, tail output, or follow a running job's progress.
---

# Viewing Wherobots Job Run Logs

## Command

```bash
wherobots job-runs logs <run-id> [flags]
```

### Flags
- `-f, --follow` — stream logs in real time until completion (Ctrl+C to detach)
- `-t, --tail <N>` — last N log lines (batch mode only)
- `--interval <seconds>` — poll interval when following (default: `2`)
- `--output` — `text` (default) or `json` (not supported with `--follow`)

## Examples

```bash
wherobots job-runs logs abc-123-def                  # All logs
wherobots job-runs logs abc-123-def --follow         # Stream live
wherobots job-runs logs abc-123-def --tail 50        # Last 50 lines
wherobots job-runs logs abc-123-def -f --interval 1  # Faster polling
```

## Debugging a failed job

1. Find the job: `wherobots job-runs failed -l 5`
2. View the tail: `wherobots job-runs logs <run-id> --tail 100`
3. Look for stack traces, error messages, or OOM indicators
4. Check metrics if needed: `wherobots job-runs metrics <run-id>`

## Guidance

- If the user doesn't have a run ID, suggest `wherobots job-runs list` to find one.
- Recommend `--follow` for running jobs, `--tail` for completed/failed jobs.
- `WHEROBOTS_API_KEY` must be set. The `wherobots` CLI must be installed.
