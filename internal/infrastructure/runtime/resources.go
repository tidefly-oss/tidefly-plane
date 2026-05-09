package runtime

// ResourceConfig beschreibt CPU/Memory/Restart-Limits eines Containers.
// Wird sowohl für UpdateResources als auch für CreateContainer verwendet.
// Nullwerte = unlimited / Docker-Default.
type ResourceConfig struct {
	// CPUCores — Anzahl CPU-Kerne (z.B. 0.5, 1.0, 2.0).
	// 0 = unlimited. Wird intern zu NanoCPUs konvertiert (cores * 1e9).
	CPUCores float64 `json:"cpu_cores"`

	// MemoryMB — RAM-Limit in Megabyte.
	// 0 = unlimited. Docker minimum: 6 MB.
	MemoryMB int64 `json:"memory_mb"`

	// MemorySwapMB — Swap-Limit in Megabyte (RAM + Swap zusammen).
	// -1 = unlimited swap, 0 = kein Swap (nur RAM), >0 = explizites Limit.
	// Muss >= MemoryMB sein wenn gesetzt.
	MemorySwapMB int64 `json:"memory_swap_mb"`

	// RestartPolicy — Container-Neustart-Verhalten.
	// "no" | "always" | "on-failure" | "unless-stopped"
	// "" = unverändert lassen
	RestartPolicy string `json:"restart_policy"`

	// MaxRetries — nur relevant wenn RestartPolicy = "on-failure"
	MaxRetries int `json:"max_retries,omitempty"`
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
