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
