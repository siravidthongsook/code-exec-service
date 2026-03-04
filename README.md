# code-exec-service

Standalone execution microservice for untrusted code sandboxing.

This module is intentionally independent from `online_grader_for_my_student` so it can be built, tested, and validated in isolation before migration.

## What it provides

- Isolated source staging per request
- Compile + run pipeline for supported languages
- nsjail sandbox support with runtime limits
- Backward-compatible `/run` endpoint for current worker integration
- New `/v1/execute` endpoint aligned with the extraction plan

## Supported languages

- `python`
- `cpp`
- `c`
- `js` / `javascript`
- `java`
- `go`

## Endpoints

- `GET /health`
- `POST /v1/execute`
- `POST /run` (legacy compatibility mode)
- gRPC `codeexec.v1.CodeExecutionService/Execute` on port `9090`

`/v1/execute` maps to statuses: `OK`, `TLE`, `MLE`, `CE`, `RE`, `INTERNAL`.

`WA` remains an orchestrator responsibility (compare `stdout` with testcase expected output).

## API usage tutorial

### 1) Health check

```bash
curl -s http://localhost:7070/health
```

Example response:

```json
{"ok":true}
```

### 2) Execute code with `/v1/execute`

```bash
curl -s -X POST http://localhost:7070/v1/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "language": "python",
    "source": "print(input())",
    "stdin": "hello",
    "time_limit_ms": 1000,
    "memory_limit_kb": 262144
  }'
```

Example response:

```json
{
  "stdout": "hello\n",
  "stderr": "",
  "exit_code": 0,
  "time_ms": 3,
  "memory_kb": 0,
  "status": "OK",
  "output_truncated": false
}
```

Request fields:

- Required: `language`, `source`
- Common: `stdin`, `time_limit_ms`, `memory_limit_kb`
- Optional overrides: `runtime_overrides`, `request_id`, `compile_cmd`, `run_cmd`, `max_output_bytes`, `allow_network`

Status behavior:

- `OK`: run finished with exit code `0`
- `TLE`: execution exceeded time limit
- `MLE`: stderr looks like out-of-memory
- `CE`: compile failed or compile timed out
- `RE`: run finished with non-zero exit code
- `INTERNAL`: request or sandbox/service error

### 3) Legacy compatibility endpoint `/run`

`/run` is kept for older worker/orchestrator integrations. It accepts camelCase request fields and returns compile/run details.

```bash
curl -s -X POST http://localhost:7070/run \
  -H 'Content-Type: application/json' \
  -d '{
    "language": "python",
    "source": "print(1)",
    "stdin": "",
    "timeLimitMs": 1000,
    "memoryLimitKb": 262144
  }'
```

Example response:

```json
{
  "run": {
    "exitCode": 0,
    "stdout": "1\n",
    "stderr": "",
    "timedOut": false,
    "durationMs": 2,
    "outputTruncated": false
  }
}
```

### 4) gRPC usage

Service: `codeexec.v1.CodeExecutionService/Execute` on `localhost:9090`

```bash
grpcurl -plaintext -d '{
  "language": "python",
  "source": "print(input())",
  "stdin": "hi",
  "time_limit_ms": 1000,
  "memory_limit_kb": 262144
}' localhost:9090 codeexec.v1.CodeExecutionService/Execute
```

Proto contract: `proto/execution.proto`.

### 5) Request limits and validation notes

- Unknown JSON fields are rejected (`400`)
- Default max HTTP body: `2MB` (`CODE_EXEC_MAX_BODY_BYTES`)
- Default max source size: `256KB` (`CODE_EXEC_MAX_SOURCE_BYTES`)
- Default max stdin size: `256KB` (`CODE_EXEC_MAX_STDIN_BYTES`)
- If `max_output_bytes` is reached, output is cut and `output_truncated=true`

### 6) Common error scenarios

Unsupported language (`INTERNAL`):

```bash
curl -s -X POST http://localhost:7070/v1/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "language": "brainfuck",
    "source": "+.",
    "time_limit_ms": 1000,
    "memory_limit_kb": 262144
  }'
```

Example response:

```json
{
  "stdout": "",
  "stderr": "",
  "exit_code": -1,
  "time_ms": 0,
  "memory_kb": 0,
  "status": "INTERNAL",
  "sandbox_error": "unsupported language",
  "output_truncated": false
}
```

Compile error (`CE`):

```bash
curl -s -X POST http://localhost:7070/v1/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "language": "go",
    "source": "package main\nfunc main(){",
    "time_limit_ms": 1000,
    "memory_limit_kb": 262144
  }'
```

Example response (shape):

```json
{
  "status": "CE",
  "exit_code": 1,
  "stderr": "...compiler error details..."
}
```

Unknown JSON field (`400`):

```bash
curl -i -X POST http://localhost:7070/v1/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "language": "python",
    "source": "print(1)",
    "time_limit_ms": 1000,
    "memory_limit_kb": 262144,
    "unexpected": true
  }'
```

Response starts with `HTTP/1.1 400 Bad Request`.

Oversized source/stdin (`INTERNAL` from executor validation):

- If `source` exceeds `CODE_EXEC_MAX_SOURCE_BYTES`, response status is `INTERNAL` with `sandbox_error: "source too large"`.
- If `stdin` exceeds `CODE_EXEC_MAX_STDIN_BYTES`, response status is `INTERNAL` with `sandbox_error: "stdin too large"`.

### 7) Quick test matrix

Use this as a fast sanity checklist while testing integrations:

| Scenario | Endpoint | Expected HTTP | Expected status |
| --- | --- | --- | --- |
| Health check | `GET /health` | `200` | n/a (`{"ok":true}`) |
| Valid Python program | `POST /v1/execute` | `200` | `OK` |
| Runtime error (Python) | `POST /v1/execute` | `200` | `RE` |
| Time limit exceeded | `POST /v1/execute` | `200` | `TLE` |
| Memory-limit-like stderr | `POST /v1/execute` | `200` | `MLE` |
| Compile error (Go/C/C++) | `POST /v1/execute` | `200` | `CE` |
| Unsupported language | `POST /v1/execute` | `200` | `INTERNAL` |
| Unknown JSON field | `POST /v1/execute` | `400` | n/a (request rejected) |
| Legacy compatibility request | `POST /run` | `200` | n/a (`compile`/`run` structure) |
| gRPC valid request | `Execute` RPC | n/a | `OK` |

Fixture mapping for `/v1/execute` regression tests:

- `tests/fixtures/python_ok.json` -> `OK`
- `tests/fixtures/python_re.json` -> `RE`
- `tests/fixtures/python_tle.json` -> `TLE`
- `tests/fixtures/python_mle.json` -> `MLE`
- `tests/fixtures/go_ce.json` -> `CE`

## Run locally

```bash
go run ./cmd/server
```

Disable nsjail in local development if unavailable:

```bash
CODE_EXEC_USE_NSJAIL=0 go run ./cmd/server
```

## Tests

```bash
go test ./...
```

Fixtures are in `tests/fixtures/` and include service verdict cases plus an orchestrator WA parity fixture.

## Docker

```bash
docker compose up --build
```

## Contract file

Proto contract is defined at `proto/execution.proto` for v1 API evolution.

## Migration sequence (safe rollout)

1. Validate this service with parity fixtures and load tests.
2. Keep current online grader execution path unchanged.
3. Add worker backend switch (`docker` vs `rpc/http`) in the grader.
4. Canary by language or traffic percentage.
5. Roll back quickly by toggling backend config.
