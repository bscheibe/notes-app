package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"notes-app/internal/auth"
	"notes-app/internal/models"
)

// contextKey is the type for context keys
type contextKey string

const (
	userContextKey contextKey = "user"
	sessionIDKey   contextKey = "session_id"
)

// SessionIDKey is exported for use in tests
var SessionIDKey = sessionIDKey

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	store       *sessions.CookieStore
	sessionName string
	authService AuthService
}

// AuthService interface for authentication operations
type AuthService interface {
	ValidateUser(userID string) (*models.User, error)
}

// Ensure auth.AuthService implements AuthService interface
var _ AuthService = (*auth.AuthService)(nil)

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(store *sessions.CookieStore, sessionName string, authService AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		store:       store,
		sessionName: sessionName,
		authService: authService,
	}
}

// RequireAuth middleware that requires authentication
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.store.Get(r, m.sessionName)
		if err != nil {
			// If session decoding fails, redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		authenticated, ok := session.Values["authenticated"].(bool)
		if !ok || !authenticated {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Validate user exists
		user, err := m.authService.ValidateUser(userID)
		if err != nil || user == nil {
			// User not found or error, redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth middleware that optionally adds user to context
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.store.Get(r, m.sessionName)
		if err != nil {
			// Existing cookie failed to decode, start a fresh session
			session, _ = m.store.New(r, m.sessionName)
		}

		sessionID, ok := session.Values["session_id"].(string)
		sessionCreated := false
		if !ok || sessionID == "" {
			// No session ID yet (new or pre-existing-but-empty session):
			// mint a unique one and persist it so this guest is isolated
			// from every other guest with no cookie.
			sessionID = uuid.New().String()
			session.Values["session_id"] = sessionID
			if _, hasAuth := session.Values["authenticated"]; !hasAuth {
				session.Values["authenticated"] = false
			}
			sessionCreated = true
		}

		// Always add session ID to context for isolation
		ctx := context.WithValue(r.Context(), sessionIDKey, sessionID)

		authenticated, ok := session.Values["authenticated"].(bool)
		if !ok || !authenticated {
			// Not authenticated, continue with session ID only (guest)
			if sessionCreated {
				_ = session.Save(r, w)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			// No user ID, continue with session ID only (guest)
			if sessionCreated {
				_ = session.Save(r, w)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Validate user exists
		user, err := m.authService.ValidateUser(userID)
		if err != nil || user == nil {
			// User not found or error, continue with session ID only (guest)
			if sessionCreated {
				_ = session.Save(r, w)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Add user to context
		ctx = context.WithValue(ctx, userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext retrieves the user from the request context
func GetUserFromContext(r *http.Request) (*models.User, bool) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	return user, ok
}

// GetSessionIDFromContext retrieves the session ID from the request context
func GetSessionIDFromContext(r *http.Request) (string, bool) {
	sessionID, ok := r.Context().Value(sessionIDKey).(string)
	return sessionID, ok
}
