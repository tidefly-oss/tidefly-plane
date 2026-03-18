package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type Handler struct {
	svc *git.Service
	db  *gorm.DB
	log *logger.Logger
}

func New(svc *git.Service, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{svc: svc, db: db, log: log}
}

// ── Response types ────────────────────────────────────────────────────────────

type Response struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"`
	BaseURL    string   `json:"base_url,omitempty"`
	AuthType   string   `json:"auth_type"`
	IsOwner    bool     `json:"is_owner"`
	ProjectIDs []string `json:"project_ids"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func toResponse(m *models.GitIntegration, currentUserID string) Response {
	isOwner := m.UserID == currentUserID
	projectIDs := make([]string, 0, len(m.Shares))
	if isOwner {
		for _, s := range m.Shares {
			projectIDs = append(projectIDs, s.ProjectID)
		}
	}
	return Response{
		ID: m.ID, Name: m.Name, Provider: m.Provider, BaseURL: m.BaseURL,
		AuthType: m.AuthType, IsOwner: isOwner, ProjectIDs: projectIDs,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (h *Handler) visibleIDs(userID string, isAdmin bool) ([]string, error) {
	var ownedIDs []string
	if err := h.db.Model(&models.GitIntegration{}).
		Where("user_id = ?", userID).Pluck("id", &ownedIDs).Error; err != nil {
		return nil, err
	}
	var sharedIDs []string
	var err error
	if isAdmin {
		err = h.db.Raw(`SELECT DISTINCT integration_id::text FROM git_integration_shares`).Scan(&sharedIDs).Error
	} else {
		err = h.db.Raw(
			`
			SELECT git_integration_shares.integration_id::text
			FROM git_integration_shares
			JOIN project_members ON project_members.project_id::text = git_integration_shares.project_id::text
			WHERE project_members.user_id::text = ?`, userID,
		).Scan(&sharedIDs).Error
	}
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(ownedIDs)+len(sharedIDs))
	result := make([]string, 0, len(ownedIDs)+len(sharedIDs))
	for _, id := range append(ownedIDs, sharedIDs...) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result, nil
}

func (h *Handler) requireOwner(id, userID string) (*models.GitIntegration, error) {
	var m models.GitIntegration
	if err := h.db.Preload("Shares").First(&m, "id = ?", id).Error; err != nil {
		return nil, huma.Error404NotFound("integration not found")
	}
	if m.UserID != userID {
		return nil, huma.Error403Forbidden("not your integration")
	}
	return &m, nil
}

func (h *Handler) loadVisible(id, userID string, isAdmin bool) (*models.GitIntegration, error) {
	ids, err := h.visibleIDs(userID, isAdmin)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	for _, vid := range ids {
		if vid == id {
			var m models.GitIntegration
			if err := h.db.Preload("Shares").First(&m, "id = ?", id).Error; err != nil {
				return nil, huma.Error404NotFound("integration not found")
			}
			return &m, nil
		}
	}
	return nil, huma.Error404NotFound("integration not found")
}

func userFromCtx(ctx context.Context) *models.User {
	u := middleware.UserFromHumaCtx(ctx)
	if u == nil {
		return nil
	}
	user, _ := u.(*models.User)
	return user
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListInput struct{}
type ListOutput struct {
	Body []Response
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	ids, err := h.visibleIDs(user.ID, user.IsAdmin())
	if err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	if len(ids) == 0 {
		return &ListOutput{Body: []Response{}}, nil
	}
	var integrations []models.GitIntegration
	if err := h.db.Preload("Shares").Where("id IN ?", ids).Find(&integrations).Error; err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	resp := make([]Response, len(integrations))
	for i := range integrations {
		resp[i] = toResponse(&integrations[i], user.ID)
	}
	return &ListOutput{Body: resp}, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type CreateInput struct {
	Body struct {
		Name     string `json:"name" minLength:"1" maxLength:"255" doc:"Integration name"`
		Provider string `json:"provider" enum:"github,gitlab,gitea-forgejo,bitbucket" doc:"Git provider"`
		BaseURL  string `json:"base_url,omitempty" doc:"Base URL (required for Gitea/Forgejo)"`
		Token    string `json:"token" minLength:"1" doc:"Personal access token"`
		Username string `json:"username,omitempty" maxLength:"255" doc:"Username (required for Bitbucket)"`
	}
}
type CreateOutput struct {
	Body Response
}

func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if input.Body.Provider == string(types.ProviderGiteaForgejo) && input.Body.BaseURL == "" {
		return nil, huma.Error400BadRequest("base_url is required for Gitea/Forgejo")
	}
	if input.Body.Provider == string(types.ProviderBitbucket) && input.Body.Username == "" {
		return nil, huma.Error400BadRequest("username is required for Bitbucket")
	}
	baseURL := input.Body.BaseURL
	if input.Body.Provider == string(types.ProviderBitbucket) {
		baseURL = input.Body.Username
	}
	encrypted, err := h.svc.PrepareSecret(input.Body.Token)
	if err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditGitTokenAdd, Success: false,
				Details: fmt.Sprintf("provider=%s name=%s encrypt_failed", input.Body.Provider, input.Body.Name),
			},
		)
		return nil, fmt.Errorf("secure token: %w", err)
	}
	m := &models.GitIntegration{
		UserID: user.ID, Name: input.Body.Name, Provider: input.Body.Provider,
		BaseURL: baseURL, AuthType: "token", SecretEncrypted: encrypted,
	}
	if err := h.db.Create(m).Error; err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditGitTokenAdd, Success: false,
				Details: fmt.Sprintf("provider=%s name=%s", input.Body.Provider, input.Body.Name),
			},
		)
		return nil, fmt.Errorf("save integration: %w", err)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditGitTokenAdd, ResourceID: m.ID, Success: true,
			Details: fmt.Sprintf("provider=%s name=%s", m.Provider, m.Name),
		},
	)
	return &CreateOutput{Body: toResponse(m, user.ID)}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type GetInput struct {
	ID string `path:"id" doc:"Integration ID"`
}
type GetOutput struct {
	Body Response
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.loadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	return &GetOutput{Body: toResponse(m, user.ID)}, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteInput struct {
	ID string `path:"id" doc:"Integration ID"`
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.requireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	h.db.Where("integration_id = ?", input.ID).Delete(&models.GitIntegrationShare{})
	result := h.db.Delete(&models.GitIntegration{}, "id = ?", input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditGitTokenDelete, ResourceID: input.ID, Success: result.Error == nil,
			Details: fmt.Sprintf("provider=%s name=%s", m.Provider, m.Name),
		},
	)
	if result.Error != nil {
		return nil, fmt.Errorf("delete integration: %w", result.Error)
	}
	return nil, nil
}

// ── SetShares ─────────────────────────────────────────────────────────────────

type SetSharesInput struct {
	ID   string `path:"id" doc:"Integration ID"`
	Body struct {
		ProjectIDs []string `json:"project_ids" doc:"Project IDs to share with"`
	}
}
type SetSharesOutput struct {
	Body Response
}

func (h *Handler) SetShares(ctx context.Context, input *SetSharesInput) (*SetSharesOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.requireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	txErr := h.db.Transaction(
		func(tx *gorm.DB) error {
			if err := tx.Where("integration_id = ?", m.ID).Delete(&models.GitIntegrationShare{}).Error; err != nil {
				return err
			}
			for _, pid := range input.Body.ProjectIDs {
				if err := tx.Create(
					&models.GitIntegrationShare{
						IntegrationID: m.ID, ProjectID: pid,
					},
				).Error; err != nil {
					return err
				}
			}
			return nil
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditGitRepoLink, ResourceID: m.ID, Success: txErr == nil,
			Details: fmt.Sprintf(
				"integration=%s projects=%d [%s]", m.Name, len(input.Body.ProjectIDs),
				strings.Join(input.Body.ProjectIDs, ","),
			),
		},
	)
	if txErr != nil {
		return nil, fmt.Errorf("update shares: %w", txErr)
	}
	h.db.Preload("Shares").First(m, "id = ?", m.ID)
	return &SetSharesOutput{Body: toResponse(m, user.ID)}, nil
}

// ── Validate ──────────────────────────────────────────────────────────────────

type ValidateInput struct {
	ID string `path:"id" doc:"Integration ID"`
}
type ValidateOutput struct {
	Body struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}
}

func (h *Handler) Validate(ctx context.Context, input *ValidateInput) (*ValidateOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.requireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	out := &ValidateOutput{}
	verr := h.svc.ValidateIntegration(ctx, types.ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL)
	if verr != nil {
		out.Body.Valid = false
		out.Body.Error = verr.Error()
	} else {
		out.Body.Valid = true
	}
	return out, nil
}

// ── ListRepositories ──────────────────────────────────────────────────────────

type ListRepositoriesInput struct {
	ID string `path:"id" doc:"Integration ID"`
}
type ListRepositoriesOutput struct {
	Body []types.Repository
}

func (h *Handler) ListRepositories(ctx context.Context, input *ListRepositoriesInput) (*ListRepositoriesOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.loadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	repos, err := h.svc.ListRepositories(ctx, types.ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch repositories: %w", err)
	}
	return &ListRepositoriesOutput{Body: repos}, nil
}

// ── ListBranches ──────────────────────────────────────────────────────────────

type ListBranchesInput struct {
	ID    string `path:"id" doc:"Integration ID"`
	Owner string `path:"owner" doc:"Repository owner"`
	Repo  string `path:"repo" doc:"Repository name"`
}
type ListBranchesOutput struct {
	Body []types.Branch
}

func (h *Handler) ListBranches(ctx context.Context, input *ListBranchesInput) (*ListBranchesOutput, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	m, err := h.loadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	branches, err := h.svc.ListBranches(
		ctx, types.ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL, input.Owner, input.Repo,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch branches: %w", err)
	}
	return &ListBranchesOutput{Body: branches}, nil
}
