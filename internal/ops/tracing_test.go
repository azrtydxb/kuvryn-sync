package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSetupTracingStaysOffWithoutAnEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	before := otel.GetTracerProvider()
	shutdown, err := SetupTracing(context.Background(), logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("a tracer provider was installed with no OTLP endpoint configured")
	}
}

func TestSetupTracingExportsWhenAnEndpointIsSet(t *testing.T) {
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
	t.Setenv("OTEL_SDK_DISABLED", "")
	shutdown, err := SetupTracing(context.Background(), logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatalf("tracer provider is %T, want the OTLP-exporting SDK provider", otel.GetTracerProvider())
	}
}

func TestSpanErrorsAreRedacted(t *testing.T) {
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
	recorder := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	_, finish := NewOTelTracer("test").Start(context.Background(), "reconcile")
	finish(errors.New("fetch failed: password=hunter2"))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	if strings.Contains(spans[0].Status().Description, "hunter2") {
		t.Fatalf("span status leaks the secret: %q", spans[0].Status().Description)
	}
	for _, event := range spans[0].Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.String(), "hunter2") {
				t.Fatalf("span event leaks the secret: %s", attr.Value.String())
			}
		}
	}
}

func TestSetupTracingNamesTheServiceSolder(t *testing.T) {
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	shutdown, err := SetupTracing(context.Background(), logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	recorder := tracetest.NewSpanRecorder()
	otel.GetTracerProvider().(*sdktrace.TracerProvider).RegisterSpanProcessor(recorder)
	_, finish := NewOTelTracer("test").Start(context.Background(), "reconcile")
	finish(nil)
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	for _, attr := range spans[0].Resource().Attributes() {
		if attr.Key == "service.name" {
			if attr.Value.AsString() != "solder" {
				t.Fatalf("service.name = %q, want solder", attr.Value.AsString())
			}
			return
		}
	}
	t.Fatal("span resource has no service.name")
}
