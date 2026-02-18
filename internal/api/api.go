// Package api implements the HTTP handlers for the oficina server.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
	"github.com/albertocavalcante/oficina/internal/store"
)

const (
	longPollTimeout   = 30 * time.Second
	claimPollInterval = 2 * time.Second
)

// Server is the oficina HTTP server.
type Server struct {
	store  *store.Store
	logger *slog.Logger
}

// New creates a new API server.
func New(s *store.Store, logger *slog.Logger) *Server {
	return &Server{store: s, logger: logger}
}

// Handler returns the HTTP handler with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Dashboard UI (embedded static files) ---
	mux.Handle("GET /", http.FileServer(http.Dir("ui/dist")))

	// --- Job management API ---
	mux.HandleFunc("POST /api/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleCancelJob)
	mux.HandleFunc("GET /api/jobs/{id}/logs", s.handleGetLogs)
	mux.HandleFunc("GET /api/jobs/{id}/stream", s.handleStreamLogs)

	// --- Agent API ---
	mux.HandleFunc("POST /api/agents/register", s.handleRegisterAgent)
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("POST /api/agent/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /api/agent/next", s.handleClaimJob)
	mux.HandleFunc("POST /api/agent/jobs/{id}/log", s.handleAgentLog)
	mux.HandleFunc("POST /api/agent/jobs/{id}/result", s.handleAgentResult)

	// --- Health ---
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	})

	return withCORS(mux)
}

// --- Job Handlers ---

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req models.SubmitJobRequest
	if err := readJSON(r.Body, &req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		httpError(w, "command is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "job"
	}

	job := s.store.CreateJob(&req)
	s.logger.Info("job created", "id", job.ID, "name", job.Name)
	writeJSON(w, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.ListJobs())
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.store.GetJob(r.PathValue("id"))
	if !ok {
		httpError(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if err := s.store.CancelJob(r.PathValue("id")); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	logs := s.store.GetLogs(r.PathValue("id"))
	if logs == nil {
		logs = []models.LogLine{}
	}
	writeJSON(w, logs)
}

// handleStreamLogs serves an SSE stream of log lines for a job.
func (s *Server) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send existing logs first.
	for _, line := range s.store.GetLogs(jobID) {
		data, _ := json.Marshal(line)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data) //nolint:gosec // SSE stream, not HTML
	}
	flusher.Flush()

	// Check if job is already finished.
	job, exists := s.store.GetJob(jobID)
	if exists && (job.Status == models.JobStatusSucceeded || job.Status == models.JobStatusFailed || job.Status == models.JobStatusCancelled) {
		_, _ = fmt.Fprintf(w, "event: done\ndata: %q\n\n", job.Status) //nolint:gosec // SSE stream, not HTML
		flusher.Flush()
		return
	}

	// Subscribe for new lines.
	ch := s.store.SubscribeLogs(jobID)
	defer s.store.UnsubscribeLogs(jobID, ch)

	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(line)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-time.After(longPollTimeout):
			// Send keepalive comment.
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

			// Check if job finished while we were waiting.
			j, exists := s.store.GetJob(jobID)
			if exists && (j.Status == models.JobStatusSucceeded || j.Status == models.JobStatusFailed || j.Status == models.JobStatusCancelled) {
				_, _ = fmt.Fprintf(w, "event: done\ndata: %q\n\n", j.Status) //nolint:gosec // SSE stream, not HTML
				flusher.Flush()
				return
			}
		}
	}
}

// --- Agent Handlers ---

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req models.AgentRegisterRequest
	if err := readJSON(r.Body, &req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httpError(w, "name is required", http.StatusBadRequest)
		return
	}

	agent := s.store.RegisterAgent(&req)
	s.logger.Info("agent registered", "id", agent.ID, "name", agent.Name, "os", agent.OS)
	writeJSON(w, agent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.ListAgents())
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		httpError(w, "X-Agent-ID header required", http.StatusBadRequest)
		return
	}
	if err := s.store.Heartbeat(agentID); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClaimJob long-polls for the next available job.
func (s *Server) handleClaimJob(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		httpError(w, "X-Agent-ID header required", http.StatusBadRequest)
		return
	}

	// Update heartbeat on every poll.
	_ = s.store.Heartbeat(agentID)

	// Try immediately.
	if job := s.store.ClaimJob(agentID); job != nil {
		s.logger.Info("job claimed", "job_id", job.ID, "agent_id", agentID)
		writeJSON(w, job)
		return
	}

	// Long-poll: check every 2 seconds up to the timeout.
	ticker := time.NewTicker(claimPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(longPollTimeout)
	defer deadline.Stop()

	for {
		select {
		case <-ticker.C:
			if job := s.store.ClaimJob(agentID); job != nil {
				s.logger.Info("job claimed", "job_id", job.ID, "agent_id", agentID)
				writeJSON(w, job)
				return
			}
		case <-deadline.C:
			w.WriteHeader(http.StatusNoContent)
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleAgentLog(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	var batch models.AgentLogBatch
	if err := readJSON(r.Body, &batch); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.store.AppendLogs(jobID, batch.Lines)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAgentResult(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	var result models.AgentJobResult
	if err := readJSON(r.Body, &result); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.CompleteJob(jobID, &result); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	s.logger.Info("job completed", "job_id", jobID, "exit_code", result.ExitCode)
	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func readJSON(body io.Reader, v any) error {
	return json.NewDecoder(body).Decode(v)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, code)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Agent-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
