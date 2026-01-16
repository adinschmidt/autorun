# AGENTS.md

This file is instructions for agentic coding tools working in this repo.

Autorun is a small Go monorepo: a cross-platform service manager with a web UI.
The frontend is embedded at build time (see `embed.go`).

## Quick Start

- Go version: `go.mod` specifies `go 1.25.3`.
- Build the binary: `go build -o autorun .`
- Run (default localhost:8080): `./autorun`
- Run with custom bind/port: `./autorun -listen 127.0.0.1 -port 3000`
- Dependency hygiene: `go mod tidy`

Security note: binding to non-localhost exposes service control; there is no auth.

## Build / Lint / Test Commands

### Build

- Build (local dev): `go build -o autorun .`
- Build stripped release binary: `go build -trimpath -ldflags="-s -w" -o autorun .`
- Cross-compile (matches CI pattern):
  - `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o autorun .`

### Test

- Run all tests: `go test ./...`
- Run all tests without cache: `go test ./... -count=1`
- Run tests in one package: `go test ./internal/api`

#### Run a single test (important)

- By exact test name:
  - `go test ./internal/api -run '^TestParseScope_DefaultsToUser$'`
- By substring/regex:
  - `go test ./internal/api -run 'TestRouter_ServiceAction'`
- Run a subtest:
  - `go test ./internal/api -run '^TestExtractServiceName$/^plain name$'`
- Verbose output:
  - `go test ./internal/api -run '^TestRouter_ServiceAction_ParsesScopeSystem$' -v`

Tip: prefer anchoring regexes with `^...$` to avoid accidentally running more.

### Lint / Static Checks

This repo does not currently configure third-party linters (golangci-lint, etc.).
Use standard Go toolchain checks:

- Format check (should be clean): `gofmt -l .`
- Auto-format: `gofmt -w .`
- Vet: `go vet ./...`

Optional (if you have it installed):

- `golangci-lint run ./...`  (not required by CI)

### CI

GitHub Actions workflow: `.github/workflows/build.yml`
- CI builds release binaries for Windows/macOS/Linux with `CGO_ENABLED=0`.
- CI currently does not run `go test` or `go vet`.

## Codebase Map

- Entry point: `main.go` (flag parsing, platform detection, HTTP server)
- Frontend embedding: `embed.go` (`go:embed frontend/*`)
- Platform abstraction: `internal/platform/platform.go` (`ServiceProvider` interface)
- macOS implementation: `internal/platform/launchd.go` (shells out to `launchctl`, `log`)
- Linux implementation: `internal/platform/systemd.go` (shells out to `systemctl`, `journalctl`)
- HTTP API: `internal/api/*` (router, handlers, WebSocket log streaming)
- Shared types: `internal/models/service.go` (JSON models and scope/status constants)

API endpoints are documented in `README.md`.

## Code Style Guidelines (Go)

### Formatting

- Always run `gofmt` on touched Go files.
- Prefer the standard library over extra dependencies unless there’s a clear win.

### Imports

- Use gofmt/goimports conventions (standard -> blank line -> third-party -> blank line -> local).
  - Example pattern in `main.go`: stdlib imports then `autorun/internal/...`.
- Avoid unused imports; keep import blocks minimal.

### Naming

- Exported identifiers: `CamelCase` with clear nouns/verbs (`ServiceProvider`, `NewRouter`).
- Unexported identifiers: `camelCase`.
- Acronyms: follow Go norms (`HTTP`, `URL`, `JSON`, `FS`, `ID`).
- Prefer explicit names over clever abbreviations; reserve short names for tight scopes.

### Types and Data Modeling

- Keep API payloads as explicit structs with JSON tags (`internal/models/service.go`).
- Prefer typed string enums via custom types + consts (e.g., `type Scope string`).
- Use pointers when “missing vs empty” matters in responses; otherwise keep values.

### Error Handling

- Prefer returning errors up the call stack; add context at boundaries.
- Wrap underlying errors with `%w` when callers may need `errors.Is/As`.
  - Pattern used in platform providers: `fmt.Errorf("...: %w", err)`.
- For command execution failures, include relevant output in the error.
  - Pattern used in `SystemdProvider.runSystemctl`: return output text.

### Logging

- Use `log.Printf` for operational logs and errors.
- Use `log.Fatalf` only for unrecoverable startup failures (see `main.go`).
- Avoid noisy logs in HTTP handlers unless diagnosing errors.

### HTTP / API Conventions

- Router uses `net/http` and `http.ServeMux` (no external router).
- Handlers should:
  - Parse inputs early (URL path/query/body).
  - Validate required fields and return `400` on client errors.
  - Return JSON consistently via `jsonResponse` / `errorResponse`.
- Keep handler methods small; factor helpers for repeated parsing/validation.

### WebSocket

- WebSocket lives in `internal/api/websocket.go` using `github.com/gorilla/websocket`.
- Connection lifecycle:
  - Upgrade, create a cancelable context, stop streaming on disconnect.
- Be mindful of write deadlines to avoid hanging connections.

### Platform Implementations

- Platform providers shell out to OS tools; keep commands explicit and safe.
- Avoid running privileged commands implicitly; respect scope (`user` vs `system`).
- When creating system services, write files with clear error messages.

### Tests

- Tests live alongside packages (`*_test.go`).
- Use table-driven tests when multiple cases exist (see `TestExtractServiceName`).
- Prefer `t.Run` subtests for readability.
- Keep tests hermetic:
  - Use fake providers (see `internal/api/fake_provider_test.go`) instead of calling `launchctl/systemctl`.

## Cursor / Copilot Rules

- No `.cursorrules` found.
- No `.cursor/rules/` found.
- No `.github/copilot-instructions.md` found.

If any of the above rules files are added later, update this document to include
and prioritize them.
