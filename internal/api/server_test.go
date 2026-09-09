package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/config"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/system"
)

func TestTemplates_ParseAndRenderAllPages(t *testing.T) {
	s := &Server{}
	tmpl, err := s.loadTemplates()
	if err != nil {
		t.Fatalf("failed to load embedded templates: %v", err)
	}

	mockApp := &application.Application{
		ID:          "app-mock-001",
		Name:        "mock-service",
		ProjectID:   "proj-mock",
		ServiceType: application.ServiceTypeAPI,
		Status:      application.StatusRunning,
		Port:        8080,
		Source: application.Source{
			Type:        application.SourceGit,
			GitURL:      "https://github.com/example/repo",
			GitBranch:   "main",
			ServicePath: "apps/api",
		},
		Domains:   []string{"api.example.com"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockDep := &deployment.Deployment{
		ID:          "dep-001",
		AppID:       mockApp.ID,
		Status:      deployment.StatusSuccess,
		Stage:       "completed",
		TriggeredBy: "manual",
		CommitSHA:   "abc1234",
		Duration:    4.5,
		CreatedAt:   time.Now(),
	}

	testCases := []struct {
		page string
		data map[string]any
	}{
		{
			page: "dashboard.html",
			data: map[string]any{
				"Title":             "Dashboard",
				"ServerIP":          "192.168.1.10",
				"Apps":              []*application.Application{mockApp},
				"Deployments":       []*deployment.Deployment{mockDep},
				"AppCount":          1,
				"DepCount":          1,
				"DomainCount":       1,
				"Stats":             system.SystemStats{MemoryPercent: 25, CPUPercent: 12, DiskPercent: 40, ContainersPercent: 15, ContainersRunning: 2, ContainersTotal: 3},
				"CaddyRunning":      true,
				"PrimaryDomain":     "example.com",
				"QuickAction":       "create",
			},
		},
		{
			page: "applications.html",
			data: map[string]any{
				"Title":        "Applications",
				"ServerIP":     "192.168.1.10",
				"Applications": []*application.Application{mockApp},
			},
		},
		{
			page: "application_new.html",
			data: map[string]any{
				"Title":    "New Application",
				"ServerIP": "192.168.1.10",
			},
		},
		{
			page: "application_detail.html",
			data: map[string]any{
				"Title":       mockApp.Name,
				"ServerIP":    "192.168.1.10",
				"App":         mockApp,
				"Deployments": []*deployment.Deployment{mockDep},
				"Project":     &application.Project{ID: "proj-mock", Name: "Demo Project"},
			},
		},
		{
			page: "application_logs.html",
			data: map[string]any{
				"Title":    "Logs",
				"ServerIP": "192.168.1.10",
				"App":      mockApp,
			},
		},
		{
			page: "deployments.html",
			data: map[string]any{
				"Title":        "Deployments",
				"ServerIP":     "192.168.1.10",
				"Deployments":  []*deployment.Deployment{mockDep},
				"AppNames":     map[string]string{mockApp.ID: mockApp.Name},
				"Applications": []*application.Application{mockApp},
			},
		},
		{
			page: "deployment_detail.html",
			data: map[string]any{
				"Title":      "Deployment #" + mockDep.ID,
				"ServerIP":   "192.168.1.10",
				"App":        mockApp,
				"Deployment": mockDep,
				"BuildLog":   "2026-09-09 Step 1/5 Build OK\n",
			},
		},
		{
			page: "domains.html",
			data: map[string]any{
				"Title":         "Domains",
				"ServerIP":      "192.168.1.10",
				"PrimaryDomain": "example.com",
				"Applications":  []*application.Application{mockApp},
			},
		},
		{
			page: "logs.html",
			data: map[string]any{
				"Title":        "System Logs",
				"ServerIP":     "192.168.1.10",
				"Applications": []*application.Application{mockApp},
			},
		},
		{
			page: "settings.html",
			data: map[string]any{
				"Title":    "Settings",
				"ServerIP": "192.168.1.10",
				"Settings": &system.SystemSettings{ServerIP: "192.168.1.10", PrimaryDomain: "example.com"},
				"Version":  system.Version,
			},
		},
		{
			page: "setup.html",
			data: map[string]any{
				"Title": "Initial Setup",
			},
		},
		{
			page: "setup_domain.html",
			data: map[string]any{
				"Title":    "Setup Domain",
				"ServerIP": "192.168.1.10",
			},
		},
		{
			page: "login.html",
			data: map[string]any{
				"Title": "Login",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.page, func(t *testing.T) {
			var buf bytes.Buffer
			err := tmpl.ExecuteTemplate(&buf, tc.page, tc.data)
			if err != nil {
				t.Fatalf("failed to render template %s: %v", tc.page, err)
			}
			if buf.Len() == 0 {
				t.Errorf("rendered template %s is empty", tc.page)
			}
		})
	}
}

func TestServer_HealthEndpoints(t *testing.T) {
	cfg := &config.Config{
		HTTPAddr: ":8080",
		DataDir:  t.TempDir(),
	}

	s := &Server{
		cfg: cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)

	// Test /health
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /health, got %d", rec.Code)
	}
	var healthResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if healthResp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", healthResp["status"])
	}
	if healthResp["version"] != system.Version {
		t.Fatalf("expected version %s, got %s", system.Version, healthResp["version"])
	}

	// Test /ready
	reqReady := httptest.NewRequest("GET", "/ready", nil)
	recReady := httptest.NewRecorder()
	mux.ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /ready, got %d", recReady.Code)
	}
}
