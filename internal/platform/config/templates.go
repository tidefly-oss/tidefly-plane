package config

import "github.com/spf13/viper"

func setTemplatesDefaults() {
	viper.SetDefault("TEMPLATES_DIR", "/etc/tidefly-plane/templates")
	viper.SetDefault("TEMPLATES_REPO", "https://github.com/tidefly-oss/tidefly-templates")
}
