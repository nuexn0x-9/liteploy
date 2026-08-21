# LITEPLOY Documentation Index

Welcome to the official LITEPLOY documentation! This manual is structured for both beginner VPS owners and experienced developers.

---

## 🚀 Getting Started

- **[Installation Guide](installation.md)** — Step-by-step VPS installation instructions using the one-command installer or manual binaries.
- **[Quick Start Walkthrough](quick-start.md)** — Step-by-step tutorial from clean VPS to your first deployed application with custom domains and HTTPS.
- **[System & VPS Requirements](requirements.md)** — RAM, CPU, disk, and OS compatibility matrix for $5/month VPS hosting.

---

## ⚙️ Configuration & Operations

- **[Configuration Reference](configuration.md)** — Environment variables, server listening ports, and startup flags.
- **[Updates & Upgrades](updates.md)** — Updating LITEPLOY to new releases safely.
- **[Backup & Recovery](backup.md)** — Backing up filesystem state and restoring from snapshots.
- **[Troubleshooting Guide](troubleshooting.md)** — Diagnosing common startup, Docker, domain, and deployment issues.
- **[Frequently Asked Questions (FAQ)](faq.md)** — Answers to common questions about LITEPLOY.

---

## 📦 Deploying Workloads

- **[Application Management](applications.md)** — Creating, editing, and deleting applications.
- **[Git & Private Repositories](git-deployment.md)** — Deploying from GitHub, GitLab, or Gitea using PAT tokens or SSH Deploy Keys.
- **[Docker Image Registries](docker-images.md)** — Deploying pre-built images from Docker Hub, GHCR, or private registries.
- **[Environment Variables & Secrets](environment-variables.md)** — Injecting `.env` key-value pairs and raw env files with secret masking.
- **[Custom Domains & HTTPS](domains.md)** — Mapping custom domains with automated Caddy SSL certificates.
- **[Realtime Logs & SSE](logs.md)** — Tailing live build logs and container runtime outputs.
- **[Automated Webhooks](webhooks.md)** — Setting up automated deployments on `git push`.
- **[Container Lifecycle](lifecycle.md)** — Starting, stopping, restarting, and container recovery.

---

## 🔬 Architecture & Deep Dives

- **[Technical Architecture](architecture.md)** — Inside LITEPLOY: Go engine, atomic storage, Docker Engine API, and Caddy integration.
- **[Filesystem State Storage](storage.md)** — Zero-database design using atomic JSON writes.
- **[Security Architecture](security.md)** — Session security, CSRF protection, secret scrubbing, and VPS hardening.
