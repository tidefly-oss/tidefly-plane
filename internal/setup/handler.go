package setup

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

type setupAdminInput struct {
	Body struct {
		FirstName string `json:"first_name" minLength:"1"`
		LastName  string `json:"last_name"  minLength:"1"`
		Email     string `json:"email"      minLength:"3"`
		Password  string `json:"password"   minLength:"8"`
	}
}

type setupAdminOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *Handler) setupAdmin(ctx context.Context, input *setupAdminInput) (*setupAdminOutput, error) {
	var count int64
	if err := h.db.WithContext(ctx).Model(&models.User{}).Count(&count).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to check users")
	}
	if count > 0 {
		return nil, huma.NewError(http.StatusConflict, "setup already completed")
	}

	hash, err := auth.HashPassword(input.Body.Password)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user := &models.User{
		ID:        uuid.New().String(),
		Email:     input.Body.Email,
		Password:  hash,
		Name:      input.Body.FirstName + " " + input.Body.LastName,
		Role:      models.RoleAdmin,
		Active:    true,
		Confirmed: true,
	}

	if err := h.db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to create admin user")
	}

	out := &setupAdminOutput{}
	out.Body.Message = "admin user created successfully"
	return out, nil
}
