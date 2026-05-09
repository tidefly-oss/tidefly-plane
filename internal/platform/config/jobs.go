package config

import "github.com/spf13/viper"

func setJobsDefaults() {
	viper.SetDefault("JOBS_ENABLED", true)
	viper.SetDefault("JOBS_CLEANUP_CRON", "0 3 * * *")
	viper.SetDefault("JOBS_CLEANUP_OLDER_THAN", "24h")
	viper.SetDefault("JOBS_CLEANUP_STOPPED_CONTAINERS", true)
	viper.SetDefault("JOBS_CLEANUP_DANGLING_IMAGES", true)
	viper.SetDefault("JOBS_CLEANUP_UNUSED_VOLUMES", false)
	viper.SetDefault("JOBS_LOG_RETENTION_CRON", "0 4 * * *")
	viper.SetDefault("JOBS_LOG_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_AUDIT_RETENTION_DAYS", 90)
	viper.SetDefault("JOBS_NOTIFICATION_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_METRICS_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_HEALTH_CHECK_CRON", "*/5 * * * *")
	viper.SetDefault("JOBS_CONCURRENCY", 5)
}
