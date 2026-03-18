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

// User is the central user model shared across the entire application.
// Authboss interface methods are defined here (same package requirement).
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

	// Authboss Recovery
	RecoverSelector    string    `gorm:"size:255;index" json:"-"`
	RecoverVerifier    string    `gorm:"size:255"       json:"-"`
	RecoverTokenExpiry time.Time `                      json:"-"`

	// Authboss Confirm
	ConfirmSelector string `gorm:"size:255;index" json:"-"`
	ConfirmVerifier string `gorm:"size:255"       json:"-"`
	Confirmed       bool   `gorm:"default:false"  json:"-"`

	// Authboss Lock
	AttemptCount int       `json:"-"`
	LastAttempt  time.Time `json:"-"`
	Locked       time.Time `json:"-"`

	// Relations
	ProjectMembers []ProjectMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"project_members,omitempty"`
}

func (u *User) BeforeCreate(*gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// ── Authboss interface methods (must live in same package as User struct) ─────

func (u *User) GetPID() string               { return u.Email }
func (u *User) PutPID(pid string)            { u.Email = pid }
func (u *User) GetPassword() string          { return u.Password }
func (u *User) PutPassword(pwd string)       { u.Password = pwd }
func (u *User) GetEmail() string             { return u.Email }
func (u *User) PutEmail(email string)        { u.Email = email }
func (u *User) GetConfirmed() bool           { return true } // self-hosted: always confirmed
func (u *User) PutConfirmed(c bool)          { u.Confirmed = c }
func (u *User) GetConfirmSelector() string   { return u.ConfirmSelector }
func (u *User) PutConfirmSelector(s string)  { u.ConfirmSelector = s }
func (u *User) GetConfirmVerifier() string   { return u.ConfirmVerifier }
func (u *User) PutConfirmVerifier(v string)  { u.ConfirmVerifier = v }
func (u *User) GetRecoverSelector() string   { return u.RecoverSelector }
func (u *User) PutRecoverSelector(s string)  { u.RecoverSelector = s }
func (u *User) GetRecoverVerifier() string   { return u.RecoverVerifier }
func (u *User) PutRecoverVerifier(v string)  { u.RecoverVerifier = v }
func (u *User) GetRecoverExpiry() time.Time  { return u.RecoverTokenExpiry }
func (u *User) PutRecoverExpiry(t time.Time) { u.RecoverTokenExpiry = t }
func (u *User) GetAttemptCount() int         { return u.AttemptCount }
func (u *User) PutAttemptCount(n int)        { u.AttemptCount = n }
func (u *User) GetLastAttempt() time.Time    { return u.LastAttempt }
func (u *User) PutLastAttempt(t time.Time)   { u.LastAttempt = t }
func (u *User) GetLocked() time.Time         { return u.Locked }
func (u *User) PutLocked(t time.Time)        { u.Locked = t }

// ── Token (authboss remember-me) ──────────────────────────────────────────────

type Token struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	UserID    string `gorm:"size:36;index;not null"`
	Token     string `gorm:"size:255;uniqueIndex;not null"`
	CreatedAt time.Time
}

// ── ProjectMember ─────────────────────────────────────────────────────────────

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

// ── SystemSettings ────────────────────────────────────────────────────────────

type SystemSettings struct {
	ID                    uint      `gorm:"primaryKey;autoIncrement"`
	InstanceName          string    `gorm:"size:255;default:'Tidefly'" json:"instance_name"`
	InstanceURL           string    `gorm:"size:500"                   json:"instance_url"`
	RegistrationMode      string    `gorm:"size:50;default:'invite'"   json:"registration_mode"`
	SMTPHost              string    `gorm:"size:255"                   json:"smtp_host"`
	SMTPPort              int       `gorm:"default:587"                json:"smtp_port"`
	SMTPUsername          string    `gorm:"size:255"                   json:"smtp_username"`
	SMTPPassword          string    `gorm:"size:255"                   json:"-"`
	SMTPFrom              string    `gorm:"size:255"                   json:"smtp_from"`
	SMTPTLSEnabled        bool      `gorm:"default:true"               json:"smtp_tls_enabled"`
	SessionTimeoutHours   int       `gorm:"default:24"                 json:"session_timeout_hours"`
	NotificationsEnabled  bool      `gorm:"default:false"              json:"notifications_enabled"`
	SlackWebhookURL       string    `gorm:"size:500"                   json:"slack_webhook_url"`
	DiscordWebhookURL     string    `gorm:"size:500"                   json:"discord_webhook_url"`
	NotifyOnDeploy        bool      `gorm:"default:true"               json:"notify_on_deploy"`
	NotifyOnContainerDown bool      `gorm:"default:true"               json:"notify_on_container_down"`
	NotifyOnWebhookFail   bool      `gorm:"default:true"               json:"notify_on_webhook_fail"`
	UpdatedAt             time.Time `                                  json:"updated_at"`
}
