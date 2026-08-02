package repository

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"notes-app/internal/models"
)

// NoteRepository handles file system operations for notes
type NoteRepository struct {
	baseDirectory string
	logger        *slog.Logger
}

// NewNoteRepository creates a new note repository
func NewNoteRepository(directory string, logger *slog.Logger) (*NoteRepository, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create notes directory: %w", err)
	}

	logger.Info("Notes repository initialized", "directory", directory)

	return &NoteRepository{
		baseDirectory: directory,
		logger:        logger,
	}, nil
}

// getUserDirectory returns the directory for a specific user/session
func (r *NoteRepository) getUserDirectory(userID string) string {
	if userID == "" {
		// Should not happen with session isolation, but provide base directory as fallback
		r.logger.Warn("Empty userID provided, using base directory - this indicates a session isolation issue")
		return r.baseDirectory
	}
	return filepath.Join(r.baseDirectory, userID)
}

// ensureUserDirectory creates the user directory if it doesn't exist
func (r *NoteRepository) ensureUserDirectory(userID string) error {
	userDir := r.getUserDirectory(userID)
	return os.MkdirAll(userDir, 0755)
}

// List returns all markdown files in the repository for a specific user
func (r *NoteRepository) List(userID string) ([]string, error) {
	userDir := r.getUserDirectory(userID)
	files, err := os.ReadDir(userDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var notes []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".md" {
			notes = append(notes, file.Name())
		}
	}

	return notes, nil
}

// Get retrieves a note by filename for a specific user
func (r *NoteRepository) Get(userID string, filename string) (*models.Note, error) {
	userDir := r.getUserDirectory(userID)
	filePath := filepath.Join(userDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read note: %w", err)
	}

	// Get file info for timestamps
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Extract title from filename
	title := extractTitleFromFilename(filename)

	return &models.Note{
		Filename: filename,
		Title:    strings.ReplaceAll(title, "-", " "),
		Content:  string(content),
		Created:  info.ModTime(), // Using modtime as created time
		Modified: info.ModTime(),
	}, nil
}

// Save creates or updates a note for a specific user
func (r *NoteRepository) Save(userID string, req *models.CreateNoteRequest) (*models.Note, error) {
	// Ensure user directory exists
	if err := r.ensureUserDirectory(userID); err != nil {
		return nil, fmt.Errorf("failed to create user directory: %w", err)
	}

	filename := r.generateFilename(req.Title, req.OriginalFilename)
	userDir := r.getUserDirectory(userID)
	filePath := filepath.Join(userDir, filename)

	// Write content to file
	if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
		return nil, fmt.Errorf("failed to save note: %w", err)
	}

	r.logger.Info("Note saved", "filename", filename, "user_id", userID, "size", len(req.Content))

	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	return &models.Note{
		Filename: filename,
		Title:    req.Title,
		Content:  req.Content,
		Created:  info.ModTime(),
		Modified: info.ModTime(),
	}, nil
}

// Delete removes a note by filename for a specific user
func (r *NoteRepository) Delete(userID string, filename string) error {
	userDir := r.getUserDirectory(userID)
	filePath := filepath.Join(userDir, filename)
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	r.logger.Info("Note deleted", "filename", filename, "user_id", userID)
	return nil
}

// Ping checks if the repository is accessible
func (r *NoteRepository) Ping() error {
	_, err := os.Stat(r.baseDirectory)
	if err != nil {
		return fmt.Errorf("repository directory not accessible: %w", err)
	}
	return nil
}

// generateFilename creates a filename based on title and optional original filename
func (r *NoteRepository) generateFilename(title, originalFilename string) string {
	sanitizedTitle := sanitizeFilename(title)

	// If editing an existing note, check if title changed
	if originalFilename != "" {
		expectedPrefix := sanitizedTitle + "-"
		if strings.HasPrefix(originalFilename, expectedPrefix) {
			// Title same, update existing file
			return originalFilename
		}
	}

	// New note or title changed, create new file with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	return fmt.Sprintf("%s-%s.md", sanitizedTitle, timestamp)
}

// extractTitleFromFilename extracts the title from a filename
func extractTitleFromFilename(filename string) string {
	// Remove .md extension
	name := strings.TrimSuffix(filename, ".md")
	// Remove timestamp (last 19 characters: "YYYY-MM-DD-HH-MM-SS")
	if len(name) > 20 {
		// Find the last hyphen before the timestamp
		lastHyphen := strings.LastIndex(name[:len(name)-19], "-")
		if lastHyphen != -1 {
			return name[:lastHyphen]
		}
	}
	return name
}

// sanitizeFilename removes unsafe characters from filename
func sanitizeFilename(title string) string {
	var result []rune
	for _, char := range title {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == ' ' {
			result = append(result, char)
		}
	}
	// Replace spaces with hyphens
	sanitized := string(result)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	return sanitized
}
