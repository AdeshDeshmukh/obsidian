package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type EnqueueReq struct {
	JobType  string                 `json:"job_type"`
	Payload  map[string]interface{} `json:"payload"`
	Priority int                    `json:"priority"`
}

type JobTemplate struct {
	QueueName string
	JobType   string
	Payload   map[string]interface{}
	Priority  int
}

var defaultJobTemplates = []JobTemplate{
	{"email-queue", "log", map[string]interface{}{"message": "Welcome email dispatched to demo@example.com"}, 5},
	{"email-queue", "noop", map[string]interface{}{"to": "user@example.com", "subject": "Password Reset"}, 3},
	{"image-queue", "sleep", map[string]interface{}{"seconds": 2}, 7},
	{"image-queue", "noop", map[string]interface{}{"url": "https://picsum.photos/800", "width": 200}, 7},
	{"report-queue", "sleep", map[string]interface{}{"seconds": 3}, 2},
	{"report-queue", "log", map[string]interface{}{"message": "Quarterly report generation complete for Q3-2026"}, 2},
	{"email-queue", "fail", map[string]interface{}{"reason": "demo failure"}, 1},
}

func main() {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	token := os.Getenv("SEEDER_TOKEN")
	apiKey := os.Getenv("SEEDER_API_KEY")
	if token == "" && apiKey == "" {
		slog.Error("SEEDER_TOKEN or SEEDER_API_KEY env var required")
		os.Exit(1)
	}

	queueIDs := make(map[string]string)

	slog.Info("Obsidian demo seeder initializing", "api_url", apiURL)

	if emailQueueID := os.Getenv("EMAIL_QUEUE_ID"); emailQueueID != "" {
		queueIDs["email-queue"] = emailQueueID
	}
	if imageQueueID := os.Getenv("IMAGE_QUEUE_ID"); imageQueueID != "" {
		queueIDs["image-queue"] = imageQueueID
	}
	if reportQueueID := os.Getenv("REPORT_QUEUE_ID"); reportQueueID != "" {
		queueIDs["report-queue"] = reportQueueID
	}
	if defaultQueueID := os.Getenv("QUEUE_ID"); defaultQueueID != "" {
		queueIDs["default"] = defaultQueueID
		if _, ok := queueIDs["email-queue"]; !ok {
			queueIDs["email-queue"] = defaultQueueID
		}
		if _, ok := queueIDs["image-queue"]; !ok {
			queueIDs["image-queue"] = defaultQueueID
		}
		if _, ok := queueIDs["report-queue"]; !ok {
			queueIDs["report-queue"] = defaultQueueID
		}
	}

	if len(queueIDs) == 0 {
		slog.Error("No queue IDs configured. Set QUEUE_ID, EMAIL_QUEUE_ID, IMAGE_QUEUE_ID, or REPORT_QUEUE_ID")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := 30 * time.Second
	if envInterval := os.Getenv("SEED_INTERVAL"); envInterval != "" {
		if d, err := time.ParseDuration(envInterval); err == nil && d > 0 {
			interval = d
		}
	}

	slog.Info("Starting seeder loop", "interval", interval, "configured_queues", len(queueIDs))

	// Initial seed
	seedJobs(ctx, apiURL, token, apiKey, queueIDs, defaultJobTemplates)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Seeder shutting down gracefully...")
			return
		case <-ticker.C:
			seedJobs(ctx, apiURL, token, apiKey, queueIDs, defaultJobTemplates)
		}
	}
}

func seedJobs(ctx context.Context, apiURL, token, apiKey string, queueIDs map[string]string, templates []JobTemplate) {
	if len(templates) == 0 {
		return
	}
	count := 3 + rand.Intn(3)
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tmpl := templates[rand.Intn(len(templates))]
		queueID, ok := queueIDs[tmpl.QueueName]
		if !ok {
			continue
		}

		if err := enqueue(ctx, apiURL, token, apiKey, queueID, tmpl.JobType, tmpl.Payload, tmpl.Priority); err != nil {
			slog.Error("Failed to seed job", "queue", tmpl.QueueName, "job_type", tmpl.JobType, "error", err)
		} else {
			slog.Info("Seeded job", "queue", tmpl.QueueName, "job_type", tmpl.JobType)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func enqueue(ctx context.Context, apiURL, token, apiKey, queueID, jobType string, payload map[string]interface{}, priority int) error {
	req := EnqueueReq{JobType: jobType, Payload: payload, Priority: priority}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/queues/%s/jobs", apiURL, queueID), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else if apiKey != "" {
		httpReq.Header.Set("X-API-Key", apiKey)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
