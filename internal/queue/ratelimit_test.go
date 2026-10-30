package queue_test

import (
	"context"
	"testing"
	"time"

	"obsidian/internal/queue"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockRateLimitRow struct {
	tokens       float64
	maxTokens    float64
	refillRate   float64
	lastRefillAt time.Time
	err          error
}

func (m mockRateLimitRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	*(dest[0].(*float64)) = m.tokens
	*(dest[1].(*float64)) = m.maxTokens
	*(dest[2].(*float64)) = m.refillRate
	*(dest[3].(*time.Time)) = m.lastRefillAt
	return nil
}

type mockRateLimitDB struct {
	row       mockRateLimitRow
	execCalls []string
	execArgs  [][]any
}

func (m *mockRateLimitDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.row
}

func (m *mockRateLimitDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, sql)
	m.execArgs = append(m.execArgs, args)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func TestConsumeToken_UnconstrainedQueue(t *testing.T) {
	ctx := context.Background()
	db := &mockRateLimitDB{row: mockRateLimitRow{err: pgx.ErrNoRows}}

	allowed, err := queue.ConsumeToken(ctx, db, "queue-unconstrained")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected unconfigured queue to allow token consumption")
	}
}

func TestConsumeToken_SufficientTokens(t *testing.T) {
	ctx := context.Background()
	db := &mockRateLimitDB{
		row: mockRateLimitRow{
			tokens:       5.0,
			maxTokens:    10.0,
			refillRate:   10.0,
			lastRefillAt: time.Now(),
		},
	}

	allowed, err := queue.ConsumeToken(ctx, db, "queue-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected claim to be allowed with sufficient tokens")
	}
	if len(db.execCalls) != 1 {
		t.Fatalf("expected 1 exec call deducting token, got %d", len(db.execCalls))
	}
}

func TestConsumeToken_ExhaustedTokens(t *testing.T) {
	ctx := context.Background()
	db := &mockRateLimitDB{
		row: mockRateLimitRow{
			tokens:       0.2,
			maxTokens:    10.0,
			refillRate:   1.0,
			lastRefillAt: time.Now(),
		},
	}

	allowed, err := queue.ConsumeToken(ctx, db, "queue-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected claim to be rate-limited when tokens < 1.0")
	}
}

func TestConsumeToken_RefillOverTime(t *testing.T) {
	ctx := context.Background()
	db := &mockRateLimitDB{
		row: mockRateLimitRow{
			tokens:       0.0,
			maxTokens:    10.0,
			refillRate:   10.0,
			lastRefillAt: time.Now().Add(-500 * time.Millisecond), // 0.5s * 10 = 5 tokens refilled
		},
	}

	allowed, err := queue.ConsumeToken(ctx, db, "queue-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected elapsed time to refill tokens and allow claim")
	}
}

func TestSetQueueRateLimit(t *testing.T) {
	ctx := context.Background()
	db := &mockRateLimitDB{}

	err := queue.SetQueueRateLimit(ctx, db, "queue-1", 50.0)
	if err != nil {
		t.Fatalf("unexpected error setting rate limit: %v", err)
	}
	if len(db.execCalls) != 2 {
		t.Fatalf("expected 2 exec calls (table insert + queue update), got %d", len(db.execCalls))
	}
}
