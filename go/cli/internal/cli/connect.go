package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type connectDeps struct {
	ctx              context.Context
	appConfigPath    func() (string, error)
	loadConfig       func(string) (appConfig, error)
	userHomeDir      func() (string, error)
	workingDir       func() (string, error)
	stat             func(string) (os.FileInfo, error)
	stdinIsTerminal  func(io.Reader) bool
	stdoutIsTerminal func(io.Writer) bool
	runProcess       func(context.Context, connectProcessRequest) error
}

type connectProcessRequest struct {
	Command []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

type connectPlan struct {
	sshUser           string
	sshHost           string
	sshIdentityPath   string
	remotePath        string
	remoteProjectRoot string
	tmuxSessionName   string
}

type projectSession struct {
	number   int
	attached bool
}

type connectProjectSessionMode int

const (
	connectProjectSessionAuto connectProjectSessionMode = iota
	connectProjectSessionAttachExisting
	connectProjectSessionCreateNew
)

func newConnectCommand(deps connectDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "connect [session-number]",
		Aliases: []string{"c"},
		Short:   "Connect to your Personal Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectCommand(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args, deps)
		},
	}
}

func newConnectNewCommand(deps connectDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "connect-new",
		Aliases: []string{"cn"},
		Short:   "Create a new Project Session on your Personal Server",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectNewCommand(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args, deps)
		},
	}
}

func newSessionsCommand(deps connectDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "sessions",
		Aliases: []string{"s", "l"},
		Short:   "List Project Sessions for the current Project",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionsCommand(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args, deps)
		},
	}
}

