package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracer is the default package-level tracer, initialized to a safe no-op tracer.
var Tracer trace.Tracer = otel.GetTracerProvider().Tracer("obsidian")

// InitTracer sets up OpenTelemetry with OTLP HTTP exporter (Jaeger compatible).
// Call this once at the start of main.
// OTEL_EXPORTER_OTLP_ENDPOINT env var controls the exporter URL (default: http://localhost:4318).
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	Tracer = tp.Tracer(serviceName)

	slog.Info("OpenTelemetry tracer initialized", "service", serviceName)

	return tp.Shutdown, nil
}

