package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/converter"
)

func (h *ServiceJobHandler) buildImage(ctx context.Context, result *converter.Result) error {
	var out interface {
		Read([]byte) (int, error)
		Close() error
	}
	var err error

	switch {
	case result.BuildContext != nil:
		dockerfilePath := result.DockerfilePath
		if dockerfilePath == "" {
			dockerfilePath = "Dockerfile"
		}
		out, err = h.rt.BuildImageFromContext(ctx, result.BuildTag, dockerfilePath, result.BuildContext)
	case result.InlineDockerfile != "":
		out, err = h.rt.BuildImage(ctx, result.BuildTag, result.InlineDockerfile)
	default:
		return fmt.Errorf("no build context or dockerfile")
	}

	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		var line struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err == nil && line.Error != "" {
			return fmt.Errorf("build error: %s", line.Error)
		}
	}
	return scanner.Err()
}
