# Changelog

All notable changes to the LITEPLOY project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v1.0.0] - 2026-08-21

### Added
- **Primary Domain & Wildcard Subdomains:** Global primary domain configuration (`example.com`) with automatic dashboard routing (`liteploy.example.com`) and wildcard subdomain hosting (`app.example.com`, `api.example.com`).
- **Initial Setup Wizard:** Two-step guided setup for admin account creation and primary domain / wildcard DNS configuration with live DNS verification.
- **Automated Caddy TLS & Rollback:** Automated Caddy route generation for dashboard and subdomains with automatic HTTPS certificate provisioning and instant rollback on failure.
- **1-Click Backup & VPS Migration:** Complete export and import of application state, configs, and domains into compressed `.tar.gz` archives.
- **Fast Git Fetch Caching:** Incremental `git fetch` caching per application repository preserving Docker build cache layers.
- **Deployment Retention & Cleanup:** Automatic pruning of old deployment records (keeping latest 10 runs) and 1-click purge for failed deployment logs.
- **Hardened Installer & Environment:** Production installer writing environment variables to `/etc/liteploy/liteploy.env` (`chmod 600`), cryptographic 32-byte session secret generation, and strict health-check polling before confirming installation.
- **Live System Metrics:** Real-time container CPU and RAM resource monitoring via Docker Stats API.
- **Zero-Downtime Healthchecks & Rollbacks:** Configurable HTTP endpoint verification before traffic cutover and 1-click rollback to prior deployment states.
- **Persistent Volume Mounts:** Host-to-container volume mapping for database and state persistence across deployments.

---

## [v0.1.0] - 2026-08-20

### Added
- **Core Engine:** Single-binary HTTP server built with Go `net/http` and `log/slog`.
- **Filesystem Persistence:** Atomic JSON state storage without database dependencies.
- **Docker Integration:** Direct Moby SDK client integration for container building, running, stopping, restarting, and log tailing.
- **Proxy Management:** Atomic Caddy Admin API route generator and proxy loader.
- **Git Support:** Direct repo cloning via HTTPS (PAT) and SSH Deploy Keys with secret log masking (`maskWriter`).
- **Private Image Registries:** Docker Hub, GHCR, and custom private registry authentication.
- **Environment & Domain Management:** Interactive `.env` key-value editor, raw importer, domain manager, and live DNS resolution diagnostics.
- **Realtime Logging:** SSE (Server-Sent Events) live deployment build logs and container runtime output streaming.
- **Webhooks:** Automated deployment triggers for GitHub (HMAC-SHA256) and GitLab secret tokens.
- **Authentication:** HMAC signed session cookies, CSRF protection, and Bcrypt password hashing.
- **Startup Recovery:** Reconciler engine to recover container states upon server or host reboot.
