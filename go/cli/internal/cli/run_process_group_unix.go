//go:build unix

package cli

import "golang.org/x/sys/unix"

func stdioProcessGroup(pid int) int {
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		return pid
	}
	return pgid
}
