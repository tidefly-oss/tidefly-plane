package http

import (
	"context"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	backupsvc "github.com/tidefly-oss/tidefly-plane/internal/services/backup"
)

type Handler struct {
	backup *backupsvc.Service
}

func New(backup *backupsvc.Service) *Handler {
	return &Handler{backup: backup}
}

// ── Config ────────────────────────────────────────────────────────────────────

type GetConfigOutput struct {
	Body struct {
		Endpoint   string `json:"endpoint"`
		Bucket     string `json:"bucket"`
		Region     string `json:"region"`
		UseSSL     bool   `json:"use_ssl"`
		PathStyle  bool   `json:"path_style"`
		Prefix     string `json:"prefix"`
		Configured bool   `json:"configured"`
	}
}

func (h *Handler) GetConfig(_ context.Context, _ *struct{}) (*GetConfigOutput, error) {
	cfg, err := h.backup.GetConfig()
	out := &GetConfigOutput{}
	if err != nil {
		out.Body.Configured = false
		return out, nil
	}
	out.Body.Endpoint = cfg.Endpoint
	out.Body.Bucket = cfg.Bucket
	out.Body.Region = cfg.Region
	out.Body.UseSSL = cfg.UseSSL
	out.Body.PathStyle = cfg.PathStyle
	out.Body.Prefix = cfg.Prefix
	out.Body.Configured = true
	return out, nil
}

type SaveConfigInput struct {
	Body struct {
		Endpoint  string `json:"endpoint"   minLength:"1"`
		Bucket    string `json:"bucket"     minLength:"1"`
		Region    string `json:"region,omitempty"`
		AccessKey string `json:"access_key" minLength:"1"`
		SecretKey string `json:"secret_key" minLength:"1"`
		UseSSL    bool   `json:"use_ssl"`
		PathStyle bool   `json:"path_style"`
		Prefix    string `json:"prefix,omitempty"`
	}
}

type SaveConfigOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) SaveConfig(_ context.Context, input *SaveConfigInput) (*SaveConfigOutput, error) {
	prefix := input.Body.Prefix
	if prefix == "" {
		prefix = "backups"
	}
	region := input.Body.Region
	if region == "" {
		region = "us-east-1"
	}
	cfg := &models.BackupConfig{
		Endpoint:  input.Body.Endpoint,
		Bucket:    input.Body.Bucket,
		Region:    region,
		AccessKey: input.Body.AccessKey,
		SecretKey: input.Body.SecretKey,
		UseSSL:    input.Body.UseSSL,
		PathStyle: input.Body.PathStyle,
		Prefix:    prefix,
	}
	if err := h.backup.SaveConfig(cfg); err != nil {
		return nil, huma.Error500InternalServerError("failed to save config")
	}
	out := &SaveConfigOutput{}
	out.Body.OK = true
	return out, nil
}

type TestConnectionOutput struct {
	Body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
}

func (h *Handler) TestConnection(ctx context.Context, _ *struct{}) (*TestConnectionOutput, error) {
	out := &TestConnectionOutput{}
	if err := h.backup.TestConnection(ctx); err != nil {
		out.Body.OK = false
		out.Body.Message = err.Error()
		return out, nil
	}
	out.Body.OK = true
	out.Body.Message = "Connection successful"
	return out, nil
}

// ── Postgres Backup ───────────────────────────────────────────────────────────

type BackupPostgresInput struct {
	Body struct {
		ProjectID  string `json:"project_id,omitempty"`
		ServiceID  string `json:"service_id,omitempty"`
		DBName     string `json:"db_name"     minLength:"1"`
		DBHost     string `json:"db_host,omitempty"`
		DBPort     string `json:"db_port,omitempty"`
		DBUser     string `json:"db_user"     minLength:"1"`
		DBPassword string `json:"db_password" minLength:"1"`
	}
}

type BackupPostgresOutput struct {
	Body models.BackupRecord
}

func (h *Handler) BackupPostgres(ctx context.Context, input *BackupPostgresInput) (*BackupPostgresOutput, error) {
	record, err := h.backup.BackupPostgres(
		ctx, backupsvc.BackupRequest{
			ProjectID:  input.Body.ProjectID,
			ServiceID:  input.Body.ServiceID,
			DBName:     input.Body.DBName,
			DBHost:     input.Body.DBHost,
			DBPort:     input.Body.DBPort,
			DBUser:     input.Body.DBUser,
			DBPassword: input.Body.DBPassword,
		},
	)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &BackupPostgresOutput{Body: *record}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListBackupsInput struct {
	ProjectID string `query:"project_id"`
	ServiceID string `query:"service_id"`
}

type ListBackupsOutput struct {
	Body []models.BackupRecord
}

func (h *Handler) ListBackups(ctx context.Context, input *ListBackupsInput) (*ListBackupsOutput, error) {
	records, err := h.backup.ListBackups(ctx, input.ProjectID, input.ServiceID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list backups")
	}
	if records == nil {
		records = []models.BackupRecord{}
	}
	return &ListBackupsOutput{Body: records}, nil
}

// ── Download ──────────────────────────────────────────────────────────────────

type DownloadURLInput struct {
	ID string `path:"id"`
}

type DownloadURLOutput struct {
	Body struct {
		URL string `json:"url"`
	}
}

func (h *Handler) DownloadURL(ctx context.Context, input *DownloadURLInput) (*DownloadURLOutput, error) {
	id, err := strconv.ParseUint(input.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid id")
	}
	u, err := h.backup.DownloadURL(ctx, uint(id))
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &DownloadURLOutput{}
	out.Body.URL = u
	return out, nil
}

// ── Restore ───────────────────────────────────────────────────────────────────

type RestoreInput struct {
	ID   string `path:"id"`
	Body struct {
		DBName     string `json:"db_name"     minLength:"1"`
		DBHost     string `json:"db_host,omitempty"`
		DBPort     string `json:"db_port,omitempty"`
		DBUser     string `json:"db_user"     minLength:"1"`
		DBPassword string `json:"db_password" minLength:"1"`
	}
}

type RestoreOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) RestorePostgres(ctx context.Context, input *RestoreInput) (*RestoreOutput, error) {
	id, err := strconv.ParseUint(input.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid id")
	}
	if err := h.backup.RestorePostgres(
		ctx, uint(id), backupsvc.BackupRequest{
			DBName:     input.Body.DBName,
			DBHost:     input.Body.DBHost,
			DBPort:     input.Body.DBPort,
			DBUser:     input.Body.DBUser,
			DBPassword: input.Body.DBPassword,
		},
	); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &RestoreOutput{}
	out.Body.OK = true
	return out, nil
}
