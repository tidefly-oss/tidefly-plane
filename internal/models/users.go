package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRole string

const (
	RoleAdmin  UserRole = "admin"
	RoleMember UserRole = "member"
)

type User struct {
	ID                  string         `gorm:"primaryKey;size:36"            json:"id"`
	Email               string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Password            string         `gorm:"size:255"                      json:"-"`
	Name                string         `gorm:"size:255"                      json:"name"`
	Role                UserRole       `gorm:"size:50;default:'member'"      json:"role"`
	Active              bool           `gorm:"default:true"                  json:"active"`
	ForcePasswordChange bool           `gorm:"default:false"                 json:"force_password_change"`
	CreatedAt           time.Time      `                                     json:"created_at"`
	UpdatedAt           time.Time      `                                     json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index"                         json:"-"`

	ProjectMembers []ProjectMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"project_members,omitempty"`
}

func (u *User) BeforeCreate(*gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// ProjectMember links a user to a project with a role.
type ProjectMember struct {
	ID        string    `gorm:"primaryKey;size:36"                                  json:"id"`
	UserID    string    `gorm:"size:36;not null;index;uniqueIndex:idx_user_project" json:"user_id"`
	ProjectID string    `gorm:"size:36;not null;index;uniqueIndex:idx_user_project" json:"project_id"`
	Role      UserRole  `gorm:"size:50;default:'member'"                            json:"role"`
	CreatedAt time.Time `                                                           json:"created_at"`
}

func (pm *ProjectMember) BeforeCreate(*gorm.DB) error {
	if pm.ID == "" {
		pm.ID = uuid.New().String()
	}
	return nil
}

// Project is the organizational unit that groups manifest.
type Project struct {
	ID          string    `gorm:"primaryKey;size:36"           json:"id"`
	Name        string    `gorm:"size:255;not null;uniqueIndex" json:"name"`
	Description string    `gorm:"size:1000"                    json:"description"`
	Color       string    `gorm:"size:7;default:'#6366f1'"     json:"color"`
	NetworkName string    `gorm:"size:255;uniqueIndex"         json:"network_name"`
	CreatedAt   time.Time `                                    json:"created_at"`
	UpdatedAt   time.Time `                                    json:"updated_at"`

	Members []ProjectMember `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
}

func (p *Project) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.NetworkName == "" {
		p.NetworkName = "tidefly_" + p.Name
	}
	return nil
}

// SystemSettings is the singleton global config row (id=1).
type SystemSettings struct {
	ID                           uint      `gorm:"primaryKey;autoIncrement"`
	InstanceName                 string    `gorm:"size:255;default:'Tidefly'" json:"instance_name"`
	InstanceURL                  string    `gorm:"size:500"                   json:"instance_url"`
	RegistrationMode             string    `gorm:"size:50;default:'open'"     json:"registration_mode"`
	SMTPHost                     string    `gorm:"size:255"                   json:"smtp_host"`
	SMTPPort                     int       `gorm:"default:587"                json:"smtp_port"`
	SMTPUsername                 string    `gorm:"size:255"                   json:"smtp_username"`
	SMTPPassword                 string    `gorm:"size:255"                   json:"-"`
	SMTPFrom                     string    `gorm:"size:255"                   json:"smtp_from"`
	SMTPTLSEnabled               bool      `gorm:"default:true"               json:"smtp_tls_enabled"`
	ExternalNotificationsEnabled bool      `gorm:"default:false"              json:"external_notifications_enabled"`
	SlackWebhookURL              string    `gorm:"size:500"                   json:"slack_webhook_url"`
	DiscordWebhookURL            string    `gorm:"size:500"                   json:"discord_webhook_url"`
	NotifyOnDeploy               bool      `gorm:"default:true"               json:"notify_on_deploy"`
	NotifyOnContainerDown        bool      `gorm:"default:true"               json:"notify_on_container_down"`
	NotifyOnWebhookFail          bool      `gorm:"default:true"               json:"notify_on_webhook_fail"`
	CaddyBaseDomain              string    `gorm:"size:255"                   json:"caddy_base_domain"`
	APIDocsEnabled               bool      `gorm:"default:true"               json:"api_docs_enabled"`
	UpdatedAt                    time.Time `                                  json:"updated_at"`
}
