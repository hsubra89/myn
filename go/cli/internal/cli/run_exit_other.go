//go:build !unix

package cli

import "os/exec"

func exitCodeFromExitError(err *exec.ExitError) int {
	return fallbackExitCode(err)
}
