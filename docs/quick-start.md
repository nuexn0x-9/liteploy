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
1. **Environment Variables:** Scroll down to **🔐 Environment Variables (.env)** and add key-value pairs (e.g. `NODE_ENV=production`, `PORT=3000`).
2. **Persistent Volumes (Optional):** If your app stores data or sqlite files, map a host directory (e.g. `/var/lib/myapp/data -> /app/data`).

---

## Step 5: Deploy the Workload

1. Click the **[ DEPLOY ]** button.
2. LITEPLOY queues the deployment, pulls source with caching, builds the image, starts the container, and verifies health status.
3. Click **[ LOG ]** to follow real-time streaming build output.

---

## Step 6: Map Subdomain & Enjoy Instant HTTPS

1. Under **🌐 DOMAINS & NETWORK**, add your desired subdomain (e.g. `app.example.com`).
2. Because wildcard DNS (`*.example.com`) is already pointed to your VPS, the subdomain is live immediately!
3. Caddy automatically provisions a TLS certificate and serves `https://app.example.com`.
