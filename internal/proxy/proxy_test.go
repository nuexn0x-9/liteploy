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
	if !ok || len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
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
		Upstream: "127.0.0.1:3000",
	})
	if err != nil {
		t.Fatalf("initial route failed: %v", err)
	}

	// 2. Second apply that fails and triggers rollback
	err = mgr.UpsertRoute(context.Background(), &Route{
		AppID:    "app-002",
		Domains:  []string{"bad.example.com"},
		Upstream: "127.0.0.1:4000",
	})
	if err == nil {
		t.Fatalf("expected error from second apply, got nil")
	}

	// Verify rollback happened (total 3 calls)
	if callCount < 3 {
		t.Fatalf("expected at least 3 calls including rollback, got %d", callCount)
	}
}
