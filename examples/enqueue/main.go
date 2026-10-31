package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"obsidian/client"
)

func main() {
	baseURL := os.Getenv("OBSIDIAN_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	token := os.Getenv("OBSIDIAN_TOKEN")
	if token == "" {
		token = "demo-token"
	}

	c := client.New(baseURL, token, client.WithTimeout(10*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queueID := os.Getenv("OBSIDIAN_QUEUE_ID")
	if queueID == "" {
		queueID = "default"
	}

	fmt.Printf("Enqueuing job to queue %s on %s...\n", queueID, baseURL)

	resp, err := c.Enqueue(ctx, queueID, client.EnqueueRequest{
		JobType: "billing.invoice.generate",
		Payload: map[string]interface{}{
			"invoice_id": "INV-2026-001",
			"customer":   "Acme Corp",
			"amount_usd": 1500,
		},
		Priority: 10,
	})
	if err != nil {
		log.Fatalf("Failed to enqueue job: %v", err)
	}

	fmt.Printf("Successfully enqueued job: %s (status: %s)\n", resp.ID, resp.Status)

	job, err := c.GetJob(ctx, resp.ID)
	if err != nil {
		log.Printf("Failed to fetch job details: %v", err)
		return
	}

	fmt.Printf("Job details: ID=%s, Type=%s, Status=%s, Progress=%d%%\n",
		job.ID, job.JobType, job.Status, job.Progress)
}

