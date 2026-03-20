package config

import "github.com/spf13/viper"

func setRuntimeDefaults() {
	viper.SetDefault("RUNTIME_TYPE", "docker")
	viper.SetDefault("RUNTIME_SOCKET", "")
	viper.SetDefault("DOCKER_SOCK", "/var/run/docker.sock")
	viper.SetDefault("PODMAN_SOCK", "/run/user/1000/podman/podman.sock")
}
