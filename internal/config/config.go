// Package config handles LITEPLOY configuration from environment variables
// and configuration files. It is intentionally simple — no configuration
// framework is introduced.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all runtime configuration for LITEPLOY.
// Values are sourced from environment variables with sensible defaults.
type Config struct {
	// HTTP server settings
	HTTPAddr string // LITEPLOY_HTTP_ADDR, default ":8080"

	// Data directory root (filesystem persistence)
	DataDir string // LITEPLOY_DATA_DIR, default "/var/lib/liteploy"

	// Logging
	LogLevel string // LITEPLOY_LOG_LEVEL: debug|info|warn|error, default "info"
	LogJSON  bool   // LITEPLOY_LOG_JSON: true|false, default false (text in dev)

	// Docker
	DockerHost string // LITEPLOY_DOCKER_HOST, default "" (uses DOCKER_HOST env or socket)

	// Caddy Admin API
	CaddyAdminAddr string // LITEPLOY_CADDY_ADMIN, default "http://localhost:2019"

	// Session
	SessionSecret string        // LITEPLOY_SESSION_SECRET — REQUIRED in production
	SessionMaxAge time.Duration // LITEPLOY_SESSION_MAX_AGE, default 24h

	// Deployment engine
	MaxConcurrentDeployments int           // LITEPLOY_MAX_DEPLOYMENTS, default 1
	DeploymentTimeout        time.Duration // LITEPLOY_DEPLOYMENT_TIMEOUT, default 30m
	HealthCheckTimeout       time.Duration // LITEPLOY_HEALTH_CHECK_TIMEOUT, default 2m

	// Git
	GitTimeout time.Duration // LITEPLOY_GIT_TIMEOUT, default 10m

	// Development mode (relaxes some security checks for local dev)
	DevMode bool // LITEPLOY_DEV_MODE, default false
}

// Load reads configuration from environment variables.
// Missing required values cause an error; missing optional values use defaults.
func Load() (*Config, error) {
	dataDir := envString("LITEPLOY_DATA_DIR", "/var/lib/liteploy")
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}

	cfg := &Config{
		HTTPAddr:                 envString("LITEPLOY_HTTP_ADDR", ":8080"),
		DataDir:                  dataDir,
		LogLevel:                 envString("LITEPLOY_LOG_LEVEL", "info"),
		LogJSON:                  envBool("LITEPLOY_LOG_JSON", false),
		DockerHost:               envString("LITEPLOY_DOCKER_HOST", ""),
		CaddyAdminAddr:           envString("LITEPLOY_CADDY_ADMIN", "http://localhost:2019"),
		SessionSecret:            os.Getenv("LITEPLOY_SESSION_SECRET"),
		SessionMaxAge:            envDuration("LITEPLOY_SESSION_MAX_AGE", 24*time.Hour),
		MaxConcurrentDeployments: envInt("LITEPLOY_MAX_DEPLOYMENTS", 1),
		DeploymentTimeout:        envDuration("LITEPLOY_DEPLOYMENT_TIMEOUT", 30*time.Minute),
		HealthCheckTimeout:       envDuration("LITEPLOY_HEALTH_CHECK_TIMEOUT", 2*time.Minute),
		GitTimeout:               envDuration("LITEPLOY_GIT_TIMEOUT", 10*time.Minute),
		DevMode:                  envBool("LITEPLOY_DEV_MODE", false),
	}


	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}

// validate checks required fields and constraints.
func (c *Config) validate() error {
	if c.MaxConcurrentDeployments < 1 {
		return fmt.Errorf("LITEPLOY_MAX_DEPLOYMENTS must be >= 1, got %d", c.MaxConcurrentDeployments)
	}
	if c.MaxConcurrentDeployments > 4 {
		// Protect against accidentally flooding a 1 GB VPS with too many concurrent builds.
		// Keep deployment concurrency bounded — image builds can exhaust memory.
		return fmt.Errorf("LITEPLOY_MAX_DEPLOYMENTS > 4 is not allowed on this architecture")
	}
	return nil
}

// envString reads a string env var with a default.
func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool reads a boolean env var with a default.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// envInt reads an integer env var with a default.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// envDuration reads a duration env var with a default.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
