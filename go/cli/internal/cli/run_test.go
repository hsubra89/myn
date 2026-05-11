package cli

import (
	"bytes"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCommandWithoutStdioFailsClearly(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(BuildInfo{})
	cmd.SetArgs([]string{"run", "--", "echo", "hello"})
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unsupported non-stdio run mode error")
	}
	if !strings.Contains(err.Error(), "non-stdio command leases are not implemented yet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunStdioValidatesIdleAfterBeforeStartingCommand(t *testing.T) {
	tests := []struct {
		name      string
		idleAfter string
		want      string
	}{
		{name: "zero", idleAfter: "0s", want: "idle-after must be greater than zero"},
		{name: "negative", idleAfter: "-1m", want: "idle-after must be greater than zero"},
		{name: "malformed", idleAfter: "soon", want: "parse idle-after"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerCalled := false
			err := runRunCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runOptions{
				stdio:         true,
				idleAfterText: tt.idleAfter,
			}, []string{"echo"}, runDeps{
				stdinIsTerminal: func(io.Reader) bool {
					return true
				},
				stdoutIsTerminal: func(io.Writer) bool {
					return true
				},
				runStdio: func(stdioRunRequest) error {
					runnerCalled = true
					return nil
				},
			})

			if err == nil {
				t.Fatal("expected idle-after validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			if runnerCalled {
				t.Fatal("stdio runner should not start after invalid idle-after")
			}
		})
	}
}

func TestRunStdioDefaultsAndAcceptsIdleAfter(t *testing.T) {
	tests := []struct {
		name      string
		idleAfter string
		want      time.Duration
	}{
		{name: "default", idleAfter: "", want: 30 * time.Minute},
		{name: "custom", idleAfter: "45m", want: 45 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got stdioRunRequest
			err := runRunCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runOptions{
				stdio:         true,
				idleAfterText: tt.idleAfter,
			}, []string{"echo", "hello"}, runDeps{
				stdinIsTerminal: func(io.Reader) bool {
					return true
				},
				stdoutIsTerminal: func(io.Writer) bool {
					return true
				},
				runStdio: func(req stdioRunRequest) error {
					got = req
					return nil
				},
			})

			if err != nil {
				t.Fatalf("run stdio command: %v", err)
			}
			if got.IdleAfter != tt.want {
				t.Fatalf("idle-after mismatch: want %s, got %s", tt.want, got.IdleAfter)
			}
			if strings.Join(got.Command, " ") != "echo hello" {
				t.Fatalf("command mismatch: %#v", got.Command)
			}
		})
	}
}

func TestRunStdioRejectsMissingCommandBeforeStarting(t *testing.T) {
	runnerCalled := false
	err := runRunCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runOptions{
		stdio: true,
	}, nil, runDeps{
		stdinIsTerminal: func(io.Reader) bool {
			return true
		},
		stdoutIsTerminal: func(io.Writer) bool {
			return true
		},
		runStdio: func(stdioRunRequest) error {
			runnerCalled = true
			return nil
		},
	})

	if err == nil {
		t.Fatal("expected missing command error")
	}
	if !strings.Contains(err.Error(), "missing command after --") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runnerCalled {
		t.Fatal("stdio runner should not start when command is missing")
	}
}

func TestRunStdioRequiresTerminalBackedStdinAndStdout(t *testing.T) {
	tests := []struct {
		name           string
		stdinTerminal  bool
		stdoutTerminal bool
		want           string
	}{
		{name: "stdin", stdinTerminal: false, stdoutTerminal: true, want: "terminal-backed stdin"},
		{name: "stdout", stdinTerminal: true, stdoutTerminal: false, want: "terminal-backed stdout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerCalled := false
			err := runRunCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, runOptions{
				stdio: true,
			}, []string{"echo"}, runDeps{
				stdinIsTerminal: func(io.Reader) bool {
					return tt.stdinTerminal
				},
				stdoutIsTerminal: func(io.Writer) bool {
					return tt.stdoutTerminal
				},
				runStdio: func(stdioRunRequest) error {
					runnerCalled = true
					return nil
				},
			})

			if err == nil {
				t.Fatal("expected terminal requirement error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			if runnerCalled {
				t.Fatal("stdio runner should not start without terminal-backed stdio")
			}
		})
	}
}

func TestRunStdioCommandProxiesChildOutputThroughPTY(t *testing.T) {
	skipPTYIntegrationIfUnsupported(t)

	var out bytes.Buffer
	err := runStdioCommand(stdioRunRequest{
		Command: []string{"sh", "-c", "printf stdout; printf stderr >&2"},
		Stdin:   strings.NewReader(""),
		Stdout:  &out,
		Stderr:  &bytes.Buffer{},
	})

	if err != nil {
		t.Fatalf("run stdio command: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "stdout") || !strings.Contains(got, "stderr") {
		t.Fatalf("expected stdout and stderr through PTY, got %q", got)
	}
}

func TestRunStdioCommandProxiesInputThroughPTY(t *testing.T) {
	skipPTYIntegrationIfUnsupported(t)

	var out bytes.Buffer
	err := runStdioCommand(stdioRunRequest{
		Command: []string{"sh", "-c", "stty -echo; IFS= read -r line; printf 'got:%s' \"$line\""},
		Stdin:   strings.NewReader("hello\n"),
		Stdout:  &out,
		Stderr:  &bytes.Buffer{},
	})

	if err != nil {
		t.Fatalf("run stdio command: %v", err)
	}
	if !strings.Contains(out.String(), "got:hello") {
		t.Fatalf("expected child to receive stdin through PTY, got %q", out.String())
	}
}

func TestRunStdioCommandReturnsWrappedExitStatus(t *testing.T) {
	skipPTYIntegrationIfUnsupported(t)

	err := runStdioCommand(stdioRunRequest{
		Command: []string{"sh", "-c", "exit 7"},
		Stdin:   strings.NewReader(""),
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})

	code, ok := CommandExitCode(err)
	if !ok {
		t.Fatalf("expected command exit error, got %v", err)
	}
	if code != 7 {
		t.Fatalf("exit status mismatch: want 7, got %d", code)
	}
}

func TestRunStdioCommandMapsSignalExitStatus(t *testing.T) {
	skipPTYIntegrationIfUnsupported(t)

	err := runStdioCommand(stdioRunRequest{
		Command: []string{"sh", "-c", "kill -TERM $$"},
		Stdin:   strings.NewReader(""),
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})

	code, ok := CommandExitCode(err)
	if !ok {
		t.Fatalf("expected command exit error, got %v", err)
	}
	if code != 143 {
		t.Fatalf("signal exit status mismatch: want 143, got %d", code)
	}
}

func skipPTYIntegrationIfUnsupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY integration is unsupported on Windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for PTY integration tests")
	}
}
