package config

import "github.com/spf13/viper"

func setLoggingDefaults() {
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("LOG_DB_LEVEL", "warn")
	viper.SetDefault("LOG_SLOW_QUERY_MS", 500)
}
