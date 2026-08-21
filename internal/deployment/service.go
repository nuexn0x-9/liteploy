// Package deployment — Service manages deployment queuing, execution, and persistence.
//
// The engine uses a bounded in-process job queue and worker pool.
// Default: 1 concurrent deployment (conservative for 1 GB VPS).
// Concurrency is configurable up to the architecture-mandated cap.
//
// Design: one active deployment per application at a time.
// Different applications may deploy concurrently within the pool.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liteploy/liteploy/internal/storage"
)

// Executor is the interface the deployment service calls to actually build
// and run an application. Implemented by the pipeline package.
type Executor interface {
	Execute(ctx context.Context, dep *Deployment, progress io.Writer) error
}

// Service manages deployments: queuing, executing, persisting, and streaming logs.
type Service struct {
	store    *storage.Store
	logger   *slog.Logger
	executor Executor

	// Bounded job queue — protects against unbounded goroutine creation.
	// Keep deployment concurrency bounded because image builds can exhaust
	// memory on a 1 GB VPS.
	jobQueue chan *job
	workers  int

	mu          sync.RWMutex
	deployments map[string]*Deployment // keyed by deployment ID
	appLocks    map[string]*sync.Mutex // per-application lock

	// cancel functions for active jobs.
	cancelFns map[string]context.CancelFunc

	shutdownCh chan struct{}
	wg         sync.WaitGroup
}

type job struct {
	dep    *Deployment
	cancel context.CancelFunc
}

// NewService creates and starts a deployment Service.
// workers is the maximum number of concurrent deployments.
func NewService(
	store *storage.Store,
	logger *slog.Logger,
	executor Executor,
	workers int,
) (*Service, error) {
	if workers < 1 {
		workers = 1
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Service{
		store:       store,
		logger:      logger,
		executor:    executor,
		jobQueue:    make(chan *job, 64), // buffer up to 64 queued jobs
		workers:     workers,
		deployments: make(map[string]*Deployment),
		appLocks:    make(map[string]*sync.Mutex),
		cancelFns:   make(map[string]context.CancelFunc),
		shutdownCh:  make(chan struct{}),
	}

	if err := s.loadAll(); err != nil {
		return nil, err
	}

	s.startWorkers()
	return s, nil
}

// Enqueue creates a new deployment and adds it to the job queue.
// Returns the deployment ID immediately (non-blocking).
func (s *Service) Enqueue(ctx context.Context, appID, triggeredBy string) (*Deployment, error) {
	depID, err := s.nextDeploymentID(appID)
	if err != nil {
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	dep := &Deployment{
		ID:          depID,
		AppID:       appID,
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
		TriggeredBy: triggeredBy,
	}

	if err := s.persistDeployment(dep); err != nil {
		return nil, fmt.Errorf("enqueue: persist: %w", err)
	}

	s.mu.Lock()
	s.deployments[dep.ID] = dep
	s.mu.Unlock()

	jobCtx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.cancelFns[dep.ID] = cancel
	s.mu.Unlock()

	select {
	case s.jobQueue <- &job{dep: dep, cancel: cancel}:
		s.logger.Info("deployment enqueued", "app_id", appID, "deployment_id", depID)
	default:
		cancel()
		dep.Fail("queue", "deployment queue is full")
		s.persistDeployment(dep)
		return nil, errors.New("deployment queue is full; try again later")
	}

	_ = jobCtx
	return dep, nil
}

// Cancel attempts to cancel an active deployment.
func (s *Service) Cancel(deploymentID string) error {
	s.mu.Lock()
	cancel, ok := s.cancelFns[deploymentID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("deployment %q not found or not active", deploymentID)
	}
	cancel()
	return nil
}

// Get returns a copy of the Deployment with the given ID.
func (s *Service) Get(id string) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dep, ok := s.deployments[id]
	if !ok {
		return nil, fmt.Errorf("deployment %q not found", id)
	}
	cp := *dep
	return &cp, nil
}

// ListByApp returns all deployments for a given application, sorted newest first.
func (s *Service) ListByApp(appID string) []*Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*Deployment
	for _, dep := range s.deployments {
		if dep.AppID == appID {
			cp := *dep
			list = append(list, &cp)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// ListAll returns all deployments across all applications, sorted newest first.
func (s *Service) ListAll() []*Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Deployment, 0, len(s.deployments))
	for _, dep := range s.deployments {
		cp := *dep
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// Shutdown stops accepting new work and waits for active deployments to finish or timeout.
func (s *Service) Shutdown(timeout time.Duration) {
	close(s.shutdownCh)

	// Cancel all active jobs.
	s.mu.Lock()
	for _, cancel := range s.cancelFns {
		cancel()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		s.logger.Warn("deployment shutdown timed out; some deployments may be incomplete")
	}
}

// startWorkers launches the bounded worker goroutines.
func (s *Service) startWorkers() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
}

// worker pulls jobs from the queue and executes them serially.
func (s *Service) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdownCh:
			return
		case j := <-s.jobQueue:
			s.runJob(j)
		}
	}
}

