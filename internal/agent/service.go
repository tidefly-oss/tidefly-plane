package agent

import (
	"errors"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_ca"
)

var (
	ErrWorkerNotFound   = errors.New("worker not found")
	ErrWorkerExists     = errors.New("worker already registered")
	ErrWorkerNotRevoked = errors.New("worker is not revoked")
	ErrInvalidToken     = errors.New("invalid registration token")
	ErrCertIssueFailed  = errors.New("failed to issue certificate")
)

type WorkerRegisterInput struct {
	Token        string
	WorkerID     string
	Name         string
	Description  string
	AgentVersion string
	OS           string
	Arch         string
	RuntimeType  string
}

type CertBundle struct {
	CertPEM   string
	KeyPEM    string
	CACertPEM string
	ExpiresAt time.Time
}

type Service struct {
	store     *Store
	caService *_ca.Service
}

func NewService(store *Store, caService *_ca.Service) *Service {
	return &Service{store: store, caService: caService}
}

func (s *Service) Register(input WorkerRegisterInput) (*models.WorkerNode, *CertBundle, error) {
	if s.store.ExistsByID(input.WorkerID) {
		return nil, nil, ErrWorkerExists
	}

	token, err := s.caService.ConsumeRegistrationToken(input.Token, input.WorkerID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	bundle, err := s.issueCert(input.WorkerID)
	if err != nil {
		return nil, nil, err
	}

	worker := &models.WorkerNode{
		ID:                 input.WorkerID,
		Name:               input.Name,
		Description:        input.Description,
		Status:             models.WorkerStatusPending,
		AgentVersion:       input.AgentVersion,
		OS:                 input.OS,
		Arch:               input.Arch,
		RuntimeType:        input.RuntimeType,
		RegisteredByUserID: token.CreatedByUserID,
	}
	if err := s.store.Create(worker); err != nil {
		return nil, nil, err
	}

	return worker, bundle, nil
}

func (s *Service) RenewCert(workerID string) (*CertBundle, error) {
	if _, err := s.store.FindActive(workerID); err != nil {
		return nil, ErrWorkerNotFound
	}
	return s.issueCert(workerID)
}

func (s *Service) CreateToken(userID, label string) (*models.WorkerRegistrationToken, error) {
	return s.caService.CreateRegistrationToken(userID, label)
}

func (s *Service) ListTokens(userID string) ([]models.WorkerRegistrationToken, error) {
	return s.caService.ListRegistrationTokens(userID)
}

func (s *Service) List() ([]models.WorkerNode, error) {
	return s.store.List()
}

func (s *Service) Revoke(workerID, byUserID string) error {
	if err := s.caService.RevokeWorkerCert(workerID, byUserID); err != nil {
		return fmt.Errorf("revoke cert: %w", err)
	}
	return s.store.Revoke(workerID)
}

func (s *Service) Delete(workerID string) (*models.WorkerNode, error) {
	worker, err := s.store.FindRevoked(workerID)
	if err != nil {
		return nil, ErrWorkerNotRevoked
	}
	if err := s.store.Delete(worker); err != nil {
		return nil, err
	}
	return worker, nil
}

// ── internal ──────────────────────────────────────────────────────────────────

func (s *Service) issueCert(workerID string) (*CertBundle, error) {
	issued, certPEM, keyPEM, err := s.caService.IssueWorkerCert(workerID)
	if err != nil {
		return nil, ErrCertIssueFailed
	}
	caCertPEM, err := s.caService.GetCACertPEM()
	if err != nil {
		return nil, ErrCertIssueFailed
	}
	return &CertBundle{
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		CACertPEM: caCertPEM,
		ExpiresAt: issued.NotAfter,
	}, nil
}
