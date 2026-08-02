package handlers

import (
	"html/template"
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
	tmpl    *template.Template
	metrics *monitoring.Metrics
	tracer  *monitoring.Tracer
}

// PageData represents the data passed to HTML templates
type PageData struct {
	Message          string
	Title            string
	Content          string
	OriginalFilename string
	Notes            []string
	User             *UserInfo
}

// UserInfo represents user information for templates
type UserInfo struct {
	Name      string
	AvatarURL string
	IsGuest   bool
}

// NewNoteHandler creates a new note handler
func NewNoteHandler(service *service.NoteService, logger *slog.Logger, metrics *monitoring.Metrics, tracer *monitoring.Tracer) (*NoteHandler, error) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		return nil, err
	}

	return &NoteHandler{
		service: service,
		logger:  logger,
		tmpl:    tmpl,
		metrics: metrics,
		tracer:  tracer,
	}, nil
}

// HandleHome handles the home page with note list
func (h *NoteHandler) HandleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start tracing span if enabled
	var span trace.Span
	if h.tracer != nil {
		ctx, span = h.tracer.StartSpan(ctx, "HandleHome")
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
		h.renderError(w, "Failed to load notes")
		return
	}

	// Get user from context
	var userInfo *UserInfo
	user, userExists := middleware.GetUserFromContext(r)
	if userExists {
		userInfo = &UserInfo{
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			IsGuest:   user.Provider == models.ProviderGuest,
		}
	}

	data := PageData{
		Message: "",
		Notes:   notes,
		User:    userInfo,
	}

	h.renderTemplate(w, data)
}

// HandleViewNote handles viewing a specific note
func (h *NoteHandler) HandleViewNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start tracing span if enabled
	var span trace.Span
	if h.tracer != nil {
		ctx, span = h.tracer.StartSpan(ctx, "HandleViewNote")
		defer span.End()
	}

	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}

	// Get all notes for the sidebar
	// userID is already set above

	notes, err := h.service.ListNotes(userID)
	if err != nil {
		h.logger.Error("Failed to list notes", "error", err)
	}

	// Get user from context
	var userInfo *UserInfo
	user, userExists := middleware.GetUserFromContext(r)
	if userExists {
		userInfo = &UserInfo{
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			IsGuest:   user.Provider == models.ProviderGuest,
		}
	}

	data := PageData{
		Message:          "",
		Title:            note.Title,
		Content:          note.Content,
		OriginalFilename: note.Filename,
		Notes:            notes,
		User:             userInfo,
	}

	h.renderTemplate(w, data)
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
		http.Error(w, "Error parsing form", http.StatusBadRequest)
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

	var err error

	// If editing an existing note
	if originalFilename != "" {
		_, err = h.service.UpdateNote(userID, originalFilename, req)
		if err == nil && h.metrics != nil {
			h.metrics.RecordNoteUpdated(ctx)
		}
	} else {
		_, err = h.service.CreateNote(userID, req)
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

		// Get notes for sidebar
		notes, _ := h.service.ListNotes(userID)

		data := PageData{
			Message: h.getErrorMessage(err),
			Title:   title,
			Content: content,
			Notes:   notes,
		}
		h.renderTemplate(w, data)
		return
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderTemplate renders the HTML template with the given data
func (h *NoteHandler) renderTemplate(w http.ResponseWriter, data PageData) {
	if err := h.tmpl.Execute(w, data); err != nil {
		h.logger.Error("Failed to render template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// renderError renders an error message
func (h *NoteHandler) renderError(w http.ResponseWriter, message string) {
	data := PageData{
		Message: message,
		Notes:   []string{},
	}
	h.renderTemplate(w, data)
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
