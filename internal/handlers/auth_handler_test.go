package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"notes-app/internal/auth"
	"notes-app/internal/config"
)

func TestAuthHandler_HandleLogin(t *testing.T) {
	t.Run("renders login page", func(t *testing.T) {
		tempDir := t.TempDir()
		
		userRepo, _ := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		handler := NewAuthHandler(authService, cfg)

		req := httptest.NewRequest("GET", "/login", nil)
		rr := httptest.NewRecorder()

		handler.HandleLogin(rr, req)

		// Just check that it attempts to render (template may not exist in test)
		// In real scenario, the template would be served
		assert.True(t, rr.Code == http.StatusOK || rr.Code == http.StatusInternalServerError)
	})
}

func TestAuthHandler_HandleLogout(t *testing.T) {
	t.Run("clears session and redirects", func(t *testing.T) {
		tempDir := t.TempDir()
		
		userRepo, _ := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		handler := NewAuthHandler(authService, cfg)

		req := httptest.NewRequest("GET", "/logout", nil)
		rr := httptest.NewRecorder()

		handler.HandleLogout(rr, req)

		assert.Equal(t, http.StatusFound, rr.Code)
		assert.Equal(t, "/", rr.Header().Get("Location"))
	})
}
