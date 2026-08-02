package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"notes-app/internal/config"
	"notes-app/internal/server"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// If no notes directory is configured, use a temp directory
	if cfg.Notes.Directory == "" {
		tempDir, err := os.MkdirTemp("", "notes-app-*")
		if err != nil {
			slog.Error("Failed to create temp directory", "error", err)
			os.Exit(1)
		}
		cfg.Notes.Directory = tempDir
		slog.Info("Using temporary notes directory", "directory", tempDir)
	}

	// Setup structured logging
	setupLogging(cfg)

	slog.Info("Starting notes application", "version", "1.0.0")

	// Create server with all dependencies
	app, err := server.New(cfg, slog.Default())
	if err != nil {
		slog.Error("Failed to create server", "error", err)
		os.Exit(1)
	}

	// Clean up temp directory on shutdown
	defer func() {
		if filepath.Base(cfg.Notes.Directory)[:8] == "notes-app" {
			os.RemoveAll(cfg.Notes.Directory)
			slog.Info("Cleaned up temporary notes directory", "directory", cfg.Notes.Directory)
		}
	}()

	// Start server in a goroutine
	go func() {
		if err := app.Start(); err != nil {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Setup graceful shutdown
	GracefulShutdown(app, 10*time.Second)
}

// setupLogging configures structured logging based on config
func setupLogging(cfg *config.Config) {
	level := slog.LevelInfo
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// GracefulShutdown handles graceful server shutdown
func GracefulShutdown(app *server.Server, timeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	if err := app.Shutdown(timeout); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited properly")
}
