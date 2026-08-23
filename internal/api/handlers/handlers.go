package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"obsidian/internal/ai"
	"obsidian/internal/api/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// JSON helpers
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, errMsg string) {
	writeJSON(w, status, map[string]string{"error": errMsg})
}

// Auth Handlers
type RegisterReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "invalid email or password (min 6 chars)")
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" && role != "member" && role != "viewer" {
		role = "admin" // Default first/registered user to admin
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction failed")
		return
	}
	defer tx.Rollback(ctx)

	// Create user
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`, req.Email, string(hash), role).Scan(&userID)
	if err != nil {
		writeError(w, http.StatusConflict, "failed to create user: "+err.Error())
		return
	}

	// Create default organization
	var orgID string
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_id)
		VALUES ($1, $2)
		RETURNING id
	`, req.Email+"'s Org", userID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create default organization")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userID,
		"role":   role,
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(middleware.JWTSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token signing failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"token":  tokenStr,
		"userId": userID,
		"role":   role,
	})
}

type LoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	ctx := r.Context()
	var userID, hash, role string
	err := h.pool.QueryRow(ctx, `
		SELECT id, password_hash, role FROM users WHERE email = $1
	`, req.Email).Scan(&userID, &hash, &role)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userID,
		"role":   role,
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(middleware.JWTSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token signing failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token":  tokenStr,
		"userId": userID,
		"role":   role,
	})
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	userRole, _ := r.Context().Value(middleware.UserRoleKey).(string)

	var email string
	_ = h.pool.QueryRow(r.Context(), "SELECT email FROM users WHERE id = $1", userID).Scan(&email)

	writeJSON(w, http.StatusOK, map[string]string{
		"id":    userID,
		"email": email,
		"role":  userRole,
	})
}

// Project Handlers
type CreateProjectReq struct {
	Name string `json:"name"`
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req CreateProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	ctx := r.Context()
	// Get user's org
	var orgID string
	err := h.pool.QueryRow(ctx, `
		SELECT id FROM organizations WHERE owner_id = $1 LIMIT 1
	`, userID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve organization")
		return
	}

	apiKey := fmt.Sprintf("apiKey_%d_%s", time.Now().UnixNano(), userID[:8])

	var projectID string
	err = h.pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, api_key)
		VALUES ($1, $2, $3)
		RETURNING id
	`, orgID, req.Name, apiKey).Scan(&projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      projectID,
		"name":    req.Name,
		"apiKey":  apiKey,
	})
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	rows, err := h.pool.Query(r.Context(), `
		SELECT p.id, p.name, p.api_key, p.created_at
		FROM projects p
		JOIN organizations o ON p.org_id = o.id
		WHERE o.owner_id = $1
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	defer rows.Close()

	type Project struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		APIKey    string    `json:"api_key"`
		CreatedAt time.Time `json:"created_at"`
	}

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.APIKey, &p.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "row scanning failed")
			return
		}
		projects = append(projects, p)
	}

	writeJSON(w, http.StatusOK, projects)
}

