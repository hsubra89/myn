package cli

import (
	"os/exec"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestRenderPersonalServerBootstrapCloudInit(t *testing.T) {
	const machineAuthKey = "tskey-auth-secret"

	rendered, err := renderPersonalServerBootstrapCloudInit(personalServerBootstrapInput{
		User:                    "harish",
		PasswordHash:            "$6$abcdefghijklmnop$hashed",
		TailscaleHost:           "harish-personal-server",
		TailscaleMachineAuthKey: machineAuthKey,
		RemoteProjectRoot:       "Remote Projects",
		GitIdentity: personalServerGitIdentity{
			Name: "Harish Subramanian",
		},
		ToolPlan: personalServerBootstrapToolPlan{
			HomebrewTools: []string{"tmux", "jq", "git", "gh", "rustup", "go", "nvm"},
			CodingAgents:  []personalServerCodingAgent{personalServerCodingAgentCodex, personalServerCodingAgentClaudeCode},
		},
	})
	if err != nil {
		t.Fatalf("render cloud-init: %v", err)
	}
	if !strings.HasPrefix(rendered, "#cloud-config\n") {
		t.Fatalf("cloud-init should start with #cloud-config, got %q", rendered)
	}

	parsed := parseBootstrapCloudInit(t, rendered)
	if parsed.PackageUpdate || parsed.PackageUpgrade || parsed.PackageRebootIfRequired {
		t.Fatalf("cloud-init package updates should be disabled before Tailscale joins, got %#v", parsed)
	}
	if parsed.DisableRoot {
		t.Fatal("root account should remain available to cloud-init")
	}
	if parsed.SSHPwAuth {
		t.Fatal("password SSH authentication should remain disabled")
	}

	root := parsed.user("root")
	if len(root.SSHAuthorizedKeys) != 0 {
		t.Fatalf("root should not receive authorized SSH keys, got %#v", root.SSHAuthorizedKeys)
	}

	user := parsed.user("harish")
	if got, want := user.Shell, "/bin/bash"; got != want {
		t.Fatalf("user shell mismatch: want %q, got %q", want, got)
	}
	for _, want := range []string{"sudo", "docker"} {
		if !strings.Contains(user.Groups, want) {
			t.Fatalf("user groups should include %q, got %q", want, user.Groups)
		}
	}
	if got, want := user.Sudo, "ALL=(ALL:ALL) ALL"; got != want {
		t.Fatalf("sudo policy mismatch: want %q, got %q", want, got)
	}
	if user.LockPasswd == nil || *user.LockPasswd {
		t.Fatalf("user password should be unlocked for sudo, got %#v", user.LockPasswd)
	}
	if got, want := user.Passwd, "$6$abcdefghijklmnop$hashed"; got != want {
		t.Fatalf("password hash mismatch: want %q, got %q", want, got)
	}
	if len(user.SSHAuthorizedKeys) != 0 {
		t.Fatalf("user should not receive authorized SSH keys, got %#v", user.SSHAuthorizedKeys)
	}

	script := parsed.bootstrapScript()
	if scriptFile := parsed.writeFile("/usr/local/sbin/myn-personal-server-bootstrap.sh"); scriptFile.Permissions != "0755" {
		t.Fatalf("bootstrap script permissions mismatch: want 0755, got %#v", scriptFile)
	}
	authKeyFile := parsed.writeFile("/run/myn/tailscale-auth-key")
	if got, want := authKeyFile.Permissions, "0600"; got != want {
		t.Fatalf("Machine Auth Key file permissions mismatch: want %q, got %#v", want, authKeyFile)
	}
	if got, want := authKeyFile.Owner, "root:root"; got != want {
		t.Fatalf("Machine Auth Key file owner mismatch: want %q, got %#v", want, authKeyFile)
	}
	if got, want := authKeyFile.Content, machineAuthKey+"\n"; got != want {
		t.Fatalf("Machine Auth Key should be passed through root-only file content: want %q, got %q", want, got)
	}
	for _, want := range []string{
		"MYN_REMOTE_PROJECT_ROOT='/home/harish/Remote Projects'",
		"export MYN_USER='harish'",
		"MYN_TAILSCALE_HOST='harish-personal-server'",
		"MYN_TAILSCALE_AUTH_KEY_FILE='/run/myn/tailscale-auth-key'",
		"curl -fsSL \"https://pkgs.tailscale.com/stable/ubuntu/${VERSION_CODENAME}.noarmor.gpg\" -o /usr/share/keyrings/tailscale-archive-keyring.gpg",
		"curl -fsSL \"https://pkgs.tailscale.com/stable/ubuntu/${VERSION_CODENAME}.tailscale-keyring.list\" -o /etc/apt/sources.list.d/tailscale.list",
		"apt-get install -y tailscale",
		"if [ ! -s \"$MYN_TAILSCALE_AUTH_KEY_FILE\" ]; then",
		"fail_tailscale_join \"missing auth key file\"",
		"chmod 0600 \"$MYN_TAILSCALE_AUTH_KEY_FILE\"",
		"tailscale up --auth-key=\"file:$MYN_TAILSCALE_AUTH_KEY_FILE\" --hostname=\"$MYN_TAILSCALE_HOST\" --ssh",
		"rm -f \"$MYN_TAILSCALE_AUTH_KEY_FILE\"",
		"disable_system_openssh",
		"install -d -o \"$MYN_USER\" -g \"$MYN_USER\" \"$MYN_REMOTE_PROJECT_ROOT\"",
		"install -m 0644 -o \"$MYN_USER\" -g \"$MYN_USER\" /dev/null \"/home/$MYN_USER/.tmux.conf\"",
		"cat >\"/home/$MYN_USER/.tmux.conf\" <<'TMUXCONF'\n" + personalServerTmuxProfile + "TMUXCONF\n",
		"chown \"$MYN_USER:$MYN_USER\" \"/home/$MYN_USER/.tmux.conf\"",
		"chmod 0644 \"/home/$MYN_USER/.tmux.conf\"",
		"apt-get install -y unattended-upgrades build-essential procps file git sudo",
		"systemctl enable --now unattended-upgrades",
		"APT::Periodic::Unattended-Upgrade \"1\";",
		"https://download.docker.com/linux/ubuntu",
		"docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin",
		"brew install \"${MYN_HOMEBREW_TOOLS[@]}\"",
		"nvm install --lts",
		"nvm alias default 'lts/*'",
		"npm install -g @openai/codex",
		"curl -fsSL https://claude.ai/install.sh | bash",
		"git config --global user.name 'Harish Subramanian'",
		"MYN_SKIPPED_GIT_IDENTITY=( 'user.email' )",
		"trap mark_failed ERR",
		"write_marker \"failed\"",
		"os.chmod(path, 0o644)",
		"Personal Server Tailscale join failed",
		"Personal Server system OpenSSH disablement failed",
		"disable_systemd_unit_now ssh.socket",
		"disable_systemd_unit_now sshd.socket",
		"disable_systemd_unit_now ssh",
		"systemctl disable --now \"$unit\"",
		"systemctl is-active --quiet ssh.socket",
		"OpenSSH socket is still active",
		"MYN_PARTIAL_FAILURES+=(\"Codex install failed\")",
		"MYN_PARTIAL_FAILURES+=(\"Claude Code install failed\")",
		"\"status\"",
		"\"timestamp\"",
		"\"rebootRequired\"",
		"\"toolVersions\"",
		"myn_user = os.environ.get(\"MYN_USER\", \"\")",
		"\"sudo\", \"-H\", \"-u\", myn_user",
		"\"tailscale\": [\"tailscale\", \"version\"]",
		"\"brew\": user_command([\"/home/linuxbrew/.linuxbrew/bin/brew\", \"--version\"])",
		"\"node\": user_shell(\"source /etc/profile.d/myn-personal-server.sh >/dev/null 2>&1; node --version\")",
		"\"partialFailures\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("bootstrap script should contain %q:\n%s", want, script)
		}
	}

	for _, forbidden := range []string{
		"rustup default",
		"rustup toolchain install",
		"git clone",
		"gh auth login",
		"ssh_authorized_keys",
		"brew install mosh",
		"mosh-server",
		"MYN_SSH_HARDENING_PROFILE",
		"sshd -t",
		"AuthenticationMethods publickey",
		"$6$abcdefghijklmnop$hashed",
		machineAuthKey,
		"--auth-key=" + machineAuthKey,
		"AllowUsers root",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("bootstrap script should not contain %q:\n%s", forbidden, script)
		}
	}
}

