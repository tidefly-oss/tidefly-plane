package v1

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	adminhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/http"
	authhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/auth/http"
	containerhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/containers/http"
	deployhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/deploy/http"
	eventshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/events/http"
	githttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/git/http"
	imageshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/images/http"
	logshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/logs/http"
	networkshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/networks/http"
	notifhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/notifications/http"
	projectshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/projects/http"
	systemhttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/system/http"
	templateshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/templates/http"
	volumeshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/volumes/http"
	webhookshttp "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/webhooks/http"
	"github.com/tidefly-oss/tidefly-backend/internal/auth"
	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	webhooksvc "github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

func Register(
	api huma.API,
	e *echo.Echo,
	jwtSvc *auth.Service,
	tokenStore *auth.TokenStore,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	templateLoader *template.Loader,
	notifSvc *notifications.Service,
	gitSvc *git.Service,
	webhookSvc *webhooksvc.Service,
	asynqClient *asynq.Client,
	notifier *notifiersvc.Service,
) {
	deployer := deploy.New(rt, db)

	// ── Middleware ─────────────────────────────────────────────────────────────
	requireAuth := middleware.RequireAuthHuma(api, jwtSvc)
	requireAdmin := middleware.RequireAdminHuma(api)
	echoInject := middleware.InjectUser(db)
	echoSSE := middleware.RequireAuthSSE(jwtSvc)

	mw := huma.Middlewares{requireAuth}
	adminMw := huma.Middlewares{requireAuth, requireAdmin}

	// ── Auth ───────────────────────────────────────────────────────────────────
	authhttp.New(db, jwtSvc, tokenStore, log).RegisterRoutes(api, mw)

	// ── All other routes ───────────────────────────────────────────────────────
	adminhttp.New(db, log, notifier).RegisterRoutes(api, mw, adminMw)
	containerhttp.New(rt, deployer, db, log, caddy).RegisterRoutes(api, e, mw, echoSSE, echoInject)
	deployhttp.New(db, deployer, templateLoader, log, caddy, rt, notifSvc, notifier).RegisterRoutes(api, mw)
	eventshttp.New(rt).RegisterRoutes(e, echoSSE, echoInject)
	githttp.New(gitSvc, db, log).RegisterRoutes(api, mw)
	imageshttp.New(rt).RegisterRoutes(api, mw, adminMw)
	logshttp.New(db).RegisterRoutes(api, e, mw, adminMw, echoSSE, echoInject)
	networkshttp.New(rt, log, db).RegisterRoutes(api, mw, adminMw)
	notifhttp.New(notifSvc).RegisterRoutes(api, e, mw, echoSSE, echoInject)
	projectshttp.New(db, rt, log).RegisterRoutes(api, mw)
	systemhttp.New(rt, db).RegisterRoutes(api, mw)
	templateshttp.New(templateLoader).RegisterRoutes(api, mw)
	volumeshttp.New(rt, deployer, db, log).RegisterRoutes(api, mw, adminMw)
	webhookshttp.New(db, asynqClient, log, webhookSvc).RegisterRoutes(api, e, mw)
}
