# Configuration Reference

LITEPLOY is configured cleanly via environment variables stored in `/etc/liteploy/liteploy.env` and persistent state files.

---

## ⚙️ Environment Variables Reference

Environment variables can be set in `/etc/liteploy/liteploy.env` (`chmod 600`):

| Variable | Default Value | Description |
|---|---|---|
| `LITEPLOY_ADDR` | `:8080` | Internal network address and port LITEPLOY HTTP server listens on. |
| `LITEPLOY_DATA_DIR` | `/var/lib/liteploy/data` | Filesystem path for application & deployment atomic JSON state. |
| `LITEPLOY_CADDY_ADMIN` | `http://localhost:2019` | Caddy Admin API endpoint URL for reverse proxy updates. |
| `LITEPLOY_DOCKER_HOST` | *(unix socket)* | Docker daemon socket address (e.g. `unix:///var/run/docker.sock`). |
| `LITEPLOY_SESSION_SECRET` | *(cryptographic 32+ bytes)* | HMAC signing key for secure session cookies. |
| `LITEPLOY_SESSION_MAX_AGE` | `24h` | Session duration before requiring operator re-authentication. |
| `LITEPLOY_LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`). |
| `LITEPLOY_LOG_JSON` | `true` (in prod) | Enable structured JSON logging format. |
| `LITEPLOY_MAX_DEPLOYMENTS` | `1` | Maximum concurrent deployment builds allowed (bounded for 1 GB VPS). |
| `LITEPLOY_GIT_TIMEOUT` | `10m` | Timeout for git clone and fetch operations. |
| `LITEPLOY_DEPLOYMENT_TIMEOUT` | `30m` | Maximum time allowed for Docker build and run execution. |
| `LITEPLOY_HEALTH_CHECK_TIMEOUT`| `2m` | Maximum wait duration for zero-downtime container readiness. |
| `LITEPLOY_DEV_MODE` | `false` | Enable developer mode. |

---

## 📁 Filesystem Layout under `LITEPLOY_DATA_DIR`

All application metadata, settings, and secrets are persisted without external database dependencies:

```
/var/lib/liteploy/data/
├── applications/        # JSON application metadata (app-001.json, etc.)
├── deployments/         # JSON deployment history (dep-0001.json, etc.)
├── repos/               # Persistent Git cache per app for fast incremental builds
├── config/              # Admin users (users.json) & System settings (settings.json)
└── logs/                # Realtime deployment logs (dep-0001.log, etc.)
```
