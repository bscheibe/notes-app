package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Run("loads with defaults", func(t *testing.T) {
		cfg, err := Load("")
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "8080", cfg.Server.Port)
		assert.Equal(t, "localhost", cfg.Server.Host)
		assert.NotEmpty(t, cfg.Auth.Session.CookieSecret)
		assert.NotEmpty(t, cfg.Notes.Directory)
	})

	t.Run("PORT env var overrides server port", func(t *testing.T) {
		t.Setenv("PORT", "9090")

		cfg, err := Load("")
		require.NoError(t, err)
		assert.Equal(t, "9090", cfg.Server.Port)
	})

	t.Run("generates random cookie secret", func(t *testing.T) {
		cfg1, err := Load("")
		require.NoError(t, err)

		cfg2, err := Load("")
		require.NoError(t, err)

		// Cookie secrets should be different (randomly generated)
		assert.NotEqual(t, cfg1.Auth.Session.CookieSecret, cfg2.Auth.Session.CookieSecret)
	})
}

func TestGetSessionDuration(t *testing.T) {
	t.Run("parses valid duration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Auth.Session.Duration = "24h"

		duration := cfg.GetSessionDuration()
		assert.Equal(t, 24*time.Hour, duration)
	})

	t.Run("returns default for invalid duration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Auth.Session.Duration = "invalid"

		duration := cfg.GetSessionDuration()
		assert.Equal(t, 24*time.Hour, duration)
	})
}

func TestGetGuestSessionDuration(t *testing.T) {
	t.Run("parses valid duration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Auth.GuestSessionDuration = "2h"

		duration := cfg.GetGuestSessionDuration()
		assert.Equal(t, 2*time.Hour, duration)
	})

	t.Run("returns default for invalid duration", func(t *testing.T) {
		cfg := &Config{}
		cfg.Auth.GuestSessionDuration = "invalid"

		duration := cfg.GetGuestSessionDuration()
		assert.Equal(t, 1*time.Hour, duration)
	})
}
