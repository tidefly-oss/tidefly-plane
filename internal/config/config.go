package config

import "github.com/spf13/viper"

func Load() (*Config, error) {
	viper.AutomaticEnv()
	
	setAppDefaults()
	setDatabaseDefaults()
	setJobsDefaults()
	setLogwachterDefaults()
	setLoggingDefaults()
	setRedisDefaults()
	setRuntimeDefaults()
	setSessionDefaults()
	setSMTPDefaults()
	setTemplatesDefaults()
	setTraefikDefaults()

	cfg := ParseConfig()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
