package types

import (
	"context"
	"time"
)

type ProviderType string

const (
	ProviderGitHub       ProviderType = "github"
	ProviderGitLab       ProviderType = "gitlab"
	ProviderGiteaForgejo ProviderType = "gitea"
	ProviderBitbucket    ProviderType = "bitbucket"
)

type AuthType string

const (
	AuthTypeToken  AuthType = "token"
	AuthTypeOAuth2 AuthType = "oauth2"
)

type Repository struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	Private       bool      `json:"private"`
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Branch struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
	Protected bool   `json:"protected"`
}

type Commit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
}

type ProviderInfo struct {
	Type        ProviderType `json:"type"`
	DisplayName string       `json:"display_name"`
	BaseURL     string       `json:"base_url,omitempty"`
}

type Provider interface {
	ValidateCredentials(ctx context.Context) error
	ListRepositories(ctx context.Context) ([]Repository, error)
	GetRepository(ctx context.Context, owner, name string) (*Repository, error)
	ListBranches(ctx context.Context, owner, name string) ([]Branch, error)
	GetInfo() ProviderInfo
}
