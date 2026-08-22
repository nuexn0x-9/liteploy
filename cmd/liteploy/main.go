// LITEPLOY — Lightweight Docker Deployment Platform
//
// Single-binary server that manages Docker application deployments on a
// small VPS (target: 1 GB RAM). No database, no Redis, no external queue.
//
// Architecture:
//
//	HTTP server → handlers → services → Docker Engine API
//	                                  → Filesystem (state)
//	                                  → Caddy Admin API (proxy)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/liteploy/liteploy/internal/api"
	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/auth"
	"github.com/liteploy/liteploy/internal/config"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/docker"
	"github.com/liteploy/liteploy/internal/proxy"
	"github.com/liteploy/liteploy/internal/storage"
	"github.com/liteploy/liteploy/internal/system"
	"github.com/liteploy/liteploy/internal/webhook"
)

func main() {
	if len(os.Args) > 1 {
		handleCLI(os.Args[1], os.Args[2:])
	}


	// Load configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		// Can't use slog yet — logger not initialized.
		os.Stderr.WriteString("liteploy: config error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Initialize structured logger.
	logger := system.NewLogger(cfg.LogLevel, cfg.LogJSON)
	slog.SetDefault(logger)

	logger.Info("starting LITEPLOY",
		"version", system.Version,
		"commit", system.CommitSHA,
		"addr", cfg.HTTPAddr,
		"data_dir", cfg.DataDir,
	)

	// Initialize filesystem storage.
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		logger.Error("failed to initialize storage", "error", err, "data_dir", cfg.DataDir)
		os.Exit(1)
	}
	logger.Info("storage initialized", "root", cfg.DataDir)

	// Determine session secret. If not provided via env, persist to a file in dataDir.
	sessionSecret := []byte(cfg.SessionSecret)
	if len(sessionSecret) == 0 {
		secretFile := filepath.Join(cfg.DataDir, ".session_secret")
		if data, err := os.ReadFile(secretFile); err == nil && len(data) >= 32 {
			sessionSecret = data
		} else {
			secret, err := system.GenerateSecret(32)
			if err != nil {
				logger.Error("failed to generate session secret", "error", err)
				os.Exit(1)
			}
			sessionSecret = []byte(secret)
			_ = os.WriteFile(secretFile, sessionSecret, 0600)
			logger.Info("generated persistent session secret", "file", secretFile)
		}
	}

	// Initialize auth service.
	authSvc, err := auth.NewService(sessionSecret, cfg.SessionMaxAge, logger)
	if err != nil {
		logger.Error("failed to initialize auth service", "error", err)
		os.Exit(1)
	}

	userStore, err := auth.NewUserStore(store, logger)
	if err != nil {
		logger.Error("failed to initialize user store", "error", err)
		os.Exit(1)
	}

	// Initialize application service.
	appSvc, err := application.NewService(store, logger)
	if err != nil {
		logger.Error("failed to initialize application service", "error", err)
		os.Exit(1)
	}

	// Initialize Docker client.
	dockerClient, err := docker.NewClient(cfg.DockerHost)
	if err != nil {
		logger.Warn("failed to initialize Docker client (Docker features will be unavailable)", "error", err)
		// Don't exit — LITEPLOY can still start without Docker (e.g., for UI browsing).
		// Docker is required for actual deployments.
	} else {
		// Verify Docker connectivity.
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := dockerClient.Ping(pingCtx); err != nil {
			logger.Warn("Docker daemon not reachable", "error", err)
		} else {
			logger.Info("Docker connected")
		}
		cancel()
	}

	// Initialize proxy manager.
	proxyMgr := proxy.NewManager(cfg.CaddyAdminAddr, logger)

	// Initialize deployment pipeline executor.
	pipeline := deployment.NewPipeline(
		store,
		dockerClient,
		proxyMgr,
		appSvc,
		logger,
		cfg.GitTimeout,
		cfg.DeploymentTimeout,
		cfg.HealthCheckTimeout,
	)

	// Initialize deployment service with pipeline executor.
	depSvc, err := deployment.NewService(store, logger, pipeline, cfg.MaxConcurrentDeployments)
	if err != nil {
		logger.Error("failed to initialize deployment service", "error", err)
		os.Exit(1)
	}

	// Initialize webhook handler.
	webhookEnqueuer := &webhookEnqueuerAdapter{depSvc: depSvc}
	webhookHandler := webhook.NewHandler(logger, webhookEnqueuer, func(appID string) (string, bool) {
		app, err := appSvc.Get(appID)
		if err != nil {
			return "", false
		}
		if app.WebhookSecret == "" {
			return "", false
		}
		return app.WebhookSecret, true
	})

	// Initialize settings service.
	settingsSvc, err := system.NewSettingsService(store)
	if err != nil {
		logger.Error("failed to initialize settings service", "error", err)
		os.Exit(1)
	}

	// Initialize HTTP server.
	srv, err := api.NewServer(cfg, logger, appSvc, depSvc, authSvc, userStore, settingsSvc, proxyMgr, dockerClient)
	if err != nil {
		logger.Error("failed to initialize HTTP server", "error", err)
		os.Exit(1)
	}

	// Ensure liteploy-caddy Docker container is running.
	// This replaces the host-level caddy.service entirely.
	if dockerClient != nil {
		logger.Info("ensuring caddy container is running")
		if err := proxy.EnsureCaddyContainer(context.Background(), dockerClient, cfg.DataDir); err != nil {
			logger.Warn("caddy container setup warning — proxy may not be available", "error", err)
		} else {
			logger.Info("caddy container ready", "network", proxy.LiteployNetwork)
		}
	}

	// Synchronize routing state to proxy on boot using stable Docker DNS aliases.
	for _, app := range appSvc.List() {
		if app.Status == application.StatusRunning && len(app.Domains) > 0 && app.Port > 0 {
			if app.ContainerID != "" && dockerClient != nil {
				// Ensure existing container is attached to liteploy-network with its stable alias
				alias := fmt.Sprintf("liteploy-%s", app.ID)
				_ = dockerClient.ConnectNetwork(context.Background(), proxy.LiteployNetwork, app.ContainerID, []string{alias})
			}
			// liteploy-{appID} is the stable Docker DNS alias set on each container.
			upstream := fmt.Sprintf("liteploy-%s:%d", app.ID, app.Port)
			_ = proxyMgr.UpsertRoute(context.Background(), &proxy.Route{
				AppID:    app.ID,
				Domains:  app.Domains,
				Upstream: upstream,
			})
		}
	}

	// Synchronize dashboard route to proxy on boot if configured.
	sysSettings := settingsSvc.Get()
	if sysSettings.PrimaryDomain != "" && sysSettings.DashboardDomain != "" && sysSettings.HTTPSEnabled {
		target := cfg.HTTPAddr
		if strings.HasPrefix(target, ":") {
			target = "127.0.0.1" + target
		}
		_ = proxyMgr.SetDashboardRoute(context.Background(), sysSettings.DashboardDomain, target)
	}

	srv.SetWebhookHandler(webhookHandler)

	// Perform startup state reconciliation.
	reconciler := system.NewReconciler(dockerClient, appSvc, depSvc, logger)
	reconcileCtx, cancelReconcile := context.WithTimeout(context.Background(), 15*time.Second)
	if err := reconciler.Reconcile(reconcileCtx); err != nil {
		logger.Warn("startup state reconciliation encountered non-fatal error", "error", err)
	}
	cancelReconcile()

	// Start HTTP server in a goroutine.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for OS signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("HTTP server error", "error", err)
	case sig := <-quit:
		logger.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown.
	logger.Info("shutting down...")

	// 1. Stop accepting new deployments.
	depSvc.Shutdown(30 * time.Second)

	// 2. Shut down HTTP server.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	// 3. Close Docker client.
	if dockerClient != nil {
		dockerClient.Close()
	}

	logger.Info("shutdown complete")
}

