# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
# Build the binary (embeds frontend automatically)
go build -o autorun .

# Run the server (defaults to localhost:8080)
./autorun

# Run with custom port/address
./autorun -port 3000 -listen 0.0.0.0

# Run without building first
go run .

# Fix module dependencies
go mod tidy
```

## Architecture

Autorun is a cross-platform service manager with a web UI. It provides a REST API and WebSocket interface to manage system services (launchd on macOS, systemd on Linux).

### Key Components

- **main.go**: Entry point. Detects platform, loads embedded frontend, starts HTTP server
- **embed.go**: Embeds the `frontend/` directory into the binary using `go:embed`
- **internal/platform/platform.go**: Defines `ServiceProvider` interface and auto-detects platform
- **internal/platform/launchd.go**: macOS implementation using `launchctl`
- **internal/platform/systemd.go**: Linux implementation using `systemctl` and `journalctl`
- **internal/api/**: HTTP handlers, routing, and WebSocket log streaming
- **internal/models/service.go**: Service struct and scope constants (user/system)

### Service Scopes

Services are categorized by scope:
- `user`: User-level services (LaunchAgents on macOS, `--user` units on systemd)
- `system`: System-level services (LaunchDaemons on macOS, system units on systemd)

### API Endpoints

- `GET /api/platform` - Returns current platform (launchd/systemd)
- `GET /api/services?scope=user|system|all` - List services
- `GET /api/services/{name}?scope=...` - Get service details
- `POST /api/services/{name}/start|stop|restart|enable|disable?scope=...` - Control service
- `WS /api/services/{name}/logs?scope=...` - Stream logs via WebSocket

### Frontend

Static HTML/CSS/JS in `frontend/` is embedded at compile time. No build step required for frontend changes - just rebuild the Go binary.