// runJob executes one deployment, acquiring the per-app lock first.
func (s *Service) runJob(j *job) {
	dep := j.dep
	appLock := s.getAppLock(dep.AppID)

	// Acquire the per-application lock to ensure only one deployment
	// runs for this application at a time.
	appLock.Lock()
	defer appLock.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register the cancel function.
	s.mu.Lock()
	s.cancelFns[dep.ID] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.cancelFns, dep.ID)
		s.mu.Unlock()
	}()

	// Open the build log file for streaming.
	logPath, _ := s.buildLogPath(dep)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		s.logger.Error("deployment: failed to open log file", "error", err, "deployment_id", dep.ID)
		dep.Fail("prepare", "failed to open log file")
		s.updateDeployment(dep)
		return
	}
	defer logFile.Close()

	dep.Start()
	dep.Status = StatusPreparing
	s.updateDeployment(dep)

	s.logger.Info("deployment started", "app_id", dep.AppID, "deployment_id", dep.ID)

	// Execute the pipeline. Executor writes progress to logFile.
	if s.executor != nil {
		if err := s.executor.Execute(ctx, dep, logFile); err != nil {
			if ctx.Err() != nil {
				dep.Status = StatusCancelled
				now := time.Now().UTC()
				dep.FinishedAt = &now
				fmt.Fprintf(logFile, "\n[liteploy] [ERROR] Deployment was cancelled or timed out.\n")
			} else {
				dep.Fail("execute", sanitizeError(err))
				fmt.Fprintf(logFile, "\n[liteploy] [ERROR] %s\n", dep.Error)
			}
			s.updateDeployment(dep)
			s.logger.Error("deployment failed",
				"app_id", dep.AppID,
				"deployment_id", dep.ID,
				"error", dep.Error,
			)
			return
		}
	} else {
		// No executor set (Phase 1 bootstrap): just succeed for testing.
		dep.Succeed()
	}

	s.updateDeployment(dep)
	s.PruneOldDeployments(dep.AppID, 10)
	s.logger.Info("deployment completed",
		"app_id", dep.AppID,
		"deployment_id", dep.ID,
		"duration_s", dep.Duration,
	)
}

// PruneOldDeployments retains only the newest maxKeep deployments for an application,
// removing the older deployment metadata, directories, and logs from disk and memory.
func (s *Service) PruneOldDeployments(appID string, maxKeep int) {
	if maxKeep <= 0 {
		return
	}
	deployments := s.ListByApp(appID)
	if len(deployments) <= maxKeep {
		return
	}

	for i := maxKeep; i < len(deployments); i++ {
		oldDep := deployments[i]
		s.mu.Lock()
		delete(s.deployments, oldDep.ID)
		s.mu.Unlock()

		relDir := filepath.Join("applications", appID, "deployments", oldDep.ID)
		if absDir, err := s.store.AbsPath(relDir); err == nil {
			_ = os.RemoveAll(absDir)
		}
	}
}

// ClearFailedDeployments deletes all failed or cancelled deployments for an app.
func (s *Service) ClearFailedDeployments(appID string) int {
	deployments := s.ListByApp(appID)
	deleted := 0
	for _, dep := range deployments {
		if dep.Status == StatusFailed || dep.Status == StatusCancelled {
			s.mu.Lock()
			delete(s.deployments, dep.ID)
			s.mu.Unlock()

			relDir := filepath.Join("applications", appID, "deployments", dep.ID)
			if absDir, err := s.store.AbsPath(relDir); err == nil {
				_ = os.RemoveAll(absDir)
			}
			deleted++
		}
	}
	return deleted
}

// getAppLock returns the per-application mutex, creating it if needed.
func (s *Service) getAppLock(appID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.appLocks[appID]; !ok {
		s.appLocks[appID] = &sync.Mutex{}
	}
	return s.appLocks[appID]
}

