package client

import "time"

type Job struct {
	ID             string                 `json:"id"`
	QueueID        string                 `json:"queue_id,omitempty"`
	JobType        string                 `json:"job_type,omitempty"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Status         string                 `json:"status"`
	Priority       int16                  `json:"priority,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	Attempt        int                    `json:"attempt,omitempty"`
	Progress       int                    `json:"progress,omitempty"`
	RunAt          time.Time              `json:"run_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at,omitempty"`
}

type EnqueueRequest struct {
	JobType        string                 `json:"job_type"`
	Payload        map[string]interface{} `json:"payload"`
	Priority       int                    `json:"priority,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	RunAt          *time.Time             `json:"run_at,omitempty"`
	CallbackURL    string                 `json:"callback_url,omitempty"`
	Dependencies   []string               `json:"dependencies,omitempty"`
}

type EnqueueResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	RunAt     time.Time `json:"run_at"`
	Scheduled bool      `json:"scheduled,omitempty"`
}

type QueueStats struct {
	QueuedCount    int64 `json:"queued_count"`
	RunningCount   int64 `json:"running_count"`
	CompletedCount int64 `json:"completed_count"`
	FailedCount    int64 `json:"failed_count"`
}

type Queue struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Priority         int16      `json:"priority"`
	ConcurrencyLimit int        `json:"concurrency_limit"`
	IsPaused         bool       `json:"is_paused"`
	State            string     `json:"state"`
	CreatedAt        time.Time  `json:"created_at"`
	Stats            QueueStats `json:"stats,omitempty"`
}

type Worker struct {
	ID             string    `json:"id"`
	Hostname       string    `json:"hostname"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	CurrentJobID   *string   `json:"current_job_id,omitempty"`
	CurrentJobType *string   `json:"current_job_type,omitempty"`
}

type ListJobsResponse struct {
	Data  []Job `json:"data"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
}

