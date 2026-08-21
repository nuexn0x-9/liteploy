// Package system — Reconcile handles startup state reconciliation.
//
// On LITEPLOY restart:
//  1. Incomplete deployments (interrupted during build/start) are marked FAILED.
//  2. Actual Docker container states (identified via liteploy labels) are queried.
//  3. Application runtime states are reconciled against actual Docker status.
package system

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/docker"
)

// Reconciler reconciles filesystem state with Docker state on startup.
type Reconciler struct {
	dockerCli docker.Engine
	appSvc    *application.Service
	depSvc    *deployment.Service
	logger    *slog.Logger
}

// NewReconciler creates a state Reconciler.
func NewReconciler(
	dockerCli docker.Engine,
	appSvc *application.Service,
	depSvc *deployment.Service,
	logger *slog.Logger,
) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		dockerCli: dockerCli,
		appSvc:    appSvc,
		depSvc:    depSvc,
		logger:    logger,
	}
}

// Reconcile performs the startup recovery and state reconciliation.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.logger.Info("starting startup state reconciliation...")

	// 1. Reconcile incomplete deployments
	apps := r.appSvc.List()
	for _, app := range apps {
		deployments := r.depSvc.ListByApp(app.ID)
		for _, dep := range deployments {
			if dep.Status.IsActive() {
				r.logger.Warn("found incomplete deployment from prior run, marking failed",
					"app_id", app.ID,
					"deployment_id", dep.ID,
					"prior_status", dep.Status,
				)
				dep.Fail("recovery", "interrupted by system restart")
				// Update status without re-enqueueing
			}
		}
	}

	// 2. Reconcile Docker containers if Docker is available
	if r.dockerCli == nil {
		r.logger.Warn("docker client unavailable; skipping docker container state reconciliation")
		return nil
	}

	containers, err := r.dockerCli.ListContainers(ctx, docker.ListContainersOptions{
		All: true,
		Labels: map[string]string{
			"liteploy.managed": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("reconcile: list docker containers: %w", err)
	}

	// Map container by app_id
	containersByApp := make(map[string]docker.ContainerSummary)
	for _, c := range containers {
		appID := c.Labels["liteploy.app_id"]
		if appID != "" {
			containersByApp[appID] = c
		}
	}

	// 3. Reconcile application states
	for _, app := range apps {
		ctr, hasContainer := containersByApp[app.ID]
		if !hasContainer {
			if app.Status == application.StatusRunning {
				r.logger.Info("reconciled app state: container not found in docker",
					"app_id", app.ID,
					"old_status", app.Status,
					"new_status", application.StatusStopped,
				)
				_ = r.appSvc.UpdateStatus(app.ID, application.StatusStopped, "")
			}
			continue
		}

		// Inspect container for exact status
		info, err := r.dockerCli.InspectContainer(ctx, ctr.ID)
		if err != nil {
			r.logger.Warn("reconcile: failed to inspect container", "container_id", ctr.ID, "error", err)
			continue
		}

		var targetStatus application.Status
		if info.Status == "running" {
			targetStatus = application.StatusRunning
		} else {
			targetStatus = application.StatusStopped
		}

		if app.Status != targetStatus || app.ContainerID != info.ID {
			r.logger.Info("reconciled app runtime state from docker",
				"app_id", app.ID,
				"status", targetStatus,
				"container_id", info.ID[:12],
			)
			_ = r.appSvc.UpdateStatus(app.ID, targetStatus, info.ID)
		}
	}

	r.logger.Info("startup state reconciliation complete",
		"managed_apps", len(apps),
		"found_containers", len(containers),
	)
	return nil
}
