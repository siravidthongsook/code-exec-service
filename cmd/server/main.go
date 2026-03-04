package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"codeexecservice/internal/config"
	"codeexecservice/internal/executor"
	"codeexecservice/internal/grpcapi"
	"codeexecservice/internal/httpapi"
	"codeexecservice/proto/codeexecv1"

	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()

	execSvc, err := executor.New(cfg.Executor)
	if err != nil {
		log.Fatalf("executor init failed: %v", err)
	}

	handler := httpapi.New(execSvc, cfg.MaxBodyBytes)
	mux := http.NewServeMux()
	handler.Register(mux)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(cfg.ReadHeaderTimeout) * time.Second,
	}

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("gRPC listen failed: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(int(cfg.MaxBodyBytes)),
		grpc.MaxSendMsgSize(int(cfg.MaxBodyBytes)),
	)
	codeexecv1.RegisterCodeExecutionServiceServer(grpcServer, grpcapi.New(execSvc))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	serveErr := make(chan error, 2)

	go func() {
		log.Printf("code-exec-service HTTP listening on %s", cfg.Addr)
		serveErr <- httpServer.ListenAndServe()
	}()

	go func() {
		log.Printf("code-exec-service gRPC listening on %s", cfg.GRPCAddr)
		serveErr <- grpcServer.Serve(grpcListener)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped with error: %v", err)
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)

	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}

	for i := 0; i < 2; i++ {
		select {
		case err := <-serveErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("shutdown server error: %v", err)
			}
		default:
		}
	}
}
