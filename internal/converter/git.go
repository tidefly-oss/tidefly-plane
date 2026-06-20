package converter

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
)

type gitDetection struct {
	hasDockerfile  bool
	hasCompose     bool
	composeYAML    string
	dockerfilePath string
}

func convertGit(_ context.Context, input ConvertInput) (*Result, error) {
	if input.GitURL == "" {
		return nil, fmt.Errorf("git converter: git_url is required")
	}

	branch := input.Branch
	if branch == "" {
		branch = "main"
	}

	name := input.Name
	if name == "" {
		name = repoToName(input.GitURL)
	}

	if err := os.MkdirAll("/tmp", 0o755); err != nil {
		return nil, fmt.Errorf("git converter: ensure temp dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "tidefly-git-*")
	if err != nil {
		return nil, fmt.Errorf("git converter: create temp dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneOpts := &gogit.CloneOptions{
		URL:           input.GitURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	}
	if input.GitToken != "" {
		cloneOpts.Auth = &gogithttp.BasicAuth{
			Username: "x-token",
			Password: input.GitToken,
		}
	}

	if _, err := gogit.PlainClone(tmpDir, false, cloneOpts); err != nil {
		return nil, fmt.Errorf("git converter: clone %q: %w", input.GitURL, err)
	}

	det, err := detectInRepo(tmpDir)
	if err != nil {
		return nil, err
	}

	switch {
	case det.hasCompose:
		composeInput := input
		composeInput.Type = SourceCompose
		composeInput.ComposeYAML = det.composeYAML
		composeInput.Name = name
		result, err := convertCompose(composeInput)
		if err != nil {
			return nil, err
		}
		for _, m := range result.Manifests {
			m.Metadata.Labels["tidefly.git-url"] = input.GitURL
			m.Metadata.Labels["tidefly.git-branch"] = branch
		}
		return result, nil

	case det.hasDockerfile:
		buildTag := fmt.Sprintf("localhost/tidefly/%s:latest", name)

		// Build tar context now while tmpDir still exists
		tarBuf, err := dirToTar(tmpDir)
		if err != nil {
			return nil, fmt.Errorf("git converter: tar repo: %w", err)
		}

		m := &manifest.ServiceManifest{
			APIVersion: apiVersion,
			Kind:       kindService,
			Metadata: manifest.Metadata{
				Name: name,
				Labels: map[string]string{
					"tidefly.source":          string(SourceGit),
					"tidefly.git-url":         input.GitURL,
					"tidefly.git-branch":      branch,
					"tidefly.dockerfile-path": det.dockerfilePath,
					"tidefly.build-tag":       buildTag,
				},
			},
			Spec: manifest.ServiceSpec{
				Container: manifest.ContainerSpec{
					Image: buildTag,
					Env:   input.Env,
				},
			},
		}

		if input.Expose || input.Domain != "" {
			m.Spec.Expose = &manifest.ExposeSpec{
				Port:   input.Port,
				Domain: input.Domain,
				TLS:    true,
			}
		}

		return &Result{
			Manifests:      []*manifest.ServiceManifest{m},
			BuildRequired:  true,
			BuildTag:       buildTag,
			BuildContext:   tarBuf,
			DockerfilePath: det.dockerfilePath,
			GitURL:         input.GitURL,
			Branch:         branch,
			GitToken:       input.GitToken,
		}, nil

	default:
		return nil, fmt.Errorf("git converter: no Dockerfile or docker-compose.yml found in %q (branch: %s)", input.GitURL, branch)
	}
}

// BuildGitContext clones a repo and returns a tar buffer for BuildImageFromContext.
// Used by the ServiceManager when it needs to rebuild without re-running Convert.
func BuildGitContext(gitURL, branch, token string) (*bytes.Buffer, error) {
	if branch == "" {
		branch = "main"
	}

	tmpDir, err := os.MkdirTemp("", "tidefly-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneOpts := &gogit.CloneOptions{
		URL:           gitURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	}
	if token != "" {
		cloneOpts.Auth = &gogithttp.BasicAuth{
			Username: "x-token",
			Password: token,
		}
	}

	if _, err := gogit.PlainClone(tmpDir, false, cloneOpts); err != nil {
		return nil, fmt.Errorf("clone %q: %w", gitURL, err)
	}

	return dirToTar(tmpDir)
}

func detectInRepo(dir string) (*gitDetection, error) {
	det := &gitDetection{}

	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			det.hasCompose = true
			det.composeYAML = string(data)
			break
		}
	}

	for _, name := range []string{"Dockerfile", "dockerfile", "Dockerfile.prod"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			det.hasDockerfile = true
			det.dockerfilePath = name
			break
		}
	}

	return det, nil
}

func repoToName(gitURL string) string {
	base := strings.TrimSuffix(gitURL, ".git")
	parts := strings.Split(base, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "service"
	}
	return name
}

func dirToTar(dir string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := tw.WriteHeader(&tar.Header{
			Name:    rel,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	return buf, tw.Close()
}
