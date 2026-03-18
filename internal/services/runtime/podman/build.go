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
	// Qualify all FROM image references so Podman can resolve short names.
	// e.g. "FROM alpine/git AS clone" → "FROM docker.io/alpine/git AS clone"
	dockerfile = qualifyDockerfileFroms(dockerfile)

	// Build tar with Dockerfile inside
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer func(tw *tar.Writer) {
		err := tw.Close()
		if err != nil {
			fmt.Printf("Error closing tar writer: %v\n", err)
		}
	}(tw)

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

	resp, err := p.c.postRaw(ctx, "/libpod/build", q, "application/x-tar", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// qualifyDockerfileFroms rewrites every FROM line in a Dockerfile so that
// short image names are fully qualified with docker.io, which is required
// by Podman when no unqualified-search registries are configured.
//
// Handles all FROM variants:
//
//	FROM image
//	FROM image:tag
//	FROM image AS alias
//	FROM image:tag AS alias
//	FROM scratch          (left as-is — special Podman/Docker built-in)
func qualifyDockerfileFroms(dockerfile string) string {
	lines := strings.Split(dockerfile, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "FROM ") {
			continue
		}

		// Preserve leading whitespace (rare but valid in multi-stage)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

		// Split: FROM <image> [AS <alias>]
		parts := strings.Fields(trimmed) // ["FROM", "image", "AS", "alias"]
		if len(parts) < 2 {
			continue
		}

		img := parts[1]

		// Leave build-arg references and scratch alone
		if img == "scratch" || strings.HasPrefix(img, "$") {
			continue
		}

		qualified := qualifyImage(img, true)
		parts[1] = qualified

		lines[i] = indent + strings.Join(parts, " ")
	}
	return strings.Join(lines, "\n")
}
