package config

import (
	"context"
	"net"
	"time"
)

func (c *Config) SMTPConfigured() bool {
	return c.SMTP.Host != "" &&
		c.SMTP.Host != "localhost" &&
		c.SMTP.Host != "127.0.0.1"
}

func (c *Config) RedisConfigured() bool {
	return c.Redis.URL != "" || c.Redis.Addr != ""
}

func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

func resolveRuntimeSocket(runtimeType, runtimeSocket, podmanSocket, dockerSocket string) string {
	if runtimeSocket != "" {
		return runtimeSocket
	}
	switch runtimeType {
	case "podman":
		return podmanSocket
	case "docker":
		return dockerSocket
	default:
		return dockerSocket
	}
}

func isPortAvailable(port string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":"+port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
