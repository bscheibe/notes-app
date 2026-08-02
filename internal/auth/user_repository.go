package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"notes-app/internal/models"
)

// UserRepository interface for user operations
type UserRepository interface {
	CreateUser(user *models.User) error
	GetUserByID(id string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByOAuthID(provider models.Provider, oauthID string) (*models.User, error)
	UpdateUser(user *models.User) error
	DeleteUser(id string) error
}

// FileSystemUserRepository implements UserRepository with file system storage
type FileSystemUserRepository struct {
	directory string
	mu        sync.RWMutex
}

// NewFileSystemUserRepository creates a new file system-based user repository
func NewFileSystemUserRepository(directory string) (*FileSystemUserRepository, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create user directory: %w", err)
	}
	return &FileSystemUserRepository{
		directory: directory,
	}, nil
}

// CreateUser creates a new user
func (r *FileSystemUserRepository) CreateUser(user *models.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if user already exists by email
	existing, _ := r.getUserByEmailNoLock(user.Email)
	if existing != nil {
		return fmt.Errorf("user with email %s already exists", user.Email)
	}

	// Check if user already exists by OAuth ID
	existing, _ = r.getUserByOAuthIDNoLock(user.Provider, user.OAuthID)
	if existing != nil {
		return fmt.Errorf("user with OAuth ID %s already exists for provider %s", user.OAuthID, user.Provider)
	}

	// Set timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// Save user
	if err := r.saveUser(user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func (r *FileSystemUserRepository) GetUserByID(id string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userPath := r.userPath(id)
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // User not found
		}
		return nil, fmt.Errorf("failed to read user file: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email
func (r *FileSystemUserRepository) GetUserByEmail(email string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getUserByEmailNoLock(email)
}

// GetUserByOAuthID retrieves a user by OAuth provider and ID
func (r *FileSystemUserRepository) GetUserByOAuthID(provider models.Provider, oauthID string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getUserByOAuthIDNoLock(provider, oauthID)
}

// UpdateUser updates an existing user
func (r *FileSystemUserRepository) UpdateUser(user *models.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if user exists
	existing, err := r.getUserByIDNoLock(user.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("user with ID %s not found", user.ID)
	}

	// Update timestamp
	user.UpdatedAt = time.Now()

	// Save user
	if err := r.saveUser(user); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// DeleteUser deletes a user by ID
func (r *FileSystemUserRepository) DeleteUser(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	userPath := r.userPath(id)
	if err := os.Remove(userPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// saveUser writes a user to disk
func (r *FileSystemUserRepository) saveUser(user *models.User) error {
	userPath := r.userPath(user.ID)
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return os.WriteFile(userPath, data, 0644)
}

// userPath returns the file path for a user
func (r *FileSystemUserRepository) userPath(id string) string {
	return filepath.Join(r.directory, id+".json")
}

// getUserByIDNoLock retrieves a user by ID without locking (must be called with lock held)
func (r *FileSystemUserRepository) getUserByIDNoLock(id string) (*models.User, error) {
	userPath := r.userPath(id)
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // User not found
		}
		return nil, fmt.Errorf("failed to read user file: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// getUserByEmailNoLock retrieves a user by email without locking (must be called with lock held)
func (r *FileSystemUserRepository) getUserByEmailNoLock(email string) (*models.User, error) {
	entries, err := os.ReadDir(r.directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read user directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		userPath := filepath.Join(r.directory, entry.Name())
		data, err := os.ReadFile(userPath)
		if err != nil {
			continue
		}

		var user models.User
		if err := json.Unmarshal(data, &user); err != nil {
			continue
		}

		if user.Email == email {
			return &user, nil
		}
	}

	return nil, nil // User not found
}

// getUserByOAuthIDNoLock retrieves a user by OAuth ID without locking (must be called with lock held)
func (r *FileSystemUserRepository) getUserByOAuthIDNoLock(provider models.Provider, oauthID string) (*models.User, error) {
	entries, err := os.ReadDir(r.directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read user directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		userPath := filepath.Join(r.directory, entry.Name())
		data, err := os.ReadFile(userPath)
		if err != nil {
			continue
		}

		var user models.User
		if err := json.Unmarshal(data, &user); err != nil {
			continue
		}

		if user.Provider == provider && user.OAuthID == oauthID {
			return &user, nil
		}
	}

	return nil, nil // User not found
}
