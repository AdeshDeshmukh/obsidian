package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var DefaultClient HTTPClient = &http.Client{Timeout: 10 * time.Second}

type CallbackPayload struct {
	JobID     string    `json:"job_id"`
	QueueID   string    `json:"queue_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

func ComputeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func Dispatch(ctx context.Context, pool *pgxpool.Pool, jobID, queueID, status, errorMsg string) {
	rows, err := pool.Query(ctx, `
		SELECT id, callback_url, hmac_secret
		FROM job_callbacks
		WHERE job_id = $1
		  AND status = 'pending'
		  AND (event_type = 'all' OR event_type = $2)
	`, jobID, status)
	if err != nil {
		slog.Error("Failed to query job callbacks", "job_id", jobID, "error", err)
		return
	}
	defer rows.Close()

	type callbackRow struct {
		id          string
		callbackURL string
		hmacSecret  string
	}

	var callbacks []callbackRow
	for rows.Next() {
		var cb callbackRow
		if err := rows.Scan(&cb.id, &cb.callbackURL, &cb.hmacSecret); err == nil {
			callbacks = append(callbacks, cb)
		}
	}

	payload := CallbackPayload{
		JobID:     jobID,
		QueueID:   queueID,
		Status:    status,
		Timestamp: time.Now().UTC(),
		Error:     errorMsg,
	}

	for _, cb := range callbacks {
		go DispatchOne(context.Background(), pool, DefaultClient, cb.id, cb.callbackURL, cb.hmacSecret, payload)
	}
}

func DispatchOne(ctx context.Context, pool *pgxpool.Pool, client HTTPClient, callbackID, url, secret string, payload CallbackPayload) bool {
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	sig := ComputeSignature(secret, body)
	var delivered bool
	var lastErr string

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * 50 * time.Millisecond)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
		if err != nil {
			lastErr = err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Obsidian-Signature", sig)
		req.Header.Set("X-Obsidian-Event", payload.Status)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Sprintf("attempt %d: %s", attempt+1, err.Error())
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			delivered = true
			slog.Info("Webhook delivered",
				"callback_id", callbackID,
				"url", url,
				"job_id", payload.JobID,
				"status_code", resp.StatusCode,
			)
			break
		}
		lastErr = fmt.Sprintf("attempt %d: HTTP %d", attempt+1, resp.StatusCode)
	}

	if pool != nil {
		if delivered {
			_, _ = pool.Exec(ctx, `
				UPDATE job_callbacks
				SET status = 'delivered', delivered_at = now(), attempts = attempts + 1, last_attempt_at = now()
				WHERE id = $1
			`, callbackID)
		} else {
			slog.Warn("Webhook delivery failed after retries",
				"callback_id", callbackID,
				"url", url,
				"job_id", payload.JobID,
				"last_error", lastErr,
			)
			_, _ = pool.Exec(ctx, `
				UPDATE job_callbacks
				SET status = 'failed', attempts = attempts + 1, last_attempt_at = now()
				WHERE id = $1
			`, callbackID)
		}
	}

	return delivered
}
