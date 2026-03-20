package config

import "github.com/spf13/viper"

func setDatabaseDefaults() {
	viper.SetDefault("DATABASE_URL", "")
	viper.SetDefault("POSTGRES_USER", "tidefly")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "tidefly")
}