// webhookEnqueuerAdapter adapts the deployment.Service to the webhook.Enqueuer interface.
type webhookEnqueuerAdapter struct {
	depSvc *deployment.Service
}

func (a *webhookEnqueuerAdapter) Enqueue(appID, branch, triggeredBy string) error {
	_, err := a.depSvc.Enqueue(context.Background(), appID, triggeredBy)
	return err
}

func handleCLI(cmd string, args []string) {
	switch cmd {
	case "version", "-v", "--version":
		os.Stdout.WriteString("LITEPLOY version " + system.Version + " (" + system.CommitSHA + ")\n")
		os.Exit(0)
	case "help", "-h", "--help":
		printHelp()
		os.Exit(0)
	case "status":
		runCLIStatus(args)
		os.Exit(0)
	case "logs":
		runCLILogs(args)
		os.Exit(0)
	case "deploy":
		runCLIDeploy(args)
		os.Exit(0)
	case "run":
		// Normal server run
		return
	default:
		if strings.HasPrefix(cmd, "-") {
			printHelp()
			os.Exit(1)
		}
	}
}

func printHelp() {
	os.Stdout.WriteString(`LITEPLOY — Lightweight Docker Deployment Platform for Small VPS

Usage:
  liteploy [command] [arguments]

Commands:
  run (default)        Start the HTTP server and deployment engine
  status [app-id]      Show status of all applications or a specific application
  deploy <app-id>      Trigger deployment for an application
  logs <app-id>        View latest build and deployment logs for an application
  version              Print version and exit
  help                 Show this help text

Environment Variables:
  LITEPLOY_ADDR            Server address (default: :8080)
  LITEPLOY_DATA_DIR        Storage path (default: ./data)
  LITEPLOY_CADDY_ADMIN     Caddy Admin API (default: http://localhost:2019)
  LITEPLOY_SESSION_SECRET  HMAC secret key (32+ chars)
`)
}

