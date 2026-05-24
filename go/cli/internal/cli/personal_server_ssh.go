package cli

import (
	"strings"
)

func personalServerSSHCommandArgs(user string, host string, options ...string) []string {
	args := append([]string{"ssh"}, options...)
	args = append(args,
		"-l", strings.TrimSpace(user),
		strings.TrimSpace(host),
	)
	return args
}
