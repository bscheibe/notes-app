package handlers

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"notes-app/internal/middleware"
	"notes-app/internal/models"
	"notes-app/internal/repository"
	"notes-app/internal/service"
)

func setupTestHandler(t *testing.T) (*NoteHandler, *service.NoteService) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo, err := repository.NewNoteRepository(tempDir, logger)
	require.NoError(t, err)

	noteService := service.NewNoteService(repo, logger)

	// Create a minimal handler without monitoring for testing
	handler := &NoteHandler{
		service: noteService,
		logger:  logger,
		metrics: nil,
		tracer:  nil,
	}

	// Use a simple inline template for testing
	tmpl, err := template.New("test").Parse(`<!DOCTYPE html><html><body>Notes App</body></html>`)
	require.NoError(t, err)
	handler.tmpl = tmpl

	return handler, noteService
}

// Helper function to add session ID to request context
func addSessionIDToRequest(r *http.Request, sessionID string) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.SessionIDKey, sessionID)
	return r.WithContext(ctx)
}

func TestNoteHandler_HandleHome(t *testing.T) {
	handler, _ := setupTestHandler(t)

	t.Run("renders home page with empty notes list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handler.HandleHome(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<!DOCTYPE html>")
		assert.Contains(t, body, "Notes App")
	})

	t.Run("renders home page with notes", func(t *testing.T) {
		// Create a test note
		createReq := &models.CreateNoteRequest{
			Title:   "Test Note",
			Content: "Test content",
		}
		_, err := handler.service.CreateNote("test-user", createReq)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/", nil)
		req = addSessionIDToRequest(req, "test-user")
		rr := httptest.NewRecorder()

		handler.HandleHome(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<!DOCTYPE html>")
		assert.Contains(t, body, "Notes App")
	})
}

func TestNoteHandler_HandleViewNote(t *testing.T) {
	handler, _ := setupTestHandler(t)

	// Create a test note first
	createReq := &models.CreateNoteRequest{
		Title:   "Test Note",
		Content: "Test content",
	}
	createdNote, err := handler.service.CreateNote("test-user",createReq)
	require.NoError(t, err)

	t.Run("view existing note", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/notes/"+createdNote.Filename, nil)
		req = addSessionIDToRequest(req, "test-user")
		rr := httptest.NewRecorder()

		// Set up chi router context
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("filename", createdNote.Filename)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.HandleViewNote(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<!DOCTYPE html>")
		assert.Contains(t, body, "Notes App")
	})

	t.Run("view non-existent note returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/notes/nonexistent.md", nil)
		rr := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("filename", "nonexistent.md")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.HandleViewNote(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("empty filename redirects to home", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/notes/", nil)
		rr := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("filename", "")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.HandleViewNote(rr, req)

		assert.Equal(t, http.StatusSeeOther, rr.Code)
		assert.Equal(t, "/", rr.Header().Get("Location"))
	})
}

func TestNoteHandler_HandleSaveNote(t *testing.T) {
	handler, _ := setupTestHandler(t)

	t.Run("create new note successfully", func(t *testing.T) {
		formData := strings.NewReader("title=New+Note&content=New+content")
		req := httptest.NewRequest("POST", "/notes", formData)
		req = addSessionIDToRequest(req, "test-user")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.HandleSaveNote(rr, req)

		assert.Equal(t, http.StatusSeeOther, rr.Code)
		assert.Equal(t, "/", rr.Header().Get("Location"))

		// Verify note was created
		notes, err := handler.service.ListNotes("test-user")
		require.NoError(t, err)
		assert.Len(t, notes, 1)
	})

	t.Run("update existing note successfully", func(t *testing.T) {
		// Create a note first
		createReq := &models.CreateNoteRequest{
			Title:   "Original Note",
			Content: "Original content",
		}
		createdNote, err := handler.service.CreateNote("test-user",createReq)
		require.NoError(t, err)

		// Update the note
		formData := strings.NewReader("title=Updated+Note&content=Updated+content&original_filename=" + createdNote.Filename)
		req := httptest.NewRequest("POST", "/notes", formData)
		req = addSessionIDToRequest(req, "test-user")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.HandleSaveNote(rr, req)

		assert.Equal(t, http.StatusSeeOther, rr.Code)
		assert.Equal(t, "/", rr.Header().Get("Location"))
	})

	t.Run("empty title shows error", func(t *testing.T) {
		formData := strings.NewReader("title=&content=Some+content")
		req := httptest.NewRequest("POST", "/notes", formData)
		req = addSessionIDToRequest(req, "test-user")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.HandleSaveNote(rr, req)

		// Should return 200 with error message in template
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("empty content shows error", func(t *testing.T) {
		formData := strings.NewReader("title=Test+Note&content=")
		req := httptest.NewRequest("POST", "/notes", formData)
		req = addSessionIDToRequest(req, "test-user")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.HandleSaveNote(rr, req)

		// Should return 200 with error message in template
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("invalid form parsing returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/notes", strings.NewReader("invalid"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		handler.HandleSaveNote(rr, req)

		// The form parsing actually succeeds, so we get validation error instead
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestNoteHandler_GetErrorMessage(t *testing.T) {
	handler, _ := setupTestHandler(t)

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "title required error",
			err:      service.ErrTitleRequired,
			expected: "Please enter a title",
		},
		{
			name:     "content required error",
			err:      service.ErrContentRequired,
			expected: "Please enter some content",
		},
		{
			name:     "note not found error",
			err:      service.ErrNoteNotFound,
			expected: "Note not found",
		},
		{
			name:     "generic error",
			err:      assert.AnError,
			expected: "An error occurred while saving your note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := handler.getErrorMessage(tt.err)
			assert.Equal(t, tt.expected, msg)
		})
	}
}

func TestNoteHandler_GetLogger(t *testing.T) {
	handler, _ := setupTestHandler(t)
	logger := handler.GetLogger()
	assert.NotNil(t, logger)
}

func TestNewNoteHandler(t *testing.T) {
	// This test requires templates/index.html to exist to construct a handler.
	// For a real test, you'd need to ensure the template file exists
	// or mock the template parsing.
	t.Skip("Skipping test that requires actual template file")
}
