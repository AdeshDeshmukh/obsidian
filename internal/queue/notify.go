package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// ListenForJobs connects to PostgreSQL via a dedicated connection and listens on the 'job_queued' channel.
func ListenForJobs(ctx context.Context, databaseURL string, notifyCh chan<- string) {
	ListenForEvents(ctx, databaseURL, notifyCh, nil)
}

// ListenForEvents listens on both 'job_queued' and 'job_cancelled' channels.
func ListenForEvents(ctx context.Context, databaseURL string, notifyCh chan<- string, onCancelled func(jobID string)) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := runListenerLoop(ctx, databaseURL, notifyCh, onCancelled)
				if err != nil && ctx.Err() == nil {
					slog.Warn("Postgres LISTEN connection dropped, reconnecting in 2s...", "error", err)
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()
}

func runListenerLoop(ctx context.Context, databaseURL string, notifyCh chan<- string, onCancelled func(jobID string)) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "LISTEN job_queued; LISTEN job_cancelled;")
	if err != nil {
		return err
	}

	slog.Info("Event-driven LISTEN/NOTIFY worker channels active on 'job_queued' and 'job_cancelled'")

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
				if notification.Channel == "job_cancelled" {
					if onCancelled != nil {
						onCancelled(notification.Payload)
					}
				} else if notification.Channel == "job_queued" && notifyCh != nil {
					select {
					case notifyCh <- notification.Payload:
					default:
					}
				}
			}
		}
	}
}
