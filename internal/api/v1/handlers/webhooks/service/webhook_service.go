package service

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type WebhookService struct {
	db *gorm.DB
}

func New(db *gorm.DB) *WebhookService {
	return &WebhookService{db: db}
}

func (s *WebhookService) CheckProjectAccess(ctx context.Context, projectID string) (*models.User, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Load full user from DB — needed for return value
	var user models.User
	if err := s.db.First(&user, "id = ?", claims.UserID).Error; err != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if user.IsAdmin() {
		return &user, nil
	}

	var count int64
	if err := s.db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, claims.UserID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	if count == 0 {
		return nil, huma.Error403Forbidden("not a member of this project")
	}
	return &user, nil
}

func (s *WebhookService) List(ctx context.Context, projectID string) ([]models.Webhook, error) {
	var webhooks []models.Webhook
	if err := s.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	return webhooks, nil
}

func (s *WebhookService) Create(ctx context.Context, wh *models.Webhook) error {
	return s.db.WithContext(ctx).Create(wh).Error
}

func (s *WebhookService) Load(ctx context.Context, id, projectID string) (*models.Webhook, error) {
	var wh models.Webhook
	if err := s.db.WithContext(ctx).
		First(&wh, "id = ? AND project_id = ?", id, projectID).Error; err != nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	return &wh, nil
}

func (s *WebhookService) Update(ctx context.Context, wh *models.Webhook, updates map[string]any) error {
	return s.db.WithContext(ctx).Model(wh).Updates(updates).Error
}

func (s *WebhookService) Delete(ctx context.Context, wh *models.Webhook) error {
	return s.db.WithContext(ctx).Delete(wh).Error
}

func (s *WebhookService) Deliveries(ctx context.Context, webhookID string) ([]models.WebhookDelivery, error) {
	var deliveries []models.WebhookDelivery
	if err := s.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC").Limit(50).
		Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	return deliveries, nil
}

func (s *WebhookService) CreateDelivery(ctx context.Context, d *models.WebhookDelivery) {
	s.db.WithContext(ctx).Create(d)
}

func (s *WebhookService) UpdateDelivery(ctx context.Context, d *models.WebhookDelivery, updates map[string]any) {
	s.db.WithContext(ctx).Model(d).Updates(updates)
}

func (s *WebhookService) LoadActive(ctx context.Context, id string) (*models.Webhook, error) {
	var wh models.Webhook
	if err := s.db.WithContext(ctx).First(&wh, "id = ? AND active = true", id).Error; err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}
	return &wh, nil
}

func (s *WebhookService) UpdateLastTriggered(ctx context.Context, wh *models.Webhook, status models.WebhookStatus) {
	s.db.WithContext(ctx).Model(wh).Updates(
		map[string]any{
			"last_triggered_at": gorm.Expr("NOW()"),
			"last_status":       status,
		},
	)
}
