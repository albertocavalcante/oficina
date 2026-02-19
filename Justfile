# oficina
# Run `just` to see all available commands

set dotenv-load := false

# Default: list available commands
default:
    @just --list --unsorted

# ─── Setup ───────────────────────────────────────────────

# Initial project setup (install hooks, tools)
setup:
    lefthook install

# ─── Build ───────────────────────────────────────────────

ldflags := "-s -w"

# Build the binary
build:
    go build ./...

# Build the UI
build-ui:
    cd ui && bun install && bun run build

# Build everything (UI + Go)
build-all: build-ui build

# Install server and agent to $GOPATH/bin
install:
    CGO_ENABLED=0 go install -trimpath -ldflags="{{ldflags}}" ./cmd/server ./cmd/agent

# Install only the agent to $GOPATH/bin
install-agent:
    CGO_ENABLED=0 go install -trimpath -ldflags="{{ldflags}}" ./cmd/agent

# Install only the server to $GOPATH/bin
install-server:
    CGO_ENABLED=0 go install -trimpath -ldflags="{{ldflags}}" ./cmd/server

# Cross-compile agent for a specific OS/arch
build-agent os arch:
    @mkdir -p dist
    CGO_ENABLED=0 GOOS={{os}} GOARCH={{arch}} go build -trimpath -ldflags="{{ldflags}}" -o dist/oficina-agent-{{os}}-{{arch}}{{ if os == "windows" { ".exe" } else { "" } }} ./cmd/agent

# Cross-compile server for a specific OS/arch
build-server os arch:
    @mkdir -p dist
    CGO_ENABLED=0 GOOS={{os}} GOARCH={{arch}} go build -trimpath -ldflags="{{ldflags}}" -o dist/oficina-server-{{os}}-{{arch}}{{ if os == "windows" { ".exe" } else { "" } }} ./cmd/server

# Build release binaries for all platforms
release: build-ui
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    platforms=("darwin/amd64" "darwin/arm64" "linux/amd64" "linux/arm64" "windows/amd64")
    for p in "${platforms[@]}"; do
        os="${p%/*}"
        arch="${p#*/}"
        ext=""; [[ "$os" == "windows" ]] && ext=".exe"
        echo "Building agent ${os}/${arch}..."
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="{{ldflags}}" -o "dist/oficina-agent-${os}-${arch}${ext}" ./cmd/agent
        echo "Building server ${os}/${arch}..."
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="{{ldflags}}" -o "dist/oficina-server-${os}-${arch}${ext}" ./cmd/server
    done
    echo "Done. Binaries in dist/"
    ls -lh dist/

# ─── Install (OS-specific) ─────────────────────────────

# Install agent as a launchd service (macOS)
[macos]
install-agent-service server name="oficina-agent" labels="":
    scripts/install.sh --server {{server}} --name {{name}} {{ if labels != "" { "--labels " + labels } else { "" } }} --service

# Install server as a launchd service (macOS)
[macos]
install-server-service:
    scripts/install.sh --component server --service

# Install agent as a systemd service (Linux)
[linux]
install-agent-service server name="oficina-agent" labels="":
    scripts/install.sh --server {{server}} --name {{name}} {{ if labels != "" { "--labels " + labels } else { "" } }} --service

# Install server as a systemd service (Linux)
[linux]
install-server-service:
    scripts/install.sh --component server --service

# Install agent as a Windows service via NSSM
[windows]
install-agent-service server name="oficina-agent" labels="":
    powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Server {{server}} -Name {{name}} {{ if labels != "" { "-Labels " + labels } else { "" } }} -Service nssm

# Install server as a Windows service via NSSM
[windows]
install-server-service:
    powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Component server -Service nssm

# Uninstall agent service (macOS)
[macos]
uninstall-agent-service:
    -launchctl unload ~/Library/LaunchAgents/com.oficina.agent.plist
    rm -f ~/Library/LaunchAgents/com.oficina.agent.plist

