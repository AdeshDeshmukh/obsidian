package queue_test

import (
	"context"
	"strings"
	"testing"

	"obsidian/internal/queue"

	"github.com/jackc/pgx/v5"
)

type mockRow struct {
	cycleExists bool
	err         error
}

func (m mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) > 0 {
		if ptr, ok := dest[0].(*bool); ok {
			*ptr = m.cycleExists
		}
	}
	return nil
}

type mockDB struct {
	row mockRow
}

func (m mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.row
}

func TestCheckCyclicDependency_SelfLoop(t *testing.T) {
	ctx := context.Background()
	db := mockDB{}

	err := queue.CheckCyclicDependency(ctx, db, "job-1", "job-1")
	if err == nil {
		t.Fatal("expected error for self-dependency, got nil")
	}
	if !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckCyclicDependency_CycleDetected(t *testing.T) {
	ctx := context.Background()
	db := mockDB{row: mockRow{cycleExists: true}}

	err := queue.CheckCyclicDependency(ctx, db, "job-1", "job-2")
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency detected") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckCyclicDependency_ValidDAG(t *testing.T) {
	ctx := context.Background()
	db := mockDB{row: mockRow{cycleExists: false}}

	err := queue.CheckCyclicDependency(ctx, db, "job-1", "job-2")
	if err != nil {
		t.Fatalf("expected valid dependency without cycle, got: %v", err)
	}
}
