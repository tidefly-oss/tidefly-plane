package template

import "encoding/json"

// Template is the full parsed template including manifest and fields.
type Template struct {
	Slug           string          `json:"slug"`
	Name           string          `json:"name"`
	Category       string          `json:"category"`
	Icon           string          `json:"icon"`
	Description    string          `json:"description"`
	Tags           []string        `json:"tags,omitempty"`
	Versions       []string        `json:"versions"`
	DefaultVersion string          `json:"default_version"`
	DocsURL        string          `json:"docs_url,omitempty"`
	MinTidefly     string          `json:"min_tidefly,omitempty"`
	Official       bool            `json:"official"`
	Fields         []TemplateField `json:"fields"`

	// Manifest is the raw ServiceManifest JSON with {placeholder} fields.
	Manifest json.RawMessage `json:"manifest,omitempty"`

	// Containers is kept for backwards compatibility with old YAML templates.
	Containers []TemplateContainer `json:"containers,omitempty"`
}

type TemplateField struct {
	Key               string           `json:"key"`
	Label             string           `json:"label"`
	Type              string           `json:"type"` // string|port|credential|boolean|select
	Default           any              `json:"default,omitempty"`
	Placeholder       string           `json:"placeholder,omitempty"`
	Required          bool             `json:"required"`
	Generated         bool             `json:"generated"`
	StoreHash         bool             `json:"store_hash,omitempty"`
	ShowPlaintextOnce bool             `json:"show_plaintext_once,omitempty"`
	DependsOn         string           `json:"depends_on,omitempty"`
	Hint              string           `json:"hint,omitempty"`
	Options           []TemplateOption `json:"options,omitempty"`
}

type TemplateOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ── Legacy container format ───────────────────────────────────────────────────

type TemplateContainer struct {
	Name        string               `json:"name"`
	Image       string               `json:"image"`
	ImagePodman string               `json:"image_podman,omitempty"`
	Restart     string               `json:"restart,omitempty"`
	Command     string               `json:"command,omitempty"`
	Env         map[string]string    `json:"env,omitempty"`
	Ports       []TemplatePort       `json:"ports,omitempty"`
	Volumes     []TemplateVolume     `json:"volumes,omitempty"`
	Healthcheck *TemplateHealthcheck `json:"healthcheck,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty"`
}

type TemplatePort struct {
	Host      string `json:"host"`
	Container int    `json:"container"`
	Protocol  string `json:"protocol,omitempty"`
}

type TemplateVolume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
}

type TemplateHealthcheck struct {
	Test        []string `json:"test"`
	Interval    string   `json:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Retries     int      `json:"retries"`
	StartPeriod string   `json:"start_period,omitempty"`
}
