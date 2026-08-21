# Environment Variables & Secrets

LITEPLOY allows injecting runtime environment variables into your application containers.

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
     DATABASE_URL=postgres://user:pass@host:5432/dbname
     NODE_ENV=production
     ```

Click **Save Environment Variables** to apply changes.

> **Important:** Changing environment variables updates the persistent state immediately. Redeploy the application (`🚀 Deploy Now`) to inject the updated variables into the running container.
