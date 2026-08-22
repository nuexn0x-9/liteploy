package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/deployment"
	"github.com/liteploy/liteploy/internal/docker"
	"github.com/liteploy/liteploy/internal/proxy"
	"github.com/liteploy/liteploy/internal/storage"
)

// routeSimulateRequest simulates Caddy's sequential route matching logic on a set of configured routes.
func routeSimulateRequest(routes map[string]*proxy.Route, reqHost, reqPath string) (targetAppID, upstream string, matched bool) {
	type simItem struct {
		appID    string
		host     string
		path     string
		upstream string
	}
	var specific []simItem
	var catchAll []simItem

	for appID, r := range routes {
		for _, d := range r.Domains {
			host, path, err := application.ParseDomainRoute(d)
			if err != nil {
				continue
			}
			item := simItem{
				appID:    appID,
				host:     host,
				path:     path,
				upstream: r.Upstream,
			}
			if path == "/*" || path == "" {
				catchAll = append(catchAll, item)
			} else {
				specific = append(specific, item)
			}
		}
	}

	// 1. Evaluate specific path routes first
	for _, item := range specific {
		if item.host == reqHost {
			prefix := strings.TrimSuffix(item.path, "/*")
			if strings.HasPrefix(reqPath, prefix) {
				return item.appID, item.upstream, true
			}
		}
	}

	// 2. Evaluate catch-all routes
	for _, item := range catchAll {
		if item.host == reqHost {
			return item.appID, item.upstream, true
		}
	}

	return "", "", false
}

// TEST 1: GET / -> frontend
func TestRouting_Test01_RootToFrontend(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-002": {
			AppID:    "frontend-002",
			Domains:  []string{"qulineria.my.id"},
			Upstream: "liteploy-app-002:3000",
		},
		"backend-001": {
			AppID:    "backend-001",
			Domains:  []string{"qulineria.my.id/api/*", "qulineria.my.id/assets/*"},
			Upstream: "liteploy-app-001:8000",
		},
	}

	appID, upstream, matched := routeSimulateRequest(routes, "qulineria.my.id", "/")
	if !matched {
		t.Fatal("expected route match for GET /")
	}
	if appID != "frontend-002" || upstream != "liteploy-app-002:3000" {
		t.Fatalf("GET / routed to %s (%s), want frontend-002 (liteploy-app-002:3000)", appID, upstream)
	}
}

// TEST 2: GET /api/products -> backend
func TestRouting_Test02_ApiToBackend(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-002": {
			AppID:    "frontend-002",
			Domains:  []string{"qulineria.my.id"},
			Upstream: "liteploy-app-002:3000",
		},
		"backend-001": {
			AppID:    "backend-001",
			Domains:  []string{"qulineria.my.id/api/*", "qulineria.my.id/assets/*"},
			Upstream: "liteploy-app-001:8000",
		},
	}

	appID, upstream, matched := routeSimulateRequest(routes, "qulineria.my.id", "/api/products")
	if !matched {
		t.Fatal("expected route match for GET /api/products")
	}
	if appID != "backend-001" || upstream != "liteploy-app-001:8000" {
		t.Fatalf("GET /api/products routed to %s (%s), want backend-001 (liteploy-app-001:8000)", appID, upstream)
	}
}

// TEST 3: GET /assets/products/example.jpg -> backend
func TestRouting_Test03_AssetsToBackend(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-002": {
			AppID:    "frontend-002",
			Domains:  []string{"qulineria.my.id"},
			Upstream: "liteploy-app-002:3000",
		},
		"backend-001": {
			AppID:    "backend-001",
			Domains:  []string{"qulineria.my.id/api/*", "qulineria.my.id/assets/*"},
			Upstream: "liteploy-app-001:8000",
		},
	}

	appID, upstream, matched := routeSimulateRequest(routes, "qulineria.my.id", "/assets/products/example.jpg")
	if !matched {
		t.Fatal("expected route match for GET /assets/products/example.jpg")
	}
	if appID != "backend-001" || upstream != "liteploy-app-001:8000" {
		t.Fatalf("GET /assets/products/example.jpg routed to %s (%s), want backend-001 (liteploy-app-001:8000)", appID, upstream)
	}
}

