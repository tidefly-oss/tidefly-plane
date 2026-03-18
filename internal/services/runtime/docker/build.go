package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"

	"github.com/docker/docker/api/types/build"
)

func (d *Runtime) BuildImage(ctx context.Context, tag string, dockerfile string) (io.ReadCloser, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

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

	resp, err := d.client.ImageBuild(
		ctx, buf, build.ImageBuildOptions{
			Tags:        []string{tag},
			Dockerfile:  "Dockerfile",
			Remove:      true,
			ForceRemove: true,
		},
	)
	if err != nil {
		return nil, err
	}

	if resp.Body == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	return resp.Body, nil
}
