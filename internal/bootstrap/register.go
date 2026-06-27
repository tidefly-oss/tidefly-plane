package bootstrap

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/admin"
	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/backup"
	"github.com/tidefly-oss/tidefly-plane/internal/container"
	"github.com/tidefly-oss/tidefly-plane/internal/dashboard"
	"github.com/tidefly-oss/tidefly-plane/internal/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/events"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	"github.com/tidefly-oss/tidefly-plane/internal/image"
	applog "github.com/tidefly-oss/tidefly-plane/internal/log"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	middleware2 "github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/network"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/project"
	"github.com/tidefly-oss/tidefly-plane/internal/setup"
	"github.com/tidefly-oss/tidefly-plane/internal/system"
	"github.com/tidefly-oss/tidefly-plane/internal/template"
	"github.com/tidefly-oss/tidefly-plane/internal/volume"
	"github.com/tidefly-oss/tidefly-plane/internal/webhook"
	"github.com/tidefly-oss/tidefly-plane/internal/ws"

	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

func jwtValidator(jwtSvc *auth.JWTService) func(string) (*middleware2.SessionUser, error) {
	return func(token string) (*middleware2.SessionUser, error) {
		claims, err := jwtSvc.ValidateAccessToken(token)
		if err != nil {
			return nil, err
		}
		return &middleware2.SessionUser{
			UserID: claims.UserID,
			Email:  claims.Email,
			Role:   claims.Role,
		}, nil
	}
}

func Register(
	api huma.API,
	r chi.Router,
	jwtSvc *auth.JWTService,
	tokenStore *auth.TokenStore,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	templateLoader *template.Loader,
	notifSvc *notification.Service,
	gitSvc *git.Service,
	webhookSvc *webhook.Service,
	riverClient *river.Client[pgx.Tx],
	notifier *notification.Notifier,
	metricsReg *metrics.Registry,
	caService *ca.Service,
	agentClient *agent.Client,
	bus *eventbus.Bus,
	ingressAdapter ingress.Adapter,
) {
	deployer := deploy.New(rt, db)

	access.SetUserReader(func(ctx context.Context) *access.UserInfo {
		u := middleware2.UserFromHumaCtx(ctx)
		if u == nil {
			return nil
		}
		return &access.UserInfo{UserID: u.UserID, Email: u.Email, Role: u.Role}
	})

	validate := jwtValidator(jwtSvc)

	setup.NewHandler(db).RegisterRoutes(api)

	requireAuth := middleware2.RequireAuthHuma(api, validate)
	requireAdmin := middleware2.RequireAdminHuma(api)
	sseAuth := middleware2.RequireAuthSSE(validate)

	mw := huma.Middlewares{requireAuth}
	adminMw := huma.Middlewares{requireAuth, requireAdmin}

	// ── WebSocket ─────────────────────────────────────────────────────────────
	ws.NewHandler(bus, jwtSvc, log, metricsReg).RegisterRoutes(r)

	// ── Events SSE ────────────────────────────────────────────────────────────
	events.NewHandler(rt).RegisterSSERoutes(r, sseAuth)

	// ── Auth ──────────────────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware2.RateLimitAuth())
		auth.NewHandler(db, jwtSvc, tokenStore, log).RegisterRoutes(api, mw)
	})

	// ── REST ──────────────────────────────────────────────────────────────────
	admin.NewHandler(db, log, notifier, caddy).RegisterRoutes(api, adminMw)
	git.NewHandler(gitSvc, db, log, bus).RegisterRoutes(api, mw)
	image.NewHandler(rt, bus).RegisterRoutes(api, mw, adminMw)
	network.NewHandler(rt, log, db, bus).RegisterRoutes(api, mw, adminMw)
	notification.NewHandler(notifSvc).RegisterRoutes(api, mw)
	project.NewHandler(db, rt, log).RegisterRoutes(api, mw)
	template.NewHandler(templateLoader).RegisterRoutes(api, mw)
	volume.NewHandler(rt, deployer, db, log, bus).RegisterRoutes(api, mw, adminMw)
	manifest.NewHandler(db, deployer, riverClient, log, gitSvc, templateLoader, rt, ingressAdapter).RegisterRoutes(api, mw)
	dashboard.NewHandler(rt, db, log, notifSvc, metricsReg).RegisterRoutes(api, mw)

	// ── REST + SSE ────────────────────────────────────────────────────────────
	agent.NewHandler(db, caService, agentClient, bus, log).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	backup.NewHandler(db).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	container.NewHandler(rt, db, log, caddy, bus).RegisterRoutes(api, r, mw, sseAuth)
	applog.NewHandler(db).RegisterRoutes(api, r, mw, adminMw, sseAuth)
	webhook.NewHandler(db, riverClient, log, webhookSvc).RegisterRoutes(api, r, mw)

	// ── System ────────────────────────────────────────────────────────────────
	sysh := system.NewHandler(rt, log, metricsReg, bus)
	sysh.RegisterRoutes(api, mw, adminMw)
	sysh.RegisterSSERoutes(r, sseAuth)
}
