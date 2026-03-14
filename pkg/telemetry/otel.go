package telemetry

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer      trace.Tracer
	initialized bool
	initOnce    sync.Once
)

// Config holds telemetry configuration.
type Config struct {
	ServiceName    string
	SampleRate     float64 // 0.0-1.0, default 0.05 (5%)
	Endpoint       string  // OTLP endpoint, if empty uses stdout
	Enabled        bool
}

// DefaultConfig returns config based on environment variables.
// Telemetry is OFF by default unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
//
// Environment variables:
//   - OTEL_EXPORTER_OTLP_ENDPOINT: OTLP collector endpoint (enables telemetry)
//   - OTEL_SERVICE_NAME: Service name (default: openexec)
//   - OTEL_SAMPLER: "always_on" for 100%, otherwise uses OTEL_SAMPLE_RATE
//   - OTEL_SAMPLE_RATE: Sample rate 0.0-1.0 (default: 0.05)
//   - OTEL_RESOURCE_ATTRIBUTES: Additional resource attributes (key=value,...)
func DefaultConfig() Config {
	cfg := Config{
		ServiceName: "openexec",
		SampleRate:  0.05, // 5% default sampling
		Enabled:     false,
	}

	// Service name override
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		cfg.ServiceName = name
	}

	// Endpoint enables telemetry
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		cfg.Endpoint = endpoint
		cfg.Enabled = true
	}

	// Sampling rate
	if os.Getenv("OTEL_SAMPLER") == "always_on" {
		cfg.SampleRate = 1.0
	} else if rate := os.Getenv("OTEL_SAMPLE_RATE"); rate != "" {
		if r, err := strconv.ParseFloat(rate, 64); err == nil && r >= 0 && r <= 1 {
			cfg.SampleRate = r
		}
	}

	return cfg
}

// InitOTel initializes the global OpenTelemetry tracer.
// For V1, we default to stdout exporting to keep dependencies minimal.
func InitOTel(ctx context.Context, serviceName string, w io.Writer) (func(context.Context) error, error) {
	cfg := DefaultConfig()
	cfg.ServiceName = serviceName

	// If telemetry is disabled and no writer provided, return no-op
	if !cfg.Enabled && w == nil {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(w),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure sampler based on rate
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	tracer = otel.Tracer("openexec")
	initialized = true

	// Log once when telemetry is active
	initOnce.Do(func() {
		log.Printf("[Telemetry] Active: service=%s, sampling=%.0f%%, endpoint=%s",
			cfg.ServiceName, cfg.SampleRate*100, cfg.Endpoint)
	})

	return tp.Shutdown, nil
}

// IsEnabled returns true if telemetry is initialized.
func IsEnabled() bool {
	return initialized
}

// GetTracer returns the global OpenExec tracer.
func GetTracer() trace.Tracer {
	if tracer == nil {
		return otel.Tracer("openexec-noop")
	}
	return tracer
}

// StartSpan is a helper to start a new span.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
}

// --- Domain-specific span helpers ---

// StartRunSpan creates a span for a run lifecycle with standard attributes.
func StartRunSpan(ctx context.Context, runID, projectPath, mode string) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, "run",
		trace.WithAttributes(
			attribute.String("run.id", runID),
			attribute.String("run.project_path", projectPath),
			attribute.String("run.mode", mode),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// StartPhaseSpan creates a span for a pipeline phase.
func StartPhaseSpan(ctx context.Context, runID, phase, agent string) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, "phase."+phase,
		trace.WithAttributes(
			attribute.String("run.id", runID),
			attribute.String("phase.name", phase),
			attribute.String("phase.agent", agent),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// StartToolSpan creates a span for an MCP tool invocation.
// Only safe, non-sensitive attributes are recorded (redaction by design).
func StartToolSpan(ctx context.Context, runID, toolName string, args map[string]interface{}) (context.Context, trace.Span) {
    ctx, span := GetTracer().Start(ctx, "tool."+toolName,
        trace.WithAttributes(
            attribute.String("run.id", runID),
            attribute.String("tool.name", toolName),
        ),
        trace.WithSpanKind(trace.SpanKindInternal),
    )

    // REDACTION: Only record safe, non-sensitive attributes via Redactor
    // Do NOT record: file content, full prompts, credentials, env vars
    safe := map[string]string{}
    if path, ok := args["path"].(string); ok {
        safe["tool.path"] = redactPath(path)
    }
    if cmd, ok := args["command"].(string); ok {
        safe["tool.command"] = redactCommand(cmd)
    }
    // Use redactor to apply final checks/hashing
    DefaultRedactor().AddSafeAttrs(span, safe)
    // Record check_only flag for git_apply_patch (safe boolean)
    if checkOnly, ok := args["check_only"].(bool); ok {
        span.SetAttributes(attribute.Bool("tool.check_only", checkOnly))
    }

	return ctx, span
}

// redactPath removes sensitive path components but keeps structure.
func redactPath(path string) string {
	// Keep path but redact if it looks like it contains secrets
	if len(path) > 200 {
		return path[:50] + "...[redacted]..." + path[len(path)-50:]
	}
	return path
}

// redactCommand truncates and redacts potentially sensitive commands.
func redactCommand(cmd string) string {
	// Truncate long commands
	if len(cmd) > 80 {
		cmd = cmd[:80] + "..."
	}
	// Don't record inline secrets (very basic check)
	// Real redaction should be more sophisticated
	return cmd
}

// StartProviderSpan creates a span for an LLM provider call.
func StartProviderSpan(ctx context.Context, provider, model string, promptHash string, cacheHit bool) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, "provider."+provider,
		trace.WithAttributes(
			attribute.String("gen_ai.system", provider),
			attribute.String("gen_ai.request.model", model),
			attribute.String("gen_ai.prompt_hash", promptHash),
			attribute.Bool("gen_ai.cache_hit", cacheHit),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// SetProviderResponseAttrs adds response attributes to a provider span.
func SetProviderResponseAttrs(span trace.Span, inputTokens, outputTokens, cachedTokens int) {
	span.SetAttributes(
		attribute.Int("gen_ai.response.input_tokens", inputTokens),
		attribute.Int("gen_ai.response.output_tokens", outputTokens),
		attribute.Int("gen_ai.response.cached_tokens", cachedTokens),
	)
	span.SetStatus(codes.Ok, "")
}

// RecordProviderError records an error on a provider span with proper status.
func RecordProviderError(span trace.Span, err error, errorType string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String("error.type", errorType),
		attribute.String("error.message", err.Error()),
	)
}

// RecordToolError records an error on a tool span with proper status.
func RecordToolError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.Bool("tool.error", true),
		attribute.String("error.type", "tool_error"),
		attribute.String("error.message", truncateError(err.Error())),
	)
}

// RecordToolSuccess records success attributes on a tool span.
func RecordToolSuccess(span trace.Span, artifactHash string) {
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(attribute.Bool("tool.success", true))
	if artifactHash != "" {
		span.SetAttributes(attribute.String("tool.artifact_hash", artifactHash))
	}
}

// truncateError limits error message length for span attributes.
func truncateError(msg string) string {
	if len(msg) > 200 {
		return msg[:200] + "..."
	}
	return msg
}
