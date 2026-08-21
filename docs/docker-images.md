# Docker Image Registries

LITEPLOY can deploy workloads directly from pre-built Docker image registries without compiling Dockerfiles on the VPS.

---

## 📦 Public Registries

For public images on Docker Hub (e.g. `nginx:alpine`, `redis:7-alpine`, `postgres:15-alpine`):
1. Select **Docker Image** as Workload Source.
2. Enter the image reference in `ImageRef` (e.g. `nginx:latest`).
3. Click **Create Application**.

---

## 🔐 Private Registries (GHCR, Docker Hub Private, AWS ECR)

For private images:
1. Check **This is a private registry image (requires authentication)**.
2. Fill in the authentication fields:
   - **Registry Server:** `ghcr.io` or `https://index.docker.io/v1/`
   - **Username:** Your registry username or organization.
   - **Password / Access Token:** Personal Access Token or registry password.
3. LITEPLOY authenticates with the registry using Docker Engine API Base64 Auth during image pull.
