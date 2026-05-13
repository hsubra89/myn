package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConnectFromConfiguredRootStartsSSHBackedTmux(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, "Code Projects"))
	identity := seedTestSSHIdentity(t, home, ".ssh/id_ed25519", "connect@host", 0o600)
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	if err := saveAppConfig(configPath, appConfig{
		Projects: projectsConfig{
			LocalRoot:  "Code Projects",
			RemoteRoot: "Remote Projects",
		},
		SSH: sshConfig{IdentityFile: identity.Relative},
		PersonalServer: personalServerConfig{
			ServerID: 123456,
			User:     "harish",
			IPv4:     "203.0.113.10",
		},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	runner := &fakeConnectProcessRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := runConnectCommand(strings.NewReader(""), &out, &errOut, nil, connectDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		userHomeDir: func() (string, error) {
			return home, nil
		},
		workingDir: func() (string, error) {
			return filepath.Join(home, "Code Projects"), nil
		},
		stdinIsTerminal: func(io.Reader) bool {
			return true
		},
		stdoutIsTerminal: func(io.Writer) bool {
			return true
		},
		runProcess: runner.Run,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("connect should be quiet on stdout before SSH handoff, got %q", got)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("connect should be quiet on stderr before SSH handoff, got %q", got)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("process run count mismatch: want 1, got %d", len(runner.requests))
	}
	wantCommand := []string{
		"ssh",
		"-t",
		"-o", "StrictHostKeyChecking=accept-new",
		"-i", identity.PrivatePath,
		"harish@203.0.113.10",
		"bash", "-lc", `'exec tmux new-session -A -s myn-project -c $HOME/'\''Remote Projects'\'''`,
	}
	if got := runner.requests[0].Command; !reflect.DeepEqual(got, wantCommand) {
		t.Fatalf("ssh command mismatch:\nwant %#v\ngot  %#v", wantCommand, got)
	}
}

func TestConnectCommandAndAliasRouteToSameBehavior(t *testing.T) {
	for _, commandName := range []string{"connect", "c"} {
		t.Run(commandName, func(t *testing.T) {
			fixture := newConnectTestFixture(t)
			runner := &fakeConnectProcessRunner{}
			cmd := newRootCommand(BuildInfo{}, rootDeps{
				connect: fixture.deps(runner),
			})
			cmd.SetArgs([]string{commandName})
			cmd.SetIn(strings.NewReader(""))

			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute %s: %v", commandName, err)
			}
			if len(runner.requests) != 1 {
				t.Fatalf("process run count mismatch: want 1, got %d", len(runner.requests))
			}
		})
	}
}

func TestConnectRejectsPathArguments(t *testing.T) {
	fixture := newConnectTestFixture(t)
	runner := &fakeConnectProcessRunner{}
	err := runConnectCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, []string{"api"}, fixture.deps(runner))

	if err == nil {
		t.Fatal("expected path argument error")
	}
	if !strings.Contains(err.Error(), "accepts no path arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("process should not start after path argument error, got %d calls", len(runner.requests))
	}
}

