package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"codeexecservice/internal/config"
	"codeexecservice/internal/executor"
	"codeexecservice/internal/httpapi"
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

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(cfg.ReadHeaderTimeout) * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("code-exec-service listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}
