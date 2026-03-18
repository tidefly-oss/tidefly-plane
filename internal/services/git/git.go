package git

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/services/git/providers"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type Service struct {
	appKey string
}

func NewService(appKey string) *Service {
	return &Service{appKey: appKey}
}

func (s *Service) newProvider(providerType types.ProviderType, token, baseURL string) (types.Provider, error) {
	switch providerType {
	case types.ProviderGitHub:
		return providers.NewGitHub(token), nil
	case types.ProviderGitLab:
		return providers.NewGitLab(token, baseURL), nil
	case types.ProviderGiteaForgejo:
		if baseURL == "" {
			return nil, fmt.Errorf("git: base_url is required for Gitea/Forgejo")
		}
		return providers.NewGitea(token, baseURL), nil
	case types.ProviderBitbucket:
		return providers.NewBitbucket(baseURL, token), nil
	default:
		return nil, fmt.Errorf("git: unsupported provider type: %s", providerType)
	}
}

func (s *Service) PrepareSecret(plaintext string) (string, error) {
	encrypted, err := EncryptSecret(plaintext, s.appKey)
	if err != nil {
		return "", fmt.Errorf("git: preparing secret: %w", err)
	}
	return encrypted, nil
}

func (s *Service) ResolveSecret(encrypted string) (string, error) {
	plaintext, err := DecryptSecret(encrypted, s.appKey)
	if err != nil {
		return "", fmt.Errorf("git: resolving secret: %w", err)
	}
	return plaintext, nil
}

func (s *Service) ValidateIntegration(
	ctx context.Context,
	providerType types.ProviderType,
	encryptedToken, baseURL string,
) error {
	token, err := s.ResolveSecret(encryptedToken)
	if err != nil {
		return err
	}
	provider, err := s.newProvider(providerType, token, baseURL)
	if err != nil {
		return err
	}
	return provider.ValidateCredentials(ctx)
}

func (s *Service) ListRepositories(
	ctx context.Context,
	providerType types.ProviderType,
	encryptedToken, baseURL string,
) ([]types.Repository, error) {
	token, err := s.ResolveSecret(encryptedToken)
	if err != nil {
		return nil, err
	}
	provider, err := s.newProvider(providerType, token, baseURL)
	if err != nil {
		return nil, err
	}
	return provider.ListRepositories(ctx)
}

func (s *Service) ListBranches(
	ctx context.Context,
	providerType types.ProviderType,
	encryptedToken, baseURL, owner, repo string,
) ([]types.Branch, error) {
	token, err := s.ResolveSecret(encryptedToken)
	if err != nil {
		return nil, err
	}
	provider, err := s.newProvider(providerType, token, baseURL)
	if err != nil {
		return nil, err
	}
	return provider.ListBranches(ctx, owner, repo)
}
