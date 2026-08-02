package auth

import (
	"fmt"

	"github.com/google/uuid"
	"notes-app/internal/config"
	"notes-app/internal/models"
)

// OAuthProfile represents user profile data from OAuth providers
type OAuthProfile struct {
	ID       string
	Email    string
	Name     string
	AvatarURL string
}

// OAuthClient interface for OAuth operations
type OAuthClient interface {
	GetAuthURL(state string) string
	ExchangeCode(code string) (*OAuthProfile, error)
}

// AuthService handles authentication business logic
type AuthService struct {
	userRepo     UserRepository
	config       *config.Config
	oauthClients map[models.Provider]OAuthClient
}

// NewAuthService creates a new authentication service
func NewAuthService(userRepo UserRepository, cfg *config.Config) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		config:       cfg,
		oauthClients: make(map[models.Provider]OAuthClient),
	}
}

// RegisterOAuthClient registers an OAuth client for a provider
func (s *AuthService) RegisterOAuthClient(provider models.Provider, client OAuthClient) {
	s.oauthClients[provider] = client
}

// InitiateOAuth initiates the OAuth flow for a provider
func (s *AuthService) InitiateOAuth(provider models.Provider, state string) (string, error) {
	client, ok := s.oauthClients[provider]
	if !ok {
		return "", fmt.Errorf("OAuth client not registered for provider: %s", provider)
	}
	return client.GetAuthURL(state), nil
}

// HandleOAuthCallback handles the OAuth callback from a provider
func (s *AuthService) HandleOAuthCallback(provider models.Provider, code string) (*models.User, error) {
	client, ok := s.oauthClients[provider]
	if !ok {
		return nil, fmt.Errorf("OAuth client not registered for provider: %s", provider)
	}

	// Exchange code for user profile
	profile, err := client.ExchangeCode(code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Check if user already exists
	user, err := s.userRepo.GetUserByOAuthID(provider, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing user: %w", err)
	}

	// If user doesn't exist, create them
	if user == nil {
		user = &models.User{
			ID:        generateUserID(),
			Email:     profile.Email,
			Name:      profile.Name,
			AvatarURL: profile.AvatarURL,
			Provider:  provider,
			OAuthID:   profile.ID,
		}

		if err := s.userRepo.CreateUser(user); err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	} else {
		// Update user profile
		user.Email = profile.Email
		user.Name = profile.Name
		user.AvatarURL = profile.AvatarURL

		if err := s.userRepo.UpdateUser(user); err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
	}

	return user, nil
}

// ValidateUser validates a user by ID and returns the user
func (s *AuthService) ValidateUser(userID string) (*models.User, error) {
	if userID == "" {
		return nil, nil
	}
	
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, nil // User not found
	}
	return user, nil
}

// generateUserID generates a unique user ID using UUID
func generateUserID() string {
	return uuid.New().String()
}
