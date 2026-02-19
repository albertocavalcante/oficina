package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
)

const agentStatusBusy = "busy"

// --- Helpers ---

func mustCreateJob(t *testing.T, s *Store, command string) *models.Job {
	t.Helper()
	job := s.CreateJob(&models.SubmitJobRequest{
		Name:    "test",
		Command: command,
	})
	if job == nil {
		t.Fatal("CreateJob returned nil")
	}
	return job
}

func mustRegisterAgent(t *testing.T, s *Store, name string) *models.Agent {
	t.Helper()
	agent := s.RegisterAgent(&models.AgentRegisterRequest{
		Name: name,
		OS:   "linux",
		Arch: "amd64",
	})
	if agent == nil {
		t.Fatal("RegisterAgent returned nil")
	}
	return agent
}

// --- Job Tests ---

func TestCreateJob(t *testing.T) {
	s := New()
	job := s.CreateJob(&models.SubmitJobRequest{
		Name:    "build",
		Command: "make all",
		Shell:   "bash",
	})

	if job.ID == "" {
		t.Error("expected non-empty ID")
	}
	if job.Name != "build" {
		t.Errorf("expected name %q, got %q", "build", job.Name)
	}
	if job.Command != "make all" {
		t.Errorf("expected command %q, got %q", "make all", job.Command)
	}
	if job.Shell != "bash" {
		t.Errorf("expected shell %q, got %q", "bash", job.Shell)
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected status pending, got %q", job.Status)
	}
	if job.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestGetJob(t *testing.T) {
	s := New()
	job := mustCreateJob(t, s, "echo hi")

	got, ok := s.GetJob(job.ID)
	if !ok {
		t.Fatal("expected job to be found")
	}
	if got.ID != job.ID {
		t.Errorf("got ID %q, want %q", got.ID, job.ID)
	}

	_, ok = s.GetJob("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent ID")
	}
}

func TestListJobs(t *testing.T) {
	s := New()
	if jobs := s.ListJobs(); len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}

	mustCreateJob(t, s, "echo 1")
	mustCreateJob(t, s, "echo 2")

	if jobs := s.ListJobs(); len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestClaimJob(t *testing.T) {
	s := New()
	agent := mustRegisterAgent(t, s, "agent-1")

	// No jobs available.
	if got := s.ClaimJob(agent.ID); got != nil {
		t.Error("expected nil when no jobs")
	}

	job1 := mustCreateJob(t, s, "echo 1")
	time.Sleep(time.Millisecond) // ensure distinct CreatedAt
	mustCreateJob(t, s, "echo 2")

	// Claims the oldest pending job.
	claimed := s.ClaimJob(agent.ID)
	if claimed == nil {
		t.Fatal("expected a job")
	}
	if claimed.ID != job1.ID {
		t.Errorf("expected oldest job %s, got %s", job1.ID, claimed.ID)
	}
	if claimed.Status != models.JobStatusRunning {
		t.Errorf("expected running, got %q", claimed.Status)
	}
	if claimed.AgentID != agent.ID {
		t.Errorf("expected agentID %s, got %s", agent.ID, claimed.AgentID)
	}

	// Agent should be busy.
	agents := s.ListAgents()
	for _, a := range agents {
		if a.ID == agent.ID && a.Status != agentStatusBusy {
			t.Errorf("expected agent status busy, got %q", a.Status)
		}
	}

	// Claim second job.
	claimed2 := s.ClaimJob(agent.ID)
	if claimed2 == nil {
		t.Fatal("expected second job")
	}

	// No more pending jobs.
	if got := s.ClaimJob(agent.ID); got != nil {
		t.Error("expected nil when all jobs claimed")
	}
}

