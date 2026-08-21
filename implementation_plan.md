# Goal Description

Implement 4 critical production features for LITEPLOY to ensure stable long-term operation on 1GB VPS environments:
1. Docker Garbage Collection (System Prune)
2. Live Container Metrics (CPU/RAM)
3. HTTP Health Checks
4. 1-Click Deployment Rollback

## Proposed Changes

### Docker Layer (internal/docker/docker.go)
#### [MODIFY] internal/docker/docker.go
- Add `PruneAll(ctx context.Context)` to `Engine` interface. Implement it in `Client` (calls `ImagesPrune`, `ContainersPrune`, `NetworksPrune`, `BuildCachePrune`).
- Add `ContainerStats(ctx context.Context, id string) (*Stats, error)` to `Engine`. Implement it to return CPU and Memory usage % and bytes.

### Application Layer (internal/application)
#### [MODIFY] internal/application/application.go
- Add `HealthcheckPath string` to `Application`.

### Deployment Layer (internal/deployment)
#### [MODIFY] internal/deployment/pipeline.go
- Update `waitForHealth` to perform an HTTP GET request to `http://<containerIP>:<port><HealthcheckPath>` if `HealthcheckPath` is set. If not set, keep the TCP dial fallback. (Will need to look up `containerIP` from `dockerCli.InspectContainer`).
- Add `Rollback(ctx context.Context, appID string, depID string) error` logic to deployment service or handle it via a specialized pipeline step, which basically creates a new deployment utilizing the specific `ImageID` of the previous deployment.

### API Layer (internal/api)
#### [MODIFY] internal/api/handlers_app.go
- Parse and save `healthcheck_path` in `handleApplicationUpdate`.
- Add `handleSystemPrune` handler to execute `dockerCli.PruneAll`.
- Add `handleApplicationStats` handler to return a tiny HTML partial with the container's CPU and RAM stats.
- Add `handleApplicationRollback` handler to trigger a rollback deployment.

### UI Layer (web/templates/pages)
#### [MODIFY] web/templates/pages/application_detail.html
- Add input field for `HealthcheckPath`.
- Add HTMX polling element: `<div hx-get="/applications/{{.App.ID}}/stats" hx-trigger="every 5s" hx-swap="innerHTML"></div>` to display live metrics.
#### [MODIFY] web/templates/pages/settings.html
- Add a "System Cleanup" button calling `/system/prune` via HTMX.
#### [MODIFY] web/templates/pages/deployments.html
- Add a `[ ROLLBACK ]` button inside the deployment history table.

## Verification Plan
### Automated Tests
- `go build ./cmd/liteploy` and `go test ./...`

### Manual Verification
- Prune: Click button and check if Docker reclaims space.
- Stats: Open app detail and observe the RAM/CPU updating.
- Rollback: Click rollback and verify the app reverts to the older ImageID.
- Healthcheck: Deploy with `/healthz` and watch the pipeline logs.
