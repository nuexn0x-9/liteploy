// Package proxy manages Caddy reverse proxy configuration via the Caddy Admin API.
//
// LITEPLOY generates Caddy route configurations and applies them at runtime
// through Caddy's JSON configuration API. The Admin API must be restricted
// to localhost — never exposed publicly.
//
// Safety model: before any change, we save the current configuration as
// "last known good". If an update fails, we roll back.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/liteploy/liteploy/internal/docker"
)

// Route describes a single domain-to-container reverse proxy mapping.
type Route struct {
	// AppID identifies which application owns this route.
	AppID string

	// Domains is the list of hostnames routed to this application.
	Domains []string

	// Upstream is the backend address (e.g., "liteploy-app-001:3000").
	// When Caddy runs inside Docker on liteploy-network, Docker DNS resolves
	// container names and aliases directly — no host port required.
	Upstream string
}

const (
	// LiteployNetwork is the shared Docker network for Caddy and all app containers.
	LiteployNetwork = "liteploy-network"
	// CaddyContainerName is the stable name for the managed Caddy container.
	CaddyContainerName = "liteploy-caddy"
	// CaddyImage is the official Caddy image used.
	CaddyImage = "caddy:2-alpine"
)

// EnsureCaddyContainer creates and starts the liteploy-caddy Docker container if it
// does not already exist and is not running. This replaces the host-level caddy.service.
//
// Architecture:
//   - liteploy-caddy joins liteploy-network
//   - :80 and :443 are published to the host for HTTP/HTTPS traffic
//   - :2019 is published to 127.0.0.1 only, so Liteploy (running on host) can
//     still reach the Caddy Admin API without exposing it publicly
//   - Caddy data (certificates) is persisted in dataDir/caddy
//
// All application containers should also join liteploy-network with an alias
// so Caddy can resolve them via Docker DNS (e.g. "liteploy-app-001:3000").
func EnsureCaddyContainer(ctx context.Context, dockerCli docker.Engine, dataDir string) error {
	// 1. Ensure the shared network exists.
	if _, err := dockerCli.EnsureNetwork(ctx, LiteployNetwork); err != nil {
		return fmt.Errorf("proxy: ensure %s network: %w", LiteployNetwork, err)
	}

	// 2. Check if liteploy-caddy exists. If not responding, recreate cleanly.
	containers, err := dockerCli.ListContainers(ctx, docker.ListContainersOptions{
		All:    true,
		Labels: map[string]string{"managed-by": "liteploy", "liteploy.role": "caddy"},
	})
	if err != nil {
		return fmt.Errorf("proxy: list containers for caddy check: %w", err)
	}
	for _, c := range containers {
		if c.Status == "running" {
			// Check if Admin API is responsive on 127.0.0.1:2019
			testMgr := NewManager("http://127.0.0.1:2019", slog.Default())
			if testMgr.Ping(ctx) == nil {
				return nil // Already running and healthy
			}
			// Admin not responsive — remove and recreate with proper config
			_ = dockerCli.StopContainer(ctx, c.ID, 2)
			_ = dockerCli.RemoveContainer(ctx, c.ID, true)
			break
		}
		// Exists but not running — remove so we can recreate fresh
		_ = dockerCli.RemoveContainer(ctx, c.ID, true)
	}

	// 3. Pull the Caddy image (may already be cached).
	if err := dockerCli.PullImage(ctx, CaddyImage, io.Discard); err != nil {
		slog.Default().Warn("proxy: caddy image pull warning", "error", err)
	}

	// 4. Persistent data directory for TLS certificates and Caddyfile.
	caddyDataDir := filepath.Join(dataDir, "caddy")
	_ = os.MkdirAll(caddyDataDir, 0755)

	caddyfilePath := filepath.Join(caddyDataDir, "Caddyfile")
	caddyfileContent := "{\n    admin 0.0.0.0:2019\n}\n:80 {\n    respond \"OK\" 200\n}\n"
	_ = os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644)

	// 5. Create the liteploy-caddy container.
	caddyID, err := dockerCli.CreateContainer(ctx, docker.ContainerSpec{
		Name:  CaddyContainerName,
		Image: CaddyImage,
		Labels: map[string]string{
			"managed-by":    "liteploy",
			"liteploy.role": "caddy",
		},
		Binds: []string{
			caddyDataDir + ":/data",
			caddyfilePath + ":/etc/caddy/Caddyfile",
		},
		// Publish HTTP/HTTPS to all host interfaces, and Admin API strictly to 127.0.0.1
		ExtraPorts: []docker.ExtraPortBinding{
			{ContainerPort: 80, HostPort: 80, HostIP: "0.0.0.0"},
			{ContainerPort: 443, HostPort: 443, HostIP: "0.0.0.0"},
			{ContainerPort: 2019, HostPort: 2019, HostIP: "127.0.0.1"},
		},
		NetworkName:   LiteployNetwork,
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("proxy: create caddy container: %w", err)
	}

	if err := dockerCli.StartContainer(ctx, caddyID); err != nil {
		return fmt.Errorf("proxy: start caddy container (%s): %w", caddyID[:12], err)
	}

	slog.Default().Info("liteploy-caddy container created and started", "container_id", caddyID[:12])

	// Wait up to 5 seconds for Caddy Admin API to be ready
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		testMgr := NewManager("http://127.0.0.1:2019", slog.Default())
		if testMgr.Ping(ctx) == nil {
			break
		}
	}

	return nil
}

