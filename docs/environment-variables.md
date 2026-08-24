# Environment Variables & Secrets

LITEPLOY allows injecting runtime and build-time environment variables into your application containers.

---

## 🏗️ Build-Time vs. Runtime Environment Variables

LITEPLOY handles environment variables for both **runtime execution** and **static build compilation**:

1. **Runtime Injection:**
   - Variables are injected into the running container environment (`docker run -e KEY=VAL`).
   - Accessible by Node.js (`process.env`), Python (`os.environ`), Go (`os.Getenv`), PHP, etc.

2. **Build-Time Injection (Next.js, Vite, React, Nuxt):**
   - Frameworks with `NEXT_PUBLIC_*` or `VITE_*` variables require them **during `npm run build` / Docker build**.
   - Liteploy automatically:
     - Injects all configured environment variables into `.env` and `.env.production` inside the build context.
     - Passes variables as Docker `BuildArgs` (`--build-arg KEY=VAL`).
   - **Example for Single-Domain Setup:**
     ```env
     NEXT_PUBLIC_API_URL=/api
     ```
     This ensures Next.js bundles compile client requests to `/api/...` on the same domain rather than falling back to `http://localhost:8000`.

---

## 🔐 Key-Value Mode vs. Raw `.env` Importer

Under **🔐 Environment Variables (.env)** on the application detail page:

1. **Key-Value Mode:**
   - Add variables row by row (`KEY` and `VALUE`).
   - Secret Masking: Password fields conceal values by default. Click `👁` / `🔒` to toggle visibility.
2. **Raw `.env` Mode:**
   - Click **Switch to Raw .env Mode**.
   - Paste standard `.env` file contents:
     ```env
     PORT=3000
     NEXT_PUBLIC_API_URL=/api
     DATABASE_URL=postgres://user:pass@liteploy-db:5432/dbname
     NODE_ENV=production
     ```

Click **Save Environment Variables** to apply changes.

> **Important:** Changing environment variables updates the persistent state immediately. Redeploy the application (`🚀 Deploy Now`) to inject the updated variables into the build process and running container.

