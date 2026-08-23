package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// SummarizeFailure produces a 1-2 sentence AI diagnosis for a failed job execution.
func SummarizeFailure(ctx context.Context, jobType, errorMessage string, logs []string) string {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("AI_API_KEY")
	}

	// Try remote Gemini API if key is present
	if apiKey != "" {
		summary, err := callGeminiAPI(ctx, apiKey, jobType, errorMessage, logs)
		if err == nil && summary != "" {
			return summary
		}
	}

	// Smart heuristic fallback when API key is unconfigured or offline
	return generateHeuristicSummary(jobType, errorMessage, logs)
}

func callGeminiAPI(ctx context.Context, apiKey, jobType, errorMessage string, logs []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey

	prompt := fmt.Sprintf(
		"You are a DevOps AI SRE diagnosing a background job failure in Obsidian queue service.\n"+
			"Job Type: %s\n"+
			"Error: %s\n"+
			"Execution Logs:\n%s\n\n"+
			"Provide a concise, professional 1 to 2 sentence root-cause diagnosis and actionable fix recommendation.",
		jobType, errorMessage, strings.Join(logs, "\n"),
	)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API status: %d", resp.StatusCode)
	}

	var res geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Candidates) > 0 && len(res.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(res.Candidates[0].Content.Parts[0].Text), nil
	}

	return "", fmt.Errorf("empty candidates response")
}

func generateHeuristicSummary(jobType, errorMessage string, logs []string) string {
	errLower := strings.ToLower(errorMessage)
	
	if strings.Contains(errLower, "explicit failure simulated") || jobType == "fail" {
		return "Job was invoked with failure testing handler 'fail'. Root cause: Test exception trigger executed as designed."
	}
	if strings.Contains(errLower, "timeout") || strings.Contains(errLower, "context deadline exceeded") {
		return "Job execution exceeded maximum lease or HTTP timeout threshold. Recommendation: Increase max execution timeout or optimize payload size."
	}
	if strings.Contains(errLower, "connection refused") || strings.Contains(errLower, "no such host") {
		return "Upstream dependency network failure detected. Recommendation: Verify database/downstream endpoint availability and DNS resolution."
	}
	if strings.Contains(errLower, "invalid payload") || strings.Contains(errLower, "json") || strings.Contains(errLower, "unmarshal") {
		return "Malformed payload schema mismatch. Recommendation: Inspect job payload structure against required handler parameters."
	}
	
	if errorMessage != "" {
		return fmt.Sprintf("Handler execution failed for job '%s': %s. Recommendation: Inspect execution logs and verify worker dependencies.", jobType, errorMessage)
	}
	
	return fmt.Sprintf("Unexpected non-zero termination for job '%s'. Recommendation: Verify worker node resources and retry policy backoff configuration.", jobType)
}
