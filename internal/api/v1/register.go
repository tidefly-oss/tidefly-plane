package v1

import (
	"bytes"
	"io"

	"github.com/aarondl/authboss/v3"
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
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
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
	ab *authboss.Authboss,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	templateLoader *template.Loader,
	notifSvc *notifications.Service,
	gitSvc *git.Service,
	webhookSvc *webhooksvc.Service,
	asynqClient *asynq.Client,
	traefik *config.TraefikConfig,
	notifier *notifiersvc.Service,
) {
	deployer := deploy.New(rt, db)

	requireAuth := middleware.RequireAuthHuma(api, ab)
	requireAdmin := middleware.RequireAdminHuma(api, db)
	echoAuth := middleware.RequireAuth(ab)
	echoInject := middleware.InjectUser(ab, db)

	mw := huma.Middlewares{requireAuth}
	adminMw := huma.Middlewares{requireAuth, requireAdmin}

	// ── Authboss ──────────────────────────────────────────────────────────────
	e.Any(
		"/auth/:*", func(c *echo.Context) error {
			var bodyBytes []byte
			if c.Request().Body != nil {
				bodyBytes, _ = io.ReadAll(c.Request().Body)
				_ = c.Request().Body.Close()
			}
			urlCopy := *c.Request().URL
			urlCopy.Path = "/" + c.Param("*")
			rCopy := c.Request().WithContext(c.Request().Context())
			rCopy.URL = &urlCopy
			rCopy.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			rCopy.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewBuffer(bodyBytes)), nil
			}
			ab.LoadClientStateMiddleware(ab.Config.Core.Router).ServeHTTP(c.Response(), rCopy)
			return nil
		},
	)

	// ── Routes ────────────────────────────────────────────────────────────────
	adminhttp.New(db, log, notifier).RegisterRoutes(api, mw, adminMw)
	authhttp.New(db, ab, log).RegisterRoutes(api, mw)
	containerhttp.New(rt, deployer, db, log, traefik).RegisterRoutes(api, e, mw, echoAuth, echoInject)
	deployhttp.New(db, deployer, templateLoader, log, traefik, notifSvc, notifier).RegisterRoutes(api, mw)
	eventshttp.New(rt).RegisterRoutes(e, echoAuth, echoInject)
	githttp.New(gitSvc, db, log).RegisterRoutes(api, mw)
	imageshttp.New(rt).RegisterRoutes(api, mw, adminMw)
	logshttp.New(db).RegisterRoutes(api, e, mw, adminMw, echoAuth, echoInject)
	networkshttp.New(rt, log).RegisterRoutes(api, mw, adminMw)
	notifhttp.New(notifSvc).RegisterRoutes(api, e, mw, echoAuth, echoInject)
	projectshttp.New(db, rt, log).RegisterRoutes(api, mw)
	systemhttp.New(rt, db).RegisterRoutes(api, mw)
	templateshttp.New(templateLoader).RegisterRoutes(api, mw)
	volumeshttp.New(rt, deployer, db, log).RegisterRoutes(api, mw, adminMw)
	webhookshttp.New(db, asynqClient, log, webhookSvc).RegisterRoutes(api, e, mw)
}
