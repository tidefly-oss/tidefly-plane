package podman

import "strings"

// qualifyImage prepends docker.io/ to short image names.
// Podman does not resolve short names like "nginx:alpine" without registries.conf.
// Docker handles this automatically — for Podman we do it explicitly.
func qualifyImage(image string, isPodman bool) string {
	if !isPodman {
		return image
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			return image // already qualified: docker.io/..., ghcr.io/..., etc.
		}
	}
	return "docker.io/" + image
}
