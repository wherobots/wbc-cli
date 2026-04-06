---
name: using-wherobots
description: Routes to the right Wherobots tool for the task. Covers when to use the wherobots CLI, MCP server, Python DB-API, JDBC, or SDKs. Use when a user wants to interact with Wherobots Cloud and you need to determine the right approach.
---

# Using Wherobots Tools

Multiple tools exist for working with Wherobots Cloud. Choose based on what the task requires.

## Tool selection

### Wherobots MCP Server — interactive exploration and ad-hoc queries

Use for: catalog discovery, schema inspection, spatial SQL generation, small-scale query execution, documentation lookup.

- Runs on a Tiny runtime with ~10-minute time limits
- 1000-row result limit on returned data
- Best for exploratory and conversational workflows in Claude Code or VS Code
- Tools: `search_documentation_tool`, `list_catalogs_tool`, `list_tables_tool`, `describe_table_tool`, `generate_spatial_query_tool`, `execute_query_tool`

### Python DB-API / JDBC — programmatic query execution

Use for: running spatial SQL from application code, retrieving query results in Python or Java, building data pipelines.

**Python (`wherobots-python-dbapi`):**
```python
from wherobots.db import connect
from wherobots.db.region import Region
from wherobots.db.runtime import Runtime

with connect(
    host="api.cloud.wherobots.com",
    api_key=api_key,
    runtime=Runtime.TINY,
    region=Region.AWS_US_WEST_2) as conn:
    curr = conn.cursor()
    curr.execute("SELECT ... FROM ...")
    results = curr.fetchall()
```

**JDBC:** `jdbc:wherobots://api.cloud.wherobots.com` with `apiKey`, `runtime`, `region` properties.

These are the right choice when code needs to execute queries and process results. The MCP server or CLI should not be used for query execution in application code.

### `wherobots` CLI `job-runs` — batch execution

Use for: submitting Python or JAR scripts for large-scale processing, ETL, long-running computation, anything needing a dedicated runtime.

See the `submitting-jobs`, `monitoring-jobs`, and `viewing-logs` skills.

### `wherobots` CLI `api` — REST API access and session lifecycle

Use for: SQL session lifecycle management (create, stop, check status), file and storage operations, organization management, any API operation not covered above.

Key distinction: use `api sql ...` commands to **manage** SQL sessions (start, stop, status). Use DB-API, JDBC, or MCP to **execute queries** within those sessions.

See the `exploring-api` skill.

### Airflow operators — orchestrated workflows

Use for: scheduled pipelines, DAGs, production ETL orchestration.

- `WherobotsRunOperator` — submits job runs from Airflow
- `WherobotsSqlOperator` — executes SQL queries from Airflow

## Decision shortcuts

| Task | Tool |
|---|---|
| "What tables are available?" | MCP (`list_tables_tool`) |
| "Run this spatial query" (interactive) | MCP (`execute_query_tool`) |
| "Run this query in my Python app" | DB-API or JDBC |
| "Submit this script as a batch job" | CLI `job-runs create` |
| "Check my running jobs" | CLI `job-runs list` |
| "Start/stop a SQL session" | CLI `api sql ...` |
| "Manage files in S3" | CLI `api files ...` |
| "Schedule a nightly pipeline" | Airflow operators |
