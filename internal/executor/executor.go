// Package executor runs commands and streams output.
package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
)

const shellCmd = "cmd"

// Result holds the outcome of a command execution.
type Result struct {
	ExitCode int
	Error    string
}

// Run executes a job command and calls onLine for each output line.
func Run(ctx context.Context, job *models.Job, onLine func(models.LogLine)) Result {
	shell, flag := shellCommand(job.Shell)
	cmd := exec.CommandContext(ctx, shell, flag, job.Command) //nolint:gosec // executing user-provided commands is the core purpose

	// Set environment variables.
	if len(job.Env) > 0 {
		for k, v := range job.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Merge stdout and stderr via a pipe.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{ExitCode: 1, Error: fmt.Sprintf("stdout pipe: %v", err)}
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return Result{ExitCode: 1, Error: fmt.Sprintf("start: %v", err)}
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		onLine(models.LogLine{
			Timestamp: time.Now(),
			Stream:    "stdout",
			Text:      scanner.Text(),
		})
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return Result{ExitCode: exitErr.ExitCode()}
		}
		return Result{ExitCode: 1, Error: fmt.Sprintf("wait: %v", err)}
	}

	return Result{ExitCode: 0}
}

func shellCommand(shell string) (string, string) {
	if shell != "" {
		switch shell {
		case "pwsh", "powershell":
			return shell, "-Command"
		case shellCmd:
			return shellCmd, "/C"
		case "python", "python3":
			return shell, "-c"
		default:
			return shell, "-c"
		}
	}
	if runtime.GOOS == "windows" {
		return shellCmd, "/C"
	}
	return "sh", "-c"
}
