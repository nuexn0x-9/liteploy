# Installation Manual

This manual explains how to install LITEPLOY on a fresh Linux VPS.

---

## 📋 System Requirements

| Resource | Minimum Requirement | Recommended |
|---|---|---|
| **OS** | Linux (Ubuntu 20.04+, Debian 11+, Rocky 9+, Alpine 3.18+) | Ubuntu 22.04 / 24.04 LTS |
| **CPU** | 1 Core (amd64 or arm64) | 1–2 Cores |
| **RAM** | 512 MB | 1 GB or higher |
| **Disk** | 5 GB SSD | 20 GB SSD |
| **Privileges** | Root / `sudo` access | Root / `sudo` access |

---

## ⚡ Method 1: Automatic Installer (Recommended)

Run the official one-command installer on your server:

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
```

### What the installer does:
1. Detects CPU architecture (`amd64` or `arm64`).
2. Validates Docker and Git dependencies.
3. Automatically provisions `liteploy-network` and launches the `liteploy-caddy` Docker container.
4. Downloads the official LITEPLOY binary to `/usr/local/bin/liteploy` (with automatic source build fallback).
5. Creates storage directory `/var/lib/liteploy/data` and configuration directory `/etc/liteploy` (`chmod 700`).
6. Generates a 32-byte cryptographic session secret and writes `/etc/liteploy/liteploy.env` (`chmod 600`) idempotently without overwriting existing secrets.
7. Installs `/etc/systemd/system/liteploy.service` configured with `EnvironmentFile=/etc/liteploy/liteploy.env`.
8. Starts the service and executes strict health-check polling before confirming installation.

---

## 🛠️ Method 2: Manual Installation

If you prefer installing manually without the installer script:

1. **Download Binary:**
   ```bash
   # For x86_64 / amd64 Linux:
   sudo curl -fsSL https://github.com/nuexn0x-9/liteploy/releases/latest/download/liteploy-linux-amd64 -o /usr/local/bin/liteploy
   sudo chmod +x /usr/local/bin/liteploy
   ```

2. **Create Directories & Environment File:**
   ```bash
   sudo mkdir -p /var/lib/liteploy/data
   sudo mkdir -p /etc/liteploy
   sudo chmod 700 /etc/liteploy

   # Generate 32-byte session secret
   SESSION_SECRET=$(openssl rand -hex 32)

   cat <<EOF | sudo tee /etc/liteploy/liteploy.env > /dev/null
   LITEPLOY_ADDR=:8080
   LITEPLOY_DATA_DIR=/var/lib/liteploy/data
   LITEPLOY_CADDY_ADMIN=http://127.0.0.1:2019
   LITEPLOY_SESSION_SECRET=${SESSION_SECRET}
   LITEPLOY_LOG_LEVEL=info
   LITEPLOY_LOG_JSON=true
   EOF

   sudo chmod 600 /etc/liteploy/liteploy.env
   ```

3. **Install Systemd Unit:**
   ```bash
   cat <<EOF | sudo tee /etc/systemd/system/liteploy.service > /dev/null
   [Unit]
   Description=LITEPLOY - Lightweight Docker Deployment Platform
   After=network.target docker.service
   Requires=docker.service

   [Service]
   Type=simple
   User=root
   Group=root
   WorkingDirectory=/var/lib/liteploy
   ExecStart=/usr/local/bin/liteploy
   Restart=always
   RestartSec=5s
   EnvironmentFile=/etc/liteploy/liteploy.env
   LimitNOFILE=65536

   [Install]
   WantedBy=multi-user.target
   EOF

   sudo systemctl daemon-reload
   sudo systemctl enable --now liteploy
   ```

---

## 🔍 Verifying Installation

1. Verify that the LITEPLOY service is active:
   ```bash
   sudo systemctl status liteploy
   ```

2. Verify local HTTP readiness:
   ```bash
   curl -I http://127.0.0.1:8080/
   ```

3. Open your browser and navigate to `http://<your-vps-ip>:8080` to launch the **Initial Setup Wizard**!
