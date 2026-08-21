# Changelog

All notable changes to the LITEPLOY project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Comprehensive open-source documentation suite in `docs/`.
- Automated Linux installer script (`scripts/install.sh`).
- Systemd service integration template (`scripts/liteploy.service`).
- GitHub Actions CI (`ci.yml`) and Release (`release.yml`) workflows.
- `version` and `help` CLI flags (`liteploy version`, `liteploy help`).
- Security policy (`SECURITY.md`), contribution guide (`CONTRIBUTING.md`), and issue templates.

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
