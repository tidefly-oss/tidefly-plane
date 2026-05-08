package http

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *Handler) buildContextFromGit(req BuildAndDeployRequest) (*bytes.Buffer, error) {
	var token string
	if req.GitIntegrationID != "" {
		var integration models.GitIntegration
		if err := h.db.First(&integration, "id = ?", req.GitIntegrationID).Error; err != nil {
			return nil, fmt.Errorf("git integration not found")
		}
		var err error
		token, err = h.gitSvc.ResolveSecret(integration.SecretEncrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt git credentials")
		}
	}

	tmpDir, err := os.MkdirTemp("", "tidefly-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneOpts := &gogit.CloneOptions{
		URL:           req.RepoURL,
		ReferenceName: plumbing.NewBranchReferenceName(req.Branch),
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
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	return dirToTar(tmpDir)
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
		defer func(f *os.File) { _ = f.Close() }(f)
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
