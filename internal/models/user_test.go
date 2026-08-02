package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUser_Identity(t *testing.T) {
	user := &User{
		ID:        "user123",
		Email:     "test@example.com",
		Name:      "Test User",
		AvatarURL: "https://example.com/avatar.jpg",
		Provider:  ProviderGoogle,
		OAuthID:   "google123",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	assert.Equal(t, "user123", user.GetID())
	assert.True(t, user.IsAuthenticated())
	assert.Equal(t, "Test User", user.GetName())
}

func TestGuestSession_Identity(t *testing.T) {
	session := &GuestSession{
		SessionID: "guest123",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	assert.Equal(t, "guest123", session.GetID())
	assert.False(t, session.IsAuthenticated())
	assert.Equal(t, "Guest", session.GetName())
}

func TestGuestSession_IsValid(t *testing.T) {
	t.Run("valid session", func(t *testing.T) {
		session := &GuestSession{
			SessionID: "guest123",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		assert.True(t, session.IsValid())
	})

	t.Run("expired session", func(t *testing.T) {
		session := &GuestSession{
			SessionID: "guest123",
			CreatedAt: time.Now().Add(-48 * time.Hour),
			ExpiresAt: time.Now().Add(-24 * time.Hour),
		}
		assert.False(t, session.IsValid())
	})
}

func TestProvider_Constants(t *testing.T) {
	assert.Equal(t, Provider("google"), ProviderGoogle)
	assert.Equal(t, Provider("github"), ProviderGitHub)
}
