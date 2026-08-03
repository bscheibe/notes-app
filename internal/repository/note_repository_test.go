package repository

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"notes-app/internal/models"
)

func setupTestRepo(t *testing.T) (*NoteRepository, string) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := NewNoteRepository(tempDir, logger)
	require.NoError(t, err)
	require.NotNil(t, repo)

	return repo, tempDir
}

func TestNewNoteRepository(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := filepath.Join(tempDir, "notes")
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		repo, err := NewNoteRepository(newDir, logger)
		require.NoError(t, err)
		assert.NotNil(t, repo)

		// Verify directory was created
		info, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("uses existing directory", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		repo, err := NewNoteRepository(tempDir, logger)
		require.NoError(t, err)
		assert.NotNil(t, repo)
	})
}

func TestNoteRepository_Save(t *testing.T) {
	repo, _ := setupTestRepo(t)

	tests := []struct {
		name    string
		request *models.CreateNoteRequest
		wantErr bool
	}{
		{
			name: "save new note",
			request: &models.CreateNoteRequest{
				Title:   "Test Note",
				Content: "This is test content",
			},
			wantErr: false,
		},
		{
			name: "save note with special characters in title",
			request: &models.CreateNoteRequest{
				Title:   "Test Note! @#$%",
				Content: "Content",
			},
			wantErr: false,
		},
		{
			name: "save note with multiline content",
			request: &models.CreateNoteRequest{
				Title:   "Multiline Note",
				Content: "Line 1\nLine 2\nLine 3",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := repo.Save("test-user", tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, note)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, note)
				assert.NotEmpty(t, note.Filename)
				assert.Equal(t, tt.request.Title, note.Title)
				assert.Equal(t, tt.request.Content, note.Content)
				assert.True(t, note.Modified.IsZero() == false)

				// Verify file was created
				filePath := filepath.Join(repo.baseDirectory, "test-user", note.Filename)
				_, err := os.Stat(filePath)
				assert.NoError(t, err)
			}
		})
	}
}

