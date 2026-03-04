package integration

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"codeexecservice/internal/executor"
	"codeexecservice/internal/grpcapi"
	"codeexecservice/proto/codeexecv1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGRPCExecute(t *testing.T) {
	requireBinaryGRPC(t, "python3")

	execSvc, err := executor.New(executor.Options{
		TmpDir:           t.TempDir(),
		CompileTimeoutMs: 5000,
		UseNsjail:        false,
	})
	if err != nil {
		t.Fatalf("executor init failed: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	codeexecv1.RegisterCodeExecutionServiceServer(grpcServer, grpcapi.New(execSvc))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.GracefulStop()
		_ = listener.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("grpc dial failed: %v", err)
	}
	defer conn.Close()

	client := codeexecv1.NewCodeExecutionServiceClient(conn)
	resp, err := client.Execute(ctx, &codeexecv1.ExecuteRequest{
		Language:      "python",
		Source:        "print(input())",
		Stdin:         "pong",
		TimeLimitMs:   1000,
		MemoryLimitKb: 262144,
	})
	if err != nil {
		t.Fatalf("grpc execute failed: %v", err)
	}

	if resp.GetStatus() != "OK" {
		t.Fatalf("expected OK status, got %s (%s)", resp.GetStatus(), resp.GetStderr())
	}
	if strings.TrimSpace(resp.GetStdout()) != "pong" {
		t.Fatalf("unexpected stdout: %q", resp.GetStdout())
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("grpc server error: %v", err)
		}
	default:
	}
}

func requireBinaryGRPC(t *testing.T, bin string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "which", bin).Run(); err != nil {
		t.Skipf("binary %q not available", bin)
	}
}
