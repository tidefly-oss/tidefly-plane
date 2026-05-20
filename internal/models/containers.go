package models

import "time"

// ContainerMeta stores Tidefly-specific metadata for containers that
// Docker/Podman don't natively support. One row per container ID.
// For managed services, Service.ManifestJSON remains the source of truth
// for desired state — ContainerMeta is the fast-read cache for the runtime layer.
type ContainerMeta struct {
	// ContainerID is the Docker/Podman short or full container ID.
	ContainerID string `gorm:"primaryKey;type:varchar(64)" json:"container_id"`

	// ServiceID links this container to a managed Service record.
	// Empty string for plain (unmanaged) containers.
	ServiceID *string `gorm:"type:uuid;index;default:null" json:"service_id,omitempty"`

	// DeployStrategy controls how redeployments are rolled out.
	// One of: "rolling" | "recreate" | "blue-green". Default: "rolling".
	DeployStrategy string `gorm:"type:varchar(32);default:'rolling'" json:"deploy_strategy"`

	// AutoscalingEnabled — when true the autoscale job manages replica count.
	AutoscalingEnabled bool `gorm:"default:false" json:"autoscaling_enabled"`

	// Replicas is the desired replica count when autoscaling is off.
	Replicas int `gorm:"default:1" json:"replicas"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (*ContainerMeta) TableName() string { return "container_meta" }
