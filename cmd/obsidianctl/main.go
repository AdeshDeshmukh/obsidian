package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	apiURL     string
	token      string
	httpClient = &http.Client{}
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "obsidianctl",
		Short: "Admin CLI for the Obsidian distributed job scheduler",
	}

	root.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080", "Obsidian API URL")
	root.PersistentFlags().StringVar(&token, "token", os.Getenv("OBSIDIAN_TOKEN"), "JWT auth token (or set OBSIDIAN_TOKEN env var)")

	root.AddCommand(
		newQueueCmd(),
		newJobsCmd(),
		newWorkersCmd(),
		newDLQCmd(),
	)

	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newQueueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Queue management commands",
	}

	lsCmd := &cobra.Command{
		Use:   "ls <project-id>",
		Short: "List all queues for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return listQueues(cmd.OutOrStdout(), args[0])
		},
	}

	pauseCmd := &cobra.Command{
		Use:   "pause <queue-id>",
		Short: "Pause a queue (admin only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateQueueState(cmd.OutOrStdout(), args[0], "paused")
		},
	}

	resumeCmd := &cobra.Command{
		Use:   "resume <queue-id>",
		Short: "Resume a paused queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateQueueState(cmd.OutOrStdout(), args[0], "active")
		},
	}

	drainCmd := &cobra.Command{
		Use:   "drain <queue-id>",
		Short: "Drain a queue (finish in-flight, then pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateQueueState(cmd.OutOrStdout(), args[0], "draining")
		},
	}

	cmd.AddCommand(lsCmd, pauseCmd, resumeCmd, drainCmd)
	return cmd
}

func newJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Job management commands",
	}

	lsCmd := &cobra.Command{
		Use:   "ls <queue-id>",
		Short: "List jobs in a queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return listJobs(cmd.OutOrStdout(), args[0])
		},
	}

	cancelCmd := &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a job (admin only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return postAction(cmd.OutOrStdout(), "/api/jobs/"+args[0]+"/cancel")
		},
	}

	retryCmd := &cobra.Command{
		Use:   "retry <job-id>",
		Short: "Retry a failed job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return postAction(cmd.OutOrStdout(), "/api/jobs/"+args[0]+"/retry")
		},
	}

	cmd.AddCommand(lsCmd, cancelCmd, retryCmd)
	return cmd
}

func newWorkersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "workers",
		Short: "List active workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkers(cmd.OutOrStdout())
		},
	}
}

func newDLQCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dlq",
		Short: "Dead letter queue management commands",
	}

	var queueID string
	var limit int

	replayCmd := &cobra.Command{
		Use:   "replay",
		Short: "Bulk replay dead-letter jobs back into queued state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return replayDLQ(cmd.OutOrStdout(), queueID, limit)
		},
	}

	replayCmd.Flags().StringVar(&queueID, "queue", "", "Optional queue ID filter")
	replayCmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of DLQ jobs to replay")

	cmd.AddCommand(replayCmd)
	return cmd
}

func replayDLQ(out io.Writer, queueID string, limit int) error {
	payload := map[string]interface{}{
		"limit": limit,
	}
	if queueID != "" {
		payload["queue_id"] = queueID
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	data, status, err := doRequest("POST", "/api/dlq/replay", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("replay failed (HTTP %d): %s", status, string(data))
	}

	var resp struct {
		ReplayedCount int      `json:"replayed_count"`
		JobIDs        []string `json:"job_ids"`
		Status        string   `json:"status"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}

	fmt.Fprintf(out, "Successfully replayed %d dead-letter jobs.\n", resp.ReplayedCount)
	return nil
}

func doRequest(method, path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, apiURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func listQueues(out io.Writer, projectID string) error {
	data, statusCode, err := doRequest("GET", "/api/projects/"+projectID+"/queues", nil)
	if err != nil {
		return err
	}
	if statusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d): %s", statusCode, string(data))
	}

	var queues []map[string]interface{}
	if err := json.Unmarshal(data, &queues); err != nil {
		var result struct {
			Queues []map[string]interface{} `json:"queues"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to parse queues response: %w", err)
		}
		queues = result.Queues
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATE\tCONCURRENCY\tPRIORITY")
	for _, q := range queues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%v\n",
			q["id"], q["name"], q["state"], q["concurrency_limit"], q["priority"])
	}
	return w.Flush()
}

func listJobs(out io.Writer, queueID string) error {
	data, statusCode, err := doRequest("GET", "/api/queues/"+queueID+"/jobs?page_size=20", nil)
	if err != nil {
		return err
	}
	if statusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d): %s", statusCode, string(data))
	}

	var jobs []map[string]interface{}
	var result struct {
		Data []map[string]interface{} `json:"data"`
		Jobs []map[string]interface{} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &result); err == nil {
		if len(result.Data) > 0 {
			jobs = result.Data
		} else {
			jobs = result.Jobs
		}
	} else {
		_ = json.Unmarshal(data, &jobs)
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSTATUS\tATTEMPT\tPRIORITY")
	for _, j := range jobs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%v\n",
			j["id"], j["job_type"], j["status"], j["attempt"], j["priority"])
	}
	return w.Flush()
}

func listWorkers(out io.Writer) error {
	data, statusCode, err := doRequest("GET", "/api/workers", nil)
	if err != nil {
		return err
	}
	if statusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d): %s", statusCode, string(data))
	}

	var workers []map[string]interface{}
	if err := json.Unmarshal(data, &workers); err != nil {
		var result struct {
			Workers []map[string]interface{} `json:"workers"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to parse workers response: %w", err)
		}
		workers = result.Workers
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tHOSTNAME\tSTATUS\tLAST_SEEN")
	for _, worker := range workers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			worker["id"], worker["hostname"], worker["status"], worker["last_seen_at"])
	}
	return w.Flush()
}

func updateQueueState(out io.Writer, queueID, state string) error {
	body := fmt.Sprintf(`{"state":"%s"}`, state)
	data, status, err := doRequest("PUT", "/api/queues/"+queueID, strings.NewReader(body))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("update failed (HTTP %d): %s", status, string(data))
	}
	fmt.Fprintf(out, "Queue %s → %s (HTTP %d)\n", queueID, state, status)
	return nil
}

func postAction(out io.Writer, path string) error {
	data, status, err := doRequest("POST", path, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("action failed (HTTP %d): %s", status, string(data))
	}
	fmt.Fprintf(out, "Done (HTTP %d)\n", status)
	return nil
}
