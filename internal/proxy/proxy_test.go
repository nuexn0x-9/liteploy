package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyRouteGeneration(t *testing.T) {
	mgr := NewManager("http://localhost:2019", nil)

	// Add routes
	mgr.routes["app-001"] = &Route{
		AppID:    "app-001",
		Domains:  []string{"example.com", "www.example.com"},
		Upstream: "app-001:3000",
	}

	cfg := mgr.buildCaddyConfig()
	if cfg == nil {
		t.Fatal("buildCaddyConfig returned nil")
	}

	apps, ok := cfg["apps"].(map[string]any)
	if !ok {
		t.Fatal("missing apps key in caddy config")
	}

	httpApp, ok := apps["http"].(map[string]any)
	if !ok {
		t.Fatal("missing http key in caddy config")
	}

	servers, ok := httpApp["servers"].(map[string]any)
	if !ok {
		t.Fatal("missing servers key in caddy config")
	}

	liteployServer, ok := servers["liteploy"].(map[string]any)
	if !ok {
		t.Fatal("missing liteploy server in caddy config")
	}

	routes, ok := liteployServer["routes"].([]map[string]any)
	if !ok || len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
}


func TestProxyDashboardRoute(t *testing.T) {
	mgr := NewManager("http://localhost:2019", nil)

	// Set dashboard domain
	mgr.dashboardDomain = "liteploy.example.com"
	mgr.dashboardTarget = "127.0.0.1:8080"

	cfg := mgr.buildCaddyConfig()
	apps := cfg["apps"].(map[string]any)
	httpApp := apps["http"].(map[string]any)
	servers := httpApp["servers"].(map[string]any)
	liteployServer := servers["liteploy"].(map[string]any)
	routes := liteployServer["routes"].([]map[string]any)

	if len(routes) != 1 {
		t.Fatalf("expected 1 dashboard route, got %d", len(routes))
	}

	match := routes[0]["match"].([]map[string]any)
	hosts := match[0]["host"].([]string)
	if len(hosts) != 1 || hosts[0] != "liteploy.example.com" {
		t.Fatalf("unexpected host in dashboard route: %v", hosts)
	}

	if mgr.GetDashboardDomain() != "liteploy.example.com" {
		t.Fatalf("expected liteploy.example.com, got %s", mgr.GetDashboardDomain())
	}
}

func TestProxyRollbackOnFailure(t *testing.T) {
	var callCount int
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call succeeds (sets lastKnownGood)
			w.WriteHeader(http.StatusOK)
			return
		}
		if callCount == 2 {
			// Second call fails (triggers rollback)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid config"))
			return
		}
		if callCount == 3 {
			// Rollback call succeeds
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer mockCaddy.Close()

	mgr := NewManager(mockCaddy.URL, nil)

	// 1. Initial good apply
	err := mgr.UpsertRoute(context.Background(), &Route{
		AppID:    "app-001",
		Domains:  []string{"good.example.com"},
		Upstream: "app-001:3000",
	})
	if err != nil {
		t.Fatalf("initial route failed: %v", err)
	}

	// 2. Second apply that fails and triggers rollback
	err = mgr.UpsertRoute(context.Background(), &Route{
		AppID:    "app-002",
		Domains:  []string{"bad.example.com"},
		Upstream: "app-002:4000",
	})
	if err == nil {
		t.Fatalf("expected error from second apply, got nil")
	}

	// Verify rollback happened (total 3 calls)
	if callCount < 3 {
		t.Fatalf("expected at least 3 calls including rollback, got %d", callCount)
	}
}

func TestProxyRouteOrdering(t *testing.T) {
	mgr := NewManager("http://localhost:2019", nil)

	// Frontend: catch-all on qulineria.my.id
	mgr.routes["frontend-002"] = &Route{
		AppID:    "frontend-002",
		Domains:  []string{"qulineria.my.id"},
		Upstream: "liteploy-app-002:3000",
	}

	// Backend: /api/* and /assets/* on qulineria.my.id, plus api.qulineria.my.id
	mgr.routes["backend-001"] = &Route{
		AppID:    "backend-001",
		Domains:  []string{"qulineria.my.id/api/*", "qulineria.my.id/assets/*", "api.qulineria.my.id"},
		Upstream: "liteploy-app-001:8000",
	}

	cfg := mgr.buildCaddyConfig()
	apps := cfg["apps"].(map[string]any)
	httpApp := apps["http"].(map[string]any)
	servers := httpApp["servers"].(map[string]any)
	liteployServer := servers["liteploy"].(map[string]any)
	routes := liteployServer["routes"].([]map[string]any)

	// We expect 4 routes:
	// 1. qulineria.my.id /assets/* -> liteploy-app-001:8000
	// 2. qulineria.my.id /api/*    -> liteploy-app-001:8000
	// 3. api.qulineria.my.id       -> liteploy-app-001:8000
	// 4. qulineria.my.id           -> liteploy-app-002:3000
	if len(routes) != 4 {
		t.Fatalf("expected 4 routes, got %d", len(routes))
	}

	// First two routes MUST have path matchers (specific routes)
	for i := 0; i < 2; i++ {
		match := routes[i]["match"].([]map[string]any)[0]
		paths, ok := match["path"].([]string)
		if !ok || len(paths) == 0 {
			t.Errorf("route %d should be path-specific, but has no path matcher: %v", i, match)
		}
		handle := routes[i]["handle"].([]map[string]any)[0]
		upstreams := handle["upstreams"].([]map[string]any)
		dial := upstreams[0]["dial"].(string)
		if dial != "liteploy-app-001:8000" {
			t.Errorf("route %d dial = %q, want liteploy-app-001:8000", i, dial)
		}
	}

	// Last route for qulineria.my.id MUST be catch-all to frontend:3000
	lastMatch := routes[3]["match"].([]map[string]any)[0]
	if _, hasPath := lastMatch["path"]; hasPath {
		t.Errorf("catch-all route should not have specific path, got %v", lastMatch["path"])
	}
	lastHandle := routes[3]["handle"].([]map[string]any)[0]
	lastDial := lastHandle["upstreams"].([]map[string]any)[0]["dial"].(string)
	if lastDial != "liteploy-app-002:3000" {
		t.Errorf("catch-all route dial = %q, want liteploy-app-002:3000", lastDial)
	}
}

func TestProxyDuplicateRouteDetection(t *testing.T) {
	routes := map[string]*Route{
		"app-001": {
			AppID:    "app-001",
			Domains:  []string{"example.com/api/*"},
			Upstream: "app-001:8000",
		},
		"app-002": {
			AppID:    "app-002",
			Domains:  []string{"example.com/api/*"},
			Upstream: "app-002:8000",
		},
	}

	err := ValidateRoutes(routes)
	if err == nil {
		t.Fatal("expected conflict error for duplicate host/path between apps, got nil")
	}
}

func TestProxyInvalidUpstreamDetection(t *testing.T) {
	invalidUpstreams := []string{
		"",
		"localhost:8000",
		"127.0.0.1:8000",
		"0.0.0.0:8000",
		"app-001:invalidport",
		"app-001:0",
		"app-001:99999",
	}

	for _, u := range invalidUpstreams {
		err := ValidateUpstream(u)
		if err == nil {
			t.Errorf("ValidateUpstream(%q) expected error, got nil", u)
		}
	}

	validUpstreams := []string{
		"liteploy-app-001:8000",
		"app-002:3000",
		"my-backend:80",
	}
	for _, u := range validUpstreams {
		if err := ValidateUpstream(u); err != nil {
			t.Errorf("ValidateUpstream(%q) unexpected error: %v", u, err)
		}
	}
}