func TestNoteRepository_Get(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Create a test note first
	createReq := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	createdNote, err := repo.Save("test-user", createReq)
	require.NoError(t, err)

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "get existing note",
			filename: createdNote.Filename,
			wantErr:  false,
		},
		{
			name:     "get non-existent note",
			filename: "nonexistent.md",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := repo.Get("test-user", tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
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

func TestNoteRepository_List(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Create some test notes
	for _, title := range []string{"First Note", "Second Note", "Third Note"} {
		req := &models.CreateNoteRequest{
			Title:   title,
			Content: "Content",
		}
		_, err := repo.Save("test-user", req)
		require.NoError(t, err)
	}

	// Create a non-markdown file (should be ignored)
	nonMarkdownFile := filepath.Join(repo.baseDirectory, "test-user", "readme.txt")
	err := os.WriteFile(nonMarkdownFile, []byte("text"), 0644)
	require.NoError(t, err)

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(repo.baseDirectory, "test-user", "subdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// List notes
	list, err := repo.List("test-user")
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all are markdown files
	for _, filename := range list {
		assert.True(t, filepath.Ext(filename) == ".md")
	}
}

func TestNoteRepository_Delete(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Create a test note
	createReq := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	createdNote, err := repo.Save("test-user", createReq)
	require.NoError(t, err)

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "delete existing note",
			filename: createdNote.Filename,
			wantErr:  false,
		},
		{
			name:     "delete non-existent note",
			filename: "nonexistent.md",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Delete("test-user", tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify file was deleted
				filePath := filepath.Join(repo.baseDirectory, tt.filename)
				_, err := os.Stat(filePath)
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}

func TestNoteRepository_Update(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Create initial note
	createReq := &models.CreateNoteRequest{
		Title:   "Original Title",
		Content: "Original content",
	}
	originalNote, err := repo.Save("test-user", createReq)
	require.NoError(t, err)

	// Update with same title (should update in place)
	updateReq := &models.CreateNoteRequest{
		Title:            "Original Title",
		Content:          "Updated content",
		OriginalFilename: originalNote.Filename,
	}
	updatedNote, err := repo.Save("test-user", updateReq)
	require.NoError(t, err)

	// When title is same, filename should remain the same
	assert.Equal(t, originalNote.Filename, updatedNote.Filename)
	assert.Equal(t, "Updated content", updatedNote.Content)

	// Update with different title (should create new file)
	updateReq2 := &models.CreateNoteRequest{
		Title:            "New Title",
		Content:          "New content",
		OriginalFilename: originalNote.Filename,
	}
	newNote, err := repo.Save("test-user", updateReq2)
	require.NoError(t, err)

	// When title changes, filename should be different
	assert.NotEqual(t, originalNote.Filename, newNote.Filename)
	assert.Equal(t, "New Title", newNote.Title)
}

func TestNoteRepository_Ping(t *testing.T) {
	t.Run("successful ping", func(t *testing.T) {
		repo, _ := setupTestRepo(t)
		err := repo.Ping()
		assert.NoError(t, err)
	})

	t.Run("ping with invalid directory", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		repo := &NoteRepository{
			baseDirectory: "/nonexistent/directory/path",
			logger:        logger,
		}
		err := repo.Ping()
		assert.Error(t, err)
	})
}

func TestExtractTitleFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "simple filename",
			filename: "test-note-2024-01-01-12-00-00.md",
			want:     "test-note",
		},
		{
			name:     "filename with underscores",
			filename: "my_test_note-2024-01-01-12-00-00.md",
			want:     "my_test_note",
		},
		{
			name:     "filename without timestamp",
			filename: "simple.md",
			want:     "simple",
		},
		{
			name:     "filename with special chars",
			filename: "test-note-123.md",
			want:     "test-note-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleFromFilename(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "simple title",
			title: "Test Note",
			want:  "Test-Note",
		},
		{
			name:  "title with special characters",
			title: "Test Note! @#$%",
			want:  "Test-Note-",
		},
		{
			name:  "title with spaces",
			title: "My Long Title Name",
			want:  "My-Long-Title-Name",
		},
		{
			name:  "title with underscores",
			title: "test_note_name",
			want:  "test_note_name",
		},
		{
			name:  "title with numbers",
			title: "Test 123 Note",
			want:  "Test-123-Note",
		},
		{
			name:  "title with mixed case",
			title: "MixedCaseTitle",
			want:  "MixedCaseTitle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.title)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNoteRepository_GenerateFilename(t *testing.T) {
	repo, _ := setupTestRepo(t)

	t.Run("new note gets timestamp", func(t *testing.T) {
		filename := repo.generateFilename("Test Note", "")

		assert.Contains(t, filename, "Test-Note")
		assert.True(t, filepath.Ext(filename) == ".md")
		// Verify it has multiple hyphens (indicating timestamp)
		assert.GreaterOrEqual(t, strings.Count(filename, "-"), 3)
	})

	t.Run("same title with original filename keeps filename", func(t *testing.T) {
		original := "Test-Note-2024-01-01-12-00-00.md"
		filename := repo.generateFilename("Test Note", original)
		assert.Equal(t, original, filename)
	})

	t.Run("different title creates new filename", func(t *testing.T) {
		original := "Test-Note-2024-01-01-12-00-00.md"
		filename := repo.generateFilename("Different Title", original)
		assert.NotEqual(t, original, filename)
		assert.Contains(t, filename, "Different-Title")
	})
}

// Benchmarks for performance-critical operations

func BenchmarkSanitizeFilename(b *testing.B) {
	testCases := []string{
		"Test Note",
		"My Long Title Name With Many Words",
		"Test Note! @#$%",
		"test_note_name",
		"Test 123 Note",
	}

	for _, tc := range testCases {
		b.Run(tc, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sanitizeFilename(tc)
			}
		})
	}
}

func BenchmarkExtractTitleFromFilename(b *testing.B) {
	testCases := []string{
		"test-note-2024-01-01-12-00-00.md",
		"my_test_note-2024-01-01-12-00-00.md",
		"simple.md",
		"very-long-filename-with-many-hyphens-2024-01-01-12-00-00.md",
	}

	for _, tc := range testCases {
		b.Run(tc, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				extractTitleFromFilename(tc)
			}
		})
	}
}

func BenchmarkNoteRepository_Save(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	req := &models.CreateNoteRequest{
		Title:   "Benchmark Note",
		Content: "This is benchmark content for testing save performance",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.Save("test-user", req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNoteRepository_Get(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	// Create a test note
	req := &models.CreateNoteRequest{
		Title:   "Benchmark Note",
		Content: "This is benchmark content for testing get performance",
	}
	note, err := repo.Save("test-user", req)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.Get("test-user", note.Filename)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNoteRepository_List(b *testing.B) {
	tempDir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := NewNoteRepository(tempDir, logger)
	if err != nil {
		b.Fatal(err)
	}

	// Create some test notes
	for i := 0; i < 10; i++ {
		req := &models.CreateNoteRequest{
			Title:   "Test Note",
			Content: "Content",
		}
		_, err := repo.Save("test-user", req)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.List("test-user")
		if err != nil {
			b.Fatal(err)
		}
	}
}
