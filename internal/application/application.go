// Package application defines the Application domain model and its persistence.
//
// An Application is the core unit managed by LITEPLOY: a named deployable
// workload with a source (Git / image / Compose), domain bindings, environment
// variables, and a health state.
package application

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SourceType identifies how an Application gets its workload.
type SourceType string

const (
	SourceGit     SourceType = "git"
	SourceImage   SourceType = "image"
	SourceCompose SourceType = "compose"
)

// Status is the runtime health state of an Application.
type Status string

const (
	StatusUnknown   Status = "unknown"
	StatusCreated   Status = "created"
	StatusRunning   Status = "running"
	StatusStopped   Status = "stopped"
	StatusDeploying Status = "deploying"
	StatusFailed    Status = "failed"
)

// Application is the persisted representation of a managed workload.
// Fields are exported for JSON serialization.
type Application struct {
	// ID is a short, human-readable unique identifier (e.g. "app-001").
	// It is used as the directory name under applications/.
	ID string `json:"id"`

	// Name is a human-readable label.
	Name string `json:"name"`

	// CreatedAt is set once on creation.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is refreshed on every mutation.
	UpdatedAt time.Time `json:"updated_at"`

	// Source describes where the application comes from.
	Source Source `json:"source"`

	// Status is the current runtime state. This is derived from Docker state
	// on startup and updated by the deployment engine.
	Status Status `json:"status"`

	// Domains is the list of domain names routed to this Application.
	Domains []string `json:"domains,omitempty"`

	// Port is the container port LITEPLOY routes traffic to.
	Port int `json:"port"`

	// ContainerID is the current Docker container ID (empty if not running).
	ContainerID string `json:"container_id,omitempty"`

	// ImageID is the current Docker image ID.
	ImageID string `json:"image_id,omitempty"`

	// LastDeploymentID is the ID of the most recent deployment.
	LastDeploymentID string `json:"last_deployment_id,omitempty"`

	// HealthcheckPath is an optional HTTP path (e.g. "/health") used by the deployment
	// pipeline to verify the container is ready before switching traffic.
	HealthcheckPath string `json:"healthcheck_path,omitempty"`

	// NetworkName is the Docker network this application is attached to.
	NetworkName string `json:"network_name,omitempty"`

	// Labels is a map of extra Docker labels applied to the container.
	// Always includes "liteploy.app_id" = ID for recovery after restart.
	Labels map[string]string `json:"labels,omitempty"`

	// WebhookSecret is the HMAC secret for this application's webhook endpoint.
	// Stored hashed or as an opaque token — never logged.
	WebhookSecret string `json:"webhook_secret,omitempty"`

	// ResourceLimits optionally caps container resource usage.
	ResourceLimits *ResourceLimits `json:"resource_limits,omitempty"`

	// Volumes defines host-to-container directory mappings for persistent storage.
	Volumes []Volume `json:"volumes,omitempty"`
}

// Volume defines a persistent storage mapping.
type Volume struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
}

// Source describes where an application's workload comes from.
type Source struct {
	Type SourceType `json:"type"`

	// Git source
	GitURL         string `json:"git_url,omitempty"`
	GitBranch      string `json:"git_branch,omitempty"`
	DockerfilePath string `json:"dockerfile_path,omitempty"` // default "Dockerfile"
	GitAuthType    string `json:"git_auth_type,omitempty"`    // "none", "token", "ssh_key"
	GitToken       string `json:"git_token,omitempty"`        // Personal Access Token / password
	GitSSHKey      string `json:"git_ssh_key,omitempty"`      // Private SSH key

	// Image source
	ImageRef     string        `json:"image_ref,omitempty"` // e.g. "nginx:latest"
	RegistryAuth *RegistryAuth `json:"registry_auth,omitempty"`

	// Compose source (raw YAML content stored separately, referenced here)
	ComposeFile string `json:"compose_file,omitempty"` // relative path to compose yaml
}

// RegistryAuth contains credentials for private Docker registries.
type RegistryAuth struct {
	ServerAddress string `json:"server_address,omitempty"` // e.g. "https://index.docker.io/v1/" or "ghcr.io"
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
}

// ResourceLimits caps Docker container resources.
type ResourceLimits struct {
	MemoryMB int64   `json:"memory_mb,omitempty"` // 0 = no limit
	CPUs     float64 `json:"cpus,omitempty"`      // 0 = no limit
}

// appNameRe validates application names: alphanumeric, hyphens, underscores, 1-64 chars.
var appNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// Validate checks Application fields for correctness.
// Call before persisting or deploying.
func (a *Application) Validate() error {
	var errs []string

	if a.ID == "" {
		errs = append(errs, "id is required")
	}

	if a.Name == "" {
		errs = append(errs, "name is required")
	} else if !appNameRe.MatchString(a.Name) {
		errs = append(errs, "name must be 1-64 alphanumeric/hyphen/underscore characters")
	}

	if err := validateSource(a.Source); err != nil {
		errs = append(errs, "source: "+err.Error())
	}

	if a.Port < 0 || a.Port > 65535 {
		errs = append(errs, fmt.Sprintf("port %d is invalid", a.Port))
	}

	for _, d := range a.Domains {
		if err := validateDomain(d); err != nil {
			errs = append(errs, fmt.Sprintf("domain %q: %v", d, err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// ManagedLabels returns the Docker labels that LITEPLOY always applies,
// allowing containers to be identified after a LITEPLOY restart.
func (a *Application) ManagedLabels() map[string]string {
	return map[string]string{
		"liteploy.app_id":   a.ID,
		"liteploy.app_name": a.Name,
		"liteploy.managed":  "true",
	}
}

// validateSource checks that a Source has consistent, valid fields.
func validateSource(s Source) error {
	switch s.Type {
	case SourceGit:
		if s.GitURL == "" {
			return errors.New("git_url is required for git source")
		}
		// Basic URL safety check — no shell metacharacters.
		if strings.ContainsAny(s.GitURL, ";|&`$(){}[]<>\\\"'") {
			return errors.New("git_url contains disallowed characters")
		}
		// Prevent obvious SSRF / local path abuse.
		lower := strings.ToLower(s.GitURL)
		if !strings.HasPrefix(lower, "https://") &&
			!strings.HasPrefix(lower, "http://") &&
			!strings.HasPrefix(lower, "git@") &&
			!strings.HasPrefix(lower, "ssh://") {
			return errors.New("git_url must use https://, http://, git@, or ssh:// scheme")
		}
	case SourceImage:
		if s.ImageRef == "" {
			return errors.New("image_ref is required for image source")
		}
		if strings.ContainsAny(s.ImageRef, ";|&`$()") {
			return errors.New("image_ref contains disallowed characters")
		}
	case SourceCompose:
		// Compose file path is validated when the file is stored.
	case "":
		return errors.New("source type is required")
	default:
		return fmt.Errorf("unknown source type %q", s.Type)
	}
	return nil
}

// domainRe validates domain/subdomain names.
var domainRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// validateDomain checks a single domain name.
func validateDomain(d string) error {
	if len(d) > 253 {
		return errors.New("domain too long (max 253 chars)")
	}
	if !domainRe.MatchString(d) {
		return errors.New("invalid domain format")
	}
	return nil
}
