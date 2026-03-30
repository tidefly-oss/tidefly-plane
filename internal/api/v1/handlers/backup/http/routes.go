package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	e *echo.Echo,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	echoAuth echo.MiddlewareFunc,
	echoInject echo.MiddlewareFunc,
) {
	// ── Config (admin only) ───────────────────────────────────────────────────
	huma.Register(
		api,
		shared.Op("backup-get-config", "GET", "/api/v1/backups/config", "Get S3 backup config", "Backup", adminMw...),
		h.GetConfig,
	)
	huma.Register(
		api,
		shared.Op("backup-save-config", "PUT", "/api/v1/backups/config", "Save S3 backup config", "Backup", adminMw...),
		h.SaveConfig,
	)
	huma.Register(
		api,
		shared.Op(
			"backup-test-connection",
			"POST",
			"/api/v1/backups/config/test",
			"Test S3 connection",
			"Backup",
			adminMw...,
		),
		h.TestConnection,
	)
	// ── Postgres backup → S3 ─────────────────────────────────────────────────
	huma.Register(
		api, huma.Operation{
			OperationID:   "backup-postgres-create",
			Method:        http.MethodPost,
			Path:          "/api/v1/backups/postgres",
			Summary:       "Create a Postgres backup and upload to S3",
			Tags:          []string{"Backup"},
			DefaultStatus: http.StatusCreated,
			Middlewares:   adminMw,
		}, h.BackupPostgres,
	)
	// ── Postgres backup → direct download (no S3) ────────────────────────────
	e.POST("/api/v1/backups/postgres/download", h.DownloadPostgresBackup, echoAuth, echoInject)
	// ── List & Download ───────────────────────────────────────────────────────
	huma.Register(
		api,
		shared.Op("backup-list", "GET", "/api/v1/backups", "List backups", "Backup", mw...),
		h.ListBackups,
	)
	huma.Register(
		api,
		shared.Op(
			"backup-download-url",
			"GET",
			"/api/v1/backups/{id}/download",
			"Get presigned download URL",
			"Backup",
			adminMw...,
		),
		h.DownloadURL,
	)
	// ── Restore ───────────────────────────────────────────────────────────────
	huma.Register(
		api,
		shared.Op(
			"backup-postgres-restore",
			"POST",
			"/api/v1/backups/{id}/restore",
			"Restore a Postgres backup from S3",
			"Backup",
			adminMw...,
		),
		h.RestorePostgres,
	)
}
