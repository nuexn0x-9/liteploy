# Automated Webhook Deployments

LITEPLOY supports automated deployments on `git push` via webhooks for GitHub, GitLab, and Gitea.

---

## 🔗 Configuring GitHub Webhooks

1. Go to your GitHub repository **Settings -> Webhooks -> Add webhook**.
2. **Payload URL:** Copy the Webhook URL from LITEPLOY (`http://<your-vps-ip>:8080/api/webhooks/app-xxx`).
3. **Content type:** `application/json`
4. **Secret:** Copy the Webhook Secret from LITEPLOY.
5. Select **Just the push event**.
6. Click **Add webhook**.

---

## 🛡️ Security Model

- **Asynchronous Processing:** Webhooks return `202 Accepted` immediately and enqueue the build asynchronously to prevent HTTP timeouts.
- **HMAC Validation:** GitHub signatures (`X-Hub-Signature-256`) and GitLab tokens (`X-Gitlab-Token`) are verified cryptographically before accepting payloads.
