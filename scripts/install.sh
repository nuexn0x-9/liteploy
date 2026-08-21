#!/usr/bin/env bash
# LITEPLOY One-Command Production Installer Script
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 1. Root Privilege Check
if [ "$(id -u)" -ne 0 ]; then
    log_error "This script must be run as root (or using sudo)."
fi

# 2. Operating System Check
if [ "$(uname -s)" != "Linux" ]; then
    log_error "LITEPLOY installer only supports Linux OS."
fi

# 3. CPU Architecture Detection
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64|amd64)
        BINARY_ARCH="amd64"
        ;;
    aarch64|arm64)
        BINARY_ARCH="arm64"
        ;;
    *)
        log_error "Unsupported CPU architecture: ${ARCH}. LITEPLOY supports amd64 and arm64."
        ;;
esac

log_info "Detected Architecture: linux-${BINARY_ARCH}"

# 4. Dependency Checks
command -v curl >/dev/null 2>&1 || log_error "curl is required but not installed."

# Verify Docker
if ! command -v docker >/dev/null 2>&1; then
    log_warn "Docker Engine is not installed or not in PATH."
    log_warn "Installing Docker is recommended before continuing."
    log_info "You can install Docker via: curl -fsSL https://get.docker.com | sh"
else
    log_info "Docker detected: $(docker --version)"
fi

# 5. Download Binary
REPO="nuexn0x-9/liteploy"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/liteploy-linux-${BINARY_ARCH}"

# Fallback URL if downloading main branch raw compiled binary during initial setup
RAW_FALLBACK_URL="https://raw.githubusercontent.com/${REPO}/main/bin/liteploy-linux-${BINARY_ARCH}"

TARGET_BIN="/usr/local/bin/liteploy"
TMP_BIN="/tmp/liteploy_download_${BINARY_ARCH}"

log_info "Downloading LITEPLOY binary for linux-${BINARY_ARCH}..."

if curl -fsSL "${DOWNLOAD_URL}" -o "${TMP_BIN}" 2>/dev/null; then
    log_info "Downloaded release binary from GitHub Releases."
elif curl -fsSL "${RAW_FALLBACK_URL}" -o "${TMP_BIN}" 2>/dev/null; then
    log_info "Downloaded binary from main repository fallback."
else
    log_error "Failed to download LITEPLOY binary from ${DOWNLOAD_URL}. Please check internet connection or repository status."
fi

chmod +x "${TMP_BIN}"
mv -f "${TMP_BIN}" "${TARGET_BIN}"
log_success "Installed binary to ${TARGET_BIN}"

# 6. Create Data Directory
DATA_DIR="/var/lib/liteploy/data"
mkdir -p "${DATA_DIR}"
chmod 755 /var/lib/liteploy
log_info "Created data directory: ${DATA_DIR}"

# 7. Systemd Service Setup
SERVICE_FILE="/etc/systemd/system/liteploy.service"

if command -v systemctl >/dev/null 2>&1; then
    log_info "Configuring Systemd service..."
    cat <<EOF > "${SERVICE_FILE}"
[Unit]
Description=LITEPLOY — Lightweight Docker Deployment Platform
After=network.target docker.service
Requires=docker.service
Wants=caddy.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/var/lib/liteploy
ExecStart=${TARGET_BIN}
Restart=always
RestartSec=5s

# Security & Environment
Environment=LITEPLOY_ADDR=:8080
Environment=LITEPLOY_DATA_DIR=${DATA_DIR}
Environment=LITEPLOY_CADDY_ADMIN=http://localhost:2019
Environment=LITEPLOY_LOG_LEVEL=info
Environment=LITEPLOY_LOG_JSON=true

LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable liteploy.service
    systemctl restart liteploy.service
    log_success "LITEPLOY Systemd service created and started!"
else
    log_warn "systemd not detected. Please run LITEPLOY manually using: ${TARGET_BIN}"
fi

# 8. Print Success Information
IP_ADDR="$(curl -s https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"

echo -e "\n=================================================="
echo -e "${GREEN}🎉 LITEPLOY Installation Complete!${NC}"
echo -e "=================================================="
echo -e "Dashboard URL: ${BLUE}http://${IP_ADDR}:8080${NC}"
echo -e "Data Storage:  ${DATA_DIR}"
echo -e "Service Logs:  sudo journalctl -u liteploy -f"
echo -e "==================================================\n"
