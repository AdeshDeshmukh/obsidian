package worker

import (
	"context"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"obsidian/internal/queue"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	pool        *pgxpool.Pool
	workerID    string
	queueID     string
	concurrency int
}

func New(pool *pgxpool.Pool, workerID, queueID string, concurrency int) *Pool {
	return &Pool{pool: pool, workerID: workerID, queueID: queueID, concurrency: concurrency}
}

func (p *Pool) Run(ctx context.Context) {
	// Register the worker in the DB
	if err := RegisterWorker(ctx, p.pool, p.workerID, "active"); err != nil {
		slog.Warn("Failed to register worker in DB", "worker_id", p.workerID, "error", err)
	}

	// Start self-heartbeat
	stopWorkerHeartbeat := StartWorkerHeartbeat(ctx, p.pool, p.workerID, 5*time.Second)
	defer stopWorkerHeartbeat()

	// Handle graceful shutdown signals
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Create a separate context for running job handlers
	jobCtx, cancelJobs := context.WithCancel(context.Background())
	defer cancelJobs()

	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup

	notifyCh := make(chan string, 100)
	databaseURL := p.pool.Config().ConnString()
	queue.ListenForJobs(ctx, databaseURL, notifyCh)

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	slog.Info("Worker pool started", "worker_id", p.workerID, "queue_id", p.queueID, "concurrency", p.concurrency)

	claimAndDispatch := func() {
		freeSlots := p.concurrency - len(sem)
		if freeSlots <= 0 {
			return
		}

		jobs, err := queue.ClaimBatch(ctx, p.pool, p.queueID, p.workerID, freeSlots, 30*time.Second)
		if err != nil {
			slog.Error("Error claiming job batch", "worker_id", p.workerID, "error", err)
			return
		}

		for _, j := range jobs {
			sem <- struct{}{}
			wg.Add(1)
			go func(job queue.ClaimedJob) {
				defer wg.Done()
				defer func() { <-sem }()
				Execute(jobCtx, p.pool, job, p.workerID)
			}(j)
		}
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutdown signal received, draining active jobs...", "worker_id", p.workerID)
			_ = RegisterWorker(context.Background(), p.pool, p.workerID, "draining")
			
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				slog.Info("All running jobs completed successfully", "worker_id", p.workerID)
			case <-time.After(10 * time.Second):
				slog.Warn("Shutdown timeout reached, force-aborting remaining jobs...", "worker_id", p.workerID)
				cancelJobs()
				<-done
			}
			
			_ = RegisterWorker(context.Background(), p.pool, p.workerID, "dead")
			slog.Info("Graceful shutdown complete", "worker_id", p.workerID)
			return

		case notifiedQueueID := <-notifyCh:
			// Event-driven instant wakeup on job enqueue!
			if p.queueID == "" || p.queueID == notifiedQueueID {
				claimAndDispatch()
			}

		case <-ticker.C:
			claimAndDispatch()
		}
	}
}
