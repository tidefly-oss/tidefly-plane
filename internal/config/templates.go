package config

import "github.com/spf13/viper"

func setTemplatesDefaults() {
	viper.SetDefault("TEMPLATES_DIR", "../../tidefly-plane-templates")
}
