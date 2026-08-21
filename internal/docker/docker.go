// Package docker provides a clean interface over the Docker Engine API.
//
// All Docker operations are isolated here. The rest of LITEPLOY depends only
// on the Engine interface, not on Docker SDK types directly.
//
// Docker socket access is equivalent to root on the host — treat it with care.
package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/go-connections/nat"
)

// Engine defines the minimal Docker operations required by LITEPLOY.
// Defined at the consumer side so the implementation can be replaced in tests.
type Engine interface {
	// Ping verifies Docker daemon connectivity.
	Ping(ctx context.Context) error

	// ListContainers returns containers matching the given filters.
	ListContainers(ctx context.Context, opts ListContainersOptions) ([]ContainerSummary, error)

	// InspectContainer returns detailed info for a container.
	InspectContainer(ctx context.Context, id string) (*ContainerInfo, error)

	// CreateContainer creates a new container from the given spec.
	CreateContainer(ctx context.Context, spec ContainerSpec) (string, error)

	// StartContainer starts a stopped container.
	StartContainer(ctx context.Context, id string) error

	// StopContainer stops a running container with a timeout.
	StopContainer(ctx context.Context, id string, timeoutSec int) error

	// RemoveContainer removes a container (must be stopped).
	RemoveContainer(ctx context.Context, id string, force bool) error

	// PullImage pulls an image from a registry.
	PullImage(ctx context.Context, ref string, w io.Writer) error

	// PullImageWithAuth pulls an image with optional private registry credentials.
	PullImageWithAuth(ctx context.Context, ref string, auth *RegistryAuth, w io.Writer) error

	// BuildImage builds an image from a build context directory.
	BuildImage(ctx context.Context, opts BuildOptions, w io.Writer) (string, error)

	// StreamLogs streams container logs to w.
	StreamLogs(ctx context.Context, id string, opts LogOptions, w io.Writer) error

	// EnsureNetwork creates the network if it does not exist, returns network ID.
	EnsureNetwork(ctx context.Context, name string) (string, error)

	// ConnectNetwork connects a running container to a network with optional aliases.
	ConnectNetwork(ctx context.Context, netName, containerID string, aliases []string) error

	// RemoveNetwork removes a Docker network.
	RemoveNetwork(ctx context.Context, id string) error

	// ListNetworks returns networks matching the given filter.
	ListNetworks(ctx context.Context, name string) ([]NetworkSummary, error)

	// RemoveImage removes an image by reference.
	RemoveImage(ctx context.Context, ref string, force bool) error

	// PruneAll performs a system prune (containers, networks, images, build cache).
	PruneAll(ctx context.Context) error

	// GetContainerStats retrieves a single snapshot of container CPU/RAM usage.
	GetContainerStats(ctx context.Context, id string) (*ContainerStats, error)
}

// --- Domain types ----

// ContainerSummary is a lightweight container description.
type ContainerSummary struct {
	ID     string
	Name   string // without leading /
	Image  string
	Status string // Docker status string (e.g., "running", "exited")
	Labels map[string]string
	Ports  []PortBinding
}

// PortBinding maps a container port to a host port.
type PortBinding struct {
	ContainerPort int
	HostPort      int
}

// ContainerInfo holds detailed container state.
type ContainerInfo struct {
	ID      string
	Name    string
	Status  string // "running", "exited", etc.
	ExitCode int
	Image   string
	Labels    map[string]string
	Ports     []PortBinding
	Health    string // "healthy", "unhealthy", "starting", "" (no healthcheck)
	IPAddress string // internal container IP
}

// ContainerSpec defines a container to create.
type ContainerSpec struct {
	Name            string
	Image           string
	Cmd             []string   // optional override of image CMD
	Env             []string   // KEY=VALUE
	Labels          map[string]string
	Binds           []string   // "/host/path:/container/path"
	ContainerPort   int
	HostPort        int        // 0 = no host port binding
	HostIP          string     // bind host IP for port publishing, default "0.0.0.0"
	ExtraPorts      []ExtraPortBinding // additional host-published ports (e.g. for Caddy :80/:443)
	NetworkName     string
	NetworkAliases  []string   // DNS aliases within NetworkName (Docker internal DNS)
	MemoryMB        int64      // 0 = no limit
	CPUs            float64    // 0 = no limit
	RestartPolicy   string     // "no", "always", "on-failure", "unless-stopped"
}

// ExtraPortBinding defines an additional host-published port for a container.
type ExtraPortBinding struct {
	ContainerPort int
	HostPort      int
	HostIP        string  // "" = "0.0.0.0"
	Protocol      string  // "tcp" or "udp", default "tcp"
}

