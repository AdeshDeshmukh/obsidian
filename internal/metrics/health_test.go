package metrics

import (
	"testing"
)

func TestComputeScore(t *testing.T) {
	tests := []struct {
		name          string
		dlqRate       float64
		failRate      float64
		queueDepth    int
		expectedScore int
		expectedGrade string
	}{
		{
			name:          "perfect health",
			dlqRate:       0.0,
			failRate:      0.0,
			queueDepth:    0,
			expectedScore: 100,
			expectedGrade: "A",
		},
		{
			name:          "minor backlog, no failures",
			dlqRate:       0.0,
			failRate:      0.0,
			queueDepth:    100, // penalty: 100/50 = 2
			expectedScore: 98,
			expectedGrade: "A",
		},
		{
			name:          "moderate degradation",
			dlqRate:       0.04, // penalty: 0.04 * 500 = 20
			failRate:      0.06, // penalty: 0.06 * 150 = 9
			queueDepth:    250,  // penalty: 250 / 50 = 5
			expectedScore: 66,   // 100 - 20 - 9 - 5 = 66
			expectedGrade: "C",
		},
		{
			name:          "grade B boundary",
			dlqRate:       0.03, // 15 penalty
			failRate:      0.04, // 6 penalty
			queueDepth:    50,   // 1 penalty -> 100 - 22 = 78
			expectedScore: 78,
			expectedGrade: "B",
		},
		{
			name:          "grade D boundary",
			dlqRate:       0.08, // 40 penalty
			failRate:      0.08, // 12 penalty
			queueDepth:    200,  // 4 penalty -> 100 - 56 = 44
			expectedScore: 44,
			expectedGrade: "D",
		},
		{
			name:          "maximum penalties reaching zero",
			dlqRate:       0.20, // max 50
			failRate:      0.40, // max 30
			queueDepth:    1500, // max 20 -> 100 - 50 - 30 - 20 = 0
			expectedScore: 0,
			expectedGrade: "F",
		},
		{
			name:          "extreme failure rates cannot go below zero",
			dlqRate:       1.0,
			failRate:      1.0,
			queueDepth:    100000,
			expectedScore: 0,
			expectedGrade: "F",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, grade := ComputeScore(tt.dlqRate, tt.failRate, tt.queueDepth)
			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}
			if grade != tt.expectedGrade {
				t.Errorf("expected grade %s, got %s", tt.expectedGrade, grade)
			}
		})
	}
}
