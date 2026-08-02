package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"notes-app/internal/models"
)

// MockAuthService is a mock implementation of AuthService
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) ValidateUser(userID string) (*models.User, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func TestAuthMiddleware_RequireAuth(t *testing.T) {
	t.Run("redirects to login when not authenticated", func(t *testing.T) {
		store := sessions.NewCookieStore([]byte("test-secret"))
		mockAuthService := new(MockAuthService)
		middleware := NewAuthMiddleware(store, "test_session", mockAuthService)

		req := httptest.NewRequest("GET", "/protected", nil)
		rr := httptest.NewRecorder()

		handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusFound, rr.Code)
		assert.Equal(t, "/login", rr.Header().Get("Location"))
	})
}

func TestAuthMiddleware_OptionalAuth(t *testing.T) {
	t.Run("continues without user when not authenticated", func(t *testing.T) {
		store := sessions.NewCookieStore([]byte("test-secret"))
		mockAuthService := new(MockAuthService)
		middleware := NewAuthMiddleware(store, "test_session", mockAuthService)

		req := httptest.NewRequest("GET", "/public", nil)
		rr := httptest.NewRecorder()

		handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
