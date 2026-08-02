package service

import (
	"errors"
	"log/slog"

	"notes-app/internal/models"
	"notes-app/internal/repository"
)

var (
	ErrTitleRequired   = errors.New("title is required")
	ErrContentRequired = errors.New("content is required")
	ErrNoteNotFound    = errors.New("note not found")
)

// NoteService handles business logic for notes
type NoteService struct {
	repo   *repository.NoteRepository
	logger *slog.Logger
}

// NewNoteService creates a new note service
func NewNoteService(repo *repository.NoteRepository, logger *slog.Logger) *NoteService {
	return &NoteService{
		repo:   repo,
		logger: logger,
	}
}

// ListNotes returns all available notes for a user
func (s *NoteService) ListNotes(userID string) ([]string, error) {
	notes, err := s.repo.List(userID)
	if err != nil {
		s.logger.Error("Failed to list notes", "user_id", userID, "error", err)
		return nil, err
	}
	return notes, nil
}

// GetNote retrieves a specific note by filename for a user
func (s *NoteService) GetNote(userID string, filename string) (*models.Note, error) {
	note, err := s.repo.Get(userID, filename)
	if err != nil {
		s.logger.Error("Failed to get note", "filename", filename, "user_id", userID, "error", err)
		return nil, ErrNoteNotFound
	}
	return note, nil
}

// CreateNote creates a new note for a user
func (s *NoteService) CreateNote(userID string, req *models.CreateNoteRequest) (*models.Note, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	note, err := s.repo.Save(userID, req)
	if err != nil {
		s.logger.Error("Failed to create note", "title", req.Title, "user_id", userID, "error", err)
		return nil, err
	}

	s.logger.Info("Note created", "filename", note.Filename, "title", note.Title, "user_id", userID)
	return note, nil
}

// UpdateNote updates an existing note for a user
func (s *NoteService) UpdateNote(userID string, filename string, req *models.CreateNoteRequest) (*models.Note, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	// Set the original filename to enable proper file handling
	req.OriginalFilename = filename

	note, err := s.repo.Save(userID, req)
	if err != nil {
		s.logger.Error("Failed to update note", "filename", filename, "user_id", userID, "error", err)
		return nil, err
	}

	s.logger.Info("Note updated", "filename", note.Filename, "title", note.Title, "user_id", userID)
	return note, nil
}

// DeleteNote removes a note for a user
func (s *NoteService) DeleteNote(userID string, filename string) error {
	if err := s.repo.Delete(userID, filename); err != nil {
		s.logger.Error("Failed to delete note", "filename", filename, "user_id", userID, "error", err)
		return err
	}

	s.logger.Info("Note deleted", "filename", filename, "user_id", userID)
	return nil
}

// validateRequest validates the create/update request
func (s *NoteService) validateRequest(req *models.CreateNoteRequest) error {
	if req.Title == "" {
		return ErrTitleRequired
	}
	if req.Content == "" {
		return ErrContentRequired
	}
	return nil
}
