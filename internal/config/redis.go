package config

import "github.com/spf13/viper"

func setRedisDefaults() {
	viper.SetDefault("REDIS_URL", "")
	viper.SetDefault("REDIS_ADDR", "127.0.0.1:6379")
	viper.SetDefault("REDIS_USER", "tidefly")
	viper.SetDefault("REDIS_PASSWORD", "")
}