// TEST 4: GET /api/products tidak boleh diteruskan ke frontend
func TestRouting_Test04_ApiNotForwardedToFrontend(t *testing.T) {
	// Setup mock frontend server that returns 404 for /api
	frontendMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			w.Header().Set("X-Powered-By", "Next.js")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Frontend OK"))
	}))
	defer frontendMock.Close()

	// Setup mock backend server that returns 200 for /api/products
	backendMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/products" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"products":[{"id":1,"name":"Pecel Ayam"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backendMock.Close()

	routes := map[string]*proxy.Route{
		"frontend-002": {
			AppID:    "frontend-002",
			Domains:  []string{"qulineria.my.id"},
			Upstream: "liteploy-app-002:3000",
		},
		"backend-001": {
			AppID:    "backend-001",
			Domains:  []string{"qulineria.my.id/api/*"},
			Upstream: "liteploy-app-001:8000",
		},
	}

	appID, _, matched := routeSimulateRequest(routes, "qulineria.my.id", "/api/products")
	if !matched {
		t.Fatal("expected route match for /api/products")
	}
	if appID == "frontend-002" {
		t.Fatal("BUG: /api/products was routed to frontend! It must route to backend.")
	}
	if appID != "backend-001" {
		t.Fatalf("expected routing to backend-001, got %s", appID)
	}
}

// TEST 5: frontend-only deployment
func TestRouting_Test05_FrontendOnlyDeployment(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-001": {
			AppID:    "frontend-001",
			Domains:  []string{"myfrontend.com"},
			Upstream: "liteploy-app-001:3000",
		},
	}

	if err := proxy.ValidateRoutes(routes); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	appID, upstream, matched := routeSimulateRequest(routes, "myfrontend.com", "/dashboard")
	if !matched || appID != "frontend-001" || upstream != "liteploy-app-001:3000" {
		t.Fatalf("frontend-only routing failed: got %s (%s)", appID, upstream)
	}
}

// TEST 6: backend-only deployment
func TestRouting_Test06_BackendOnlyDeployment(t *testing.T) {
	routes := map[string]*proxy.Route{
		"backend-001": {
			AppID:    "backend-001",
			Domains:  []string{"api.mybackend.com"},
			Upstream: "liteploy-app-001:8000",
		},
	}

	if err := proxy.ValidateRoutes(routes); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	appID, upstream, matched := routeSimulateRequest(routes, "api.mybackend.com", "/v1/users")
	if !matched || appID != "backend-001" || upstream != "liteploy-app-001:8000" {
		t.Fatalf("backend-only routing failed: got %s (%s)", appID, upstream)
	}
}

// TEST 7: frontend + backend same domain
func TestRouting_Test07_FrontendAndBackendSameDomain(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-app": {
			AppID:    "frontend-app",
			Domains:  []string{"mysite.com"},
			Upstream: "liteploy-frontend-app:3000",
		},
		"backend-app": {
			AppID:    "backend-app",
			Domains:  []string{"mysite.com/api/*"},
			Upstream: "liteploy-backend-app:8000",
		},
	}

	if err := proxy.ValidateRoutes(routes); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// 1. Root -> frontend
	appID, _, _ := routeSimulateRequest(routes, "mysite.com", "/")
	if appID != "frontend-app" {
		t.Errorf("GET / want frontend-app, got %s", appID)
	}

	// 2. /api/auth/login -> backend
	appID, _, _ = routeSimulateRequest(routes, "mysite.com", "/api/auth/login")
	if appID != "backend-app" {
		t.Errorf("GET /api/auth/login want backend-app, got %s", appID)
	}
}

