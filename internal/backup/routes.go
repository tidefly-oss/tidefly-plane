package backup

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	r chi.Router,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	// ── Config (admin only) ───────────────────────────────────────────────────
	huma.Register(api, httpx.Op("backup-get-config", "GET", httpx.V1+"/backups/config", "Get S3 backup config", "Backup", adminMw...), h.getConfig)
	huma.Register(api, httpx.Op("backup-save-config", "PUT", httpx.V1+"/backups/config", "Save S3 backup config", "Backup", adminMw...), h.saveConfig)
	huma.Register(api, httpx.Op("backup-test-connection", "POST", httpx.V1+"/backups/config/test", "Test S3 connection", "Backup", adminMw...), h.testConnection)

	// ── Postgres backup → S3 ──────────────────────────────────────────────────
	huma.Register(api, huma.Operation{
		OperationID:   "backup-postgres-create",
		Method:        http.MethodPost,
		Path:          httpx.V1 + "/backups/postgres",
		Summary:       "Create a Postgres backup and upload to S3",
		Tags:          []string{"Backup"},
		DefaultStatus: http.StatusCreated,
		Middlewares:   adminMw,
	}, h.backupPostgres)

	// ── Postgres backup → direct download (no S3) ─────────────────────────────
	r.With(sseAuth).Post(httpx.V1+"/backups/postgres/download", h.DownloadPostgresBackup)

	// ── List & Download ───────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("backup-list", "GET", httpx.V1+"/backups", "List backups", "Backup", mw...), h.listBackups)
	huma.Register(api, httpx.Op("backup-download-url", "GET", httpx.V1+"/backups/{id}/download", "Get presigned download URL", "Backup", adminMw...), h.downloadURL)

	// ── Restore ───────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("backup-postgres-restore", "POST", httpx.V1+"/backups/{id}/restore", "Restore a Postgres backup from S3", "Backup", adminMw...), h.restorePostgres)
}
