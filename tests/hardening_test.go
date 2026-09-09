package tests

import (
	"context"
	"os"
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

// -----------------------------------------------------------------------------
// 1. MONOREPO SECURITY & PATH TRAVERSAL TESTS
// -----------------------------------------------------------------------------

func TestHardening_MonorepoPathTraversalValidation(t *testing.T) {
	cases := []struct {
		name        string
		servicePath string
		buildCtx    string
		dockerfile  string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid subpaths",
			servicePath: "services/api",
			buildCtx:    "services/api",
			dockerfile:  "Dockerfile",
			wantErr:     false,
		},
		{
			name:        "path traversal with double dots in service_path",
			servicePath: "../escaped",
			wantErr:     true,
			errMsg:      "illegal path traversal",
		},
		{
			name:        "absolute path in service_path",
			servicePath: "/etc/passwd",
			wantErr:     true,
			errMsg:      "must be a relative path",
		},
		{
			name:     "path traversal in build_context",
			buildCtx: "services/../../escaped",
			wantErr:  true,
			errMsg:   "illegal path traversal",
		},
		{
			name:       "path traversal in dockerfile_path",
			dockerfile: "../../Dockerfile",
			wantErr:    true,
			errMsg:     "illegal path traversal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &application.Application{
				ID:   "app-test-sec",
				Name: "sec-app",
				Source: application.Source{
					Type:           application.SourceGit,
					GitURL:         "https://github.com/example/repo.git",
					ServicePath:    tc.servicePath,
					BuildContext:   tc.buildCtx,
					DockerfilePath: tc.dockerfile,
				},
			}
			err := app.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// 2. SERVICE TYPE ENFORCEMENT RULES
// -----------------------------------------------------------------------------

func TestHardening_ServiceTypeRules(t *testing.T) {
	// Worker cannot have domains
	workerApp := &application.Application{
		ID:          "app-worker-01",
		Name:        "queue-worker",
		ServiceType: application.ServiceTypeWorker,
		Port:        0,
		Domains:     []string{"worker.example.com"},
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/repo.git",
		},
	}
	if err := workerApp.Validate(); err == nil {
		t.Fatal("expected validation error when assigning domain to worker service, got nil")
	} else if !strings.Contains(err.Error(), "cannot have public domain routes") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Internal service cannot have public domains
	internalApp := &application.Application{
		ID:          "app-internal-01",
		Name:        "cache-redis",
		ServiceType: application.ServiceTypeInternal,
		Port:        6379,
		Domains:     []string{"redis.example.com"},
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/repo.git",
		},
	}
	if err := internalApp.Validate(); err == nil {
		t.Fatal("expected validation error when assigning domain to internal service, got nil")
	} else if !strings.Contains(err.Error(), "cannot have public domain routes") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Web service CAN have public domains
	webApp := &application.Application{
		ID:          "app-web-01",
		Name:        "web-portal",
		ServiceType: application.ServiceTypeWeb,
		Port:        3000,
		Domains:     []string{"portal.example.com"},
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/repo.git",
		},
	}
	if err := webApp.Validate(); err != nil {
		t.Fatalf("unexpected error for valid web service: %v", err)
	}
}

// -----------------------------------------------------------------------------
// 3. PROJECT NETWORK ISOLATION & UNIQUE FRIENDLY NAMES
// -----------------------------------------------------------------------------

