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
		switch os.Args[1] {
		case "version", "-v", "--version":
			os.Stdout.WriteString("LITEPLOY version " + system.Version + " (" + system.CommitSHA + ")\n")
			os.Exit(0)
		case "help", "-h", "--help":
			os.Stdout.WriteString("LITEPLOY — Lightweight Docker Deployment Platform for Small VPS\n\nUsage:\n  liteploy [command]\n\nCommands:\n  run (default)  Start the HTTP server and deployment engine\n  version        Print version and exit\n  help           Show this help text\n\nEnvironment Variables:\n  LITEPLOY_ADDR            Server address (default: :8080)\n  LITEPLOY_DATA_DIR        Storage path (default: ./data)\n  LITEPLOY_CADDY_ADMIN     Caddy Admin API (default: http://localhost:2019)\n  LITEPLOY_SESSION_SECRET  HMAC secret key (32+ chars)\n")
			os.Exit(0)
		}
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