func TestRenderPersonalServerBootstrapScriptDisablesOpenSSHSockets(t *testing.T) {
	script := renderPersonalServerBootstrapScript(personalServerBootstrapInput{
		User:                    "harish",
		PasswordHash:            "$6$abcdefghijklmnop$hashed",
		TailscaleHost:           "harish-personal-server",
		TailscaleMachineAuthKey: "tskey-auth-secret",
		RemoteProjectRoot:       "projects",
		ToolPlan:                defaultPersonalServerBootstrapToolPlan(),
	})

	want := `disable_system_openssh() {
  disable_systemd_unit_now ssh.socket
  disable_systemd_unit_now sshd.socket
  disable_systemd_unit_now ssh
  disable_systemd_unit_now sshd
  if systemctl is-active --quiet ssh.socket 2>/dev/null || systemctl is-active --quiet sshd.socket 2>/dev/null; then
    fail_openssh_disable "OpenSSH socket is still active"
  fi
  if systemctl is-active --quiet ssh 2>/dev/null || systemctl is-active --quiet sshd 2>/dev/null; then
    fail_openssh_disable "OpenSSH service is still active"
  fi
}`
	if !strings.Contains(script, want) {
		t.Fatalf("bootstrap script should disable and verify OpenSSH socket units:\n%s", script)
	}
}

