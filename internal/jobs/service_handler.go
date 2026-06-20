package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	agentpb "github.com/tidefly-oss/tidefly-plane/internal/agent/proto"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"gorm.io/gorm"
)

type serviceLogger interface {
	Info(string, string, ...any)
	Warn(string, string, ...any)
	Error(string, string, error, ...any)
	Warnw(string, string, ...any)
}

type ServiceJobHandler struct {
	db          *gorm.DB
	rt          runtime.Runtime
	ingress     ingress.Adapter
	log         serviceLogger
	client      *asynq.Client
	agentClient *agent.Client
	notifSvc    *notification.Service
	notifier    *notification.Notifier
}

func NewServiceJobHandler(
	db *gorm.DB,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
	log serviceLogger,
	client *asynq.Client,
	agentClient *agent.Client,
	notifSvc *notification.Service,
	notifier *notification.Notifier,
) *ServiceJobHandler {
	return &ServiceJobHandler{
		db: db, rt: rt, ingress: ingressAdapter, log: log,
		client: client, agentClient: agentClient, notifSvc: notifSvc, notifier: notifier,
	}
}

func (h *ServiceJobHandler) markFailed(svc *models.Service, err error) {
	h.db.Model(svc).Updates(map[string]any{
		"status":     models.ServiceStatusFailed,
		"last_error": err.Error(),
	})
}

func (h *ServiceJobHandler) removeContainers(ctx context.Context, serviceName string) {
	containers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return
	}
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == serviceName || ct.Name == serviceName ||
			strings.HasPrefix(ct.Name, serviceName+"-") {
			_ = h.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
			_ = h.rt.DeleteContainer(ctx, ct.ID, true)
		}
	}
}

func (h *ServiceJobHandler) ensureNetwork(ctx context.Context, name string) error {
	if err := h.rt.CreateNetwork(ctx, name); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
			return nil
		}
		return err
	}
	return nil
}

func resolvedToDeployCmd(svc *models.Service, resolved *manifest.ResolvedManifest) *agentpb.CmdDeploy {
	env := make([]string, 0, len(resolved.Container.Env))
	for _, e := range resolved.Container.Env {
		env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}
	ports := make([]*agentpb.PortSpec, 0)
	if resolved.Expose.Port > 0 {
		ports = append(ports, &agentpb.PortSpec{
			ContainerPort: int32(resolved.Expose.Port),
			HostPort:      0,
			Protocol:      "tcp",
		})
	}
	return &agentpb.CmdDeploy{
		ProjectId:   svc.ProjectID,
		ServiceName: svc.Name,
		Image:       resolved.Container.Image,
		Env:         env,
		Ports:       ports,
		Labels: map[string]string{
			"tidefly.service":    svc.Name,
			"tidefly.service-id": svc.ID.String(),
			"tidefly.project":    svc.ProjectID,
		},
		Network: proxyNetwork,
	}
}
