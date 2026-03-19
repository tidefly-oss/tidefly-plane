package podman

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

func (p *Runtime) BuildImage(ctx context.Context, tag string, dockerfile string) (io.ReadCloser, error) {
	dockerfile = qualifyDockerfileFroms(dockerfile)

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer func() {
		if err := tw.Close(); err != nil {
			fmt.Printf("Error closing tar writer: %v\n", err)
		}
	}()

	dfBytes := []byte(dockerfile)
	if err := tw.WriteHeader(
		&tar.Header{
			Name: "Dockerfile",
			Size: int64(len(dfBytes)),
			Mode: 0644,
		},
	); err != nil {
		return nil, err
	}
	if _, err := tw.Write(dfBytes); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("dockerfile", "Dockerfile")
	q.Set("t", tag)
	q.Set("rm", "true")
	q.Set("forcerm", "true")

	resp, err := p.c.postRaw(
		ctx,
		"/libpod/build",
		q,
		"application/x-tar",
		bytes.NewReader(buf.Bytes()),
	) //nolint:bodyclose
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func qualifyDockerfileFroms(dockerfile string) string {
	lines := strings.Split(dockerfile, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "FROM ") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		img := parts[1]
		if img == "scratch" || strings.HasPrefix(img, "$") {
			continue
		}
		parts[1] = qualifyImage(img, true)
		lines[i] = indent + strings.Join(parts, " ")
	}
	return strings.Join(lines, "\n")
}
