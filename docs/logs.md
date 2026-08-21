# Realtime Logs & SSE Streaming

LITEPLOY streams logs using **Server-Sent Events (SSE)** without buffering entire log files in RAM.

---

## 🚀 Build Logs

When a deployment is triggered:
- Log output from Git clone and Docker build steps are streamed live to `/deployments/{id}`.
- SSE endpoint `/api/deployments/{id}/events` tails the build log file on disk.
- **Auto-scroll:** The log window automatically scrolls to bottom unless unchecked.

---

## 📄 Container Runtime Logs

To view live stdout/stderr runtime logs of an active container:
- Click **📄 Runtime Logs** on the Application Detail page.
- Endpoint `/api/applications/{id}/logs/stream` streams logs directly from the Docker Engine API.
