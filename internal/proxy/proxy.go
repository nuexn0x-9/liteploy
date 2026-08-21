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

	// 2. Check if liteploy-caddy is already running.
	containers, err := dockerCli.ListContainers(ctx, docker.ListContainersOptions{
		All:    true,
		Labels: map[string]string{"managed-by": "liteploy", "liteploy.role": "caddy"},
	})
	if err != nil {
		return fmt.Errorf("proxy: list containers for caddy check: %w", err)
	}
	for _, c := range containers {
		if c.Status == "running" {
			return nil // Already running — nothing to do.
		}
		// Exists but not running — start it.
		if err := dockerCli.StartContainer(ctx, c.ID); err != nil {
			return fmt.Errorf("proxy: start existing caddy container: %w", err)
		}
		return nil
	}

	// 3. Pull the Caddy image (may already be cached).
	if err := dockerCli.PullImage(ctx, CaddyImage, io.Discard); err != nil {
		// Non-fatal: if already pulled, this is a no-op for most implementations.
		// If it truly fails and the image is absent, ContainerCreate will fail below.
		slog.Default().Warn("proxy: caddy image pull warning", "error", err)
	}

	// 4. Persistent data directory for TLS certificates.
	caddyDataDir := filepath.Join(dataDir, "caddy")

	// 5. Create the liteploy-caddy container.
	_, err = dockerCli.CreateContainer(ctx, docker.ContainerSpec{
		Name:  CaddyContainerName,
		Image: CaddyImage,
		// Run caddy with admin API bound to all interfaces inside the container.
		// The admin port (2019) is only published to 127.0.0.1 on the host.
		Cmd: []string{"caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"},
		Env: []string{
			"CADDY_ADMIN=0.0.0.0:2019",
		},
		Labels: map[string]string{
			"managed-by":    "liteploy",
			"liteploy.role": "caddy",
		},
		Binds: []string{
			caddyDataDir + ":/data",
		},
		// No primary ContainerPort — all ports via ExtraPorts.
		ExtraPorts: []docker.ExtraPortBinding{
			{ContainerPort: 80, HostPort: 80, HostIP: "0.0.0.0"},
			{ContainerPort: 443, HostPort: 443, HostIP: "0.0.0.0"},
			// Admin API published only to localhost so Liteploy (on host) can manage Caddy.
			{ContainerPort: 2019, HostPort: 2019, HostIP: "127.0.0.1"},
		},
		NetworkName:   LiteployNetwork,
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		return fmt.Errorf("proxy: create caddy container: %w", err)
	}

	if err := dockerCli.StartContainer(ctx, CaddyContainerName); err != nil {
		return fmt.Errorf("proxy: start caddy container: %w", err)
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