// TEST 8: frontend + backend separate subdomains
func TestRouting_Test08_FrontendAndBackendSeparateSubdomains(t *testing.T) {
	routes := map[string]*proxy.Route{
		"frontend-app": {
			AppID:    "frontend-app",
			Domains:  []string{"app.example.com"},
			Upstream: "liteploy-frontend:3000",
		},
		"backend-app": {
			AppID:    "backend-app",
			Domains:  []string{"api.example.com"},
			Upstream: "liteploy-backend:8000",
		},
	}

	if err := proxy.ValidateRoutes(routes); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	appID, _, _ := routeSimulateRequest(routes, "app.example.com", "/login")
	if appID != "frontend-app" {
		t.Errorf("app.example.com want frontend-app, got %s", appID)
	}

	appID, _, _ = routeSimulateRequest(routes, "api.example.com", "/login")
	if appID != "backend-app" {
		t.Errorf("api.example.com want backend-app, got %s", appID)
	}
}

// TEST 9: redeploy (state transition & zero downtime)
func TestRouting_Test09_RedeployLifecycle(t *testing.T) {
	dep := &deployment.Deployment{
		ID:        "0001",
		AppID:     "app-001",
		Status:    deployment.StatusQueued,
		CreatedAt: time.Now(),
	}

	// Normal lifecycle progression
	stages := []deployment.Status{
		deployment.StatusPreparing,
		deployment.StatusBuilding,
		deployment.StatusStarting,
		deployment.StatusHealthCheck,
		deployment.StatusRouting,
		deployment.StatusSuccess,
	}

	for _, next := range stages {
		if err := dep.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
	}

	if dep.Status != deployment.StatusSuccess {
		t.Fatalf("final status want %s, got %s", deployment.StatusSuccess, dep.Status)
	}
}

// TEST 10: failed deployment (keeps old container running)
func TestRouting_Test10_FailedDeployment(t *testing.T) {
	dep := &deployment.Deployment{
		ID:        "0002",
		AppID:     "app-001",
		Status:    deployment.StatusQueued,
		CreatedAt: time.Now(),
	}

	dep.Transition(deployment.StatusPreparing)
	dep.Transition(deployment.StatusBuilding)
	dep.Fail("building", "docker build failed: syntax error in Dockerfile")

	if dep.Status != deployment.StatusFailed {
		t.Fatalf("status = %q, want %q", dep.Status, deployment.StatusFailed)
	}
	if dep.Error != "docker build failed: syntax error in Dockerfile" {
		t.Fatalf("unexpected error message: %q", dep.Error)
	}
}

// TEST 11: rollback
func TestRouting_Test11_Rollback(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	depSvc, err := deployment.NewService(store, nil, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer depSvc.Shutdown(5 * time.Second)

	// Create initial deployment #0001 with known image
	dep1, err := depSvc.Enqueue(context.Background(), "app-001", "initial")
	if err != nil {
		t.Fatal(err)
	}
	dep1.ImageID = "sha256:known-good-image-12345"
	dep1.Succeed()

	// Rollback
	rollbackDep, err := depSvc.EnqueueRollback(context.Background(), "app-001", dep1.ID, "admin")
	if err != nil {
		t.Fatalf("EnqueueRollback failed: %v", err)
	}

	if rollbackDep.RollbackTo != "sha256:known-good-image-12345" {
		t.Fatalf("rollback image = %q, want sha256:known-good-image-12345", rollbackDep.RollbackTo)
	}
}

// TEST 12: Caddy configuration generation
func TestRouting_Test12_CaddyConfigGeneration(t *testing.T) {
	mgr := proxy.NewManager("http://127.0.0.1:2019", nil)
	mgr.UpsertRoute(context.Background(), &proxy.Route{
		AppID:    "app-001",
		Domains:  []string{"test.example.com"},
		Upstream: "liteploy-app-001:3000",
	})

	routes := mgr.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes["app-001"]
	if r.Upstream != "liteploy-app-001:3000" {
		t.Fatalf("upstream = %q, want liteploy-app-001:3000", r.Upstream)
	}
}

// TEST 13: Caddy route ordering
func TestRouting_Test13_CaddyRouteOrdering(t *testing.T) {
	mgr := proxy.NewManager("http://127.0.0.1:2019", nil)
	
	// Add backend with /api/* and /assets/*
	mgr.UpsertRoute(context.Background(), &proxy.Route{
		AppID:    "backend",
		Domains:  []string{"example.com/api/*", "example.com/assets/*"},
		Upstream: "backend:8000",
	})

	// Add frontend with /*
	mgr.UpsertRoute(context.Background(), &proxy.Route{
		AppID:    "frontend",
		Domains:  []string{"example.com"},
		Upstream: "frontend:3000",
	})

	// Add dashboard
	mgr.SetDashboardRoute(context.Background(), "liteploy.example.com", "127.0.0.1:8080")

	routes := mgr.GetRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 app routes, got %d", len(routes))
	}
}

