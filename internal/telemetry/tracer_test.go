package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestTracerSpanCreation(t *testing.T) {
	ctx := context.Background()

	ctx, span := Tracer.Start(ctx, "TestSpan")
	if span == nil {
		t.Fatal("expected non-nil span from Tracer.Start")
	}

	span.SetAttributes(
		attribute.String("test.key", "value"),
		attribute.Int("test.count", 42),
	)
	span.End()
}

