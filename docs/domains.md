# Custom Domains & Reverse Proxy Routing

LITEPLOY integrates seamlessly with **Caddy Reverse Proxy** to handle domain routing and automatic HTTPS.

---

## 🌐 Adding a Domain

1. On your DNS provider (Cloudflare, Namecheap, GoDaddy, Route53), point your domain's `A` or `AAAA` record to your VPS IP address.
2. In LITEPLOY, open the application detail page.
3. Under **🌐 Custom Domains & SSL**, enter your domain name (e.g. `app.yourdomain.com`).
4. Click **+ Add Domain**.

---

## 🔍 DNS Resolution Diagnostic

Click **🔍 Test DNS** next to any domain to perform a live host lookup:
- **Green (`✓ Resolves to: 1.2.3.4`):** DNS is properly configured and pointing to your server.
- **Red (`✗ DNS not resolved yet`):** DNS propagation is pending or pointing to the wrong IP.

---

## 🔄 Caddy Admin API Integration

LITEPLOY updates Caddy dynamically via `POST http://localhost:2019/load`. Zero manual Caddyfile reloads required.
