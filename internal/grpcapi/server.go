package grpcapi

import (
	"context"

	"codeexecservice/internal/executor"
	"codeexecservice/proto/codeexecv1"
)

type Server struct {
	codeexecv1.UnimplementedCodeExecutionServiceServer

	exec *executor.Executor
}

func New(exec *executor.Executor) *Server {
	return &Server{exec: exec}
}

func (s *Server) Execute(ctx context.Context, req *codeexecv1.ExecuteRequest) (*codeexecv1.ExecuteResponse, error) {
	if req == nil {
		return &codeexecv1.ExecuteResponse{
			Status:       "INTERNAL",
			SandboxError: "request is required",
			ExitCode:     -1,
		}, nil
	}

	resp := s.exec.Execute(ctx, executor.Request{
		Language:         req.GetLanguage(),
		Source:           req.GetSource(),
		Stdin:            req.GetStdin(),
		TimeLimitMs:      int(req.GetTimeLimitMs()),
		MemoryLimitKb:    int(req.GetMemoryLimitKb()),
		RuntimeOverrides: req.GetRuntimeOverrides(),
		RequestID:        req.GetRequestId(),
		CompileCmd:       req.GetCompileCmd(),
		RunCmd:           req.GetRunCmd(),
		MaxOutputBytes:   int(req.GetMaxOutputBytes()),
		AllowNetwork:     req.GetAllowNetwork(),
	})

	return &codeexecv1.ExecuteResponse{
		Stdout:          resp.Stdout,
		Stderr:          resp.Stderr,
		ExitCode:        int32(resp.ExitCode),
		TimeMs:          int32(resp.TimeMs),
		MemoryKb:        int32(resp.MemoryKb),
		Status:          resp.Status,
		SandboxError:    resp.SandboxError,
		OutputTruncated: resp.OutputTruncated,
	}, nil
}
