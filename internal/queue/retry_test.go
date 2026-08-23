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
