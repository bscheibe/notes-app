package auth

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"notes-app/internal/models"
)

func TestFileSystemUserRepository_CreateUser(t *testing.T) {
	t.Run("creates user successfully", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:        "user123",
			Email:     "test@example.com",
			Name:      "Test User",
			AvatarURL: "https://example.com/avatar.jpg",
			Provider:  models.ProviderGoogle,
			OAuthID:   "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		// Verify user was created
		retrieved, err := repo.GetUserByID("user123")
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", retrieved.Email)
		assert.Equal(t, "Test User", retrieved.Name)
		assert.False(t, retrieved.CreatedAt.IsZero())
		assert.False(t, retrieved.UpdatedAt.IsZero())
	})

	t.Run("prevents duplicate email", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user1 := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		user2 := &models.User{
			ID:       "user456",
			Email:    "test@example.com", // Same email
			Name:     "Another User",
			Provider: models.ProviderGitHub,
			OAuthID:  "github456",
		}

		err = repo.CreateUser(user1)
		require.NoError(t, err)

		err = repo.CreateUser(user2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("prevents duplicate OAuth ID", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user1 := &models.User{
			ID:       "user123",
			Email:    "test1@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		user2 := &models.User{
			ID:       "user456",
			Email:    "test2@example.com",
			Name:     "Another User",
			Provider: models.ProviderGoogle, // Same provider
			OAuthID:  "google123",          // Same OAuth ID
		}

		err = repo.CreateUser(user1)
		require.NoError(t, err)

		err = repo.CreateUser(user2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestFileSystemUserRepository_GetUserByID(t *testing.T) {
	t.Run("retrieves existing user", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		retrieved, err := repo.GetUserByID("user123")
		require.NoError(t, err)
		assert.Equal(t, "user123", retrieved.ID)
		assert.Equal(t, "test@example.com", retrieved.Email)
	})

	t.Run("returns nil for non-existent user", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user, err := repo.GetUserByID("nonexistent")
		require.NoError(t, err)
		assert.Nil(t, user)
	})
}

func TestFileSystemUserRepository_GetUserByEmail(t *testing.T) {
	t.Run("retrieves user by email", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		retrieved, err := repo.GetUserByEmail("test@example.com")
		require.NoError(t, err)
		assert.Equal(t, "user123", retrieved.ID)
		assert.Equal(t, "test@example.com", retrieved.Email)
	})

	t.Run("returns nil for non-existent email", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user, err := repo.GetUserByEmail("nonexistent@example.com")
		require.NoError(t, err)
		assert.Nil(t, user)
	})
}

func TestFileSystemUserRepository_GetUserByOAuthID(t *testing.T) {
	t.Run("retrieves user by OAuth ID", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		retrieved, err := repo.GetUserByOAuthID(models.ProviderGoogle, "google123")
		require.NoError(t, err)
		assert.Equal(t, "user123", retrieved.ID)
		assert.Equal(t, models.ProviderGoogle, retrieved.Provider)
		assert.Equal(t, "google123", retrieved.OAuthID)
	})

	t.Run("returns nil for non-existent OAuth ID", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user, err := repo.GetUserByOAuthID(models.ProviderGoogle, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, user)
	})

	t.Run("distinguishes between providers", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user1 := &models.User{
			ID:       "user123",
			Email:    "test1@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "123",
		}

		user2 := &models.User{
			ID:       "user456",
			Email:    "test2@example.com",
			Name:     "Another User",
			Provider: models.ProviderGitHub,
			OAuthID:  "123", // Same OAuth ID but different provider
		}

		err = repo.CreateUser(user1)
		require.NoError(t, err)

		err = repo.CreateUser(user2)
		require.NoError(t, err)

		// Should retrieve the correct user for each provider
		googleUser, err := repo.GetUserByOAuthID(models.ProviderGoogle, "123")
		require.NoError(t, err)
		assert.Equal(t, "user123", googleUser.ID)

		githubUser, err := repo.GetUserByOAuthID(models.ProviderGitHub, "123")
		require.NoError(t, err)
		assert.Equal(t, "user456", githubUser.ID)
	})
}

func TestFileSystemUserRepository_UpdateUser(t *testing.T) {
	t.Run("updates existing user", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		// Update user
		user.Name = "Updated Name"
		user.AvatarURL = "https://example.com/new-avatar.jpg"

		err = repo.UpdateUser(user)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetUserByID("user123")
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)
		assert.Equal(t, "https://example.com/new-avatar.jpg", retrieved.AvatarURL)
		assert.True(t, retrieved.UpdatedAt.After(retrieved.CreatedAt))
	})

	t.Run("fails to update non-existent user", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "nonexistent",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.UpdateUser(user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestFileSystemUserRepository_DeleteUser(t *testing.T) {
	t.Run("deletes existing user", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)

		err = repo.DeleteUser("user123")
		require.NoError(t, err)

		// Verify deletion
		retrieved, err := repo.GetUserByID("user123")
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("deleting non-existent user is no-op", func(t *testing.T) {
		tempDir := t.TempDir()
		repo, err := NewFileSystemUserRepository(tempDir)
		require.NoError(t, err)

		err = repo.DeleteUser("nonexistent")
		assert.NoError(t, err)
	})
}

func TestFileSystemUserRepository_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := NewFileSystemUserRepository(tempDir)
	require.NoError(t, err)

	// Create multiple users concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			user := &models.User{
				ID:       fmt.Sprintf("user%d", i),
				Email:    fmt.Sprintf("test%d@example.com", i),
				Name:     "Test User",
				Provider: models.ProviderGoogle,
				OAuthID:  fmt.Sprintf("google%d", i),
			}
			_ = repo.CreateUser(user)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFileSystemUserRepository_DirectoryCreation(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := tempDir + "/users/subdir"

		repo, err := NewFileSystemUserRepository(newDir)
		require.NoError(t, err)

		// Verify directory was created
		info, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Verify we can create a user
		user := &models.User{
			ID:       "user123",
			Email:    "test@example.com",
			Name:     "Test User",
			Provider: models.ProviderGoogle,
			OAuthID:  "google123",
		}

		err = repo.CreateUser(user)
		require.NoError(t, err)
	})
}
