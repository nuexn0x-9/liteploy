# Updating & Upgrading LITEPLOY

LITEPLOY is packaged as a single binary, making upgrades straightforward.

---

## ⚡ Automatic Upgrade

Run the installer script to download the latest binary and restart the service:

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
```

---

## 🛠️ Manual Upgrade

1. Download the new binary:
   ```bash
   curl -fsSL https://github.com/nuexn0x-9/liteploy/releases/latest/download/liteploy-linux-amd64 -o /tmp/liteploy
   chmod +x /tmp/liteploy
   ```
2. Replace the binary and restart:
   ```bash
   sudo systemctl stop liteploy
   sudo mv -f /tmp/liteploy /usr/local/bin/liteploy
   sudo systemctl start liteploy
   ```
3. Check status:
   ```bash
   sudo systemctl status liteploy
   ```

> **Note:** Updating LITEPLOY does **not** interrupt running application containers. Application state is preserved on disk and reconciled automatically upon boot.
