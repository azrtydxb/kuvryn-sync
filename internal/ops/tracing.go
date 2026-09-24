package ops

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/azrtydxb/solder/internal/redact"
)

// OTelTracer implements Tracer using the global OpenTelemetry provider.
type OTelTracer struct {
	tracer trace.Tracer
}

// NewOTelTracer creates a tracer on the global OpenTelemetry provider, which
// SetupTracing configures.
func NewOTelTracer(name string) OTelTracer {
	return OTelTracer{tracer: otel.Tracer(name)}
}

func (t OTelTracer) Start(ctx context.Context, name string) (context.Context, func(error)) {
	ctx, span := t.tracer.Start(ctx, name)
	return ctx, func(err error) {
		if err != nil {
			// Spans leave the cluster, so they get the same redaction as
			// status messages.
			message := redact.String(err.Error())
			span.RecordError(errors.New(message))
			span.SetStatus(codes.Error, message)
		}
		span.End()
	}
}

// SetupTracing installs an OpenTelemetry tracer provider that exports spans
// over OTLP gRPC when an OTLP endpoint is configured through the standard
// environment variables (OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, plus the gRPC exporter's OTEL_* options). Without
// one, or with OTEL_SDK_DISABLED=true, tracing stays a no-op. The returned
// function flushes and stops the exporter.
func SetupTracing(ctx context.Context, log logr.Logger) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return noop, nil
	}
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return noop, err
	}
	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES, read last, override
	// the default name.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", "solder")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return noop, err
	}
	// Export failures go to the manager's log, not OTel's own stderr logger.
	otel.SetLogger(log)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Error(err, "Failed to export traces")
	}))
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}
