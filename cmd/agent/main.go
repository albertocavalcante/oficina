// Package main is the entry point for the oficina agent.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/albertocavalcante/oficina/internal/executor"
	"github.com/albertocavalcante/oficina/internal/models"
)

const (
	httpTimeout           = 30 * time.Second
	logFlushInterval      = 500 * time.Millisecond
	logBatchSize          = 50
	pollRetryDelay        = 5 * time.Second
	longPollClientTimeout = 35 * time.Second
	logChannelSize        = 256
)

func main() {
	var (
		serverURL string
		name      string
		labels    []string
	)

	cmd := &cobra.Command{
		Use:   "oficina-agent",
		Short: "Oficina polling agent",
		RunE: func(_ *cobra.Command, _ []string) error {
			if name == "" {
				h, _ := os.Hostname()
				name = h
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			agent := &agentRunner{
				serverURL:  serverURL,
				name:       name,
				labels:     labels,
				logger:     logger,
				client:     &http.Client{Timeout: httpTimeout},
				pollClient: &http.Client{Timeout: longPollClientTimeout},
			}

			return agent.run(ctx)
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8080", "oficina server URL")
	cmd.Flags().StringVar(&name, "name", "", "agent name (default: hostname)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "agent labels (e.g. windows,x64)")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type agentRunner struct {
	serverURL  string
	name       string
	agentID    string
	labels     []string
	logger     *slog.Logger
	client     *http.Client // short requests (register, logs, result)
	pollClient *http.Client // long-poll with extended timeout
}

func (a *agentRunner) run(ctx context.Context) error {
	// Register with server.
	agent, err := a.register(ctx)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	a.agentID = agent.ID
	a.logger.Info("registered with server",
		"agent_id", a.agentID,
		"name", agent.Name,
	)

	// Poll loop.
	a.logger.Info("polling for jobs", "server", a.serverURL)
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("shutting down")
			return nil
		default:
		}

		job, err := a.pollForJob(ctx)
		if err != nil {
			a.logger.Error("poll error", "error", err)
			time.Sleep(pollRetryDelay)
			continue
		}
		if job == nil {
			continue // long-poll returned 204, loop immediately
		}

		a.logger.Info("job received", "id", job.ID, "name", job.Name)
		a.executeJob(ctx, job)
	}
}

func (a *agentRunner) register(ctx context.Context) (*models.Agent, error) {
	req := models.AgentRegisterRequest{
		Name:   a.name,
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		Labels: a.labels,
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.serverURL+"/api/agents/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq) //nolint:gosec // URL is from user-provided --server flag
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var agent models.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (a *agentRunner) pollForJob(ctx context.Context) (*models.Job, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", a.serverURL+"/api/agent/next", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-ID", a.agentID)

	resp, err := a.pollClient.Do(req) //nolint:gosec // URL is from user-provided --server flag
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	var job models.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (a *agentRunner) executeJob(ctx context.Context, job *models.Job) {
	// Buffer log lines and flush periodically.
	var logBuf []models.LogLine
	flushTicker := time.NewTicker(logFlushInterval)
	defer flushTicker.Stop()

	logCh := make(chan models.LogLine, logChannelSize)
	doneCh := make(chan executor.Result, 1)

	// Run command in goroutine.
	go func() {
		result := executor.Run(ctx, job, func(line models.LogLine) {
			logCh <- line
		})
		doneCh <- result
	}()

	// Collect and flush logs.
	flush := func() {
		if len(logBuf) == 0 {
			return
		}
		a.sendLogs(ctx, job.ID, logBuf)
		logBuf = logBuf[:0]
	}

	var result executor.Result
	running := true
	for running {
		select {
		case line := <-logCh:
			logBuf = append(logBuf, line)
			if len(logBuf) >= logBatchSize {
				flush()
			}
		case <-flushTicker.C:
			flush()
		case result = <-doneCh:
			running = false
		}
	}

	// Drain remaining log lines.
	close(logCh)
	for line := range logCh {
		logBuf = append(logBuf, line)
	}
	flush()

	// Report result.
	a.sendResult(ctx, job.ID, &result)
	a.logger.Info("job finished", "id", job.ID, "exit_code", result.ExitCode)
}

func (a *agentRunner) sendLogs(ctx context.Context, jobID string, lines []models.LogLine) {
	batch := models.AgentLogBatch{Lines: lines}
	body, _ := json.Marshal(batch)

	req, err := http.NewRequestWithContext(ctx, "POST", a.serverURL+"/api/agent/jobs/"+jobID+"/log", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", a.agentID)

	resp, err := a.client.Do(req) //nolint:gosec // URL is from user-provided --server flag
	if err != nil {
		a.logger.Error("failed to send logs", "error", err)
		return
	}
	_ = resp.Body.Close()
}

func (a *agentRunner) sendResult(ctx context.Context, jobID string, result *executor.Result) {
	r := models.AgentJobResult{
		ExitCode: result.ExitCode,
		Error:    result.Error,
	}
	body, _ := json.Marshal(r)

	req, err := http.NewRequestWithContext(ctx, "POST", a.serverURL+"/api/agent/jobs/"+jobID+"/result", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", a.agentID)

	resp, err := a.client.Do(req) //nolint:gosec // URL is from user-provided --server flag
	if err != nil {
		a.logger.Error("failed to send result", "error", err)
		return
	}
	_ = resp.Body.Close()
}
