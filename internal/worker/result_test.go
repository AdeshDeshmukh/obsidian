package worker

import (
	"context"
	"encoding/json"
	"testing"
)

func TestResultHandlerRegistration(t *testing.T) {
	jobType := "test.math.sum"
	RegisterResultHandler(jobType, func(ctx context.Context, payload []byte) (interface{}, error) {
		var input struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, err
		}
		return map[string]int{"sum": input.A + input.B}, nil
	})

	payloadBytes, _ := json.Marshal(map[string]int{"a": 10, "b": 32})
	handler, ok := handlers[jobType]
	if !ok {
		t.Fatalf("expected handler for %s to be registered", jobType)
	}

	res, err := handler(context.Background(), payloadBytes)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}

	resMap, ok := res.(map[string]int)
	if !ok || resMap["sum"] != 42 {
		t.Fatalf("expected result sum 42, got %+v", res)
	}
}
