package backup

import (
	"context"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

type Handler struct {
	svc *Service
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{svc: NewService(db)}
}

// ── Config ────────────────────────────────────────────────────────────────────

type getConfigOutput struct {
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

func (h *Handler) getConfig(_ context.Context, _ *struct{}) (*getConfigOutput, error) {
	cfg, err := h.svc.GetConfig()
	out := &getConfigOutput{}
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

type saveConfigInput struct {
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

type saveConfigOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) saveConfig(_ context.Context, input *saveConfigInput) (*saveConfigOutput, error) {
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
	if err := h.svc.SaveConfig(cfg); err != nil {
		return nil, huma.Error500InternalServerError("failed to save config")
	}
	out := &saveConfigOutput{}
	out.Body.OK = true
	return out, nil
}

type testConnectionOutput struct {
	Body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
}

func (h *Handler) testConnection(ctx context.Context, _ *struct{}) (*testConnectionOutput, error) {
	out := &testConnectionOutput{}
	if err := h.svc.TestConnection(ctx); err != nil {
		out.Body.OK = false
		out.Body.Message = err.Error()
		return out, nil
	}
	out.Body.OK = true
	out.Body.Message = "Connection successful"
	return out, nil
}

// ── Postgres Backup ───────────────────────────────────────────────────────────

type backupPostgresInput struct {
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

type backupPostgresOutput struct {
	Body models.BackupRecord
}

func (h *Handler) backupPostgres(ctx context.Context, input *backupPostgresInput) (*backupPostgresOutput, error) {
	record, err := h.svc.BackupPostgres(ctx, BackupRequest{
		ProjectID:  input.Body.ProjectID,
		ServiceID:  input.Body.ServiceID,
		DBName:     input.Body.DBName,
		DBHost:     input.Body.DBHost,
		DBPort:     input.Body.DBPort,
		DBUser:     input.Body.DBUser,
		DBPassword: input.Body.DBPassword,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &backupPostgresOutput{Body: *record}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

type listBackupsInput struct {
	ProjectID string `query:"project_id"`
	ServiceID string `query:"service_id"`
}

type listBackupsOutput struct {
	Body []models.BackupRecord
}

func (h *Handler) listBackups(ctx context.Context, input *listBackupsInput) (*listBackupsOutput, error) {
	records, err := h.svc.ListBackups(ctx, input.ProjectID, input.ServiceID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list backups")
	}
	if records == nil {
		records = []models.BackupRecord{}
	}
	return &listBackupsOutput{Body: records}, nil
}

// ── Download URL ──────────────────────────────────────────────────────────────

type downloadURLInput struct {
	ID string `path:"id"`
}

type downloadURLOutput struct {
	Body struct {
		URL string `json:"url"`
	}
}

func (h *Handler) downloadURL(ctx context.Context, input *downloadURLInput) (*downloadURLOutput, error) {
	id, err := strconv.ParseUint(input.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid id")
	}
	u, err := h.svc.DownloadURL(ctx, uint(id))
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &downloadURLOutput{}
	out.Body.URL = u
	return out, nil
}

// ── Restore ───────────────────────────────────────────────────────────────────

type restoreInput struct {
	ID   string `path:"id"`
	Body struct {
		DBName     string `json:"db_name"     minLength:"1"`
		DBHost     string `json:"db_host,omitempty"`
		DBPort     string `json:"db_port,omitempty"`
		DBUser     string `json:"db_user"     minLength:"1"`
		DBPassword string `json:"db_password" minLength:"1"`
	}
}

type restoreOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) restorePostgres(ctx context.Context, input *restoreInput) (*restoreOutput, error) {
	id, err := strconv.ParseUint(input.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid id")
	}
	if err := h.svc.RestorePostgres(ctx, uint(id), BackupRequest{
		DBName:     input.Body.DBName,
		DBHost:     input.Body.DBHost,
		DBPort:     input.Body.DBPort,
		DBUser:     input.Body.DBUser,
		DBPassword: input.Body.DBPassword,
	}); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &restoreOutput{}
	out.Body.OK = true
	return out, nil
}
