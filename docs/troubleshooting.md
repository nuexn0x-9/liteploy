# Troubleshooting Guide

This manual covers diagnostic steps for common operational issues.

---

## 1. LITEPLOY Service Fails to Start

- **Symptom:** `systemctl status liteploy` shows `failed` or `exited`.
- **Cause:** Port conflict, invalid permissions, or missing data directory.
- **Diagnosis:** Run `journalctl -u liteploy -n 50 --no-pager` to view startup logs.
- **Solution:**
  - Verify port `8080` is not in use (`netstat -tulpn | grep 8080`).
  - Verify `/var/lib/liteploy/data` exists and is writable.

---

## 2. Docker Daemon Not Reachable

- **Symptom:** Logs show `Docker daemon not reachable`.
- **Cause:** Docker service is stopped or UNIX socket permission issue.
- **Diagnosis:** Run `docker ps` on the command line.
- **Solution:**
  - Start Docker: `sudo systemctl start docker`.
  - Ensure LITEPLOY runs as root or has access to `/var/run/docker.sock`.

---

## 3. Git Clone Fails (`Authentication Failed`)

- **Symptom:** Deployment log shows `git clone failed`.
- **Cause:** Repository is private and PAT token or SSH key is missing/expired.
- **Diagnosis:** Test git clone manually using your PAT or SSH key.
- **Solution:** Update your Personal Access Token or SSH Deploy Key in LITEPLOY.

---

## 4. Container Out of Memory (OOM)

- **Symptom:** Container exits unexpectedly with status `137`.
- **Cause:** Container exceeded configured RAM limit or host VPS ran out of memory.
- **Diagnosis:** Run `dmesg -T | grep -i oom` or inspect container stats.
- **Solution:** Increase the RAM limit in Application Configuration or add Swap memory to your VPS.

---

## 5. Domain Returns 502 Bad Gateway

- **Symptom:** Visiting `https://yourdomain.com` results in `502 Bad Gateway` or Cloudflare error.
- **Possible Causes & Solutions:**
  1. **Old Host Caddy Service Running:**
     - If `caddy.service` was previously installed on the host systemd, it may still be binding ports 80/443.
     - **Fix:** Stop and disable host Caddy:
       ```bash
       sudo systemctl stop caddy
       sudo systemctl disable caddy
       sudo systemctl restart liteploy
       ```
  2. **`liteploy-caddy` Container Not Running:**
     - Verify with `sudo docker ps`. If `liteploy-caddy` is missing:
       ```bash
       sudo systemctl restart liteploy
       ```
  3. **Cloudflare SSL/TLS Encryption Mode:**
     - If using Cloudflare proxy, go to **Cloudflare Dashboard** -> **SSL/TLS** -> set mode to **Full** (or **Full Strict**). Avoid *Flexible* as it can create protocol mismatches on port 80.
  4. **Application Container Starting / Unhealthy:**
     - Check if your backend/frontend container is running (`docker ps`) and inspect its logs on the dashboard.
