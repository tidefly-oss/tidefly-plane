package template

// Template is the parsed representation of a service template YAML file.
type Template struct {
	Slug           string              `yaml:"slug"            json:"slug"`
	Name           string              `yaml:"name"            json:"name"`
	Category       string              `yaml:"category"        json:"category"`
	Icon           string              `yaml:"icon"            json:"icon"`
	Description    string              `yaml:"description"     json:"description"`
	Versions       []string            `yaml:"versions"        json:"versions"`
	DefaultVersion string              `yaml:"default_version" json:"default_version"`
	Fields         []TemplateField     `yaml:"fields"          json:"fields"`
	Containers     []TemplateContainer `yaml:"containers"      json:"containers,omitempty"`
}

type TemplateField struct {
	Key               string           `yaml:"key"                json:"key"`
	Label             string           `yaml:"label"              json:"label"`
	Type              string           `yaml:"type"               json:"type"`
	Default           string           `yaml:"default"            json:"default,omitempty"`
	Placeholder       string           `yaml:"placeholder"        json:"placeholder,omitempty"`
	Required          bool             `yaml:"required"           json:"required"`
	Generated         bool             `yaml:"generated"          json:"generated"`
	StoreHash         bool             `yaml:"store_hash"         json:"store_hash,omitempty"`
	ShowPlaintextOnce bool             `yaml:"show_plaintext_once" json:"show_plaintext_once,omitempty"`
	Options           []TemplateOption `yaml:"options"            json:"options,omitempty"`
}

type TemplateOption struct {
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label" json:"label"`
}

type TemplateContainer struct {
	Name        string               `yaml:"name"         json:"name"`
	Image       string               `yaml:"image"        json:"image"`
	ImagePodman string               `yaml:"image_podman" json:"image_podman,omitempty"`
	Restart     string               `yaml:"restart"      json:"restart,omitempty"`
	Command     string               `yaml:"command"      json:"command,omitempty"`
	Env         map[string]string    `yaml:"env"          json:"env,omitempty"`
	Ports       []TemplatePort       `yaml:"ports"        json:"ports,omitempty"`
	Volumes     []TemplateVolume     `yaml:"volumes"      json:"volumes,omitempty"`
	Healthcheck *TemplateHealthcheck `yaml:"healthcheck"  json:"healthcheck,omitempty"`
	Labels      map[string]string    `yaml:"labels"       json:"labels,omitempty"`
}

type TemplatePort struct {
	Host      string `yaml:"host"      json:"host"`
	Container int    `yaml:"container" json:"container"`
	Protocol  string `yaml:"protocol"  json:"protocol,omitempty"`
}

type TemplateVolume struct {
	Name  string `yaml:"name"  json:"name"`
	Mount string `yaml:"mount" json:"mount"`
}

type TemplateHealthcheck struct {
	Test        []string `yaml:"test"         json:"test"`
	Interval    string   `yaml:"interval"     json:"interval,omitempty"`
	Timeout     string   `yaml:"timeout"      json:"timeout,omitempty"`
	Retries     int      `yaml:"retries"      json:"retries"`
	StartPeriod string   `yaml:"start_period" json:"start_period,omitempty"`
}
