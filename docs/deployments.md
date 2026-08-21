# Deployment Pipeline & State Machine

Every deployment in LITEPLOY progresses through an explicit, deterministic state machine.

---

## 🔄 Deployment Lifecycle

```
QUEUED ──► PREPARING ──► BUILDING ──► STARTING ──► HEALTH_CHECK ──► ROUTING ──► SUCCESS
  │            │            │           │             │               │
  └────────────┴────────────┴───────────┴─────────────┴───────────────┴────► FAILED
```

### Stage Details:
1. **QUEUED:** Deployment placed in bounded job queue.
2. **PREPARING:** Git repository cloned into temporary build directory.
3. **BUILDING:** Dockerfile image compiled or Docker image pulled.
4. **STARTING:** New container created and attached to `liteploy-net`.
5. **HEALTH_CHECK:** Container health status verified.
6. **ROUTING:** Caddy reverse proxy updated with zero-downtime swap.
7. **SUCCESS:** Previous container removed; deployment marked complete.

---

## 🔒 Concurrency & Per-Application Locking

- **Per-App Lock:** Prevents race conditions when deploying the same app twice simultaneously.
- **Worker Pool:** Bounded worker pool (default 1 concurrent build) prevents RAM exhaustion on 1 GB VPS servers.

---

## ?? Zero-Downtime HTTP Healthchecks

During the HEALTH_CHECK stage, LITEPLOY ensures your new container is ready to accept traffic before cutting over routing from the old container.
You can configure a specific **Healthcheck Path** (e.g. /api/health) in the application settings.
If set, LITEPLOY will poll that URL via HTTP GET and wait for a 200 OK response before proceeding. If the container fails to return 200 within the timeout, the deployment is aborted, and the old container remains untouched.

---

## ? 1-Click Rollback

If a new deployment succeeds but contains logical errors, you can instantly revert to a previous version without waiting for a new Git clone and Docker build.
In the **Deployments History** table, click the **[ ROLLBACK ]** button next to a previously successful deployment. LITEPLOY will immediately queue a special deployment that skips the build phase and directly starts the historical Docker ImageID.