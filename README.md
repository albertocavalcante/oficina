# oficina

[![CI](https://github.com/albertocavalcante/oficina/actions/workflows/ci.yml/badge.svg)](https://github.com/albertocavalcante/oficina/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=albertocavalcante_oficina&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=albertocavalcante_oficina)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=albertocavalcante_oficina&metric=coverage)](https://sonarcloud.io/summary/new_code?id=albertocavalcante_oficina)
[![Go Reference](https://pkg.go.dev/badge/github.com/albertocavalcante/oficina.svg)](https://pkg.go.dev/github.com/albertocavalcante/oficina)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertocavalcante/oficina)](https://goreportcard.com/report/github.com/albertocavalcante/oficina)

Minimalistic pull-based job execution system. A central server holds jobs and serves a web dashboard; lightweight agents poll for work over plain HTTP and stream results back. No inbound ports needed on agent machines.

## Architecture

oficina uses a **pull-based** architecture — agents initiate all connections. The server never pushes to agents, making it easy to deploy across network boundaries.

```
 ┌─────────┐        ┌──────────┐        ┌─────────┐
 │ Browser │──SSE──►│  Server  │◄─poll──│  Agent  │
 │  (UI)   │        │  :8080   │        │         │
 └─────────┘        └──────────┘        └─────────┘
```

- **Server**: Go HTTP server with REST API, SSE log streaming, and embedded SvelteKit dashboard
- **Agent**: Lightweight polling agent — registers itself, long-polls for jobs, executes commands, streams logs back. Requires only outbound HTTP to the server.
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

### Install the Agent

The agent is a single static binary with zero dependencies.

#### Quick install (with just)

```bash
# Install to $GOPATH/bin
just install-agent

# Cross-compile for a specific target
just build-agent linux amd64
just build-agent windows amd64
just build-agent darwin arm64

# Build all platforms at once → dist/
just release
```

#### Install as a system service

The Justfile has OS-aware recipes — the right service manager is used automatically (launchd on macOS, systemd on Linux, NSSM on Windows):

```bash
# macOS → launchd, Linux → systemd, Windows → NSSM
just install-agent-service http://server-host:8080
just install-agent-service http://server-host:8080 my-agent
just install-agent-service http://server-host:8080 my-agent "label1,label2"

# Uninstall
just uninstall-agent-service
```

#### Install scripts

For machines without `just`, use the install scripts directly. They auto-detect the platform, build from source (if Go is available) or use a pre-built binary from `dist/`, and optionally install as a service.

**macOS / Linux:**

```bash
# Run the agent (binary-only install)
scripts/install.sh --server http://server-host:8080 --name my-agent

# Install + register as system service
scripts/install.sh --server http://server-host:8080 --name my-agent --service
```

**Windows (PowerShell):**

```powershell
# Run the agent (binary-only install)
scripts\install.ps1 -Server http://server-host:8080 -Name my-agent

# Install + register as NSSM service
scripts\install.ps1 -Server http://server-host:8080 -Name my-agent -Service nssm

# Or use Task Scheduler instead
scripts\install.ps1 -Server http://server-host:8080 -Name my-agent -Service schtasks
```

#### Manual run (no install)

```bash
# macOS / Linux
./oficina-agent --server http://server-host:8080 --name my-agent

# Windows
.\oficina-agent.exe --server http://server-host:8080 --name my-agent
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

## Container Images

Both components ship as multi-stage container builds. The server runs from `scratch` (just the static binary + UI assets), while the agent uses `alpine` since it needs a shell to execute commands.

```bash
# Build both (auto-detects podman or docker)
just container

# Or individually
just container-server   # → oficina-server
just container-agent    # → oficina-agent

# Force a specific runtime
just engine=docker container
```

### Run

```bash
# Server
docker run -p 8080:8080 oficina-server

# Agent
docker run oficina-agent --server http://<server-host>:8080 --name my-agent
```

Works the same with `podman`.

The server image is ~10 MB (scratch + static Go binary + UI assets). The agent image is ~15 MB (alpine + static Go binary).

## Deployment Examples

### Compose

The quickest way to run the full stack locally. Builds from source and starts a server with two agents. Works with both Docker Compose and Podman Compose:

```bash
just compose-up
# → http://localhost:8080 — dashboard with 2 agents

just compose-down
```

Or without `just`:

```bash
cd examples/compose
docker compose up --build   # or: podman compose up --build
```

See [`examples/compose/`](examples/compose/) for details.

### Apple Containers (macOS 26+)

For macOS 26+ on Apple Silicon, a shell script orchestrates the native `container` runtime:

```bash
cd examples/apple-containers
./run.sh
# → http://localhost:8080 — dashboard with 1 agent
# Ctrl+C to stop
```

See [`examples/apple-containers/`](examples/apple-containers/) for details.

## Helm

A minimal Helm chart lives in `chart/`. Deploys as **BestEffort** QoS by default (no resource requests/limits).

```bash
helm install oficina ./chart
```

The agent auto-discovers the server via the in-cluster service name. Override values as needed:

```bash
helm install oficina ./chart \
  --set server.image.repository=registry.example.com/oficina-server \
  --set agent.image.repository=registry.example.com/oficina-agent \
  --set agent.name=k8s-agent
```

### Ingress

Standard Kubernetes Ingress (default):

```bash
helm install oficina ./chart \
  --set server.ingress.enabled=true \
  --set server.ingress.kind=Ingress \
  --set server.ingress.host=oficina.example.com \
  --set server.ingress.className=nginx
```

[Project Contour](https://projectcontour.io/) HTTPProxy:

```bash
helm install oficina ./chart \
  --set server.ingress.enabled=true \
  --set server.ingress.kind=HTTPProxy \
  --set server.ingress.host=oficina.example.com \
  --set server.ingress.httpProxy.tls.secretName=oficina-tls
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
