// Package deployment — Pipeline implements the complete build and run workflow
// for an Application deployment.
//
// Pipeline lifecycle:
//
//	PREPARING (fetch/clone source)
//	  ↓
//	BUILDING (docker build or pull)
//	  ↓
//	STARTING (create & start container)
//	  ↓
//	HEALTH_CHECK (verify readiness)
//	  ↓
//	ROUTING (configure Caddy proxy)
//	  ↓
//	SUCCESS (cleanup old container & temp files)
//
// Error handling:
//
//	If any step fails, the new container is cleaned up, the previous container
//	remains active (zero-downtime pattern), and the deployment is marked FAILED.
package deployment

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/docker"
	"github.com/liteploy/liteploy/internal/git"
	"github.com/liteploy/liteploy/internal/proxy"
	"github.com/liteploy/liteploy/internal/storage"
)

// Pipeline coordinates Docker, Git, Storage, and Proxy to execute a deployment.
type Pipeline struct {
	store      *storage.Store
	dockerCli  docker.Engine
	proxyMgr   *proxy.Manager
	appSvc     *application.Service
	logger     *slog.Logger
	gitTimeout time.Duration
	buildTimeout time.Duration
	healthTimeout time.Duration
}

// NewPipeline creates a deployment Pipeline.
func NewPipeline(
	store *storage.Store,
	dockerCli docker.Engine,
	proxyMgr *proxy.Manager,
	appSvc *application.Service,
	logger *slog.Logger,
	gitTimeout, buildTimeout, healthTimeout time.Duration,
) *Pipeline {
	if gitTimeout <= 0 {
		gitTimeout = 10 * time.Minute
	}
	if buildTimeout <= 0 {
		buildTimeout = 20 * time.Minute
	}
	if healthTimeout <= 0 {
		healthTimeout = 2 * time.Minute
	}

	return &Pipeline{
		store:         store,
		dockerCli:     dockerCli,
		proxyMgr:      proxyMgr,
		appSvc:        appSvc,
		logger:        logger,
		gitTimeout:    gitTimeout,
		buildTimeout:  buildTimeout,
		healthTimeout: healthTimeout,
	}
}

