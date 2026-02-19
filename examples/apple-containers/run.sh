#!/usr/bin/env bash
# oficina — Apple Containers (macOS 26+, Apple Silicon)
#
# Builds images from source, creates a shared network, starts a server
# and one agent. Press Ctrl+C to stop and clean up.
#
# Usage:
#   ./run.sh

set -euo pipefail

NETWORK="oficina"
SERVER="server"
AGENT="agent-01"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

cleanup() {
    echo ""
    echo "Stopping containers..."
    container stop "$AGENT" 2>/dev/null || true
    container stop "$SERVER" 2>/dev/null || true
    echo "Removing network..."
    container network remove "$NETWORK" 2>/dev/null || true
    echo "Done."
}
trap cleanup EXIT

echo "==> Creating network: $NETWORK"
container network create "$NETWORK"

echo "==> Building server image..."
container build --file "$ROOT/Containerfile" --tag oficina-server "$ROOT"

echo "==> Building agent image..."
container build --file "$ROOT/Containerfile.agent" --tag oficina-agent "$ROOT"

echo "==> Starting server..."
container run --name "$SERVER" \
    --network "$NETWORK" \
    --publish 8080:8080 \
    --detach \
    oficina-server

echo "==> Starting agent..."
container run --name "$AGENT" \
    --network "$NETWORK" \
    --detach \
    oficina-agent \
    --server "http://${SERVER}.test:8080" \
    --name "$AGENT" \
    --labels "apple,general"

echo ""
echo "oficina is running:"
echo "  Dashboard: http://localhost:8080"
echo "  Server:    $SERVER"
echo "  Agent:     $AGENT"
echo ""
echo "Press Ctrl+C to stop."

# Wait until interrupted.
while true; do sleep 60; done
