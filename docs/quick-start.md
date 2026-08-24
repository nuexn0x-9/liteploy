# Quick Start Walkthrough

This guide takes you through the complete end-to-end flow of setting up LITEPLOY, configuring wildcard DNS, and deploying your first web application.

---

## The Deployment Path

```
Clean VPS ──► Install LITEPLOY ──► Setup Wizard ──► Wildcard DNS ──► Secure Dashboard ──► Deploy App (Subdomain) ──► Automatic HTTPS
```

---

## Step 1: Install LITEPLOY on your VPS

Run the official one-command installer on your server:

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
```

The installer automatically configures dependencies, generates secure session secrets in `/etc/liteploy/liteploy.env`, starts the systemd service, and verifies health check status.

---

## Step 2: Initial Setup Wizard

1. Open `http://<your-vps-ip>:8080` in your web browser.
2. **Admin Credentials:** Enter your administrator username and access code.
3. **Primary Domain Configuration:**
   - Enter your root domain (e.g. `example.com`).
   - Add two `A` records in your DNS provider:
     - `A @ -> <YOUR_VPS_IP>`
     - `A * -> <YOUR_VPS_IP>`
   - Click **[ VERIFY DNS & ENABLE HTTPS ]** (or click *Skip for now*).
4. You can now access your dashboard securely via `https://liteploy.example.com`!

---

## Step 3: Create Your First Application

1. In the navigation bar, click **[ + NEW APPLICATION ]**.
2. Fill in the workload details:
   - **Application Name:** `my-web-app`
   - **Workload Source:** `Git Repository` (or `Docker Image`)
   - **Repository URL:** `https://github.com/yourname/my-web-app.git`
   - **Branch:** `main`
   - **Container Port:** `3000` *(the internal port your app listens on)*
   - **Healthcheck Path:** `/health` or `/`
3. Click **[ CREATE APPLICATION ]**.

---

## Step 4: Add Environment Variables & Volumes

On the application detail page:
1. **Environment Variables:** Scroll down to **🔐 Environment Variables (.env)** and add key-value pairs (e.g. `PORT=3000`, `NODE_ENV=production`, or `NEXT_PUBLIC_API_URL=/api` for single-domain frontend apps). LITEPLOY automatically injects these into `.env.production` during the build step and passes them as runtime container variables.
2. **Persistent Volumes (Optional):** If your app stores database files or uploads, map a host directory (e.g. `/var/lib/myapp/data -> /app/data`).

---

## Step 5: Deploy the Workload

1. Click the **[ DEPLOY ]** button.
2. LITEPLOY queues the deployment, pulls source with caching, builds the image, starts the container, and verifies health status.
3. Click **[ LOG ]** to follow real-time streaming build output.

---

## Step 6: Map Domain, Subdomain, or Path Routing

Under **🌐 DOMAINS & NETWORK**, add your desired routing target:
- **Subdomain Routing:** Enter `app.example.com` (ready instantly with wildcard DNS).
- **Same-Domain Path Routing:** For backend/API services, enter Domain: `example.com` and Path: `/api/*` (and `/assets/*`). For frontend services, enter Domain: `example.com` and Path: `/*`.
- Caddy automatically manages routing order and provisions TLS certificates.