func runConnectCommand(stdin io.Reader, stdout io.Writer, stderr io.Writer, args []string, deps connectDeps) error {
	sessionNumber, hasSessionNumber, err := parseProjectSessionNumberArg("myn connect", args)
	if err != nil {
		return err
	}
	deps = fillConnectDeps(deps)

	plan, err := planPersonalServerConnectionFromDeps(deps)
	if err != nil {
		return err
	}
	if !deps.stdinIsTerminal(stdin) {
		return fmt.Errorf("myn connect requires terminal-backed stdin")
	}
	if !deps.stdoutIsTerminal(stdout) {
		return fmt.Errorf("myn connect requires terminal-backed stdout")
	}

	mode := connectProjectSessionAuto
	if hasSessionNumber {
		mode = connectProjectSessionAttachExisting
	}
	return deps.runProcess(deps.ctx, connectProcessRequest{
		Command: connectSSHCommand(plan, mode, sessionNumber),
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func runConnectNewCommand(stdin io.Reader, stdout io.Writer, stderr io.Writer, args []string, deps connectDeps) error {
	if len(args) != 0 {
		return fmt.Errorf("myn connect-new accepts no arguments")
	}
	deps = fillConnectDeps(deps)

	plan, err := planPersonalServerConnectionFromDeps(deps)
	if err != nil {
		return err
	}
	if !deps.stdinIsTerminal(stdin) {
		return fmt.Errorf("myn connect-new requires terminal-backed stdin")
	}
	if !deps.stdoutIsTerminal(stdout) {
		return fmt.Errorf("myn connect-new requires terminal-backed stdout")
	}

	return deps.runProcess(deps.ctx, connectProcessRequest{
		Command: connectSSHCommand(plan, connectProjectSessionCreateNew, 0),
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func runSessionsCommand(stdin io.Reader, stdout io.Writer, stderr io.Writer, args []string, deps connectDeps) error {
	if len(args) != 0 {
		return fmt.Errorf("myn sessions accepts no arguments")
	}
	deps = fillConnectDeps(deps)

	plan, err := planPersonalServerConnectionFromDeps(deps)
	if err != nil {
		return err
	}

	var remoteOutput bytes.Buffer
	if err := deps.runProcess(deps.ctx, connectProcessRequest{
		Command: sessionsSSHCommand(plan),
		Stdout:  &remoteOutput,
		Stderr:  stderr,
	}); err != nil {
		return err
	}

	for _, session := range parseProjectSessions(remoteOutput.String(), plan.tmuxSessionName) {
		if session.attached {
			fmt.Fprintf(stdout, "%d  attached\n", session.number)
			continue
		}
		fmt.Fprintf(stdout, "%d\n", session.number)
	}
	return nil
}

func fillConnectDeps(deps connectDeps) connectDeps {
	if deps.ctx == nil {
		deps.ctx = context.Background()
	}
	if deps.appConfigPath == nil {
		env := os.Getenv
		deps.appConfigPath = func() (string, error) {
			return defaultAppConfigPath(env)
		}
	}
	if deps.loadConfig == nil {
		deps.loadConfig = loadAppConfig
	}
	if deps.userHomeDir == nil {
		deps.userHomeDir = os.UserHomeDir
	}
	if deps.workingDir == nil {
		deps.workingDir = os.Getwd
	}
	if deps.stat == nil {
		deps.stat = os.Stat
	}
	if deps.stdinIsTerminal == nil {
		deps.stdinIsTerminal = readerIsTerminal
	}
	if deps.stdoutIsTerminal == nil {
		deps.stdoutIsTerminal = writerIsTerminal
	}
	if deps.runProcess == nil {
		deps.runProcess = runConnectProcess
	}
	return deps
}

func planPersonalServerConnectionFromDeps(deps connectDeps) (connectPlan, error) {
	configPath, err := deps.appConfigPath()
	if err != nil {
		return connectPlan{}, err
	}
	cfg, err := deps.loadConfig(configPath)
	if err != nil {
		return connectPlan{}, err
	}
	home, err := deps.userHomeDir()
	if err != nil {
		return connectPlan{}, fmt.Errorf("find user home directory: %w", err)
	}
	return planPersonalServerConnection(cfg, home, deps)
}

func planPersonalServerConnection(cfg appConfig, home string, deps connectDeps) (connectPlan, error) {
	if strings.TrimSpace(cfg.Projects.LocalRoot) == "" {
		return connectPlan{}, fmt.Errorf("local project root is not configured; run `myn configure`")
	}
	if strings.TrimSpace(cfg.Projects.RemoteRoot) == "" {
		return connectPlan{}, fmt.Errorf("remote project root is not configured; run `myn configure`")
	}
	if strings.TrimSpace(cfg.SSH.IdentityFile) == "" {
		return connectPlan{}, fmt.Errorf("SSH identity is not configured; run `myn configure`")
	}
	connectionState, connectionConfig := cfg.PersonalServer.connectionConfigState()
	if connectionState != personalServerConnectionConfigReady {
		return connectPlan{}, connectionState.validationError()
	}

	localRootPath, err := localProjectRootPath(cfg.Projects.LocalRoot, home)
	if err != nil {
		return connectPlan{}, err
	}
	if err := validateExistingDirectory(deps.stat, localRootPath, "local project root"); err != nil {
		return connectPlan{}, err
	}

	_, identityPath, err := normalizeSSHIdentityFile(cfg.SSH.IdentityFile, home, deps.stat)
	if err != nil {
		return connectPlan{}, err
	}
	if err := validateExistingRegularFile(deps.stat, identityPath, "SSH identity file"); err != nil {
		return connectPlan{}, err
	}

	cwd, err := deps.workingDir()
	if err != nil {
		return connectPlan{}, fmt.Errorf("find current working directory: %w", err)
	}
	localRelativePath, err := localPathRelativeToProjectRoot(localRootPath, cwd)
	if err != nil {
		return connectPlan{}, err
	}

	remoteRoot, err := normalizeRemoteProjectRoot(cfg.Projects.RemoteRoot)
	if err != nil {
		return connectPlan{}, err
	}
	remotePath := remoteRoot
	remoteProjectRoot := remoteRoot
	if localRelativePath != "." {
		remoteRelativePath := filepath.ToSlash(localRelativePath)
		remotePath = path.Join(remoteRoot, remoteRelativePath)
		remoteProjectRoot = path.Join(remoteRoot, strings.Split(remoteRelativePath, "/")[0])
	}

	return connectPlan{
		sshUser:           connectionConfig.User,
		sshHost:           connectionConfig.Host,
		sshIdentityPath:   identityPath,
		remotePath:        remotePath,
		remoteProjectRoot: remoteProjectRoot,
		tmuxSessionName:   connectTmuxSessionName(remoteProjectRoot),
	}, nil
}

func localPathRelativeToProjectRoot(localRootPath string, cwd string) (string, error) {
	localRootPath = filepath.Clean(localRootPath)
	cwd = filepath.Clean(cwd)

	relative, err := filepath.Rel(localRootPath, cwd)
	if err != nil {
		return "", fmt.Errorf("current working directory %q is outside configured local project root %q", cwd, localRootPath)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("current working directory %q is outside configured local project root %q", cwd, localRootPath)
	}

	return relative, nil
}

func validateExistingDirectory(stat func(string) (os.FileInfo, error), value string, name string) error {
	info, err := stat(value)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s must be an existing directory", name)
		}
		return fmt.Errorf("check %s: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must be an existing directory", name)
	}
	return nil
}

func validateExistingRegularFile(stat func(string) (os.FileInfo, error), value string, name string) error {
	info, err := stat(value)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s does not exist", name)
		}
		return fmt.Errorf("check %s: %w", name, err)
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", name)
	}
	return nil
}

func connectSSHCommand(plan connectPlan, mode connectProjectSessionMode, sessionNumber int) []string {
	command := personalServerSSHCommandArgs(
		plan.sshIdentityPath,
		plan.sshUser,
		plan.sshHost,
		"-t",
		"-o", "StrictHostKeyChecking=accept-new",
	)
	return append(command, "bash", "-lc", shellQuote(connectRemoteHandoffCommand(plan, mode, sessionNumber)))
}

func sessionsSSHCommand(plan connectPlan) []string {
	command := personalServerSSHCommandArgs(
		plan.sshIdentityPath,
		plan.sshUser,
		plan.sshHost,
		"-o", "StrictHostKeyChecking=accept-new",
	)
	return append(command, "bash", "-lc", shellQuote(sessionsRemoteListCommand()))
}

func connectRemoteHandoffCommand(plan connectPlan, mode connectProjectSessionMode, sessionNumber int) string {
	switch mode {
	case connectProjectSessionAttachExisting:
		return connectRemoteAttachExistingCommand(plan, sessionNumber)
	case connectProjectSessionCreateNew:
		return connectRemoteCreateNewCommand(plan)
	default:
		return connectRemoteAutoCommand(plan)
	}
}

func connectRemoteAutoCommand(plan connectPlan) string {
	sessionName := plan.tmuxSessionName
	if sessionName == "" {
		sessionName = connectTmuxSessionName(plan.remoteProjectRoot)
	}
	sessionNameArg := shellQuote(sessionName)

	lines := []string{
		connectRemoteRequireTmuxLine(),
		connectRemoteProjectSessionNumberFunction(),
		"project_session_base=" + shellQuote(sessionName),
		"selected_number=0",
		`selected_session=""`,
		`while IFS= read -r session_name; do`,
		`  [ -n "$session_name" ] || continue`,
		`  if session_number="$(project_session_number "$project_session_base" "$session_name")"; then`,
		`    if [ "$selected_number" -eq 0 ] || [ "$session_number" -lt "$selected_number" ]; then`,
		`      selected_number="$session_number"`,
		`      selected_session="$session_name"`,
		`    fi`,
		`  fi`,
		`done < <(tmux list-sessions -F '#{session_name}' 2>/dev/null || true)`,
		`if [ -n "$selected_session" ]; then`,
		`  exec tmux attach-session -t "=$selected_session"`,
		`fi`,
	}
	lines = append(lines, connectRemoteStartDirLines(plan)...)
	lines = append(lines, `exec tmux new-session -s `+sessionNameArg+` -c "$start_dir"`)
	return strings.Join(lines, "\n")
}

func connectRemoteAttachExistingCommand(plan connectPlan, sessionNumber int) string {
	sessionName := plan.tmuxSessionName
	if sessionName == "" {
		sessionName = connectTmuxSessionName(plan.remoteProjectRoot)
	}
	sessionName = projectSessionTmuxSessionName(sessionName, sessionNumber)
	sessionTarget := shellQuote("=" + sessionName)

	return strings.Join([]string{
		connectRemoteRequireTmuxLine(),
		"if tmux has-session -t " + sessionTarget + " 2>/dev/null; then",
		"  exec tmux attach-session -t " + sessionTarget,
		"fi",
		"printf '%s\\n' " + shellQuote(fmt.Sprintf("Project Session %d does not exist; run `myn sessions` to list sessions.", sessionNumber)) + " >&2",
		"exit 1",
	}, "\n")
}

func connectRemoteCreateNewCommand(plan connectPlan) string {
	sessionName := plan.tmuxSessionName
	if sessionName == "" {
		sessionName = connectTmuxSessionName(plan.remoteProjectRoot)
	}

	lines := []string{
		connectRemoteRequireTmuxLine(),
		connectRemoteProjectSessionNumberFunction(),
		"project_session_base=" + shellQuote(sessionName),
		"highest_number=0",
		`while IFS= read -r session_name; do`,
		`  [ -n "$session_name" ] || continue`,
		`  if session_number="$(project_session_number "$project_session_base" "$session_name")"; then`,
		`    if [ "$session_number" -gt "$highest_number" ]; then`,
		`      highest_number="$session_number"`,
		`    fi`,
		`  fi`,
		`done < <(tmux list-sessions -F '#{session_name}' 2>/dev/null || true)`,
		`new_number=$((highest_number + 1))`,
		`if [ "$new_number" -eq 1 ]; then`,
		`  new_session="$project_session_base"`,
		`else`,
		`  new_session="${project_session_base}-${new_number}"`,
		`fi`,
	}
	lines = append(lines, connectRemoteStartDirLines(plan)...)
	lines = append(lines, `exec tmux new-session -s "$new_session" -c "$start_dir"`)
	return strings.Join(lines, "\n")
}

func connectRemoteStartDirLines(plan connectPlan) []string {
	remotePath := remoteHomePathExpression(plan.remotePath)
	remoteProjectRoot := remoteHomePathExpression(plan.remoteProjectRoot)

	lines := []string{
		`start_dir="$HOME"`,
		"if [ -d " + remotePath + " ]; then",
		"  start_dir=" + remotePath,
	}
	if remoteProjectRoot != remotePath {
		lines = append(lines,
			"elif [ -d "+remoteProjectRoot+" ]; then",
			"  start_dir="+remoteProjectRoot,
		)
	}
	lines = append(lines,
		"fi",
	)
	return lines
}

func connectRemoteRequireTmuxLine() string {
	return `command -v tmux >/dev/null 2>&1 || { printf '%s\n' 'tmux is not installed on the Personal Server' >&2; exit 127; }`
}

func connectRemoteProjectSessionNumberFunction() string {
	return strings.Join([]string{
		`project_session_number() {`,
		`  local base="$1"`,
		`  local name="$2"`,
		`  if [ "$name" = "$base" ]; then`,
		`    printf '%s\n' 1`,
		`    return 0`,
		`  fi`,
		`  local prefix="${base}-"`,
		`  local suffix="${name#"$prefix"}"`,
		`  if [ "$suffix" != "$name" ] && [[ "$suffix" =~ ^[0-9]+$ ]] && [[ ! "$suffix" == 0* ]] && [ "$suffix" -gt 1 ]; then`,
		`    printf '%s\n' "$suffix"`,
		`    return 0`,
		`  fi`,
		`  return 1`,
		`}`,
	}, "\n")
}

func sessionsRemoteListCommand() string {
	return strings.Join([]string{
		connectRemoteRequireTmuxLine(),
		`tmux list-sessions -F '#{session_name} #{session_attached}' 2>/dev/null || true`,
	}, "\n")
}

func connectTmuxSessionName(remoteProjectRoot string) string {
	var normalized strings.Builder
	lastWasSeparator := false
	for _, r := range strings.ToLower(remoteProjectRoot) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			normalized.WriteRune(r)
			lastWasSeparator = false
			continue
		}
		if normalized.Len() > 0 && !lastWasSeparator {
			normalized.WriteByte('-')
			lastWasSeparator = true
		}
	}

	value := strings.Trim(normalized.String(), "-")
	if value == "" {
		return "myn-project"
	}
	return "myn-" + value
}

