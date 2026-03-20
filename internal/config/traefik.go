package config

import "github.com/spf13/viper"

func setTraefikDefaults() {
	viper.SetDefault("TRAEFIK_ENABLED", false)
	viper.SetDefault("TRAEFIK_BASE_DOMAIN", "")
	viper.SetDefault("TRAEFIK_ACME_EMAIL", "")
	viper.SetDefault("TRAEFIK_ACME_STAGING", false)
	viper.SetDefault("TRAEFIK_NETWORK", "tidefly_internal")
	viper.SetDefault("TRAEFIK_ENTRYPOINT_HTTP", "web")
	viper.SetDefault("TRAEFIK_ENTRYPOINT_HTTPS", "websecure")
	viper.SetDefault("TRAEFIK_FORCE_HTTPS", true)
}