// ListContainersOptions filters for ListContainers.
type ListContainersOptions struct {
	All    bool
	Labels map[string]string // filter by these labels
}

// BuildOptions configures an image build.
type BuildOptions struct {
	ContextDir     string // absolute path to build context
	DockerfilePath string // relative to context dir, default "Dockerfile"
	Tags           []string
	BuildArgs      map[string]string
	NoCache        bool
}

// LogOptions configures log streaming.
type LogOptions struct {
	Follow     bool
	Tail       string // "all" or a number
	Timestamps bool
}

// NetworkSummary is a lightweight network description.
type NetworkSummary struct {
	ID   string
	Name string
}

// ContainerStats holds memory and CPU usage.
type ContainerStats struct {
	MemoryUsageBytes int64
	MemoryLimitBytes int64
	CPUPercent       float64
}

// Client is the production Docker Engine client.
type Client struct {
	cli *dockerclient.Client
}

// NewClient creates a Client using the default Docker socket or DOCKER_HOST env.
// If host is empty, the Docker SDK's default (DOCKER_HOST or /var/run/docker.sock) is used.
func NewClient(host string) (*Client, error) {
	var opts []dockerclient.Opt
	if host != "" {
		opts = append(opts, dockerclient.WithHost(host))
	}
	// Negotiate API version with the daemon for forward compatibility.
	opts = append(opts, dockerclient.WithAPIVersionNegotiation())

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker: create client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping verifies Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}
	return nil
}

// ListContainers returns containers matching the given options.
func (c *Client) ListContainers(ctx context.Context, opts ListContainersOptions) ([]ContainerSummary, error) {
	f := filters.NewArgs()
	for k, v := range opts.Labels {
		f.Add("label", k+"="+v)
	}

	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     opts.All,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("docker list containers: %w", err)
	}

	result := make([]ContainerSummary, 0, len(ctrs))
	for _, ctr := range ctrs {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}
		result = append(result, ContainerSummary{
			ID:     ctr.ID,
			Name:   name,
			Image:  ctr.Image,
			Status: ctr.Status,
			Labels: ctr.Labels,
		})
	}
	return result, nil
}

// InspectContainer returns detailed container info.
func (c *Client) InspectContainer(ctx context.Context, id string) (*ContainerInfo, error) {
	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s: %w", id, err)
	}

	health := ""
	if info.State.Health != nil {
		health = info.State.Health.Status
	}

	ip := info.NetworkSettings.IPAddress
	if ip == "" && len(info.NetworkSettings.Networks) > 0 {
		for _, net := range info.NetworkSettings.Networks {
			ip = net.IPAddress
			break
		}
	}

	var ports []PortBinding
	if info.NetworkSettings != nil && info.NetworkSettings.Ports != nil {
		for containerPort, bindings := range info.NetworkSettings.Ports {
			for _, b := range bindings {
				hp, _ := strconv.Atoi(b.HostPort)
				if hp > 0 {
					ports = append(ports, PortBinding{
						ContainerPort: containerPort.Int(),
						HostPort:      hp,
					})
				}
			}
		}
	}

	return &ContainerInfo{
		ID:        info.ID,
		Name:      strings.TrimPrefix(info.Name, "/"),
		Status:    info.State.Status,
		ExitCode:  info.State.ExitCode,
		Image:     info.Config.Image,
		Labels:    info.Config.Labels,
		Ports:     ports,
		Health:    health,
		IPAddress: ip,
	}, nil
}

