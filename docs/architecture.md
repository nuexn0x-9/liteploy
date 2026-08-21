# Technical Architecture Overview

LITEPLOY is designed as a single-binary, self-hosted deployment platform engineered specifically for resource-constrained Virtual Private Servers (VPS).

---

## 🏗️ Core Architectural Stack

```
LITEPLOY
├── HTTP / Web UI (net/http + html/template + HTMX)
├── Deployment Engine (State machine + bounded worker queue)
├── Docker Integration (Moby Engine SDK via UNIX socket)
├── Proxy Integration (Caddy Admin API via http://127.0.0.1:2019)
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
- **No Host-Level Caddy Daemon:** Replaced by a lightweight, managed `liteploy-caddy` Docker container (`caddy:2-alpine`).

---

## 🌐 Reverse Proxy & Domain Routing Flow

```
Incoming Request (e.g. https://liteploy.example.com or https://app.example.com)
                            │
                            ▼
        Docker: liteploy-caddy Container (:80, :443)
        (Automatic TLS via Let's Encrypt / ZeroSSL)
                            │
            ┌───────────────┴───────────────┐
            ▼                               ▼
    Dashboard Route                  Application Route
 (host.docker.internal:8080)    (liteploy-app-xxx:PORT)
            │                               │
            ▼                               ▼
     LITEPLOY Binary               Docker Application Container
      (Host Process)               (Inside liteploy-network)
```

---

## 🐳 Internal Docker Networking

LITEPLOY creates and manages a dedicated, isolated Docker bridge network (**`liteploy-network`**).
Both the reverse proxy container (`liteploy-caddy`) and all application workloads share this network.

Containers are given stable DNS aliases based on their Application ID (e.g., `liteploy-app-001`, `liteploy-app-002`):
- **Caddy Reverse Proxy** dials application backends directly via Docker DNS: `http://liteploy-app-002:3000`.
- Application containers **do not need to expose dynamic host ports** (e.g. 32768+) to the VPS host, improving security and reducing host port clutter.
- **Multi-Tier Application Communication:**
  - Backend connects to Database using `postgres://user:pass@liteploy-db-app:5432`
  - Frontend connects to Backend internally using `http://liteploy-backend-app:8000`