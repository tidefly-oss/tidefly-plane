package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/crypto"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

const (
	caValidityYears  = 10
	certValidityDays = 90
	renewBeforeDays  = 30

	caSubject   = "Tidefly Internal CA"
	caOrg       = "Tidefly"
	certOrg     = "Tidefly"
	keyBits     = 4096 // CA key
	certKeyBits = 2048 // issued cert keys
)

// Service manages the Tidefly internal CA.
type Service struct {
	db            *gorm.DB
	encryptionKey []byte // 32-byte AES key from config
}

func New(db *gorm.DB, encryptionKey []byte) *Service {
	return &Service{db: db, encryptionKey: encryptionKey}
}

// Init ensures a CA exists in the database.
// If none exists, it creates one. Safe to call on every startup.
func (s *Service) Init() error {
	var count int64
	s.db.Model(&models.CertificateAuthority{}).Count(&count)
	if count > 0 {
		return nil
	}
	return s.createCA()
}

// GetTLSConfig returns a *tls.Config for the gRPC server that requires
// and validates worker client certificates signed by our CA.
func (s *Service) GetTLSConfig() (*tls.Config, error) {
	caCert, caKey, err := s.loadCA()
	if err != nil {
		return nil, fmt.Errorf("ca: load CA: %w", err)
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(caCert)

	tlsCert := tls.Certificate{
		Certificate: [][]byte{caCert.Raw},
		PrivateKey:  caKey,
		Leaf:        caCert,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// IssueWorkerCert issues a new mTLS client certificate for a worker.
// ownerID is the worker's UUID. Returns the cert and key as PEM strings.
func (s *Service) IssueWorkerCert(ownerID string) (*models.IssuedCertificate, string, string, error) {
	return s.issueCert("worker", ownerID, fmt.Sprintf("tidefly-worker-%s", ownerID))
}

// IssuePlaneCert issues the plane's own server certificate.
func (s *Service) IssuePlaneCert(dnsNames []string) (*models.IssuedCertificate, string, string, error) {
	issued, certPEM, keyPEM, err := s.issueCert("plane", "plane", "tidefly-plane")
	if err != nil {
		return nil, "", "", err
	}
	return issued, certPEM, keyPEM, nil
}

// RevokeWorkerCert marks a worker's certificate as revoked.
func (s *Service) RevokeWorkerCert(workerID string, revokedBy string) error {
	now := time.Now()
	return s.db.Model(&models.IssuedCertificate{}).
		Where("owner_type = ? AND owner_id = ? AND revoked = false", "worker", workerID).
		Updates(map[string]any{
			"revoked":    true,
			"revoked_at": now,
			"revoked_by": revokedBy,
		}).Error
}

// RenewExpiring finds all certs expiring within renewBeforeDays and reissues them.
// Called by the background renewal job.
func (s *Service) RenewExpiring() error {
	threshold := time.Now().Add(renewBeforeDays * 24 * time.Hour)

	var expiring []models.IssuedCertificate
	if err := s.db.Where(
		"revoked = false AND not_after < ? AND renewed_to_id IS NULL",
		threshold,
	).Find(&expiring).Error; err != nil {
		return fmt.Errorf("ca: query expiring certs: %w", err)
	}

	for _, old := range expiring {
		newIssued, _, _, err := s.issueCert(old.OwnerType, old.OwnerID, old.Subject)
		if err != nil {
			return fmt.Errorf("ca: renew cert for %s/%s: %w", old.OwnerType, old.OwnerID, err)
		}

		// Link old → new
		if err := s.db.Model(&old).Updates(map[string]any{
			"renewed_to_id": newIssued.ID,
		}).Error; err != nil {
			return fmt.Errorf("ca: link renewed cert: %w", err)
		}
		newIssued.RenewedFromID = &old.ID
		s.db.Save(&newIssued)
	}

	return nil
}

// GetWorkerCertPEM returns the decrypted cert and key PEM for a worker.
// Used when the worker reconnects and needs its cert refreshed.
func (s *Service) GetWorkerCertPEM(workerID string) (certPEM, keyPEM string, err error) {
	var issued models.IssuedCertificate
	if err := s.db.Where(
		"owner_type = ? AND owner_id = ? AND revoked = false AND renewed_to_id IS NULL",
		"worker", workerID,
	).Order("created_at DESC").First(&issued).Error; err != nil {
		return "", "", fmt.Errorf("ca: find worker cert: %w", err)
	}

	certPEM, err = crypto.DecryptString(s.encryptionKey, issued.CertPEM)
	if err != nil {
		return "", "", fmt.Errorf("ca: decrypt cert: %w", err)
	}
	keyPEM, err = crypto.DecryptString(s.encryptionKey, issued.KeyPEM)
	if err != nil {
		return "", "", fmt.Errorf("ca: decrypt key: %w", err)
	}
	return certPEM, keyPEM, nil
}

// GetCACertPEM returns the CA certificate as PEM (public, not encrypted).
// Sent to workers during registration so they can verify the plane.
func (s *Service) GetCACertPEM() (string, error) {
	var ca models.CertificateAuthority
	if err := s.db.First(&ca).Error; err != nil {
		return "", fmt.Errorf("ca: load CA record: %w", err)
	}
	certPEM, err := crypto.DecryptString(s.encryptionKey, ca.CertPEM)
	if err != nil {
		return "", fmt.Errorf("ca: decrypt CA cert: %w", err)
	}
	return certPEM, nil
}

// ── private ──────────────────────────────────────────────────────────────────

func (s *Service) createCA() error {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return fmt.Errorf("ca: generate key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caSubject,
			Organization: []string{caOrg},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(caValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("ca: create certificate: %w", err)
	}

	certPEM := pemEncodeCert(certDER)
	keyPEM := pemEncodeKey(key)

	encCert, err := crypto.EncryptString(s.encryptionKey, certPEM)
	if err != nil {
		return fmt.Errorf("ca: encrypt cert: %w", err)
	}
	encKey, err := crypto.EncryptString(s.encryptionKey, keyPEM)
	if err != nil {
		return fmt.Errorf("ca: encrypt key: %w", err)
	}

	record := &models.CertificateAuthority{
		CertPEM:   encCert,
		KeyPEM:    encKey,
		Subject:   caSubject,
		NotBefore: template.NotBefore,
		NotAfter:  template.NotAfter,
		Serial:    serial.String(),
	}

	return s.db.Create(record).Error
}

func (s *Service) issueCert(ownerType, ownerID, commonName string) (*models.IssuedCertificate, string, string, error) {
	caCert, caKey, err := s.loadCA()
	if err != nil {
		return nil, "", "", err
	}

	key, err := rsa.GenerateKey(rand.Reader, certKeyBits)
	if err != nil {
		return nil, "", "", fmt.Errorf("ca: generate cert key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, "", "", err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{certOrg},
		},
		NotBefore: now,
		NotAfter:  now.Add(certValidityDays * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, "", "", fmt.Errorf("ca: sign certificate: %w", err)
	}

	certPEM := pemEncodeCert(certDER)
	keyPEM := pemEncodeKey(key)

	encCert, err := crypto.EncryptString(s.encryptionKey, certPEM)
	if err != nil {
		return nil, "", "", fmt.Errorf("ca: encrypt issued cert: %w", err)
	}
	encKey, err := crypto.EncryptString(s.encryptionKey, keyPEM)
	if err != nil {
		return nil, "", "", fmt.Errorf("ca: encrypt issued key: %w", err)
	}

	issued := &models.IssuedCertificate{
		OwnerType: ownerType,
		OwnerID:   ownerID,
		CertPEM:   encCert,
		KeyPEM:    encKey,
		Subject:   commonName,
		NotBefore: template.NotBefore,
		NotAfter:  template.NotAfter,
		Serial:    serial.String(),
	}

	if err := s.db.Create(issued).Error; err != nil {
		return nil, "", "", fmt.Errorf("ca: save issued cert: %w", err)
	}

	return issued, certPEM, keyPEM, nil
}

func (s *Service) loadCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	var record models.CertificateAuthority
	if err := s.db.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("ca: no CA found — call Init() first")
		}
		return nil, nil, fmt.Errorf("ca: load CA record: %w", err)
	}

	certPEM, err := crypto.DecryptString(s.encryptionKey, record.CertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("ca: decrypt CA cert: %w", err)
	}
	keyPEM, err := crypto.DecryptString(s.encryptionKey, record.KeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("ca: decrypt CA key: %w", err)
	}

	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, nil, err
	}
	key, err := parseKeyPEM(keyPEM)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

// ── PEM helpers ───────────────────────────────────────────────────────────────

func pemEncodeCert(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func pemEncodeKey(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func parseCertPEM(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("ca: failed to decode cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse certificate: %w", err)
	}
	return cert, nil
}

func parseKeyPEM(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("ca: failed to decode key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse private key: %w", err)
	}
	return key, nil
}

func randomSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca: generate serial: %w", err)
	}
	return serial, nil
}
