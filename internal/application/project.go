package application

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/liteploy/liteploy/internal/storage"
)

// Project represents a collection of related applications/services sharing a monorepo or domain scope.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	RepoURL     string    `json:"repo_url,omitempty"`
	GitBranch   string    `json:"git_branch,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectService manages Project persistence and retrieval.
type ProjectService struct {
	store *storage.Store
	mu    sync.RWMutex
	items map[string]*Project
}

func NewProjectService(store *storage.Store) (*ProjectService, error) {
	svc := &ProjectService{
		store: store,
		items: make(map[string]*Project),
	}
	if err := svc.loadAll(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *ProjectService) loadAll() error {
	dirs, err := s.store.ListDir("projects")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, id := range dirs {
		var p Project
		rel := filepath.Join("projects", id, "project.json")
		if err := s.store.ReadJSON(rel, &p); err == nil {
			s.items[p.ID] = &p
		}
	}
	return nil
}

func (s *ProjectService) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Project, 0, len(s.items))
	for _, p := range s.items {
		res = append(res, p)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})
	return res
}

func (s *ProjectService) Get(id string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, nil
}

func (s *ProjectService) Create(p *Project) error {
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	rel := filepath.Join("projects", p.ID, "project.json")
	if err := s.store.WriteJSON(rel, p); err != nil {
		return err
	}
	s.items[p.ID] = p
	return nil
}

func (s *ProjectService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	rel := filepath.Join("projects", id)
	if abs, err := s.store.AbsPath(rel); err == nil {
		_ = os.RemoveAll(abs)
	}
	return nil
}