// Execute runs the complete deployment pipeline.
func (p *Pipeline) Execute(ctx context.Context, dep *Deployment, progress io.Writer) error {
	app, err := p.appSvc.Get(dep.AppID)
	if err != nil {
		return fmt.Errorf("pipeline: get app %q: %w", dep.AppID, err)
	}

	fmt.Fprintf(progress, "[liteploy] === Starting Deployment #%s for %s ===\n", dep.ID, app.Name)

	var newImageID string

	// -------------------------------------------------------------
	// Step 1: PREPARING (Source resolution)
	// -------------------------------------------------------------
	dep.Transition(StatusPreparing)
	dep.Stage = "preparing"
	fmt.Fprintf(progress, "[liteploy] Step 1/5: Preparing source (%s)...\n", app.Source.Type)

	var buildContextDir string
	var imageName string

	if dep.RollbackTo != "" {
		fmt.Fprintf(progress, "[liteploy] ROLLBACK INITIATED\n")
		fmt.Fprintf(progress, "[liteploy] Skipping build/pull. Reverting to Image: %s\n", dep.RollbackTo)
		imageName = dep.RollbackTo
	} else {
		switch app.Source.Type {
		case application.SourceGit:
		relBuildDir := filepath.Join("repos", app.ID)
		absBuildDir, err := p.store.AbsPath(relBuildDir)
		if err != nil {
			return fmt.Errorf("prepare build dir: %w", err)
		}

		fmt.Fprintf(progress, "[liteploy] Synchronizing %s (branch: %s)...\n", app.Source.GitURL, app.Source.GitBranch)
		if app.Source.GitToken != "" {
			fmt.Fprintf(progress, "[liteploy] Git authentication: Personal Access Token (PAT) configured\n")
		} else if app.Source.GitSSHKey != "" {
			fmt.Fprintf(progress, "[liteploy] Git authentication: SSH Private Key configured\n")
		}

		cloneResult, err := git.Sync(ctx, git.CloneOptions{
			URL:       app.Source.GitURL,
			Branch:    app.Source.GitBranch,
			Depth:     1,
			TargetDir: absBuildDir,
			AuthToken: app.Source.GitToken,
			SSHKey:    app.Source.GitSSHKey,
			Timeout:   p.gitTimeout,
			Progress:  progress,
		})
		if err != nil {
			return fmt.Errorf("git sync: %w", err)
		}

		dep.CommitSHA = cloneResult.CommitSHA
		buildContextDir = absBuildDir
		imageName = fmt.Sprintf("liteploy-%s:%s", app.ID, dep.ID)
		fmt.Fprintf(progress, "[liteploy] Synced commit %s\n", cloneResult.CommitSHA)

	case application.SourceImage:
		imageName = app.Source.ImageRef
		fmt.Fprintf(progress, "[liteploy] Using Docker image: %s\n", imageName)

	case application.SourceCompose:
		return fmt.Errorf("compose deployment pipeline not implemented in this phase")

	default:
		return fmt.Errorf("unsupported source type: %s", app.Source.Type)
	}
	}

	// -------------------------------------------------------------
	// Step 2: BUILDING (Image creation / pull)
	// -------------------------------------------------------------
	if dep.RollbackTo == "" {
		dep.Transition(StatusBuilding)
		dep.Stage = "building"
		fmt.Fprintf(progress, "[liteploy] Step 2/5: Building/pulling image...\n")

		if app.Source.Type == application.SourceGit {
			dockerfile := app.Source.DockerfilePath
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			fmt.Fprintf(progress, "[liteploy] Building Dockerfile: %s\n", dockerfile)
			imgID, err := p.dockerCli.BuildImage(ctx, docker.BuildOptions{
				ContextDir:     buildContextDir,
				DockerfilePath: dockerfile,
				Tags:           []string{imageName},
				NoCache:        false,
			}, progress)
			if err != nil {
				return fmt.Errorf("docker build: %w", err)
			}
			newImageID = imgID
			dep.ImageID = imgID
			fmt.Fprintf(progress, "[liteploy] Build complete. Image: %s\n", imgID)
		} else if app.Source.Type == application.SourceImage {
			var regAuth *docker.RegistryAuth
			if app.Source.RegistryAuth != nil {
				regAuth = &docker.RegistryAuth{
					ServerAddress: app.Source.RegistryAuth.ServerAddress,
					Username:      app.Source.RegistryAuth.Username,
					Password:      app.Source.RegistryAuth.Password,
				}
			}
			fmt.Fprintf(progress, "[liteploy] Pulling %s...\n", imageName)
			if err := p.dockerCli.PullImageWithAuth(ctx, imageName, regAuth, progress); err != nil {
				return fmt.Errorf("docker pull: %w", err)
			}
			newImageID = imageName
			dep.ImageID = imageName
			fmt.Fprintf(progress, "[liteploy] Pull complete.\n")
		}
	} else {
		newImageID = dep.RollbackTo
		dep.ImageID = dep.RollbackTo
	}

	// -------------------------------------------------------------
	// Step 3: STARTING (Container creation & start)
	// -------------------------------------------------------------
	dep.Transition(StatusStarting)
	dep.Stage = "starting"
	fmt.Fprintf(progress, "[liteploy] Step 3/5: Creating and starting container...\n")

	// Ensure common internal network exists
	networkName := proxy.LiteployNetwork
	if p.dockerCli != nil {
		if _, err := p.dockerCli.EnsureNetwork(ctx, networkName); err != nil {
			p.logger.Warn("pipeline: ensure network warning", "network", networkName, "error", err)
		}
	}

	containerName := fmt.Sprintf("liteploy-%s-%s", app.ID, dep.ID)
	// stableAlias lets Caddy (inside liteploy-network) resolve this container
	// via Docker DNS as "liteploy-{appID}" — stable across redeployments.
	stableAlias := fmt.Sprintf("liteploy-%s", app.ID)
	labels := app.ManagedLabels()
	labels["liteploy.deployment_id"] = dep.ID

	// Read environment variables (encrypted at rest if configured)
	var envList []string
	envPath := filepath.Join("applications", app.ID, "env.json")
	var envVars map[string]string
	if err := p.store.ReadJSON(envPath, &envVars); err == nil {
		for k, v := range envVars {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	var memMB int64
	var cpus float64
	if app.ResourceLimits != nil {
		memMB = app.ResourceLimits.MemoryMB
		cpus = app.ResourceLimits.CPUs
	}

	var binds []string
	for _, vol := range app.Volumes {
		binds = append(binds, fmt.Sprintf("%s:%s", vol.HostPath, vol.ContainerPath))
	}

	containerID, err := p.dockerCli.CreateContainer(ctx, docker.ContainerSpec{
		Name:           containerName,
		Image:          imageName,
		Env:            envList,
		Labels:         labels,
		Binds:          binds,
		ContainerPort:  app.Port,
		HostPort:       0, // no host port needed — Caddy reaches container via Docker DNS
		NetworkName:    networkName,
		NetworkAliases: []string{stableAlias},
		MemoryMB:       memMB,
		CPUs:           cpus,
		RestartPolicy:  "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	dep.ContainerID = containerID

	fmt.Fprintf(progress, "[liteploy] Container created: %s. Starting...\n", containerID[:12])
	if err := p.dockerCli.StartContainer(ctx, containerID); err != nil {
		// Cleanup created container
		_ = p.dockerCli.RemoveContainer(context.Background(), containerID, true)
		return fmt.Errorf("start container: %w", err)
	}

	// -------------------------------------------------------------
	// Step 4: HEALTH_CHECK (Wait for readiness)
	// -------------------------------------------------------------
	dep.Transition(StatusHealthCheck)
	dep.Stage = "health_check"
	fmt.Fprintf(progress, "[liteploy] Step 4/5: Running health checks...\n")

	if err := p.waitForHealth(ctx, containerID, app); err != nil {
		fmt.Fprintf(progress, "[liteploy] Health check failed: %v. Cleaning up new container.\n", err)
		_ = p.dockerCli.StopContainer(context.Background(), containerID, 5)
		_ = p.dockerCli.RemoveContainer(context.Background(), containerID, true)
		return fmt.Errorf("health check failed: %w", err)
	}
	fmt.Fprintf(progress, "[liteploy] Container is healthy.\n")

	// -------------------------------------------------------------
	// Step 5: ROUTING (Configure Caddy reverse proxy)
	// -------------------------------------------------------------
	dep.Transition(StatusRouting)
	dep.Stage = "routing"
	fmt.Fprintf(progress, "[liteploy] Step 5/5: Configuring reverse proxy routes...\n")

	if len(app.Domains) > 0 && app.Port > 0 {
		// Caddy runs in liteploy-network and resolves the stable alias via Docker DNS.
		// Format: liteploy-{appID}:{containerPort}
		upstream := fmt.Sprintf("liteploy-%s:%d", app.ID, app.Port)
		fmt.Fprintf(progress, "[liteploy] Routing %v -> %s (Docker DNS)\n", app.Domains, upstream)
		if err := p.proxyMgr.UpsertRoute(ctx, &proxy.Route{
			AppID:    app.ID,
			Domains:  app.Domains,
			Upstream: upstream,
		}); err != nil {
			p.logger.Warn("pipeline: caddy route update warning", "app_id", app.ID, "error", err)
			fmt.Fprintf(progress, "[liteploy] Warning: Caddy route update failed: %v\n", err)
		} else {
			fmt.Fprintf(progress, "[liteploy] Proxy routing active.\n")
		}
	} else {
		fmt.Fprintf(progress, "[liteploy] No domains configured or port is 0, skipping proxy routing.\n")
	}

	// -------------------------------------------------------------
	// SUCCESS & Zero-downtime Old Container Cleanup
	// -------------------------------------------------------------
	oldContainerID := app.ContainerID
	dep.OldContainerID = oldContainerID

	// Update application state
	app.Status = application.StatusRunning
	app.ContainerID = containerID
	if newImageID != "" {
		app.ImageID = newImageID
	}
	app.LastDeploymentID = dep.ID
	app.NetworkName = networkName
	_ = p.appSvc.Update(ctx, app)

	// Clean up old container now that the new one is healthy and routing is switched
	if oldContainerID != "" && oldContainerID != containerID {
		fmt.Fprintf(progress, "[liteploy] Stopping and removing previous container: %s\n", oldContainerID[:12])
		_ = p.dockerCli.StopContainer(context.Background(), oldContainerID, 10)
		_ = p.dockerCli.RemoveContainer(context.Background(), oldContainerID, true)
	}

	dep.Succeed()
	fmt.Fprintf(progress, "[liteploy] === Deployment #%s Successful (duration: %.1fs) ===\n", dep.ID, dep.Duration)
	return nil
}

// waitForHealth monitors the container until it is running and responding.
func (p *Pipeline) waitForHealth(ctx context.Context, containerID string, app *application.Application) error {
	deadline := time.Now().Add(p.healthTimeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	httpClient := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if now.After(deadline) {
				return fmt.Errorf("timeout waiting for container readiness after %v", p.healthTimeout)
			}

			info, err := p.dockerCli.InspectContainer(ctx, containerID)
			if err != nil {
				return fmt.Errorf("inspect container: %w", err)
			}

			if info.Status == "exited" || info.Status == "dead" {
				return fmt.Errorf("container exited prematurely with code %d", info.ExitCode)
			}

			if info.Status == "running" {
				// Wait for Docker's native healthcheck if configured
				if info.Health == "healthy" {
					return nil
				}
				if info.Health == "unhealthy" {
					return fmt.Errorf("docker container healthcheck status: unhealthy")
				}

				// If custom HTTP healthcheck is defined, ping it
				if app.HealthcheckPath != "" && app.Port > 0 && info.IPAddress != "" {
					url := fmt.Sprintf("http://%s:%d%s", info.IPAddress, app.Port, app.HealthcheckPath)
					req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
					if err == nil {
						resp, err := httpClient.Do(req)
						if err == nil {
							resp.Body.Close()
							if resp.StatusCode >= 200 && resp.StatusCode < 400 {
								return nil // Healthy!
							}
						}
					}
					continue // Try again on next tick
				}

				// If no health checks defined, just ensure it stays running.
				if info.Health == "" && app.HealthcheckPath == "" {
					return nil
				}
			}
		}
	}
}

// checkTCPDial verifies TCP port connectivity.
func checkTCPDial(hostPort string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", hostPort, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkHTTPGet verifies HTTP response from a health URL.
func checkHTTPGet(url string, timeout time.Duration) bool {
	client := http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
