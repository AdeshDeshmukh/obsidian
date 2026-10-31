package worker

import (
	"context"
	"testing"
)

func TestProgressReporterContext(t *testing.T) {
	ctx := context.Background()

	fallback := GetProgressReporter(ctx)
	if fallback == nil {
		t.Fatal("expected non-nil fallback progress reporter")
	}
	fallback(50)

	var recordedProgress int
	customReporter := func(pct int) {
		recordedProgress = pct
	}

	ctxWithReporter := WithProgressReporter(ctx, customReporter)
	retrieved := GetProgressReporter(ctxWithReporter)
	if retrieved == nil {
		t.Fatal("expected non-nil retrieved progress reporter")
	}

	retrieved(75)
	if recordedProgress != 75 {
		t.Fatalf("expected recorded progress 75, got %d", recordedProgress)
	}
}

