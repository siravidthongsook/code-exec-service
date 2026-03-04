package executor

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestExecutePythonOK(t *testing.T) {
	requireBinary(t, "python3")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "python",
		Source:        "print(input())",
		Stdin:         "hello",
		TimeLimitMs:   1000,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "OK" {
		t.Fatalf("expected OK, got %s (%s)", resp.Status, resp.Stderr)
	}
	if strings.TrimSpace(resp.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", resp.Stdout)
	}
}

func TestExecuteGoCompileError(t *testing.T) {
	requireBinary(t, "go")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "go",
		Source:        "package main\nfunc main(){",
		TimeLimitMs:   1000,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "CE" {
		t.Fatalf("expected CE, got %s (%s)", resp.Status, resp.Stderr)
	}
	if resp.ExitCode == 0 {
		t.Fatalf("expected non-zero compile exit code")
	}
}

func TestExecutePythonRuntimeError(t *testing.T) {
	requireBinary(t, "python3")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "python",
		Source:        "raise RuntimeError('boom')",
		TimeLimitMs:   1000,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "RE" {
		t.Fatalf("expected RE, got %s (%s)", resp.Status, resp.Stderr)
	}
}

func TestExecutePythonTimeout(t *testing.T) {
	requireBinary(t, "python3")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "python",
		Source:        "import time\ntime.sleep(2)",
		TimeLimitMs:   200,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "TLE" {
		t.Fatalf("expected TLE, got %s (%s)", resp.Status, resp.Stderr)
	}
}

func TestExecuteMemoryLimitHeuristic(t *testing.T) {
	requireBinary(t, "python3")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "python",
		Source:        "import sys\nsys.stderr.write('out of memory\\n')\nraise SystemExit(1)",
		TimeLimitMs:   1000,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "MLE" {
		t.Fatalf("expected MLE, got %s (%s)", resp.Status, resp.Stderr)
	}
}

func TestExecuteOutputTruncated(t *testing.T) {
	requireBinary(t, "python3")

	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:       "python",
		Source:         "print('x' * 1000)",
		TimeLimitMs:    1000,
		MemoryLimitKb:  256000,
		MaxOutputBytes: 64,
	})

	if resp.Status != "OK" {
		t.Fatalf("expected OK, got %s (%s)", resp.Status, resp.Stderr)
	}
	if !resp.OutputTruncated {
		t.Fatalf("expected output_truncated=true")
	}
	if len(resp.Stdout) > 64 {
		t.Fatalf("expected truncated output length <= 64, got %d", len(resp.Stdout))
	}
}

func TestExecuteUnsupportedLanguage(t *testing.T) {
	execSvc := mustExecutor(t)
	resp := execSvc.Execute(context.Background(), Request{
		Language:      "haskell",
		Source:        "main = putStrLn \"hi\"",
		TimeLimitMs:   1000,
		MemoryLimitKb: 256000,
	})

	if resp.Status != "INTERNAL" {
		t.Fatalf("expected INTERNAL, got %s", resp.Status)
	}
	if !strings.Contains(resp.SandboxError, "unsupported language") {
		t.Fatalf("unexpected sandbox_error: %q", resp.SandboxError)
	}
}

func mustExecutor(t *testing.T) *Executor {
	t.Helper()
	execSvc, err := New(Options{
		TmpDir:           t.TempDir(),
		CompileTimeoutMs: 5000,
		MaxOutputBytes:   65536,
		MaxSourceBytes:   262144,
		MaxStdinBytes:    262144,
		UseNsjail:        false,
	})
	if err != nil {
		t.Fatalf("executor init failed: %v", err)
	}
	return execSvc
}

func requireBinary(t *testing.T, bin string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "which", bin).Run(); err != nil {
		t.Skipf("binary %q not available", bin)
	}
}
