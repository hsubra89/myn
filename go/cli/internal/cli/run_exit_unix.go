//go:build unix

package cli

import (
	"os/exec"
	"syscall"
)

func exitCodeFromExitError(err *exec.ExitError) int {
	status, ok := err.Sys().(syscall.WaitStatus)
	if !ok {
		return fallbackExitCode(err)
	}
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	if status.Exited() {
		return status.ExitStatus()
	}
	return fallbackExitCode(err)
}