// CreateContainer creates a container from a spec.
func (c *Client) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	// Primary container port (no host binding if HostPort == 0)
	if spec.ContainerPort > 0 {
		cPort := nat.Port(fmt.Sprintf("%d/tcp", spec.ContainerPort))
		exposedPorts[cPort] = struct{}{}
		if spec.HostPort > 0 {
			hostIP := spec.HostIP
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			portBindings[cPort] = []nat.PortBinding{
				{HostIP: hostIP, HostPort: fmt.Sprintf("%d", spec.HostPort)},
			}
		}
	}

	// Additional published ports (e.g. Caddy :80, :443, :2019)
	for _, ep := range spec.ExtraPorts {
		proto := ep.Protocol
		if proto == "" {
			proto = "tcp"
		}
		cPort := nat.Port(fmt.Sprintf("%d/%s", ep.ContainerPort, proto))
		exposedPorts[cPort] = struct{}{}
		hostIP := ep.HostIP
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}
		portBindings[cPort] = []nat.PortBinding{
			{HostIP: hostIP, HostPort: fmt.Sprintf("%d", ep.HostPort)},
		}
	}

	hostCfg := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(restartPolicy(spec.RestartPolicy))},
		Binds:         spec.Binds,
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": "10m",
				"max-file": "3",
			},
		},
	}
	if len(portBindings) > 0 {
		hostCfg.PortBindings = portBindings
	}

	// Apply resource limits if set.
	if spec.MemoryMB > 0 {
		hostCfg.Memory = spec.MemoryMB * 1024 * 1024
	}
	if spec.CPUs > 0 {
		hostCfg.NanoCPUs = int64(spec.CPUs * 1e9)
	}

	containerCfg := &container.Config{
		Image:        spec.Image,
		Cmd:          spec.Cmd,
		Env:          spec.Env,
		Labels:       spec.Labels,
		ExposedPorts: exposedPorts,
	}

	networkCfg := &network.NetworkingConfig{}
	if spec.NetworkName != "" {
		eps := &network.EndpointSettings{}
		if len(spec.NetworkAliases) > 0 {
			eps.Aliases = spec.NetworkAliases
		}
		networkCfg.EndpointsConfig = map[string]*network.EndpointSettings{
			spec.NetworkName: eps,
		}
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("docker create container: %w", err)
	}
	return resp.ID, nil
}

// StartContainer starts a container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker start %s: %w", id, err)
	}
	return nil
}

// StopContainer stops a container with a grace period.
func (c *Client) StopContainer(ctx context.Context, id string, timeoutSec int) error {
	timeout := timeoutSec
	if err := c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("docker stop %s: %w", id, err)
	}
	return nil
}

// RemoveContainer removes a container.
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	if err := c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("docker rm %s: %w", id, err)
	}
	return nil
}