// Queue Handlers
type CreateQueueReq struct {
	Name             string `json:"name"`
	Priority         int16  `json:"priority"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
}

func (h *Handler) CreateQueue(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	var req CreateQueueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid queue parameters")
		return
	}

	if req.ConcurrencyLimit <= 0 {
		req.ConcurrencyLimit = 10
	}

	var queueID string
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO queues (project_id, name, priority, concurrency_limit)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, projectID, req.Name, req.Priority, req.ConcurrencyLimit).Scan(&queueID)
	if err != nil {
		writeError(w, http.StatusConflict, "queue name already exists in project")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                queueID,
		"name":              req.Name,
		"priority":          req.Priority,
		"concurrency_limit": req.ConcurrencyLimit,
	})
}

func (h *Handler) ListQueues(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")

	rows, err := h.pool.Query(r.Context(), `
		SELECT q.id, q.name, q.priority, q.concurrency_limit, q.is_paused, q.created_at,
		       count(j.id) FILTER (WHERE j.status = 'queued') as queued_count,
		       count(j.id) FILTER (WHERE j.status = 'running') as running_count,
		       count(j.id) FILTER (WHERE j.status = 'completed') as completed_count,
		       count(j.id) FILTER (WHERE j.status = 'failed') as failed_count
		FROM queues q
		LEFT JOIN jobs j ON j.queue_id = q.id
		WHERE q.project_id = $1
		GROUP BY q.id
		ORDER BY q.created_at DESC
	`, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query queues")
		return
	}
	defer rows.Close()

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
		CreatedAt        time.Time  `json:"created_at"`
		Stats            QueueStats `json:"stats"`
	}

	queues := []Queue{}
	for rows.Next() {
		var q Queue
		if err := rows.Scan(&q.ID, &q.Name, &q.Priority, &q.ConcurrencyLimit, &q.IsPaused, &q.CreatedAt,
			&q.Stats.QueuedCount, &q.Stats.RunningCount, &q.Stats.CompletedCount, &q.Stats.FailedCount); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		queues = append(queues, q)
	}

	writeJSON(w, http.StatusOK, queues)
}

type UpdateQueueReq struct {
	Priority         *int16 `json:"priority,omitempty"`
	ConcurrencyLimit *int   `json:"concurrency_limit,omitempty"`
	IsPaused         *bool  `json:"is_paused,omitempty"`
}

func (h *Handler) UpdateQueue(w http.ResponseWriter, r *http.Request) {
	queueID := chi.URLParam(r, "queueId")
	var req UpdateQueueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid update payload")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction initiation failed")
		return
	}
	defer tx.Rollback(ctx)

	var currentPriority int16
	var currentLimit int
	var currentPaused bool

	err = tx.QueryRow(ctx, `
		SELECT priority, concurrency_limit, is_paused FROM queues WHERE id = $1 FOR UPDATE
	`, queueID).Scan(&currentPriority, &currentLimit, &currentPaused)
	if err != nil {
		writeError(w, http.StatusNotFound, "queue not found")
		return
	}

	if req.Priority != nil {
		currentPriority = *req.Priority
	}
	if req.ConcurrencyLimit != nil {
		currentLimit = *req.ConcurrencyLimit
	}
	if req.IsPaused != nil {
		currentPaused = *req.IsPaused
	}

	_, err = tx.Exec(ctx, `
		UPDATE queues
		SET priority = $1, concurrency_limit = $2, is_paused = $3
		WHERE id = $4
	`, currentPriority, currentLimit, currentPaused, queueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                queueID,
		"priority":          currentPriority,
		"concurrency_limit": currentLimit,
		"is_paused":         currentPaused,
	})
}

// Job Handlers
type CreateJobReq struct {
	JobType        string                 `json:"job_type"`
	Payload        map[string]interface{} `json:"payload"`
	Priority       int16                  `json:"priority"`
	RunAt          *time.Time             `json:"run_at,omitempty"`
	CronExpr       *string                `json:"cron_expr,omitempty"`
	DependsOn      []string               `json:"depends_on,omitempty"`
	BatchID        *string                `json:"batch_id,omitempty"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	queueID := chi.URLParam(r, "queueId")
	var req CreateJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.JobType == "" {
		writeError(w, http.StatusBadRequest, "invalid job payload")
		return
	}

	if strings.TrimSpace(req.JobType) == "" {
		writeError(w, http.StatusBadRequest, "job_type is required and cannot be empty")
		return
	}

	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload structure")
		return
	}

	ctx := r.Context()

	// If cron_expr is provided, register a recurring job definition
	if req.CronExpr != nil && *req.CronExpr != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, err := parser.Parse(*req.CronExpr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cron expression: "+err.Error())
			return
		}

		nextRun := sched.Next(time.Now())

		var schedJobID string
		err = h.pool.QueryRow(ctx, `
			INSERT INTO scheduled_jobs (queue_id, cron_expr, job_type, payload_template, next_run_at, is_active)
			VALUES ($1, $2, $3, $4, $5, true)
			RETURNING id
		`, queueID, *req.CronExpr, req.JobType, payloadBytes, nextRun).Scan(&schedJobID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to schedule cron definition")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"id":          schedJobID,
			"scheduled":   true,
			"next_run_at": nextRun,
		})
		return
	}

	// Normal immediate/delayed job insertion
	runAt := time.Now()
	if req.RunAt != nil {
		runAt = *req.RunAt
	}

	status := "queued"
	if runAt.After(time.Now()) {
		status = "scheduled"
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(ctx)

	// Check idempotency key — if a matching job already exists, return it
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		var existingID, existingStatus string
		var existingRunAt time.Time
		err := h.pool.QueryRow(ctx, `
			SELECT id, status, run_at FROM jobs
			WHERE queue_id = $1 AND idempotency_key = $2
		`, queueID, *req.IdempotencyKey).Scan(&existingID, &existingStatus, &existingRunAt)
		if err == nil {
			// Job already exists — return it idempotently
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":         existingID,
				"status":     existingStatus,
				"run_at":     existingRunAt,
				"idempotent": true,
			})
			return
		}
	}

	var jobID string
	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at, batch_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, queueID, req.JobType, payloadBytes, status, req.Priority, runAt, req.BatchID, req.IdempotencyKey).Scan(&jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "job insertion failed")
		return
	}

	// Add dependencies (DAG)
	for _, parentID := range req.DependsOn {
		_, err = tx.Exec(ctx, `
			INSERT INTO job_dependencies (job_id, depends_on_job_id)
			VALUES ($1, $2)
		`, jobID, parentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid dependency: "+parentID)
			return
		}
	}

	// Logging transaction creation
	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'info', 'Job created and enqueued')
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "logging insert failed")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit job creation")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      jobID,
		"status":  status,
		"run_at":  runAt,
	})
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	queueID := chi.URLParam(r, "queueId")

	// Basic filtering & pagination
	statusFilter := r.URL.Query().Get("status")
	jobTypeFilter := r.URL.Query().Get("job_type")

	query := `
		SELECT id, job_type, status, priority, run_at, attempt, created_at, updated_at
		FROM jobs
		WHERE queue_id = $1
	`
	args := []interface{}{queueID}
	placeholderIndex := 2

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", placeholderIndex)
		args = append(args, statusFilter)
		placeholderIndex++
	}
	if jobTypeFilter != "" {
		query += fmt.Sprintf(" AND job_type = $%d", placeholderIndex)
		args = append(args, jobTypeFilter)
		placeholderIndex++
	}

	// Parse pagination query parameters
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 200 {
				limit = 200 // Cap to 200 to prevent DB stress
			}
		}
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	offset := (page - 1) * limit

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", placeholderIndex, placeholderIndex+1)
	args = append(args, limit, offset)

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query jobs: "+err.Error())
		return
	}
	defer rows.Close()

	type Job struct {
		ID        string    `json:"id"`
		JobType   string    `json:"job_type"`
		Status    string    `json:"status"`
		Priority  int16     `json:"priority"`
		RunAt     time.Time `json:"run_at"`
		Attempt   int       `json:"attempt"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	jobs := []Job{}
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.JobType, &j.Status, &j.Priority, &j.RunAt, &j.Attempt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		jobs = append(jobs, j)
	}

	// Get total count for pagination metadata
	var total int64
	countQuery := `SELECT count(*) FROM jobs WHERE queue_id = $1`
	countArgs := []interface{}{queueID}
	countIdx := 2
	if statusFilter != "" {
		countQuery += fmt.Sprintf(" AND status = $%d", countIdx)
		countArgs = append(countArgs, statusFilter)
		countIdx++
	}
	if jobTypeFilter != "" {
		countQuery += fmt.Sprintf(" AND job_type = $%d", countIdx)
		countArgs = append(countArgs, jobTypeFilter)
	}
	_ = h.pool.QueryRow(r.Context(), countQuery, countArgs...).Scan(&total)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  jobs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	ctx := r.Context()

	type JobDetails struct {
		ID             string                 `json:"id"`
		QueueID        string                 `json:"queue_id"`
		JobType        string                 `json:"job_type"`
		Payload        map[string]interface{} `json:"payload"`
		Status         string                 `json:"status"`
		Priority       int16                  `json:"priority"`
		RunAt          time.Time              `json:"run_at"`
		Attempt        int                    `json:"attempt"`
		ClaimedBy      *string                `json:"claimed_by"`
		LeaseExpiresAt *time.Time             `json:"lease_expires_at"`
		CreatedAt      time.Time              `json:"created_at"`
		UpdatedAt      time.Time              `json:"updated_at"`
		Executions     []interface{}          `json:"executions"`
		Logs           []interface{}          `json:"logs"`
		AISummary      string                 `json:"ai_summary,omitempty"`
	}

	var j JobDetails
	var rawPayload []byte

	err := h.pool.QueryRow(ctx, `
		SELECT id, queue_id, job_type, payload, status, priority, run_at, attempt, claimed_by, lease_expires_at, created_at, updated_at
		FROM jobs
		WHERE id = $1
	`, jobID).Scan(&j.ID, &j.QueueID, &j.JobType, &rawPayload, &j.Status, &j.Priority, &j.RunAt, &j.Attempt, &j.ClaimedBy, &j.LeaseExpiresAt, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
		} else {
			writeError(w, http.StatusInternalServerError, "database error")
		}
		return
	}

	_ = json.Unmarshal(rawPayload, &j.Payload)

	var lastErrorMsg string
	// Fetch executions
	execRows, err := h.pool.Query(ctx, `
		SELECT id, worker_id, attempt, started_at, finished_at, outcome, error_message, duration_ms
		FROM job_executions
		WHERE job_id = $1
		ORDER BY started_at ASC
	`, jobID)
	if err == nil {
		defer execRows.Close()
		for execRows.Next() {
			var id, attempt, durationVal interface{}
			var workerID, startedAt, finishedAt, outcome interface{}
			var errMsg *string
			_ = execRows.Scan(&id, &workerID, &attempt, &startedAt, &finishedAt, &outcome, &errMsg, &durationVal)
			if errMsg != nil {
				lastErrorMsg = *errMsg
			}
			j.Executions = append(j.Executions, map[string]interface{}{
				"id":            id,
				"worker_id":     workerID,
				"attempt":       attempt,
				"started_at":    startedAt,
				"finished_at":   finishedAt,
				"outcome":       outcome,
				"error_message": errMsg,
				"duration_ms":   durationVal,
			})
		}
	}

	var logMsgs []string
	// Fetch logs
	logRows, err := h.pool.Query(ctx, `
		SELECT level, message, logged_at
		FROM job_logs
		WHERE job_id = $1
		ORDER BY logged_at ASC
	`, jobID)
	if err == nil {
		defer logRows.Close()
		for logRows.Next() {
			var level, message, loggedAt interface{}
			_ = logRows.Scan(&level, &message, &loggedAt)
			if msgStr, ok := message.(string); ok {
				logMsgs = append(logMsgs, msgStr)
			}
			j.Logs = append(j.Logs, map[string]interface{}{
				"level":     level,
				"message":   message,
				"logged_at": loggedAt,
			})
		}
	}

	// Generate AI Failure Summary if job is failed or in dead_letter queue
	if j.Status == "failed" || j.Status == "dead_letter" {
		if lastErrorMsg == "" {
			// Check if DLQ record has reason
			_ = h.pool.QueryRow(ctx, `SELECT failure_reason FROM dead_letter_queue WHERE original_job_id = $1`, jobID).Scan(&lastErrorMsg)
		}
		j.AISummary = ai.SummarizeFailure(ctx, j.JobType, lastErrorMsg, logMsgs)
	}

	writeJSON(w, http.StatusOK, j)
}

func (h *Handler) RetryJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(ctx)

	// Verify job exists
	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM jobs WHERE id = $1 FOR UPDATE
	`, jobID).Scan(&status)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	if status != "failed" && status != "dead_letter" {
		writeError(w, http.StatusBadRequest, "only failed or dead_letter jobs can be retried")
		return
	}

	// Update status back to queued and reset attempts
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'queued', attempt = 0, run_at = now(), updated_at = now()
		WHERE id = $1
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update job status")
		return
	}

	// Delete from DLQ
	_, err = tx.Exec(ctx, `
		DELETE FROM dead_letter_queue WHERE original_job_id = $1
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear DLQ record")
		return
	}

	// Log retry
	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'info', 'Manually queued for retry')
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write log")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "job enqueued for retry"})
}

func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM jobs WHERE id = $1 FOR UPDATE
	`, jobID).Scan(&status)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	if status != "queued" && status != "scheduled" {
		writeError(w, http.StatusBadRequest, "only queued or scheduled jobs can be cancelled")
		return
	}

	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel job")
		return
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'info', 'Job cancelled by user')
	`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "log write failed")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "job cancelled successfully"})
}

// Workers handler
func (h *Handler) ListWorkers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT w.id, w.hostname, w.started_at, w.last_seen_at, w.status,
		       j.id AS current_job_id, j.job_type AS current_job_type
		FROM workers w
		LEFT JOIN jobs j ON j.claimed_by = w.id AND j.status = 'running'
		ORDER BY w.last_seen_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query workers")
		return
	}
	defer rows.Close()

	type Worker struct {
		ID             string    `json:"id"`
		Hostname       string    `json:"hostname"`
		StartedAt      time.Time `json:"started_at"`
		LastSeenAt     time.Time `json:"last_seen_at"`
		Status         string    `json:"status"`
		CurrentJobID   *string   `json:"current_job_id,omitempty"`
		CurrentJobType *string   `json:"current_job_type,omitempty"`
	}

	workers := []Worker{}
	for rows.Next() {
		var wk Worker
		if err := rows.Scan(&wk.ID, &wk.Hostname, &wk.StartedAt, &wk.LastSeenAt, &wk.Status, &wk.CurrentJobID, &wk.CurrentJobType); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		workers = append(workers, wk)
	}

	writeJSON(w, http.StatusOK, workers)
}

// Metrics handler
func (h *Handler) GetSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type QueueSize struct {
		QueueID string `json:"queue_id"`
		Name    string `json:"name"`
		Size    int64  `json:"size"`
	}

	type HealthMetrics struct {
		QueueSizes    []QueueSize `json:"queue_sizes"`
		ActiveWorkers int64       `json:"active_workers"`
		FailedCount   int64       `json:"failed_count"`
		SuccessCount  int64       `json:"success_count"`
		DLQCount      int64       `json:"dlq_count"`
		AvgDurationMs int64       `json:"avg_duration_ms"`
	}

	metrics := HealthMetrics{QueueSizes: []QueueSize{}}

	// Query queue sizes
	rows, err := h.pool.Query(ctx, `
		SELECT q.id, q.name, count(j.id)
		FROM queues q
		LEFT JOIN jobs j ON j.queue_id = q.id AND j.status IN ('queued', 'scheduled', 'running')
		GROUP BY q.id, q.name
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var qs QueueSize
			if err := rows.Scan(&qs.QueueID, &qs.Name, &qs.Size); err == nil {
				metrics.QueueSizes = append(metrics.QueueSizes, qs)
			}
		}
	}

	// Active workers (seen in last 15s)
	_ = h.pool.QueryRow(ctx, `
		SELECT count(*) FROM workers WHERE last_seen_at >= now() - interval '15 seconds' AND status = 'active'
	`).Scan(&metrics.ActiveWorkers)

	// Completed / failed runs in last 24h
	_ = h.pool.QueryRow(ctx, `
		SELECT count(*) FROM job_executions WHERE started_at >= now() - interval '24 hours' AND outcome = 'success'
	`).Scan(&metrics.SuccessCount)

	_ = h.pool.QueryRow(ctx, `
		SELECT count(*) FROM job_executions WHERE started_at >= now() - interval '24 hours' AND outcome = 'failure'
	`).Scan(&metrics.FailedCount)

	// DLQ Entries count
	_ = h.pool.QueryRow(ctx, `
		SELECT count(*) FROM dead_letter_queue
	`).Scan(&metrics.DLQCount)

	// Avg Duration MS
	_ = h.pool.QueryRow(ctx, `
		SELECT COALESCE(ROUND(AVG(duration_ms)), 0) FROM job_executions WHERE finished_at >= now() - interval '24 hours' AND outcome = 'success'
	`).Scan(&metrics.AvgDurationMs)

	writeJSON(w, http.StatusOK, metrics)
}

