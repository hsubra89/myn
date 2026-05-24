package cli

import (
	"strings"
)

func personalServerSSHCommandArgs(identityFile string, user string, host string, options ...string) []string {
	args := append([]string{"ssh"}, options...)
	args = append(args,
		"-o", "IdentitiesOnly=yes",
		"-i", identityFile,
		"-l", strings.TrimSpace(user),
		strings.TrimSpace(host),
	)
	return args
}

func personalServerTailscaleSSHCommandArgs(user string, host string, options ...string) []string {
	args := append([]string{"ssh"}, options...)
	args = append(args,
		"-l", strings.TrimSpace(user),
		strings.TrimSpace(host),
	)
	return args
}
