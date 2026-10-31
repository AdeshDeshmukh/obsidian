package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"obsidian/internal/webhook"
)

type mockTransport func(req *http.Request) (*http.Response, error)

func (m mockTransport) Do(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestComputeSignature(t *testing.T) {
	secret := "secret-key-123"
	body := []byte(`{"job_id":"job-1","status":"completed"}`)

	sig := webhook.ComputeSignature(secret, body)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("expected signature to have prefix sha256=, got: %s", sig)
	}

	sig2 := webhook.ComputeSignature(secret, body)
	if sig != sig2 {
		t.Fatalf("expected deterministic signature, got %s vs %s", sig, sig2)
	}

	differentSig := webhook.ComputeSignature("different-secret", body)
	if sig == differentSig {
		t.Fatal("expected different signature for different secret")
	}
}

func TestDispatchOne_Success(t *testing.T) {
	var receivedSig string
	var receivedEvent string
	var receivedBody []byte

	client := mockTransport(func(req *http.Request) (*http.Response, error) {
		receivedSig = req.Header.Get("X-Obsidian-Signature")
		receivedEvent = req.Header.Get("X-Obsidian-Event")
		receivedBody, _ = io.ReadAll(req.Body)
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		return rec.Result(), nil
	})

	payload := webhook.CallbackPayload{
		JobID:     "job-100",
		QueueID:   "queue-200",
		Status:    "completed",
		Timestamp: time.Now().UTC(),
	}

	secret := "test-secret"
	delivered := webhook.DispatchOne(context.Background(), nil, client, "cb-1", "http://callback.internal/webhook", secret, payload)

	if !delivered {
		t.Fatal("expected webhook delivery to succeed")
	}
	if receivedEvent != "completed" {
		t.Fatalf("expected event header 'completed', got %q", receivedEvent)
	}
	if receivedSig != webhook.ComputeSignature(secret, receivedBody) {
		t.Fatalf("signature mismatch: got %q", receivedSig)
	}
}

func TestDispatchOne_RetryAndFail(t *testing.T) {
	var attempts int32
	client := mockTransport(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusInternalServerError)
		return rec.Result(), nil
	})

	payload := webhook.CallbackPayload{
		JobID:     "job-fail",
		QueueID:   "queue-fail",
		Status:    "failed",
		Timestamp: time.Now().UTC(),
		Error:     "terminal timeout",
	}

	delivered := webhook.DispatchOne(context.Background(), nil, client, "cb-fail", "http://mock.endpoint/webhook", "secret", payload)

	if delivered {
		t.Fatal("expected delivery to fail for 500 responses")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 delivery attempts, got %d", attempts)
	}
}
