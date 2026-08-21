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
├── Settings & Domain Service (Primary domain, wildcard routing, DNS lookup)
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

---

## 🌐 Reverse Proxy & Domain Routing Flow

```
Incoming Request (e.g. https://liteploy.example.com or https://app.example.com)
                            │
                            ▼
           Caddy Reverse Proxy (:80, :443)
           (Automatic TLS via Let's Encrypt)
                            │
            ┌───────────────┴───────────────┐
            ▼                               ▼
    Dashboard Route                  Application Route
  (127.0.0.1:8080)             (liteploy-app-xxx:PORT)
            │                               │
            ▼                               ▼
     LITEPLOY Binary               Docker Application Container
```

---

## 🐳 Internal Docker Networking

LITEPLOY places all managed containers inside a dedicated, isolated Docker bridge network (`liteploy-net`).
Containers are named based on their Application ID (e.g. `liteploy-app-001-0001`).

This architecture enables **Multi-Tier Application Communication** without exposing internal ports to the public internet:
- Backend connects to Database using `postgres://user:pass@liteploy-db-app:5432`
- Frontend connects to Backend internally using `http://liteploy-backend-app:8000`