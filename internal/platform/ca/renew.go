package ca

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

const (
	renewCheckInterval = 24 * time.Hour
)

// StartRenewalJob starts a background goroutine that checks for expiring
// certificates every 24 hours and automatically renews them.
// Call this once on startup after ca.Init().
func (s *Service) StartRenewalJob(ctx context.Context) {
	go func() {
		if err := s.runRenewal(); err != nil {
			slog.Error("ca: initial renewal check failed", "error", err)
		}
		ticker := time.NewTicker(renewCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("ca: renewal job stopped")
				return
			case <-ticker.C:
				if err := s.runRenewal(); err != nil {
					slog.Error("ca: renewal check failed", "error", err)
				}
			}
		}
	}()
}

func (s *Service) runRenewal() error {
	slog.Info("ca: checking for expiring certificates")
	if err := s.RenewExpiring(); err != nil {
		return fmt.Errorf("renew expiring certs: %w", err)
	}
	s.checkCAExpiry()
	slog.Info("ca: renewal check complete")
	return nil
}

func (s *Service) checkCAExpiry() {
	var ca models.CertificateAuthority
	if err := s.db.Select("not_after").First(&ca).Error; err != nil {
		slog.Error("ca: could not check CA expiry", "error", err)
		return
	}
	daysLeft := int(time.Until(ca.NotAfter).Hours() / 24)
	if daysLeft < 365 {
		slog.Warn("ca: CA certificate expiring soon — manual renewal required",
			"days_left", daysLeft,
			"expires_at", ca.NotAfter,
		)
	}
}
