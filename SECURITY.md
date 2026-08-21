# LITEPLOY Security Policy

## Security Model Overview

LITEPLOY is designed to run self-hosted Docker workloads securely on small Linux VPS servers. Security is implemented through strict architectural boundaries:

1. **Authentication & Sessions:**
   - Cookie sessions are signed using HMAC-SHA256 with a 32-byte secret key.
   - User passwords are hashed using Bcrypt (cost factor 12).
   - Sessions are bound to HTTP-only cookies.

2. **CSRF Protection:**
   - All state-modifying requests (`POST`, `PUT`, `DELETE`) require a valid CSRF token.
   - CSRF tokens are validated on both standard HTML form submissions (`_csrf`) and HTMX AJAX requests (`X-CSRF-Token` header).

3. **Command Injection Prevention:**
   - Zero raw shell invocations (`sh -c` or `bash -c`).
   - All external processes (e.g. `git clone`) use `exec.CommandContext` with separate argument vectors.

4. **Credential & Secret Protection:**
   - Sensitive credentials (Git PAT tokens, SSH Deploy Keys, Registry Passwords) are stored securely and never written to standard output.
   - Log streaming is wrapped in an active `maskWriter` that redacts secret tokens (`******`) from build outputs before writing to disk or streaming via SSE.

5. **Path Traversal & Filesystem Safety:**
   - Storage operations use strict `filepath.Rel` and `filepath.Clean` validation (`safePath()`) to prevent escaping the designated `data_dir`.
   - Temporary SSH keys are written with `0600` file permissions and removed immediately after cloning.

---

## ⚠️ Docker Socket Privilege Notice

Access to the Docker daemon socket (`/var/run/docker.sock`) grants root-equivalent access to the host operating system.

- **Isolation:** LITEPLOY communicates with Docker via the official Moby Go SDK over the UNIX socket.
- **Hardening:** Containers created by LITEPLOY do not run in `--privileged` mode by default, nor do they mount host directories unless explicitly configured.
- **Server Access:** Protect access to your VPS server and the LITEPLOY dashboard credentials at all times.

---

## Reporting a Vulnerability

If you discover a security vulnerability in LITEPLOY, please **do not** open a public GitHub issue.

Instead, please report the vulnerability directly to the project maintainers:

- **Email:** Send details to `security@liteploy.io` (or contact the repository maintainer on GitHub).
- **Information to include:**
  - Description of the vulnerability and potential impact.
  - Step-by-step instructions or proof-of-concept script to reproduce the issue.
  - Affected version of LITEPLOY.

We will acknowledge receipt of your report within 48 hours and provide status updates as we work on a fix.
