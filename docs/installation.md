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
1. Detects your CPU architecture (`amd64` or `arm64`).
2. Checks for Docker Engine installation.
3. Downloads the official LITEPLOY binary to `/usr/local/bin/liteploy`.
4. Creates data directory `/var/lib/liteploy/data`.
5. Installs and starts the Systemd service (`liteploy.service`).

---

## 🛠️ Method 2: Manual Installation

If you prefer installing manually without the installer script:

1. **Download the Binary:**
   ```bash
   # For x86_64 / amd64 Linux:
   curl -fsSL https://github.com/nuexn0x-9/liteploy/releases/latest/download/liteploy-linux-amd64 -o /usr/local/bin/liteploy
   chmod +x /usr/local/bin/liteploy
   ```

2. **Create Data Directory:**
   ```bash
   sudo mkdir -p /var/lib/liteploy/data
   ```

3. **Install Systemd Unit:**
   ```bash
   sudo curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/liteploy.service -o /etc/systemd/system/liteploy.service
   sudo systemctl daemon-reload
   sudo systemctl enable --now liteploy
   ```

---

## 🔍 Verifying Installation

Verify that the LITEPLOY service is active and running:

```bash
sudo systemctl status liteploy
```

Expected output:
```
● liteploy.service - LITEPLOY — Lightweight Docker Deployment Platform
     Loaded: loaded (/etc/systemd/system/liteploy.service; enabled)
     Active: active (running) since Thu 2026-08-20 22:30:00 UTC
```

You can now open your browser and navigate to `http://<your-vps-ip>:8080` to complete initial setup!
