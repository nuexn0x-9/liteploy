package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Set dev mode so session secret is not required.
	os.Setenv("LITEPLOY_DEV_MODE", "true")
	defer os.Unsetenv("LITEPLOY_DEV_MODE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.DataDir != "/var/lib/liteploy" {
		t.Errorf("DataDir = %q, want /var/lib/liteploy", cfg.DataDir)
	}
	if cfg.MaxConcurrentDeployments != 1 {
		t.Errorf("MaxConcurrentDeployments = %d, want 1", cfg.MaxConcurrentDeployments)
	}
	if cfg.SessionMaxAge != 24*time.Hour {
		t.Errorf("SessionMaxAge = %v, want 24h", cfg.SessionMaxAge)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("LITEPLOY_DEV_MODE", "true")
	os.Setenv("LITEPLOY_HTTP_ADDR", ":9090")
	os.Setenv("LITEPLOY_MAX_DEPLOYMENTS", "2")
	defer func() {
		os.Unsetenv("LITEPLOY_DEV_MODE")
		os.Unsetenv("LITEPLOY_HTTP_ADDR")
		os.Unsetenv("LITEPLOY_MAX_DEPLOYMENTS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.MaxConcurrentDeployments != 2 {
		t.Errorf("MaxConcurrentDeployments = %d, want 2", cfg.MaxConcurrentDeployments)
	}
}

func TestLoad_RequiresSessionSecret(t *testing.T) {
	// Ensure no dev mode and no secret.
	os.Unsetenv("LITEPLOY_DEV_MODE")
	os.Unsetenv("LITEPLOY_SESSION_SECRET")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail without session secret in production mode")
	}
}

func TestLoad_MaxDeploymentsTooHigh(t *testing.T) {
	os.Setenv("LITEPLOY_DEV_MODE", "true")
	os.Setenv("LITEPLOY_MAX_DEPLOYMENTS", "5")
	defer func() {
		os.Unsetenv("LITEPLOY_DEV_MODE")
		os.Unsetenv("LITEPLOY_MAX_DEPLOYMENTS")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail when MAX_DEPLOYMENTS > 4")
	}
}