func TestConnectValidatesLocalPreconditionsBeforeSSH(t *testing.T) {
	tests := []struct {
		name       string
		mutateCfg  func(*appConfig)
		setup      func(connectTestFixture)
		mutateDeps func(*connectDeps)
		want       string
	}{
		{
			name: "missing configured local root",
			mutateCfg: func(cfg *appConfig) {
				cfg.Projects.LocalRoot = ""
			},
			want: "local project root is not configured",
		},
		{
			name: "missing configured remote root",
			mutateCfg: func(cfg *appConfig) {
				cfg.Projects.RemoteRoot = ""
			},
			want: "remote project root is not configured",
		},
		{
			name: "missing configured SSH identity",
			mutateCfg: func(cfg *appConfig) {
				cfg.SSH.IdentityFile = ""
			},
			want: "SSH identity is not configured",
		},
		{
			name: "missing Personal Server Configuration",
			mutateCfg: func(cfg *appConfig) {
				cfg.PersonalServer = personalServerConfig{}
			},
			want: "Personal Server Configuration is missing",
		},
		{
			name: "missing Personal Server User",
			mutateCfg: func(cfg *appConfig) {
				cfg.PersonalServer.User = ""
			},
			want: "missing Personal Server User",
		},
		{
			name: "missing saved address",
			mutateCfg: func(cfg *appConfig) {
				cfg.PersonalServer.IPv4 = ""
				cfg.PersonalServer.IPv6 = ""
			},
			want: "missing a saved Personal Server address",
		},
		{
			name: "missing local root directory",
			setup: func(fixture connectTestFixture) {
				if err := os.RemoveAll(fixture.localRoot); err != nil {
					t.Fatalf("remove local root: %v", err)
				}
			},
			want: "local project root must be an existing directory",
		},
		{
			name: "missing SSH identity file",
			setup: func(fixture connectTestFixture) {
				if err := os.Remove(fixture.identity.PrivatePath); err != nil {
					t.Fatalf("remove SSH identity: %v", err)
				}
			},
			want: "SSH identity file does not exist",
		},
		{
			name: "non-terminal stdin",
			mutateDeps: func(deps *connectDeps) {
				deps.stdinIsTerminal = func(io.Reader) bool {
					return false
				}
			},
			want: "terminal-backed stdin",
		},
		{
			name: "non-terminal stdout",
			mutateDeps: func(deps *connectDeps) {
				deps.stdoutIsTerminal = func(io.Writer) bool {
					return false
				}
			},
			want: "terminal-backed stdout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newConnectTestFixture(t)
			cfg := fixture.validConfig()
			if tt.mutateCfg != nil {
				tt.mutateCfg(&cfg)
			}
			fixture.saveConfig(t, cfg)
			if tt.setup != nil {
				tt.setup(fixture)
			}

			runner := &fakeConnectProcessRunner{}
			deps := fixture.deps(runner)
			if tt.mutateDeps != nil {
				tt.mutateDeps(&deps)
			}
			err := runConnectCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, nil, deps)

			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: want %q in %q", tt.want, err.Error())
			}
			if len(runner.requests) != 0 {
				t.Fatalf("process should not start after validation error, got %d calls", len(runner.requests))
			}
		})
	}
}

func TestConnectPreservesSSHExitStatus(t *testing.T) {
	fixture := newConnectTestFixture(t)
	runner := &fakeConnectProcessRunner{
		err: commandExitError{code: 77},
	}
	err := runConnectCommand(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, nil, fixture.deps(runner))

	code, ok := CommandExitCode(err)
	if !ok {
		t.Fatalf("expected command exit error, got %v", err)
	}
	if code != 77 {
		t.Fatalf("exit status mismatch: want 77, got %d", code)
	}
}

type connectTestFixture struct {
	home       string
	localRoot  string
	identity   testSSHIdentity
	configPath string
}

func newConnectTestFixture(t *testing.T) connectTestFixture {
	t.Helper()

	home := t.TempDir()
	localRoot := filepath.Join(home, "projects")
	mkdirAll(t, localRoot)
	identity := seedTestSSHIdentity(t, home, ".ssh/id_ed25519", "connect@host", 0o600)
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	if err := saveAppConfig(configPath, appConfig{
		Projects: projectsConfig{
			LocalRoot:  "projects",
			RemoteRoot: "projects",
		},
		SSH: sshConfig{IdentityFile: identity.Relative},
		PersonalServer: personalServerConfig{
			ServerID: 123456,
			User:     "harish",
			IPv4:     "203.0.113.10",
		},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	return connectTestFixture{
		home:       home,
		localRoot:  localRoot,
		identity:   identity,
		configPath: configPath,
	}
}

func (fixture connectTestFixture) validConfig() appConfig {
	return appConfig{
		Projects: projectsConfig{
			LocalRoot:  "projects",
			RemoteRoot: "projects",
		},
		SSH: sshConfig{IdentityFile: fixture.identity.Relative},
		PersonalServer: personalServerConfig{
			ServerID: 123456,
			User:     "harish",
			IPv4:     "203.0.113.10",
		},
	}
}

func (fixture connectTestFixture) saveConfig(t *testing.T, cfg appConfig) {
	t.Helper()

	if err := saveAppConfig(fixture.configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func (fixture connectTestFixture) deps(runner *fakeConnectProcessRunner) connectDeps {
	return connectDeps{
		appConfigPath: func() (string, error) {
			return fixture.configPath, nil
		},
		userHomeDir: func() (string, error) {
			return fixture.home, nil
		},
		workingDir: func() (string, error) {
			return fixture.localRoot, nil
		},
		stdinIsTerminal: func(io.Reader) bool {
			return true
		},
		stdoutIsTerminal: func(io.Writer) bool {
			return true
		},
		runProcess: runner.Run,
	}
}

type fakeConnectProcessRunner struct {
	requests []connectProcessRequest
	err      error
}

func (r *fakeConnectProcessRunner) Run(_ context.Context, req connectProcessRequest) error {
	req.Command = append([]string(nil), req.Command...)
	r.requests = append(r.requests, req)
	return r.err
}
