package http

import "github.com/tidefly-oss/tidefly-plane/internal/models"

// ── Register ──────────────────────────────────────────────────────────────────

type AgentRegisterInput struct {
	Body struct {
		Token        string `json:"token"                 required:"true"`
		WorkerID     string `json:"worker_id"             required:"true"`
		Name         string `json:"name"                  required:"true"`
		Description  string `json:"description,omitempty"`
		AgentVersion string `json:"agent_version,omitempty"`
		OS           string `json:"os,omitempty"`
		Arch         string `json:"arch,omitempty"`
		RuntimeType  string `json:"runtime_type,omitempty"`
	}
}

type AgentRegisterOutput struct {
	Body struct {
		WorkerID  string `json:"worker_id"`
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

// ── RenewCert ─────────────────────────────────────────────────────────────────

type AgentRenewCertInput struct {
	Body struct {
		WorkerID string `json:"worker_id" required:"true"`
	}
}

type AgentRenewCertOutput struct {
	Body struct {
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

// ── CreateToken ───────────────────────────────────────────────────────────────

type AgentCreateTokenInput struct {
	Body struct {
		Label string `json:"label,omitempty"`
	}
}

type AgentCreateTokenOutput struct {
	Body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
		Label     string `json:"label"`
	}
}

// ── ListTokens ────────────────────────────────────────────────────────────────

type AgentListTokensInput struct{}
type AgentListTokensOutput struct {
	Body []models.WorkerRegistrationToken
}

// ── ListWorkers ───────────────────────────────────────────────────────────────

type AgentListWorkersInput struct{}
type AgentListWorkersOutput struct {
	Body []models.WorkerNode
}

// ── RevokeWorker ──────────────────────────────────────────────────────────────

type AgentRevokeWorkerInput struct {
	ID string `path:"id"`
}
type AgentRevokeWorkerOutput struct{}

// ── DeleteWorker ──────────────────────────────────────────────────────────────

type AgentDeleteWorkerInput struct {
	ID string `path:"id"`
}
type AgentDeleteWorkerOutput struct{}

// ── ListWorkerContainers ──────────────────────────────────────────────────────

type AgentListWorkerContainersInput struct {
	ID string `path:"id"`
}
