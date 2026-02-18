#!/usr/bin/env bash
#
# Install oficina agent (and optionally server) on macOS or Linux.
#
# Usage:
#   ./install.sh --server http://host:8080 --name my-agent
#   ./install.sh --server http://host:8080 --name my-agent --service
#   ./install.sh --component server --service
#
# The script will:
#   1. Look for a pre-built binary next to this script (dist/ or same dir)
#   2. Otherwise build from source if Go is available
#   3. Copy the binary to --install-dir (default: /usr/local/bin)
#   4. Optionally install as a system service (--service)
#
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────
COMPONENT="agent"
SERVER_URL=""
AGENT_NAME=""
AGENT_LABELS=""
# /usr/local/bin on macOS, /usr/bin on Linux (RHEL-friendly: always in systemd PATH)
DEFAULT_INSTALL_DIR="/usr/local/bin"
if [[ "$(uname -s)" == "Linux" ]]; then
    DEFAULT_INSTALL_DIR="/usr/bin"
fi
INSTALL_DIR="$DEFAULT_INSTALL_DIR"
INSTALL_SERVICE=false
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." 2>/dev/null && pwd || echo "")"

# ── Parse args ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server)       SERVER_URL="$2"; shift 2 ;;
        --name)         AGENT_NAME="$2"; shift 2 ;;
        --labels)       AGENT_LABELS="$2"; shift 2 ;;
        --install-dir)  INSTALL_DIR="$2"; shift 2 ;;
        --component)    COMPONENT="$2"; shift 2 ;;
        --service)      INSTALL_SERVICE=true; shift ;;
        -h|--help)
            sed -n '3,13p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ "$COMPONENT" == "agent" && -z "$SERVER_URL" ]]; then
    echo "Error: --server is required for agent install"
    echo "Run with --help for usage"
    exit 1
fi

if [[ "$COMPONENT" == "agent" && -z "$AGENT_NAME" ]]; then
    AGENT_NAME="$(hostname)"
    echo "No --name given, using hostname: ${AGENT_NAME}"
fi

# ── Detect OS/arch ────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "Unsupported architecture: ${ARCH}"; exit 1 ;;
esac

BINARY_NAME="oficina-${COMPONENT}-${OS}-${ARCH}"
TARGET_NAME="oficina-${COMPONENT}"

echo "Platform: ${OS}/${ARCH}"
echo "Component: ${COMPONENT}"

# ── Find or build binary ─────────────────────────────────
BINARY=""

# Check for pre-built binary next to script or in dist/
for candidate in \
    "${SCRIPT_DIR}/${BINARY_NAME}" \
    "${SCRIPT_DIR}/../dist/${BINARY_NAME}" \
    "${REPO_DIR}/dist/${BINARY_NAME}"; do
    if [[ -f "$candidate" ]]; then
        BINARY="$candidate"
        echo "Found pre-built binary: ${BINARY}"
        break
    fi
done

# Build from source if no binary found
if [[ -z "$BINARY" ]]; then
    if ! command -v go &>/dev/null; then
        echo "Error: no pre-built binary found and Go is not installed"
        echo "Either build first with 'just release' or install Go"
        exit 1
    fi
    if [[ -z "$REPO_DIR" || ! -f "${REPO_DIR}/go.mod" ]]; then
        echo "Error: not in the oficina repo and no pre-built binary found"
        exit 1
    fi
    echo "Building from source..."
    cd "$REPO_DIR"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "dist/${BINARY_NAME}" "./cmd/${COMPONENT}"
    BINARY="${REPO_DIR}/dist/${BINARY_NAME}"
    echo "Built: ${BINARY}"
fi

# ── Install binary ────────────────────────────────────────
echo "Installing to ${INSTALL_DIR}/${TARGET_NAME}"
if [[ -w "$INSTALL_DIR" ]]; then
    cp "$BINARY" "${INSTALL_DIR}/${TARGET_NAME}"
    chmod +x "${INSTALL_DIR}/${TARGET_NAME}"
