package config

import "time"

// Centralized execution defaults to prevent drift
const (
	DefaultMaxIterations = 10
	DefaultMaxRetries    = 3
	DefaultMaxReviewCycles = 3
	DefaultThrashThreshold = 3
	DefaultServerPort      = 8765
)

// DefaultRetryBackoff defines the standard backoff strategy for agents
var DefaultRetryBackoff = []time.Duration{
	0, 
	5 * time.Second, 
	15 * time.Second, 
	30 * time.Second,
}

// System-wide timeouts
const (
	DefaultTaskTimeout = 600 * time.Second
	DefaultRequestTimeout = 30 * time.Second
)