func TestCompleteJob(t *testing.T) {
	s := New()
	agent := mustRegisterAgent(t, s, "agent-1")

	// Exit 0 → succeeded.
	job := mustCreateJob(t, s, "echo ok")
	s.ClaimJob(agent.ID)

	if err := s.CompleteJob(job.ID, &models.AgentJobResult{ExitCode: 0}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := s.GetJob(job.ID)
	if got.Status != models.JobStatusSucceeded {
		t.Errorf("expected succeeded, got %q", got.Status)
	}
	if got.EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Error("expected exit code 0")
	}

	// Agent should be free (online).
	agents := s.ListAgents()
	for _, a := range agents {
		if a.ID == agent.ID && a.Status != agentStatusOnline {
			t.Errorf("expected agent online, got %q", a.Status)
		}
	}

	// Exit non-zero → failed.
	job2 := mustCreateJob(t, s, "exit 1")
	s.ClaimJob(agent.ID)

	if err := s.CompleteJob(job2.ID, &models.AgentJobResult{ExitCode: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got2, _ := s.GetJob(job2.ID)
	if got2.Status != models.JobStatusFailed {
		t.Errorf("expected failed, got %q", got2.Status)
	}

	// Error string → failed (even with exit 0).
	job3 := mustCreateJob(t, s, "some cmd")
	s.ClaimJob(agent.ID)

	if err := s.CompleteJob(job3.ID, &models.AgentJobResult{ExitCode: 0, Error: "timeout"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got3, _ := s.GetJob(job3.ID)
	if got3.Status != models.JobStatusFailed {
		t.Errorf("expected failed with error string, got %q", got3.Status)
	}

	// Not found.
	if err := s.CompleteJob("nonexistent", &models.AgentJobResult{ExitCode: 0}); err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestCancelJob(t *testing.T) {
	s := New()
	agent := mustRegisterAgent(t, s, "agent-1")

	// Cancel pending → ok.
	job := mustCreateJob(t, s, "echo cancel me")
	if err := s.CancelJob(job.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := s.GetJob(job.ID)
	if got.Status != models.JobStatusCancelled {
		t.Errorf("expected cancelled, got %q", got.Status)
	}
	if got.EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}

	// Cancel running → error.
	job2 := mustCreateJob(t, s, "echo running")
	s.ClaimJob(agent.ID)
	if err := s.CancelJob(job2.ID); err == nil {
		t.Error("expected error cancelling running job")
	}

	// Cancel completed → error.
	if err := s.CompleteJob(job2.ID, &models.AgentJobResult{ExitCode: 0}); err != nil {
		t.Fatalf("unexpected error completing job: %v", err)
	}
	if err := s.CancelJob(job2.ID); err == nil {
		t.Error("expected error cancelling completed job")
	}

	// Not found → error.
	if err := s.CancelJob("nonexistent"); err == nil {
		t.Error("expected error for nonexistent job")
	}
}

// --- Agent Tests ---

func TestRegisterAgent(t *testing.T) {
	s := New()
	agent := s.RegisterAgent(&models.AgentRegisterRequest{
		Name:   "worker-1",
		OS:     "linux",
		Arch:   "amd64",
		Labels: []string{"gpu"},
	})

	if agent.ID == "" {
		t.Error("expected non-empty ID")
	}
	if agent.Name != "worker-1" {
		t.Errorf("expected name %q, got %q", "worker-1", agent.Name)
	}
	if agent.Status != agentStatusOnline {
		t.Errorf("expected online, got %q", agent.Status)
	}

	// Re-register with same name updates fields, keeps ID.
	originalID := agent.ID
	updated := s.RegisterAgent(&models.AgentRegisterRequest{
		Name:   "worker-1",
		OS:     "darwin",
		Arch:   "arm64",
		Labels: []string{"m1"},
	})
	if updated.ID != originalID {
		t.Errorf("expected same ID %s, got %s", originalID, updated.ID)
	}
	if updated.OS != "darwin" {
		t.Errorf("expected OS %q, got %q", "darwin", updated.OS)
	}
	if updated.Arch != "arm64" {
		t.Errorf("expected Arch %q, got %q", "arm64", updated.Arch)
	}
}

func TestHeartbeat(t *testing.T) {
	s := New()
	agent := mustRegisterAgent(t, s, "hb-agent")

	before := agent.LastSeen
	time.Sleep(time.Millisecond)

	if err := s.Heartbeat(agent.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same-package access to verify internal state.
	if !s.agents[agent.ID].LastSeen.After(before) {
		t.Error("expected LastSeen to be updated")
	}

	// Not found.
	if err := s.Heartbeat("nonexistent"); err == nil {
		t.Error("expected error for nonexistent agent")
	}

	// Busy agent stays busy on heartbeat.
	mustCreateJob(t, s, "echo busy")
	s.ClaimJob(agent.ID)

	if err := s.Heartbeat(agent.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.agents[agent.ID].Status != agentStatusBusy {
		t.Errorf("expected busy after heartbeat, got %q", s.agents[agent.ID].Status)
	}
}

func TestListAgents(t *testing.T) {
	s := New()

	// Empty.
	if agents := s.ListAgents(); len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// Online agent.
	agent := mustRegisterAgent(t, s, "online-agent")
	agents := s.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Status != agentStatusOnline {
		t.Errorf("expected online, got %q", agents[0].Status)
	}

	// Marks offline if LastSeen > 60s ago.
	s.mu.Lock()
	agent.LastSeen = time.Now().Add(-2 * agentOfflineThreshold)
	s.mu.Unlock()

	agents = s.ListAgents()
	if agents[0].Status != "offline" {
		t.Errorf("expected offline, got %q", agents[0].Status)
	}

	// Busy agents NOT marked offline even with old LastSeen.
	mustCreateJob(t, s, "busy work")
	s.ClaimJob(agent.ID)

	s.mu.Lock()
	agent.LastSeen = time.Now().Add(-2 * agentOfflineThreshold)
	s.mu.Unlock()

	agents = s.ListAgents()
	if agents[0].Status != agentStatusBusy {
		t.Errorf("expected busy (not offline), got %q", agents[0].Status)
	}
}

// --- Log Tests ---

func TestAppendAndGetLogs(t *testing.T) {
	s := New()
	job := mustCreateJob(t, s, "echo log test")

	lines := []models.LogLine{
		{Timestamp: time.Now(), Stream: "stdout", Text: "line 1"},
		{Timestamp: time.Now(), Stream: "stderr", Text: "line 2"},
	}
	s.AppendLogs(job.ID, lines)

	got := s.GetLogs(job.ID)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0].Text != "line 1" || got[1].Text != "line 2" {
		t.Errorf("unexpected log content: %+v", got)
	}

	// Empty job returns nil.
	if got := s.GetLogs("nonexistent"); got != nil {
		t.Errorf("expected nil for nonexistent job, got %v", got)
	}
}

func TestSubscribeLogs(t *testing.T) {
	s := New()
	job := mustCreateJob(t, s, "echo sub")

	ch := s.SubscribeLogs(job.ID)

	line := models.LogLine{Timestamp: time.Now(), Stream: "stdout", Text: "hello"}
	s.AppendLogs(job.ID, []models.LogLine{line})

	select {
	case got := <-ch:
		if got.Text != "hello" {
			t.Errorf("expected %q, got %q", "hello", got.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for log line")
	}

	// Unsubscribe closes channel.
	s.UnsubscribeLogs(job.ID, ch)
	_, open := <-ch
	if open {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestSubscribeLogsFanOut(t *testing.T) {
	s := New()
	job := mustCreateJob(t, s, "echo fanout")

	ch1 := s.SubscribeLogs(job.ID)
	ch2 := s.SubscribeLogs(job.ID)

	line := models.LogLine{Timestamp: time.Now(), Stream: "stdout", Text: "broadcast"}
	s.AppendLogs(job.ID, []models.LogLine{line})

	for i, ch := range []chan models.LogLine{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Text != "broadcast" {
				t.Errorf("subscriber %d: expected %q, got %q", i, "broadcast", got.Text)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}

	s.UnsubscribeLogs(job.ID, ch1)
	s.UnsubscribeLogs(job.ID, ch2)
}

func TestSubscribeLogsSlowSubscriber(t *testing.T) {
	s := New()
	job := mustCreateJob(t, s, "echo slow")

	ch := s.SubscribeLogs(job.ID)

	// Send more lines than the buffer can hold (logSubscriberBuffer = 64).
	total := logSubscriberBuffer + 20
	lines := make([]models.LogLine, 0, total)
	for i := range total {
		lines = append(lines, models.LogLine{
			Timestamp: time.Now(),
			Stream:    "stdout",
			Text:      fmt.Sprintf("line-%d", i),
		})
	}
	s.AppendLogs(job.ID, lines)

	// Drain: only buffer-size lines should have been delivered.
	var received int
	for len(ch) > 0 {
		<-ch
		received++
	}
	if received != logSubscriberBuffer {
		t.Errorf("expected %d lines (buffer size), got %d", logSubscriberBuffer, received)
	}

	s.UnsubscribeLogs(job.ID, ch)
}

// --- Concurrency ---

func TestConcurrency(t *testing.T) {
	s := New()
	agent := mustRegisterAgent(t, s, "concurrent-agent")

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job := s.CreateJob(&models.SubmitJobRequest{Name: "test", Command: "echo concurrent"})
			if claimed := s.ClaimJob(agent.ID); claimed != nil {
				_ = s.CompleteJob(claimed.ID, &models.AgentJobResult{ExitCode: 0})
			}
			s.AppendLogs(job.ID, []models.LogLine{
				{Timestamp: time.Now(), Stream: "stdout", Text: "line"},
			})
			s.ListJobs()
			s.ListAgents()
		}()
	}
	wg.Wait()
}
