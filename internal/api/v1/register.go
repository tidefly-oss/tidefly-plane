package v1

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	adminhttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/http"
	agenthttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/agent/http"
	authhttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/auth/http"
	backuphttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/backup/http"
	containerhttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/http"
	githttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/git/http"
	imageshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/images/http"
	logshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/logs/http"
	networkshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/networks/http"
	notifhttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/notifications/http"
	projectshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/projects/http"
	serviceshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/services/http"
	setuphttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/setup/http"
	systemhttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/system/http"
	templateshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/templates/http"
	volumeshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/volumes/http"
	webhookshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/webhooks/http"
	wshttp "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/ws/http"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/backup"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/template"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

func Register(
	api huma.API,
	r chi.Router,
	jwtSvc *auth.Service,
	tokenStore *auth.TokenStore,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	templateLoader *template.Loader,
	notifSvc *notification.Service,
	gitSvc *git.Service,
	webhookSvc *webhook.Service,
	asynqClient *asynq.Client,
	notifier *notification.Notifier,
	metricsReg *metrics.Registry,
	caService *ca.Service,
	agentClient *agentsvc.Client,
	bus *eventbus.Bus,
	ingressAdapter ingress.Adapter,
) {
	deployer := deploy.New(rt, db)

	setuphttp.New(db).RegisterRoutes(api)

	requireAuth := middleware.RequireAuthHuma(api, jwtSvc)
	requireAdmin := middleware.RequireAdminHuma(api)
	sseAuth := middleware.RequireAuthSSE(jwtSvc)

	mw := huma.Middlewares{requireAuth}
	adminMw := huma.Middlewares{requireAuth, requireAdmin}

	// ── WebSocket ─────────────────────────────────────────────────────────────
	wshttp.New(bus, jwtSvc, log, metricsReg).RegisterRoutes(r)

	// ── REST ──────────────────────────────────────────────────────────────────
	authhttp.New(db, jwtSvc, tokenStore, log).RegisterRoutes(api, mw)
	adminhttp.New(db, log, notifier, caddy).RegisterRoutes(api, mw, adminMw)
	githttp.New(gitSvc, db, log, bus).RegisterRoutes(api, mw)
	imageshttp.New(rt, bus).RegisterRoutes(api, mw, adminMw)
	networkshttp.New(rt, log, db, bus).RegisterRoutes(api, mw, adminMw)
	notifhttp.New(notifSvc).RegisterRoutes(api, mw)
	projectshttp.New(db, rt, log).RegisterRoutes(api, mw)
	templateshttp.New(templateLoader).RegisterRoutes(api, mw)
	volumeshttp.New(rt, deployer, db, log, bus).RegisterRoutes(api, mw, adminMw)
	serviceshttp.New(db, deployer, asynqClient, log, gitSvc, templateLoader, rt, ingressAdapter).RegisterRoutes(api, mw)

	// ── REST + SSE ────────────────────────────────────────────────────────────
	agenthttp.New(db, caService, agentClient, bus).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	backuphttp.New(backup.New(db)).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	containerhttp.New(rt, db, log, caddy, bus).RegisterRoutes(api, r, mw, sseAuth)
	logshttp.New(db).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	webhookshttp.New(db, asynqClient, log, webhookSvc).RegisterRoutes(api, r, mw)

	// ── SSE ───────────────────────────────────────────────────────────────────
	systemHandler := systemhttp.New(rt, log, metricsReg, bus)
	systemHandler.RegisterRoutes(api, mw, adminMw)
	systemHandler.RegisterSSERoutes(r, sseAuth)
}
