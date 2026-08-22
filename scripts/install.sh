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

# Ensure openssl is installed for secure token generation
if ! command -v openssl >/dev/null 2>&1; then
    log_info "Installing openssl..."
    apt-get update -yqq && apt-get install -y openssl >/dev/null 2>&1 || true
fi

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

# 5. Download or Build Binary
REPO="nuexn0x-9/liteploy"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/liteploy-linux-${BINARY_ARCH}"

TARGET_BIN="/usr/local/bin/liteploy"
TMP_BIN="/tmp/liteploy_download_${BINARY_ARCH}"

log_info "Installing Liteploy binary..."

DOWNLOAD_SUCCESS=false
LATEST_REMOTE_COMMIT=$(git ls-remote "https://github.com/${REPO}.git" HEAD 2>/dev/null | awk '{print $1}' | cut -c1-7 || echo "")

if [ "${FORCE_BUILD:-0}" != "1" ] && curl -sL -w "%{http_code}" "${DOWNLOAD_URL}" -o "${TMP_BIN}" | grep -q '200'; then
    # Check if file size is > 5MB to ensure it's not a tiny XML error page
    FILESIZE=$(stat -c%s "${TMP_BIN}" 2>/dev/null || stat -f%z "${TMP_BIN}" 2>/dev/null || echo 0)
    if [ "$FILESIZE" -gt 5000000 ]; then
        chmod +x "${TMP_BIN}" 2>/dev/null || true
        BIN_INFO=$("${TMP_BIN}" version 2>/dev/null || echo "")
        
        # If remote commit is known and binary doesn't have it, or binary is outdated, compile from source
        if [ -n "${LATEST_REMOTE_COMMIT}" ] && ! echo "${BIN_INFO}" | grep -q "${LATEST_REMOTE_COMMIT}"; then
            log_warn "GitHub Release binary is behind main (${BIN_INFO}). Latest commit is ${LATEST_REMOTE_COMMIT}."
            log_info "Building latest code directly from GitHub main branch..."
        elif echo "${BIN_INFO}" | grep -q "v1.0.0"; then
            DOWNLOAD_SUCCESS=true
            log_ok "Downloaded latest ${BIN_INFO}"
        fi
    fi
fi

if [ "$DOWNLOAD_SUCCESS" = false ]; then
    log_info "Compiling latest LITEPLOY from GitHub main branch..."
    
    if ! command -v git >/dev/null 2>&1; then
        apt-get update -yqq && apt-get install -y git >/dev/null 2>&1 || log_error "Git is required for fallback build."
    fi
    
    if ! command -v go >/dev/null 2>&1; then
        log_info "Installing Go compiler..."
        GO_VERSION="1.22.1"
        curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-${BINARY_ARCH}.tar.gz" -o /tmp/go.tar.gz
        rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
        export PATH=$PATH:/usr/local/go/bin
    fi
    
    rm -rf /tmp/liteploy-source
    git clone --depth 1 "https://github.com/${REPO}.git" /tmp/liteploy-source >/dev/null 2>&1 || log_error "Failed to clone repository."
    cd /tmp/liteploy-source
    
    VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "latest")
    LDFLAGS="-s -w -X github.com/liteploy/liteploy/internal/system.Version=v1.0.0 -X github.com/liteploy/liteploy/internal/system.CommitSHA=${VERSION}"
    
    log_info "Compiling binary from source..."
    GOOS=linux GOARCH=${BINARY_ARCH} go build -ldflags "${LDFLAGS}" -o "${TMP_BIN}" ./cmd/liteploy || log_error "Source compilation failed."
    cd - >/dev/null
    rm -rf /tmp/liteploy-source
fi

chmod +x "${TMP_BIN}"
mv -f "${TMP_BIN}" "${TARGET_BIN}"
log_ok "Binary installed to ${TARGET_BIN}"

# 6. Create Directories
DATA_DIR="/var/lib/liteploy/data"
mkdir -p "${DATA_DIR}"
mkdir -p /var/lib/liteploy
chmod 755 /var/lib/liteploy

