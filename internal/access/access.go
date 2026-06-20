// Package access provides shared access control helpers for container, network, and volume filtering.
// It has zero dependency on middleware — callers pass in userID/role directly.
package access

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

// ── Labels ────────────────────────────────────────────────────────────────────

const (
	LabelManaged   = "tidefly-plane.managed"
	LabelInternal  = "tidefly.internal"
	LabelService   = "tidefly.service"
	LabelServiceID = "tidefly.service-id"
	LabelProject   = "tidefly.project"
	LabelSlot      = "tidefly.slot"
	LabelProjectID = "tidefly-plane.project_id"
)

// NetworkProxy is the Caddy proxy network — always hidden from users.
const NetworkProxy = "tidefly_proxy"

// IsInternal returns true if the resource should be hidden from users.
func IsInternal(labels map[string]string) bool {
	return labels[LabelInternal] == "true"
}

// IsManaged returns true if the resource was created by Tidefly.
func IsManaged(labels map[string]string) bool {
	return labels[LabelManaged] == "true"
}

// ── Store ─────────────────────────────────────────────────────────────────────

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *gorm.DB { return s.db }

// AllowedNetworks returns the Docker network names visible to a user
// based on their project memberships.
func (s *Store) AllowedNetworks(userID string) (map[string]struct{}, error) {
	var members []models.ProjectMember
	if err := s.db.Where("user_id = ?", userID).Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return map[string]struct{}{}, nil
	}
	projectIDs := make([]string, len(members))
	for i, m := range members {
		projectIDs[i] = m.ProjectID
	}
	var projects []models.Project
	if err := s.db.Select("network_name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
		return nil, err
	}
	nets := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		if p.NetworkName != "" {
			nets[p.NetworkName] = struct{}{}
		}
	}
	return nets, nil
}

// NetworkAllowed returns true if any of the given networks are in the allowed set.
func NetworkAllowed(networks []string, allowed map[string]struct{}) bool {
	for _, n := range networks {
		if _, ok := allowed[n]; ok {
			return true
		}
	}
	return false
}

// ── Container access ──────────────────────────────────────────────────────────

// CheckProjectMembership checks if userID is a member of the container's project.
// Callers must check admin role themselves before calling this.
func CheckProjectMembership(db *gorm.DB, userID string, labels map[string]string) error {
	projectID := labels[LabelProjectID]
	if projectID == "" {
		return fmt.Errorf("access denied: container is not part of any project")
	}
	var count int64
	if err := db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("access check failed")
	}
	if count == 0 {
		return fmt.Errorf("access denied: not a member of this container's project")
	}
	return nil
}

// ── Context user interface ────────────────────────────────────────────────────
// UserReader abstracts middleware.UserFromHumaCtx to avoid import cycle.
// Bootstrap wires this once via SetUserReader.

type UserInfo struct {
	UserID string
	Email  string
	Role   string
}

type userReaderFn func(context.Context) *UserInfo

var globalUserReader userReaderFn

// SetUserReader wires the middleware context accessor into the access package.
// Call once at app startup in bootstrap.
func SetUserReader(fn func(context.Context) *UserInfo) {
	globalUserReader = fn
}

// CurrentUser returns the authenticated user from context, or nil.
func CurrentUser(ctx context.Context) *UserInfo {
	if globalUserReader == nil {
		return nil
	}
	return globalUserReader(ctx)
}

// IsAdmin returns true if the context user has the admin role.
func IsAdmin(ctx context.Context) bool {
	u := CurrentUser(ctx)
	return u != nil && u.Role == string(models.RoleAdmin)
}

// UserID returns the authenticated user's ID, or empty string.
func UserID(ctx context.Context) string {
	u := CurrentUser(ctx)
	if u == nil {
		return ""
	}
	return u.UserID
}
