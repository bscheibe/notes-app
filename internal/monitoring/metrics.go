package monitoring

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelPrometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Metrics holds all application metrics
type Metrics struct {
	// HTTP metrics
	HttpRequestsTotal      *prometheus.CounterVec
	HttpRequestDuration    *prometheus.HistogramVec
	HttpRequestsInProgress *prometheus.GaugeVec

	// Application metrics
	NotesTotal       prometheus.Counter
	NotesCreated     prometheus.Counter
	NotesUpdated     prometheus.Counter
	NotesDeleted     prometheus.Counter
	NotesReadErrors  prometheus.Counter
	NotesWriteErrors prometheus.Counter

	// OTEL metrics
	noteCreateCounter metric.Int64Counter
	noteUpdateCounter metric.Int64Counter
	noteDeleteCounter metric.Int64Counter
	noteReadDuration  metric.Float64Histogram
	noteWriteDuration metric.Float64Histogram
}

// NewMetrics initializes all Prometheus and OTEL metrics
func NewMetrics(logger *slog.Logger) (*Metrics, error) {
	m := &Metrics{
		// HTTP metrics
		HttpRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HttpRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		HttpRequestsInProgress: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "http_requests_in_progress",
				Help: "Number of HTTP requests currently in progress",
			},
			[]string{"method", "path"},
		),

		// Application metrics
		NotesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_total",
				Help: "Total number of notes",
			},
		),
		NotesCreated: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_created_total",
				Help: "Total number of notes created",
			},
		),
		NotesUpdated: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_updated_total",
				Help: "Total number of notes updated",
			},
		),
		NotesDeleted: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_deleted_total",
				Help: "Total number of notes deleted",
			},
		),
		NotesReadErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_read_errors_total",
				Help: "Total number of note read errors",
			},
		),
		NotesWriteErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "notes_write_errors_total",
				Help: "Total number of note write errors",
			},
		),
	}

	// Setup OTEL metrics
	if err := m.setupOTELMetrics(logger); err != nil {
		return nil, err
	}

	logger.Info("Metrics initialized")
	return m, nil
}

// setupOTELMetrics initializes OpenTelemetry metrics
func (m *Metrics) setupOTELMetrics(logger *slog.Logger) error {
	meter := otel.Meter("notes-app")

	var err error
	m.noteCreateCounter, err = meter.Int64Counter("notes.created",
		metric.WithDescription("Number of notes created"))
	if err != nil {
		return err
	}

	m.noteUpdateCounter, err = meter.Int64Counter("notes.updated",
		metric.WithDescription("Number of notes updated"))
	if err != nil {
		return err
	}

	m.noteDeleteCounter, err = meter.Int64Counter("notes.deleted",
		metric.WithDescription("Number of notes deleted"))
	if err != nil {
		return err
	}

	m.noteReadDuration, err = meter.Float64Histogram("notes.read.duration",
		metric.WithDescription("Duration of note read operations"))
	if err != nil {
		return err
	}

	m.noteWriteDuration, err = meter.Float64Histogram("notes.write.duration",
		metric.WithDescription("Duration of note write operations"))
	if err != nil {
		return err
	}

	return nil
}

// RecordNoteCreated records a note creation metric
func (m *Metrics) RecordNoteCreated(ctx context.Context) {
	m.NotesCreated.Inc()
	m.noteCreateCounter.Add(ctx, 1)
}

// RecordNoteUpdated records a note update metric
func (m *Metrics) RecordNoteUpdated(ctx context.Context) {
	m.NotesUpdated.Inc()
	m.noteUpdateCounter.Add(ctx, 1)
}

// RecordNoteDeleted records a note deletion metric
func (m *Metrics) RecordNoteDeleted(ctx context.Context) {
	m.NotesDeleted.Inc()
	m.noteDeleteCounter.Add(ctx, 1)
}

// RecordNoteReadError records a note read error metric
func (m *Metrics) RecordNoteReadError() {
	m.NotesReadErrors.Inc()
}

// RecordNoteWriteError records a note write error metric
func (m *Metrics) RecordNoteWriteError() {
	m.NotesWriteErrors.Inc()
}

// RecordNoteReadDuration records the duration of a note read operation
func (m *Metrics) RecordNoteReadDuration(ctx context.Context, duration float64) {
	m.noteReadDuration.Record(ctx, duration, metric.WithAttributes(attribute.String("operation", "read")))
}

// RecordNoteWriteDuration records the duration of a note write operation
func (m *Metrics) RecordNoteWriteDuration(ctx context.Context, duration float64) {
	m.noteWriteDuration.Record(ctx, duration, metric.WithAttributes(attribute.String("operation", "write")))
}

// SetupPrometheusExporter sets up Prometheus exporter for OTEL metrics
func SetupPrometheusExporter(logger *slog.Logger) (http.Handler, error) {
	exporter, err := otelPrometheus.New()
	if err != nil {
		logger.Error("Failed to create Prometheus exporter", "error", err)
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	logger.Info("Prometheus exporter configured")
	return promhttp.Handler(), nil
}
