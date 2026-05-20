package converter

import "github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"

// APIInput is the unified request body for POST /api/v1/services.
// Supports all source types — the converter auto-detects which to use.
//
// Source priority (first non-empty wins):
//  1. manifest_json — pre-built ServiceManifest or StackManifest JSON
//  2. git_url       — clone repo, detect Dockerfile or docker-compose.yml
//  3. compose       — inline docker-compose.yml
//  4. dockerfile    — inline Dockerfile content
//  5. image         — plain OCI image reference
type APIInput struct {
	// ── Manifest (highest priority) ───────────────────────────────────────────
	// Pre-resolved ServiceManifest or StackManifest JSON.
	// When set, all other fields except git_token are ignored.
	ManifestJSON string `json:"manifest_json,omitempty"`

	// ── Source types (mutually exclusive) ────────────────────────────────────
	Image       string `json:"image,omitempty"`
	ComposeYAML string `json:"compose,omitempty"`
	Dockerfile  string `json:"dockerfile,omitempty"`
	GitURL      string `json:"git_url,omitempty"`

	// ── Identity ──────────────────────────────────────────────────────────────
	Name      string `json:"name,omitempty"       maxLength:"128"`
	StackName string `json:"stack_name,omitempty" maxLength:"128"`
	ProjectID string `json:"project_id,omitempty" format:"uuid"`

	// ── Networking ────────────────────────────────────────────────────────────
	Domain string `json:"domain,omitempty" maxLength:"253"`
	Port   int    `json:"port,omitempty"   minimum:"1" maximum:"65535"`
	Expose bool   `json:"expose,omitempty"`

	// ── Git ───────────────────────────────────────────────────────────────────
	Branch           string `json:"branch,omitempty"             maxLength:"256"`
	GitIntegrationID string `json:"git_integration_id,omitempty" format:"uuid"`

	// ── Runtime ───────────────────────────────────────────────────────────────
	Env      []manifest.EnvVar `json:"env,omitempty"`
	Replicas int               `json:"replicas,omitempty" minimum:"1" maximum:"20"`
	Strategy string            `json:"strategy,omitempty" enum:"rolling,recreate,blue-green"`
}

func (a *APIInput) ToConvertInput(gitToken string) ConvertInput {
	return ConvertInput{
		ManifestJSON: a.ManifestJSON,
		Image:        a.Image,
		ComposeYAML:  a.ComposeYAML,
		Dockerfile:   a.Dockerfile,
		GitURL:       a.GitURL,
		Name:         a.Name,
		StackName:    a.StackName,
		ProjectID:    a.ProjectID,
		Domain:       a.Domain,
		Port:         a.Port,
		Expose:       a.Expose,
		Branch:       a.Branch,
		GitToken:     gitToken,
		Env:          a.Env,
		Replicas:     a.Replicas,
		Strategy:     a.Strategy,
	}
}

func (a *APIInput) ServiceName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.ManifestJSON != "" {
		return "" // name comes from manifest
	}
	if a.Image != "" {
		return imageToName(a.Image)
	}
	if a.GitURL != "" {
		return repoToName(a.GitURL)
	}
	return "service"
}
