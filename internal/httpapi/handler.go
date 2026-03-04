package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"codeexecservice/internal/executor"
)

type executionService interface {
	Execute(ctx context.Context, req executor.Request) executor.ExecuteResponse
	RunDetailed(ctx context.Context, req executor.Request) executor.DetailedResponse
}

type Handler struct {
	svc          executionService
	maxBodyBytes int64
}

type ExecuteRequest struct {
	Language         string            `json:"language"`
	Source           string            `json:"source"`
	Stdin            string            `json:"stdin"`
	TimeLimitMs      int               `json:"time_limit_ms"`
	MemoryLimitKb    int               `json:"memory_limit_kb"`
	RuntimeOverrides map[string]string `json:"runtime_overrides,omitempty"`
	RequestID        string            `json:"request_id,omitempty"`

	CompileCmd     string `json:"compile_cmd,omitempty"`
	RunCmd         string `json:"run_cmd,omitempty"`
	MaxOutputBytes int    `json:"max_output_bytes,omitempty"`
	AllowNetwork   bool   `json:"allow_network,omitempty"`
}

type legacyRunRequest struct {
	Language       string `json:"language"`
	Source         string `json:"source"`
	Stdin          string `json:"stdin"`
	TimeLimitMs    int    `json:"timeLimitMs"`
	MemoryLimitKb  int    `json:"memoryLimitKb"`
	CompileCmd     string `json:"compileCmd,omitempty"`
	RunCmd         string `json:"runCmd,omitempty"`
	MaxOutputBytes int    `json:"maxOutputBytes,omitempty"`
	AllowNetwork   bool   `json:"allowNetwork,omitempty"`
}

func New(svc executionService, maxBodyBytes int64) *Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 2 * 1024 * 1024
	}
	return &Handler{svc: svc, maxBodyBytes: maxBodyBytes}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/v1/execute", h.handleExecute)
	mux.HandleFunc("/run", h.handleLegacyRun)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecuteRequest
	if err := decodeJSONBody(r, h.maxBodyBytes, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := h.svc.Execute(r.Context(), mapExecuteRequest(req))
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleLegacyRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req legacyRunRequest
	if err := decodeJSONBody(r, h.maxBodyBytes, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := h.svc.RunDetailed(r.Context(), executor.Request{
		Language:       req.Language,
		Source:         req.Source,
		Stdin:          req.Stdin,
		TimeLimitMs:    req.TimeLimitMs,
		MemoryLimitKb:  req.MemoryLimitKb,
		CompileCmd:     req.CompileCmd,
		RunCmd:         req.RunCmd,
		MaxOutputBytes: req.MaxOutputBytes,
		AllowNetwork:   req.AllowNetwork,
	})
	respondJSON(w, http.StatusOK, resp)
}

func mapExecuteRequest(req ExecuteRequest) executor.Request {
	return executor.Request{
		Language:         req.Language,
		Source:           req.Source,
		Stdin:            req.Stdin,
		TimeLimitMs:      req.TimeLimitMs,
		MemoryLimitKb:    req.MemoryLimitKb,
		RuntimeOverrides: req.RuntimeOverrides,
		RequestID:        req.RequestID,
		CompileCmd:       req.CompileCmd,
		RunCmd:           req.RunCmd,
		MaxOutputBytes:   req.MaxOutputBytes,
		AllowNetwork:     req.AllowNetwork,
	}
}

func decodeJSONBody(r *http.Request, maxBodyBytes int64, target any) error {
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxBodyBytes {
		return errors.New("request body too large")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
