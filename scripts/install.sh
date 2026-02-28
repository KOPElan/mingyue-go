#!/usr/bin/env bash
# install.sh — Install mingyue as a systemd service.
#
# Usage: sudo bash scripts/install.sh [BINARY_PATH]
#
# BINARY_PATH: optional path to the mingyue binary (default: ./mingyue or build output)
#
# Requirements: systemd, root or sudo privileges.
# Tested on: Ubuntu 22.04 / Debian 12 / RHEL 9 / CentOS Stream 9
set -euo pipefail

BINARY_NAME="mingyue"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="mingyue"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
CONFIG_DIR="/etc/mingyue"
DATA_DIR="/var/lib/mingyue"
LOG_DIR="/var/log/mingyue"
RUN_USER="root"

# ── Helper functions ──────────────────────────────────────────────────────────

log() { echo "[install] $*"; }
die() { echo "[install] ERROR: $*" >&2; exit 1; }

# ── Verify root privileges ────────────────────────────────────────────────────

if [ "$(id -u)" -ne 0 ]; then
  die "This script must be run as root (or with sudo)."
fi

# ── Detect systemd ───────────────────────────────────────────────────────────

if ! command -v systemctl &>/dev/null; then
  die "systemd is required but not found. Cannot install as a service."
fi

# ── Locate the binary ────────────────────────────────────────────────────────

BINARY_SRC="${1:-}"

if [ -z "$BINARY_SRC" ]; then
  # Try common build output locations in order.
  for candidate in \
    "./${BINARY_NAME}" \
    "./cmd/${BINARY_NAME}/${BINARY_NAME}" \
    "$(go env GOPATH 2>/dev/null)/bin/${BINARY_NAME}" \
    ; do
    if [ -f "$candidate" ] && [ -x "$candidate" ]; then
      BINARY_SRC="$candidate"
      break
    fi
  done
fi

if [ -z "$BINARY_SRC" ]; then
  log "Binary not found in default locations. Building from source..."
  if ! command -v go &>/dev/null; then
    die "go is required to build the binary. Install Go or provide the binary path as the first argument."
  fi
  go build -o "/tmp/${BINARY_NAME}" ./cmd/mingyue
  BINARY_SRC="/tmp/${BINARY_NAME}"
  log "Built binary: $BINARY_SRC"
fi

if [ ! -f "$BINARY_SRC" ] || [ ! -x "$BINARY_SRC" ]; then
  die "Binary not found or not executable: $BINARY_SRC"
fi

# ── Create directories ────────────────────────────────────────────────────────

log "Creating directories..."
install -d -m 755 "$CONFIG_DIR"
install -d -m 755 "$DATA_DIR"
install -d -m 750 "$LOG_DIR"

# ── Install binary ────────────────────────────────────────────────────────────

log "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
install -m 755 "$BINARY_SRC" "${INSTALL_DIR}/${BINARY_NAME}"

# ── Write systemd unit file ───────────────────────────────────────────────────

log "Writing systemd service unit: $SERVICE_FILE..."
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=mingyue Linux Operations Agent
Documentation=https://github.com/KOPElan/mingyue-go
After=network.target
Wants=network.target

[Service]
Type=simple
User=${RUN_USER}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} agent start
ExecStop=${INSTALL_DIR}/${BINARY_NAME} agent stop
Restart=on-failure
RestartSec=5s

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

# Hardening (best-effort; override in /etc/systemd/system/${SERVICE_NAME}.service.d/)
ProtectSystem=full
ReadWritePaths=${DATA_DIR} ${LOG_DIR} /run
NoNewPrivileges=false
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# ── Enable and start service ──────────────────────────────────────────────────

log "Reloading systemd daemon..."
systemctl daemon-reload

log "Enabling ${SERVICE_NAME} service (start on boot)..."
systemctl enable "$SERVICE_NAME"

log "Starting ${SERVICE_NAME} service..."
systemctl start "$SERVICE_NAME"

# ── Verify service is running ─────────────────────────────────────────────────

sleep 1
if systemctl is-active --quiet "$SERVICE_NAME"; then
  log "Service ${SERVICE_NAME} is running."
else
  die "Service ${SERVICE_NAME} failed to start. Check 'journalctl -u ${SERVICE_NAME}' for details."
fi

log "Installation complete."
log ""
log "Useful commands:"
log "  systemctl status ${SERVICE_NAME}       — check status"
log "  journalctl -u ${SERVICE_NAME} -f       — follow logs"
log "  systemctl stop ${SERVICE_NAME}         — stop service"
log "  systemctl restart ${SERVICE_NAME}      — restart service"
log "  sudo bash scripts/uninstall.sh         — remove service"
