#!/bin/bash
set -euo pipefail

APP_NAME="nft"
DEFAULT_SERVICE_NAME="nft"
LEGACY_SERVICE_NAME="NetworkFilesTransfer"
DEFAULT_SERVICE_USER="root"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$(pwd)}"
SERVICE_NAME="${SERVICE_NAME:-$DEFAULT_SERVICE_NAME}"
SERVICE_USER="${SERVICE_USER:-$DEFAULT_SERVICE_USER}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
LEGACY_SERVICE_FILE="/etc/systemd/system/${LEGACY_SERVICE_NAME}.service"
META_FILE="$INSTALL_DIR/.install-meta"

extract_port() {
    local config_path="$1"
    local port
    port="$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$config_path" | head -n 1)"
    if [ -z "${port:-}" ]; then
        port="9000"
    fi
    printf '%s' "$port"
}

PORT="$(extract_port "$SCRIPT_DIR/config.json")"

echo ">>> Installing $APP_NAME"
echo "Source directory: $SCRIPT_DIR"
echo "Install directory: $INSTALL_DIR"
echo "Service name: $SERVICE_NAME"
echo "Service user: $SERVICE_USER"
echo "Port: $PORT"

mkdir -p "$INSTALL_DIR"

FILES=(
    "$APP_NAME"
    "index.html"
    "download.html"
    "config.json"
    "version.txt"
    "app.css"
    "qrcode.min.js"
    "favicon.svg"
    "install.sh"
    "uninstall.sh"
    "nft-linux-guide.md"
)

if [ "$SCRIPT_DIR" != "$INSTALL_DIR" ]; then
    for file in "${FILES[@]}"; do
        if [ -f "$SCRIPT_DIR/$file" ]; then
            cp "$SCRIPT_DIR/$file" "$INSTALL_DIR/$file"
        fi
    done
fi

chmod +x "$INSTALL_DIR/$APP_NAME" "$INSTALL_DIR/install.sh" "$INSTALL_DIR/uninstall.sh"

{
    printf 'SERVICE_NAME=%q\n' "$SERVICE_NAME"
    printf 'INSTALL_DIR=%q\n' "$INSTALL_DIR"
    printf 'PORT=%q\n' "$PORT"
    printf 'SERVICE_USER=%q\n' "$SERVICE_USER"
} > "$META_FILE"

if [ "$SERVICE_NAME" != "$LEGACY_SERVICE_NAME" ]; then
    systemctl stop "$LEGACY_SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$LEGACY_SERVICE_NAME" 2>/dev/null || true
    if [ -f "$LEGACY_SERVICE_FILE" ]; then
        rm -f "$LEGACY_SERVICE_FILE"
    fi
fi

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=NetworkFilesTransfer Service
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$APP_NAME
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
systemctl restart "$SERVICE_NAME"

if command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${PORT}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
fi

echo "------------------------------------------------"
echo "Install complete"
echo "Install directory: $INSTALL_DIR"
echo "Service status: $(systemctl is-active "$SERVICE_NAME" 2>/dev/null || true)"
echo "Service commands:"
echo "  Start:   systemctl start $SERVICE_NAME"
echo "  Stop:    systemctl stop $SERVICE_NAME"
echo "  Restart: systemctl restart $SERVICE_NAME"
echo "  Status:  systemctl status $SERVICE_NAME"
echo "  Logs:    journalctl -u $SERVICE_NAME -f"
echo "------------------------------------------------"