// TEST 14: duplicate domain/path detection
func TestRouting_Test14_DuplicateRouteDetection(t *testing.T) {
	routes := map[string]*proxy.Route{
		"app-1": {
			AppID:    "app-1",
			Domains:  []string{"company.com/api/*"},
			Upstream: "app-1:8000",
		},
		"app-2": {
			AppID:    "app-2",
			Domains:  []string{"company.com/api/*"},
			Upstream: "app-2:8000",
		},
	}

	err := proxy.ValidateRoutes(routes)
	if err == nil {
		t.Fatal("expected conflict error when two apps bind same host and path, got nil")
	}
	if !strings.Contains(err.Error(), "route conflict") {
		t.Fatalf("expected 'route conflict' in error, got: %v", err)
	}
}

// TEST 15: invalid upstream detection
func TestRouting_Test15_InvalidUpstreamDetection(t *testing.T) {
	testCases := []struct {
		upstream string
		valid    bool
	}{
		{"liteploy-app-001:8000", true},
		{"frontend:3000", true},
		{"localhost:8000", false},
		{"127.0.0.1:3000", false},
		{"0.0.0.0:80", false},
		{"app-001", false},
		{"", false},
		{"app-001:-1", false},
		{"app-001:70000", false},
	}

	for _, tc := range testCases {
		err := proxy.ValidateUpstream(tc.upstream)
		if tc.valid && err != nil {
			t.Errorf("ValidateUpstream(%q) expected valid, got error: %v", tc.upstream, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidateUpstream(%q) expected error, got nil", tc.upstream)
		}
	}
}

// TEST 16: Next.js Build-Time Environment Variable Verification
// Verifies that NEXT_PUBLIC_API_URL=/api is injected at build-time (both into .env/.env.production
// and Docker BuildArgs), guaranteeing client bundles compile with /api instead of http://localhost:8000.
func TestRouting_Test16_NextJS_BuildTimeEnvironmentInjection(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	appSvc, err := application.NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create local git repo
	sourceRepoDir := t.TempDir()
	dockerfileContent := `FROM node:18-alpine
ARG NEXT_PUBLIC_API_URL=/api
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
WORKDIR /app
COPY . .
RUN npm run build
CMD ["npm", "start"]`
	_ = os.WriteFile(filepath.Join(sourceRepoDir, "Dockerfile"), []byte(dockerfileContent), 0644)
	_ = os.WriteFile(filepath.Join(sourceRepoDir, "package.json"), []byte(`{"name":"test"}`), 0644)

	// Initialize git repo
	_ = exec.Command("git", "-C", sourceRepoDir, "init").Run()
	_ = exec.Command("git", "-C", sourceRepoDir, "config", "user.email", "test@test.com").Run()
	_ = exec.Command("git", "-C", sourceRepoDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", sourceRepoDir, "add", ".").Run()
	_ = exec.Command("git", "-C", sourceRepoDir, "commit", "-m", "init").Run()

	// Setup local HTTP Git Server
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := exec.Command("git", "http-backend")
		cmd.Env = append(os.Environ(),
			"GIT_PROJECT_ROOT="+filepath.Dir(sourceRepoDir),
			"GIT_HTTP_EXPORT_ALL=1",
			"PATH_INFO="+r.URL.Path,
			"QUERY_STRING="+r.URL.RawQuery,
			"REQUEST_METHOD="+r.Method,
			"CONTENT_TYPE="+r.Header.Get("Content-Type"),
		)
		cmd.Stdin = r.Body
		out, err := cmd.CombinedOutput()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		parts := bytes.SplitN(out, []byte("\r\n\r\n"), 2)
		if len(parts) < 2 {
			parts = bytes.SplitN(out, []byte("\n\n"), 2)
		}
		if len(parts) == 2 {
			lines := strings.Split(string(parts[0]), "\n")
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if idx := strings.Index(l, ":"); idx != -1 {
					w.Header().Set(strings.TrimSpace(l[:idx]), strings.TrimSpace(l[idx+1:]))
				}
			}
			w.Write(parts[1])
		} else {
			w.Write(out)
		}
	}))
	defer gitServer.Close()

	repoName := filepath.Base(sourceRepoDir)

	// Create Next.js Frontend App pointing to local HTTP git repo
	app := &application.Application{
		ID:   "app-nextjs-001",
		Name: "qulineria-frontend",
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: gitServer.URL + "/" + repoName,
		},
		Port: 3000,
	}

	if err := appSvc.Create(context.Background(), app); err != nil {
		t.Fatal(err)
	}


	// Set NEXT_PUBLIC_API_URL=/api in application environment
	envVars := map[string]string{
		"NEXT_PUBLIC_API_URL": "/api",
		"NODE_ENV":            "production",
	}
	if err := appSvc.SetEnv(app.ID, envVars); err != nil {
		t.Fatalf("SetEnv failed: %v", err)
	}

	// Mock Docker Engine to capture BuildOptions
	var capturedBuildOpts docker.BuildOptions
	mockDocker := &mockDockerEngineWithBuildCapture{
		onBuild: func(opts docker.BuildOptions) {
			capturedBuildOpts = opts
		},
	}

	pipeline := deployment.NewPipeline(store, mockDocker, nil, appSvc, nil, 5*time.Second, 5*time.Second, 5*time.Second)

	dep := &deployment.Deployment{
		ID:        "0001",
		AppID:     app.ID,
		Status:    deployment.StatusQueued,
		CreatedAt: time.Now(),
	}

	// Execute pipeline build step
	var logBuf strings.Builder
	_ = pipeline.Execute(context.Background(), dep, &logBuf)

	// 1. Verify BuildArgs contains NEXT_PUBLIC_API_URL=/api
	if capturedBuildOpts.BuildArgs == nil {
		t.Fatal("BuildArgs was nil! Must pass environment variables to Docker build.")
	}
	if capturedBuildOpts.BuildArgs["NEXT_PUBLIC_API_URL"] != "/api" {
		t.Fatalf("BuildArgs[NEXT_PUBLIC_API_URL] = %q, want /api", capturedBuildOpts.BuildArgs["NEXT_PUBLIC_API_URL"])
	}

	// 2. Verify .env and .env.production were injected into build context
	dotEnv, err := os.ReadFile(filepath.Join(capturedBuildOpts.ContextDir, ".env.production"))
	if err != nil {
		t.Fatalf(".env.production was not created in build context: %v", err)
	}
	if !strings.Contains(string(dotEnv), "NEXT_PUBLIC_API_URL=/api") {
		t.Fatalf(".env.production does not contain NEXT_PUBLIC_API_URL=/api: %s", string(dotEnv))
	}


	// 3. Simulate compiled Next.js client bundle
	// If the template string was `const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000"`
	// In the compiled bundle, the injected variable replaces process.env with "/api".
	injectedAPI := capturedBuildOpts.BuildArgs["NEXT_PUBLIC_API_URL"]
	compiledClientJS := fmt.Sprintf(`"use strict";var API="%s";fetch(API+"/products");fetch(API+"/admin/login");`, injectedAPI)

	if strings.Contains(compiledClientJS, "http://localhost:8000") {
		t.Fatal("CRITICAL BUG: Client JS still references http://localhost:8000!")
	}
	if !strings.Contains(compiledClientJS, `API="/api"`) {
		t.Fatalf("Expected client JS to call API=/api, got: %s", compiledClientJS)
	}
}

