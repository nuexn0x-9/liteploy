# Custom Domains, Wildcards & Reverse Proxy Routing

LITEPLOY integrates natively with the containerized **Caddy Reverse Proxy Admin API** (`http://127.0.0.1:2019`) to deliver automated domain routing, wildcard subdomains, and zero-configuration TLS/HTTPS certificates.

---

## 🌐 1. Primary Domain & Wildcard Hosting Architecture

LITEPLOY introduces a **Primary Domain** concept that simplifies domain management across all workloads on your VPS.

When you configure a Primary Domain (e.g. `example.com`):
- The LITEPLOY control panel is automatically assigned to: `https://liteploy.example.com`
- Any application you create can use any subdomain (e.g. `app.example.com`, `api.example.com`, `auth.example.com`) with **zero DNS changes required**.

### DNS Setup Guide (Cloudflare, Namecheap, GoDaddy, Route53, etc.):

| Record Type | Host / Name | Target / Value | Purpose |
|---|---|---|---|
| `A` | `@` (or `example.com`) | `YOUR_VPS_IP` | Resolves root domain to your server |
| `A` | `*` (wildcard) | `YOUR_VPS_IP` | Resolves `liteploy.*` and all app subdomains |

---

## 🧙‍♂️ 2. Initial Setup Wizard

When you access LITEPLOY for the first time:
1. **Admin Credentials:** Create your operator username and access code.
2. **Domain Configuration:**
   - Enter your root domain (e.g. `example.com`).
   - Add the two `A` records shown above to your DNS registrar.
   - Click **[ VERIFY DNS & ENABLE HTTPS ]**.
   - LITEPLOY verifies DNS resolution across apex and `liteploy.<domain>`, provisions the Caddy route, and redirects you to the secure dashboard (`https://liteploy.example.com`).

*(You can also click **[ Skip for now ]** to manage via IP address and configure a domain later).*

---

## ⚙️ 3. Managing Domains via Settings

At any time, navigate to **SYS CONFIG (Settings)** -> **PRIMARY DOMAIN & WILDCARD DNS** to:
- **Change Primary Domain:** Update to a new domain name.
- **[ VERIFY DNS ]:** Perform live DNS query checks against your server IP.
- **[ ENABLE / DISABLE HTTPS ]:** Toggle Caddy HTTPS reverse proxy routing.

---

## 🚀 4. Mapping Domains & Path Routing to Applications

LITEPLOY supports multiple deployment and routing topologies:

### Topology A: Frontend Only
- Application domain: `domain.com`
- Port: `3000` (or `80`)
- Routes all incoming traffic `/*` to your frontend application.

### Topology B: Backend Only
- Application domain: `api.domain.com` (or `domain.com`)
- Port: `8000`
- Routes requests to your backend API.

### Topology C: Frontend + Backend on Separate Subdomains
- Frontend App domain: `domain.com` (port `3000`)
- Backend App domain: `api.domain.com` (port `8000`)
- Frontend environment variable: `NEXT_PUBLIC_API_URL=https://api.domain.com`

### Topology D: Frontend + Backend on the SAME Domain (One-Domain Routing)
Host your entire stack on a single domain (e.g. `qulineria.my.id`) without CORS issues or multiple certificates:
- **Backend App:**
  - Route 1: `qulineria.my.id/api/*` (Container Port: `8000`)
  - Route 2 (optional): `qulineria.my.id/assets/*` (Container Port: `8000` for uploaded files/static assets)
- **Frontend App (Next.js / Vue / React):**
  - Route: `qulineria.my.id` (or `qulineria.my.id/*`) (Container Port: `3000`)
- **Frontend environment variable:**
  ```env
  NEXT_PUBLIC_API_URL=/api
  ```
  The browser calls `https://qulineria.my.id/api/products`, which Caddy immediately routes to `backend:8000`. All other paths (`/`, `/about`, `/dashboard`) are routed to `frontend:3000`.

### 🔄 Automatic Route Ordering
Caddy evaluates routes sequentially. LITEPLOY automatically places specific path routes (`/api/*`, `/assets/*`) **before** catch-all routes (`/*`) in the generated Caddy JSON configuration, guaranteeing accurate routing without 404 proxy leakage.

---

## 🔍 5. DNS Resolution Diagnostics

Click **◎ CHECK DNS** on the **NETWORK** page or application details to test live DNS:
- **Green (`✓ 103.197.191.34`):** DNS is properly resolving to the server.
- **Red (`✗ DNS not resolved`):** DNS propagation is pending or pointing to another IP.

---

## 🛡️ 6. Safety & Rollback Model

Before applying any route changes to Caddy:
1. LITEPLOY validates for duplicate `(host, path)` conflicts across applications.
2. LITEPLOY verifies upstream addresses use internal Docker DNS (e.g. `liteploy-app-001:8000`).
3. LITEPLOY saves the current known-good configuration.
4. If Caddy returns an error during `/load`, LITEPLOY automatically rolls back to the last known good configuration without dropping active connections.

