package queue

import (
	"testing"
	"time"
)

func TestComputeBackoff(t *testing.T) {
	tests := []struct {
		name       string
		policy     RetryPolicy
		attempt    int
		expected   time.Duration
	}{
		{
			name: "Fixed strategy first attempt",
			policy: RetryPolicy{
				Strategy:    "fixed",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 3,
			},
			attempt:  0,
			expected: 1000 * time.Millisecond,
		},
		{
			name: "Fixed strategy second attempt",
			policy: RetryPolicy{
				Strategy:    "fixed",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 3,
			},
			attempt:  1,
			expected: 1000 * time.Millisecond,
		},
		{
			name: "Linear strategy first attempt",
			policy: RetryPolicy{
				Strategy:    "linear",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 5,
			},
			attempt:  0,
			expected: 1000 * time.Millisecond,
		},
		{
			name: "Linear strategy third attempt",
			policy: RetryPolicy{
				Strategy:    "linear",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 5,
			},
			attempt:  2,
			expected: 3000 * time.Millisecond,
		},
		{
			name: "Exponential strategy first attempt",
			policy: RetryPolicy{
				Strategy:    "exponential",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 5,
			},
			attempt:  0,
			expected: 1000 * time.Millisecond,
		},
		{
			name: "Exponential strategy third attempt",
			policy: RetryPolicy{
				Strategy:    "exponential",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 5,
			},
			attempt:  2,
			expected: 4000 * time.Millisecond, // 1000 * 2^2
		},
		{
			name: "Exponential strategy fourth attempt",
			policy: RetryPolicy{
				Strategy:    "exponential",
				BaseDelayMs: 1000,
				MaxDelayMs:  30000,
				MaxAttempts: 5,
			},
			attempt:  3,
			expected: 8000 * time.Millisecond, // 1000 * 2^3
		},
		{
			name: "Exponential max delay capping",
			policy: RetryPolicy{
				Strategy:    "exponential",
				BaseDelayMs: 1000,
				MaxDelayMs:  5000,
				MaxAttempts: 5,
			},
			attempt:  3, // 1000 * 2^3 = 8000
			expected: 5000 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeBackoff(tt.policy, tt.attempt)
			if got != tt.expected {
				t.Errorf("ComputeBackoff() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestComputeBackoff_ExponentialJitter(t *testing.T) {
	policy := RetryPolicy{
		Strategy:    "exponential_jitter",
		BaseDelayMs: 1000,
		MaxDelayMs:  10000,
		MaxAttempts: 5,
	}

	attempt := 2 // base * 2^2 = 4000ms. Range with ±30%: [2800ms, 5200ms]
	minExpected := 2800 * time.Millisecond
	maxExpected := 5200 * time.Millisecond

	seenVariance := false
	var firstVal time.Duration

	for i := 0; i < 50; i++ {
		got := ComputeBackoff(policy, attempt)
		if got < minExpected || got > maxExpected {
			t.Fatalf("expected delay between %v and %v, got %v", minExpected, maxExpected, got)
		}
		if i == 0 {
			firstVal = got
		} else if got != firstVal {
			seenVariance = true
		}
	}

	if !seenVariance {
		t.Fatal("expected jitter to produce varying backoff values across calls")
	}

	cappingPolicy := RetryPolicy{
		Strategy:    "exponential_jitter",
		BaseDelayMs: 1000,
		MaxDelayMs:  2000,
		MaxAttempts: 5,
	}
	capped := ComputeBackoff(cappingPolicy, 4)
	if capped > 2000*time.Millisecond {
		t.Fatalf("expected backoff capped at 2000ms, got %v", capped)
	}
}
