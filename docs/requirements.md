# System Requirements & VPS Sizing

LITEPLOY is engineered specifically for low-resource Linux Virtual Private Servers (VPS).

---

## 💻 Hardware Requirements

| Resource | Baseline Minimum | Recommended Setup |
|---|---|---|
| **RAM** | 512 MB | 1 GB or higher |
| **CPU** | 1 vCPU | 1–2 vCPUs |
| **Disk** | 5 GB SSD | 20 GB+ SSD |
| **Architecture** | `amd64` (x86_64) or `arm64` (aarch64) | `amd64` / `arm64` |

---

## 🐧 Software Compatibility

- **Linux Kernel:** 4.19 or higher
- **Supported Linux Distributions:**
  - Ubuntu 20.04 LTS / 22.04 LTS / 24.04 LTS
  - Debian 11 (Bullseye) / Debian 12 (Bookworm)
  - CentOS Stream 9 / Rocky Linux 9 / AlmaLinux 9
  - Alpine Linux 3.18+
- **Container Engine:** Docker Engine 20.10+ (or Docker CE)
- **Reverse Proxy:** Caddy 2.7+ (optional; required if using automatic domain routing)

---

## 📊 VPS Memory Allocation Breakdown (1 GB VPS Example)

On a typical $5/mo VPS (1 GB RAM):

```
+------------------------------------------------------+
| Host System & Linux Kernel       ~ 150 MB RAM        |
+------------------------------------------------------+
| Docker Engine Daemon             ~  80 MB RAM        |
+------------------------------------------------------+
| LITEPLOY Engine                  ~  18.5 MB RAM      |
+------------------------------------------------------+
| Caddy Reverse Proxy              ~  30 MB RAM        |
+------------------------------------------------------+
| Available RAM for Applications   ~ 700+ MB RAM       |
+------------------------------------------------------+
```

Unlike heavy management panels that take 400 MB+ RAM for panel infrastructure alone, LITEPLOY leaves **over 70% of total VPS memory available** for your user applications.
