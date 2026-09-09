# Changelog

All notable changes to the LITEPLOY project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v1.2.0] - 2026-09-09

### Added
- **Monorepo Architecture Support:** Native support for `ServicePath`, `BuildContext`, and `DockerfilePath` configuration per application, allowing multiple frontend/backend/worker services to build from a single Git repository.
- **Monorepo Security & Symlink Protection:** Strict subpath validation blocking directory traversal (`..`, absolute paths) and real-path symlink evaluation (`filepath.EvalSymlinks`) ensuring builds cannot escape repository boundaries.
- **Microservices & Project-Level Network Isolation:** Project abstraction (`liteploy-project-{project_id}`) providing private Docker bridge networks. Services within the same project communicate privately via internal DNS aliases.
- **Microservice DNS Naming:** Canonical container alias `liteploy-app-{id}`, compatibility alias `liteploy-{id}`, and friendly DNS alias `strings.ToLower(app.Name)` unique within each project.
- **Dynamic Multi-Network Caddy Routing:** Caddy reverse proxy dynamically attaches to project bridge networks when exposing public `web` or `api` services.
- **Service Type Rules Matrix:** Defined 4 distinct service types (`web`, `api`, `worker`, `internal`). Public domain routing is strictly prohibited on `worker` and `internal` services.
- **Zero-Leak SSE Log Streaming:** Replaced unbounded goroutine/pipe SSE streaming with seek-based chunk reading (`ReadBuildLogChunk`), eliminating memory and file-descriptor leaks during live build log observation on low-spec VPS.
- **Retro Gaming CRT / Pixel UI Redesign:** Unified design across all pages (`retro-window`, `topbar`, `sidebar`, `retro-content-canvas`) with an accessible soft retro palette, live CRT scanline effects, dynamic server stats, and mobile sidebar navigation.
- **Dynamic Resource Telemetry:** Added live container count and resource monitoring percentages to the dashboard and top bar.
- **Automated Hardening Test Suite:** Added comprehensive hardening tests covering monorepo traversal validation, service type enforcement, project friendly name uniqueness, microservice network isolation, concurrency mutual exclusion, and zero-leak build log streaming.

---

## [v1.1.0] - 2026-08-24

### Added
- **One-Domain Multi-Path Routing:** Single-domain architecture (`qulineria.my.id` → frontend, `qulineria.my.id/api/*` → backend, `qulineria.my.id/assets/*` → backend static assets) without CORS or multiple SSL certificates.
- **Strict Caddy JSON Route Ordering:** Generator automatically sorts specific path routes (`/api/*`, `/assets/*`) longest-first before catch-all routes (`/*`) in Caddy JSON configuration to eliminate 404 proxy leakage.
- **Next.js / Frontend Build-Time Environment Injection:** Automatically writes `.env` / `.env.production` in build context and passes `BuildArgs` during `docker build` so `NEXT_PUBLIC_*` variables (e.g. `NEXT_PUBLIC_API_URL=/api`) are baked into compiled JavaScript bundles at build-time.
- **Native Management CLI:** Headless commands `liteploy status [app]`, `liteploy deploy <app>`, `liteploy logs <app>`, `liteploy version`, and `liteploy help`.
- **Zero-Downtime Deployment Guarantee:** Containers must pass health checks and proxy configuration must be verified before stopping previous containers.
- **Route Validation & Safety:** Automatic duplicate `(host, path)` detection across different applications and enforcement of internal Docker DNS (`liteploy-app-xxx:PORT`), rejecting loopback addresses (`localhost`/`127.0.0.1`).
- **Comprehensive Automated Test Suite:** 18 end-to-end and integration test scenarios in `tests/routing_e2e_test.go` covering real routing, Caddy Admin API inspection, and regression tests.
- **Audited Production Installer:** Multi-stage health checks verifying systemd service, HTTP readiness (:8080/health), Caddy Admin API (:2019), and Docker network integrity before printing success.

---

## [v1.0.0] - 2026-08-21


### Added
- **Containerized Caddy Reverse Proxy:** Fully migrated Caddy from host systemd service to an isolated Docker container (`liteploy-caddy` on `liteploy-network`), completely eliminating 502 DNS resolution errors between host and internal container hostnames.
- **Direct Docker DNS Reverse Proxy Routing:** Caddy dials application backends directly via internal Docker DNS (`liteploy-app-xxx:PORT`), eliminating the need to expose dynamic host ports.
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
