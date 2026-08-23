package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"obsidian/internal/api"
	"obsidian/internal/testutil"
)

var testUserCounter int64

// Helper to get a JWT token from a running test server
func getTestToken(t *testing.T, ts *httptest.Server) string {
	t.Helper()

	// Register
	seq := atomic.AddInt64(&testUserCounter, 1)
	uniqueEmail := fmt.Sprintf("test-usr-%d-%d@obsidian.io", time.Now().UnixNano(), seq)
	body := fmt.Sprintf(`{"email":%q,"password":"testpass123","role":"admin"}`, uniqueEmail)
	resp, err := http.Post(ts.URL+"/api/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode auth response: %v", err)
	}

	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("no token in auth response: %v", result)
	}
	return token
}

func TestHealthEndpoint(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	router := api.NewRouter(pool)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if result["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %q", result["status"])
	}
}

func TestAuthRejectsMissingToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	router := api.NewRouter(pool)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
}

func TestCreateJobValidatesJobType(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	router := api.NewRouter(pool)
	ts := httptest.NewServer(router)
	defer ts.Close()

	token := getTestToken(t, ts)

	// Create project
	projBody := `{"name":"Test Project"}`
	req, err := http.NewRequest("POST", ts.URL+"/api/projects", bytes.NewBufferString(projBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	projResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create project request failed: %v", err)
	}
	var projResult map[string]interface{}
	_ = json.NewDecoder(projResp.Body).Decode(&projResult)
	_ = projResp.Body.Close()
	projID, _ := projResult["id"].(string)

	// Create queue
	queueBody := `{"name":"test-queue","priority":1,"concurrency_limit":5}`
	req, err = http.NewRequest("POST", ts.URL+"/api/projects/"+projID+"/queues", bytes.NewBufferString(queueBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	queueResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create queue request failed: %v", err)
	}
	var queueResult map[string]interface{}
	_ = json.NewDecoder(queueResp.Body).Decode(&queueResult)
	_ = queueResp.Body.Close()
	queueID, _ := queueResult["id"].(string)

	// Try creating a job with empty job_type
	jobBody := `{"job_type":"","payload":{},"priority":1}`
	req, err = http.NewRequest("POST", ts.URL+"/api/queues/"+queueID+"/jobs", bytes.NewBufferString(jobBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	jobResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create job request failed: %v", err)
	}
	if jobResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty job_type, got %d", jobResp.StatusCode)
	}
	_ = jobResp.Body.Close()

	// Try creating a valid job
	validJobBody := `{"job_type":"noop","payload":{},"priority":1}`
	req, err = http.NewRequest("POST", ts.URL+"/api/queues/"+queueID+"/jobs", bytes.NewBufferString(validJobBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	validResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create valid job request failed: %v", err)
	}
	if validResp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 for valid job, got %d", validResp.StatusCode)
	}
	_ = validResp.Body.Close()
}

func TestListJobsPaginationMetadata(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	router := api.NewRouter(pool)
	ts := httptest.NewServer(router)
	defer ts.Close()

	token := getTestToken(t, ts)

	// Create project + queue
	projBody := `{"name":"Pagination Test"}`
	req, err := http.NewRequest("POST", ts.URL+"/api/projects", bytes.NewBufferString(projBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	projResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	var projResult map[string]interface{}
	_ = json.NewDecoder(projResp.Body).Decode(&projResult)
	_ = projResp.Body.Close()
	projID := projResult["id"].(string)

	queueBody := `{"name":"page-queue","priority":1,"concurrency_limit":5}`
	req, err = http.NewRequest("POST", ts.URL+"/api/projects/"+projID+"/queues", bytes.NewBufferString(queueBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	queueResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	var queueResult map[string]interface{}
	_ = json.NewDecoder(queueResp.Body).Decode(&queueResult)
	_ = queueResp.Body.Close()
	queueID := queueResult["id"].(string)

	// Create 3 jobs
	for i := 0; i < 3; i++ {
		jobBody := `{"job_type":"noop","payload":{},"priority":1}`
		req, err = http.NewRequest("POST", ts.URL+"/api/queues/"+queueID+"/jobs", bytes.NewBufferString(jobBody))
		if err != nil {
			t.Fatalf("failed to build job request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		jr, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("insert job failed: %v", err)
		}
		_ = jr.Body.Close()
	}

	// List with limit=2
	req, err = http.NewRequest("GET", ts.URL+"/api/queues/"+queueID+"/jobs?limit=2&page=1", nil)
	if err != nil {
		t.Fatalf("failed to build list request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list jobs request failed: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listResult map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	// Verify pagination metadata
	if _, ok := listResult["total"]; !ok {
		t.Error("response missing 'total' field")
	}
	if _, ok := listResult["page"]; !ok {
		t.Error("response missing 'page' field")
	}
	if _, ok := listResult["limit"]; !ok {
		t.Error("response missing 'limit' field")
	}
	if _, ok := listResult["data"]; !ok {
		t.Error("response missing 'data' field")
	}

	total := int(listResult["total"].(float64))
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}

	data := listResult["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("expected 2 items on page, got %d", len(data))
	}
}

func TestIdempotencyKeyPreventsDoubleCreation(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	router := api.NewRouter(pool)
	ts := httptest.NewServer(router)
	defer ts.Close()

	token := getTestToken(t, ts)

	// Create project + queue
	projBody := `{"name":"Idempotency Test"}`
	req, err := http.NewRequest("POST", ts.URL+"/api/projects", bytes.NewBufferString(projBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	projResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	var projResult map[string]interface{}
	_ = json.NewDecoder(projResp.Body).Decode(&projResult)
	_ = projResp.Body.Close()
	projID := projResult["id"].(string)

	queueBody := `{"name":"idem-queue","priority":1,"concurrency_limit":5}`
	req, err = http.NewRequest("POST", ts.URL+"/api/projects/"+projID+"/queues", bytes.NewBufferString(queueBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	queueResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	var queueResult map[string]interface{}
	_ = json.NewDecoder(queueResp.Body).Decode(&queueResult)
	_ = queueResp.Body.Close()
	queueID := queueResult["id"].(string)

	// First creation with idempotency key
	jobBody := `{"job_type":"noop","payload":{},"priority":1,"idempotency_key":"unique-key-123"}`
	req, err = http.NewRequest("POST", ts.URL+"/api/queues/"+queueID+"/jobs", bytes.NewBufferString(jobBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	firstResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first job creation request failed: %v", err)
	}
	var firstResult map[string]interface{}
	_ = json.NewDecoder(firstResp.Body).Decode(&firstResult)
	_ = firstResp.Body.Close()

	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first creation expected 201, got %d", firstResp.StatusCode)
	}
	firstID := firstResult["id"].(string)

	// Second creation with same key — should return existing job
	req, err = http.NewRequest("POST", ts.URL+"/api/queues/"+queueID+"/jobs", bytes.NewBufferString(jobBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	secondResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second job creation request failed: %v", err)
	}
	var secondResult map[string]interface{}
	_ = json.NewDecoder(secondResp.Body).Decode(&secondResult)
	_ = secondResp.Body.Close()

	if secondResp.StatusCode != http.StatusOK {
		t.Errorf("idempotent re-creation expected 200, got %d", secondResp.StatusCode)
	}

	secondID := secondResult["id"].(string)
	if secondID != firstID {
		t.Errorf("idempotent response returned different ID: first=%s, second=%s", firstID, secondID)
	}

	idempotent, ok := secondResult["idempotent"].(bool)
	if !ok || !idempotent {
		t.Error("expected idempotent=true in response")
	}
}
