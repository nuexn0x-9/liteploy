# Application Management

An **Application** in LITEPLOY represents a single workload (a Git repository or Docker image) configured to run as a Docker container.

---

## ➕ Creating an Application

1. Click **+ New Application** from the navigation bar.
2. Enter a unique **Application Name** (1–64 alphanumeric characters, hyphens, or underscores).
3. Select your **Workload Source Type**:
   - **Git Repository:** Build from Dockerfile in a repository.
   - **Docker Image:** Pull pre-built image from registry.
4. Specify the **Container Port** (e.g. `3000`, `8080`, `80`).
5. Click **Create Application**.

---

## ⚙️ Modifying Configuration

On the Application Detail page (`/applications/{id}`), you can modify:
- Application Name & Container Port
- Git Repository URL, Branch, and Dockerfile path
- Authentication credentials (PAT / SSH Key)
- RAM limit cap (MB)

Click **Save Configuration** to apply updates.

---

## 🗑️ Deleting an Application

1. Scroll to the bottom of the Application Detail page to the **⚠️ Danger Zone**.
2. Click **Delete Application**.
3. Confirm deletion. LITEPLOY will stop and remove active containers, clear routing from Caddy, and remove state records.
