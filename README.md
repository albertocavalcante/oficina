# oficina

[![CI](https://github.com/albertocavalcante/oficina/actions/workflows/ci.yml/badge.svg)](https://github.com/albertocavalcante/oficina/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=albertocavalcante_oficina&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=albertocavalcante_oficina)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=albertocavalcante_oficina&metric=coverage)](https://sonarcloud.io/summary/new_code?id=albertocavalcante_oficina)
[![Go Reference](https://pkg.go.dev/badge/github.com/albertocavalcante/oficina.svg)](https://pkg.go.dev/github.com/albertocavalcante/oficina)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertocavalcante/oficina)](https://goreportcard.com/report/github.com/albertocavalcante/oficina)

Minimalistic pull-based job execution system. Deploy the server anywhere (Kubernetes, VM, etc.), run the agent on machines that can only make outbound HTTP — no inbound ports, no SSH, no VPN required.

## Architecture

```
 ┌─────────┐        ┌──────────┐        ┌─────────┐
 │ Browser │──SSE──►│  Server  │◄─poll──│  Agent  │
 │  (UI)   │        │  :8080   │        │(Windows)│
 └─────────┘        └──────────┘        └─────────┘
```

- **Server**: Go HTTP server with REST API, SSE log streaming, and embedded SvelteKit dashboard
- **Agent**: Lightweight polling agent — registers itself, long-polls for jobs, executes commands, streams logs back
- **UI**: Dark-themed SvelteKit 5 SPA with live log viewer

## Quick Start

### Prerequisites

- Go 1.24+
- [Bun](https://bun.sh/) (for UI build)
- [just](https://github.com/casey/just) (task runner)

### Build

```bash
# Build everything
just build-all

# Or separately
just build-ui   # Svelte UI → ui/dist/
just build       # Go binaries
```

### Run the Server

```bash
go run ./cmd/server --port 8080
```

The dashboard is at `http://localhost:8080`. The server serves the static UI files from `ui/dist/`.

### Run the Agent

On the machine where you want to execute jobs:

```bash
go run ./cmd/agent --server http://<server-host>:8080 --name my-windows-box
```

The agent registers itself, then long-polls for jobs. When a job is assigned, it executes the command and streams stdout/stderr back to the server.

### Cross-compile Agent for Windows

```bash
GOOS=windows GOARCH=amd64 go build -o oficina-agent.exe ./cmd/agent
```

Copy `oficina-agent.exe` to the Windows machine and run:

```powershell
.\oficina-agent.exe --server http://<server-host>:8080 --name win-desktop
```

### Submit a Job

Via the UI dashboard, or directly via API:

```bash
curl -X POST http://localhost:8080/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{"name": "hello", "command": "echo hello world"}'
```

### View Logs

Open a job in the dashboard for live streaming logs, or fetch via API:

```bash
# Static logs
curl http://localhost:8080/api/jobs/<job-id>/logs

# SSE stream (real-time)
curl -N http://localhost:8080/api/jobs/<job-id>/stream
```

## API Reference

### Jobs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/jobs` | Create job `{name, command, shell?}` |
| `GET` | `/api/jobs` | List all jobs |
| `GET` | `/api/jobs/{id}` | Get job details |
| `POST` | `/api/jobs/{id}/cancel` | Cancel a pending job |
| `GET` | `/api/jobs/{id}/logs` | Get log lines (JSON array) |
| `GET` | `/api/jobs/{id}/stream` | SSE stream of log lines |

### Agent Protocol

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/agents/register` | Register `{name, os, arch, labels?}` |
| `GET` | `/api/agents` | List registered agents |
| `POST` | `/api/agent/heartbeat` | Heartbeat (`X-Agent-ID` header) |
| `GET` | `/api/agent/next` | Long-poll for next job (30s timeout) |
| `POST` | `/api/agent/jobs/{id}/log` | Upload log batch |
| `POST` | `/api/agent/jobs/{id}/result` | Report job result `{exitCode, error?}` |

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |

## Development

```bash
just setup      # Install git hooks
just check      # Run all quality checks (fmt, vet, lint, test)
just test       # Run tests
just lint       # Run golangci-lint
```

## License

MIT
