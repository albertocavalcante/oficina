//go:build !windows

package executor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/albertocavalcante/oficina/internal/models"
)

func TestRunSuccess(t *testing.T) {
	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{Command: "echo hello"}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
	if len(lines) != 1 || lines[0].Text != "hello" {
		t.Errorf("expected 1 line %q, got %v", "hello", lines)
	}
}

func TestRunMultipleLines(t *testing.T) {
	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{
		Command: "echo line1; echo line2; echo line3",
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for i, want := range []string{"line1", "line2", "line3"} {
		if lines[i].Text != want {
			t.Errorf("line %d: expected %q, got %q", i, want, lines[i].Text)
		}
	}
}

func TestRunFailure(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantCode int
	}{
		{"exit 1", "exit 1", 1},
		{"false", "false", 1},
		{"exit 42", "exit 42", 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Run(context.Background(), &models.Job{Command: tt.command}, func(_ models.LogLine) {})
			if result.ExitCode != tt.wantCode {
				t.Errorf("expected exit %d, got %d", tt.wantCode, result.ExitCode)
			}
		})
	}
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := Run(ctx, &models.Job{Command: "sleep 10"}, func(_ models.LogLine) {})
	elapsed := time.Since(start)

	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for cancelled command")
	}
	if elapsed > 5*time.Second {
		t.Errorf("command was not killed promptly: %v", elapsed)
	}
}

func TestRunStderrMerged(t *testing.T) {
	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{
		Command: "echo stdout_line && echo stderr_line >&2",
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	texts := make([]string, 0, len(lines))
	for _, l := range lines {
		texts = append(texts, l.Text)
	}
	combined := strings.Join(texts, "\n")
	if !strings.Contains(combined, "stdout_line") {
		t.Error("missing stdout_line in output")
	}
	if !strings.Contains(combined, "stderr_line") {
		t.Error("missing stderr_line in output")
	}
}

func TestShellCommand(t *testing.T) {
	tests := []struct {
		shell    string
		wantCmd  string
		wantFlag string
	}{
		{shell: "", wantCmd: "sh", wantFlag: "-c"},
		{shell: "bash", wantCmd: "bash", wantFlag: "-c"},
		{shell: "zsh", wantCmd: "zsh", wantFlag: "-c"},
		{shell: "pwsh", wantCmd: "pwsh", wantFlag: "-Command"},
		{shell: "powershell", wantCmd: "powershell", wantFlag: "-Command"},
		{shell: "cmd", wantCmd: "cmd", wantFlag: "/C"},
		{shell: "python", wantCmd: "python", wantFlag: "-c"},
		{shell: "python3", wantCmd: "python3", wantFlag: "-c"},
	}
	for _, tt := range tests {
		name := tt.shell
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			cmd, flag := shellCommand(tt.shell)
			if cmd != tt.wantCmd {
				t.Errorf("shellCommand(%q) cmd = %q, want %q", tt.shell, cmd, tt.wantCmd)
			}
			if flag != tt.wantFlag {
				t.Errorf("shellCommand(%q) flag = %q, want %q", tt.shell, flag, tt.wantFlag)
			}
		})
	}
}

func TestRunWithExplicitShell(t *testing.T) {
	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{
		Command: "echo from_bash",
		Shell:   "bash",
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
	if len(lines) != 1 || lines[0].Text != "from_bash" {
		t.Errorf("expected %q, got %v", "from_bash", lines)
	}
}

func TestRunWithEnv(t *testing.T) {
	// Job env var is set.
	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{
		Command: "echo $MY_VAR",
		Env:     map[string]string{"MY_VAR": "test_value"},
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if len(lines) == 0 || lines[0].Text != "test_value" {
		t.Errorf("expected %q, got %v", "test_value", lines)
	}

	// Parent env (HOME) is inherited.
	lines = nil
	result = Run(context.Background(), &models.Job{
		Command: "printenv HOME",
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0 for HOME check, got %d", result.ExitCode)
	}
	if len(lines) == 0 || lines[0].Text == "" {
		t.Error("expected HOME to be inherited from parent env")
	}
}

func TestRunWithEnvOverride(t *testing.T) {
	original := os.Getenv("HOME")
	if original == "" {
		t.Skip("HOME not set")
	}

	var lines []models.LogLine
	result := Run(context.Background(), &models.Job{
		Command: "printenv HOME",
		Env:     map[string]string{"HOME": "/tmp/override"},
	}, func(l models.LogLine) {
		lines = append(lines, l)
	})

	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if len(lines) == 0 || lines[0].Text != "/tmp/override" {
		t.Errorf("expected %q, got %v", "/tmp/override", lines)
	}
}
