package config

import "github.com/spf13/viper"

func setLogwachterDefaults() {
	viper.SetDefault("LOGWATCHER_ENABLED", true)
	viper.SetDefault("LOGWATCHER_POLL_INTERVAL", "15s")
	viper.SetDefault("LOGWATCHER_TAIL_LINES", "50")
	viper.SetDefault("LOGWATCHER_MAX_MESSAGE_LEN", 300)
	viper.SetDefault("LOGWATCHER_DEDUP_WINDOW", "2m")
}
