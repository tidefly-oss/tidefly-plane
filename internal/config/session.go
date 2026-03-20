package config

import "github.com/spf13/viper"

func setSessionDefaults() {
	viper.SetDefault("SESSION_SECRET", "")
	viper.SetDefault("COOKIES_SECRET", "")
}
