package v1

import (
	"bytes"
	"io"
	"net/http"

	"github.com/aarondl/authboss/v3"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	adminhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin"
	authhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/auth"
	containerhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/containers"
	deployhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/events"
	githandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/git"
	imagehandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/images"
	loghandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/logs"
	networkhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/networks"
	notifhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/notifications"
	projecthandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/projects"
	systemhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/system"
	templatehandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/templates"
	volumehandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/volumes"
	webhookhandler "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/webhooks"
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
	notifierSvc *notifiersvc.Service,
) {
	deployer := deploy.New(rt, db)

	// ── Huma Middlewares ──────────────────────────────────────────────────────
	requireAuth := middleware.RequireAuthHuma(api, ab)
	requireAdmin := middleware.RequireAdminHuma(api, db)

	// ── Echo Auth Middleware (für SSE/WS Endpoints) ───────────────────────────
	echoAuth := middleware.RequireAuth(ab)
	echoInject := middleware.InjectUser(ab, db)

	// ── Handler Instanzen ─────────────────────────────────────────────────────
	sys := systemhandler.New(rt)
	sysMet := systemhandler.NewMetrics(db)
	ct := containerhandler.New(rt, deployer, db, log, traefik)
	img := imagehandler.New(rt)
	vol := volumehandler.New(rt, deployer, db, log)
	net := networkhandler.New(rt, log)
	proj := projecthandler.New(db, rt, log)
	tmpl := templatehandler.New(templateLoader)
	dep := deployhandler.New(db, deployer, templateLoader, log, traefik, notifSvc, notifierSvc)
	lg := loghandler.New(db)
	notif := notifhandler.New(notifSvc)
	evt := events.New(rt)
	gt := githandler.New(gitSvc, db, log)
	adm := adminhandler.New(db, log, notifierSvc)
	wh := webhookhandler.New(db, asynqClient, log, webhookSvc)
	authH := authhandler.New(ab, db, log)

	mw := func(extra ...func(huma.Context, func(huma.Context))) huma.Middlewares {
		base := huma.Middlewares{requireAuth}
		return append(base, extra...)
	}
	adminMw := mw(requireAdmin)

	// ── Auth (Authboss — Login/Logout) ────────────────────────────────────────
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
	huma.Register(api, op("auth-me", "GET", "/api/v1/auth/me", "Current user", mw()...), authH.CurrentUser)
	huma.Register(
		api, op("auth-change-password", "POST", "/api/v1/auth/change-password", "Change password", mw()...),
		authH.ChangePassword,
	)

	// ── System ────────────────────────────────────────────────────────────────
	huma.Register(api, op("system-health", "GET", "/api/v1/system/health", "Health check", mw()...), sys.Health)
	huma.Register(api, op("system-info", "GET", "/api/v1/system/info", "Runtime info", mw()...), sys.Info)
	huma.Register(
		api, op("system-overview", "GET", "/api/v1/system/overview", "Dashboard overview", mw()...), sys.Overview,
	)
	huma.Register(
		api, op("system-metrics", "GET", "/api/v1/system/metrics", "Historical metrics", mw()...), sysMet.Metrics,
	)
	huma.Register(api, op("system-ports", "GET", "/api/v1/system/ports", "Used host ports", mw()...), sys.UsedPorts)

	// ── Containers (Huma) ─────────────────────────────────────────────────────
	huma.Register(api, op("containers-list", "GET", "/api/v1/containers", "List containers", mw()...), ct.List)
	huma.Register(api, op("containers-get", "GET", "/api/v1/containers/{id}", "Get container", mw()...), ct.Get)
	huma.Register(
		api, op("containers-start", "POST", "/api/v1/containers/{id}/start", "Start container", mw()...), ct.Start,
	)
	huma.Register(
		api, op("containers-stop", "POST", "/api/v1/containers/{id}/stop", "Stop container", mw()...), ct.Stop,
	)
	huma.Register(
		api, op("containers-restart", "POST", "/api/v1/containers/{id}/restart", "Restart container", mw()...),
		ct.Restart,
	)
	huma.Register(
		api, op("containers-delete", "DELETE", "/api/v1/containers/{id}", "Delete container", mw()...), ct.Delete,
	)
	huma.Register(
		api, op("containers-get-resources", "GET", "/api/v1/containers/{id}/resources", "Get resource limits", mw()...),
		ct.GetResources,
	)
	huma.Register(
		api, op(
			"containers-update-resources", "PATCH", "/api/v1/containers/{id}/resources", "Update resource limits",
			mw()...,
		), ct.UpdateResources,
	)
	huma.Register(
		api, op("containers-compose", "POST", "/api/v1/containers/compose", "Deploy Compose stack", mw()...),
		ct.DeployCompose,
	)
	huma.Register(
		api, op("containers-delete-stack", "DELETE", "/api/v1/containers/stacks/{stack_id}", "Delete stack", mw()...),
		ct.DeleteStack,
	)

	// ── Containers (Echo — SSE/WS) ────────────────────────────────────────────
	e.GET("/api/v1/containers/:id/logs", ct.Logs, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/stats", ct.Stats, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/exec", ct.Exec, echoAuth, echoInject)
	e.POST("/api/v1/containers/dockerfile", ct.BuildAndDeploy, echoAuth, echoInject)

	// ── Images ────────────────────────────────────────────────────────────────
	huma.Register(api, op("images-list", "GET", "/api/v1/images", "List images", mw()...), img.List)
	huma.Register(
		api, op("images-containers", "GET", "/api/v1/images/{id}/containers", "Containers using image", mw()...),
		img.Containers,
	)
	huma.Register(api, op("images-delete", "DELETE", "/api/v1/images/{id}", "Delete image", adminMw...), img.Delete)

	// ── Volumes ───────────────────────────────────────────────────────────────
	huma.Register(api, op("volumes-list", "GET", "/api/v1/volumes", "List volumes", mw()...), vol.List)
	huma.Register(
		api, op("volumes-containers", "GET", "/api/v1/volumes/{id}/containers", "Containers using volume", mw()...),
		vol.Containers,
	)
	huma.Register(api, op("volumes-delete", "DELETE", "/api/v1/volumes/{id}", "Delete volume", adminMw...), vol.Delete)

	// ── Networks ──────────────────────────────────────────────────────────────
	huma.Register(api, op("networks-list", "GET", "/api/v1/networks", "List networks", mw()...), net.List)
	huma.Register(api, op("networks-get", "GET", "/api/v1/networks/{id}", "Get network", mw()...), net.Get)
	huma.Register(
		api, op("networks-containers", "GET", "/api/v1/networks/{id}/containers", "Network containers", mw()...),
		net.Containers,
	)
	huma.Register(
		api, op("networks-delete", "DELETE", "/api/v1/networks/{id}", "Delete network", adminMw...), net.Delete,
	)

	// ── Projects ──────────────────────────────────────────────────────────────
	huma.Register(api, op("projects-list", "GET", "/api/v1/projects", "List projects", mw()...), proj.List)
	huma.Register(api, op("projects-create", "POST", "/api/v1/projects", "Create project", mw()...), proj.Create)
	huma.Register(api, op("projects-get", "GET", "/api/v1/projects/{id}", "Get project", mw()...), proj.Get)
	huma.Register(api, op("projects-update", "PUT", "/api/v1/projects/{id}", "Update project", mw()...), proj.Update)
	huma.Register(api, op("projects-delete", "DELETE", "/api/v1/projects/{id}", "Delete project", mw()...), proj.Delete)
	huma.Register(
		api, op("projects-containers", "GET", "/api/v1/projects/{id}/containers", "Project containers", mw()...),
		proj.ListContainers,
	)

	// ── Templates ─────────────────────────────────────────────────────────────
	huma.Register(
		api, op("templates-list", "GET", "/api/v1/services/templates", "List templates", mw()...), tmpl.ListTemplates,
	)
	huma.Register(
		api, op("templates-get", "GET", "/api/v1/services/templates/{slug}", "Get template", mw()...), tmpl.GetTemplate,
	)

	// ── Deploy ────────────────────────────────────────────────────────────────
	huma.Register(api, op("deploy-list", "GET", "/api/v1/deploy", "List services", mw()...), dep.ListServices)
	huma.Register(api, op("deploy-create", "POST", "/api/v1/deploy", "Deploy service", mw()...), dep.DeployService)
	huma.Register(
		api, op("deploy-delete", "DELETE", "/api/v1/deploy/{id}", "Delete service", mw()...), dep.DeleteService,
	)
	huma.Register(
		api, op(
			"deploy-credentials-shown", "POST", "/api/v1/deploy/{id}/credentials/shown", "Mark credentials shown",
			mw()...,
		), dep.MarkCredentialsShown,
	)

	// ── Logs (Huma) ───────────────────────────────────────────────────────────
	huma.Register(api, op("logs-app", "GET", "/api/v1/logs/app", "App logs", mw()...), lg.ListAppLogs)
	huma.Register(api, op("logs-audit", "GET", "/api/v1/logs/audit", "Audit logs", adminMw...), lg.ListAuditLogs)

	// ── Logs (Echo — SSE) ─────────────────────────────────────────────────────
	e.GET("/api/v1/logs/app/stream", lg.StreamAppLogs, echoAuth, echoInject)

	// ── Notifications (Huma) ──────────────────────────────────────────────────
	huma.Register(api, op("notif-list", "GET", "/api/v1/notifications", "List notifications", mw()...), notif.List)
	huma.Register(
		api, op("notif-list-all", "GET", "/api/v1/notifications/all", "All notifications", mw()...), notif.ListAll,
	)
	huma.Register(
		api, op("notif-count", "GET", "/api/v1/notifications/count", "Notification count", mw()...), notif.Count,
	)
	huma.Register(
		api, op("notif-ack", "POST", "/api/v1/notifications/{id}/acknowledge", "Acknowledge", mw()...),
		notif.Acknowledge,
	)
	huma.Register(
		api, op("notif-delete", "DELETE", "/api/v1/notifications/{id}", "Delete notification", mw()...), notif.Delete,
	)
	huma.Register(
		api, op("notif-delete-acked", "DELETE", "/api/v1/notifications/acknowledged", "Delete acknowledged", mw()...),
		notif.DeleteAcknowledged,
	)

	// ── Notifications (Echo — SSE) ────────────────────────────────────────────
	e.GET("/api/v1/notifications/stream", notif.Stream, echoAuth, echoInject)

	// ── Events (Echo — SSE) ───────────────────────────────────────────────────
	e.GET("/api/v1/events/stream", evt.Stream, echoAuth, echoInject)

	// ── Git ───────────────────────────────────────────────────────────────────
	huma.Register(api, op("git-list", "GET", "/api/v1/git/integrations", "List integrations", mw()...), gt.List)
	huma.Register(api, op("git-create", "POST", "/api/v1/git/integrations", "Create integration", mw()...), gt.Create)
	huma.Register(api, op("git-get", "GET", "/api/v1/git/integrations/{id}", "Get integration", mw()...), gt.Get)
	huma.Register(
		api, op("git-delete", "DELETE", "/api/v1/git/integrations/{id}", "Delete integration", mw()...), gt.Delete,
	)
	huma.Register(
		api, op("git-validate", "POST", "/api/v1/git/integrations/{id}/validate", "Validate token", mw()...),
		gt.Validate,
	)
	huma.Register(
		api, op("git-repos", "GET", "/api/v1/git/integrations/{id}/repositories", "List repositories", mw()...),
		gt.ListRepositories,
	)
	huma.Register(
		api, op("git-shares", "PUT", "/api/v1/git/integrations/{id}/shares", "Set shares", mw()...), gt.SetShares,
	)
	huma.Register(
		api, op(
			"git-branches", "GET", "/api/v1/git/integrations/{id}/repositories/{owner}/{repo}/branches",
			"List branches", mw()...,
		), gt.ListBranches,
	)

	// ── Admin ─────────────────────────────────────────────────────────────────
	huma.Register(api, op("admin-users-list", "GET", "/api/v1/admin/users", "List users", adminMw...), adm.ListUsers)
	huma.Register(
		api, op("admin-users-create", "POST", "/api/v1/admin/users", "Create user", adminMw...), adm.CreateUser,
	)
	huma.Register(api, op("admin-users-get", "GET", "/api/v1/admin/users/{id}", "Get user", adminMw...), adm.GetUser)
	huma.Register(
		api, op("admin-users-update", "PATCH", "/api/v1/admin/users/{id}", "Update user", adminMw...), adm.UpdateUser,
	)
	huma.Register(
		api, op("admin-users-delete", "DELETE", "/api/v1/admin/users/{id}", "Delete user", adminMw...), adm.DeleteUser,
	)
	huma.Register(
		api,
		op("admin-users-reset-pw", "POST", "/api/v1/admin/users/{id}/reset-password", "Reset password", adminMw...),
		adm.ResetUserPassword,
	)
	huma.Register(
		api, op("admin-users-projects", "PUT", "/api/v1/admin/users/{id}/projects", "Set project members", adminMw...),
		adm.SetProjectMembers,
	)
	huma.Register(
		api, op("admin-settings-get", "GET", "/api/v1/admin/settings", "Get settings", adminMw...), adm.GetSettings,
	)
	huma.Register(
		api, op("admin-settings-update", "PATCH", "/api/v1/admin/settings", "Update settings", adminMw...),
		adm.UpdateSettings,
	)
	huma.Register(
		api, op(
			"admin-settings-test", "POST", "/api/v1/admin/settings/test/{channel}", "Test notification channel",
			adminMw...,
		),
		adm.TestNotification,
	)

	// ── Webhooks (Huma — Management) ──────────────────────────────────────────
	huma.Register(api, op("webhooks-list", "GET", "/api/v1/projects/{pid}/webhooks", "List webhooks", mw()...), wh.List)
	huma.Register(
		api, op("webhooks-create", "POST", "/api/v1/projects/{pid}/webhooks", "Create webhook", mw()...), wh.Create,
	)
	huma.Register(
		api, op("webhooks-get", "GET", "/api/v1/projects/{pid}/webhooks/{id}", "Get webhook", mw()...), wh.Get,
	)
	huma.Register(
		api, op("webhooks-update", "PATCH", "/api/v1/projects/{pid}/webhooks/{id}", "Update webhook", mw()...),
		wh.Update,
	)
	huma.Register(
		api, op("webhooks-delete", "DELETE", "/api/v1/projects/{pid}/webhooks/{id}", "Delete webhook", mw()...),
		wh.Delete,
	)
	huma.Register(
		api, op("webhooks-rotate", "POST", "/api/v1/projects/{pid}/webhooks/{id}/rotate", "Rotate secret", mw()...),
		wh.RotateSecret,
	)
	huma.Register(
		api,
		op("webhooks-deliveries", "GET", "/api/v1/projects/{pid}/webhooks/{id}/deliveries", "List deliveries", mw()...),
		wh.Deliveries,
	)

	// ── Webhooks (Echo — public receiver, kein Auth) ──────────────────────────
	e.POST("/webhooks/:id", wh.Receive)
}

// op ist ein Hilfskonstruktor für huma.Operation um Boilerplate zu reduzieren.
func op(id, method, path, summary string, mws ...func(huma.Context, func(huma.Context))) huma.Operation {
	return huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Middlewares: mws,
	}
}

// ensure net/http is used
var _ http.Handler
