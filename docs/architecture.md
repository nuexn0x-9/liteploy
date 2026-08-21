# Technical Architecture Overview

LITEPLOY is designed as a single-binary, self-hosted deployment platform engineered specifically for resource-constrained Virtual Private Servers (VPS).

---

## 🏗️ Core Architectural Stack

```
LITEPLOY
├── HTTP / Web UI (net/http + html/template + HTMX)
├── Deployment Engine (State machine + bounded worker queue)
├── Docker Integration (Moby Engine SDK via UNIX socket)
├── Proxy Integration (Caddy Admin API via http://localhost:2019)
├── Storage Layer (Atomic JSON filesystem state)
├── Auth & Security (HMAC session cookies + CSRF + Bcrypt)
└── Log Engine (SSE streaming + disk tailing)
```

---

## 💡 What LITEPLOY Deliberately Omits

To maintain an idle memory footprint of **~18.5 MB RAM**:

- **No PostgreSQL / MySQL:** Replaced by atomic JSON file storage.
- **No Redis / RabbitMQ:** Replaced by bounded in-process Go channels (`chan *job`).
- **No Heavy SPA Bundles:** Replaced by server-rendered HTML templates + HTMX.
- **No External CLI Executables:** Direct API communication with Docker Engine SDK and Caddy Admin API.
