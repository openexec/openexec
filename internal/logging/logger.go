// Package logging provides structured logging using slog for better observability.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger to provide a consistent logging interface.
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration options.
type Config struct {
	// Level is the minimum log level to output.
	Level slog.Level
	// Format is the output format ("json" or "text").
	Format string
	// Output is the writer to output logs to.
	Output io.Writer
	// AddSource adds source code location to log entries.
	AddSource bool
}

// DefaultConfig returns the default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:     slog.LevelInfo,
		Format:    "text",
		Output:    os.Stdout,
		AddSource: false,
	}
}

// New creates a new Logger with the given configuration.
func New(cfg Config) *Logger {
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// Default returns the default logger.
func Default() *Logger {
	return New(DefaultConfig())
}

// FromEnv creates a logger configured from environment variables.
// LOG_LEVEL: debug, info, warn, error (default: info)
// LOG_FORMAT: json, text (default: text)
func FromEnv() *Logger {
	cfg := DefaultConfig()

	if level := os.Getenv("LOG_LEVEL"); level != "" {
		switch strings.ToLower(level) {
		case "debug":
			cfg.Level = slog.LevelDebug
		case "info":
			cfg.Level = slog.LevelInfo
		case "warn", "warning":
			cfg.Level = slog.LevelWarn
		case "error":
			cfg.Level = slog.LevelError
		}
	}

	if format := os.Getenv("LOG_FORMAT"); format != "" {
		cfg.Format = format
	}

	return New(cfg)
}

// With returns a new Logger with the given attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// WithGroup returns a new Logger with the given group name.
func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		Logger: l.Logger.WithGroup(name),
	}
}

// WithComponent returns a new Logger with a component attribute.
func (l *Logger) WithComponent(name string) *Logger {
	return l.With("component", name)
}

// WithError returns a new Logger with an error attribute.
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return l.With("error", err.Error())
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

// DebugContext logs a debug message with context.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Logger.DebugContext(ctx, msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

// InfoContext logs an info message with context.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Logger.InfoContext(ctx, msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

// WarnContext logs a warning message with context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Logger.WarnContext(ctx, msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

// ErrorContext logs an error message with context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Logger.ErrorContext(ctx, msg, args...)
}

// global logger instance
var defaultLogger = Default()

// SetDefault sets the default logger.
func SetDefault(l *Logger) {
	defaultLogger = l
	slog.SetDefault(l.Logger)
}

// GetDefault returns the default logger.
func GetDefault() *Logger {
	return defaultLogger
}

// Package-level convenience functions that use the default logger.

// Debug logs a debug message using the default logger.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Info logs an info message using the default logger.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs an error message using the default logger.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}
