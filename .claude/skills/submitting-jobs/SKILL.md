---
name: submitting-jobs
description: Submits Wherobots Cloud job runs from local Python files or S3 URIs. Resolves storage using the customer's S3 Storage Integration first, falling back to Wherobots managed storage. Use when a user wants to run, submit, or execute a Python script on Wherobots Cloud.
---

# Submitting Wherobots Job Runs

## Storage resolution order

Job runs execute from S3. The CLI accepts a local `.py` file or an `s3://` URI — local files are uploaded automatically. Resolve where the script will live in this order:

### 1. Customer's own S3 via Storage Integration (preferred)

If the customer has an S3 Storage Integration configured:
- Direct `s3://` URI: `s3://their-bucket/scripts/my_job.py`
- Upload override: `--upload-path s3://their-bucket/prefix`
- Environment variable: `WHEROBOTS_UPLOAD_PATH`

### 2. Wherobots managed storage (fallback)

If no Storage Integration exists or the customer is unsure, the CLI automatically resolves the managed storage bucket via the organization API. No extra configuration needed — just pass the local `.py` file.

## Command reference

```bash
wherobots job-runs create <script> -n <name> [flags]
```

### Required
- `<script>` — local `.py` file path or `s3://` URI (positional)
- `-n, --name` — job run name

### Optional
- `-r, --runtime` — compute size (default: `tiny`)
- `--run-region` — region (default: `aws-us-west-2`)
- `--timeout` — seconds (default: `3600`)
- `--args` — space-separated arguments for the script
- `-c, --spark-config` — `key=value` (repeatable)
- `--dep-pypi` — `name==version` (repeatable)
- `--dep-file` — S3 URI for `.jar`, `.whl`, `.zip`, `.json` (repeatable)
- `-w, --watch` — stream logs until completion
- `--upload-path` — override S3 upload destination
- `--no-upload` — skip auto-upload (script already in S3)
- `--output` — `text` (default) or `json`

## Examples

**Local file, managed storage:**
```bash
wherobots job-runs create ./my_script.py -n my-job -w
```

**Local file, customer S3:**
```bash
wherobots job-runs create ./my_script.py -n my-job --upload-path s3://my-bucket/jobs -w
```

**Script already in S3:**
```bash
wherobots job-runs create s3://my-bucket/scripts/my_script.py -n my-job --no-upload -w
```

**With dependencies and Spark config:**
```bash
wherobots job-runs create ./etl.py -n nightly-etl \
  -r medium \
  --timeout 7200 \
  --dep-pypi pandas==2.1.0 \
  --dep-file s3://my-bucket/libs/utils.whl \
  -c spark.sql.shuffle.partitions=200 \
  -w
```

## Guidance

- This skill is for batch job execution. If the user wants to run an interactive query, use the MCP server or Python DB-API instead — see the `using-wherobots` skill.
- Ask whether the customer has an S3 Storage Integration or should use managed storage.
- Default to `-w` (watch) so they see logs in real time.
- Only ask about runtime size if they mention performance needs or large data.
- Only ask about dependencies if they mention libraries their script needs.
- `WHEROBOTS_API_KEY` must be set. The `wherobots` CLI must be installed.
