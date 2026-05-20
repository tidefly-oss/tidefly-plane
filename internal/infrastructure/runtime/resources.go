package runtime

// ResourceConfig beschreibt CPU/Memory/Restart-Limits eines Containers.
// Wird sowohl für UpdateResources als auch für CreateContainer verwendet.
// Nullwerte = unlimited / Docker-Default.
type ResourceConfig struct {
	CPUCores      float64 `json:"cpu_cores"`
	MemoryMB      int64   `json:"memory_mb"`
	MemorySwapMB  int64   `json:"memory_swap_mb"`
	RestartPolicy string  `json:"restart_policy"`
	MaxRetries    int     `json:"max_retries,omitempty"`
	// NEU:
	Replicas       int                `json:"replicas,omitempty"`
	DeployStrategy string             `json:"deploy_strategy,omitempty"`
	Autoscaling    *AutoscalingConfig `json:"autoscaling,omitempty"`
}

type AutoscalingConfig struct {
	Enabled bool    `json:"enabled"`
	Min     int     `json:"min"`
	Max     int     `json:"max"`
	Metric  string  `json:"metric"` // cpu | memory | requests
	Target  float64 `json:"target"` // percent
}

// UpdateResult gibt zurück was nach UpdateResources passiert ist
type UpdateResult struct {
	// RestartRequired — true wenn Container neugestartet wurde (RAM verringert)
	RestartRequired bool `json:"restart_required"`
	// Applied — welche Felder live angewendet wurden
	Applied []string `json:"applied"`
}

// ValidRestartPolicies — erlaubte Werte für RestartPolicy
var ValidRestartPolicies = []string{"no", "always", "on-failure", "unless-stopped"}
