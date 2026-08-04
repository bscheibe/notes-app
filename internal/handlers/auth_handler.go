package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"notes-app/internal/auth"
	"notes-app/internal/config"
	"notes-app/internal/models"
)

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authService *auth.AuthService
	config      *config.Config
	sessionName string
	store       *sessions.CookieStore
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *auth.AuthService, cfg *config.Config) *AuthHandler {
	store := sessions.NewCookieStore([]byte(cfg.Auth.Session.CookieSecret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(cfg.GetSessionDuration().Seconds()),
		HttpOnly: true,
		Secure:   cfg.Server.Host != "localhost", // Secure in production
		SameSite: http.SameSiteLaxMode,
	}

	return &AuthHandler{
		authService: authService,
		config:      cfg,
		sessionName: cfg.Auth.Session.CookieName,
		store:       store,
	}
}

// GetStore returns the session store for use by middleware
func (h *AuthHandler) GetStore() *sessions.CookieStore {
	return h.store
}

// GetSessionName returns the session name for use by middleware
func (h *AuthHandler) GetSessionName() string {
	return h.sessionName
}

// HandleLogin renders the login page
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Render the login page template
	tmpl, err := template.ParseFiles("templates/login.html")
	if err != nil {
		http.Error(w, "Failed to load template", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "text/html")
	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

// HandleGoogleOAuth initiates Google OAuth flow
func (h *AuthHandler) HandleGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		http.Error(w, "Failed to generate OAuth state", http.StatusInternalServerError)
		return
	}

	authURL, err := h.authService.InitiateOAuth(models.ProviderGoogle, state)
	if err != nil {
		http.Error(w, "Failed to initiate OAuth", http.StatusInternalServerError)
		return
	}

	// Store state in session for CSRF protection
	session, err := h.store.Get(r, h.sessionName)
	if err != nil {
		// If session decoding fails, create a new session
		session, _ = h.store.New(r, h.sessionName)
	}

	session.Values["oauth_state"] = state
	session.Values["oauth_provider"] = string(models.ProviderGoogle)
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleGitHubOAuth initiates GitHub OAuth flow
func (h *AuthHandler) HandleGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		http.Error(w, "Failed to generate OAuth state", http.StatusInternalServerError)
		return
	}

	authURL, err := h.authService.InitiateOAuth(models.ProviderGitHub, state)
	if err != nil {
		http.Error(w, "Failed to initiate OAuth", http.StatusInternalServerError)
		return
	}

	// Store state in session for CSRF protection
	session, err := h.store.Get(r, h.sessionName)
	if err != nil {
		// If session decoding fails, create a new session
		session, _ = h.store.New(r, h.sessionName)
	}

	session.Values["oauth_state"] = state
	session.Values["oauth_provider"] = string(models.ProviderGitHub)
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleOAuthCallback handles OAuth callback from providers
func (h *AuthHandler) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		http.Error(w, "Authorization code not provided", http.StatusBadRequest)
		return
	}

	// Verify state for CSRF protection
	session, err := h.store.Get(r, h.sessionName)
	if err != nil {
		http.Error(w, "Invalid session - please try authentication again", http.StatusBadRequest)
		return
	}

	storedState, ok := session.Values["oauth_state"].(string)
	if !ok || storedState != state {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	providerStr, ok := session.Values["oauth_provider"].(string)
	if !ok {
		http.Error(w, "Provider not found in session", http.StatusBadRequest)
		return
	}

	provider := models.Provider(providerStr)

	// Handle OAuth callback
	user, err := h.authService.HandleOAuthCallback(provider, code)
	if err != nil {
		http.Error(w, "Failed to handle OAuth callback", http.StatusInternalServerError)
		return
	}

	// Clear OAuth state from session
	delete(session.Values, "oauth_state")
	delete(session.Values, "oauth_provider")

	// Store user session
	session.Values["user_id"] = user.ID
	session.Values["authenticated"] = true
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleGuestLogin creates a guest session
func (h *AuthHandler) HandleGuestLogin(w http.ResponseWriter, r *http.Request) {
	// Store guest session
	session, err := h.store.Get(r, h.sessionName)
	if err != nil {
		// If session decoding fails (e.g., due to cookie secret change), create a new session
		session, _ = h.store.New(r, h.sessionName)
	}

	session.Values["authenticated"] = false
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout logs out the user
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := h.store.Get(r, h.sessionName)
	if err != nil {
		// If session decoding fails, create a new session to clear cookie
		session, _ = h.store.New(r, h.sessionName)
		session.Options.MaxAge = -1 // Delete cookie
		_ = session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Clear session
	session.Values["user_id"] = nil
	session.Values["authenticated"] = false
	session.Options.MaxAge = -1 // Delete cookie
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// generateState generates a random state parameter for OAuth
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RegisterRoutes registers authentication routes
func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/login", h.HandleLogin)
	r.Get("/auth/google", h.HandleGoogleOAuth)
	r.Get("/auth/github", h.HandleGitHubOAuth)
	r.Get("/auth/callback", h.HandleOAuthCallback)
	r.Get("/auth/guest", h.HandleGuestLogin)
	r.Get("/logout", h.HandleLogout)
}
