package config

import "github.com/spf13/viper"

func setSMTPDefaults() {
	// Dev default: Mailpit (no auth, no TLS)
	viper.SetDefault("SMTP_HOST", "localhost")
	viper.SetDefault("SMTP_PORT", "1025")
	viper.SetDefault("SMTP_USER", "")
	viper.SetDefault("SMTP_PASSWORD", "")
	viper.SetDefault("SMTP_FROM", "")
	viper.SetDefault("SMTP_TLS", "none")
}
