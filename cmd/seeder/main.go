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
	{"report-queue", "compute", map[string]interface{}{"a": 40, "b": 2}, 4},
	{"email-queue", "fail", map[string]interface{}{"reason": "demo failure"}, 1},
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	token := os.Getenv("SEEDER_TOKEN")
	apiKey := os.Getenv("SEEDER_API_KEY")

	slog.Info("Obsidian demo seeder initializing", "api_url", apiURL)

	if token == "" && apiKey == "" {
		slog.Info("No credentials supplied, auto-authenticating with demo credentials...")
		var err error
		token, err = autoAuthenticate(ctx, apiURL)
		if err != nil {
			slog.Error("Auto-authentication failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Auto-authenticated successfully with demo user")
	}

	queueIDs := make(map[string]string)
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
		slog.Info("No queue IDs configured, discovering/provisioning default queues...")
		if err := ensureDefaultQueues(ctx, apiURL, token, apiKey, queueIDs); err != nil {
			slog.Error("Failed to discover or provision default queues", "error", err)
			os.Exit(1)
		}
	}

	if len(queueIDs) == 0 {
		slog.Error("No queue IDs available for seeding")
		os.Exit(1)
	}

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

func autoAuthenticate(ctx context.Context, apiURL string) (string, error) {
	demoEmail := "demo@obsidian.local"
	demoPassword := "demopassword123"

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    demoEmail,
		"password": demoPassword,
	})

	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Try login
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/api/auth/login", bytes.NewBuffer(loginPayload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var res struct {
						Token string `json:"token"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Token != "" {
						return res.Token, nil
					}
				}
			} else {
				lastErr = err
			}
		}

		// Try register
		regPayload, _ := json.Marshal(map[string]string{
			"email":    demoEmail,
			"password": demoPassword,
			"role":     "admin",
		})
		req, err = http.NewRequestWithContext(ctx, "POST", apiURL+"/api/auth/register", bytes.NewBuffer(regPayload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
					var res struct {
						Token string `json:"token"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Token != "" {
						return res.Token, nil
					}
				}
			} else {
				lastErr = err
			}
		}

		slog.Warn("Seeder waiting for API to become available...", "attempt", attempt, "error", lastErr)
		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("failed to auto-authenticate after retries: %v", lastErr)
}

func ensureDefaultQueues(ctx context.Context, apiURL, token, apiKey string, queueIDs map[string]string) error {
	setAuthHeader := func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
	}

	// 1. Get or create project
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/api/projects", nil)
	if err != nil {
		return err
	}
	setAuthHeader(req)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return fmt.Errorf("failed to decode projects: %w", err)
	}

	var projectID string
	if len(projects) > 0 {
		projectID = projects[0].ID
	} else {
		body, _ := json.Marshal(map[string]string{"name": "Obsidian Demo"})
		createReq, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/api/projects", bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		setAuthHeader(createReq)
		cResp, err := httpClient.Do(createReq)
		if err != nil {
			return err
		}
		defer cResp.Body.Close()
		var newProj struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(cResp.Body).Decode(&newProj); err != nil || newProj.ID == "" {
			return fmt.Errorf("failed to create default project: %w", err)
		}
		projectID = newProj.ID
	}

	// 2. Get existing queues
	qReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/projects/%s/queues", apiURL, projectID), nil)
	if err != nil {
		return err
	}
	setAuthHeader(qReq)
	qResp, err := httpClient.Do(qReq)
	if err != nil {
		return err
	}
	defer qResp.Body.Close()

	var queues []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(qResp.Body).Decode(&queues); err != nil {
		return fmt.Errorf("failed to decode queues: %w", err)
	}

	for _, q := range queues {
		queueIDs[q.Name] = q.ID
	}

	targetQueues := []string{"email-queue", "image-queue", "report-queue"}
	for _, qName := range targetQueues {
		if _, exists := queueIDs[qName]; exists {
			continue
		}
		body, _ := json.Marshal(map[string]interface{}{
			"name":              qName,
			"priority":          1,
			"concurrency_limit": 10,
		})
		createQReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/projects/%s/queues", apiURL, projectID), bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		setAuthHeader(createQReq)
		res, err := httpClient.Do(createQReq)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var newQ struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(res.Body).Decode(&newQ); err == nil && newQ.ID != "" {
			queueIDs[qName] = newQ.ID
		}
	}

	if defaultID, ok := queueIDs["email-queue"]; ok {
		queueIDs["default"] = defaultID
	}
	return nil
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