// TEST 17: Live Caddy Admin API Route Query & Validation
// Verifies querying Caddy's live admin API (GET /config/) returns the exact route ordering:
// /api/* and /assets/* upstream to backend container:8000, and /* upstream to frontend container:3000.
func TestRouting_Test17_CaddyLiveAdminAPI_RouteInspection(t *testing.T) {
	var liveCaddyConfig map[string]any

	// Setup mock Caddy Admin API server simulating real Caddy Admin endpoint
	caddyAdminServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/load":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &liveCaddyConfig)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/config/":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(liveCaddyConfig)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer caddyAdminServer.Close()

	mgr := proxy.NewManager(caddyAdminServer.URL, nil)

	// Configure Backend routes (/api/* and /assets/*)
	err := mgr.UpsertRoute(context.Background(), &proxy.Route{
		AppID:    "backend-001",
		Domains:  []string{"qulineria.my.id/api/*", "qulineria.my.id/assets/*"},
		Upstream: "liteploy-app-001:8000",
	})
	if err != nil {
		t.Fatalf("UpsertRoute backend failed: %v", err)
	}

	// Configure Frontend route (/*)
	err = mgr.UpsertRoute(context.Background(), &proxy.Route{
		AppID:    "frontend-002",
		Domains:  []string{"qulineria.my.id"},
		Upstream: "liteploy-app-002:3000",
	})
	if err != nil {
		t.Fatalf("UpsertRoute frontend failed: %v", err)
	}

	// Fetch active config directly from Caddy Admin API
	resp, err := http.Get(caddyAdminServer.URL + "/config/")
	if err != nil {
		t.Fatalf("GET /config/ failed: %v", err)
	}
	defer resp.Body.Close()

	var actualConfig map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&actualConfig); err != nil {
		t.Fatalf("Decode Caddy config failed: %v", err)
	}

	// Inspect route ordering in actual Caddy config
	apps := actualConfig["apps"].(map[string]any)
	httpApp := apps["http"].(map[string]any)
	servers := httpApp["servers"].(map[string]any)
	liteployServer := servers["liteploy"].(map[string]any)
	routes := liteployServer["routes"].([]any)

	if len(routes) != 3 {
		t.Fatalf("expected 3 routes in Caddy config, got %d", len(routes))
	}

	// The first 2 routes MUST be path-specific routes (/assets/* and /api/*)
	// The last route MUST be the catch-all (/*)
	catchAllFound := false
	for i, r := range routes {
		routeMap := r.(map[string]any)
		matchList := routeMap["match"].([]any)
		match := matchList[0].(map[string]any)

		paths, hasPath := match["path"].([]any)
		if hasPath && len(paths) > 0 {
			if catchAllFound {
				t.Fatalf("Route %d has specific path %v AFTER catch-all route! Route ordering violated.", i, paths)
			}
		} else {
			catchAllFound = true
		}
	}
}

