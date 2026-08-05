package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"notes-app/internal/middleware"
	"notes-app/internal/models"
	"notes-app/internal/monitoring"
	"notes-app/internal/service"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// NoteHandler handles HTTP requests for notes
type NoteHandler struct {
	service *service.NoteService
	logger  *slog.Logger
	metrics *monitoring.Metrics
	tracer  *monitoring.Tracer
}

// NewNoteHandler creates a new note handler
func NewNoteHandler(service *service.NoteService, logger *slog.Logger, metrics *monitoring.Metrics, tracer *monitoring.Tracer) (*NoteHandler, error) {
	return &NoteHandler{
		service: service,
		logger:  logger,
		metrics: metrics,
		tracer:  tracer,
	}, nil
}

// HandleHome lists notes for the current user
func (h *NoteHandler) HandleHome(w http.ResponseWriter, r *http.Request) {
	// Start tracing span if enabled
	var span trace.Span
	if h.tracer != nil {
		_, span = h.tracer.StartSpan(r.Context(), "HandleHome")
		defer span.End()
	}

	// Get user ID from context (either user ID or session ID for guests)
	userID := ""
	if user, userExists := middleware.GetUserFromContext(r); userExists {
		userID = user.ID
	} else if sessionID, sessionExists := middleware.GetSessionIDFromContext(r); sessionExists {
		userID = sessionID
	}

	notes, err := h.service.ListNotes(userID)
	if err != nil {
		h.logger.Error("Failed to list notes", "error", err)
		if h.tracer != nil {
			h.tracer.RecordError(span, err)
		}
		if h.metrics != nil {
			h.metrics.RecordNoteReadError()
		}
		h.writeError(w, http.StatusInternalServerError, "Failed to load notes")
		return
	}

	h.writeJSON(w, http.StatusOK, models.NoteList{Notes: toNoteStubs(notes)})
}

// HandleViewNote returns a specific note
func (h *NoteHandler) HandleViewNote(w http.ResponseWriter, r *http.Request) {
	// Start tracing span if enabled
	var span trace.Span
	if h.tracer != nil {
		_, span = h.tracer.StartSpan(r.Context(), "HandleViewNote")
		defer span.End()
	}

	filename := chi.URLParam(r, "filename")
	if filename == "" {
		h.writeError(w, http.StatusBadRequest, "Filename is required")
		return
	}

	// Get user ID from context (either user ID or session ID for guests)
	userID := ""
	if user, userExists := middleware.GetUserFromContext(r); userExists {
		userID = user.ID
	} else if sessionID, sessionExists := middleware.GetSessionIDFromContext(r); sessionExists {
		userID = sessionID
	}

	note, err := h.service.GetNote(userID, filename)
	if err != nil {
		h.logger.Error("Note not found", "filename", filename, "error", err)
		if h.tracer != nil {
			h.tracer.RecordError(span, err)
		}
		if h.metrics != nil {
			h.metrics.RecordNoteReadError()
		}
		h.writeError(w, http.StatusNotFound, "Note not found")
		return
	}

	h.writeJSON(w, http.StatusOK, note)
}

// HandleSaveNote handles creating or updating a note
func (h *NoteHandler) HandleSaveNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start tracing span if enabled
	var span trace.Span
	if h.tracer != nil {
		ctx, span = h.tracer.StartSpan(ctx, "HandleSaveNote")
		defer span.End()
	}

	if err := r.ParseForm(); err != nil {
		h.logger.Error("Error parsing form", "error", err)
		if h.tracer != nil {
			h.tracer.RecordError(span, err)
		}
		h.writeError(w, http.StatusBadRequest, "Error parsing form")
		return
	}

	title := r.FormValue("title")
	content := r.FormValue("content")
	originalFilename := r.FormValue("original_filename")

	// Get user ID from context (either user ID or session ID for guests)
	userID := ""
	if user, userExists := middleware.GetUserFromContext(r); userExists {
		userID = user.ID
	} else if sessionID, sessionExists := middleware.GetSessionIDFromContext(r); sessionExists {
		userID = sessionID
	}

	req := &models.CreateNoteRequest{
		Title:            title,
		Content:          content,
		OriginalFilename: originalFilename,
	}

	var note *models.Note
	var err error

	// If editing an existing note
	if originalFilename != "" {
		note, err = h.service.UpdateNote(userID, originalFilename, req)
		if err == nil && h.metrics != nil {
			h.metrics.RecordNoteUpdated(ctx)
		}
	} else {
		note, err = h.service.CreateNote(userID, req)
		if err == nil && h.metrics != nil {
			h.metrics.RecordNoteCreated(ctx)
		}
	}

	if err != nil {
		h.logger.Error("Failed to save note", "error", err)
		if h.tracer != nil {
			h.tracer.RecordError(span, err)
		}
		if h.metrics != nil {
			h.metrics.RecordNoteWriteError()
		}
		h.writeError(w, http.StatusBadRequest, h.getErrorMessage(err))
		return
	}

	h.writeJSON(w, http.StatusOK, note)
}

// writeJSON writes a JSON response with the given status code
func (h *NoteHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response
func (h *NoteHandler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

// getErrorMessage converts service errors to user-friendly messages
func (h *NoteHandler) getErrorMessage(err error) string {
	switch err {
	case service.ErrTitleRequired:
		return "Please enter a title"
	case service.ErrContentRequired:
		return "Please enter some content"
	case service.ErrNoteNotFound:
		return "Note not found"
	default:
		return "An error occurred while saving your note"
	}
}

// GetLogger returns the logger for external use
func (h *NoteHandler) GetLogger() *slog.Logger {
	return h.logger
}

// toNoteStubs converts filenames into minimal note list entries
func toNoteStubs(filenames []string) []models.Note {
	notes := make([]models.Note, 0, len(filenames))
	for _, filename := range filenames {
		notes = append(notes, models.Note{Filename: filename})
	}
	return notes
}