CONF_DIR="/etc/liteploy"
mkdir -p "${CONF_DIR}"
chmod 700 "${CONF_DIR}"

ENV_FILE="${CONF_DIR}/liteploy.env"

# 7. Environment File Setup & Secure Session Secret Generation
log_info "Configuring environment..."

EXISTING_SECRET=""
if [ -f "${ENV_FILE}" ]; then
    EXISTING_SECRET=$(grep -E '^LITEPLOY_SESSION_SECRET=' "${ENV_FILE}" 2>/dev/null | cut -d'=' -f2- | tr -d '"'\'' ')
fi

# If existing secret is missing, too short, or is a literal placeholder, generate a new secure one
if [ -z "${EXISTING_SECRET}" ] || [ "${#EXISTING_SECRET}" -lt 32 ] || [ "${EXISTING_SECRET}" = "LITEPLOY_SESSION_SECRET" ] || [ "${EXISTING_SECRET}" = "default-liteploy-session-secret-change-me" ]; then
    log_info "Generating cryptographic session secret..."
    SESSION_SECRET=""
    if command -v openssl >/dev/null 2>&1; then
        SESSION_SECRET=$(openssl rand -hex 32)
    fi
    if [ -z "${SESSION_SECRET}" ] || [ "${#SESSION_SECRET}" -lt 32 ]; then
        SESSION_SECRET=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
    fi
    
    if [ "${#SESSION_SECRET}" -lt 32 ]; then
        log_error "Failed to generate secure session secret (less than 32 bytes)."
    fi
else
    log_info "Preserving existing session secret from ${ENV_FILE}"
    SESSION_SECRET="${EXISTING_SECRET}"
fi

# Write environment file with strict 600 permissions
cat <<EOF > "${ENV_FILE}"
LITEPLOY_ADDR=:8080
LITEPLOY_DATA_DIR=${DATA_DIR}
LITEPLOY_CADDY_ADMIN=http://127.0.0.1:2019
LITEPLOY_SESSION_SECRET=${SESSION_SECRET}
LITEPLOY_LOG_LEVEL=info
LITEPLOY_LOG_JSON=true
EOF

chmod 600 "${ENV_FILE}"
log_ok "Environment configured in ${ENV_FILE} (chmod 600)"

# 8a. Migrate from legacy caddy.service (host-level) to Docker container.
#     Liteploy now manages Caddy as a Docker container on liteploy-network.
if command -v systemctl > /dev/null 2>&1 && systemctl list-units --full --all 2>/dev/null | grep -q 'caddy.service'; then
    log_warn "Legacy caddy.service detected — migrating to Docker container..."

    # Backup existing Caddy config if present
    if [ -d /etc/caddy ]; then
        CADDY_BACKUP="/var/lib/liteploy/caddy-backup-$(date +%Y%m%d-%H%M%S)"
        mkdir -p "${CADDY_BACKUP}"
        cp -r /etc/caddy "${CADDY_BACKUP}/" 2>/dev/null || true
        log_ok "Caddy config backed up to ${CADDY_BACKUP}"
    fi

    systemctl stop caddy.service 2>/dev/null || true
    systemctl disable caddy.service 2>/dev/null || true
    pkill -9 -f /usr/bin/caddy 2>/dev/null || pkill -9 caddy 2>/dev/null || true
    log_ok "Legacy caddy.service stopped and disabled"
    log_info "Liteploy will start liteploy-caddy Docker container automatically on boot."
fi

# Pre-pull Caddy image and launch liteploy-caddy container
if command -v docker > /dev/null 2>&1; then
    log_info "Ensuring liteploy-network and liteploy-caddy container..."
    docker pull caddy:2-alpine --quiet 2>/dev/null || true
    docker network create liteploy-network 2>/dev/null || true
    mkdir -p "${DATA_DIR}/caddy"

    cat <<'CADDY_EOF' > "${DATA_DIR}/caddy/Caddyfile"
{
    admin 0.0.0.0:2019
}
:80 {
    respond "OK" 200
}
CADDY_EOF

    if ! docker ps --format '{{.Names}}' | grep -q '^liteploy-caddy$'; then
        docker rm -f liteploy-caddy 2>/dev/null || true
        docker run -d \
          --name liteploy-caddy \
          --network liteploy-network \
          --restart unless-stopped \
          -v "${DATA_DIR}/caddy:/data" \
          -v "${DATA_DIR}/caddy/Caddyfile:/etc/caddy/Caddyfile" \
          -p 80:80 \
          -p 443:443 \
          -p 127.0.0.1:2019:2019 \
          caddy:2-alpine || log_warn "Caddy container launch had warnings"
        log_ok "liteploy-caddy container initialized"
    else
        log_ok "liteploy-caddy container running"
    fi
fi

# 8. Systemd Service Setup
SERVICE_FILE="/etc/systemd/system/liteploy.service"

if command -v systemctl >/dev/null 2>&1; then
    log_info "Installing systemd service..."

    cat <<EOF > "${SERVICE_FILE}"
[Unit]
Description=LITEPLOY - Lightweight Docker Deployment Platform
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/var/lib/liteploy
ExecStart=${TARGET_BIN}
Restart=always
RestartSec=5s
EnvironmentFile=${ENV_FILE}
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable liteploy.service >/dev/null 2>&1
    log_info "Starting Liteploy service..."
    systemctl restart liteploy.service

    # 9. Strict Service Health Check
    log_info "Verifying service health and subsystems..."
    HEALTHY=false
    for i in $(seq 1 20); do
        sleep 1
        
        # 1. Check systemd active status
        if ! systemctl is-active --quiet liteploy; then
            continue
        fi

        # 2. Check Liteploy HTTP response code (/health or /)
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/health 2>/dev/null || curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/ 2>/dev/null || echo "000")
        if [ "$HTTP_CODE" = "000" ] || [ "$HTTP_CODE" -ge 500 ]; then
            continue
        fi

        # 3. Check Caddy Admin API
        CADDY_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:2019/config/ 2>/dev/null || echo "000")
        if [ "$CADDY_CODE" != "200" ]; then
            continue
        fi

        # 4. Check Docker network
        if ! docker network inspect liteploy-network >/dev/null 2>&1; then
            continue
        fi

        # 5. Check Caddy container running
        if ! docker ps --format '{{.Names}}' | grep -q '^liteploy-caddy$'; then
            continue
        fi

        HEALTHY=true
        break
    done

    if [ "$HEALTHY" = false ]; then
        echo ""
        echo -e "${RED}[ERROR] LITEPLOY health verification failed.${NC}"
        echo ""
        echo -e "${YELLOW}--- systemctl status liteploy ---${NC}"
        systemctl status liteploy --no-pager || true
        echo ""
        echo -e "${YELLOW}--- journalctl -u liteploy -n 50 ---${NC}"
        journalctl -u liteploy -n 50 --no-pager || true
        echo ""
        echo -e "${YELLOW}--- docker ps ---${NC}"
        docker ps || true
        echo ""
        exit 1
    fi

    log_ok "Liteploy HTTP service is healthy (:8080)"
    log_ok "Caddy reverse proxy container is healthy (:2019, :80, :443)"
    log_ok "Shared Docker network (liteploy-network) is active"
else
    log_warn "systemd not detected. Please run LITEPLOY manually using: ${TARGET_BIN}"
fi

# 10. Print Installation Summary
IP_ADDR="$(curl -s --max-time 3 https://api.ipify.org 2>/dev/null || curl -s --max-time 3 https://icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')"

echo ""
echo -e "=================================================="
echo -e "${GREEN}🚀 LITEPLOY Installation Complete!${NC}"
echo -e "=================================================="
echo -e "Dashboard URL: ${BLUE}http://${IP_ADDR}:8080${NC}"
echo -e "Environment:   ${ENV_FILE} (chmod 600)"
echo -e "Data Storage:  ${DATA_DIR}"
echo -e "Service Logs:  sudo journalctl -u liteploy -f"
echo -e "=================================================="
echo ""

