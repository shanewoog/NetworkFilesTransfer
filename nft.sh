#!/bin/bash
set -euo pipefail

APP_NAME="nft"
DEFAULT_INSTALL_DIR="/opt/nft"
DEFAULT_SERVICE_NAME="nft"
DEFAULT_SERVICE_USER="root"
LEGACY_SERVICE_NAME="NetworkFilesTransfer"

# 发布包：buildLinux64.ps1 在本文件末尾追加一行标记和 base64(tar.gz) 数据。
# 标记行之后的内容只被当作数据，不参与执行。
PAYLOAD_MARKER="__NFT_PAYLOAD_BELOW__"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="$SCRIPT_DIR/$(basename "${BASH_SOURCE[0]}")"

WORK_DIR=""
PAYLOAD_DIR=""

cleanup() {
if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
rm -rf "$WORK_DIR"
fi
}
trap cleanup EXIT

die() {
echo "Error: $*" >&2
exit 1
}

INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
case "$INSTALL_DIR" in
/*) ;;
*) die "INSTALL_DIR must be an absolute path: $INSTALL_DIR" ;;
esac
if [ "$INSTALL_DIR" != "/" ]; then
INSTALL_DIR="${INSTALL_DIR%/}"
fi

META_FILE="$INSTALL_DIR/.install-meta"

read_meta() {
local key="$1"
[ -f "$META_FILE" ] || return 0
sed -n "s/^${key}=//p" "$META_FILE" | head -n 1
}

META_SERVICE_NAME="$(read_meta META_SERVICE_NAME)"
META_SERVICE_USER="$(read_meta META_SERVICE_USER)"
META_PORT="$(read_meta META_PORT)"

if [ -z "$META_SERVICE_NAME" ]; then
META_SERVICE_NAME="$(read_meta SERVICE_NAME)"
fi
if [ -z "$META_SERVICE_USER" ]; then
META_SERVICE_USER="$(read_meta SERVICE_USER)"
fi
if [ -z "$META_PORT" ]; then
META_PORT="$(read_meta PORT)"
fi

SERVICE_NAME="${SERVICE_NAME:-${META_SERVICE_NAME:-$DEFAULT_SERVICE_NAME}}"
SERVICE_USER="${SERVICE_USER:-${META_SERVICE_USER:-$DEFAULT_SERVICE_USER}}"
PORT="${PORT:-${META_PORT:-}}"

case "$SERVICE_NAME" in
""|*[!A-Za-z0-9_.@-]*) die "invalid service name: $SERVICE_NAME" ;;
esac

SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
SERVICE_FILE="$SYSTEMD_DIR/${SERVICE_NAME}.service"
LEGACY_SERVICE_FILE="$SYSTEMD_DIR/${LEGACY_SERVICE_NAME}.service"

RELEASE_FILES=(
"nft"
"index.html"
"download.html"
"app.css"
"qrcode.min.js"
"favicon.svg"
"version.txt"
"nft-linux-guide.md"
)

usage() {
cat <<EOF
Usage: sudo ./nft.sh [command]

Without a command:
install directory missing and service not running -> install
install directory exists and service running -> update
otherwise this help is printed

Commands:
install   Install to $DEFAULT_INSTALL_DIR (keeps existing config, does not start service)
update    Stop service, replace files, keep config/data, restart service
uninstall Stop and remove the systemd service
status    Show service status

Environment:
INSTALL_DIR   Install directory (default: $DEFAULT_INSTALL_DIR)
SERVICE_NAME  systemd service name (default: $DEFAULT_SERVICE_NAME)
SERVICE_USER  systemd service user (default: $DEFAULT_SERVICE_USER)
SYSTEMD_DIR   systemd unit directory (default: /etc/systemd/system)
EOF
}

require_root() {
if [ "$(id -u)" -ne 0 ]; then
die "this command must be run as root"
fi
}

require_systemctl() {
command -v systemctl >/dev/null 2>&1 || die "systemctl is not available"
}

payload_line_count() {
awk -v m="$PAYLOAD_MARKER" 'found { n++ } $0 == m { found = 1 } END { print n + 0 }' "$SCRIPT_PATH"
}

prepare_payload() {
[ -f "$SCRIPT_PATH" ] || die "run this script from a file: sudo ./nft.sh <command>"
command -v awk >/dev/null 2>&1 || die "awk is not available"
command -v base64 >/dev/null 2>&1 || die "base64 is not available"
command -v tar >/dev/null 2>&1 || die "tar is not available"

local lines
lines="$(payload_line_count)"
if [ "${lines:-0}" -le 0 ]; then
die "no embedded payload in $SCRIPT_PATH; use the nft.sh built by buildLinux64.ps1"
fi

WORK_DIR="$(mktemp -d)" || die "failed to create temp directory"
PAYLOAD_DIR="$WORK_DIR/payload"
mkdir -p "$PAYLOAD_DIR"

if ! awk -v m="$PAYLOAD_MARKER" 'found { print } $0 == m { found = 1 }' "$SCRIPT_PATH" | base64 -d | tar -xzf - -C "$PAYLOAD_DIR"; then
die "failed to extract embedded payload"
fi
}

extract_port() {
local config_path="$1"
local port
port="$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$config_path" | head -n 1)"
printf '%s' "${port:-9000}"
}

resolve_port() {
if [ -f "$INSTALL_DIR/config.json" ]; then
PORT="$(extract_port "$INSTALL_DIR/config.json")"
elif [ -z "${PORT:-}" ]; then
PORT="${META_PORT:-9000}"
fi
case "$PORT" in
""|*[!0-9]*) PORT="9000" ;;
esac
}

open_firewall() {
if command -v firewall-cmd >/dev/null 2>&1; then
firewall-cmd --permanent --add-port="${PORT}/tcp" >/dev/null 2>&1 || true
firewall-cmd --reload >/dev/null 2>&1 || true
fi
}

close_firewall() {
if command -v firewall-cmd >/dev/null 2>&1 && [ -n "${PORT:-}" ]; then
firewall-cmd --permanent --remove-port="${PORT}/tcp" >/dev/null 2>&1 || true
firewall-cmd --reload >/dev/null 2>&1 || true
fi
}

write_meta() {
{
printf 'META_SERVICE_NAME=%s\n' "$SERVICE_NAME"
printf 'META_SERVICE_USER=%s\n' "$SERVICE_USER"
printf 'META_PORT=%s\n' "$PORT"
} > "$META_FILE"
}

write_service_file() {
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
}

cleanup_legacy_service() {
if [ "$SERVICE_NAME" != "$LEGACY_SERVICE_NAME" ]; then
systemctl stop "$LEGACY_SERVICE_NAME" 2>/dev/null || true
systemctl disable "$LEGACY_SERVICE_NAME" 2>/dev/null || true
rm -f "$LEGACY_SERVICE_FILE"
fi
}

check_payload_files() {
local file
for file in "${RELEASE_FILES[@]}"; do
[ -f "$PAYLOAD_DIR/$file" ] || die "missing file in embedded payload: $file"
done
[ -f "$PAYLOAD_DIR/config.json" ] || die "missing file in embedded payload: config.json"
}

copy_payload_files() {
local file
for file in "${RELEASE_FILES[@]}"; do
cp -f "$PAYLOAD_DIR/$file" "$INSTALL_DIR/$file"
done

if [ -f "$INSTALL_DIR/config.json" ]; then
echo "Kept existing config: $INSTALL_DIR/config.json"
else
cp -f "$PAYLOAD_DIR/config.json" "$INSTALL_DIR/config.json"
echo "Created config: $INSTALL_DIR/config.json"
fi

if [ "$SCRIPT_PATH" != "$INSTALL_DIR/nft.sh" ]; then
awk -v m="$PAYLOAD_MARKER" '{ print } $0 == m { exit }' "$SCRIPT_PATH" > "$INSTALL_DIR/nft.sh" || die "failed to install nft.sh"
fi

chmod +x "$INSTALL_DIR/$APP_NAME" "$INSTALL_DIR/nft.sh"
rm -f "$INSTALL_DIR/install.sh" "$INSTALL_DIR/uninstall.sh"
}

install_service() {
mkdir -p "$INSTALL_DIR"
check_payload_files
systemctl stop "$SERVICE_NAME" 2>/dev/null || true
copy_payload_files
resolve_port
write_meta
cleanup_legacy_service
write_service_file
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
open_firewall
}

cmd_install() {
require_root
require_systemctl
prepare_payload

echo ">>> Installing $APP_NAME"
echo "Script: $SCRIPT_PATH"
echo "Install directory: $INSTALL_DIR"
echo "Service name: $SERVICE_NAME"
echo "Service user: $SERVICE_USER"

install_service

echo "------------------------------------------------"
echo "Install complete (service not started)"
echo "Config: $INSTALL_DIR/config.json"
echo "Start service: systemctl start $SERVICE_NAME"
echo "Status: $INSTALL_DIR/nft.sh status"
echo "------------------------------------------------"
}

cmd_update() {
require_root
require_systemctl
prepare_payload

[ -d "$INSTALL_DIR" ] || die "install directory not found: $INSTALL_DIR (run install first)"
[ -f "$INSTALL_DIR/$APP_NAME" ] || die "$APP_NAME not found in $INSTALL_DIR (run install first)"

local old_version new_version
old_version="$(cat "$INSTALL_DIR/version.txt" 2>/dev/null || echo "unknown")"
new_version="$(cat "$PAYLOAD_DIR/version.txt" 2>/dev/null || echo "unknown")"

echo ">>> Updating $APP_NAME: $old_version -> $new_version"
echo "Install directory: $INSTALL_DIR"
echo "Service name: $SERVICE_NAME"

systemctl stop "$SERVICE_NAME" 2>/dev/null || true

copy_payload_files
resolve_port
write_meta
cleanup_legacy_service
write_service_file
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
open_firewall

if ! systemctl start "$SERVICE_NAME"; then
die "failed to start $SERVICE_NAME; check: journalctl -u $SERVICE_NAME -n 50"
fi

if [ "$SCRIPT_PATH" != "$INSTALL_DIR/nft.sh" ]; then
if rm -f "$SCRIPT_PATH"; then
echo "Removed update script: $SCRIPT_PATH"
else
echo "Warning: failed to remove update script: $SCRIPT_PATH" >&2
fi
fi

echo "------------------------------------------------"
echo "Update complete: $new_version"
echo "Status: systemctl status $SERVICE_NAME"
echo "------------------------------------------------"
}

cmd_uninstall() {
require_root
require_systemctl
case "$INSTALL_DIR" in
/|"") die "refusing to remove $INSTALL_DIR" ;;
esac

echo ">>> Uninstalling $APP_NAME"
echo "Install directory: $INSTALL_DIR"
echo "Service name: $SERVICE_NAME"

systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true
systemctl stop "$LEGACY_SERVICE_NAME" 2>/dev/null || true
systemctl disable "$LEGACY_SERVICE_NAME" 2>/dev/null || true

rm -f "$SERVICE_FILE" "$LEGACY_SERVICE_FILE"
systemctl daemon-reload

resolve_port
close_firewall

if [ -d "$INSTALL_DIR" ]; then
echo "Files in install directory:"
ls -la "$INSTALL_DIR"
echo "Warning: removing the install directory deletes config, uploads, temp and share.db."
local confirm=""
read -r -p "Remove install directory $INSTALL_DIR and all contents? (y/N): " confirm || true
if [[ "$confirm" =~ ^[Yy]$ ]]; then
cd /
rm -rf "$INSTALL_DIR"
echo "Removed: $INSTALL_DIR"
else
echo "Kept install directory: $INSTALL_DIR"
fi
fi

echo "------------------------------------------------"
echo "Uninstall complete"
echo "------------------------------------------------"
}

cmd_status() {
require_systemctl
systemctl status "$SERVICE_NAME" --no-pager || true
echo "Config: $INSTALL_DIR/config.json"
echo "Logs: journalctl -u $SERVICE_NAME -f"
}

is_service_active() {
systemctl is-active --quiet "$SERVICE_NAME" >/dev/null 2>&1
}

auto_command() {
if [ ! -d "$INSTALL_DIR" ] && ! is_service_active; then
echo ">>> No install directory and service not running: install"
cmd_install
return
fi

if [ -d "$INSTALL_DIR" ] && is_service_active; then
echo ">>> Install directory exists and service is running: update"
cmd_update
return
fi

if [ ! -d "$INSTALL_DIR" ]; then
echo "Install directory not found: $INSTALL_DIR" >&2
else
echo "Install directory exists but service $SERVICE_NAME is not running." >&2
echo "Start it with: systemctl start $SERVICE_NAME" >&2
echo "Or reinstall with: sudo ./nft.sh install" >&2
fi
usage
exit 1
}

case "${1:-}" in
"")
auto_command
;;
install)
cmd_install
;;
update)
cmd_update
;;
uninstall)
cmd_uninstall
;;
status)
cmd_status
;;
*)
usage
exit 1
;;
esac

exit 0
