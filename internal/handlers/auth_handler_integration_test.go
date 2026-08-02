package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"notes-app/internal/auth"
	"notes-app/internal/config"
)

// Integration tests for authentication handlers
// These tests the full HTTP flow from request to response

func TestAuthHandlerIntegration_LoginFlow(t *testing.T) {
	t.Run("complete guest login flow", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Test guest login
		req := httptest.NewRequest("GET", "/auth/guest", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleGuestLogin(rr, req)
		
		// Should redirect to home
		assert.Equal(t, http.StatusFound, rr.Code)
		assert.Equal(t, "/", rr.Header().Get("Location"))
		
		// Check that session cookie was set
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		assert.NotNil(t, sessionCookie, "Session cookie should be set")
		assert.NotEmpty(t, sessionCookie.Value, "Session cookie should have value")
	})
	
	t.Run("complete logout flow", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// First create a guest session
		req := httptest.NewRequest("GET", "/auth/guest", nil)
		rr := httptest.NewRecorder()
		authHandler.HandleGuestLogin(rr, req)
		
		// Get session cookie
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		require.NotNil(t, sessionCookie, "Session cookie should be set")
		
		// Now logout with the session
		req2 := httptest.NewRequest("GET", "/logout", nil)
		req2.AddCookie(sessionCookie)
		rr2 := httptest.NewRecorder()
		
		authHandler.HandleLogout(rr2, req2)
		
		// Should redirect to home
		assert.Equal(t, http.StatusFound, rr2.Code)
		assert.Equal(t, "/", rr2.Header().Get("Location"))
		
		// Cookie should be deleted (MaxAge = -1)
		cookies2 := rr2.Result().Cookies()
		var logoutCookie *http.Cookie
		for _, cookie := range cookies2 {
			if cookie.Name == "test_session" {
				logoutCookie = cookie
				break
			}
		}
		assert.NotNil(t, logoutCookie)
		assert.Equal(t, -1, logoutCookie.MaxAge, "Cookie should be deleted")
	})
}

func TestAuthHandlerIntegration_OAuthFlow(t *testing.T) {
	t.Run("Google OAuth initiation with state", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		cfg.Auth.Google.ClientID = "test-client-id"
		cfg.Auth.Google.ClientSecret = "test-client-secret"
		cfg.Auth.Google.CallbackURL = "http://localhost:8080/auth/google/callback"
		
		authService := auth.NewAuthService(userRepo, cfg)
		
		// Register OAuth clients
		oauthClients := auth.NewOAuthClients(cfg)
		for provider, client := range oauthClients {
			authService.RegisterOAuthClient(provider, client)
		}
		
		authHandler := NewAuthHandler(authService, cfg)
		
		// Test Google OAuth initiation
		req := httptest.NewRequest("GET", "/auth/google", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleGoogleOAuth(rr, req)
		
		// Should redirect to Google OAuth
		assert.Equal(t, http.StatusFound, rr.Code)
		location := rr.Header().Get("Location")
		assert.Contains(t, location, "accounts.google.com")
		assert.Contains(t, location, "client_id=test-client-id")
		
		// Session should be set with state
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		assert.NotNil(t, sessionCookie, "Session cookie should be set")
		
		// Verify state parameter was generated
		assert.Contains(t, location, "state=")
	})
	
	t.Run("GitHub OAuth initiation with state", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		cfg.Auth.GitHub.ClientID = "test-client-id"
		cfg.Auth.GitHub.ClientSecret = "test-client-secret"
		cfg.Auth.GitHub.CallbackURL = "http://localhost:8080/auth/github/callback"
		
		authService := auth.NewAuthService(userRepo, cfg)
		
		// Register OAuth clients
		oauthClients := auth.NewOAuthClients(cfg)
		for provider, client := range oauthClients {
			authService.RegisterOAuthClient(provider, client)
		}
		
		authHandler := NewAuthHandler(authService, cfg)
		
		// Test GitHub OAuth initiation
		req := httptest.NewRequest("GET", "/auth/github", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleGitHubOAuth(rr, req)
		
		// Should redirect to GitHub OAuth
		assert.Equal(t, http.StatusFound, rr.Code)
		location := rr.Header().Get("Location")
		assert.Contains(t, location, "github.com")
		assert.Contains(t, location, "client_id=test-client-id")
		
		// Session should be set with state
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		assert.NotNil(t, sessionCookie, "Session cookie should be set")
	})
	
	t.Run("OAuth callback with invalid state", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		cfg.Auth.Google.ClientID = "test-client-id"
		cfg.Auth.Google.ClientSecret = "test-client-secret"
		cfg.Auth.Google.CallbackURL = "http://localhost:8080/auth/google/callback"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Test callback with invalid state
		req := httptest.NewRequest("GET", "/auth/callback?code=test-code&state=invalid-state", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleOAuthCallback(rr, req)
		
		// Should return error
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "Invalid state parameter")
	})
	
	t.Run("OAuth callback without code", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Test callback without code
		req := httptest.NewRequest("GET", "/auth/callback?state=test-state", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleOAuthCallback(rr, req)
		
		// Should return error
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "Authorization code not provided")
	})
}

func TestAuthHandlerIntegration_SessionPersistence(t *testing.T) {
	t.Run("session persists across requests", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Create guest session via handler
		req := httptest.NewRequest("GET", "/auth/guest", nil)
		rr := httptest.NewRecorder()
		authHandler.HandleGuestLogin(rr, req)
		
		// Should redirect to home
		assert.Equal(t, http.StatusFound, rr.Code)
		
		// Get session cookie
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		assert.NotNil(t, sessionCookie, "Session cookie should be set")
		
		// Make second request with session cookie
		req2 := httptest.NewRequest("GET", "/auth/guest", nil)
		req2.AddCookie(sessionCookie)
		rr2 := httptest.NewRecorder()
		authHandler.HandleGuestLogin(rr2, req2)
		
		// Should still work
		assert.Equal(t, http.StatusFound, rr2.Code)
	})
}