func (h *Handler) GetThroughput(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Throughput series by hour for last 24 hours
	rows, err := h.pool.Query(ctx, `
		SELECT date_trunc('hour', finished_at) AS hr,
		       count(CASE WHEN outcome = 'success' THEN 1 END) AS successes,
		       count(CASE WHEN outcome = 'failure' THEN 1 END) AS failures
		FROM job_executions
		WHERE finished_at >= now() - interval '24 hours'
		GROUP BY hr
		ORDER BY hr ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to calculate throughput: "+err.Error())
		return
	}
	defer rows.Close()

	type ThroughputPoint struct {
		Hour      time.Time `json:"hour"`
		Successes int64     `json:"successes"`
		Failures  int64     `json:"failures"`
	}

	series := []ThroughputPoint{}
	for rows.Next() {
		var pt ThroughputPoint
		if err := rows.Scan(&pt.Hour, &pt.Successes, &pt.Failures); err != nil {
			writeError(w, http.StatusInternalServerError, "scan throughput point failed")
			return
		}
		series = append(series, pt)
	}

	writeJSON(w, http.StatusOK, series)
}

// DLQ Handler
func (h *Handler) ListDLQ(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT dlq.id, dlq.original_job_id, dlq.queue_id, q.name AS queue_name,
		       dlq.payload, dlq.failure_reason, dlq.attempts_made, dlq.moved_at
		FROM dead_letter_queue dlq
		JOIN queues q ON q.id = dlq.queue_id
		ORDER BY dlq.moved_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query DLQ entries: "+err.Error())
		return
	}
	defer rows.Close()

	type DLQEntry struct {
		ID            string                 `json:"id"`
		OriginalJobID string                 `json:"original_job_id"`
		QueueID       string                 `json:"queue_id"`
		QueueName     string                 `json:"queue_name"`
		Payload       map[string]interface{} `json:"payload"`
		FailureReason string                 `json:"failure_reason"`
		AttemptsMade  int                    `json:"attempts_made"`
		MovedAt       time.Time              `json:"moved_at"`
	}

	entries := []DLQEntry{}
	for rows.Next() {
		var e DLQEntry
		var payloadBytes []byte
		if err := rows.Scan(&e.ID, &e.OriginalJobID, &e.QueueID, &e.QueueName, &payloadBytes, &e.FailureReason, &e.AttemptsMade, &e.MovedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		_ = json.Unmarshal(payloadBytes, &e.Payload)
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// CreateBatchJobs Handler
func (h *Handler) CreateBatchJobs(w http.ResponseWriter, r *http.Request) {
	queueID := chi.URLParam(r, "queueId")
	var req struct {
		Jobs []CreateJobReq `json:"jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Jobs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid batch payload")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(ctx)

	var createdIDs []string
	for _, jReq := range req.Jobs {
		payloadBytes, err := json.Marshal(jReq.Payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job payload")
			return
		}
		var jobID string
		err = tx.QueryRow(ctx, `
			INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at, batch_id, idempotency_key)
			VALUES ($1, $2, $3, 'queued', $4, now(), $5, $6)
			RETURNING id
		`, queueID, jReq.JobType, payloadBytes, jReq.Priority, jReq.BatchID, jReq.IdempotencyKey).Scan(&jobID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to insert batch job: "+err.Error())
			return
		}
		createdIDs = append(createdIDs, jobID)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"count":   len(createdIDs),
		"job_ids": createdIDs,
	})
}

