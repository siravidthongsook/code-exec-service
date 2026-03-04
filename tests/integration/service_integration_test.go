package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"codeexecservice/internal/executor"
	"codeexecservice/internal/httpapi"
)

type executeResponse struct {
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"exit_code"`
	Status       string `json:"status"`
	SandboxError string `json:"sandbox_error"`
}

func TestExecuteFixtures(t *testing.T) {
	requireBinary(t, "python3")
	requireBinary(t, "go")

	execSvc, err := executor.New(executor.Options{
		TmpDir:           t.TempDir(),
		CompileTimeoutMs: 5000,
		UseNsjail:        false,
	})
	if err != nil {
		t.Fatalf("executor init failed: %v", err)
	}

	h := httpapi.New(execSvc, 2*1024*1024)
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cases := []struct {
		name     string
		fixture  string
		expected string
	}{
		{name: "python ok", fixture: "python_ok.json", expected: "OK"},
		{name: "python re", fixture: "python_re.json", expected: "RE"},
		{name: "python tle", fixture: "python_tle.json", expected: "TLE"},
		{name: "python mle", fixture: "python_mle.json", expected: "MLE"},
		{name: "go ce", fixture: "go_ce.json", expected: "CE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := readFixture(t, tc.fixture)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/execute", bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("build request failed: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}

			var parsed executeResponse
			if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if parsed.Status != tc.expected {
				t.Fatalf("expected status %s, got %s (stderr=%q, sandbox_error=%q)", tc.expected, parsed.Status, parsed.Stderr, parsed.SandboxError)
			}
		})
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "fixtures", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", name, err)
	}
	return data
}

func requireBinary(t *testing.T, bin string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "which", bin).Run(); err != nil {
		t.Skipf("binary %q not available", bin)
	}
}
