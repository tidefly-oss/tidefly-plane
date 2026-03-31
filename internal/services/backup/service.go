package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ── Config ────────────────────────────────────────────────────────────────────

func (s *Service) GetConfig() (*models.BackupConfig, error) {
	var cfg models.BackupConfig
	if err := s.db.First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *Service) SaveConfig(cfg *models.BackupConfig) error {
	cfg.ID = 1 // always upsert the single global config
	return s.db.Save(cfg).Error
}

func (s *Service) TestConnection(ctx context.Context) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return fmt.Errorf("no backups config found")
	}
	client, err := s.newMinioClient(cfg)
	if err != nil {
		return err
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}
	return nil
}

// ── Postgres Backup ───────────────────────────────────────────────────────────

type BackupRequest struct {
	ProjectID  string
	ServiceID  string
	DBName     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
}

func (s *Service) BackupPostgres(ctx context.Context, req BackupRequest) (*models.BackupRecord, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("no backups config: %w", err)
	}

	record := &models.BackupRecord{
		ProjectID: req.ProjectID,
		ServiceID: req.ServiceID,
		Type:      "postgres",
		Status:    "running",
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	// Run pg_dump
	data, err := s.pgDump(ctx, req)
	if err != nil {
		s.markFailed(record, err.Error())
		return record, fmt.Errorf("pg_dump failed: %w", err)
	}

	// Upload to S3
	key := fmt.Sprintf(
		"%s/%s/%s/%s.sql.gz",
		cfg.Prefix, req.ProjectID, req.ServiceID,
		time.Now().UTC().Format("2006-01-02T15-04-05"),
	)

	size, err := s.upload(ctx, cfg, key, data)
	if err != nil {
		s.markFailed(record, err.Error())
		return record, fmt.Errorf("upload failed: %w", err)
	}

	record.S3Key = key
	record.SizeBytes = size
	record.Status = "completed"
	s.db.Save(record)

	return record, nil
}

func (s *Service) pgDump(ctx context.Context, req BackupRequest) ([]byte, error) {
	port := req.DBPort
	if port == "" {
		port = "5432"
	}
	host := req.DBHost
	if host == "" {
		host = "localhost"
	}

	cmd := exec.CommandContext(
		ctx,
		"pg_dump",
		"-h", host,
		"-p", port,
		"-U", req.DBUser,
		"-d", req.DBName,
		"--no-password",
		"-F", "c", // custom format (compressed)
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+req.DBPassword)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return out.Bytes(), nil
}

func (s *Service) upload(ctx context.Context, cfg *models.BackupConfig, key string, data []byte) (int64, error) {
	client, err := s.newMinioClient(cfg)
	if err != nil {
		return 0, err
	}

	r := bytes.NewReader(data)
	info, err := client.PutObject(
		ctx, cfg.Bucket, key, r, int64(len(data)), minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		},
	)
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

// ── List & Download ───────────────────────────────────────────────────────────

func (s *Service) ListBackups(_ context.Context, projectID, serviceID string) ([]models.BackupRecord, error) {
	var records []models.BackupRecord
	q := s.db.Order("created_at DESC")
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if serviceID != "" {
		q = q.Where("service_id = ?", serviceID)
	}
	return records, q.Find(&records).Error
}

func (s *Service) DownloadURL(ctx context.Context, recordID uint) (string, error) {
	var record models.BackupRecord
	if err := s.db.First(&record, recordID).Error; err != nil {
		return "", fmt.Errorf("backups not found")
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return "", err
	}

	client, err := s.newMinioClient(cfg)
	if err != nil {
		return "", err
	}

	u, err := client.PresignedGetObject(ctx, cfg.Bucket, record.S3Key, 15*time.Minute, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign failed: %w", err)
	}
	return u.String(), nil
}

func (s *Service) RestorePostgres(ctx context.Context, recordID uint, req BackupRequest) error {
	var record models.BackupRecord
	if err := s.db.First(&record, recordID).Error; err != nil {
		return fmt.Errorf("backups not found")
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}

	client, err := s.newMinioClient(cfg)
	if err != nil {
		return err
	}

	obj, err := client.GetObject(ctx, cfg.Bucket, record.S3Key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = obj.Close() }()

	data, err := io.ReadAll(obj)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	return s.pgRestore(ctx, req, data)
}

func (s *Service) pgRestore(ctx context.Context, req BackupRequest, data []byte) error {
	port := req.DBPort
	if port == "" {
		port = "5432"
	}
	host := req.DBHost
	if host == "" {
		host = "localhost"
	}

	cmd := exec.CommandContext(
		ctx,
		"pg_restore",
		"-h", host,
		"-p", port,
		"-U", req.DBUser,
		"-d", req.DBName,
		"--no-password",
		"--clean",
		"--if-exists",
		"-F", "c",
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+req.DBPassword)
	cmd.Stdin = bytes.NewReader(data)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *Service) newMinioClient(cfg *models.BackupConfig) (*minio.Client, error) {
	return minio.New(
		cfg.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.UseSSL,
			Region: cfg.Region,
		},
	)
}

func (s *Service) markFailed(record *models.BackupRecord, errMsg string) {
	record.Status = "failed"
	record.Error = errMsg
	s.db.Save(record)
}
