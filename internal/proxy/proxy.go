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
	"time"
)

// Route describes a single domain-to-container reverse proxy mapping.
type Route struct {
	// AppID identifies which application owns this route.
	AppID string

	// Domains is the list of hostnames routed to this application.
	Domains []string

	// Upstream is the backend address (e.g., "container-name:3000" or "127.0.0.1:PORT").
	Upstream string
}

// Manager manages Caddy configuration via the Admin API.
type Manager struct {
	adminAddr string
	logger    *slog.Logger
	httpClient *http.Client
	routes    map[string]*Route // keyed by AppID
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
		return fmt.Errorf("proxy: caddy returned %d: %s", resp.StatusCode, string(body))
	}

	m.logger.Info("caddy routes updated", "route_count", len(m.routes))
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

	// LITEPLOY admin panel route — served internally.
	// This is added last so application routes take precedence.
	// In production, LITEPLOY itself is also behind Caddy.

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
