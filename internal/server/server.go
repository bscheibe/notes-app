package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"notes-app/internal/auth"
	"notes-app/internal/config"
	"notes-app/internal/handlers"
	authMiddleware "notes-app/internal/middleware"
	"notes-app/internal/monitoring"
	"notes-app/internal/repository"
	"notes-app/internal/service"
)

// Server represents the HTTP server
type Server struct {
	config         *config.Config
	router         *chi.Mux
	server         *http.Server
	logger         *slog.Logger
	handler        *handlers.NoteHandler
	authHandler    *handlers.AuthHandler
	authMiddleware *authMiddleware.AuthMiddleware
	service        *service.NoteService
	repo           *repository.NoteRepository
	userRepo       auth.UserRepository
	authService    *auth.AuthService
	metrics        *monitoring.Metrics
	tracer         *monitoring.Tracer
	health         *monitoring.HealthChecker
}

// New creates a new server instance with all dependencies
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	// Initialize repository
	repo, err := repository.NewNoteRepository(cfg.Notes.Directory, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Initialize authentication components
	authDir := filepath.Join(cfg.Notes.Directory, "auth")
	
	userRepo, err := auth.NewFileSystemUserRepository(filepath.Join(authDir, "users"))
	if err != nil {
		return nil, fmt.Errorf("failed to create user repository: %w", err)
	}

	authService := auth.NewAuthService(userRepo, cfg)
	
	// Register OAuth clients if credentials are provided
	oauthClients := auth.NewOAuthClients(cfg)
	for provider, client := range oauthClients {
		authService.RegisterOAuthClient(provider, client)
	}

	authHandler := handlers.NewAuthHandler(authService, cfg)
	authMiddleware := authMiddleware.NewAuthMiddleware(authHandler.GetStore(), authHandler.GetSessionName(), authService)

	// Initialize monitoring
	var metrics *monitoring.Metrics
	var tracer *monitoring.Tracer
	var health *monitoring.HealthChecker

	if cfg.Monitoring.Enabled {
		metrics, err = monitoring.NewMetrics(logger)
		if err != nil {
			logger.Warn("Failed to initialize metrics", "error", err)
		}

		if cfg.Monitoring.TracingEnabled {
			tracer, err = monitoring.NewTracer(cfg.Monitoring.ServiceName, logger)
			if err != nil {
				logger.Warn("Failed to initialize tracer", "error", err)
			}
		}

		health = monitoring.NewHealthChecker()
		health.AddCheck("repository", func() monitoring.Check {
			if err := repo.Ping(); err != nil {
				return monitoring.Check{Status: "unhealthy", Message: err.Error()}
			}
			return monitoring.Check{Status: "healthy"}
		})
	}

	// Initialize service
	noteService := service.NewNoteService(repo, logger)

	// Initialize handler
	noteHandler, err := handlers.NewNoteHandler(noteService, logger, metrics, tracer)
	if err != nil {
		return nil, fmt.Errorf("failed to create handler: %w", err)
	}

	// Setup router
	r := chi.NewRouter()
	// One trusted hop: GCP's load balancer / Cloud Run front door appends the
	// real client IP as the last entry in X-Forwarded-For.
	r.Use(chiMiddleware.ClientIPFromXFFTrustedProxies(1))
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	// Setup routes
	setupRoutes(r, noteHandler, authHandler, authMiddleware, metrics, health, logger)

	return &Server{
		config:         cfg,
		router:         r,
		logger:         logger,
		handler:        noteHandler,
		authHandler:    authHandler,
		authMiddleware: authMiddleware,
		service:        noteService,
		repo:           repo,
		userRepo:       userRepo,
		authService:    authService,
		metrics:        metrics,
		tracer:         tracer,
		health:         health,
	}, nil
}

// setupRoutes configures all HTTP routes
func setupRoutes(r *chi.Mux, handler *handlers.NoteHandler, authHandler *handlers.AuthHandler, authMiddleware *authMiddleware.AuthMiddleware, metrics *monitoring.Metrics, health *monitoring.HealthChecker, logger *slog.Logger) {
	// Serve static files
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Monitoring endpoints
	if metrics != nil {
		promHandler, err := monitoring.SetupPrometheusExporter(logger)
		if err == nil {
			r.Handle("/metrics", promHandler)
		}
	}

	if health != nil {
		r.Get("/health", health.HandleHealth())
		r.Get("/healthz", health.HandleReadiness())
		r.Get("/livez", health.HandleLiveness())
	}

	// Authentication routes
	authHandler.RegisterRoutes(r)

	// Application routes with optional auth
	r.With(authMiddleware.OptionalAuth).Get("/", handler.HandleHome)
	
	// Protected routes requiring authentication
	r.With(authMiddleware.OptionalAuth).Route("/notes", func(r chi.Router) {
		r.Post("/", handler.HandleSaveNote)
		r.Get("/{filename}", handler.HandleViewNote)
	})
}

// Start begins the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)
	
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Server starting", "address", addr, "notes_dir", s.config.Notes.Directory)
	
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(timeout time.Duration) error {
	s.logger.Info("Server shutting down")

	// Shutdown tracer if enabled
	if s.tracer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := s.tracer.Shutdown(ctx); err != nil {
			s.logger.Error("Failed to shutdown tracer", "error", err)
		}
	}
	
	if s.server != nil {
		return s.server.Close()
	}
	
	return nil
}