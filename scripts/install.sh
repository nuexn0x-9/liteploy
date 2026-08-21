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
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
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
log_info "Detecting architecture..."
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
log_ok "linux-${BINARY_ARCH} detected"

# 4. Dependency Checks
command -v curl >/dev/null 2>&1 || log_error "curl is required but not installed."

log_info "Detecting Docker..."
if ! command -v docker >/dev/null 2>&1; then
    log_warn "Docker Engine is not installed."
    log_info "Installing Docker..."
    curl -fsSL https://get.docker.com | sh >/dev/null 2>&1 || log_error "Failed to install Docker automatically."
    log_ok "Docker installed"
else
    log_ok "Docker detected"
fi

log_info "Detecting Git..."
if ! command -v git >/dev/null 2>&1; then
    log_warn "Git is not installed."
    log_info "Installing Git..."
    apt-get update -yqq && apt-get install -y git >/dev/null 2>&1 || log_error "Failed to install Git."
    log_ok "Git installed"
else
    log_ok "Git detected"
fi

# 5. Download Binary
REPO="nuexn0x-9/liteploy"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/liteploy-linux-${BINARY_ARCH}"

TARGET_BIN="/usr/local/bin/liteploy"
TMP_BIN="/tmp/liteploy_download_${BINARY_ARCH}"

log_info "Installing Liteploy..."

# Try downloading from GitHub Releases
DOWNLOAD_SUCCESS=false
if curl -sL -w "%{http_code}" "${DOWNLOAD_URL}" -o "${TMP_BIN}" | grep -q '200'; then
    # Check if file size is > 5MB to ensure it's not a tiny XML error page
    FILESIZE=$(stat -c%s "${TMP_BIN}" 2>/dev/null || stat -f%z "${TMP_BIN}" 2>/dev/null || echo 0)
    if [ "$FILESIZE" -gt 5000000 ]; then
        DOWNLOAD_SUCCESS=true
    fi
fi

if [ "$DOWNLOAD_SUCCESS" = false ]; then
    log_warn "GitHub Release not found or download failed."
    log_info "Falling back to source build..."
    
    # Check if git is installed
    if ! command -v git >/dev/null 2>&1; then
        apt-get update -yqq && apt-get install -y git >/dev/null 2>&1 || log_error "Git is required for fallback build. Please install git."
    fi
    
    # Check if Go is installed
    if ! command -v go >/dev/null 2>&1; then
        log_info "Installing Go compiler temporarily..."
        GO_VERSION="1.22.1"
        curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-${BINARY_ARCH}.tar.gz" -o /tmp/go.tar.gz
        rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
        export PATH=$PATH:/usr/local/go/bin
    fi
    
    # Clone and build
    rm -rf /tmp/liteploy-source
    git clone --depth 1 "https://github.com/${REPO}.git" /tmp/liteploy-source >/dev/null 2>&1 || log_error "Failed to clone repository."
    cd /tmp/liteploy-source
    
    VERSION=$(git rev-parse --short HEAD)
    LDFLAGS="-s -w -X github.com/liteploy/liteploy/internal/system.Version=source-fallback -X github.com/liteploy/liteploy/internal/system.CommitSHA=${VERSION}"
    
    log_info "Compiling binary from source..."
    GOOS=linux GOARCH=${BINARY_ARCH} go build -ldflags "${LDFLAGS}" -o "${TMP_BIN}" ./cmd/liteploy || log_error "Source compilation failed."
    cd - >/dev/null
    rm -rf /tmp/liteploy-source
fi

chmod +x "${TMP_BIN}"
mv -f "${TMP_BIN}" "${TARGET_BIN}"
log_ok "Binary installed"

# 6. Create Data Directory
DATA_DIR="/var/lib/liteploy/data"
mkdir -p "${DATA_DIR}"
chmod 755 /var/lib/liteploy

# 7. Systemd Service Setup
SERVICE_FILE="/etc/systemd/system/liteploy.service"

if command -v systemctl >/dev/null 2>&1; then
    log_info "Starting service..."
    # Generate or retain existing session secret
    SESSION_SECRET=""
    if [ -f "${SERVICE_FILE}" ] && grep -q "LITEPLOY_SESSION_SECRET=" "${SERVICE_FILE}"; then
        SESSION_SECRET=$(grep "LITEPLOY_SESSION_SECRET=" "${SERVICE_FILE}" | head -n 1 | cut -d'=' -f2)
    fi
    if [ -z "${SESSION_SECRET}" ]; then
        SESSION_SECRET=$(openssl rand -hex 32 2>/dev/null || tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 64 2>/dev/null || echo "default-liteploy-session-secret-change-me")
    fi

    cat <<EOF > "${SERVICE_FILE}"
[Unit]
Description=LITEPLOY - Lightweight Docker Deployment Platform
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
Environment=LITEPLOY_SESSION_SECRET=${SESSION_SECRET}
Environment=LITEPLOY_LOG_LEVEL=info
Environment=LITEPLOY_LOG_JSON=true

LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable liteploy.service >/dev/null 2>&1
    systemctl restart liteploy.service
    log_ok "Liteploy running"
else
    log_warn "systemd not detected. Please run LITEPLOY manually using: ${TARGET_BIN}"
fi

# 8. Print Success Information
IP_ADDR="$(curl -s https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"

echo -e "\n=================================================="
echo -e "${GREEN}🚀 LITEPLOY Installation Complete!${NC}"
echo -e "=================================================="
echo -e "Dashboard URL: ${BLUE}http://${IP_ADDR}:8080${NC}"
echo -e "Data Storage:  ${DATA_DIR}"
echo -e "Service Logs:  sudo journalctl -u liteploy -f"
echo -e "==================================================\n"
