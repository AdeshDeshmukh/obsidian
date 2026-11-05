package grpcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	pb "obsidian/internal/grpc/pb"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedObsidianServiceServer
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Server {
	return &Server{pool: pool}
}

func (s *Server) Register(grpcServer *grpc.Server) {
	pb.RegisterObsidianServiceServer(grpcServer, s)
}

func (s *Server) Serve(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gRPC listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	s.Register(grpcServer)

	slog.Info("gRPC server listening", "addr", addr)
	return grpcServer.Serve(lis)
}

func (s *Server) EnqueueJob(ctx context.Context, req *pb.EnqueueJobRequest) (*pb.EnqueueJobResponse, error) {
	if req.QueueId == "" || req.JobType == "" {
		return nil, status.Error(codes.InvalidArgument, "queue_id and job_type are required")
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	var jobID string
	var isDuplicate bool

	err := s.pool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, idempotency_key)
		VALUES ($1, $2, $3, 'queued', $4, NULLIF($5, ''))
		ON CONFLICT (queue_id, idempotency_key) DO UPDATE
			SET updated_at = now()
		RETURNING id, (xmax != 0) AS is_duplicate
	`, req.QueueId, req.JobType, json.RawMessage(payload), req.Priority, req.IdempotencyKey).
		Scan(&jobID, &isDuplicate)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to enqueue job: %v", err)
	}

	return &pb.EnqueueJobResponse{
		JobId:     jobID,
		Status:    "queued",
		Duplicate: isDuplicate,
	}, nil
}

func (s *Server) GetJobStatus(ctx context.Context, req *pb.GetJobStatusRequest) (*pb.GetJobStatusResponse, error) {
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	var jobID, jobStatus string
	var attempt, progress int32

	err := s.pool.QueryRow(ctx, `
		SELECT j.id, j.status, j.attempt,
		       COALESCE((SELECT je.progress FROM job_executions je WHERE je.job_id = j.id AND je.finished_at IS NULL LIMIT 1), 0)
		FROM jobs j
		WHERE j.id = $1
	`, req.JobId).Scan(&jobID, &jobStatus, &attempt, &progress)

	if err != nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %v", err)
	}

	return &pb.GetJobStatusResponse{
		JobId:    jobID,
		Status:   jobStatus,
		Attempt:  attempt,
		Progress: progress,
	}, nil
}

func (s *Server) CancelJob(ctx context.Context, req *pb.CancelJobRequest) (*pb.CancelJobResponse, error) {
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'scheduled')
	`, req.JobId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cancel failed: %v", err)
	}
	return &pb.CancelJobResponse{Success: tag.RowsAffected() > 0}, nil
}

func (s *Server) ListQueues(ctx context.Context, req *pb.ListQueuesRequest) (*pb.ListQueuesResponse, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(state::text, 'active'), concurrency_limit
		FROM queues
		WHERE project_id = $1 AND deleted_at IS NULL
		ORDER BY created_at
	`, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	defer rows.Close()

	var queues []*pb.Queue
	for rows.Next() {
		var q pb.Queue
		if err := rows.Scan(&q.Id, &q.Name, &q.State, &q.ConcurrencyLimit); err == nil {
			queues = append(queues, &q)
		}
	}

	return &pb.ListQueuesResponse{Queues: queues}, nil
}
