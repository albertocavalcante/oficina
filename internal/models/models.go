// Package models defines the core domain types for oficina.
package models

import "time"

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job represents a unit of work to be executed by an agent.
type Job struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Shell     string            `json:"shell,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Status    JobStatus         `json:"status"`
	AgentID   string            `json:"agentId,omitempty"`
	ExitCode  *int              `json:"exitCode,omitempty"`
	Error     string            `json:"error,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	StartedAt *time.Time        `json:"startedAt,omitempty"`
	EndedAt   *time.Time        `json:"endedAt,omitempty"`
}

// Agent represents a registered worker agent.
type Agent struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Labels     []string  `json:"labels,omitempty"`
	Status     string    `json:"status"` // "online", "offline", "busy"
	LastSeen   time.Time `json:"lastSeen"`
	CurrentJob string    `json:"currentJob,omitempty"`
}

// LogLine represents a single line of job output.
type LogLine struct {
	Timestamp time.Time `json:"ts"`
	Stream    string    `json:"stream"` // "stdout", "stderr"
	Text      string    `json:"text"`
}

// SubmitJobRequest is the API request for creating a job.
type SubmitJobRequest struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Shell   string            `json:"shell,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Labels  []string          `json:"labels,omitempty"`
}

// AgentRegisterRequest is sent by the agent on startup.
type AgentRegisterRequest struct {
	Name   string   `json:"name"`
	OS     string   `json:"os"`
	Arch   string   `json:"arch"`
	Labels []string `json:"labels,omitempty"`
}

// AgentJobResult is sent by the agent when a job finishes.
type AgentJobResult struct {
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

// AgentLogBatch is a batch of log lines sent by the agent.
type AgentLogBatch struct {
	Lines []LogLine `json:"lines"`
}
