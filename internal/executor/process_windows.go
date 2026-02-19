//go:build windows

package executor

import "os/exec"

// setupProcessGroup is a no-op on Windows. exec.CommandContext kills the
// process directly, which is sufficient for most cases.
func setupProcessGroup(_ *exec.Cmd) {}
