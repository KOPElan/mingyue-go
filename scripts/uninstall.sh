#!/usr/bin/env bash
# uninstall.sh — Remove the mingyue systemd service and optionally its data.
#
# Usage: sudo bash scripts/uninstall.sh [--purge]
#
# --purge: also delete config (/etc/mingyue), data (/var/lib/mingyue),
#          and logs (/var/log/mingyue) directories.
#
# Requirements: systemd, root or sudo privileges.
set -euo pipefail

BINARY_NAME="mingyue"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="mingyue"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
CONFIG_DIR="/etc/mingyue"
DATA_DIR="/var/lib/mingyue"
LOG_DIR="/var/log/mingyue"

PURGE=false
for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=true ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# ── Helper functions ──────────────────────────────────────────────────────────

log() { echo "[uninstall] $*"; }
die() { echo "[uninstall] ERROR: $*" >&2; exit 1; }

# ── Verify root privileges ────────────────────────────────────────────────────

if [ "$(id -u)" -ne 0 ]; then
  die "This script must be run as root (or with sudo)."
fi

# ── Detect systemd ───────────────────────────────────────────────────────────

if ! command -v systemctl &>/dev/null; then
  die "systemd is required but not found. Cannot uninstall a service in a non-systemd environment."
fi

# ── Stop and disable service ──────────────────────────────────────────────────

if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  log "Stopping ${SERVICE_NAME} service..."
  systemctl stop "$SERVICE_NAME"
fi

if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
  log "Disabling ${SERVICE_NAME} service..."
  systemctl disable "$SERVICE_NAME"
fi

# ── Remove systemd unit file ──────────────────────────────────────────────────

if [ -f "$SERVICE_FILE" ]; then
  log "Removing systemd unit: $SERVICE_FILE..."
  rm -f "$SERVICE_FILE"
fi

log "Reloading systemd daemon..."
systemctl daemon-reload
systemctl reset-failed 2>/dev/null || true

# ── Remove binary ─────────────────────────────────────────────────────────────

if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
  log "Removing binary: ${INSTALL_DIR}/${BINARY_NAME}..."
  rm -f "${INSTALL_DIR}/${BINARY_NAME}"
fi

# ── Optionally remove data directories ───────────────────────────────────────

if [ "$PURGE" = true ]; then
  log "--purge specified: removing config, data, and log directories..."

  for dir in "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"; do
    if [ -d "$dir" ]; then
      log "  Removing $dir..."
      rm -rf "$dir"
    fi
  done

  log "Purge complete. All mingyue data has been removed."
else
  log "Data directories preserved (use --purge to remove them):"
  for dir in "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"; do
    [ -d "$dir" ] && log "  $dir"
  done
fi

log "Uninstallation complete."