// updateDeployment persists the deployment and updates the in-memory copy.
func (s *Service) updateDeployment(dep *Deployment) {
	s.mu.Lock()
	s.deployments[dep.ID] = dep
	s.mu.Unlock()
	if err := s.persistDeployment(dep); err != nil {
		s.logger.Error("failed to persist deployment", "deployment_id", dep.ID, "error", err)
	}
}

// persistDeployment writes the Deployment to storage.
func (s *Service) persistDeployment(dep *Deployment) error {
	relPath := filepath.Join("applications", dep.AppID, "deployments", dep.ID, "meta.json")
	return s.store.WriteJSON(relPath, dep)
}

// buildLogPath returns the path of the build log for a deployment.
func (s *Service) buildLogPath(dep *Deployment) (string, error) {
	logDir := filepath.Join("applications", dep.AppID, "deployments", dep.ID)
	if err := s.store.MkdirAll(logDir); err != nil {
		return "", err
	}
	return s.store.AbsPath(filepath.Join(logDir, "build.log"))
}

// StreamBuildLog streams the build log of a deployment to the given writer.
// It streams the file in bounded chunks; the caller should handle io.EOF.
func (s *Service) StreamBuildLog(deploymentID string, w io.Writer) error {
	s.mu.RLock()
	dep, ok := s.deployments[deploymentID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("deployment %q not found", deploymentID)
	}

	logPath, err := s.buildLogPath(dep)
	if err != nil {
		return err
	}

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no log yet
		}
		return fmt.Errorf("open build log: %w", err)
	}
	defer f.Close()

	// Stream in bounded chunks to avoid large heap allocation.
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(w, f, buf)
	return err
}

// loadAll reads all deployment records from storage at startup.
func (s *Service) loadAll() error {
	appDirs, err := s.store.ListDir("applications")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("deployment service loadAll: %w", err)
	}

	for _, appID := range appDirs {
		if strings.HasPrefix(appID, ".") {
			continue
		}
		deployDir := filepath.Join("applications", appID, "deployments")
		depDirs, err := s.store.ListDir(deployDir)
		if err != nil {
			continue // app has no deployments yet
		}
		for _, depID := range depDirs {
			if strings.HasPrefix(depID, ".") {
				continue
			}
			relPath := filepath.Join("applications", appID, "deployments", depID, "meta.json")
			var dep Deployment
			if err := s.store.ReadJSON(relPath, &dep); err != nil {
				s.logger.Warn("failed to load deployment", "path", relPath, "error", err)
				continue
			}
			s.deployments[dep.ID] = &dep
		}
	}
	return nil
}

// nextDeploymentID generates the next sequential deployment ID for an app.
func (s *Service) nextDeploymentID(appID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	max := 0
	for _, dep := range s.deployments {
		if dep.AppID != appID {
			continue
		}
		var n int
		fmt.Sscanf(dep.ID, "%d", &n)
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("%04d", max+1), nil
}

// sanitizeError produces a safe, non-secret error message for deployment records.
// The caller must ensure err itself contains no secrets; this adds a prefix.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Truncate very long errors to avoid huge persistent messages.
	if len(msg) > 512 {
		msg = msg[:512] + "... (truncated)"
	}
	return msg
}

// EnqueueRollback enqueues a deployment that skips build and rolls back to a specific image.
func (s *Service) EnqueueRollback(ctx context.Context, appID, oldDepID, triggeredBy string) (*Deployment, error) {
	oldDep, err := s.Get(oldDepID)
	if err != nil {
		return nil, fmt.Errorf("rollback target not found: %w", err)
	}
	if oldDep.ImageID == "" {
		return nil, fmt.Errorf("rollback target has no image ID")
	}

	depID, err := s.nextDeploymentID(appID)
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}

	dep := &Deployment{
		ID:          depID,
		AppID:       appID,
		Status:      StatusQueued,
		Stage:       "queued",
		CreatedAt:   time.Now(),
		TriggeredBy: triggeredBy,
		RollbackTo:  oldDep.ImageID,
	}

	if err := s.persistDeployment(dep); err != nil {
		return nil, fmt.Errorf("persist deployment: %w", err)
	}
	s.updateDeployment(dep)

	s.jobQueue <- &job{dep: dep}
	return dep, nil
}