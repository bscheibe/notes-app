package monitoring

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of the application
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Checks    map[string]Check  `json:"checks,omitempty"`
}

// Check represents a single health check
type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthChecker manages health checks
type HealthChecker struct {
	checks map[string]func() Check
	mu     sync.RWMutex
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make(map[string]func() Check),
	}
}

// AddCheck adds a health check function
func (h *HealthChecker) AddCheck(name string, checkFunc func() Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = checkFunc
}

// CheckHealth runs all health checks and returns the overall status
func (h *HealthChecker) CheckHealth() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Checks:    make(map[string]Check),
	}

	allHealthy := true
	for name, checkFunc := range h.checks {
		check := checkFunc()
		status.Checks[name] = check
		
		if check.Status != "healthy" {
			allHealthy = false
		}
	}

	if !allHealthy {
		status.Status = "unhealthy"
	}

	return status
}

// HandleHealth returns an HTTP handler for health checks
func (h *HealthChecker) HandleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := h.CheckHealth()
		
		w.Header().Set("Content-Type", "application/json")
		
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		_ = json.NewEncoder(w).Encode(status)
	}
}

// HandleReadiness returns an HTTP handler for readiness checks
func (h *HealthChecker) HandleReadiness() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := h.CheckHealth()
		
		w.Header().Set("Content-Type", "application/json")
		
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Not Ready"))
		}
	}
}

// HandleLiveness returns an HTTP handler for liveness checks
func (h *HealthChecker) HandleLiveness() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}
}