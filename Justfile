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

# Build the binary
build:
    go build ./...

# Build the UI
build-ui:
    cd ui && bun install && bun run build

# Build everything (UI + Go)
build-all: build-ui build

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
