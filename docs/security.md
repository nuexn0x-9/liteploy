# Security Architecture & Hardening

LITEPLOY incorporates strict security principles across its entire codebase and runtime environment.

---

## 🛡️ Core Security Controls

- **Cryptographic Session Tokens:** HMAC-SHA256 signed session cookies backed by a high-entropy 32-byte secret generated via `openssl rand -hex 32` or `/dev/urandom`.
- **Environment Isolation:** Secrets and environment variables are stored in `/etc/liteploy/liteploy.env` with strict `chmod 600` file permissions accessible only by `root`.
- **Domain & Input Sanitization:** Regex-based domain validation prevents host header injection, path traversal, and command injection attacks.
- **CSRF Protection:** Synchronizer token validation across all state-changing `POST` and `DELETE` actions.
- **Bcrypt Password Hashing:** User access codes are hashed using Bcrypt with a high work factor (cost 12).
- **Command Injection Prevention:** All system execution uses structured argument slices (`exec.CommandContext`) without passing raw strings to a shell interpreter.
- **Secret Redaction in Logs:** `maskWriter` scrubs Git Personal Access Tokens (PAT), passwords, and sensitive URLs from build logs and SSE streams.
- **Caddy Admin API Protection:** The Caddy Admin API (`:2019`) is bound strictly to `127.0.0.1` and never exposed externally.

---

## 🔒 Recommended VPS Hardening

```bash
# Configure UFW Firewall
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 80/tcp   # HTTP (for Let's Encrypt challenge & redirect)
sudo ufw allow 443/tcp  # HTTPS (for secure dashboard and app traffic)
sudo ufw allow 8080/tcp # (Optional: direct dashboard access before domain is configured)
sudo ufw enable
```
