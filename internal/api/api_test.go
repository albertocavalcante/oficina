package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
	"github.com/albertocavalcante/oficina/internal/store"
)

// --- Helpers ---

func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s := store.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(s, logger)
	return srv.Handler(), s
}

func newTestServerFast(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	s := store.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(s, logger)
	srv.longPollTimeout = 100 * time.Millisecond
	srv.claimPollInterval = 10 * time.Millisecond
	return srv, s
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i < len(headers)-1; i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return v
}

type sseEvent struct {
	Event string
	Data  string
}

func parseSSEEvents(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	var events []sseEvent
	var current sseEvent
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if current.Data != "" || current.Event != "" {
				events = append(events, current)
				current = sseEvent{}
			}
		}
	}
	if current.Data != "" || current.Event != "" {
		events = append(events, current)
	}
	return events
}

// --- Health ---

func TestHealthz(t *testing.T) {
	handler, _ := newTestServer(t)
	rec := doRequest(t, handler, "GET", "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected %q, got %q", "ok", rec.Body.String())
	}
}

// --- Job Handlers ---

func TestCreateJob(t *testing.T) {
	handler, _ := newTestServer(t)

	// Valid request.
	rec := doRequest(t, handler, "POST", "/api/jobs", models.SubmitJobRequest{
		Name: "build", Command: "make all",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	job := decodeJSON[models.Job](t, rec)
	if job.ID == "" {
		t.Error("expected non-empty ID")
	}
	if job.Name != "build" {
		t.Errorf("expected name %q, got %q", "build", job.Name)
	}

	// Missing command → 400.
	rec = doRequest(t, handler, "POST", "/api/jobs", models.SubmitJobRequest{Name: "no-cmd"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	// Empty name defaults to "job".
	rec = doRequest(t, handler, "POST", "/api/jobs", models.SubmitJobRequest{Command: "echo hi"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	job = decodeJSON[models.Job](t, rec)
	if job.Name != "job" {
		t.Errorf("expected default name %q, got %q", "job", job.Name)
	}

	// Invalid JSON → 400.
	req := httptest.NewRequest("POST", "/api/jobs", strings.NewReader("{bad"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestListJobs(t *testing.T) {
	handler, s := newTestServer(t)

	// Empty → [].
	rec := doRequest(t, handler, "GET", "/api/jobs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	jobs := decodeJSON[[]models.Job](t, rec)
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}

	// After creates → correct count.
	s.CreateJob(&models.SubmitJobRequest{Name: "j1", Command: "echo 1"})
	s.CreateJob(&models.SubmitJobRequest{Name: "j2", Command: "echo 2"})

	rec = doRequest(t, handler, "GET", "/api/jobs", nil)
	jobs = decodeJSON[[]models.Job](t, rec)
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestGetJob(t *testing.T) {
	handler, s := newTestServer(t)

	job := s.CreateJob(&models.SubmitJobRequest{Name: "get-test", Command: "echo hi"})

	// Found → 200.
	rec := doRequest(t, handler, "GET", "/api/jobs/"+job.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	got := decodeJSON[models.Job](t, rec)
	if got.ID != job.ID {
		t.Errorf("got ID %q, want %q", got.ID, job.ID)
	}

	// Not found → 404.
	rec = doRequest(t, handler, "GET", "/api/jobs/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestCancelJob(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "cancel-agent", OS: "linux", Arch: "amd64"})

	// Pending → 204.
	job := s.CreateJob(&models.SubmitJobRequest{Name: "cancel-test", Command: "echo hi"})
	rec := doRequest(t, handler, "POST", "/api/jobs/"+job.ID+"/cancel", nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Running → 400.
	job2 := s.CreateJob(&models.SubmitJobRequest{Name: "running-test", Command: "echo hi"})
	s.ClaimJob(agent.ID)
	rec = doRequest(t, handler, "POST", "/api/jobs/"+job2.ID+"/cancel", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	// Not found → 400.
	rec = doRequest(t, handler, "POST", "/api/jobs/nonexistent/cancel", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetLogs(t *testing.T) {
	handler, s := newTestServer(t)
	job := s.CreateJob(&models.SubmitJobRequest{Name: "log-test", Command: "echo hi"})

	// Empty → [].
	rec := doRequest(t, handler, "GET", "/api/jobs/"+job.ID+"/logs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	logs := decodeJSON[[]models.LogLine](t, rec)
	if len(logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(logs))
	}

	// With logs → returns them.
	s.AppendLogs(job.ID, []models.LogLine{
		{Timestamp: time.Now(), Stream: "stdout", Text: "hello"},
	})
	rec = doRequest(t, handler, "GET", "/api/jobs/"+job.ID+"/logs", nil)
	logs = decodeJSON[[]models.LogLine](t, rec)
	if len(logs) != 1 || logs[0].Text != "hello" {
		t.Errorf("expected 1 log line %q, got %v", "hello", logs)
	}
}

// --- Agent Handlers ---

func TestRegisterAgent(t *testing.T) {
	handler, _ := newTestServer(t)

	// Valid → 200 + agent.
	rec := doRequest(t, handler, "POST", "/api/agents/register", models.AgentRegisterRequest{
		Name: "worker-1", OS: "linux", Arch: "amd64",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	agent := decodeJSON[models.Agent](t, rec)
	if agent.ID == "" {
		t.Error("expected non-empty ID")
	}
	if agent.Name != "worker-1" {
		t.Errorf("expected name %q, got %q", "worker-1", agent.Name)
	}

	// Missing name → 400.
	rec = doRequest(t, handler, "POST", "/api/agents/register", models.AgentRegisterRequest{OS: "linux"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestListAgents(t *testing.T) {
	handler, s := newTestServer(t)

	// Empty → [].
	rec := doRequest(t, handler, "GET", "/api/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	agents := decodeJSON[[]models.Agent](t, rec)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// After register → 1 agent.
	s.RegisterAgent(&models.AgentRegisterRequest{Name: "a1", OS: "linux", Arch: "amd64"})
	rec = doRequest(t, handler, "GET", "/api/agents", nil)
	agents = decodeJSON[[]models.Agent](t, rec)
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestHeartbeat(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "hb", OS: "linux", Arch: "amd64"})

	// Valid → 204.
	rec := doRequest(t, handler, "POST", "/api/agent/heartbeat", nil, "X-Agent-ID", agent.ID)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}

	// Missing header → 400.
	rec = doRequest(t, handler, "POST", "/api/agent/heartbeat", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	// Unknown agent → 404.
	rec = doRequest(t, handler, "POST", "/api/agent/heartbeat", nil, "X-Agent-ID", "nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- Claim Job (Long-Poll) ---

func TestClaimJobImmediate(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "claimer", OS: "linux", Arch: "amd64"})
	s.CreateJob(&models.SubmitJobRequest{Name: "claim-test", Command: "echo hi"})

	rec := doRequest(t, handler, "GET", "/api/agent/next", nil, "X-Agent-ID", agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	job := decodeJSON[models.Job](t, rec)
	if job.Status != models.JobStatusRunning {
		t.Errorf("expected running, got %q", job.Status)
	}
}

func TestClaimJobMissingHeader(t *testing.T) {
	handler, _ := newTestServer(t)

	rec := doRequest(t, handler, "GET", "/api/agent/next", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestClaimJobContextCancel(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "cancel-agent", OS: "linux", Arch: "amd64"})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/agent/next", nil).WithContext(ctx)
	req.Header.Set("X-Agent-ID", agent.ID)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Handler returned after context cancellation.
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after context cancellation")
	}
}

// --- Agent Log & Result ---

func TestAgentLog(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "logger", OS: "linux", Arch: "amd64"})
	job := s.CreateJob(&models.SubmitJobRequest{Name: "log-test", Command: "echo hi"})
	s.ClaimJob(agent.ID)

	batch := models.AgentLogBatch{
		Lines: []models.LogLine{
			{Timestamp: time.Now(), Stream: "stdout", Text: "hello"},
		},
	}
	rec := doRequest(t, handler, "POST", "/api/agent/jobs/"+job.ID+"/log", batch)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify via GET logs.
	rec = doRequest(t, handler, "GET", "/api/jobs/"+job.ID+"/logs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	logs := decodeJSON[[]models.LogLine](t, rec)
	if len(logs) != 1 || logs[0].Text != "hello" {
		t.Errorf("expected 1 log line %q, got %v", "hello", logs)
	}
}

func TestAgentResult(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "result-agent", OS: "linux", Arch: "amd64"})

	// Success (exit 0) → 204 + succeeded.
	job := s.CreateJob(&models.SubmitJobRequest{Name: "result-test", Command: "echo ok"})
	s.ClaimJob(agent.ID)
	rec := doRequest(t, handler, "POST", "/api/agent/jobs/"+job.ID+"/result", models.AgentJobResult{ExitCode: 0})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := s.GetJob(job.ID)
	if got.Status != models.JobStatusSucceeded {
		t.Errorf("expected succeeded, got %q", got.Status)
	}

	// Failure (exit 1) → 204 + failed.
	job2 := s.CreateJob(&models.SubmitJobRequest{Name: "fail-test", Command: "exit 1"})
	s.ClaimJob(agent.ID)
	rec = doRequest(t, handler, "POST", "/api/agent/jobs/"+job2.ID+"/result", models.AgentJobResult{ExitCode: 1})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	got2, _ := s.GetJob(job2.ID)
	if got2.Status != models.JobStatusFailed {
		t.Errorf("expected failed, got %q", got2.Status)
	}

	// Not found → 404.
	rec = doRequest(t, handler, "POST", "/api/agent/jobs/nonexistent/result", models.AgentJobResult{ExitCode: 0})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- CORS ---

func TestCORS(t *testing.T) {
	handler, _ := newTestServer(t)

	// OPTIONS → 204 + CORS headers.
	rec := doRequest(t, handler, "OPTIONS", "/api/jobs", nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header on OPTIONS")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing CORS methods header on OPTIONS")
	}
	if rec.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("missing CORS headers header on OPTIONS")
	}

	// Regular request also has CORS headers.
	rec = doRequest(t, handler, "GET", "/healthz", nil)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header on regular request")
	}
}

// --- SSE Streaming ---

func TestStreamLogsCompletedJob(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "sse-agent", OS: "linux", Arch: "amd64"})

	job := s.CreateJob(&models.SubmitJobRequest{Name: "sse-test", Command: "echo hi"})
	s.ClaimJob(agent.ID)
	s.AppendLogs(job.ID, []models.LogLine{
		{Timestamp: time.Now(), Stream: "stdout", Text: "hello"},
	})
	if err := s.CompleteJob(job.ID, &models.AgentJobResult{ExitCode: 0}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/jobs/"+job.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // test server URL is safe
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	events := parseSSEEvents(t, resp.Body)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %+v", len(events), events)
	}

	// First event should contain the log line.
	if !strings.Contains(events[0].Data, "hello") {
		t.Errorf("expected first event to contain %q, got %q", "hello", events[0].Data)
	}

	// Last event should be a done event.
	last := events[len(events)-1]
	if last.Event != "done" {
		t.Errorf("expected done event, got %+v", last)
	}
}

func TestStreamLogsLiveDelivery(t *testing.T) {
	handler, s := newTestServer(t)
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "sse-live", OS: "linux", Arch: "amd64"})

	job := s.CreateJob(&models.SubmitJobRequest{Name: "live-test", Command: "echo live"})
	s.ClaimJob(agent.ID) // Running, not completed — handler will subscribe.

	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/jobs/"+job.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // test server URL is safe
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Wait for subscription to be established, then append.
	time.Sleep(100 * time.Millisecond)
	s.AppendLogs(job.ID, []models.LogLine{
		{Timestamp: time.Now(), Stream: "stdout", Text: "live-line"},
	})

	// Read one SSE data event.
	scanner := bufio.NewScanner(resp.Body)
	var data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if !strings.Contains(data, "live-line") {
		t.Errorf("expected %q in SSE data, got %q", "live-line", data)
	}
}

// --- Configurable Timeout Tests ---

func TestClaimJobLongPollTimeout(t *testing.T) {
	srv, s := newTestServerFast(t)
	handler := srv.Handler()
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "timeout-agent", OS: "linux", Arch: "amd64"})

	// No jobs available — long-poll should time out and return 204.
	rec := doRequest(t, handler, "GET", "/api/agent/next", nil, "X-Agent-ID", agent.ID)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestClaimJobTickerClaim(t *testing.T) {
	srv, s := newTestServerFast(t)
	handler := srv.Handler()
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "ticker-agent", OS: "linux", Arch: "amd64"})

	// Start long-poll in a goroutine, then create a job after a short delay.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/agent/next", nil)
	req.Header.Set("X-Agent-ID", agent.ID)

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Create job after 30ms — ticker (10ms interval) should pick it up.
	time.Sleep(30 * time.Millisecond)
	s.CreateJob(&models.SubmitJobRequest{Name: "ticker-job", Command: "echo ticker"})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after ticker claimed job")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	job := decodeJSON[models.Job](t, rec)
	if job.Status != models.JobStatusRunning {
		t.Errorf("expected running, got %q", job.Status)
	}
}

func TestStreamLogsKeepaliveAndDoneAfterTimeout(t *testing.T) {
	srv, s := newTestServerFast(t)
	handler := srv.Handler()
	agent := s.RegisterAgent(&models.AgentRegisterRequest{Name: "keepalive-agent", OS: "linux", Arch: "amd64"})

	job := s.CreateJob(&models.SubmitJobRequest{Name: "keepalive-test", Command: "echo keep"})
	s.ClaimJob(agent.ID) // Running — handler will subscribe.

	testSrv := httptest.NewServer(handler)
	defer testSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", testSrv.URL+"/api/jobs/"+job.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // test server URL is safe
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Wait for the keepalive timeout (100ms) to fire, then complete the job.
	time.Sleep(150 * time.Millisecond)
	if err := s.CompleteJob(job.ID, &models.AgentJobResult{ExitCode: 0}); err != nil {
		t.Fatal(err)
	}

	// Read all SSE output.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)

	// Should contain keepalive comment.
	if !strings.Contains(output, ": keepalive") {
		t.Errorf("expected keepalive comment in output:\n%s", output)
	}

	// Should contain done event.
	if !strings.Contains(output, "event: done") {
		t.Errorf("expected done event in output:\n%s", output)
	}
}
