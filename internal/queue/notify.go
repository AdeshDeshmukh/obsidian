package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// ListenForJobs connects to PostgreSQL via a dedicated connection and listens on the 'job_queued' channel.
func ListenForJobs(ctx context.Context, databaseURL string, notifyCh chan<- string) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := runListenerLoop(ctx, databaseURL, notifyCh)
				if err != nil && ctx.Err() == nil {
					slog.Warn("Postgres LISTEN connection dropped, reconnecting in 2s...", "error", err)
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()
}

func runListenerLoop(ctx context.Context, databaseURL string, notifyCh chan<- string) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "LISTEN job_queued;")
	if err != nil {
		return err
	}

	slog.Info("Event-driven LISTEN/NOTIFY worker channel active on 'job_queued'")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			notification, err := conn.WaitForNotification(ctx)
			if err != nil {
				return err
			}
			if notification != nil {
				select {
				case notifyCh <- notification.Payload:
				default:
				}
			}
		}
	}
}
