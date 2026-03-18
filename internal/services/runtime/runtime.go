package runtime

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type RuntimeType string

const (
	RuntimeDocker RuntimeType = "docker"
	RuntimePodman RuntimeType = "podman"
)

type ContainerStatus string

const (
	StatusRunning ContainerStatus = "running"
	StatusStopped ContainerStatus = "stopped"
	StatusPaused  ContainerStatus = "paused"
	StatusExited  ContainerStatus = "exited"
	StatusCreated ContainerStatus = "created"
	StatusUnknown ContainerStatus = "unknown"
)

type Container struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Status   ContainerStatus   `json:"status"`
	State    string            `json:"state"`
	Created  time.Time         `json:"created"`
	Ports    []Port            `json:"ports"`
	Labels   map[string]string `json:"labels,omitempty"`
	Mounts   []Mount           `json:"mounts,omitempty"`
	Networks []string          `json:"networks,omitempty"`
}

type ContainerDetails struct {
	Container
	Command       string   `json:"command"`
	Entrypoint    []string `json:"entrypoint"`
	Env           []string `json:"env"`
	Mounts        []Mount  `json:"mounts"`
	Networks      []string `json:"networks"`
	RestartPolicy string   `json:"restart_policy"`
}

// ContainerSpec describes a container to be created via Deploy.
type ContainerSpec struct {
	Name        string
	Image       string
	Env         []string
	Ports       []PortBinding
	Volumes     []VolumeMount
	Labels      map[string]string
	Healthcheck *Healthcheck
	Restart     string
	Command     string
	Network     string
}

type PortBinding struct {
	HostPort      string // e.g. "5432"
	ContainerPort int
	Protocol      string // "tcp" | "udp"
}

type VolumeMount struct {
	Name  string // volume name
	Mount string // container path
}

type Healthcheck struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	Retries     int
	StartPeriod time.Duration
}

type Port struct {
	HostIP        string `json:"host_ip"`
	HostPort      uint16 `json:"host_port"`
	ContainerPort uint16 `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

type Image struct {
	ID      string    `json:"id"`
	Tags    []string  `json:"tags"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
}

type Volume struct {
	Name      string            `json:"name"`
	Driver    string            `json:"driver"`
	Mountpath string            `json:"mountpath"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type Network struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Driver string            `json:"driver"`
	Scope  string            `json:"scope"`
	IPAM   []NetworkSubnet   `json:"ipam,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

type NetworkSubnet struct {
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
}

type SystemInfo struct {
	RuntimeType    RuntimeType `json:"runtime_type"`
	Version        string      `json:"version"`
	APIVersion     string      `json:"api_version"`
	OS             string      `json:"os"`
	Architecture   string      `json:"architecture"`
	TotalMemory    int64       `json:"total_memory"`
	ContainerCount int         `json:"container_count"`
	RunningCount   int         `json:"running_count"`
	PausedCount    int         `json:"paused_count"`
	StoppedCount   int         `json:"stopped_count"`
	CPUPercent     float64     `json:"cpu_percent"`
	MemUsedMB      int64       `json:"mem_used_mb"`
	MemTotalMB     int64       `json:"mem_total_mb"`
	MemPercent     float64     `json:"mem_percent"`
	DiskUsedMB     int64       `json:"disk_used_mb"`
	DiskTotalMB    int64       `json:"disk_total_mb"`
	DiskPercent    float64     `json:"disk_percent"`
	UptimeSeconds  uint64      `json:"uptime_seconds"`
}

type StopOptions struct {
	Timeout *int
}

type LogOptions struct {
	Follow     bool
	Tail       string
	Since      string
	Timestamps bool
}

// Runtime is the abstract interface for Docker, Podman, etc.
// Implementations live in runtime/docker and runtime/podman.
type Runtime interface {
	Type() RuntimeType
	Ping(ctx context.Context) error
	SystemInfo(ctx context.Context) (SystemInfo, error)

	// ListContainers lifecycle
	ListContainers(ctx context.Context, all bool) ([]Container, error)
	// ListAllContainers returns every container including tidefly.internal ones.
	// Use only for host-level operations like port conflict detection.
	ListAllContainers(ctx context.Context) ([]Container, error)
	GetContainer(ctx context.Context, id string) (*ContainerDetails, error)
	CreateContainer(ctx context.Context, spec ContainerSpec) error
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string, opts StopOptions) error
	RestartContainer(ctx context.Context, id string, opts StopOptions) error
	DeleteContainer(ctx context.Context, id string, force bool) error
	ContainerLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, id string) (io.ReadCloser, error)
	BuildImage(ctx context.Context, tag string, dockerfile string) (io.ReadCloser, error)

	// GetResources Resource Limits — Hybrid: live wo möglich, restart bei RAM-Verringerung
	GetResources(ctx context.Context, containerID string) (*ResourceConfig, error)
	UpdateResources(ctx context.Context, containerID string, cfg ResourceConfig) (*UpdateResult, error)

	// ExecAttach — interactive TTY-Session via WebSocket (bash → sh fallback)
	ExecAttach(ctx context.Context, containerID string, ws *websocket.Conn) error

	// ListImages Images
	ListImages(ctx context.Context) ([]Image, error)
	DeleteImage(ctx context.Context, id string, force bool) error

	// ListVolumes Volumes
	ListVolumes(ctx context.Context) ([]Volume, error)
	CreateVolume(ctx context.Context, name string) error
	DeleteVolume(ctx context.Context, name string) error

	// ListNetworks Networks
	ListNetworks(ctx context.Context) ([]Network, error)
	GetNetwork(ctx context.Context, id string) (*Network, error)
	CreateNetwork(ctx context.Context, name string) error
	DeleteNetwork(ctx context.Context, id string) error

	// EventStream provides a real-time stream of container events (start, stop, die, etc.) for live updates in the UI.
	EventStream(ctx context.Context) (<-chan ContainerEvent, <-chan error)
}

// MapStatus helper functions
func MapStatus(state string) ContainerStatus {
	switch strings.ToLower(state) {

	case "running":
		return StatusRunning

	case "exited", "stopped", "dead":
		return StatusStopped

	case "paused":
		return StatusPaused

	case "created", "configured":
		return StatusCreated

	default:
		return StatusUnknown
	}
}
