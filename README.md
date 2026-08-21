# ⚡ LITEPLOY

> **Lightweight Docker Deployment Platform for Small VPS**  
> Deploy, run, and manage Docker workloads on $5/mo servers (1 GB RAM target) with zero database overhead.

[![CI](https://github.com/nuexn0x-9/liteploy/actions/workflows/ci.yml/badge.svg)](https://github.com/nuexn0x-9/liteploy/actions)
[![Go Version](https://img.shields.io/github/go-mod/go-version/nuexn0x-9/liteploy)](https://github.com/nuexn0x-9/liteploy)
[![License](https://img.shields.io/badge/license-Pending--Owner--Selection-blue.svg)](#license)
[![RAM Idle](https://img.shields.io/badge/idle_RAM-~18.5_MB-success.svg)](#resource-philosophy)

---

## 🎯 Overview

**LITEPLOY** is a self-hosted, single-binary application deployment platform engineered specifically for resource-constrained Virtual Private Servers (VPS). 

If you want the convenience of modern PaaS platforms (like Vercel or Render) on your own $5/month VPS (1 CPU, 1 GB RAM) without sacrificing 400+ MB of RAM to heavy management panels, LITEPLOY is built for you.

---

## 🤔 Why LITEPLOY Exists

Traditional self-hosted deployment panels often ship with PostgreSQL, Redis, background task queues, complex microservice architectures, and heavy frontend SPA bundles. On a 1 GB RAM VPS, these panels can consume 40%–60% of host memory before you even deploy your first application!

**LITEPLOY solves this problem with a lightweight architecture:**

- **Zero Database:** Application state and deployment history are persisted atomically directly on the filesystem as structured JSON.
- **Zero Redis / Queue Infrastructure:** Bounded in-process worker pool and job queues guard against memory spikes.
- **Embedded Frontend:** Server-rendered HTML templates + HTMX compiled directly into a single ~17 MB binary.
- **Docker & Caddy Integration:** Direct communication with the local Docker Engine API and Caddy Reverse Proxy Admin API for automatic HTTPS.

---

## 🏗️ Architecture Overview

```
                        +----------------------------+
                        |   Browser / Git Webhooks   |
                        +----------------------------+
                                      |
                                      v
                        +----------------------------+
                        |     LITEPLOY Binary        |
                        | (net/http + HTMX + Slog)   |
                        +----------------------------+
                          /           |            \
                         /            |             \
                        v             v              v
            +---------------+  +------------+  +------------------+
            | Filesystem    |  | Docker     |  | Caddy Proxy      |
            | Atomic State  |  | Engine API |  | Admin API (:2019)|
            +---------------+  +------------+  +------------------+
                                      |                 |
                                      v                 v
                               +----------------------------------+
                               | Isolated Application Containers  |
                               +----------------------------------+
```

## 🚀 Key Features

- **⚡ Lightweight Footprint:** Observed idle memory footprint of **~18.5 MB RSS** in internal testing.
- **🎨 Retro 8-bit Console UI:** A fast, responsive, game-inspired HTMX control panel with mobile sidebar support.
- **📦 Zero-Downtime HTTP Healthchecks:** Validates container HTTP endpoints before switching traffic.
- **♻️ 1-Click Rollbacks:** Instantly revert to a previous successful image deployment in seconds.
- **📈 Live Container Metrics:** Real-time CPU and RAM monitoring straight from Docker Stats API.
- **🧹 System Auto-Prune:** Built-in garbage collection to keep your $5 VPS disk clean.
- **🗄️ Persistent Volumes:** Map host directories to containers to ensure database/state survival.
- **🔄 Dual Workload Sources:** Deploy directly from Git repositories or Docker image registries.
- **🔑 Private Repository Authentication:** Support for Personal Access Tokens (PAT) and SSH Private Keys.
- **🌐 Domain Management & Automatic HTTPS:** Easily map custom domains. Caddy automatically issues Let's Encrypt / ZeroSSL TLS certificates.
- **🔒 Secret & Environment Manager:** Interactive `.env` key-value editor with secret masking.
- **📡 Realtime Log Streaming:** Live SSE build log tailing and runtime stdout/stderr streaming.
- **🔗 Automated Webhooks:** Automatic deployment triggers on `git push` with HMAC-SHA256 signatures.
- **🛡️ Built-in Security:** HMAC-SHA256 cookie session auth, CSRF token validation, bcrypt password hashing.
- **🔧 Automatic Startup Recovery:** Reconciles container states and proxy routes automatically on VPS reboot.

---

## 💾 Resource Philosophy

LITEPLOY is designed with strict resource constraints:

- **Observed Idle RSS:** ~18.5 MB RAM *(measured in internal baseline testing)*
- **Target Active Peak:** < 50 MB RAM
- **Build Concurrency:** Bounded worker queue (default 1 concurrent build) to prevent RAM exhaustion.

> **Note:** While LITEPLOY itself consumes minimal RAM, application workloads (e.g. Node.js apps, Docker build steps, database containers) consume host memory independently. Always size container RAM limits accordingly.

---

## 🌍 Supported Environments

- **Operating Systems:** Linux (Ubuntu 20.04/22.04/24.04, Debian 11/12, CentOS/Rocky Linux 9, Alpine Linux 3.18+)
- **Architectures:** `amd64` (x86_64), `arm64` (aarch64)
- **Prerequisites:** Docker Engine 20.10+, Caddy 2.7+ (optional but recommended for automatic HTTPS)

---

## ⚡ Quick Installation

Run the official one-command installer on a clean Linux VPS (requires root or `sudo` privileges):

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | bash
```

The installer automatically:
1. Detects your CPU architecture (`amd64` or `arm64`).
2. Verifies Docker availability.
3. Installs the compiled binary to `/usr/local/bin/liteploy`.
4. Creates and starts the Systemd service (`liteploy.service`).

---

## 🚀 Quick Start Guide

1. **Access the Dashboard:** Open `http://<your-vps-ip>:8080` in your browser.
2. **Initial Setup:** On first launch, create your administrator credentials.
3. **Create Application:** Click **+ New Application**, enter a name, and select workload source.
4. **Configure & Deploy:** Set container port (e.g. `3000`), map Persistent Volumes, set a Healthcheck Path, and click **🚀 Deploy Now**.
5. **Add Custom Domain:** Under the application's **Domains** card, add `app.yourdomain.com` to enable automatic HTTPS routing.

---

## 📚 Complete Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory:

- [Installation Guide](docs/installation.md)
- [Quick Start Walkthrough](docs/quick-start.md)
- [Tutorial: Deploying a Multi-Tier App (Frontend + Backend + DB)](docs/tutorials/multi-tier-deployment.md)
- [System & VPS Requirements](docs/requirements.md)
- [Configuration Reference](docs/configuration.md)
- [Managing Applications](docs/applications.md)
- [Git & Private Repositories](docs/git-deployment.md)
- [Docker Image Registries](docs/docker-images.md)
- [Persistent Volumes](docs/features/volumes.md)
- [Environment Variables & Secrets](docs/environment-variables.md)
- [Custom Domains & HTTPS](docs/domains.md)
- [Realtime Logs & SSE](docs/logs.md)
- [Automated Webhooks](docs/webhooks.md)
- [State Persistence & Storage](docs/storage.md)
- [Updates & Maintenance](docs/updates.md)
- [Disaster Recovery & Backup](docs/backup.md)
- [Troubleshooting Guide](docs/troubleshooting.md)
- [Technical Architecture](docs/architecture.md)
- [Frequently Asked Questions (FAQ)](docs/faq.md)

---

## 🔄 Updating LITEPLOY

To update LITEPLOY to the latest version, re-run the installer script:

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | bash
```

Or restart the Systemd service manually after replacing the binary:

```bash
sudo systemctl restart liteploy
```

---

## 🛠️ Management Commands

```bash
# Check service status
sudo systemctl status liteploy

# View live system logs
sudo journalctl -u liteploy -f

# Restart LITEPLOY
sudo systemctl restart liteploy

# Stop LITEPLOY
sudo systemctl stop liteploy
```

---

## 🔒 Security Policy

Security is a core priority for LITEPLOY. Please review our [SECURITY.md](SECURITY.md) for details on security architecture, Docker socket privileges, and instructions for reporting vulnerabilities.

---

## 🤝 Contributing

We welcome open-source contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on setting up your local environment, running tests, and submitting pull requests.

---

## 📄 License

*(Project license selection is pending repository owner approval. See [LICENSE](LICENSE) for details once updated.)*
