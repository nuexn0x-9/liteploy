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

## 🚀 4. Mapping Domains to Applications

To route incoming traffic to a specific containerized app:
1. Open the application detail page (e.g. `qulineria-api`).
2. Under **🌐 Domains & Network**, enter your target hostname:
   - If using wildcard DNS: enter `api.example.com` (ready instantly).
   - If using a separate custom domain: enter `custom-client.org` (ensure `A` record points to your VPS IP).
3. Click **+ Add Domain**.
4. LITEPLOY immediately syncs routing tables to Caddy via `POST http://127.0.0.1:2019/load`.
5. Caddy routes traffic directly to the container alias inside `liteploy-network` (e.g. `liteploy-app-001:8000`).

---

## 🔍 5. DNS Resolution Diagnostics

Click **◎ CHECK DNS** on the **NETWORK** page or application details to test live DNS:
- **Green (`✓ 103.197.191.34`):** DNS is properly resolving to the server.
- **Red (`✗ DNS not resolved`):** DNS propagation is pending or pointing to another IP.

---

## 🛡️ 6. Safety & Rollback Model

Before applying any route changes to Caddy:
1. LITEPLOY caches the current valid configuration.
2. If Caddy returns an error during `/load`, LITEPLOY automatically rolls back to the last known good configuration without dropping active connections.
