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

`/v1/execute` maps to statuses: `OK`, `TLE`, `MLE`, `CE`, `RE`, `INTERNAL`.

`WA` remains an orchestrator responsibility (compare `stdout` with testcase expected output).

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
