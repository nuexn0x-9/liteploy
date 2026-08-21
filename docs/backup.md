# Backup & Disaster Recovery

Because LITEPLOY uses a zero-database filesystem architecture, backup and disaster recovery are simple.

---

## 💾 Backing Up State

To create a full snapshot backup of your LITEPLOY configuration and application data:

```bash
# Archive the data directory
sudo tar -czvf liteploy-backup-$(date +%Y%m%d).tar.gz /var/lib/liteploy/data
```

---

## 🔄 Restoring State

To restore LITEPLOY on a new VPS server:

1. Install LITEPLOY using the installer script.
2. Stop the service:
   ```bash
   sudo systemctl stop liteploy
   ```
3. Extract your backup archive:
   ```bash
   sudo tar -xzvf liteploy-backup-20260820.tar.gz -C /
   ```
4. Start the service:
   ```bash
   sudo systemctl start liteploy
   ```
5. Click **🚀 Deploy Now** on your applications to start fresh containers from the restored configurations.
