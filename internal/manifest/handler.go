package manifest

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"github.com/tidefly-oss/tidefly-plane/internal/template"
	"gorm.io/gorm"
)

type Handler struct {
	mgr        *Manager
	log        *applogger.Logger
	templateLd *template.Loader
}

func NewHandler(
	db *gorm.DB,
	deployer *deploy.Deployer,
	queue *river.Client[pgx.Tx],
	log *applogger.Logger,
	gitSvc *git.Service,
	templateLd *template.Loader,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
) *Handler {
	return &Handler{
		mgr:        NewManager(db, deployer, queue, log, gitSvc, rt, ingressAdapter),
		log:        log,
		templateLd: templateLd,
	}
}