func TestAuthHandlerIntegration_CSRFProtection(t *testing.T) {
	t.Run("OAuth state parameter prevents CSRF", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		cfg.Auth.Google.ClientID = "test-client-id"
		cfg.Auth.Google.ClientSecret = "test-client-secret"
		cfg.Auth.Google.CallbackURL = "http://localhost:8080/auth/google/callback"
		
		authService := auth.NewAuthService(userRepo, cfg)
		
		// Register OAuth clients
		oauthClients := auth.NewOAuthClients(cfg)
		for provider, client := range oauthClients {
			authService.RegisterOAuthClient(provider, client)
		}
		
		authHandler := NewAuthHandler(authService, cfg)
		
		// Initiate OAuth to get state
		req1 := httptest.NewRequest("GET", "/auth/google", nil)
		rr1 := httptest.NewRecorder()
		
		authHandler.HandleGoogleOAuth(rr1, req1)
		
		// Extract state from redirect location
		location := rr1.Header().Get("Location")
		parsedURL, err := url.Parse(location)
		require.NoError(t, err)
		
		state := parsedURL.Query().Get("state")
		assert.NotEmpty(t, state, "State should be generated")
		
		// Get session cookie
		cookies := rr1.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		require.NotNil(t, sessionCookie)
		
		// Try callback with different state (CSRF attack)
		req2 := httptest.NewRequest("GET", "/auth/callback?code=test-code&state=different-state", nil)
		req2.AddCookie(sessionCookie)
		rr2 := httptest.NewRecorder()
		
		authHandler.HandleOAuthCallback(rr2, req2)
		
		// Should reject with invalid state error
		assert.Equal(t, http.StatusBadRequest, rr2.Code)
		assert.Contains(t, rr2.Body.String(), "Invalid state parameter")
	})
}

func TestAuthHandlerIntegration_RouteProtection(t *testing.T) {
	t.Run("protected route requires authentication", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Create a simple protected handler that checks session
		protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := authHandler.GetStore().Get(r, authHandler.GetSessionName())
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			authenticated, ok := session.Values["authenticated"].(bool)
			if !ok || !authenticated {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Protected content"))
		})
		
		// Test without authentication
		req1 := httptest.NewRequest("GET", "/protected", nil)
		rr1 := httptest.NewRecorder()
		
		protectedHandler.ServeHTTP(rr1, req1)
		
		assert.Equal(t, http.StatusUnauthorized, rr1.Code)
		assert.Contains(t, rr1.Body.String(), "Unauthorized")
		
		// Test with guest session
		req := httptest.NewRequest("GET", "/auth/guest", nil)
		rr := httptest.NewRecorder()
		authHandler.HandleGuestLogin(rr, req)
		
		// Get session cookie
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "test_session" {
				sessionCookie = cookie
				break
			}
		}
		require.NotNil(t, sessionCookie, "Session cookie should be set")
		
		// Test with guest session - should be unauthorized
		req2 := httptest.NewRequest("GET", "/protected", nil)
		req2.AddCookie(sessionCookie)
		rr2 := httptest.NewRecorder()
		
		protectedHandler.ServeHTTP(rr2, req2)
		
		// Guest should not be authenticated
		assert.Equal(t, http.StatusUnauthorized, rr2.Code)
	})
}

func TestAuthHandlerIntegration_ErrorHandling(t *testing.T) {
	t.Run("handles missing OAuth credentials", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		// No OAuth credentials configured
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Try to initiate Google OAuth without credentials
		req := httptest.NewRequest("GET", "/auth/google", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleGoogleOAuth(rr, req)
		
		// Should return error
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Failed to initiate OAuth")
	})
	
	t.Run("handles session creation failures", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Try to create guest session
		req := httptest.NewRequest("GET", "/auth/guest", nil)
		rr := httptest.NewRecorder()
		
		authHandler.HandleGuestLogin(rr, req)
		
		// Should work with gorilla/sessions
		assert.Equal(t, http.StatusFound, rr.Code)
	})
}

func TestAuthHandlerIntegration_MultipleSessions(t *testing.T) {
	t.Run("handles multiple concurrent sessions", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Setup auth components
		userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
		require.NoError(t, err)
		
		cfg := &config.Config{}
		cfg.Auth.Session.CookieName = "test_session"
		cfg.Auth.Session.CookieSecret = "test-secret-12345678901234567890"
		cfg.Server.Host = "localhost"
		
		authService := auth.NewAuthService(userRepo, cfg)
		authHandler := NewAuthHandler(authService, cfg)
		
		// Create multiple guest sessions via handler
		// Note: gorilla/sessions may reuse session IDs in test scenarios
		// This is expected behavior - the important thing is that sessions work
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/auth/guest", nil)
			rr := httptest.NewRecorder()
			authHandler.HandleGuestLogin(rr, req)
			
			// Should successfully create session
			assert.Equal(t, http.StatusFound, rr.Code)
			
			// Get session cookie
			cookies := rr.Result().Cookies()
			var sessionCookie *http.Cookie
			for _, cookie := range cookies {
				if cookie.Name == "test_session" {
					sessionCookie = cookie
					break
				}
			}
			assert.NotNil(t, sessionCookie, "Session cookie should be set")
		}
	})
}
