# Automatic HTTPS & SSL Certificates

LITEPLOY utilizes a containerized **Caddy Reverse Proxy (`liteploy-caddy`)** to provision and renew TLS/SSL certificates automatically via Let's Encrypt and ZeroSSL.

---

## 🔒 How Automated HTTPS Works in LITEPLOY

1. **Dashboard Routing (`liteploy.<primary_domain>`):**
   - When Primary Domain is configured, LITEPLOY registers `liteploy.<primary_domain>` in Caddy's HTTP server configuration listening on ports `:80` and `:443`.
   - Caddy automatically completes the ACME HTTP-01 or TLS-ALPN-01 challenge.
   - All HTTP traffic is automatically redirected to secure HTTPS (`https://liteploy.<primary_domain>`).

2. **Application Subdomains & Custom Domains:**
   - Every domain mapped to an application (e.g. `app.example.com` or `custom-domain.com`) is dynamically registered in Caddy.
   - Caddy requests certificates on-demand or upon first request, ensuring zero maintenance and automatic 90-day renewals.
   - Certificate data is stored persistently on the host at `/var/lib/liteploy/data/caddy`, surviving container restarts.

---

## ☁️ Cloudflare Integration & SSL/TLS Configuration

If your domain is managed through **Cloudflare**:

| Cloudflare SSL/TLS Mode | Status | Notes |
|---|---|---|
| **Full (Recommended)** | ✅ Working | Encrypts end-to-end between visitor, Cloudflare, and Caddy. |
| **Full (Strict)** | ✅ Working | Requires active certificate on VPS (Caddy). |
| **Flexible** | ⚠️ Not Recommended | May cause 502 or redirect loops because Cloudflare calls port 80 while Caddy enforces HTTPS redirect. |

> **Pro Tip for Let's Encrypt Direct Issue:** If you want Let's Encrypt certificates directly issued to Caddy without Cloudflare proxy interference, set your DNS record to **DNS Only (Grey Cloud)**.

---

## 📋 VPS Firewall Requirements

Ensure incoming traffic on ports `80` (HTTP) and `443` (HTTPS) is allowed on your VPS firewall:

```bash
# Ubuntu / Debian (UFW)
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw reload

# CentOS / RHEL / Rocky Linux (Firewalld)
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

---

## 🔄 Automatic Rollback Protection

If a bad route or domain configuration causes Caddy to reject a configuration update:
1. LITEPLOY intercepts the non-200 HTTP response from `http://127.0.0.1:2019/load`.
2. The proxy manager automatically rolls back to the `lastKnownGood` configuration.
3. Active TLS connections and valid application routes remain uninterrupted.