func projectSessionTmuxSessionName(defaultSessionName string, number int) string {
	if number <= 1 {
		return defaultSessionName
	}
	return fmt.Sprintf("%s-%d", defaultSessionName, number)
}

func projectSessionNumberFromTmuxName(defaultSessionName string, name string) (int, bool) {
	if name == defaultSessionName {
		return 1, true
	}
	suffix, ok := strings.CutPrefix(name, defaultSessionName+"-")
	if !ok {
		return 0, false
	}
	number, err := strconv.Atoi(suffix)
	if err != nil || number <= 1 || strconv.Itoa(number) != suffix {
		return 0, false
	}
	return number, true
}

func parseProjectSessionNumberArg(commandName string, args []string) (int, bool, error) {
	if len(args) == 0 {
		return 0, false, nil
	}
	if len(args) > 1 {
		return 0, false, fmt.Errorf("%s accepts at most one Project Session number", commandName)
	}
	number, err := strconv.Atoi(args[0])
	if err != nil || number < 1 {
		return 0, false, fmt.Errorf("Project Session number must be a positive integer")
	}
	return number, true, nil
}

func parseProjectSessions(output string, defaultSessionName string) []projectSession {
	var sessions []projectSession
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		number, ok := projectSessionNumberFromTmuxName(defaultSessionName, fields[0])
		if !ok {
			continue
		}
		sessions = append(sessions, projectSession{
			number:   number,
			attached: fields[1] != "" && fields[1] != "0",
		})
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].number < sessions[j].number
	})
	return sessions
}

func remoteHomePathExpression(remoteRoot string) string {
	remoteRoot = path.Clean(remoteRoot)
	if remoteRoot == "." {
		return `"$HOME"`
	}
	return `"$HOME"/` + shellQuote(remoteRoot)
}

func runConnectProcess(ctx context.Context, req connectProcessRequest) error {
	if len(req.Command) == 0 {
		return fmt.Errorf("missing process command")
	}

	cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)
	cmd.Stdin = req.Stdin
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return commandExitError{code: exitCodeFromExitError(exitErr)}
		}
		return fmt.Errorf("run %s: %w", req.Command[0], err)
	}
	return nil
}
