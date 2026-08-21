# Quick Start Walkthrough

This guide takes you through the complete end-to-end flow of deploying your first web application with LITEPLOY.

---

## The Deployment Path

```
Clean VPS ──► Docker ──► LITEPLOY ──► Dashboard ──► Application ──► Git Repo ──► Build ──► Container ──► Domain ──► HTTPS
```

---

## Step 1: Install LITEPLOY on your VPS

Run the installation script on your server:

```bash
curl -fsSL https://raw.githubusercontent.com/nuexn0x-9/liteploy/main/scripts/install.sh | sudo bash
```

---

## Step 2: Create Administrator Account

1. Open `http://<your-vps-ip>:8080` in your web browser.
2. Complete the initial setup wizard by choosing an administrator username and a secure password.
3. Log in to access the Dashboard.

---

## Step 3: Create Your First Application

1. Click **+ New Application** in the header.
2. Fill in the general fields:
   - **Application Name:** `my-web-app`
   - **Workload Source:** `Git Repository`
   - **Repository URL:** `https://github.com/yourname/my-web-app.git`
   - **Branch:** `main`
   - **Container Port:** `3000` *(the port your web app listens on inside its container)*
3. Click **Create Application**.

---

## Step 4: Add Environment Variables

On the application detail page:
1. Scroll down to **🔐 Environment Variables (.env)**.
2. Add key-value pairs (e.g. `NODE_ENV=production`, `PORT=3000`).
3. Click **Save Environment Variables**.

---

## Step 5: Deploy the Workload

1. Click the **🚀 Deploy Now** button at the top right.
2. LITEPLOY will queue the deployment, clone the repository, build the Dockerfile, start the container, and perform health checks.
3. Click **View Log** to follow the real-time SSE build output.

---

## Step 6: Map Custom Domain & Automatic HTTPS

1. Ensure your domain's DNS `A` record points to `<your-vps-ip>` (e.g. `app.yourdomain.com -> 1.2.3.4`).
2. On the application detail page under **🌐 Custom Domains & SSL**, enter `app.yourdomain.com` and click **+ Add Domain**.
3. Click **🔍 Test DNS** to verify DNS resolution.
4. Caddy Reverse Proxy will automatically issue a free Let's Encrypt / ZeroSSL TLS certificate and route incoming HTTPS traffic to your container!