func runCLIStatus(args []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading storage: %v\n", err)
		os.Exit(1)
	}
	appSvc, err := application.NewService(store, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading applications: %v\n", err)
		os.Exit(1)
	}

	if len(args) > 0 {
		appID := args[0]
		app, err := appSvc.Get(appID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Application %q not found\n", appID)
			os.Exit(1)
		}
		fmt.Printf("ID:          %s\n", app.ID)
		fmt.Printf("Name:        %s\n", app.Name)
		fmt.Printf("Status:      %s\n", app.Status)
		fmt.Printf("Port:        %d\n", app.Port)
		fmt.Printf("Domains:     %s\n", strings.Join(app.Domains, ", "))
		fmt.Printf("Container:   %s\n", app.ContainerID)
		fmt.Printf("Image:       %s\n", app.ImageID)
		fmt.Printf("Source:      %s\n", app.Source.Type)
		return
	}

	apps := appSvc.List()
	if len(apps) == 0 {
		fmt.Println("No applications found.")
		return
	}

	fmt.Printf("%-10s %-20s %-12s %-8s %-30s\n", "APP ID", "NAME", "STATUS", "PORT", "DOMAINS")
	fmt.Println(strings.Repeat("-", 85))
	for _, a := range apps {
		doms := strings.Join(a.Domains, ", ")
		if len(doms) > 30 {
			doms = doms[:27] + "..."
		}
		fmt.Printf("%-10s %-20s %-12s %-8d %-30s\n", a.ID, a.Name, a.Status, a.Port, doms)
	}
}

func runCLIDeploy(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: liteploy deploy <app-id>")
		os.Exit(1)
	}
	appID := args[0]

	// Try active server HTTP API first
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:8080/api/applications/%s/deploy", appID), "application/json", nil)
	if err == nil && resp.StatusCode == http.StatusAccepted {
		fmt.Printf("[liteploy] Deployment enqueued for %s via active server.\n", appID)
		resp.Body.Close()
		return
	}
	if resp != nil {
		resp.Body.Close()
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading storage: %v\n", err)
		os.Exit(1)
	}
	appSvc, err := application.NewService(store, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading applications: %v\n", err)
		os.Exit(1)
	}
	app, err := appSvc.Get(appID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Application %q not found\n", appID)
		os.Exit(1)
	}

	dockerCli, err := docker.NewClient(cfg.DockerHost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Docker: %v\n", err)
		os.Exit(1)
	}
	defer dockerCli.Close()

	proxyMgr := proxy.NewManager(cfg.CaddyAdminAddr, nil)
	pipeline := deployment.NewPipeline(store, dockerCli, proxyMgr, appSvc, nil, cfg.GitTimeout, cfg.DeploymentTimeout, cfg.HealthCheckTimeout)
	depSvc, err := deployment.NewService(store, nil, pipeline, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing deployment service: %v\n", err)
		os.Exit(1)
	}

	dep, err := depSvc.Enqueue(context.Background(), app.ID, "cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enqueuing deployment: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[liteploy] Deployment #%s started for %s...\n", dep.ID, app.Name)
	time.Sleep(3 * time.Second)
	depSvc.Shutdown(30 * time.Second)
}

func runCLILogs(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: liteploy logs <app-id>")
		os.Exit(1)
	}
	appID := args[0]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading storage: %v\n", err)
		os.Exit(1)
	}
	depSvc, err := deployment.NewService(store, nil, nil, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading deployments: %v\n", err)
		os.Exit(1)
	}

	deps := depSvc.ListByApp(appID)
	if len(deps) == 0 {
		fmt.Printf("No deployments found for application %q\n", appID)
		return
	}

	latest := deps[0]
	fmt.Printf("=== Latest Deployment #%s (%s) ===\n", latest.ID, latest.Status)
	_ = depSvc.StreamBuildLog(latest.ID, os.Stdout)
}

