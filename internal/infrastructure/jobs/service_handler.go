package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/converter"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

const (
	TaskServiceDeploy   = "service:deploy"
	TaskServiceRedeploy = "service:redeploy"
	TaskServiceUpdate   = "service:update"
	TaskServiceDelete   = "service:delete"
)

// ── Payloads ──────────────────────────────────────────────────────────────────

type ServiceDeployPayload struct {
	ServiceID string             `json:"service_id"`
	Input     converter.APIInput `json:"input"`
	GitToken  string             `json:"git_token"`
}

type ServiceRedeployPayload struct {
	ServiceID     string `json:"service_id"`
	ImageOverride string `json:"image_override,omitempty"`
	GitToken      string `json:"git_token,omitempty"`
}

type ServiceUpdatePayload struct {
	ServiceID string `json:"service_id"`
	Image     string `json:"image,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Replicas  int    `json:"replicas,omitempty"`
}

type ServiceDeletePayload struct {
	ServiceID string `json:"service_id"`
}

// ── Enqueue helpers ───────────────────────────────────────────────────────────

func EnqueueServiceDeploy(client *asynq.Client, serviceID string, input converter.APIInput, gitToken string) error {
	data, err := json.Marshal(ServiceDeployPayload{ServiceID: serviceID, Input: input, GitToken: gitToken})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceDeploy, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

func EnqueueServiceRedeploy(client *asynq.Client, serviceID, imageOverride string) error {
	data, err := json.Marshal(ServiceRedeployPayload{ServiceID: serviceID, ImageOverride: imageOverride})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceRedeploy, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

func EnqueueServiceUpdate(client *asynq.Client, serviceID, image, domain string, replicas int) error {
	data, err := json.Marshal(ServiceUpdatePayload{ServiceID: serviceID, Image: image, Domain: domain, Replicas: replicas})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceUpdate, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

func EnqueueServiceDelete(client *asynq.Client, serviceID string) error {
	data, err := json.Marshal(ServiceDeletePayload{ServiceID: serviceID})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceDelete, data,
		asynq.MaxRetry(0), asynq.Timeout(5*time.Minute), asynq.Queue("critical")))
	return err
}

// ── Handler ───────────────────────────────────────────────────────────────────

type serviceLogger interface {
	Info(string, string, ...any)
	Warn(string, string, ...any)
	Error(string, string, error, ...any)
	Warnw(string, string, ...any)
}

type ServiceJobHandler struct {
	db      *gorm.DB
	rt      runtime.Runtime
	ingress ingress.Adapter
	log     serviceLogger
	client  *asynq.Client
}

func NewServiceJobHandler(db *gorm.DB, rt runtime.Runtime, ingressAdapter ingress.Adapter, log serviceLogger, client *asynq.Client) *ServiceJobHandler {
	return &ServiceJobHandler{db: db, rt: rt, ingress: ingressAdapter, log: log, client: client}
}

// ── Shared helpers ────────────────────────────────────────────────────────────

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
