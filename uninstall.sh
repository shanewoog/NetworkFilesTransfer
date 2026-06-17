#!/bin/bash
set -euo pipefail

APP_NAME="nft"
DEFAULT_SERVICE_NAME="nft"
LEGACY_SERVICE_NAME="NetworkFilesTransfer"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="$SCRIPT_DIR"
SERVICE_NAME="$DEFAULT_SERVICE_NAME"
SERVICE_USER="root"
PORT=""
META_FILE="$INSTALL_DIR/.install-meta"

if [ -f "$META_FILE" ]; then
    # shellcheck disable=SC1090
    . "$META_FILE"
fi

SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
LEGACY_SERVICE_FILE="/etc/systemd/system/${LEGACY_SERVICE_NAME}.service"

echo ">>> Uninstalling $APP_NAME"
echo "Install directory: $INSTALL_DIR"
echo "Service name: $SERVICE_NAME"

systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true
systemctl stop "$LEGACY_SERVICE_NAME" 2>/dev/null || true
systemctl disable "$LEGACY_SERVICE_NAME" 2>/dev/null || true

if [ -f "$SERVICE_FILE" ]; then
    rm -f "$SERVICE_FILE"
fi
if [ -f "$LEGACY_SERVICE_FILE" ]; then
    rm -f "$LEGACY_SERVICE_FILE"
fi

systemctl daemon-reload

if command -v firewall-cmd >/dev/null 2>&1 && [ -n "${PORT:-}" ]; then
    firewall-cmd --permanent --remove-port="${PORT}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
fi

echo "Files in install directory:"
ls -la "$INSTALL_DIR"

read -r -p "Remove install directory $INSTALL_DIR and all contents? (y/N): " confirm
if [[ "$confirm" =~ ^[Yy]$ ]]; then
    cd /
    rm -rf "$INSTALL_DIR"
    echo "Removed: $INSTALL_DIR"
else
    echo "Kept install directory: $INSTALL_DIR"
fi

echo "------------------------------------------------"
echo "Uninstall complete"
echo "------------------------------------------------"