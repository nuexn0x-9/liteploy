# Security Architecture & Hardening

LITEPLOY incorporates strict security measures across its full stack.

---

## 🛡️ Core Security Controls

- **Authentication:** HMAC-SHA256 signed session cookies, Bcrypt password hashing (cost 12).
- **CSRF Protection:** Form validation (`_csrf`) & HTMX request headers (`X-CSRF-Token`).
- **Command Injection Prevention:** Zero raw string shell execution (`exec.CommandContext` with separate argument slices).
- **Secret Redaction:** `maskWriter` scrubs Git PAT tokens and credentials from build logs.
- **Docker Privilege:** Containers do not run in privileged mode by default.

---

## 🔒 Recommended VPS Hardening

1. **Firewall (UFW):**
   ```bash
   sudo ufw default deny incoming
   sudo ufw default allow outgoing
   sudo ufw allow 22/tcp   # SSH
   sudo ufw allow 80/tcp   # HTTP
   sudo ufw allow 443/tcp  # HTTPS
   sudo ufw allow 8080/tcp # LITEPLOY Dashboard
   sudo ufw enable
   ```
2. **Reverse Proxy Dashboard Protection:**
   Route LITEPLOY dashboard behind Caddy with custom domain (e.g. `panel.yourdomain.com`).
