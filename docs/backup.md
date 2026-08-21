# Backup & Disaster Recovery

Because LITEPLOY uses a zero-database filesystem architecture, backup, migration, and disaster recovery are extremely simple.

---

## 💾 1-Click Web UI Backup & Migration (Recommended)

You can backup and restore your entire LITEPLOY configuration directly from the Web Interface without using SSH:

1. Go to **SYS CONFIG** (`/settings`) on your dashboard.
2. Scroll to the **💾 BACKUP & MIGRATION** section.
3. Click **[ DOWNLOAD BACKUP ARCHIVE (.tar.gz) ]**.
4. A compact, complete archive containing all your applications, domain mappings, environment variables, and admin credentials will be downloaded to your computer.

### Restoring on a New VPS:
1. Run the LITEPLOY installer on your new VPS:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | bash
   ```
2. Open the new LITEPLOY dashboard in your browser.
3. Go to **SYS CONFIG** -> **IMPORT & RESTORE BACKUP**.
4. Choose your `.tar.gz` backup file and click **[ RESTORE BACKUP ]**.
5. All your applications and settings are instantly restored! Click **[ DEPLOY ]** to spin up your containers on the new server.

---

## 🖥️ Manual CLI Backup & Restore (Alternative)

If you prefer using the command line:

```bash
# Archive the data directory
sudo tar -czvf liteploy-backup-$(date +%Y%m%d).tar.gz /var/lib/liteploy/data
```

### Restoring via CLI:
```bash
sudo systemctl stop liteploy
sudo tar -xzvf liteploy-backup-YYYYMMDD.tar.gz -C /
sudo systemctl start liteploy
```
