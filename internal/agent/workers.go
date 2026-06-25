package agent

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
)

// ── Register ──────────────────────────────────────────────────────────────────

type workerRegisterInput struct {
	Body struct {
		Token        string `json:"token"                   required:"true"`
		WorkerID     string `json:"worker_id"               required:"true"`
		Name         string `json:"name"                    required:"true"`
		Description  string `json:"description,omitempty"`
		AgentVersion string `json:"agent_version,omitempty"`
		OS           string `json:"os,omitempty"`
		Arch         string `json:"arch,omitempty"`
		RuntimeType  string `json:"runtime_type,omitempty"`
	}
}

type registerOutput struct {
	Body struct {
		WorkerID  string `json:"worker_id"`
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

func (h *Handler) register(_ context.Context, input *workerRegisterInput) (*registerOutput, error) {
	_, bundle, err := h.svc.Register(WorkerRegisterInput{
		Token:        input.Body.Token,
		WorkerID:     input.Body.WorkerID,
		Name:         input.Body.Name,
		Description:  input.Body.Description,
		AgentVersion: input.Body.AgentVersion,
		OS:           input.Body.OS,
		Arch:         input.Body.Arch,
		RuntimeType:  input.Body.RuntimeType,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkerExists):
			return nil, huma.Error409Conflict("worker already registered")
		case errors.Is(err, ErrInvalidToken):
			return nil, huma.Error401Unauthorized(err.Error())
		default:
			h.log.Error("agent", "register failed", err)
			return nil, huma.Error500InternalServerError("registration failed")
		}
	}

	out := &registerOutput{}
	out.Body.WorkerID = input.Body.WorkerID
	out.Body.CertPEM = bundle.CertPEM
	out.Body.KeyPEM = bundle.KeyPEM
	out.Body.CACertPEM = bundle.CACertPEM
	out.Body.ExpiresAt = bundle.ExpiresAt.Format(time.RFC3339)
	return out, nil
}

// ── RenewCert ─────────────────────────────────────────────────────────────────

type renewCertInput struct {
	Body struct {
		WorkerID string `json:"worker_id" required:"true"`
	}
}

type renewCertOutput struct {
	Body struct {
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

func (h *Handler) renewCert(_ context.Context, input *renewCertInput) (*renewCertOutput, error) {
	bundle, err := h.svc.RenewCert(input.Body.WorkerID)
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			return nil, huma.Error404NotFound("worker not found")
		}
		h.log.Error("agent", "renew cert failed", err)
		return nil, huma.Error500InternalServerError("failed to renew certificate")
	}

	out := &renewCertOutput{}
	out.Body.CertPEM = bundle.CertPEM
	out.Body.KeyPEM = bundle.KeyPEM
	out.Body.CACertPEM = bundle.CACertPEM
	out.Body.ExpiresAt = bundle.ExpiresAt.Format(time.RFC3339)
	return out, nil
}

// ── CreateToken ───────────────────────────────────────────────────────────────

type createTokenInput struct {
	Body struct {
		Label string `json:"label,omitempty"`
	}
}

type createTokenOutput struct {
	Body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
		Label     string `json:"label"`
	}
}

func (h *Handler) createToken(ctx context.Context, input *createTokenInput) (*createTokenOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	token, err := h.svc.CreateToken(claims.UserID, input.Body.Label)
	if err != nil {
		h.log.Error("agent", "create token failed", err)
		return nil, huma.Error500InternalServerError("failed to create token")
	}

	out := &createTokenOutput{}
	out.Body.Token = token.Token
	out.Body.ExpiresAt = token.ExpiresAt.Format(time.RFC3339)
	out.Body.Label = token.Label
	return out, nil
}

// ── ListTokens ────────────────────────────────────────────────────────────────

type listTokensOutput struct {
	Body []models.WorkerRegistrationToken
}

func (h *Handler) listTokens(ctx context.Context, _ *struct{}) (*listTokensOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	tokens, err := h.svc.ListTokens(claims.UserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list tokens")
	}
	return &listTokensOutput{Body: tokens}, nil
}

// ── ListWorkers ───────────────────────────────────────────────────────────────

type listWorkersOutput struct {
	Body []models.WorkerNode
}

func (h *Handler) listWorkers(_ context.Context, _ *struct{}) (*listWorkersOutput, error) {
	workers, err := h.svc.List()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workers")
	}
	return &listWorkersOutput{Body: workers}, nil
}

// ── RevokeWorker ──────────────────────────────────────────────────────────────

type revokeWorkerInput struct {
	ID string `path:"id"`
}

func (h *Handler) revokeWorker(ctx context.Context, input *revokeWorkerInput) (*struct{}, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.svc.Revoke(input.ID, claims.UserID); err != nil {
		h.log.Error("agent", "revoke worker failed", err)
		return nil, huma.Error500InternalServerError("failed to revoke worker")
	}
	h.bus.Publish(_eventbus.Event{
		Type:    _eventbus.EventWorkerUpdated,
		Topic:   _eventbus.TopicWorkers,
		Payload: _eventbus.WorkerUpdatedPayload{ID: input.ID, Status: "revoked"},
	})
	return &struct{}{}, nil
}

// ── DeleteWorker ──────────────────────────────────────────────────────────────

type deleteWorkerInput struct {
	ID string `path:"id"`
}

func (h *Handler) deleteWorker(ctx context.Context, input *deleteWorkerInput) (*struct{}, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	worker, err := h.svc.Delete(input.ID)
	if err != nil {
		if errors.Is(err, ErrWorkerNotRevoked) {
			return nil, huma.Error404NotFound("worker not found or not revoked")
		}
		h.log.Error("agent", "delete worker failed", err)
		return nil, huma.Error500InternalServerError("failed to delete worker")
	}
	h.bus.Publish(_eventbus.Event{
		Type:    _eventbus.EventWorkerUpdated,
		Topic:   _eventbus.TopicWorkers,
		Payload: _eventbus.WorkerUpdatedPayload{ID: worker.ID, Status: "deleted"},
	})
	return &struct{}{}, nil
}