func TestRenderPersonalServerBootstrapScriptCleansAuthKeyOnGenericFailure(t *testing.T) {
	script := renderPersonalServerBootstrapScript(personalServerBootstrapInput{
		User:                    "harish",
		PasswordHash:            "$6$abcdefghijklmnop$hashed",
		TailscaleHost:           "harish-personal-server",
		TailscaleMachineAuthKey: "tskey-auth-secret",
		RemoteProjectRoot:       "projects",
		ToolPlan:                defaultPersonalServerBootstrapToolPlan(),
	})

	want := `mark_failed() {
  local status="$?"
  local command="${BASH_COMMAND:-unknown command}"
  trap - ERR
  cleanup_tailscale_auth_key
  write_marker "failed" "$command (exit $status)"
  exit "$status"
}`
	if !strings.Contains(script, want) {
		t.Fatalf("bootstrap script should remove the Machine Auth Key before writing generic failure markers:\n%s", script)
	}
}

func TestRenderPersonalServerBootstrapScriptIsValidBash(t *testing.T) {
	script := renderPersonalServerBootstrapScript(personalServerBootstrapInput{
		User:                    "harish",
		PasswordHash:            "$6$abcdefghijklmnop$hashed",
		TailscaleHost:           "harish-personal-server",
		TailscaleMachineAuthKey: "tskey-auth-secret",
		RemoteProjectRoot:       "projects",
		ToolPlan:                defaultPersonalServerBootstrapToolPlan(),
	})

	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap script should be valid bash: %v\n%s", err, output)
	}
}

type parsedBootstrapCloudInit struct {
	PackageUpdate           bool                 `yaml:"package_update"`
	PackageUpgrade          bool                 `yaml:"package_upgrade"`
	PackageRebootIfRequired bool                 `yaml:"package_reboot_if_required"`
	DisableRoot             bool                 `yaml:"disable_root"`
	SSHPwAuth               bool                 `yaml:"ssh_pwauth"`
	Users                   []bootstrapCloudUser `yaml:"users"`
	WriteFiles              []bootstrapWriteFile `yaml:"write_files"`
}

type bootstrapCloudUser struct {
	Name              string   `yaml:"name"`
	Shell             string   `yaml:"shell"`
	Groups            string   `yaml:"groups"`
	Sudo              string   `yaml:"sudo"`
	LockPasswd        *bool    `yaml:"lock_passwd"`
	Passwd            string   `yaml:"passwd"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
}

type bootstrapWriteFile struct {
	Path        string `yaml:"path"`
	Owner       string `yaml:"owner"`
	Permissions string `yaml:"permissions"`
	Content     string `yaml:"content"`
}

func parseBootstrapCloudInit(t *testing.T, rendered string) parsedBootstrapCloudInit {
	t.Helper()

	var parsed parsedBootstrapCloudInit
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("parse cloud-init YAML: %v\n%s", err, rendered)
	}
	return parsed
}

func (parsed parsedBootstrapCloudInit) user(name string) bootstrapCloudUser {
	for _, user := range parsed.Users {
		if user.Name == name {
			return user
		}
	}
	return bootstrapCloudUser{}
}

func (parsed parsedBootstrapCloudInit) bootstrapScript() string {
	return parsed.writeFile("/usr/local/sbin/myn-personal-server-bootstrap.sh").Content
}

func (parsed parsedBootstrapCloudInit) writeFile(path string) bootstrapWriteFile {
	for _, file := range parsed.WriteFiles {
		if file.Path == path {
			return file
		}
	}
	return bootstrapWriteFile{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
