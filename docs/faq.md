# Frequently Asked Questions (FAQ)

### Q: Can LITEPLOY run on a $5/month VPS with 1 GB RAM?
**A:** Yes! LITEPLOY's internal engine consumes approximately 18.5 MB RAM at idle, leaving over 70% of host memory available for your applications.

### Q: Does LITEPLOY require a database like PostgreSQL or MySQL?
**A:** No. LITEPLOY persists all application state and deployment history as structured JSON files directly on the filesystem using atomic write operations.

### Q: Does updating LITEPLOY restart my running application containers?
**A:** No. Running application containers managed by Docker continue operating uninterrupted when LITEPLOY is restarted or updated.

### Q: Is multi-server / clustering supported?
**A:** No. LITEPLOY deliberately focuses on single-server self-hosting simplicity for small VPS instances.

### Q: How do I backup my LITEPLOY setup?
**A:** Simply copy or archive the `/var/lib/liteploy/data` directory.
