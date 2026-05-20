package http

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/agent/repository"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"gorm.io/gorm"
)

type Handler struct {
	workers     *repository.WorkerRepository
	caService   *ca.Service
	agentClient *agentsvc.Client
	bus         *eventbus.Bus
}

func New(db *gorm.DB, caService *ca.Service, agentClient *agentsvc.Client, bus *eventbus.Bus) *Handler {
	return &Handler{
		workers:     repository.NewWorkerRepository(db),
		caService:   caService,
		agentClient: agentClient,
		bus:         bus,
	}
}

func (h *Handler) Register(_ context.Context, input *RegisterInput) (*RegisterOutput, error) {
	if h.workers.ExistsByID(input.Body.WorkerID) {
		return nil, huma.Error409Conflict("worker already registered")
	}

	token, err := h.caService.ConsumeRegistrationToken(input.Body.Token, input.Body.WorkerID)
	if err != nil {
		return nil, huma.Error401Unauthorized(err.Error())
	}

	issued, certPEM, keyPEM, err := h.caService.IssueWorkerCert(input.Body.WorkerID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to issue certificate")
	}

	caCertPEM, err := h.caService.GetCACertPEM()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to retrieve CA cert")
	}

	worker := &models.WorkerNode{
		ID:                 input.Body.WorkerID,
		Name:               input.Body.Name,
		Description:        input.Body.Description,
		Status:             models.WorkerStatusPending,
		AgentVersion:       input.Body.AgentVersion,
		OS:                 input.Body.OS,
		Arch:               input.Body.Arch,
		RuntimeType:        input.Body.RuntimeType,
		RegisteredByUserID: token.CreatedByUserID,
	}
	if err := h.workers.Create(worker); err != nil {
		return nil, huma.Error500InternalServerError("failed to create worker record")
	}

	out := &RegisterOutput{}
	out.Body.WorkerID = input.Body.WorkerID
	out.Body.CertPEM = certPEM
	out.Body.KeyPEM = keyPEM
	out.Body.CACertPEM = caCertPEM
	out.Body.ExpiresAt = issued.NotAfter.Format(time.RFC3339)
	return out, nil
}

func (h *Handler) RenewCert(_ context.Context, input *RenewCertInput) (*RenewCertOutput, error) {
	if _, err := h.workers.FindActive(input.Body.WorkerID); err != nil {
		return nil, huma.Error404NotFound("worker not found")
	}

	issued, certPEM, keyPEM, err := h.caService.IssueWorkerCert(input.Body.WorkerID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to issue certificate")
	}

	caCertPEM, err := h.caService.GetCACertPEM()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to retrieve CA cert")
	}

	out := &RenewCertOutput{}
	out.Body.CertPEM = certPEM
	out.Body.KeyPEM = keyPEM
	out.Body.CACertPEM = caCertPEM
	out.Body.ExpiresAt = issued.NotAfter.Format(time.RFC3339)
	return out, nil
}

func (h *Handler) CreateToken(ctx context.Context, input *CreateTokenInput) (*CreateTokenOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	token, err := h.caService.CreateRegistrationToken(claims.UserID, input.Body.Label)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create token")
	}

	out := &CreateTokenOutput{}
	out.Body.Token = token.Token
	out.Body.ExpiresAt = token.ExpiresAt.Format(time.RFC3339)
	out.Body.Label = token.Label
	return out, nil
}

func (h *Handler) ListTokens(ctx context.Context, _ *ListTokensInput) (*ListTokensOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	tokens, err := h.caService.ListRegistrationTokens(claims.UserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list tokens")
	}

	return &ListTokensOutput{Body: tokens}, nil
}

func (h *Handler) ListWorkers(_ context.Context, _ *ListWorkersInput) (*ListWorkersOutput, error) {
	workers, err := h.workers.List()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workers")
	}
	return &ListWorkersOutput{Body: workers}, nil
}

func (h *Handler) RevokeWorker(ctx context.Context, input *RevokeWorkerInput) (*RevokeWorkerOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.caService.RevokeWorkerCert(input.ID, claims.UserID); err != nil {
		return nil, huma.Error500InternalServerError("failed to revoke certificate")
	}
	if err := h.workers.Revoke(input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to revoke worker")
	}
	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventWorkerUpdated,
		Topic:   eventbus.TopicWorkers,
		Payload: eventbus.WorkerUpdatedPayload{ID: input.ID, Status: "revoked"},
	})
	return &RevokeWorkerOutput{}, nil
}

func (h *Handler) DeleteWorker(ctx context.Context, input *DeleteWorkerInput) (*DeleteWorkerOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	worker, err := h.workers.FindRevoked(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("worker not found or not revoked")
	}
	if err := h.workers.Delete(worker); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete worker")
	}
	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventWorkerUpdated,
		Topic:   eventbus.TopicWorkers,
		Payload: eventbus.WorkerUpdatedPayload{ID: input.ID, Status: "deleted"},
	})
	return &DeleteWorkerOutput{}, nil
}
