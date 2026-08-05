package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		Port string
		Host string
	}
	Notes struct {
		Directory string
	}
	Logging struct {
		Level  string
		Format string
	}
	Monitoring struct {
		Enabled        bool
		ServiceName    string
		TracingEnabled bool
	}
	Auth struct {
		Google struct {
			ClientID     string
			ClientSecret string
			CallbackURL  string
		}
		GitHub struct {
			ClientID     string
			ClientSecret string
			CallbackURL  string
		}
		Session struct {
			CookieName   string
			CookieSecret string
			Duration     string // Duration string (e.g., "24h")
		}
		GuestSessionDuration string // Duration string for guest sessions
	}
}

// Load loads the configuration from file and environment variables
// configFile is an optional path to a specific config file
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set config file settings
	v.SetConfigType("yaml")

	if configFile != "" {
		// Use specific config file if provided
		v.SetConfigFile(configFile)
	} else {
		// Otherwise search for config files in standard locations
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/notes-app/")
		v.AddConfigPath("$HOME/.notes-app")
	}

	// Set defaults
	v.SetDefault("server.port", "8080")
	v.SetDefault("server.host", "localhost")
	v.SetDefault("notes.directory", "")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("monitoring.enabled", true)
	v.SetDefault("monitoring.service_name", "notes-app")
	v.SetDefault("monitoring.tracing_enabled", true)

	// Auth defaults
	v.SetDefault("auth.google.client_id", "")
	v.SetDefault("auth.google.client_secret", "")
	v.SetDefault("auth.google.callback_url", "http://localhost:8080/auth/google/callback")
	v.SetDefault("auth.github.client_id", "")
	v.SetDefault("auth.github.client_secret", "")
	v.SetDefault("auth.github.callback_url", "http://localhost:8080/auth/github/callback")
	v.SetDefault("auth.session.cookie_name", "notes_session")
	v.SetDefault("auth.session.cookie_secret", "")
	v.SetDefault("auth.session.duration", "24h")
	v.SetDefault("auth.guest_session_duration", "1h")

	// Map environment variables to nested config keys
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Allow environment variable overrides
	v.SetEnvPrefix("NOTES_APP")
	v.AutomaticEnv()

	// Cloud Run (and App Engine, Heroku, etc.) injects the listen port via
	// the bare PORT env var rather than NOTES_APP_SERVER_PORT.
	if port := os.Getenv("PORT"); port != "" {
		v.Set("server.port", port)
	}

	// Read config file (if exists)
	if err := v.ReadInConfig(); err != nil {
		// If config file not found, we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			slog.Warn("Could not read config file, using defaults", "error", err)
		}
	}

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set default notes directory if not specified
	if cfg.Notes.Directory == "" {
		cfg.Notes.Directory = filepath.Join(os.TempDir(), "notes-app")
	}

	// Generate random cookie secret if not specified
	if cfg.Auth.Session.CookieSecret == "" {
		cfg.Auth.Session.CookieSecret = generateRandomSecret()
		slog.Warn("Generated random cookie secret - set NOTES_APP_AUTH_SESSION_COOKIE_SECRET for production")
	}

	return &cfg, nil
}

// generateRandomSecret generates a random 32-byte secret for session cookies
func generateRandomSecret() string {
	b := make([]byte, 32)
	// Use crypto/rand for secure random generation
	if _, err := rand.Read(b); err != nil {
		// If crypto/rand fails, panic - this is a critical security failure
		panic("failed to generate secure random secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// GetSessionDuration returns the session duration as a time.Duration
func (c *Config) GetSessionDuration() time.Duration {
	duration, err := time.ParseDuration(c.Auth.Session.Duration)
	if err != nil {
		return 24 * time.Hour // Default to 24 hours
	}
	return duration
}

// GetGuestSessionDuration returns the guest session duration as a time.Duration
func (c *Config) GetGuestSessionDuration() time.Duration {
	duration, err := time.ParseDuration(c.Auth.GuestSessionDuration)
	if err != nil {
		return 1 * time.Hour // Default to 1 hour
	}
	return duration
}
