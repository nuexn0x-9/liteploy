# Storage & Persistence Architecture

LITEPLOY operates with **Zero Database Dependencies**.

---

## 💾 Atomic Filesystem Engine

All persistent application configurations and deployment histories are saved as human-readable JSON files in `LITEPLOY_DATA_DIR` (`/var/lib/liteploy/data`).

### Write Safety Guarantees:
1. **Atomic Write Pattern:** Data is written to a temporary file (`.tmp`), synced to disk (`fsync`), and renamed atomically (`os.Rename`).
2. **Corruption Prevention:** Partial writes caused by power loss do not corrupt existing state files.
3. **Path Traversal Protection:** All storage subpaths are validated via `filepath.Rel` and `filepath.Clean` (`safePath()`).

---

## ?? System Garbage Collection (Auto-Prune)

Docker deployments inherently consume disk space as old images, unused networks, and build caches accumulate. On a typical 20GB VPS, this can lead to disk exhaustion within weeks.

LITEPLOY includes a built-in **System Cleanup** tool available in the **SYS CONFIG** dashboard.
Running this tool safely executes a Docker garbage collection pass (docker system prune -af), which clears out all dangling images, unused networks, and stopped containers, reclaiming gigabytes of disk space without affecting your active workloads.