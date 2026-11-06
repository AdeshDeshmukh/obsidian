package worker

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRunningJobCancellationRegistry(t *testing.T) {
	jobID := "test-job-cancel-123"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	RegisterRunningJob(jobID, cancel)

	// Cancellation should succeed
	cancelled := CancelRunningJob(jobID)
	if !cancelled {
		t.Fatalf("expected CancelRunningJob to return true for registered job")
	}

	// Context should be marked canceled
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected context to be cancelled")
	}

	UnregisterRunningJob(jobID)

	// Second cancellation attempt should return false
	if CancelRunningJob(jobID) {
		t.Fatalf("expected CancelRunningJob to return false after unregistering")
	}
}

func TestConcurrentJobCancellation(t *testing.T) {
	const jobCount = 20
	var wg sync.WaitGroup

	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobID := "concurrent-job"

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			RegisterRunningJob(jobID, cancel)
			_ = CancelRunningJob(jobID)
			UnregisterRunningJob(jobID)

			_ = ctx.Err()
		}(i)
	}

	wg.Wait()
}
