package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type FailureItem struct {
	JobType       string `json:"job_type"`
	FailureReason string `json:"failure_reason"`
}

type FailureGroup struct {
	Pattern  string   `json:"pattern"`
	Count    int      `json:"count"`
	JobTypes []string `json:"job_types"`
	Samples  []string `json:"samples"`
}

type TriageResult struct {
	Groups      []FailureGroup `json:"groups"`
	AISummary   string         `json:"ai_summary"`
	TotalFailed int            `json:"total_failed"`
}

func TriageDLQ(ctx context.Context, failures []FailureItem) TriageResult {
	if len(failures) == 0 {
		return TriageResult{
			Groups:      []FailureGroup{},
			AISummary:   "No failures to triage.",
			TotalFailed: 0,
		}
	}

	patternMap := make(map[string]*FailureGroup)
	for _, f := range failures {
		pattern := ClassifyError(f.FailureReason)
		if g, ok := patternMap[pattern]; ok {
			g.Count++
			if len(g.Samples) < 3 {
				g.Samples = append(g.Samples, f.FailureReason)
			}
			found := false
			for _, jt := range g.JobTypes {
				if jt == f.JobType {
					found = true
					break
				}
			}
			if !found {
				g.JobTypes = append(g.JobTypes, f.JobType)
			}
		} else {
			patternMap[pattern] = &FailureGroup{
				Pattern:  pattern,
				Count:    1,
				JobTypes: []string{f.JobType},
				Samples:  []string{f.FailureReason},
			}
		}
	}

	var groups []FailureGroup
	for _, g := range patternMap {
		groups = append(groups, *g)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are an SRE triaging %d failed jobs in the Obsidian queue system.\n\n", len(failures)))
	sb.WriteString("Failure pattern summary (ranked by impact):\n")
	for _, g := range groups {
		sb.WriteString(fmt.Sprintf("- Pattern '%s': %d jobs, types: %v\n", g.Pattern, g.Count, g.JobTypes))
		sb.WriteString(fmt.Sprintf("  Sample errors: %s\n", strings.Join(g.Samples, "; ")))
	}
	sb.WriteString("\nProvide a ranked list of root causes (most impactful first) and specific fix recommendations. Keep each bullet to 2 sentences max.")

	aiSummary := callGeminiForTriage(ctx, groups, sb.String())

	return TriageResult{
		Groups:      groups,
		AISummary:   aiSummary,
		TotalFailed: len(failures),
	}
}

func callGeminiForTriage(ctx context.Context, groups []FailureGroup, prompt string) string {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("AI_API_KEY")
	}
	if apiKey == "" {
		return GenerateHeuristicTriage(groups, prompt)
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return GenerateHeuristicTriage(groups, prompt)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return GenerateHeuristicTriage(groups, prompt)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return GenerateHeuristicTriage(groups, prompt)
	}
	defer resp.Body.Close()

	var res geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return GenerateHeuristicTriage(groups, prompt)
	}

	if len(res.Candidates) > 0 && len(res.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(res.Candidates[0].Content.Parts[0].Text)
	}

	return GenerateHeuristicTriage(groups, prompt)
}

func ClassifyError(errMsg string) string {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "timeout"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "unreachable"):
		return "network_failure"
	case strings.Contains(lower, "json") || strings.Contains(lower, "unmarshal") || strings.Contains(lower, "invalid payload"):
		return "payload_error"
	case strings.Contains(lower, "explicit failure"):
		return "test_failure"
	case strings.Contains(lower, "lease expired"):
		return "worker_crash"
	default:
		return "unknown_error"
	}
}

func GenerateHeuristicTriage(groups []FailureGroup, prompt string) string {
	if len(groups) > 0 {
		switch groups[0].Pattern {
		case "timeout":
			return "Primary cause: timeout errors suggest workers are hitting execution limits. Recommendation: increase lease duration or optimize handler performance."
		case "network_failure":
			return "Primary cause: network failures indicate upstream dependency outages. Recommendation: verify downstream service health and add circuit breaker."
		case "payload_error":
			return "Primary cause: payload validation errors suggest client-side schema drift. Recommendation: validate payloads at enqueue time with a schema registry."
		case "worker_crash":
			return "Primary cause: worker lease expirations suggest out-of-memory or SIGKILL termination. Recommendation: increase worker container memory and check node resource health."
		case "test_failure":
			return "Primary cause: simulated test failure exceptions triggered. Recommendation: review integration testing suites."
		}
	}
	return "Mixed failure patterns detected. Recommendation: inspect individual job logs for specific error context."
}
