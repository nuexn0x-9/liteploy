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

func TestProxyApplyMockCaddy(t *testing.T) {
	var receivedConfig bool
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/load" && r.Method == http.MethodPost {
			receivedConfig = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockCaddy.Close()

	mgr := NewManager(mockCaddy.URL, nil)
	err := mgr.UpsertRoute(context.Background(), &Route{
		AppID:    "app-001",
		Domains:  []string{"test.local"},
		Upstream: "127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("UpsertRoute failed: %v", err)
	}
	if !receivedConfig {
		t.Error("mock caddy did not receive /load request")
	}
}