// RegistryAuth holds credentials for private registries.
type RegistryAuth struct {
	ServerAddress string `json:"server_address,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
}

// PullImage pulls a public image, streaming progress to w.
func (c *Client) PullImage(ctx context.Context, ref string, w io.Writer) error {
	return c.PullImageWithAuth(ctx, ref, nil, w)
}

// PullImageWithAuth pulls an image with optional private registry credentials.
func (c *Client) PullImageWithAuth(ctx context.Context, ref string, regAuth *RegistryAuth, w io.Writer) error {
	pullOpts := image.PullOptions{}
	if regAuth != nil && (regAuth.Username != "" || regAuth.Password != "") {
		authConfig := registry.AuthConfig{
			ServerAddress: regAuth.ServerAddress,
			Username:      regAuth.Username,
			Password:      regAuth.Password,
		}
		encodedJSON, _ := json.Marshal(authConfig)
		pullOpts.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	rc, err := c.cli.ImagePull(ctx, ref, pullOpts)
	if err != nil {
		return fmt.Errorf("docker pull %s: %w", ref, err)
	}
	defer rc.Close()

	// Stream the pull progress in bounded chunks.
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(w, rc, buf)
	return err
}

// BuildImage builds an image from a Dockerfile and returns the image ID.
func (c *Client) BuildImage(ctx context.Context, opts BuildOptions, w io.Writer) (string, error) {
	// Create a tar archive of the build context.
	tarReader, err := createBuildContextTar(opts.ContextDir)
	if err != nil {
		return "", fmt.Errorf("create build context: %w", err)
	}
	defer tarReader.Close()

	dockerfilePath := opts.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	buildArgs := make(map[string]*string)
	for k, v := range opts.BuildArgs {
		v2 := v
		buildArgs[k] = &v2
	}

	resp, err := c.cli.ImageBuild(ctx, tarReader, types.ImageBuildOptions{
		Dockerfile: dockerfilePath,
		Tags:       opts.Tags,
		BuildArgs:  buildArgs,
		NoCache:    opts.NoCache,
		Remove:     true,      // remove intermediate containers after build
		ForceRemove: true,
	})
	if err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	defer resp.Body.Close()

	// Stream build output and capture the final image ID.
	imageID := ""
	buf := make([]byte, 32*1024)
	dec := json.NewDecoder(io.TeeReader(resp.Body, w))

	type buildMsg struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
		Aux    *struct {
			ID string `json:"ID"`
		} `json:"aux"`
	}

	for {
		var msg buildMsg
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			break
		}
		if msg.Error != "" {
			return "", fmt.Errorf("docker build error: %s", msg.Error)
		}
		if msg.Aux != nil && msg.Aux.ID != "" {
			imageID = msg.Aux.ID
		}
		_ = buf // used by TeeReader above
	}

	return imageID, nil
}

// StreamLogs streams container logs to w.
func (c *Client) StreamLogs(ctx context.Context, id string, opts LogOptions, w io.Writer) error {
	tail := opts.Tail
	if tail == "" {
		tail = "100"
	}

	rc, err := c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Tail:       tail,
		Timestamps: opts.Timestamps,
	})
	if err != nil {
		return fmt.Errorf("docker logs %s: %w", id, err)
	}
	defer rc.Close()

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(w, rc, buf)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// EnsureNetwork creates a network if it does not exist.
func (c *Client) EnsureNetwork(ctx context.Context, name string) (string, error) {
	nets, err := c.ListNetworks(ctx, name)
	if err != nil {
		return "", err
	}
	for _, n := range nets {
		if n.Name == name {
			return n.ID, nil
		}
	}

	resp, err := c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:     "bridge",
		Attachable: true,
	})
	if err != nil {
		return "", fmt.Errorf("docker create network %s: %w", name, err)
	}
	return resp.ID, nil
}

// ListNetworks returns networks with the given name.
func (c *Client) ListNetworks(ctx context.Context, name string) ([]NetworkSummary, error) {
	f := filters.NewArgs()
	if name != "" {
		f.Add("name", name)
	}

	nets, err := c.cli.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker list networks: %w", err)
	}

	result := make([]NetworkSummary, 0, len(nets))
	for _, n := range nets {
		result = append(result, NetworkSummary{ID: n.ID, Name: n.Name})
	}
	return result, nil
}

// RemoveNetwork removes a Docker network.
func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	if err := c.cli.NetworkRemove(ctx, id); err != nil {
		return fmt.Errorf("docker rm network %s: %w", id, err)
	}
	return nil
}

// ConnectNetwork connects a running container to a network with optional aliases.
func (c *Client) ConnectNetwork(ctx context.Context, netName, containerID string, aliases []string) error {
	var eps *network.EndpointSettings
	if len(aliases) > 0 {
		eps = &network.EndpointSettings{Aliases: aliases}
	}
	if err := c.cli.NetworkConnect(ctx, netName, containerID, eps); err != nil {
		// Ignore error if already connected
		if strings.Contains(strings.ToLower(err.Error()), "already exists") ||
			strings.Contains(strings.ToLower(err.Error()), "already connected") {
			return nil
		}
		return fmt.Errorf("docker connect network %s to %s: %w", netName, containerID, err)
	}
	return nil
}

// RemoveImage removes an image.
func (c *Client) RemoveImage(ctx context.Context, ref string, force bool) error {
	_, err := c.cli.ImageRemove(ctx, ref, image.RemoveOptions{Force: force})
	if err != nil {
		return fmt.Errorf("docker rmi %s: %w", ref, err)
	}
	return nil
}

// restartPolicy normalises the restart policy name.
func restartPolicy(s string) string {
	switch s {
	case "always", "on-failure", "unless-stopped":
		return s
	default:
		return "no"
	}
}
// PruneAll performs a system prune (containers, networks, images, build cache).
func (c *Client) PruneAll(ctx context.Context) error {
	f := filters.NewArgs()
	if _, err := c.cli.ContainersPrune(ctx, f); err != nil {
		return fmt.Errorf("containers prune: %w", err)
	}
	if _, err := c.cli.NetworksPrune(ctx, f); err != nil {
		return fmt.Errorf("networks prune: %w", err)
	}
	if _, err := c.cli.ImagesPrune(ctx, f); err != nil {
		return fmt.Errorf("images prune: %w", err)
	}
	if _, err := c.cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{}); err != nil {
		return fmt.Errorf("build cache prune: %w", err)
	}
	return nil
}

// GetContainerStats retrieves a single snapshot of container CPU/RAM usage.
func (c *Client) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	stats, err := c.cli.ContainerStats(ctx, id, false) // stream=false
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()

	var v types.StatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&v); err != nil {
		return nil, err
	}

	// Calculate memory
	memUsage := v.MemoryStats.Usage
	if v.MemoryStats.Stats != nil {
		if cgroupV1Cache, ok := v.MemoryStats.Stats["cache"]; ok {
			memUsage -= cgroupV1Cache
		} else if inactiveFile, ok := v.MemoryStats.Stats["inactive_file"]; ok {
			memUsage -= inactiveFile
		}
	}
	memLimit := v.MemoryStats.Limit

	// Calculate CPU percent
	var cpuPercent float64
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	return &ContainerStats{
		MemoryUsageBytes: int64(memUsage),
		MemoryLimitBytes: int64(memLimit),
		CPUPercent:       cpuPercent,
	}, nil
}