package http

import "github.com/tidefly-oss/tidefly-plane/internal/models"

// ── Register ──────────────────────────────────────────────────────────────────

type RegisterInput struct {
	Body struct {
		Token        string `json:"token" required:"true"`
		WorkerID     string `json:"worker_id" required:"true"`
		Name         string `json:"name" required:"true"`
		Description  string `json:"description,omitempty"`
		AgentVersion string `json:"agent_version,omitempty"`
		OS           string `json:"os,omitempty"`
		Arch         string `json:"arch,omitempty"`
		RuntimeType  string `json:"runtime_type,omitempty"`
	}
}

type RegisterOutput struct {
	Body struct {
		WorkerID  string `json:"worker_id"`
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

// ── RenewCert ─────────────────────────────────────────────────────────────────

type RenewCertInput struct {
	Body struct {
		WorkerID string `json:"worker_id" required:"true"`
	}
}

type RenewCertOutput struct {
	Body struct {
		CertPEM   string `json:"cert_pem"`
		KeyPEM    string `json:"key_pem"`
		CACertPEM string `json:"ca_cert_pem"`
		ExpiresAt string `json:"expires_at"`
	}
}

// ── CreateToken ───────────────────────────────────────────────────────────────

type CreateTokenInput struct {
	Body struct {
		Label string `json:"label,omitempty"`
	}
}

type CreateTokenOutput struct {
	Body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
		Label     string `json:"label"`
	}
}

// ── ListTokens ────────────────────────────────────────────────────────────────

type ListTokensInput struct{}

type ListTokensOutput struct {
	Body []models.WorkerRegistrationToken
}

// ── ListWorkers ───────────────────────────────────────────────────────────────

type ListWorkersInput struct{}

type ListWorkersOutput struct {
	Body []models.WorkerNode
}

// ── RevokeWorker ──────────────────────────────────────────────────────────────

type RevokeWorkerInput struct {
	ID string `path:"id"`
}

type RevokeWorkerOutput struct{}

// ── DeleteWorker ──────────────────────────────────────────────────────────────

type DeleteWorkerInput struct {
	ID string `path:"id"`
}

type DeleteWorkerOutput struct{}
