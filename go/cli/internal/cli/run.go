package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type runOptions struct {
	stdio         bool
	idleAfterText string
}

func newRunCommand() *cobra.Command {
	opts := runOptions{
		idleAfterText: defaultStdioIdleAfter.String(),
	}
	cmd := &cobra.Command{
		Use:          "run [--stdio] [--idle-after duration] -- <command...>",
		Short:        "Run a command with an Idle Lease",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRunCommand(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), opts, args, runDeps{})
		},
	}
	cmd.Flags().BoolVar(&opts.stdio, "stdio", false, "run the command under a terminal-backed Stdio Lease")
	cmd.Flags().StringVar(&opts.idleAfterText, "idle-after", defaultStdioIdleAfter.String(), "idle window for stdio activity")
	return cmd
}

type runDeps struct {
	stdioExecutor stdioLeaseExecutor
}

func runRunCommand(stdin io.Reader, stdout io.Writer, _ io.Writer, opts runOptions, args []string, deps runDeps) error {
	if !opts.stdio {
		return fmt.Errorf("non-stdio command leases are not implemented yet; pass --stdio to run an interactive stdio command")
	}

	return deps.stdioExecutor.Run(stdioLeaseExecutionRequest{
		Command:       append([]string(nil), args...),
		IdleAfterText: opts.idleAfterText,
		Stdin:         stdin,
		Stdout:        stdout,
	})
}
