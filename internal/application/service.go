// Package application — Service provides CRUD operations and lifecycle management
// for Applications. It sits between HTTP handlers and the storage layer.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liteploy/liteploy/internal/storage"
)

// Service manages Applications: creating, reading, updating, deleting,
// and listing them. It owns the in-memory cache for fast access.
type Service struct {
	store  *storage.Store
	logger *slog.Logger

	mu   sync.RWMutex
	apps map[string]*Application // keyed by ID
}

// NewService creates a Service and loads existing applications from storage.
func NewService(store *storage.Store, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &Service{
		store:  store,
		logger: logger,
		apps:   make(map[string]*Application),
	}
	if err := svc.loadAll(); err != nil {
		return nil, fmt.Errorf("application service: load: %w", err)
	}
	return svc, nil
}

// Create validates and persists a new Application.
// The caller must supply a non-empty unique ID.
func (s *Service) Create(ctx context.Context, app *Application) error {
	if err := app.Validate(); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.apps[app.ID]; exists {
		return fmt.Errorf("application %q already exists", app.ID)
	}

	now := time.Now().UTC()
	app.CreatedAt = now
	app.UpdatedAt = now
	app.Status = StatusCreated

	// Set managed labels for Docker recovery.
	if app.Labels == nil {
		app.Labels = make(map[string]string)
	}
	for k, v := range app.ManagedLabels() {
		app.Labels[k] = v
	}

	if err := s.persist(app); err != nil {
		return err
	}

	s.apps[app.ID] = app
	s.logger.Info("application created", "app_id", app.ID, "name", app.Name)
	return nil
}

// Get returns a copy of the Application with the given ID.
func (s *Service) Get(id string) (*Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.apps[id]
	if !ok {
		return nil, fmt.Errorf("application %q not found", id)
	}
	cp := *app
	return &cp, nil
}

// List returns all Applications sorted by creation time (oldest first).
func (s *Service) List() []*Application {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Application, 0, len(s.apps))
	for _, app := range s.apps {
		cp := *app
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list
}

// Update validates and persists changes to an existing Application.
// Only mutable fields are allowed to change; ID and CreatedAt are immutable.
func (s *Service) Update(ctx context.Context, app *Application) error {
	if err := app.Validate(); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.apps[app.ID]
	if !ok {
		return fmt.Errorf("application %q not found", app.ID)
	}

	// Preserve immutable fields.
	app.CreatedAt = existing.CreatedAt
	app.UpdatedAt = time.Now().UTC()

	// Ensure managed labels are present.
	if app.Labels == nil {
		app.Labels = make(map[string]string)
	}
	for k, v := range app.ManagedLabels() {
		app.Labels[k] = v
	}

	if err := s.persist(app); err != nil {
		return err
	}

	s.apps[app.ID] = app
	s.logger.Info("application updated", "app_id", app.ID)
	return nil
}

// UpdateStatus updates only the Status and optionally ContainerID of an Application.
// Used by the deployment engine to reflect runtime state changes.
func (s *Service) UpdateStatus(id string, status Status, containerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[id]
	if !ok {
		return fmt.Errorf("application %q not found", id)
	}

	app.Status = status
	if containerID != "" {
		app.ContainerID = containerID
	}
	app.UpdatedAt = time.Now().UTC()

	return s.persist(app)
}

// Delete removes an Application and all its associated data.
// The caller must ensure no deployment is active.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.apps[id]; !ok {
		return fmt.Errorf("application %q not found", id)
	}

	// Remove the application directory from storage.
	appDir := filepath.Join("applications", id)
	absDir, err := s.store.AbsPath(appDir)
	if err != nil {
		return fmt.Errorf("delete application: %w", err)
	}

	if err := os.RemoveAll(absDir); err != nil {
		return fmt.Errorf("delete application %q: remove directory: %w", id, err)
	}

	delete(s.apps, id)
	s.logger.Info("application deleted", "app_id", id)
	return nil
}

// persist writes the Application to storage (called with mu held).
func (s *Service) persist(app *Application) error {
	relPath := filepath.Join("applications", app.ID, "application.json")
	if err := s.store.WriteJSON(relPath, app); err != nil {
		return fmt.Errorf("persist application %q: %w", app.ID, err)
	}
	return nil
}

// loadAll reads all Application records from storage at startup.
func (s *Service) loadAll() error {
	entries, err := s.store.ListDir("applications")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no applications yet, that's fine
		}
		return fmt.Errorf("list applications dir: %w", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry, ".") {
			continue // skip hidden/temp files
		}
		relPath := filepath.Join("applications", entry, "application.json")
		var app Application
		if err := s.store.ReadJSON(relPath, &app); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				s.logger.Warn("application directory exists but application.json missing", "dir", entry)
				continue
			}
			return fmt.Errorf("load application %q: %w", entry, err)
		}
		s.apps[app.ID] = &app
		s.logger.Debug("loaded application", "app_id", app.ID, "name", app.Name)
	}

	s.logger.Info("applications loaded", "count", len(s.apps))
	return nil
}

// Reload re-reads all applications from storage into memory.
func (s *Service) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAll()
}

// GenerateID generates a new unique 3-digit application ID (e.g. "app-001").
// It inspects existing IDs and increments from the highest.
func (s *Service) GenerateID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	max := 0
	for id := range s.apps {
		var n int
		_, _ = fmt.Sscanf(id, "app-%d", &n)
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("app-%03d", max+1)
}

// GetEnv reads environment variables for an application.
func (s *Service) GetEnv(id string) (map[string]string, error) {
	relPath := filepath.Join("applications", id, "env.json")
	var env map[string]string
	err := s.store.ReadJSON(relPath, &env)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	if env == nil {
		env = make(map[string]string)
	}
	return env, nil
}

// SetEnv persists environment variables for an application.
func (s *Service) SetEnv(id string, env map[string]string) error {
	relPath := filepath.Join("applications", id, "env.json")
	return s.store.WriteJSON(relPath, env)
}
