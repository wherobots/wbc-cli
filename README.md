# wherobots CLI

Dynamic, agent-first CLI for the Wherobots API.

This binary does **not** hardcode endpoint logic. On each run, it loads an OpenAPI 3.x spec, builds commands at runtime, and executes requests with deterministic machine-friendly output.

## Runtime contract

- Spec source default: `https://api.cloud.wherobots.com/openapi.json`
- Override base API URL: `WHEROBOTS_API_URL` (base URL only; `/openapi.json` is appended if missing)
- Auth: `WHEROBOTS_API_KEY` (**required** on all invocations)
- Auth header: `x-api-key: $WHEROBOTS_API_KEY`
- CLI name/binary: `wherobots`

## Quick start

```bash
export WHEROBOTS_API_KEY='...'
# optional:
export WHEROBOTS_API_URL='https://api.cloud.wherobots.com'

# discover surface
wherobots --tree

# execute generated operation (example shape)
wherobots <resource> <verb> --id user-123 --limit 10 --metadata-json '{"k":"v"}'
```

## How commands are generated

- Noun/resource hierarchy comes from OpenAPI paths.
- Verb command names prefer `operationId` (normalized); fallback is HTTP heuristic (`GET -> list/get`, `POST -> create`, etc.).
- Scalar inputs are exposed as named flags (for example `--id`, `--limit`, `--enabled`).
- Object/array inputs are exposed as `*-json` named flags (for example `--filter-json`, `--metadata-json`).
- Nested paths are represented as nested commands.

## Flags and input rules

- Per-operation flags are generated dynamically from parameter and request-body schema.
  - Scalar types (`string`, `integer`, `number`, `boolean`) use scalar flags.
  - Object/array types use JSON string flags (`*-json`).
  - `--help` for each operation includes expected type and sample for every generated flag.
- `--json '<raw-json>'`: raw request body override.
  - Takes precedence over generated body-field flags.
  - Must be valid JSON when provided.
- `-q, --query key=value`: repeatable query pairs.
  - Format must be exactly `key=value`.
  - Useful as fallback; typed named query flags are preferred.
  - Duplicate keys: last value wins.
- `--dry-run`: prints curl equivalent instead of executing.
- `--tree`: prints full command tree.

## Output and errors (strict)

- Success: raw response body to `stdout`.
- Response must be valid JSON and non-empty, or command fails.
- Failure: error to `stderr` and non-zero exit.
- Usage/arg/flag issues include operation-aware hint text with required path/body fields.

## Constraints and type behavior

- OpenAPI support: **3.x only** (parsed with `libopenapi` v3 model).
- Path/query/request-body schema types are used for hints and required-field discovery.
- Runtime validation is intentionally minimal:
  - required named path flags
  - required query presence
  - required body field presence
  - scalar type parsing
  - JSON validity and object/array shape checks for `*-json` flags and `--json`
- No deep nested-schema validation/coercion beyond top-level typed inputs.

## File upload/download limitations

- No multipart/form-data builder.
- No binary request body/file streaming mode.
- No direct `@file`/stdin body ingestion helper.
- No binary response streaming to file.
- Non-JSON responses are rejected.

If an endpoint needs file upload/download or non-JSON content, use `--dry-run` and execute/customize the emitted curl manually.

## Spec loading and cache

- Cache location: `~/.cache/wherobots/spec.json` (+ metadata file).
- TTL env: `OPENAPI_CACHE_TTL` (Go duration like `15m`, or integer minutes).
- Spec fetch timeout env: `OPENAPI_HTTP_TIMEOUT` (Go duration).
- On fetch failure, cached spec is used when available.

## Build and release

```bash
make test
make build
```

- PR validation workflow runs tests + build.
- Release workflow publishes a rolling `latest-prerelease` on each `main` commit.
