package eventbus

// Topic constants
const (
	TopicContainers    = "containers"
	TopicImages        = "images"
	TopicNetworks      = "networks"
	TopicVolumes       = "volumes"
	TopicServices      = "manifest"
	TopicDeploy        = "deploy"
	TopicWorkers       = "workers"
	TopicGit           = "git"
	TopicNotifications = "notifications"
	TopicMetrics       = "metrics"
)

// Event type constants
const (
	// EventContainerUpdated Containers
	EventContainerUpdated = "container.updated"
	EventContainerDeleted = "container.deleted"

	// EventImageDeleted Images
	EventImageDeleted = "image.deleted"

	// EventNetworkDeleted Networks
	EventNetworkDeleted = "network.deleted"

	// EventVolumeDeleted Volumes
	EventVolumeDeleted = "volume.deleted"

	// EventServiceCreated Services
	EventServiceCreated = "service.created"
	EventServiceUpdated = "service.updated"
	EventServiceDeleted = "service.deleted"

	// EventDeployProgress Deploy
	EventDeployProgress = "deploy.progress"
	EventDeployDone     = "deploy.done"
	EventDeployFailed   = "deploy.failed"
	// EventWorkerUpdated Workers
	EventWorkerUpdated = "worker.updated"

	// EventGitIntegrationCreated Git
	EventGitIntegrationCreated = "git.integration.created"
	EventGitIntegrationDeleted = "git.integration.deleted"

	// EventNotificationCreated Notifications
	EventNotificationCreated = "notification.created"

	// EventSystemMetrics Metrics
	EventSystemMetrics = "system.metrics"
)

// Event is the envelope sent over WebSocket to all subscribed clients.
type Event struct {
	Type    string `json:"type"`
	Topic   string `json:"topic"`
	Payload any    `json:"payload"`
}

// ── Payloads ──────────────────────────────────────────────────────────────────

type ContainerUpdatedPayload struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	State  string `json:"state"`
}

type ContainerDeletedPayload struct {
	ID string `json:"id"`
}

type ImageDeletedPayload struct {
	ID string `json:"id"`
}

type NetworkDeletedPayload struct {
	ID string `json:"id"`
}

type VolumeDeletedPayload struct {
	Name string `json:"name"`
}

type ServicePayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

type DeployProgressPayload struct {
	DeployID  string `json:"deploy_id"`
	ServiceID string `json:"service_id"`
	Step      string `json:"step"`
	Progress  int    `json:"progress"` // 0-100
	Message   string `json:"message,omitempty"`
}

type DeployDonePayload struct {
	DeployID  string `json:"deploy_id"`
	ServiceID string `json:"service_id"`
}

type DeployFailedPayload struct {
	DeployID  string `json:"deploy_id"`
	ServiceID string `json:"service_id"`
	Error     string `json:"error"`
}

type WorkerUpdatedPayload struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
}

type GitIntegrationPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type NotificationCreatedPayload struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

type SystemMetricsPayload struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
}
