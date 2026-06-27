// Package converter transforms various input formats (manifest, image, compose, dockerfile, git) into resolved service manifests.
package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
)

type SourceType string

const (
	SourceManifest   SourceType = "manifest"
	SourceStack      SourceType = "stack" // docker-compose → multiple manifest
	SourceImage      SourceType = "image"
	SourceCompose    SourceType = "compose" // legacy direct API field
	SourceDockerfile SourceType = "dockerfile"
	SourceGit        SourceType = "git"
)

type ConvertInput struct {
	Type         SourceType
	ManifestJSON string
	Image        string
	ComposeYAML  string
	StackName    string
	Dockerfile   string
	BuildTag     string
	GitURL       string
	Branch       string
	GitToken     string
	Name         string
	ProjectID    string
	Domain       string
	Port         int
	Expose       bool
	Env          []manifest.EnvVar
	Replicas     int
	Strategy     string
}

type Result struct {
	Manifests []*manifest.ServiceManifest

	// Build fields — set when the image needs to be built
	BuildRequired    bool
	BuildTag         string
	BuildContext     *bytes.Buffer
	DockerfilePath   string
	InlineDockerfile string
	GitURL           string
	Branch           string
	GitToken         string
}

// HasBuild returns true when this result requires a build step.
func (r *Result) HasBuild() bool {
	return r.BuildRequired
}

type Converter interface {
	Convert(ctx context.Context, input ConvertInput) (*Result, error)
}

type DefaultConverter struct{}

func New() *DefaultConverter {
	return &DefaultConverter{}
}

func (c *DefaultConverter) Convert(ctx context.Context, input ConvertInput) (*Result, error) {
	if input.Type == "" {
		input.Type = DetectType(input)
	}
	if input.Type == "" {
		return nil, fmt.Errorf("converter: cannot detect source type — provide manifest_json, image, compose, dockerfile, or git_url")
	}

	switch input.Type {
	case SourceManifest:
		return convertManifest(ctx, input)
	case SourceStack:
		return convertStack(ctx, input)
	case SourceImage:
		return convertImage(input)
	case SourceCompose:
		return convertCompose(input)
	case SourceDockerfile:
		return convertDockerfile(input)
	case SourceGit:
		return convertGit(ctx, input)
	default:
		return nil, fmt.Errorf("converter: unsupported source type %q", input.Type)
	}
}

func DetectType(input ConvertInput) SourceType {
	switch {
	case input.ManifestJSON != "":
		return SourceManifest
	case input.GitURL != "":
		return SourceGit
	case input.ComposeYAML != "":
		return SourceCompose
	case input.Dockerfile != "":
		return SourceDockerfile
	case input.Image != "":
		return SourceImage
	default:
		return ""
	}
}

// convertManifest handles both ServiceManifest and StackManifest JSON.
// It detects the kind field and routes accordingly.
func convertManifest(ctx context.Context, input ConvertInput) (*Result, error) {
	// Peek at the kind field
	var peek struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(input.ManifestJSON), &peek); err != nil {
		return nil, fmt.Errorf("manifest converter: invalid JSON: %w", err)
	}

	switch peek.Kind {
	case "stack":
		return convertStackManifest(ctx, input)
	default:
		return convertServiceManifest(ctx, input)
	}
}

// convertServiceManifest handles a ServiceManifest — may have a BuildSpec.
func convertServiceManifest(ctx context.Context, input ConvertInput) (*Result, error) {
	var m manifest.ServiceManifest
	if err := json.Unmarshal([]byte(input.ManifestJSON), &m); err != nil {
		return nil, fmt.Errorf("manifest converter: invalid JSON: %w", err)
	}
	if m.Metadata.Name == "" {
		return nil, fmt.Errorf("manifest converter: metadata.name is required")
	}

	result := &Result{Manifests: []*manifest.ServiceManifest{&m}}

	// If the manifest has a build spec, resolve it into a build result
	if b := m.Spec.Container.Build; b != nil {
		buildResult, err := resolveBuildSpec(ctx, m.Metadata.Name, b, input.GitToken)
		if err != nil {
			return nil, fmt.Errorf("manifest converter: resolve build: %w", err)
		}
		result.BuildRequired = true
		result.BuildTag = buildResult.BuildTag
		result.BuildContext = buildResult.BuildContext
		result.DockerfilePath = buildResult.DockerfilePath
		result.InlineDockerfile = buildResult.InlineDockerfile
		result.GitURL = buildResult.GitURL
		result.Branch = buildResult.Branch
		result.GitToken = input.GitToken

		// Set the image to the build tag so the runtime knows what to run
		m.Spec.Container.Image = buildResult.BuildTag
		m.Spec.Container.Build = nil // resolved — remove from stored manifest
	}

	return result, nil
}

// convertStackManifest handles a StackManifest (docker-compose → N manifest).
func convertStackManifest(_ context.Context, input ConvertInput) (*Result, error) {
	var sm manifest.StackManifest
	if err := json.Unmarshal([]byte(input.ManifestJSON), &sm); err != nil {
		return nil, fmt.Errorf("stack converter: invalid JSON: %w", err)
	}
	if sm.Spec.Compose == "" {
		return nil, fmt.Errorf("stack converter: spec.compose is required")
	}

	composeInput := ConvertInput{
		Type:        SourceCompose,
		ComposeYAML: sm.Spec.Compose,
		Name:        sm.Metadata.Name,
		StackName:   sm.Metadata.Name,
		Domain:      sm.Spec.Domain,
		Expose:      sm.Spec.Expose,
	}
	return convertCompose(composeInput)
}

// convertStack handles SourceStack from APIInput (compose YAML submitted directly).
func convertStack(_ context.Context, input ConvertInput) (*Result, error) {
	return convertCompose(input)
}

// resolveBuildSpec converts a BuildSpec into build parameters.
// For git contexts, it clones the repo and creates a tar buffer.
// For inline dockerfiles, it returns the dockerfile content directly.
type buildParams struct {
	BuildTag         string
	BuildContext     *bytes.Buffer
	DockerfilePath   string
	InlineDockerfile string
	GitURL           string
	Branch           string
}

func resolveBuildSpec(_ context.Context, serviceName string, b *manifest.BuildSpec, gitToken string) (*buildParams, error) {
	tag := fmt.Sprintf("localhost/tidefly/%s:latest", serviceName)
	dockerfilePath := b.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	// Inline dockerfile — no build context needed
	if b.DockerfileInline != "" {
		return &buildParams{
			BuildTag:         tag,
			InlineDockerfile: b.DockerfileInline,
			DockerfilePath:   dockerfilePath,
		}, nil
	}

	// Git context — clone and tar
	if b.IsGitContext() {
		gitURL, branch := b.GitURL()
		tarBuf, err := BuildGitContext(gitURL, branch, gitToken)
		if err != nil {
			return nil, fmt.Errorf("git clone %q: %w", gitURL, err)
		}
		return &buildParams{
			BuildTag:       tag,
			BuildContext:   tarBuf,
			DockerfilePath: dockerfilePath,
			GitURL:         gitURL,
			Branch:         branch,
		}, nil
	}

	// Local context — context must be "." for now (no local filesystem access from API)
	return &buildParams{
		BuildTag:       tag,
		DockerfilePath: dockerfilePath,
	}, nil
}
