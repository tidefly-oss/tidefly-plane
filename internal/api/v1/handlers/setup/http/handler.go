package http

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

type SetupAdminInput struct {
	Body struct {
		FirstName string `json:"first_name" minLength:"1"`
		LastName  string `json:"last_name"  minLength:"1"`
		Email     string `json:"email"      minLength:"3"`
		Password  string `json:"password"   minLength:"8"`
	}
}

type SetupAdminOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *Handler) SetupAdmin(ctx context.Context, input *SetupAdminInput) (*SetupAdminOutput, error) {
	// Only callable if no users exist yet
	var count int64
	if err := h.db.WithContext(ctx).Model(&models.User{}).Count(&count).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to check users")
	}
	if count > 0 {
		return nil, huma.NewError(http.StatusConflict, "setup already completed")
	}

	hash, err := hashPassword(input.Body.Password)
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

	out := &SetupAdminOutput{}
	out.Body.Message = "admin user created successfully"
	return out, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, 64*1024, 3, 2,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}