func TestHardening_ProjectFriendlyNameUniqueness(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.New(dir)
	appSvc, _ := application.NewService(store, nil)

	ctx := context.Background()
	app1 := &application.Application{
		ID:          "app-p1-001",
		Name:        "api-service",
		ProjectID:   "proj-finance",
		ServiceType: application.ServiceTypeAPI,
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/finance.git",
		},
	}
	if err := appSvc.Create(ctx, app1); err != nil {
		t.Fatalf("failed to create app1: %v", err)
	}

	// Another app in the SAME project with SAME name (case-insensitive) must be rejected
	app2 := &application.Application{
		ID:          "app-p1-002",
		Name:        "API-SERVICE", // duplicate in project
		ProjectID:   "proj-finance",
		ServiceType: application.ServiceTypeAPI,
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/finance.git",
		},
	}
	if err := appSvc.Create(ctx, app2); err == nil {
		t.Fatal("expected error creating duplicate friendly name in same project, got nil")
	} else if !strings.Contains(err.Error(), "already exists in project") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// An app in a DIFFERENT project with the SAME name must be allowed
	app3 := &application.Application{
		ID:          "app-p2-001",
		Name:        "api-service",
		ProjectID:   "proj-marketing",
		ServiceType: application.ServiceTypeAPI,
		Source: application.Source{
			Type:   application.SourceGit,
			GitURL: "https://github.com/example/marketing.git",
		},
	}
	if err := appSvc.Create(ctx, app3); err != nil {
		t.Fatalf("failed to create app in different project: %v", err)
	}
}

func TestHardening_MicroserviceNetworkIsolationInPipeline(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.New(dir)
	appSvc, _ := application.NewService(store, nil)

	var capturedSpec docker.ContainerSpec
	mockEngine := &mockDockerEngineWithBuildCapture{
		onCreate: func(spec docker.ContainerSpec) {
			capturedSpec = spec
		},
	}

	pipeline := deployment.NewPipeline(store, mockEngine, nil, appSvc, nil, 2*time.Second, 2*time.Second, 2*time.Second)

	// Case 1: Application with ProjectID should use liteploy-project-{projectID}
	appProject := &application.Application{
		ID:          "app-proj-01",
		Name:        "inventory-api",
		ProjectID:   "ecommerce-prod",
		ServiceType: application.ServiceTypeAPI,
		Port:        8080,
		Domains:     []string{"inventory.example.com"},
		Source: application.Source{
			Type:     application.SourceImage,
			ImageRef: "nginx:alpine",
		},
	}
	_ = appSvc.Create(context.Background(), appProject)

	depProject := &deployment.Deployment{
		ID:        "dep-proj-01",
		AppID:     appProject.ID,
		Status:    deployment.StatusQueued,
		CreatedAt: time.Now().UTC(),
	}

	var logBuf strings.Builder
	if err := pipeline.Execute(context.Background(), depProject, &logBuf); err != nil {
		t.Fatalf("pipeline execution failed: %v", err)
	}

	expectedNet := "liteploy-project-ecommerce-prod"
	if capturedSpec.NetworkName != expectedNet {
		t.Fatalf("expected NetworkName %q, got %q", expectedNet, capturedSpec.NetworkName)
	}

	// Verify canonical alias liteploy-app-{id} and friendly alias inventory-api
	hasCanonical := false
	hasFriendly := false
	for _, a := range capturedSpec.NetworkAliases {
		if a == "liteploy-app-app-proj-01" {
			hasCanonical = true
		}
		if a == "inventory-api" {
			hasFriendly = true
		}
	}
	if !hasCanonical {
		t.Fatalf("missing canonical alias liteploy-app-app-proj-01 in %v", capturedSpec.NetworkAliases)
	}
	if !hasFriendly {
		t.Fatalf("missing friendly alias inventory-api in %v", capturedSpec.NetworkAliases)
	}

	// Case 2: Standalone application (no ProjectID) should fallback to liteploy-network
	appStandalone := &application.Application{
		ID:          "app-standalone-01",
		Name:        "standalone-web",
		ServiceType: application.ServiceTypeWeb,
		Port:        3000,
		Domains:     []string{"web.example.com"},
		Source: application.Source{
			Type:     application.SourceImage,
			ImageRef: "nginx:alpine",
		},
	}
	_ = appSvc.Create(context.Background(), appStandalone)

	depStandalone := &deployment.Deployment{
		ID:        "dep-standalone-01",
		AppID:     appStandalone.ID,
		Status:    deployment.StatusQueued,
		CreatedAt: time.Now().UTC(),
	}

	if err := pipeline.Execute(context.Background(), depStandalone, &logBuf); err != nil {
		t.Fatalf("standalone pipeline failed: %v", err)
	}

	if capturedSpec.NetworkName != proxy.LiteployNetwork {
		t.Fatalf("expected fallback NetworkName %q, got %q", proxy.LiteployNetwork, capturedSpec.NetworkName)
	}
}

