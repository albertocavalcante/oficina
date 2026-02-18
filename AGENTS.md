# oficina

Minimalistic pull-based job execution system with web dashboard.

## Architecture

```
               ┌──────────┐
 Browser ─────►│  Server   │◄──── Agent (polls)
   (SSE)       │ :8080     │      (Windows/Linux)
               └──────────┘
```

- **Server** (`cmd/server/`): Go HTTP server, serves API + static UI from `ui/dist/`
- **Agent** (`cmd/agent/`): Polls server for jobs, executes commands, streams logs back
- **UI** (`ui/`): SvelteKit 5 SPA, built to static files via `@sveltejs/adapter-static`

## Commands

```bash
just setup                      # Install hooks and tools
just build                      # Build Go binaries
just build-ui                   # Build Svelte UI
just build-all                  # Build everything (UI + Go)
just install                    # Install server + agent to $GOPATH/bin
just install-agent              # Install only agent to $GOPATH/bin
just build-agent linux amd64    # Cross-compile agent for target
just release                    # Build all platform binaries → dist/
just install-agent-service URL  # Install agent as OS service (OS-aware)
just uninstall-agent-service    # Uninstall agent service (OS-aware)
just container                  # Build all container images
just helm-install               # Install Helm chart
just test                       # Run tests
just lint                       # Run linter
just fmt                        # Format code
just check                      # Run all quality checks (what CI runs)
```

## Structure

```
cmd/
  server/          -- Server CLI (Cobra)
  agent/           -- Agent CLI (Cobra)
internal/
  models/          -- Core domain types (Job, Agent, LogLine)
  store/           -- In-memory storage with SSE log subscriptions
  api/             -- HTTP handlers (job CRUD, agent long-poll, SSE streaming)
  executor/        -- Command execution with line-by-line output streaming
scripts/
  install.sh       -- macOS/Linux install script (binary + service)
  install.ps1      -- Windows install script (binary + NSSM/schtasks)
chart/             -- Helm chart (BestEffort QoS, Ingress + HTTPProxy)
ui/
  src/
    lib/api.ts     -- TypeScript API client (fetch + EventSource)
    app.css        -- Dark theme CSS variables
    routes/
      +layout.svelte   -- Shell layout (nav)
      +page.svelte     -- Jobs dashboard
      jobs/[id]/       -- Job detail with live log viewer
      agents/          -- Agents page
  dist/            -- Built static files (gitignored)
```

## API

### Job Management
- `POST /api/jobs` — Create job `{name, command, shell?}`
- `GET /api/jobs` — List all jobs
- `GET /api/jobs/{id}` — Get job by ID
- `POST /api/jobs/{id}/cancel` — Cancel pending job
- `GET /api/jobs/{id}/logs` — Get job logs (JSON array)
- `GET /api/jobs/{id}/stream` — SSE stream of log lines + `done` event

### Agent Protocol
- `POST /api/agents/register` — Register agent `{name, os, arch, labels?}`
- `GET /api/agents` — List agents
- `POST /api/agent/heartbeat` — Agent heartbeat (header: `X-Agent-ID`)
- `GET /api/agent/next` — Long-poll for next job (header: `X-Agent-ID`, 30s timeout)
- `POST /api/agent/jobs/{id}/log` — Upload log batch `{lines: [{ts, stream, text}]}`
- `POST /api/agent/jobs/{id}/result` — Report completion `{exitCode, error?}`

## Conventions

- Semantic commits: `type(scope): description` (max 72 chars)
- Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
- Format with gofmt before committing
- Lint with golangci-lint -- all warnings are errors
- Tests must pass with race detector enabled
- SonarCloud for code coverage and quality gates (never Codecov)

## Do NOT

- Add dependencies to the main go.mod for dev-only tools (use tools.go.mod)
- Commit secrets or .env files
- Skip lefthook hooks (no --no-verify)
