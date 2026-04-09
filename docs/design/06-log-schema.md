# Log Schema v1

**File:** `~/.owl/logs/<run_id>.jsonl`
**Sidecar:** `~/.owl/logs/<run_id>.meta.json`

## Run ID Format

```
<sanitized-agent-name>-<unix-ms>-<4-byte-hex>
```

Example: `code-reviewer-1744201200000-a3b4c5d6`

- Agent name is lowercased, spaces replaced with hyphens, non-alphanumeric stripped, truncated to 20 chars.
- Unix millisecond timestamp provides rough ordering without a ULID dependency.
- 4-byte random hex suffix prevents collisions when agents are hatched in rapid succession.

## Log Entry (.jsonl)

Each line in the `.jsonl` file is a single JSON object:

```json
{
  "schema_version": "1",
  "ts": "2026-04-09T10:30:00.123Z",
  "run_id": "code-reviewer-1744201200000-a3b4c5d6",
  "agent_name": "code-reviewer",
  "agent_id": "3",
  "level": "info",
  "message": "Starting work on task...",
  "tool_name": "",
  "tool_result": "",
  "model_id": "",
  "tokens_in": 0,
  "tokens_out": 0
}
```

### Fields

| Field            | Type    | Description                                          |
|------------------|---------|------------------------------------------------------|
| `schema_version` | string  | Always `"1"` for this schema                        |
| `ts`             | string  | RFC3339 UTC timestamp                                |
| `run_id`         | string  | Stable run identifier (matches filename prefix)     |
| `agent_name`     | string  | Agent's display name at time of hatch               |
| `agent_id`       | string  | Agent's numeric ID within the daemon session        |
| `level`          | string  | Log level (see below)                               |
| `message`        | string  | Human-readable log line                             |
| `tool_name`      | string  | Populated for `level=tool` entries                  |
| `tool_result`    | string  | Tool output (redacted for sensitive patterns)       |
| `model_id`       | string  | Populated for `level=info` token usage entries      |
| `tokens_in`      | integer | Prompt tokens (populated for usage entries)         |
| `tokens_out`     | integer | Completion tokens (populated for usage entries)     |

### Levels

| Level      | Written by                                  |
|------------|---------------------------------------------|
| `info`     | General engine events, token usage entries  |
| `warn`     | Non-fatal issues (Viche registration, etc.) |
| `error`    | Fatal errors stopping the agent             |
| `debug`    | Verbose tool input/output detail            |
| `tool`     | Tool execution entries (with tool_name)     |
| `thinking` | LLM reasoning / scaffolding phases          |

## Run Meta Sidecar (.meta.json)

```json
{
  "run_id": "code-reviewer-1744201200000-a3b4c5d6",
  "agent_name": "code-reviewer",
  "agent_id": "3",
  "start_time": "2026-04-09T10:30:00Z",
  "model_id": "anthropic/claude-sonnet-4-6",
  "status": "running"
}
```

The sidecar is written at hatch time. `status` is `"running"` initially; future phases will update it to `"stopped"` or `"error"`.

## Redaction

Before returning entries via `logs.Reader`, the following patterns trigger redaction of the value that follows them:

- `sk-` (Anthropic/OpenAI API keys)
- `key-`
- `token=`
- `password=`
- `Bearer `

The value portion is replaced with `[REDACTED]`.

## CLI Commands

```
owl logs list                              # list all runs (table)
owl logs list --agent <name> --json        # filter + JSON output
owl logs show <run-id>                     # pretty-print entries
owl logs show <run-id> --level error       # filter by level
owl logs show <run-id> --json              # raw JSON lines
owl logs tail                              # most recent run
owl logs tail --agent <name> -f            # follow a specific agent
owl logs query --agent <name> --since 1h   # structured filter query
owl logs query --level error --limit 50    # recent errors
```