// -----------------------------------------------------------------------------
// 4. DEPLOYMENT CONCURRENCY MUTUAL EXCLUSION
// -----------------------------------------------------------------------------

func TestHardening_DeploymentConcurrencyMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.New(dir)

	depSvc, err := deployment.NewService(store, nil, nil, 1)
	if err != nil {
		t.Fatalf("failed to create deployment service: %v", err)
	}
	defer depSvc.Shutdown(1 * time.Second)

	ctx := context.Background()
	appID := "concurrency-test-app"

	// Enqueue first deployment
	dep1, err := depSvc.Enqueue(ctx, appID, "manual")
	if err != nil {
		t.Fatalf("failed to enqueue dep1: %v", err)
	}
	if dep1.Status != deployment.StatusQueued {
		t.Fatalf("expected StatusQueued, got %s", dep1.Status)
	}

	// Immediately try to enqueue a second deployment for the same app
	_, err2 := depSvc.Enqueue(ctx, appID, "webhook")
	if err2 == nil {
		t.Fatal("expected conflict error when enqueuing concurrent deployment for same app, got nil")
	}
	if !strings.Contains(err2.Error(), "already has an active deployment") {
		t.Fatalf("unexpected error message: %v", err2)
	}
}

// -----------------------------------------------------------------------------
// 5. RESOURCE SAFETY: ZERO-LEAK BUILD LOG CHUNK READING
// -----------------------------------------------------------------------------

func TestHardening_ResourceSafety_ReadBuildLogChunk(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.New(dir)

	depSvc, _ := deployment.NewService(store, nil, nil, 1)
	// Create deployment properly via Enqueue
	appID := "log-test-app"
	dep, err := depSvc.Enqueue(context.Background(), appID, "manual")
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}
	depID := dep.ID

	// Wait for background job to finish so the log file is not truncated concurrently
	for i := 0; i < 50; i++ {
		d, err := depSvc.Get(depID)
		if err == nil && !d.Status.IsActive() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	logAbs, _ := store.AbsPath(filepath.Join("applications", appID, "deployments", depID, "build.log"))
	_ = os.MkdirAll(filepath.Dir(logAbs), 0755)
	_ = os.WriteFile(logAbs, []byte("LINE 1\nLINE 2\nLINE 3\n"), 0640)

	// Read initial chunk from offset 0
	chunk1, nextOffset, err := depSvc.ReadBuildLogChunk(depID, 0)
	if err != nil {
		t.Fatalf("failed to read log chunk: %v", err)
	}
	if string(chunk1) != "LINE 1\nLINE 2\nLINE 3\n" {
		t.Fatalf("unexpected chunk content: %q", string(chunk1))
	}
	if nextOffset != int64(len(chunk1)) {
		t.Fatalf("expected nextOffset %d, got %d", len(chunk1), nextOffset)
	}

	// Append more content to log file (simulating progress)
	f, err := os.OpenFile(logAbs, os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		t.Fatalf("failed to open log file for append: %v", err)
	}
	_, _ = f.WriteString("LINE 4\n")
	f.Close()

	// Read from offset: should only read LINE 4 without re-reading previous lines
	chunk2, finalOffset, err := depSvc.ReadBuildLogChunk(depID, nextOffset)
	if err != nil {
		t.Fatalf("failed to read next chunk: %v", err)
	}
	if string(chunk2) != "LINE 4\n" {
		t.Fatalf("expected chunk2 to contain only new bytes 'LINE 4\\n', got %q", string(chunk2))
	}
	if finalOffset <= nextOffset {
		t.Fatalf("finalOffset (%d) should be greater than nextOffset (%d)", finalOffset, nextOffset)
	}
}