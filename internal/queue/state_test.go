package queue

import "testing"

func TestQueueStateValidity(t *testing.T) {
	tests := []struct {
		state    string
		valid    bool
		canClaim bool
	}{
		{string(QueueStateActive), true, true},
		{string(QueueStatePaused), true, false},
		{string(QueueStateDraining), true, false},
		{string(QueueStateMaintenance), true, false},
		{"unknown", false, false},
		{"invalid_state", false, false},
		{"", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := IsValidQueueState(tt.state); got != tt.valid {
				t.Errorf("IsValidQueueState(%q) = %v; want %v", tt.state, got, tt.valid)
			}
			if tt.valid {
				qs := QueueState(tt.state)
				if got := qs.CanClaimJobs(); got != tt.canClaim {
					t.Errorf("QueueState(%q).CanClaimJobs() = %v; want %v", tt.state, got, tt.canClaim)
				}
			}
		})
	}
}
