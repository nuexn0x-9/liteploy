# Storage & Persistence Architecture

LITEPLOY operates with **Zero Database Dependencies**.

---

## 💾 Atomic Filesystem Engine

All persistent application configurations and deployment histories are saved as human-readable JSON files in `LITEPLOY_DATA_DIR` (`/var/lib/liteploy/data`).

### Write Safety Guarantees:
1. **Atomic Write Pattern:** Data is written to a temporary file (`.tmp`), synced to disk (`fsync`), and renamed atomically (`os.Rename`).
2. **Corruption Prevention:** Partial writes caused by power loss do not corrupt existing state files.
3. **Path Traversal Protection:** All storage subpaths are validated via `filepath.Rel` and `filepath.Clean` (`safePath()`).
