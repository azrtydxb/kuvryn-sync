package ops

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTelTracer implements Tracer using the global OpenTelemetry provider.
type OTelTracer struct {
	tracer trace.Tracer
}

// NewOTelTracer creates an optional OpenTelemetry tracer. Exporters are configured by the embedding process.
func NewOTelTracer(name string) OTelTracer {
	if name == "" {
		name = "github.com/azrtydxb/solder"
	}
	return OTelTracer{tracer: otel.Tracer(name)}
}

func (t OTelTracer) Start(ctx context.Context, name string) (context.Context, func(error)) {
	ctx, span := t.tracer.Start(ctx, name)
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}
