# Automatic HTTPS & SSL Certificates

LITEPLOY relies on Caddy Reverse Proxy to automatically manage TLS certificates via Let's Encrypt and ZeroSSL.

---

## 🔒 How HTTPS Works

1. When a custom domain (e.g. `app.yourdomain.com`) is added to an application, LITEPLOY registers the host routing rule with Caddy.
2. When the first HTTPS request arrives, Caddy performs the ACME HTTP-01 challenge automatically.
3. TLS certificates are provisioned and renewed in the background without downtime.

---

## 📋 Firewall Requirements for SSL

Ensure ports `80` (HTTP) and `443` (HTTPS) are open on your VPS firewall:

```bash
# Ubuntu UFW
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw reload
```