// Retry Policy Handlers
type CreateRetryPolicyReq struct {
	Name        string `json:"name"`
	Strategy    string `json:"strategy"`
	BaseDelayMs int    `json:"base_delay_ms"`
	MaxDelayMs  int    `json:"max_delay_ms"`
	MaxAttempts int    `json:"max_attempts"`
}

func (h *Handler) CreateRetryPolicy(w http.ResponseWriter, r *http.Request) {
	var req CreateRetryPolicyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid retry policy payload")
		return
	}

	strategy := strings.ToLower(strings.TrimSpace(req.Strategy))
	if strategy != "fixed" && strategy != "linear" && strategy != "exponential" {
		strategy = "fixed"
	}
	if req.BaseDelayMs <= 0 {
		req.BaseDelayMs = 1000
	}
	if req.MaxDelayMs <= 0 {
		req.MaxDelayMs = 300000
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 5
	}

	var policyID string
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO retry_policies (name, strategy, base_delay_ms, max_delay_ms, max_attempts)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, req.Name, strategy, req.BaseDelayMs, req.MaxDelayMs, req.MaxAttempts).Scan(&policyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create retry policy: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            policyID,
		"name":          req.Name,
		"strategy":      strategy,
		"base_delay_ms": req.BaseDelayMs,
		"max_delay_ms":  req.MaxDelayMs,
		"max_attempts":  req.MaxAttempts,
	})
}

func (h *Handler) ListRetryPolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, strategy, base_delay_ms, max_delay_ms, max_attempts
		FROM retry_policies
		ORDER BY name ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list retry policies: "+err.Error())
		return
	}
	defer rows.Close()

	type Policy struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Strategy    string `json:"strategy"`
		BaseDelayMs int    `json:"base_delay_ms"`
		MaxDelayMs  int    `json:"max_delay_ms"`
		MaxAttempts int    `json:"max_attempts"`
	}

	policies := []Policy{}
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.Strategy, &p.BaseDelayMs, &p.MaxDelayMs, &p.MaxAttempts); err == nil {
			policies = append(policies, p)
		}
	}

	writeJSON(w, http.StatusOK, policies)
}
