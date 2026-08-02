package models

import "time"

// Provider represents the OAuth provider
type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderGitHub Provider = "github"
	ProviderGuest  Provider = "guest"
)

// User represents an authenticated user from OAuth providers
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Provider  Provider  `json:"provider"`
	OAuthID   string    `json:"oauth_id"` // Provider's user ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GuestSession represents a temporary guest user session
type GuestSession struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Identity interface for polymorphic handling of different user types
type Identity interface {
	GetID() string
	IsAuthenticated() bool
	GetName() string
}

// GetID returns the user ID
func (u *User) GetID() string {
	return u.ID
}

// IsAuthenticated returns true for Users
func (u *User) IsAuthenticated() bool {
	return true
}

// GetName returns the user's name
func (u *User) GetName() string {
	return u.Name
}

// GetID returns the session ID
func (g *GuestSession) GetID() string {
	return g.SessionID
}

// IsAuthenticated returns false for Guests
func (g *GuestSession) IsAuthenticated() bool {
	return false
}

// GetName returns a generic name for guests
func (g *GuestSession) GetName() string {
	return "Guest"
}

// IsValid checks if the guest session is still valid
func (g *GuestSession) IsValid() bool {
	return time.Now().Before(g.ExpiresAt)
}