# Uninstall server service (macOS)
[macos]
uninstall-server-service:
    -launchctl unload ~/Library/LaunchAgents/com.oficina.server.plist
    rm -f ~/Library/LaunchAgents/com.oficina.server.plist

# Uninstall agent service (Linux)
[linux]
uninstall-agent-service:
    -sudo systemctl disable --now oficina-agent
    sudo rm -f /etc/systemd/system/oficina-agent.service
    sudo systemctl daemon-reload

# Uninstall server service (Linux)
[linux]
uninstall-server-service:
    -sudo systemctl disable --now oficina-server
    sudo rm -f /etc/systemd/system/oficina-server.service
    sudo systemctl daemon-reload

# Uninstall agent service (Windows)
[windows]
uninstall-agent-service:
    nssm stop oficina-agent
    nssm remove oficina-agent confirm

# Uninstall server service (Windows)
[windows]
uninstall-server-service:
    nssm stop oficina-server
    nssm remove oficina-server confirm

# ─── Test ────────────────────────────────────────────────

# Run tests
test:
    go test ./...

# Run tests with race detector
test-race:
    go test -race ./...

# Run tests with coverage
coverage:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Run tests with gotestsum
test-sum:
    go tool -modfile=tools.go.mod gotestsum --format pkgname-and-test-fails

# Run tests with gotestsum + race + coverage (CI-style)
test-ci:
    go tool -modfile=tools.go.mod gotestsum --format pkgname-and-test-fails --jsonfile test-output.json -- -race -coverprofile=coverage.out ./...

# ─── Lint & Format ───────────────────────────────────────

# Run linter
lint:
    go tool -modfile=tools/lint/go.mod golangci-lint run

# Format code
fmt:
    gofmt -w .
    @test -f dprint.json && command -v dprint > /dev/null && dprint fmt || true

# Check formatting (no changes)
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
    @test -f dprint.json && command -v dprint > /dev/null && dprint check || true

# Vet code
vet:
    go vet ./...

# Tidy Go modules
tidy:
    go mod tidy
    go mod tidy -modfile=tools.go.mod
    cd tools/lint && go mod tidy

# Clean local artifacts
clean:
    rm -f coverage.out coverage.html test-output.json
    rm -rf dist/

# ─── Container ──────────────────────────────────────────

# Container runtime: override with `just engine=docker container`
engine := if `command -v podman 2>/dev/null || true` != "" { "podman" } else { "docker" }
image := "oficina"

# Build server container image
container-server:
    {{engine}} build -f Containerfile -t {{image}}-server .

# Build agent container image
container-agent:
    {{engine}} build -f Containerfile.agent -t {{image}}-agent .

# Build all container images
container: container-server container-agent

# ─── Compose ──────────────────────────────────────────

# Start server + agents with Docker/Podman Compose
compose-up *ARGS:
    {{engine}} compose -f examples/compose/compose.yaml up --build {{ARGS}}

# Stop and remove Compose services
compose-down *ARGS:
    {{engine}} compose -f examples/compose/compose.yaml down {{ARGS}}

# ─── Helm ───────────────────────────────────────────────

# Install chart into current kubectl context
helm-install *ARGS:
    helm install oficina chart/ {{ARGS}}

# Upgrade existing release
helm-upgrade *ARGS:
    helm upgrade oficina chart/ {{ARGS}}

# Uninstall release
helm-uninstall:
    helm uninstall oficina

# Render templates locally (dry-run)
helm-template *ARGS:
    helm template oficina chart/ {{ARGS}}

# ─── Check ───────────────────────────────────────────────

# Run all quality checks (what CI runs)
check: fmt-check vet lint test

# ─── Editor ──────────────────────────────────────────────

# Open in VS Code
code:
    code .

# Open in Cursor
cursor:
    cursor .

# Open in Zed
zed:
    zed .

# ─── GitHub ──────────────────────────────────────────────

# Open issues in browser
issues:
    gh issue list --web

# Open pull requests in browser
prs:
    gh pr list --web

# Open actions in browser
actions:
    gh run list --web
