# NetworkFilesTransfer build script

$versionPrefix = "1.0"
$linuxBinaryName = "nft"

function Get-NextVersion {
    param(
        [string]$VersionPrefix
    )

    $versionFile = "version.txt"
    $nextPatch = 0

    if (Test-Path $versionFile) {
        $currentVersion = ((Get-Content $versionFile -Raw) -replace '^\uFEFF', '').Trim()
        if ($currentVersion -match '^(?<major>\d+)\.(?<minor>\d+)\.(?<patch>\d+)$') {
            $currentPrefix = "$($Matches.major).$($Matches.minor)"
            if ($currentPrefix -eq $VersionPrefix) {
                $nextPatch = [int]$Matches.patch + 1
            }
        }
    }

    return "$VersionPrefix.$nextPatch"
}

function Write-UnixTextFile {
    param(
        [string]$Path,
        [string]$Content
    )

    $normalized = $Content -replace "`r?`n", "`n"
    $encoding = New-Object System.Text.UTF8Encoding($false)
    $fullPath = Join-Path (Get-Location) $Path
    $directory = Split-Path -Parent $fullPath
    if ($directory -and !(Test-Path $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    [System.IO.File]::WriteAllText($fullPath, $normalized, $encoding)
}

function Write-MarkdownTextFile {
    param(
        [string]$Path,
        [string]$Content
    )

    $normalized = $Content -replace "`r?`n", "`r`n"
    $encoding = New-Object System.Text.UTF8Encoding($true)
    $fullPath = Join-Path (Get-Location) $Path
    $directory = Split-Path -Parent $fullPath
    if ($directory -and !(Test-Path $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    [System.IO.File]::WriteAllText($fullPath, $normalized, $encoding)
}

function Write-InstallScripts {
    $installScript = @'
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
'@

    $uninstallScript = @'
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
'@

    Write-UnixTextFile -Path "install.sh" -Content $installScript
    Write-UnixTextFile -Path "uninstall.sh" -Content $uninstallScript
}

function Write-LinuxOperationGuide {
    param(
        [string]$Path
    )

    $u = {
        param([int[]]$CodePoints)
        -join ($CodePoints | ForEach-Object { [char]$_ })
    }

    $guideLines = @(
        "# NFT Linux $(& $u @(0x64CD,0x4F5C,0x6587,0x6863))"
        ""
        "$(& $u @(0x672C,0x6587,0x6863,0x7528,0x4E8E)) Linux $(& $u @(0x670D,0x52A1,0x5668,0x4E0A,0x7684,0x5B89,0x88C5,0x3001,0x5378,0x8F7D,0x548C,0x5E38,0x7528,0x670D,0x52A1,0x7BA1,0x7406,0x3002))"
        ""
        "## $(& $u @(0x5B89,0x88C5))"
        ""
        "1. $(& $u @(0x8FDB,0x5165,0x53D1,0x5E03,0x5305,0x6240,0x5728,0x76EE,0x5F55))"
        ""
        '```bash'
        'cd /path/to/dist'
        '```'
        ""
        "2. $(& $u @(0x7ED9,0x4E8C,0x8FDB,0x5236,0x548C,0x811A,0x672C,0x52A0,0x6267,0x884C,0x6743,0x9650))"
        ""
        '```bash'
        'chmod +x nft install.sh uninstall.sh'
        '```'
        ""
        "3. $(& $u @(0x6267,0x884C,0x5B89,0x88C5))"
        ""
        '```bash'
        'sudo ./install.sh'
        '```'
        ""
        "$(& $u @(0x9ED8,0x8BA4,0x884C,0x4E3A)):"
        ""
        "- $(& $u @(0x5B89,0x88C5,0x76EE,0x5F55)): $(& $u @(0x6267,0x884C,0x5B89,0x88C5,0x547D,0x4EE4,0x65F6,0x7684,0x5F53,0x524D,0x76EE,0x5F55))"
        "- $(& $u @(0x670D,0x52A1,0x540D)): ``nft``"
        "- $(& $u @(0x670D,0x52A1,0x7528,0x6237)): ``root``"
        "- $(& $u @(0x9632,0x706B,0x5899)): $(& $u @(0x81EA,0x52A8,0x6309)) ``config.json`` $(& $u @(0x4E2D,0x7684)) ``port`` $(& $u @(0x5F00,0x653E,0x5BF9,0x5E94)) TCP $(& $u @(0x7AEF,0x53E3))"
        ""
        "$(& $u @(0x53EF,0x9009,0x53C2,0x6570)):"
        ""
        "- $(& $u @(0x6307,0x5B9A,0x5B89,0x88C5,0x76EE,0x5F55))"
        ""
        '```bash'
        'sudo INSTALL_DIR=/opt/nft ./install.sh'
        '```'
        ""
        "- $(& $u @(0x6307,0x5B9A,0x670D,0x52A1,0x540D))"
        ""
        '```bash'
        'sudo SERVICE_NAME=nft ./install.sh'
        '```'
        ""
        "## $(& $u @(0x5378,0x8F7D))"
        ""
        "$(& $u @(0x5728,0x5B89,0x88C5,0x76EE,0x5F55,0x6267,0x884C)):"
        ""
        '```bash'
        'sudo ./uninstall.sh'
        '```'
        ""
        "$(& $u @(0x5378,0x8F7D,0x811A,0x672C,0x4F1A)):"
        ""
        "- $(& $u @(0x505C,0x6B62,0x5E76,0x7981,0x7528,0x5F53,0x524D)) ``nft`` $(& $u @(0x670D,0x52A1))"
        "- $(& $u @(0x540C,0x65F6,0x5C1D,0x8BD5,0x6E05,0x7406,0x65E7,0x7684)) ``NetworkFilesTransfer`` $(& $u @(0x670D,0x52A1))"
        "- $(& $u @(0x5220,0x9664,0x5BF9,0x5E94,0x7684)) systemd $(& $u @(0x670D,0x52A1,0x6587,0x4EF6))"
        "- $(& $u @(0x8BE2,0x95EE,0x662F,0x5426,0x5220,0x9664,0x5B89,0x88C5,0x76EE,0x5F55))"
        ""
        "## $(& $u @(0x5E38,0x7528,0x670D,0x52A1,0x7BA1,0x7406,0x547D,0x4EE4))"
        ""
        '```bash'
        'systemctl start nft'
        'systemctl stop nft'
        'systemctl restart nft'
        'systemctl status nft'
        'journalctl -u nft -f'
        '```'
    )

    $guide = $guideLines -join "`r`n"

    Write-MarkdownTextFile -Path $Path -Content $guide
}

$nextVersion = Get-NextVersion -VersionPrefix $versionPrefix

Write-Host "--- Build Version: $nextVersion ---" -ForegroundColor Cyan
Write-Host "--- Start Compiling Linux (amd64) Binary ---" -ForegroundColor Cyan

$previousGOOS = [Environment]::GetEnvironmentVariable("GOOS", "Process")
$previousGOARCH = [Environment]::GetEnvironmentVariable("GOARCH", "Process")

try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"

    Write-InstallScripts
    Write-LinuxOperationGuide -Path "nft-linux-guide.md"

    go build -ldflags "-X main.appVersion=$nextVersion" -o $linuxBinaryName .

    if ($LASTEXITCODE -ne 0) {
        Write-Host "Error: Compilation failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }

    Write-UnixTextFile -Path "version.txt" -Content $nextVersion

    if (!(Test-Path "dist")) {
        New-Item -ItemType Directory -Path "dist" | Out-Null
        Write-Host "Created 'dist' directory."
    }

    Write-LinuxOperationGuide -Path "dist/nft-linux-guide.md"

    Write-Host "--- Collecting deployment files into 'dist' ---" -ForegroundColor Cyan

    Copy-Item $linuxBinaryName "dist/" -Force
    Copy-Item "index.html" "dist/" -Force
    Copy-Item "config.json" "dist/" -Force
    Copy-Item "version.txt" "dist/" -Force
    Copy-Item "app.css" "dist/" -Force
    Copy-Item "qrcode.min.js" "dist/" -Force
    Copy-Item "favicon.svg" "dist/" -Force
    Copy-Item "download.html" "dist/" -Force
    Copy-Item "install.sh" "dist/" -Force
    Copy-Item "uninstall.sh" "dist/" -Force

    if (Test-Path $linuxBinaryName) {
        try {
            Remove-Item $linuxBinaryName -Force -ErrorAction Stop
        } catch {
            # Ignore cleanup failures for the temporary build artifact.
        }
    }
} finally {
    if ([string]::IsNullOrEmpty($previousGOOS)) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $previousGOOS
    }

    if ([string]::IsNullOrEmpty($previousGOARCH)) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $previousGOARCH
    }
}

Write-Host "------------------------------------------------" -ForegroundColor Green
Write-Host "Build Complete! Files are in the 'dist' folder." -ForegroundColor Green
Write-Host "------------------------------------------------" -ForegroundColor Green

