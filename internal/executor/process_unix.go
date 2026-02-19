//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
	"time"
)

const waitDelay = 5 * time.Second

// setupProcessGroup puts the command in its own process group and
// configures cancellation to kill the entire group. Without this,
// context cancellation only kills the shell process, leaving child
// processes (e.g. sleep) alive and holding the stdout pipe open.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay
}
