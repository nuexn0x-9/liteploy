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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/liteploy/liteploy/internal/application"
	"github.com/liteploy/liteploy/internal/docker"
)

// Route describes a single domain/path-to-container reverse proxy mapping.
type Route struct {
	// AppID identifies which application owns this route.
	AppID string

	// Domains is the list of hostnames / path patterns routed to this application.
	// Supports plain hostnames ("example.com") and path patterns ("example.com/api/*", "example.com/assets/*").
	Domains []string

	// Upstream is the internal backend address (e.g., "liteploy-app-001:3000").
	// When Caddy runs inside Docker on liteploy-network, Docker DNS resolves
	// container names and aliases directly — no host port required.
	Upstream string
}

// caddyRouteItem is an internal normalized route item for building Caddy config.
type caddyRouteItem struct {
	AppID    string
	Host     string
	Path     string // e.g. "/api/*", "/assets/*", "/*"
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

// Validate checks all currently configured routes for conflicts and validity.
func (m *Manager) Validate() error {
	return ValidateRoutes(m.routes)
}

// ValidateUpstream verifies that upstream is a valid internal address (host:port).
// Rejects loopback addresses (localhost/127.0.0.1) for app containers.
func ValidateUpstream(upstream string) error {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return errors.New("upstream is empty")
	}
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream format %q (expected host:port): %w", upstream, err)
	}
	if host == "" {
		return errors.New("upstream host is empty")
	}
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" {
		return fmt.Errorf("app upstream %q cannot dial loopback/host; must use internal Docker DNS", upstream)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid upstream port %q", portStr)
	}
	return nil
}

// ValidateRoute validates a single Route specification.
func ValidateRoute(r *Route) error {
	if r == nil {
		return errors.New("route is nil")
	}
	if r.AppID == "" {
		return errors.New("route app_id is required")
	}
	if len(r.Domains) == 0 {
		return fmt.Errorf("route for app %q has no domains", r.AppID)
	}
	if err := ValidateUpstream(r.Upstream); err != nil {
		return fmt.Errorf("route for app %q: %w", r.AppID, err)
	}
	for _, d := range r.Domains {
		host, _, err := application.ParseDomainRoute(d)
		if err != nil {
			return fmt.Errorf("route for app %q domain %q: %w", r.AppID, d, err)
		}
		if host == "" {
			return fmt.Errorf("route for app %q domain %q has empty host", r.AppID, d)
		}
	}
	return nil
}

// ValidateRoutes checks a map of routes for format correctness and duplicate collisions across apps.
func ValidateRoutes(routes map[string]*Route) error {
	type routeKey struct {
		host string
		path string
	}
	seen := make(map[routeKey]string) // routeKey -> appID

	for appID, r := range routes {
		if err := ValidateRoute(r); err != nil {
			return err
		}
		for _, d := range r.Domains {
			host, path, err := application.ParseDomainRoute(d)
			if err != nil {
				return fmt.Errorf("app %q has invalid domain %q: %w", appID, d, err)
			}
			k := routeKey{host: host, path: path}
			if existingAppID, exists := seen[k]; exists && existingAppID != appID {
				return fmt.Errorf("route conflict on host %q path %q between app %q and app %q", host, path, existingAppID, appID)
			}
			seen[k] = appID
		}
	}
	return nil
}

// UpsertRoute adds or updates a route for an application, then applies to Caddy.
func (m *Manager) UpsertRoute(ctx context.Context, route *Route) error {
	if err := ValidateRoute(route); err != nil {
		return fmt.Errorf("proxy: invalid route: %w", err)
	}

	// Stash old route in case of validation failure
	oldRoute := m.routes[route.AppID]
	m.routes[route.AppID] = route

	if err := m.Validate(); err != nil {
		if oldRoute != nil {
			m.routes[route.AppID] = oldRoute
		} else {
			delete(m.routes, route.AppID)
		}
		return fmt.Errorf("proxy: route validation failed: %w", err)
	}

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
	if err := m.Validate(); err != nil {
		return fmt.Errorf("proxy: cannot apply invalid routes: %w", err)
	}

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
// Route Ordering Rule:
// 1. Dashboard route (if set)
// 2. Specific path routes (/api/*, /assets/*) ordered by path length descending, then host/path
// 3. Generic / Catch-all routes (/*) ordered by host
// This guarantees that path-specific routes match before catch-all routes in Caddy's sequential evaluation.
func (m *Manager) buildCaddyConfig() map[string]any {
	var specificRoutes []caddyRouteItem
	var catchAllRoutes []caddyRouteItem

	for appID, route := range m.routes {
		if len(route.Domains) == 0 || route.Upstream == "" {
			continue
		}
		for _, d := range route.Domains {
			host, path, err := application.ParseDomainRoute(d)
			if err != nil || host == "" {
				continue
			}
			item := caddyRouteItem{
				AppID:    appID,
				Host:     host,
				Path:     path,
				Upstream: route.Upstream,
			}
			if path == "/*" || path == "" {
				catchAllRoutes = append(catchAllRoutes, item)
			} else {
				specificRoutes = append(specificRoutes, item)
			}
		}
	}

	// Sort specific routes: longest path first, then host, path, appID
	sort.Slice(specificRoutes, func(i, j int) bool {
		if len(specificRoutes[i].Path) != len(specificRoutes[j].Path) {
			return len(specificRoutes[i].Path) > len(specificRoutes[j].Path)
		}
		if specificRoutes[i].Host != specificRoutes[j].Host {
			return specificRoutes[i].Host < specificRoutes[j].Host
		}
		if specificRoutes[i].Path != specificRoutes[j].Path {
			return specificRoutes[i].Path < specificRoutes[j].Path
		}
		return specificRoutes[i].AppID < specificRoutes[j].AppID
	})

	// Sort catch-all routes: host, then appID
	sort.Slice(catchAllRoutes, func(i, j int) bool {
		if catchAllRoutes[i].Host != catchAllRoutes[j].Host {
			return catchAllRoutes[i].Host < catchAllRoutes[j].Host
		}
		return catchAllRoutes[i].AppID < catchAllRoutes[j].AppID
	})

	var routes []map[string]any

	// 1. Dashboard route (e.g. liteploy.example.com -> 127.0.0.1:8080)
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

	// 2. Specific path routes (e.g. /api/*, /assets/*)
	for _, item := range specificRoutes {
		prefix := strings.TrimSuffix(item.Path, "/*")
		var pathPatterns []string
		if prefix != "" && prefix != "/" {
			pathPatterns = []string{item.Path, prefix}
		} else {
			pathPatterns = []string{item.Path}
		}

		r := map[string]any{
			"match": []map[string]any{
				{
					"host": []string{item.Host},
					"path": pathPatterns,
				},
			},
			"handle": []map[string]any{
				{
					"handler": "reverse_proxy",
					"upstreams": []map[string]any{
						{"dial": item.Upstream},
					},
				},
			},
			"terminal": true,
		}
		routes = append(routes, r)
	}

	// 3. Catch-all application routes (e.g. domain.com/*)
	for _, item := range catchAllRoutes {
		r := map[string]any{
			"match": []map[string]any{
				{"host": []string{item.Host}},
			},
			"handle": []map[string]any{
				{
					"handler": "reverse_proxy",
					"upstreams": []map[string]any{
						{"dial": item.Upstream},
					},
				},
			},
			"terminal": true,
		}
		routes = append(routes, r)
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

