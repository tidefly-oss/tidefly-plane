package config

import "github.com/spf13/viper"

func setCaddyDefaults() {
	viper.SetDefault("CADDY_ENABLED", true)
	viper.SetDefault("CADDY_ADMIN_URL", "http://caddy:2019")
	viper.SetDefault("CADDY_BASE_DOMAIN", "")
	viper.SetDefault("CADDY_ACME_EMAIL", "")
	viper.SetDefault("CADDY_ACME_STAGING", false)
	viper.SetDefault("CADDY_FORCE_HTTPS", true)
	viper.SetDefault("CADDY_INTERNAL_TLS", true)
}
