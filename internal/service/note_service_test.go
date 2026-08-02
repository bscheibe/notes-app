package service

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"notes-app/internal/models"
	"notes-app/internal/repository"
)

func setupTestService(t *testing.T) (*NoteService, *repository.NoteRepository) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	require.NoError(t, err)

	service := NewNoteService(repo, logger)
	return service, repo
}

func TestNoteService_CreateNote(t *testing.T) {
	service, _ := setupTestService(t)

	tests := []struct {
		name        string
		request     *models.CreateNoteRequest
		wantErr     bool
		expectedErr error
	}{
		{
			name: "successful note creation",
			request: &models.CreateNoteRequest{
				Title:   "Test Note",
				Content: "Test content",
			},
			wantErr: false,
		},
		{
			name: "empty title returns error",
			request: &models.CreateNoteRequest{
				Title:   "",
				Content: "Test content",
			},
			wantErr:     true,
			expectedErr: ErrTitleRequired,
		},
		{
			name: "empty content returns error",
			request: &models.CreateNoteRequest{
				Title:   "Test Note",
				Content: "",
			},
			wantErr:     true,
			expectedErr: ErrContentRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := service.CreateNote("test-user", tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, note)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, note)
				assert.Equal(t, tt.request.Title, note.Title)
				assert.Equal(t, tt.request.Content, note.Content)
				assert.NotEmpty(t, note.Filename)
			}
		})
	}
}

func TestNoteService_UpdateNote(t *testing.T) {
	service, _ := setupTestService(t)

	// Create a note first
	createReq := &models.CreateNoteRequest{
		Title:   "Original Note",
		Content: "Original content",
	}
	createdNote, err := service.CreateNote("test-user", createReq)
	require.NoError(t, err)

	tests := []struct {
		name        string
		filename    string
		request     *models.CreateNoteRequest
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "successful note update",
			filename: createdNote.Filename,
			request: &models.CreateNoteRequest{
				Title:   "Updated Note",
				Content: "Updated content",
			},
			wantErr: false,
		},
		{
			name:     "empty title returns error",
			filename: createdNote.Filename,
			request: &models.CreateNoteRequest{
				Title:   "",
				Content: "Test content",
			},
			wantErr:     true,
			expectedErr: ErrTitleRequired,
		},
		{
			name:     "empty content returns error",
			filename: createdNote.Filename,
			request: &models.CreateNoteRequest{
				Title:   "Test Note",
				Content: "",
			},
			wantErr:     true,
			expectedErr: ErrContentRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := service.UpdateNote("test-user", tt.filename, tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, note)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, note)
				assert.Equal(t, tt.request.Title, note.Title)
				assert.Equal(t, tt.request.Content, note.Content)
			}
		})
	}
}

func TestNoteService_GetNote(t *testing.T) {
	service, _ := setupTestService(t)

	// Create a test note first
	createReq := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	createdNote, err := service.CreateNote("test-user", createReq)
	require.NoError(t, err)

	tests := []struct {
		name        string
		filename    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "successful note retrieval",
			filename: createdNote.Filename,
			wantErr:  false,
		},
		{
			name:        "note not found",
			filename:    "nonexistent.md",
			wantErr:     true,
			expectedErr: ErrNoteNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := service.GetNote("test-user", tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err)
				}
				assert.Nil(t, note)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, note)
				assert.Equal(t, tt.filename, note.Filename)
				assert.Equal(t, createReq.Title, note.Title)
				assert.Equal(t, createReq.Content, note.Content)
			}
		})
	}
}

func TestNoteService_ListNotes(t *testing.T) {
	service, _ := setupTestService(t)

	// Create some test notes with different titles
	titles := []string{"First Note", "Second Note", "Third Note"}
	for _, title := range titles {
		req := &models.CreateNoteRequest{
			Title:   title,
			Content: "Content",
		}
		_, err := service.CreateNote("test-user", req)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		wantErr   bool
		wantCount int
	}{
		{
			name:      "successful list",
			wantErr:   false,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes, err := service.ListNotes("test-user")

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, notes)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, notes)
				assert.Equal(t, tt.wantCount, len(notes))
			}
		})
	}
}

func TestNoteService_DeleteNote(t *testing.T) {
	service, _ := setupTestService(t)

	// Create a test note first
	createReq := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	createdNote, err := service.CreateNote("test-user", createReq)
	require.NoError(t, err)

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "successful deletion",
			filename: createdNote.Filename,
			wantErr:  false,
		},
		{
			name:     "file not found",
			filename: "nonexistent.md",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.DeleteNote("test-user", tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify note was deleted
				_, err := service.GetNote("test-user", tt.filename)
				assert.Error(t, err)
				assert.Equal(t, ErrNoteNotFound, err)
			}
		})
	}
}

func TestNoteService_validateRequest(t *testing.T) {
	service, _ := setupTestService(t)

	tests := []struct {
		name    string
		request *models.CreateNoteRequest
		wantErr error
	}{
		{
			name: "valid request",
			request: &models.CreateNoteRequest{
				Title:   "Test",
				Content: "Content",
			},
			wantErr: nil,
		},
		{
			name: "empty title",
			request: &models.CreateNoteRequest{
				Title:   "",
				Content: "Content",
			},
			wantErr: ErrTitleRequired,
		},
		{
			name: "empty content",
			request: &models.CreateNoteRequest{
				Title:   "Test",
				Content: "",
			},
			wantErr: ErrContentRequired,
		},
		{
			name: "both empty",
			request: &models.CreateNoteRequest{
				Title:   "",
				Content: "",
			},
			wantErr: ErrTitleRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateRequest(tt.request)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestNewNoteService(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	require.NoError(t, err)

	service := NewNoteService(repo, logger)

	require.NotNil(t, service)
	assert.Equal(t, repo, service.repo)
	assert.Equal(t, logger, service.logger)
}

// Benchmarks for service layer

func BenchmarkNoteService_CreateNote(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	service := NewNoteService(repo, logger)
	req := &models.CreateNoteRequest{
		Title:   "Benchmark Note",
		Content: "This is benchmark content",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.CreateNote("test-user", req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNoteService_GetNote(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	service := NewNoteService(repo, logger)

	// Create a test note
	req := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	note, err := service.CreateNote("test-user", req)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.GetNote("test-user", note.Filename)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNoteService_ListNotes(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	service := NewNoteService(repo, logger)

	// Create some test notes
	for i := 0; i < 10; i++ {
		req := &models.CreateNoteRequest{
			Title:   "Test Note",
			Content: "Content",
		}
		_, err := service.CreateNote("test-user", req)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.ListNotes("test-user")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNoteService_ValidateRequest(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	service := NewNoteService(repo, logger)
	req := &models.CreateNoteRequest{
		Title:   "Valid Title",
		Content: "Valid content",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := service.validateRequest(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}
