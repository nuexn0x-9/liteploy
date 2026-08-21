# Container Lifecycle Management

LITEPLOY provides container lifecycle management directly via the Docker Engine API.

---

## 🎮 Lifecycle Actions

- **▶ Start:** Starts a stopped container.
- **⏹ Stop:** Gracefully stops a running container with a timeout.
- **🔄 Restart:** Restarts the active container.
- **🚀 Deploy Now:** Re-clones repository, rebuilds Dockerfile, and performs a zero-downtime container swap.

---

## 🔄 Startup Reconciler

When the VPS reboots or LITEPLOY restarts:
1. The `system.Reconciler` queries Docker Engine for containers labeled `liteploy.managed=true`.
2. It reconciles saved application states with actual container states.
3. Incomplete deployments interrupted by a reboot are cleanly marked `FAILED`.
