# Tutorial: Deploying a Multi-Tier Application (Frontend + Backend + Database)

LITEPLOY is designed to be simple, but it is fully capable of running complex, multi-tier architectures like a standard MERN (MongoDB, Express, React, Node) or PERN (PostgreSQL, Express, React, Node) stack.

In this tutorial, we will deploy a **PostgreSQL Database** and a **Node.js API Backend** that connects to it. 

Because LITEPLOY places all containers inside an isolated internal Docker network (`liteploy-net`), containers can communicate with each other securely using their **internal DNS names** without exposing database ports to the public internet.

---

## Step 1: Deploy the Database (PostgreSQL)

Instead of building from source code, we will deploy our database using an official Docker Image.

1. Go to the LITEPLOY Dashboard and click **+ New Application**.
2. **Name:** `db-production`
3. **Source:** Select **Docker Image**.
4. **Image Reference:** `postgres:15-alpine`
5. **Container Port:** `5432` *(Standard PostgreSQL port)*
6. Click **Create Application**.

### Configure Database Environment Variables
PostgreSQL requires a password to initialize.
1. Scroll down to the **Environment Variables** section.
2. Add: `POSTGRES_PASSWORD=mysecretpassword`
3. Add: `POSTGRES_USER=myuser`
4. Add: `POSTGRES_DB=mydb`
5. Click **Save Environment Variables**.

### Configure Persistent Volume (CRITICAL)
If you don't map a volume, your database will be wiped out every time you redeploy or restart the container!
1. Scroll down to the **Persistent Volumes** section.
2. **Host Path:** `/var/lib/liteploy/volumes/postgres_data` *(This folder will be created on your VPS)*
3. **Container Path:** `/var/lib/postgresql/data` *(Where PostgreSQL stores data internally)*
4. Click **Update Config**.

### Deploy the Database
Click **Deploy Now** at the top right.
Once it says **SUCCESS**, note the Application ID in the header (for example: `abc12345`).
The internal DNS name for your database is `liteploy-abc12345`.

---

## Step 2: Deploy the Backend (Node.js API)

Now we will deploy the backend code from GitHub.

1. Go to the Dashboard and click **+ New Application**.
2. **Name:** `api-backend`
3. **Source:** Select **Git Repository**.
4. **Repository URL:** `https://github.com/yourusername/your-backend-api.git`
5. **Branch:** `main`
6. **Container Port:** `3000` *(Or whatever port your Express.js app listens on)*
7. **Healthcheck Path:** `/api/health` *(Optional: Ensures zero-downtime deployments)*
8. Click **Create Application**.

### Connect Backend to Database
Your Node.js app needs to know how to connect to PostgreSQL. We use the internal DNS name of the database container we just created.

Assuming your Database App ID was `abc12345`, the connection string is:
`postgres://myuser:mysecretpassword@liteploy-abc12345:5432/mydb`

1. Scroll down to **Environment Variables**.
2. Add: `DATABASE_URL=postgres://myuser:mysecretpassword@liteploy-abc12345:5432/mydb`
3. Add: `PORT=3000`
4. Click **Save Environment Variables**.

### Map Custom Domain
1. In the **Domains** section, add `api.yourdomain.com`.
2. Ensure your DNS `A` record points to your VPS IP.

### Deploy the Backend
Click **Deploy Now**. LITEPLOY will clone your repo, build the Dockerfile, and start the container.
Once successful, you can visit `https://api.yourdomain.com` in your browser. Caddy will automatically secure it with HTTPS!

---

## Step 3: Deploy the Frontend (React / Vue / Next.js)

The process for the frontend is identical to the backend.

1. Create a new Application (e.g., `web-frontend`).
2. Point it to your Frontend Git Repository.
3. Set the Container Port (e.g., `80` if using NGINX, or `3000` for Next.js).
4. In **Environment Variables**, provide your backend URL (e.g., `NEXT_PUBLIC_API_URL=https://api.yourdomain.com`).
5. Map a custom domain (e.g., `www.yourdomain.com`).
6. Click **Deploy Now**.

---

## 💡 Summary

You have successfully deployed a 3-tier application!
- **Database:** Runs securely in the background. Data is safe via Persistent Volumes. Not exposed to the public internet.
- **Backend:** Communicates with the Database via internal Docker DNS. Exposed to the internet via Caddy with auto-HTTPS.
- **Frontend:** Exposed to the internet via Caddy with auto-HTTPS. Communicates with the Backend via the public HTTPS API.
