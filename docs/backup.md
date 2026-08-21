# Backup & Disaster Recovery

Because LITEPLOY uses a zero-database filesystem architecture, backup, migration, and disaster recovery are fast, portable, and reliable.

---

## 💾 1-Click Web UI Backup & Migration (Recommended)

You can backup and restore your entire LITEPLOY configuration directly from the Web Interface:

1. Go to **SYS CONFIG** (`/settings`) on your dashboard.
2. Scroll to the **💾 BACKUP & MIGRATION** section.
3. Click **[ DOWNLOAD BACKUP ARCHIVE (.tar.gz) ]**.
4. A compact `.tar.gz` archive containing all your applications, domain mappings, Primary Domain configuration, environment variables, and admin credentials will be downloaded.

### Restoring / Migrating to a New VPS:
1. Run the LITEPLOY installer on your new VPS:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
   ```
2. Open the new LITEPLOY dashboard in your browser.
3. Go to **SYS CONFIG** -> **IMPORT & RESTORE BACKUP**.
4. Choose your `.tar.gz` backup file and click **[ RESTORE BACKUP ]**.
5. All your applications, primary domain settings, and credentials are restored instantly! Point your wildcard DNS records to the new VPS IP and click **[ DEPLOY ]** to build your workloads.

---

## 🖥️ Manual CLI Backup & Restore (Alternative)

If you prefer using the command line:

```bash
# Archive data and configuration directories
sudo tar -czvf liteploy-backup-$(date +%Y%m%d).tar.gz /var/lib/liteploy/data /etc/liteploy
```

### Restoring via CLI:
```bash
sudo systemctl stop liteploy
sudo tar -xzvf liteploy-backup-YYYYMMDD.tar.gz -C /
sudo systemctl start liteploy
```
