//go:build !unix

package cli

func stdioProcessGroup(pid int) int {
	return pid
}
