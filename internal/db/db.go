package db

import (
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func Connect(databaseURL string, isDev bool) (*gorm.DB, error) {
	logLevel := gormlogger.Warn
	if isDev {
		logLevel = gormlogger.Info
	}

	db, err := gorm.Open(
		postgres.Open(databaseURL), &gorm.Config{
			Logger: gormlogger.Default.LogMode(logLevel),
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// AutoMigrate runs on startup — additive only, no data loss.
func AutoMigrate(database *gorm.DB) error {
	return database.AutoMigrate(
		// Auth & Users
		&models.User{},
		&models.Token{},
		// Projects
		&models.Project{},
		&models.ProjectMember{},
		&models.SystemSettings{},
		// Containers & Services
		&models.Service{},
		&models.ServiceCredential{},
		&models.Stack{},
		// Git
		&models.GitIntegration{},
		&models.GitIntegrationShare{},
		// Observability
		&models.AppLog{},
		&models.AuditLog{},
		&models.Notification{},
		// Webhooks
		&models.Webhook{},
		&models.WebhookDelivery{},
		// CA
		&models.CertificateAuthority{},
		&models.IssuedCertificate{},
		&models.WorkerRegistrationToken{},
		// Worker
		&models.WorkerNode{},
	)
}