else
    sudo cp "$BINARY" "${INSTALL_DIR}/${TARGET_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${TARGET_NAME}"
fi

echo "Installed: ${INSTALL_DIR}/${TARGET_NAME}"

# ── Service setup ─────────────────────────────────────────
if [[ "$INSTALL_SERVICE" != true ]]; then
    echo "Done. Run with --service to install as a system service."
    exit 0
fi

AGENT_ARGS="--server ${SERVER_URL} --name ${AGENT_NAME}"
if [[ -n "$AGENT_LABELS" ]]; then
    AGENT_ARGS="${AGENT_ARGS} --labels ${AGENT_LABELS}"
fi

if [[ "$OS" == "darwin" ]]; then
    # ── macOS: launchd ────────────────────────────────────
    PLIST_PATH="${HOME}/Library/LaunchAgents/com.oficina.${COMPONENT}.plist"

    if [[ "$COMPONENT" == "agent" ]]; then
        PROGRAM_ARGS=$(cat <<XMLEOF
        <string>${INSTALL_DIR}/${TARGET_NAME}</string>
        <string>--server</string>
        <string>${SERVER_URL}</string>
        <string>--name</string>
        <string>${AGENT_NAME}</string>
XMLEOF
)
        if [[ -n "$AGENT_LABELS" ]]; then
            PROGRAM_ARGS="${PROGRAM_ARGS}
        <string>--labels</string>
        <string>${AGENT_LABELS}</string>"
        fi
    else
        PROGRAM_ARGS="        <string>${INSTALL_DIR}/${TARGET_NAME}</string>"
    fi

    cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.oficina.${COMPONENT}</string>
    <key>ProgramArguments</key>
    <array>
${PROGRAM_ARGS}
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/oficina-${COMPONENT}.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/oficina-${COMPONENT}.log</string>
</dict>
</plist>
EOF

    launchctl load "$PLIST_PATH" 2>/dev/null || true
    echo "Installed launchd service: ${PLIST_PATH}"
    echo "Logs: /tmp/oficina-${COMPONENT}.log"
    echo ""
    echo "Commands:"
    echo "  launchctl load   ${PLIST_PATH}"
    echo "  launchctl unload ${PLIST_PATH}"

elif [[ "$OS" == "linux" ]]; then
    # ── Linux: systemd ────────────────────────────────────
    UNIT_PATH="/etc/systemd/system/oficina-${COMPONENT}.service"

    if [[ "$COMPONENT" == "agent" ]]; then
        EXEC_START="${INSTALL_DIR}/${TARGET_NAME} ${AGENT_ARGS}"
    else
        EXEC_START="${INSTALL_DIR}/${TARGET_NAME}"
    fi

    UNIT_CONTENT="[Unit]
Description=oficina ${COMPONENT}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${EXEC_START}
Restart=always
RestartSec=5
# Hardened defaults (RHEL/SELinux friendly)
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/tmp

[Install]
WantedBy=multi-user.target"

    if [[ -w "$(dirname "$UNIT_PATH")" ]]; then
        echo "$UNIT_CONTENT" > "$UNIT_PATH"
        systemctl daemon-reload
        systemctl enable --now "oficina-${COMPONENT}"
    else
        echo "$UNIT_CONTENT" | sudo tee "$UNIT_PATH" >/dev/null
        sudo systemctl daemon-reload
        sudo systemctl enable --now "oficina-${COMPONENT}"
    fi

    # Restore SELinux context if applicable.
    if command -v restorecon &>/dev/null; then
        sudo restorecon -v "${INSTALL_DIR}/${TARGET_NAME}" 2>/dev/null || true
    fi

    echo "Installed systemd service: ${UNIT_PATH}"
    echo ""
    echo "Commands:"
    echo "  systemctl status  oficina-${COMPONENT}"
    echo "  journalctl -fu    oficina-${COMPONENT}"
    echo "  systemctl restart oficina-${COMPONENT}"
    echo "  systemctl stop    oficina-${COMPONENT}"
fi
