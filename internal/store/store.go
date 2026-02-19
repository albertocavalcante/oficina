// Package store provides in-memory storage for jobs, agents, and logs.
package store

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
)

const (
	agentOfflineThreshold = 60 * time.Second
	agentStatusOnline     = "online"
	logSubscriberBuffer   = 64
)

// Store holds all application state in memory.
type Store struct {
	mu     sync.RWMutex
	jobs   map[string]*models.Job
	agents map[string]*models.Agent
	logs   map[string][]models.LogLine // job ID -> log lines

	// SSE subscribers for log streaming, keyed by job ID.
	logSubs   map[string][]chan models.LogLine
	logSubsMu sync.RWMutex
}

// New creates an empty store.
func New() *Store {
	return &Store{
		jobs:    make(map[string]*models.Job),
		agents:  make(map[string]*models.Agent),
		logs:    make(map[string][]models.LogLine),
		logSubs: make(map[string][]chan models.LogLine),
	}
}

// --- Jobs ---

// CreateJob adds a new job to the queue.
func (s *Store) CreateJob(req *models.SubmitJobRequest) *models.Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := &models.Job{
		ID:        newID(),
		Name:      req.Name,
		Command:   req.Command,
		Shell:     req.Shell,
		Env:       req.Env,
		Status:    models.JobStatusPending,
		CreatedAt: time.Now(),
	}
	s.jobs[job.ID] = job
	return job
}

// GetJob returns a snapshot of a job by ID.
func (s *Store) GetJob(id string) (*models.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	return &cp, true
}

// ListJobs returns all jobs, newest first.
func (s *Store) ListJobs() []*models.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

// ClaimJob assigns the oldest pending job to an agent. Returns nil if no jobs available.
func (s *Store) ClaimJob(agentID string) *models.Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldest *models.Job
	for _, j := range s.jobs {
		if j.Status != models.JobStatusPending {
			continue
		}
		if oldest == nil || j.CreatedAt.Before(oldest.CreatedAt) {
			oldest = j
		}
	}

	if oldest == nil {
		return nil
	}

	now := time.Now()
	oldest.Status = models.JobStatusRunning
	oldest.AgentID = agentID
	oldest.StartedAt = &now

	// Update agent status.
	if agent, ok := s.agents[agentID]; ok {
		agent.Status = "busy"
		agent.CurrentJob = oldest.ID
	}

	return oldest
}

// CompleteJob marks a job as finished.
func (s *Store) CompleteJob(jobID string, result *models.AgentJobResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	now := time.Now()
	job.EndedAt = &now
	job.ExitCode = &result.ExitCode
	job.Error = result.Error

	if result.ExitCode == 0 && result.Error == "" {
		job.Status = models.JobStatusSucceeded
	} else {
		job.Status = models.JobStatusFailed
	}

	// Free the agent.
	if agent, ok := s.agents[job.AgentID]; ok {
		agent.Status = agentStatusOnline
		agent.CurrentJob = ""
	}

	return nil
}

// CancelJob marks a pending job as cancelled.
func (s *Store) CancelJob(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	if job.Status != models.JobStatusPending {
		return fmt.Errorf("job %s is %s, not pending", jobID, job.Status)
	}

	job.Status = models.JobStatusCancelled
	now := time.Now()
	job.EndedAt = &now
	return nil
}

// --- Agents ---

// RegisterAgent adds or updates an agent.
func (s *Store) RegisterAgent(req *models.AgentRegisterRequest) *models.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if agent with same name exists.
	for _, a := range s.agents {
		if a.Name == req.Name {
			a.OS = req.OS
			a.Arch = req.Arch
			a.Labels = req.Labels
			a.Status = agentStatusOnline
			a.LastSeen = time.Now()
			return a
		}
	}

	agent := &models.Agent{
		ID:       newID(),
		Name:     req.Name,
		OS:       req.OS,
		Arch:     req.Arch,
		Labels:   req.Labels,
		Status:   agentStatusOnline,
		LastSeen: time.Now(),
	}
	s.agents[agent.ID] = agent
	return agent
}

// Heartbeat updates an agent's last-seen time.
func (s *Store) Heartbeat(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}
	agent.LastSeen = time.Now()
	if agent.CurrentJob == "" {
		agent.Status = agentStatusOnline
	}
	return nil
}

// ListAgents returns all agents with updated status.
func (s *Store) ListAgents() []*models.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result := make([]*models.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		if now.Sub(a.LastSeen) > agentOfflineThreshold && a.Status != "busy" {
			a.Status = "offline"
		}
		result = append(result, a)
	}
	return result
}

// --- Logs ---

// AppendLogs adds log lines to a job and notifies SSE subscribers.
func (s *Store) AppendLogs(jobID string, lines []models.LogLine) {
	s.mu.Lock()
	s.logs[jobID] = append(s.logs[jobID], lines...)
	s.mu.Unlock()

	// Notify subscribers.
	s.logSubsMu.RLock()
	subs := s.logSubs[jobID]
	s.logSubsMu.RUnlock()

	for _, ch := range subs {
		for _, line := range lines {
			select {
			case ch <- line:
			default:
				// Drop if subscriber is slow.
			}
		}
	}
}

// GetLogs returns all log lines for a job.
func (s *Store) GetLogs(jobID string) []models.LogLine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logs[jobID]
}

// SubscribeLogs returns a channel that receives new log lines for a job.
// Call UnsubscribeLogs when done.
func (s *Store) SubscribeLogs(jobID string) chan models.LogLine {
	ch := make(chan models.LogLine, logSubscriberBuffer)
	s.logSubsMu.Lock()
	s.logSubs[jobID] = append(s.logSubs[jobID], ch)
	s.logSubsMu.Unlock()
	return ch
}

// UnsubscribeLogs removes a log subscriber.
func (s *Store) UnsubscribeLogs(jobID string, ch chan models.LogLine) {
	s.logSubsMu.Lock()
	defer s.logSubsMu.Unlock()

	subs := s.logSubs[jobID]
	for i, sub := range subs {
		if sub == ch {
			s.logSubs[jobID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func newID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("%x", buf)
}
