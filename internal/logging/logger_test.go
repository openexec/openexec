package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"golang.org/x/exp/slog"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != slog.LevelInfo {
		t.Errorf("expected level %v, got %v", slog.LevelInfo, cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("expected format 'text', got %q", cfg.Format)
	}
	if cfg.Output != os.Stdout {
		t.Error("expected output to be os.Stdout")
	}
	if cfg.AddSource {
		t.Error("expected AddSource to be false")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		format string
	}{
		{
			name: "text format",
			cfg: Config{
				Level:  slog.LevelInfo,
				Format: "text",
				Output: &bytes.Buffer{},
			},
			format: "text",
		},
		{
			name: "json format",
			cfg: Config{
				Level:  slog.LevelInfo,
				Format: "json",
				Output: &bytes.Buffer{},
			},
			format: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.cfg)
			if logger == nil {
				t.Fatal("expected logger to be non-nil")
			}
			if logger.Logger == nil {
				t.Fatal("expected inner slog.Logger to be non-nil")
			}
		})
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	loggerWithAttr := logger.With("key", "value")
	loggerWithAttr.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected output to contain 'key=value', got %q", output)
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	componentLogger := logger.WithComponent("telegram")
	componentLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "component=telegram") {
		t.Errorf("expected output to contain 'component=telegram', got %q", output)
	}
}

func TestLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	// Test with nil error
	nilErrLogger := logger.WithError(nil)
	if nilErrLogger != logger {
		t.Error("WithError(nil) should return the same logger")
	}

	// Test with actual error
	errLogger := logger.WithError(os.ErrNotExist)
	errLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "error=") {
		t.Errorf("expected output to contain 'error=', got %q", output)
	}
}

func TestLogger_Levels(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelDebug,
		Format: "text",
		Output: &buf,
	})

	tests := []struct {
		name     string
		logFunc  func(string, ...any)
		level    string
		expected string
	}{
		{"debug", logger.Debug, "DEBUG", "debug message"},
		{"info", logger.Info, "INFO", "info message"},
		{"warn", logger.Warn, "WARN", "warn message"},
		{"error", logger.Error, "ERROR", "error message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc(tt.expected)

			output := buf.String()
			if !strings.Contains(output, tt.level) {
				t.Errorf("expected output to contain level %q, got %q", tt.level, output)
			}
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected output to contain message %q, got %q", tt.expected, output)
			}
		})
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelWarn,
		Format: "text",
		Output: &buf,
	})

	// Debug and Info should not be logged
	logger.Debug("debug message")
	logger.Info("info message")
	if buf.Len() > 0 {
		t.Errorf("expected no output for debug/info at warn level, got %q", buf.String())
	}

	// Warn and Error should be logged
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Errorf("expected warn message to be logged, got %q", buf.String())
	}

	buf.Reset()
	logger.Error("error message")
	if !strings.Contains(buf.String(), "error message") {
		t.Errorf("expected error message to be logged, got %q", buf.String())
	}
}

func TestFromEnv(t *testing.T) {
	// Save and restore environment
	originalLevel := os.Getenv("LOG_LEVEL")
	originalFormat := os.Getenv("LOG_FORMAT")
	defer func() {
		os.Setenv("LOG_LEVEL", originalLevel)
		os.Setenv("LOG_FORMAT", originalFormat)
	}()

	tests := []struct {
		name           string
		levelEnv       string
		formatEnv      string
		expectedLevel  slog.Level
		expectedFormat string
	}{
		{
			name:           "default values",
			levelEnv:       "",
			formatEnv:      "",
			expectedLevel:  slog.LevelInfo,
			expectedFormat: "text",
		},
		{
			name:           "debug level",
			levelEnv:       "debug",
			formatEnv:      "",
			expectedLevel:  slog.LevelDebug,
			expectedFormat: "text",
		},
		{
			name:           "warn level",
			levelEnv:       "warn",
			formatEnv:      "",
			expectedLevel:  slog.LevelWarn,
			expectedFormat: "text",
		},
		{
			name:           "json format",
			levelEnv:       "",
			formatEnv:      "json",
			expectedLevel:  slog.LevelInfo,
			expectedFormat: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LOG_LEVEL", tt.levelEnv)
			os.Setenv("LOG_FORMAT", tt.formatEnv)

			logger := FromEnv()
			if logger == nil {
				t.Fatal("expected logger to be non-nil")
			}
		})
	}
}

func TestSetDefault(t *testing.T) {
	// Save original default logger
	original := GetDefault()
	defer SetDefault(original)

	var buf bytes.Buffer
	newLogger := New(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	SetDefault(newLogger)

	if GetDefault() != newLogger {
		t.Error("expected GetDefault to return the new logger")
	}

	// Test package-level functions use the new default
	Info("test message")
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("expected Info to use new default logger, got %q", buf.String())
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelDebug,
		Format: "text",
		Output: &buf,
	})

	// Save and restore
	original := GetDefault()
	defer SetDefault(original)
	SetDefault(logger)

	tests := []struct {
		name    string
		logFunc func(string, ...any)
		message string
	}{
		{"Debug", Debug, "debug pkg"},
		{"Info", Info, "info pkg"},
		{"Warn", Warn, "warn pkg"},
		{"Error", Error, "error pkg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc(tt.message)

			output := buf.String()
			if !strings.Contains(output, tt.message) {
				t.Errorf("expected output to contain %q, got %q", tt.message, output)
			}
		})
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	output := buf.String()
	// JSON output should contain these patterns
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("expected JSON output to contain msg field, got %q", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("expected JSON output to contain key field, got %q", output)
	}
}