// Manager manages Caddy configuration via the Admin API.
type Manager struct {
	adminAddr       string
	logger          *slog.Logger
	httpClient      *http.Client
	routes          map[string]*Route // keyed by AppID
	dashboardDomain string
	dashboardTarget string
	lastKnownGood   map[string]any
}

// NewManager creates a proxy Manager.
// adminAddr should be something like "http://localhost:2019".
// It must never be a public address.
func NewManager(adminAddr string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		adminAddr: adminAddr,
		logger:    logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		routes: make(map[string]*Route),
	}
}

// SetDashboardRoute sets the dashboard domain (e.g. "liteploy.example.com") routing to target (e.g. "127.0.0.1:8080").
func (m *Manager) SetDashboardRoute(ctx context.Context, domain, target string) error {
	m.dashboardDomain = domain
	m.dashboardTarget = target
	return m.apply(ctx)
}

// RemoveDashboardRoute removes the dashboard reverse proxy route.
func (m *Manager) RemoveDashboardRoute(ctx context.Context) error {
	m.dashboardDomain = ""
	m.dashboardTarget = ""
	return m.apply(ctx)
}

// GetDashboardDomain returns the active dashboard domain configured in Caddy.
func (m *Manager) GetDashboardDomain() string {
	return m.dashboardDomain
}

// UpsertRoute adds or updates a route for an application, then applies to Caddy.
func (m *Manager) UpsertRoute(ctx context.Context, route *Route) error {
	if len(route.Domains) == 0 {
		return fmt.Errorf("proxy: route for app %q has no domains", route.AppID)
	}
	if route.Upstream == "" {
		return fmt.Errorf("proxy: route for app %q has no upstream", route.AppID)
	}

	m.routes[route.AppID] = route
	return m.apply(ctx)
}

// RemoveRoute removes an application's route and reloads Caddy.
func (m *Manager) RemoveRoute(ctx context.Context, appID string) error {
	delete(m.routes, appID)
	return m.apply(ctx)
}

// apply generates and pushes the full Caddy config derived from current routes.
// Caddy Admin API supports atomic full config replacement at /load.
func (m *Manager) apply(ctx context.Context) error {
	cfg := m.buildCaddyConfig()

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("proxy: marshal caddy config: %w", err)
	}

	url := m.adminAddr + "/load"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("proxy: create caddy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy: caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		
		// Attempt rollback to last known good config if available
		if m.lastKnownGood != nil {
			if rbData, rbErr := json.Marshal(m.lastKnownGood); rbErr == nil {
				if rbReq, rbReqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rbData)); rbReqErr == nil {
					rbReq.Header.Set("Content-Type", "application/json")
					if rbResp, doErr := m.httpClient.Do(rbReq); doErr == nil {
						rbResp.Body.Close()
						m.logger.Warn("proxy: rolled back Caddy configuration after error")
					}
				}
			}
		}

		return fmt.Errorf("proxy: caddy returned %d: %s", resp.StatusCode, string(body))
	}

	m.lastKnownGood = cfg
	m.logger.Info("caddy routes updated", "route_count", len(m.routes), "dashboard_domain", m.dashboardDomain)
	return nil
}

// Ping checks that the Caddy Admin API is reachable.
func (m *Manager) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.adminAddr+"/config/", nil)
	if err != nil {
		return fmt.Errorf("proxy ping: create request: %w", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy ping: caddy admin API unreachable at %s: %w", m.adminAddr, err)
	}
	defer resp.Body.Close()
	return nil
}

// buildCaddyConfig generates the full Caddy JSON configuration from current routes.
// This uses Caddy's native config format (not Caddyfile).
func (m *Manager) buildCaddyConfig() map[string]any {
	var routes []map[string]any

	// 1. Application routes
	for _, route := range m.routes {
		if len(route.Domains) == 0 || route.Upstream == "" {
			continue
		}

		// Build a Caddy route matching the domains and proxying to the upstream.
		r := map[string]any{
			"match": []map[string]any{
				{"host": route.Domains},
			},
			"handle": []map[string]any{
				{
					"handler": "reverse_proxy",
					"upstreams": []map[string]any{
						{"dial": route.Upstream},
					},
				},
			},
			"terminal": true,
		}
		routes = append(routes, r)
	}

	// 2. LITEPLOY dashboard route (e.g. liteploy.example.com -> 127.0.0.1:8080)
	if m.dashboardDomain != "" && m.dashboardTarget != "" {
		dashRoute := map[string]any{
			"match": []map[string]any{
				{"host": []string{m.dashboardDomain}},
			},
			"handle": []map[string]any{
				{
					"handler": "reverse_proxy",
					"upstreams": []map[string]any{
						{"dial": m.dashboardTarget},
					},
				},
			},
			"terminal": true,
		}
		routes = append(routes, dashRoute)
	}

	return map[string]any{
		"admin": map[string]any{
			"listen": "0.0.0.0:2019",
		},
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"liteploy": map[string]any{
						"listen": []string{":80", ":443"},
						"routes": routes,
					},
				},
			},
		},
	}
}

// GetRoutes returns a copy of the current route map.
func (m *Manager) GetRoutes() map[string]*Route {
	result := make(map[string]*Route, len(m.routes))
	for k, v := range m.routes {
		cp := *v
		result[k] = &cp
	}
	return result
}
