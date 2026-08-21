package tests

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/liteploy/liteploy/internal/api"
	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/auth"
	"github.com/liteploy/liteploy/internal/config"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/proxy"
	"github.com/liteploy/liteploy/internal/storage"
)

func setupTestServer(t *testing.T) (*httptest.Server, *storage.Store) {
	t.Helper()
	dataDir := t.TempDir()

	store, err := storage.New(dataDir)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	cfg := &config.Config{
		HTTPAddr:                 ":0",
		DataDir:                  dataDir,
		LogLevel:                 "debug",
		SessionSecret:            "12345678901234567890123456789012",
		SessionMaxAge:            time.Hour,
		MaxConcurrentDeployments: 1,
		DeploymentTimeout:        5 * time.Minute,
		HealthCheckTimeout:       30 * time.Second,
		GitTimeout:               2 * time.Minute,
		DevMode:                  true,
	}

	authSvc, err := auth.NewService([]byte(cfg.SessionSecret), cfg.SessionMaxAge, nil)
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}

	userStore, err := auth.NewUserStore(store, nil)
	if err != nil {
		t.Fatalf("auth.NewUserStore: %v", err)
	}

	appSvc, err := application.NewService(store, nil)
	if err != nil {
		t.Fatalf("application.NewService: %v", err)
	}

	proxyMgr := proxy.NewManager("http://127.0.0.1:2019", nil)
	depSvc, err := deployment.NewService(store, nil, nil, 1)
	if err != nil {
		t.Fatalf("deployment.NewService: %v", err)
	}

	srv, err := api.NewServer(cfg, nil, appSvc, depSvc, authSvc, userStore, proxyMgr, nil)
	if err != nil {
		t.Fatalf("api.NewServer: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	return ts, store
}

func TestE2E_FullLifecycle(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // follow redirects
		},
	}

	// 1. Health check
	resp, err := client.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Setup admin user
	setupForm := url.Values{
		"username":         {"admin"},
		"password":         {"adminpassword123"},
		"confirm_password": {"adminpassword123"},
	}
	resp, err = client.PostForm(ts.URL+"/setup", setupForm)
	if err != nil {
		t.Fatalf("POST /setup: %v", err)
	}
	resp.Body.Close()

	// 3. Login
	loginForm := url.Values{
		"username": {"admin"},
		"password": {"adminpassword123"},
	}
	resp, err = client.PostForm(ts.URL+"/login", loginForm)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /login status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Access dashboard
	resp, err = client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Create application via API
	appJSON := `{"name":"qulineria-api","port":8080,"source":{"type":"image","image_ref":"nginx:alpine"}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/applications", strings.NewReader(appJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/applications: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /api/applications status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. List applications via API
	resp, err = client.Get(ts.URL + "/api/applications")
	if err != nil {
		t.Fatalf("GET /api/applications: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/applications status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// 7. Trigger deployment via API
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/applications/app-001/deploy", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/applications/app-001/deploy: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("deploy status = %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()

	// 8. View deployment
	resp, err = client.Get(ts.URL + "/deployments/0001")
	if err != nil {
		t.Fatalf("GET /deployments/0001: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /deployments/0001 status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
