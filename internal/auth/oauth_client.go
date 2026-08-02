package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"notes-app/internal/config"
	"notes-app/internal/models"
)

// GoogleOAuthClient implements OAuthClient for Google
type GoogleOAuthClient struct {
	config *oauth2.Config
}

// NewGoogleOAuthClient creates a new Google OAuth client
func NewGoogleOAuthClient(cfg *config.Config) *GoogleOAuthClient {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.Auth.Google.ClientID,
		ClientSecret: cfg.Auth.Google.ClientSecret,
		RedirectURL:  cfg.Auth.Google.CallbackURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	return &GoogleOAuthClient{
		config: oauthConfig,
	}
}

// GetAuthURL returns the Google OAuth authorization URL
func (g *GoogleOAuthClient) GetAuthURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// ExchangeCode exchanges the authorization code for user profile
func (g *GoogleOAuthClient) ExchangeCode(code string) (*OAuthProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var googleUser struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.Unmarshal(body, &googleUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user info: %w", err)
	}

	return &OAuthProfile{
		ID:        googleUser.ID,
		Email:     googleUser.Email,
		Name:      googleUser.Name,
		AvatarURL: googleUser.Picture,
	}, nil
}

// GitHubOAuthClient implements OAuthClient for GitHub
type GitHubOAuthClient struct {
	config *oauth2.Config
}

// NewGitHubOAuthClient creates a new GitHub OAuth client
func NewGitHubOAuthClient(cfg *config.Config) *GitHubOAuthClient {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.Auth.GitHub.ClientID,
		ClientSecret: cfg.Auth.GitHub.ClientSecret,
		RedirectURL:  cfg.Auth.GitHub.CallbackURL,
		Scopes: []string{
			"user:email",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
	}

	return &GitHubOAuthClient{
		config: oauthConfig,
	}
}

// GetAuthURL returns the GitHub OAuth authorization URL
func (g *GitHubOAuthClient) GetAuthURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges the authorization code for user profile
func (g *GitHubOAuthClient) ExchangeCode(code string) (*OAuthProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var githubUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.Unmarshal(body, &githubUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user info: %w", err)
	}

	// GitHub might not return email in the user endpoint, need to fetch separately
	email := githubUser.Email
	if email == "" {
		email, err = g.getPrimaryEmail(client)
		if err != nil {
			// If we can't get email, fail gracefully
			return nil, fmt.Errorf("failed to retrieve primary email from GitHub: %w", err)
		}
	}

	return &OAuthProfile{
		ID:        fmt.Sprintf("%d", githubUser.ID),
		Email:     email,
		Name:      githubUser.Name,
		AvatarURL: githubUser.AvatarURL,
	}, nil
}

// getPrimaryEmail fetches the primary email from GitHub
func (g *GitHubOAuthClient) getPrimaryEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", fmt.Errorf("failed to get user emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}

	if err := json.Unmarshal(body, &emails); err != nil {
		return "", fmt.Errorf("failed to unmarshal emails: %w", err)
	}

	for _, email := range emails {
		if email.Primary {
			return email.Email, nil
		}
	}

	return "", fmt.Errorf("no primary email found")
}

// NewOAuthClients creates OAuth clients for all configured providers
func NewOAuthClients(cfg *config.Config) map[models.Provider]OAuthClient {
	clients := make(map[models.Provider]OAuthClient)

	if cfg.Auth.Google.ClientID != "" && cfg.Auth.Google.ClientSecret != "" {
		clients[models.ProviderGoogle] = NewGoogleOAuthClient(cfg)
	}

	if cfg.Auth.GitHub.ClientID != "" && cfg.Auth.GitHub.ClientSecret != "" {
		clients[models.ProviderGitHub] = NewGitHubOAuthClient(cfg)
	}

	return clients
}
