// Package health provides startup validation and readiness checks.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status represents the health status of a component.
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusFailed   Status = "failed"
)

// CheckResult holds the result of a single health check.
type CheckResult struct {
	Name        string `json:"name"`
	Status      Status `json:"status"`
	Critical    bool   `json:"critical"`
	Message     string `json:"message,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Duration    string `json:"duration,omitempty"`
}

// Check is a function that performs a health check.
type Check struct {
	Name        string
	Critical    bool
	Run         func(ctx context.Context) (Status, string, error)
	Remediation string
}

// Checker manages health checks and readiness state.
type Checker struct {
	checks  []Check
	results map[string]CheckResult
	mu      sync.RWMutex
	ready   bool
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{
		checks:  make([]Check, 0),
		results: make(map[string]CheckResult),
	}
}

// Register adds a check to the checker.
func (c *Checker) Register(check Check) {
	c.mu.Lock()
	c.checks = append(c.checks, check)
	c.mu.Unlock()
}

// RunPreflight runs all critical checks synchronously on startup.
// Returns error if any critical check fails.
func (c *Checker) RunPreflight(ctx context.Context) error {
	var criticalErrors []string

	for _, check := range c.checks {
		start := time.Now()
		status, msg, err := check.Run(ctx)
		duration := time.Since(start)

		result := CheckResult{
			Name:        check.Name,
			Status:      status,
			Critical:    check.Critical,
			Message:     msg,
			Remediation: check.Remediation,
			Duration:    duration.String(),
		}

		if err != nil {
			result.Status = StatusFailed
			result.Message = err.Error()
		}

		c.mu.Lock()
		c.results[check.Name] = result
		c.mu.Unlock()

		// Log the result
		if result.Status == StatusOK {
			fmt.Printf("[PREFLIGHT] ✓ %s: %s\n", check.Name, msg)
		} else if result.Status == StatusDegraded {
			fmt.Printf("[PREFLIGHT] ⚠ %s: %s\n", check.Name, msg)
		} else {
			fmt.Printf("[PREFLIGHT] ✗ %s: %s\n", check.Name, msg)
			if check.Remediation != "" {
				fmt.Printf("            Remediation: %s\n", check.Remediation)
			}
		}

		if check.Critical && result.Status == StatusFailed {
			criticalErrors = append(criticalErrors, fmt.Sprintf("%s: %s", check.Name, msg))
		}
	}

	if len(criticalErrors) > 0 {
		c.mu.Lock()
		c.ready = false
		c.mu.Unlock()
		return fmt.Errorf("critical preflight checks failed: %v", criticalErrors)
	}

	c.mu.Lock()
	c.ready = true
	c.mu.Unlock()
	return nil
}

// IsReady returns the current readiness state.
func (c *Checker) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// GetStatus returns the overall status and individual check results.
func (c *Checker) GetStatus() (Status, map[string]CheckResult) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Copy results
	results := make(map[string]CheckResult)
	for k, v := range c.results {
		results[k] = v
	}

	// Determine overall status
	overall := StatusOK
	for _, r := range results {
		if r.Status == StatusFailed && r.Critical {
			return StatusFailed, results
		}
		if r.Status == StatusDegraded || (r.Status == StatusFailed && !r.Critical) {
			overall = StatusDegraded
		}
	}

	return overall, results
}

// SetReady manually sets the ready state.
func (c *Checker) SetReady(ready bool) {
	c.mu.Lock()
	c.ready = ready
	c.mu.Unlock()
}

// UpdateCheck updates a check result (for periodic re-checks).
func (c *Checker) UpdateCheck(name string, status Status, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if result, ok := c.results[name]; ok {
		result.Status = status
		result.Message = message
		c.results[name] = result

		// Update ready state based on critical checks
		allCriticalOK := true
		for _, r := range c.results {
			if r.Critical && r.Status == StatusFailed {
				allCriticalOK = false
				break
			}
		}
		c.ready = allCriticalOK
	}
}

// Common checks

// CheckEnvVar creates a check for required environment variable.
func CheckEnvVar(name string, critical bool) Check {
	return Check{
		Name:     fmt.Sprintf("env_%s", name),
		Critical: critical,
		Run: func(ctx context.Context) (Status, string, error) {
			val := os.Getenv(name)
			if val == "" {
				if critical {
					return StatusFailed, fmt.Sprintf("required env var %s not set", name), nil
				}
				return StatusDegraded, fmt.Sprintf("optional env var %s not set", name), nil
			}
			return StatusOK, fmt.Sprintf("%s is set", name), nil
		},
		Remediation: fmt.Sprintf("Set the %s environment variable", name),
	}
}

// CheckDirectory creates a check for directory existence and write access.
func CheckDirectory(path string, needWrite bool, critical bool) Check {
	return Check{
		Name:     fmt.Sprintf("dir_%s", filepath.Base(path)),
		Critical: critical,
		Run: func(ctx context.Context) (Status, string, error) {
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				return StatusFailed, fmt.Sprintf("directory %s does not exist", path), nil
			}
			if err != nil {
				return StatusFailed, fmt.Sprintf("cannot access %s: %v", path, err), nil
			}
			if !info.IsDir() {
				return StatusFailed, fmt.Sprintf("%s is not a directory", path), nil
			}

			if needWrite {
				// Test write access
				testFile := filepath.Join(path, ".health_check_test")
				f, err := os.Create(testFile)
				if err != nil {
					return StatusFailed, fmt.Sprintf("cannot write to %s: %v", path, err), nil
				}
				_ = f.Close()
				_ = os.Remove(testFile)
			}

			return StatusOK, fmt.Sprintf("directory %s accessible", path), nil
		},
		Remediation: fmt.Sprintf("Ensure directory %s exists and is writable", path),
	}
}

// CheckHTTPEndpoint creates a check for HTTP endpoint availability.
func CheckHTTPEndpoint(name, url string, timeout time.Duration, critical bool) Check {
	return Check{
		Name:     fmt.Sprintf("http_%s", name),
		Critical: critical,
		Run: func(ctx context.Context) (Status, string, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return StatusFailed, fmt.Sprintf("invalid URL %s: %v", url, err), nil
			}

			client := &http.Client{Timeout: timeout}
			resp, err := client.Do(req)
			if err != nil {
				return StatusFailed, fmt.Sprintf("cannot reach %s: %v", url, err), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return StatusOK, fmt.Sprintf("%s reachable (status %d)", name, resp.StatusCode), nil
			}

			return StatusDegraded, fmt.Sprintf("%s returned status %d", name, resp.StatusCode), nil
		},
		Remediation: fmt.Sprintf("Ensure %s is running and accessible at %s", name, url),
	}
}

// HealthResponse is the JSON response for health endpoints.
type HealthResponse struct {
	Status  Status                 `json:"status"`
	Ready   bool                   `json:"ready"`
	Checks  map[string]CheckResult `json:"checks,omitempty"`
	Version string                 `json:"version,omitempty"`
}

// Handler returns an http.HandlerFunc for the health endpoint.
func (c *Checker) Handler(detailed bool, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, checks := c.GetStatus()

		resp := HealthResponse{
			Status:  status,
			Ready:   c.IsReady(),
			Version: version,
		}

		if detailed || r.URL.Query().Get("detailed") == "true" {
			resp.Checks = checks
		}

		w.Header().Set("Content-Type", "application/json")
		if status == StatusFailed {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if status == StatusDegraded {
			w.WriteHeader(http.StatusOK) // Still OK but degraded
		} else {
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ReadyHandler returns an http.HandlerFunc for the readiness endpoint.
func (c *Checker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if c.IsReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
		}
	}
}
