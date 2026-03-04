package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeexecservice/internal/executor"
)

type stubExecutionService struct{}

func (stubExecutionService) Execute(_ context.Context, req executor.Request) executor.ExecuteResponse {
	if req.Language == "" {
		return executor.ExecuteResponse{Status: "INTERNAL", SandboxError: "missing language"}
	}
	return executor.ExecuteResponse{
		Status:   "OK",
		Stdout:   "ok",
		ExitCode: 0,
		TimeMs:   1,
	}
}

func (stubExecutionService) RunDetailed(_ context.Context, _ executor.Request) executor.DetailedResponse {
	return executor.DetailedResponse{
		Run: &executor.ExecResult{
			ExitCode:   0,
			Stdout:     "ok",
			DurationMs: 1,
		},
	}
}

func TestHealth(t *testing.T) {
	h := New(stubExecutionService{}, 1024)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestExecuteEndpoint(t *testing.T) {
	h := New(stubExecutionService{}, 1024)
	mux := http.NewServeMux()
	h.Register(mux)

	body := ExecuteRequest{Language: "python", Source: "print(1)", TimeLimitMs: 1000}
	encoded, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/execute", bytes.NewReader(encoded))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var parsed executor.ExecuteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if parsed.Status != "OK" {
		t.Fatalf("expected status OK, got %s", parsed.Status)
	}
}

func TestExecuteMethodNotAllowed(t *testing.T) {
	h := New(stubExecutionService{}, 1024)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/execute", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestLegacyRunEndpoint(t *testing.T) {
	h := New(stubExecutionService{}, 1024)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]any{
		"language":      "python",
		"source":        "print(1)",
		"timeLimitMs":   1000,
		"memoryLimitKb": 262144,
	}
	encoded, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(encoded))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var parsed executor.DetailedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if parsed.Run == nil || parsed.Run.ExitCode != 0 {
		t.Fatalf("expected successful run response")
	}
}
