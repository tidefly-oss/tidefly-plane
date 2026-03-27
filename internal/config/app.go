package config

import "github.com/spf13/viper"

func setAppDefaults() {
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("APP_PORT", "8181")
	viper.SetDefault("APP_SECRET_KEY", "")
	viper.SetDefault("API_DOCS_ENABLED", false)
	viper.SetDefault("TIDEFLY_ENCRYPTION_KEY", "")
	viper.SetDefault("AGENT_GRPC_PORT", 7443)

}
