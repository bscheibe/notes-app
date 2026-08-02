package auth

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"notes-app/internal/config"
	"notes-app/internal/models"
)

// MockUserRepository is a mock implementation of UserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) CreateUser(user *models.User) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockUserRepository) GetUserByID(id string) (*models.User, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetUserByEmail(email string) (*models.User, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetUserByOAuthID(provider models.Provider, oauthID string) (*models.User, error) {
	args := m.Called(provider, oauthID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) UpdateUser(user *models.User) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockUserRepository) DeleteUser(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

// MockOAuthClient is a mock implementation of OAuthClient
type MockOAuthClient struct {
	mock.Mock
}

func (m *MockOAuthClient) GetAuthURL(state string) string {
	args := m.Called(state)
	return args.String(0)
}

func (m *MockOAuthClient) ExchangeCode(code string) (*OAuthProfile, error) {
	args := m.Called(code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OAuthProfile), args.Error(1)
}

func TestAuthService_InitiateOAuth(t *testing.T) {
	t.Run("returns auth URL for registered provider", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		mockOAuthClient := new(MockOAuthClient)
		mockOAuthClient.On("GetAuthURL", "test-state").Return("https://accounts.google.com/o/oauth2/auth?state=test-state")
		
		service.RegisterOAuthClient(models.ProviderGoogle, mockOAuthClient)
		
		url, err := service.InitiateOAuth(models.ProviderGoogle, "test-state")
		require.NoError(t, err)
		assert.Equal(t, "https://accounts.google.com/o/oauth2/auth?state=test-state", url)
		
		mockOAuthClient.AssertExpectations(t)
	})

	t.Run("returns error for unregistered provider", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		_, err := service.InitiateOAuth(models.ProviderGoogle, "test-state")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})
}

func TestAuthService_HandleOAuthCallback(t *testing.T) {
	t.Run("creates new user for first-time login", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		mockOAuthClient := new(MockOAuthClient)
		profile := &OAuthProfile{
			ID:        "google123",
			Email:     "test@example.com",
			Name:      "Test User",
			AvatarURL: "https://example.com/avatar.jpg",
		}
		mockOAuthClient.On("ExchangeCode", "auth-code").Return(profile, nil)
		
		mockUserRepo.On("GetUserByOAuthID", models.ProviderGoogle, "google123").Return(nil, nil)
		mockUserRepo.On("CreateUser", mock.MatchedBy(func(user *models.User) bool {
			// Verify the user ID is a valid UUID
			_, err := uuid.Parse(user.ID)
			return err == nil
		})).Return(nil)
		
		service.RegisterOAuthClient(models.ProviderGoogle, mockOAuthClient)
		
		user, err := service.HandleOAuthCallback(models.ProviderGoogle, "auth-code")
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test@example.com", user.Email)
		
		// Verify the user ID is a valid UUID
		_, err = uuid.Parse(user.ID)
		assert.NoError(t, err, "User ID should be a valid UUID")
		
		mockOAuthClient.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("updates existing user", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		mockOAuthClient := new(MockOAuthClient)
		profile := &OAuthProfile{
			ID:        "google123",
			Email:     "updated@example.com",
			Name:      "Updated Name",
			AvatarURL: "https://example.com/new-avatar.jpg",
		}
		mockOAuthClient.On("ExchangeCode", "auth-code").Return(profile, nil)
		
		existingUser := &models.User{
			ID:       "user123",
			Email:    "old@example.com",
			Name:     "Old Name",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}
		mockUserRepo.On("GetUserByOAuthID", models.ProviderGoogle, "google123").Return(existingUser, nil)
		mockUserRepo.On("UpdateUser", mock.AnythingOfType("*models.User")).Return(nil)
		
		service.RegisterOAuthClient(models.ProviderGoogle, mockOAuthClient)
		
		user, err := service.HandleOAuthCallback(models.ProviderGoogle, "auth-code")
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "updated@example.com", user.Email)
		
		mockOAuthClient.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("returns error for unregistered provider", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		_, err := service.HandleOAuthCallback(models.ProviderGoogle, "auth-code")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})
}

func TestAuthService_ValidateUser(t *testing.T) {
	t.Run("validates existing user", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}
		mockUserRepo.On("GetUserByID", "user123").Return(user, nil)
		
		validatedUser, err := service.ValidateUser("user123")
		require.NoError(t, err)
		assert.NotNil(t, validatedUser)
		assert.Equal(t, "user123", validatedUser.ID)
		
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("returns nil for non-existent user", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		mockUserRepo.On("GetUserByID", "nonexistent").Return(nil, nil)
		
		validatedUser, err := service.ValidateUser("nonexistent")
		require.NoError(t, err)
		assert.Nil(t, validatedUser)
		
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("returns nil for empty user ID", func(t *testing.T) {
		mockUserRepo := new(MockUserRepository)
		cfg := &config.Config{}
		
		service := NewAuthService(mockUserRepo, cfg)
		
		validatedUser, err := service.ValidateUser("")
		require.NoError(t, err)
		assert.Nil(t, validatedUser)
	})
}