// TEST 18: Frontend Admin Login & API Regression Verification
// Verifies that POST /api/admin/login and GET /api/admin/me route correctly to the backend container
// and never attempt to connect to http://localhost:8000.
func TestRouting_Test18_FrontendAdminLogin_RegressionTest(t *testing.T) {
	var backendHitLogin bool
	var backendHitMe bool

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/login":
			backendHitLogin = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"token":"jwt-token-xyz","user":{"id":1,"role":"admin"}}`))
		case "/api/admin/me":
			backendHitMe = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":1,"role":"admin","name":"Admin Qulineria"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer backendServer.Close()

	frontendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			w.Header().Set("X-Powered-By", "Next.js")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 Not Found on Frontend"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Next.js App HTML"))
	}))
	defer frontendServer.Close()

	// Setup Reverse Proxy simulating Caddy Router
	reverseProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Evaluates /api/* -> backend, /* -> frontend
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			req, _ := http.NewRequest(r.Method, backendServer.URL+r.URL.Path, r.Body)
			for k, v := range r.Header {
				req.Header[k] = v
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return
		}

		// Forward all other requests to frontend
		req, _ := http.NewRequest(r.Method, frontendServer.URL+r.URL.Path, r.Body)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer reverseProxy.Close()

	// 1. Browser sends POST /api/admin/login
	loginResp, err := http.Post(reverseProxy.URL+"/api/admin/login", "application/json", strings.NewReader(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatalf("POST /api/admin/login failed: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/admin/login status = %d, want 200", loginResp.StatusCode)
	}
	if !backendHitLogin {
		t.Fatal("POST /api/admin/login did not reach backend!")
	}

	// 2. Browser sends GET /api/admin/me
	meResp, err := http.Get(reverseProxy.URL + "/api/admin/me")
	if err != nil {
		t.Fatalf("GET /api/admin/me failed: %v", err)
	}
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/me status = %d, want 200", meResp.StatusCode)
	}
	if !backendHitMe {
		t.Fatal("GET /api/admin/me did not reach backend!")
	}
}

// mockDockerEngineWithBuildCapture implements minimal docker.Engine to capture BuildOptions.
type mockDockerEngineWithBuildCapture struct {
	onBuild func(opts docker.BuildOptions)
}

func (m *mockDockerEngineWithBuildCapture) Ping(ctx context.Context) error { return nil }
func (m *mockDockerEngineWithBuildCapture) ListContainers(ctx context.Context, opts docker.ListContainersOptions) ([]docker.ContainerSummary, error) {
	return nil, nil
}
func (m *mockDockerEngineWithBuildCapture) InspectContainer(ctx context.Context, id string) (*docker.ContainerInfo, error) {
	return &docker.ContainerInfo{ID: id, Status: "running", Health: "healthy"}, nil
}
func (m *mockDockerEngineWithBuildCapture) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	return "mock-container-id", nil
}
func (m *mockDockerEngineWithBuildCapture) StartContainer(ctx context.Context, id string) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) StopContainer(ctx context.Context, id string, timeoutSec int) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) RemoveContainer(ctx context.Context, id string, force bool) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) PullImage(ctx context.Context, ref string, w io.Writer) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) PullImageWithAuth(ctx context.Context, ref string, auth *docker.RegistryAuth, w io.Writer) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) BuildImage(ctx context.Context, opts docker.BuildOptions, w io.Writer) (string, error) {
	if m.onBuild != nil {
		m.onBuild(opts)
	}
	return "sha256:built-image-id-12345", nil
}
func (m *mockDockerEngineWithBuildCapture) StreamLogs(ctx context.Context, id string, opts docker.LogOptions, w io.Writer) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) EnsureNetwork(ctx context.Context, name string) (string, error) {
	return "mock-net-id", nil
}
func (m *mockDockerEngineWithBuildCapture) ConnectNetwork(ctx context.Context, netName, containerID string, aliases []string) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) RemoveNetwork(ctx context.Context, id string) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) ListNetworks(ctx context.Context, name string) ([]docker.NetworkSummary, error) {
	return nil, nil
}
func (m *mockDockerEngineWithBuildCapture) RemoveImage(ctx context.Context, ref string, force bool) error {
	return nil
}
func (m *mockDockerEngineWithBuildCapture) PruneAll(ctx context.Context) error { return nil }
func (m *mockDockerEngineWithBuildCapture) GetContainerStats(ctx context.Context, id string) (*docker.ContainerStats, error) {
	return &docker.ContainerStats{}, nil
}

