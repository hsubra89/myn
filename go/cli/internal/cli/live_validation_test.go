package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

const (
	liveValidationEnabledEnv          = "MYN_LIVE_TAILSCALE"
	liveValidationHetznerAPIKeyEnv    = "HETZNER_API_KEY"
	liveValidationTailscaleAPIKeyEnv  = "TAILSCALE_API_TOKEN"
	liveValidationTailscaleTailnetEnv = "TAILSCALE_TAILNET"
	liveValidationLocationEnv         = "MYN_LIVE_HETZNER_LOCATION"
	liveValidationServerTypeEnv       = "MYN_LIVE_HETZNER_SERVER_TYPE"
	liveValidationBootstrapLimit      = 5 * time.Minute
	liveValidationTestTimeout         = 30 * time.Minute
	liveValidationUser                = "melive"
	liveValidationRemoteRoot          = "Remote Projects"
	liveValidationGitName             = "Myn Live Validation"
	liveValidationGitEmail            = "myn-live@example.invalid"
	liveValidationHomebrewIPv4Skip    = "Homebrew tools skipped: IPv4 egress to GitHub/Homebrew infrastructure is unavailable"
)

type liveValidationCredentials struct {
	HetznerAPIKey    string
	TailscaleAPIKey  string
	TailscaleTailnet string
}

func TestLoadLiveValidationEnvFileReadsCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(path, []byte(`
# local live validation secrets
IGNORED=value
HETZNER_API_KEY=' live-token '
TAILSCALE_API_TOKEN=' ts-token '
TAILSCALE_TAILNET=' example.com '
`), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := loadLiveValidationEnvFile(path)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	credentials, err := liveValidationCredentialsFromEnv(env)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if credentials.HetznerAPIKey != "live-token" {
		t.Fatalf("Hetzner token mismatch: want %q, got %q", "live-token", credentials.HetznerAPIKey)
	}
	if credentials.TailscaleAPIKey != "ts-token" {
		t.Fatalf("Tailscale token mismatch: want %q, got %q", "ts-token", credentials.TailscaleAPIKey)
	}
	if credentials.TailscaleTailnet != "example.com" {
		t.Fatalf("Tailscale tailnet mismatch: want %q, got %q", "example.com", credentials.TailscaleTailnet)
	}
}

func TestLoadLiveValidationEnvFileReportsMissingFile(t *testing.T) {
	_, err := loadLiveValidationEnvFile(filepath.Join(t.TempDir(), ".env.local"))
	if err == nil {
		t.Fatal("expected missing .env.local error")
	}
	if !strings.Contains(err.Error(), "live validation requires .env.local") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveValidationCredentialsReportsMissingHetznerToken(t *testing.T) {
	_, err := liveValidationCredentialsFromEnv(map[string]string{
		liveValidationTailscaleAPIKeyEnv:  "ts-token",
		liveValidationTailscaleTailnetEnv: "example.com",
	})
	if err == nil {
		t.Fatal("expected missing HETZNER_API_KEY error")
	}
	if !strings.Contains(err.Error(), "HETZNER_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveValidationCredentialsReportsMissingTailscaleCredentials(t *testing.T) {
	_, err := liveValidationCredentialsFromEnv(map[string]string{
		liveValidationHetznerAPIKeyEnv: "hetzner-token",
	})
	if err == nil {
		t.Fatal("expected missing Tailscale credential error")
	}
	if !strings.Contains(err.Error(), "TAILSCALE_API_TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadLiveValidationCredentialsFromRepoRootReportsMissingCredential(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".env.local"), []byte(`
HETZNER_API_KEY=hetzner-token
TAILSCALE_TAILNET=example.com
`), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	_, err := loadLiveValidationCredentialsFromRepoRoot(repoRoot)
	if err == nil {
		t.Fatal("expected missing TAILSCALE_API_TOKEN error")
	}
	if !strings.Contains(err.Error(), "TAILSCALE_API_TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLivePersonalServerProvisioning(t *testing.T) {
	if os.Getenv(liveValidationEnabledEnv) != "1" {
		t.Skipf("set %s=1 to run live Tailscale-only Personal Server validation", liveValidationEnabledEnv)
	}

	repoRoot, err := findLiveValidationRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := loadLiveValidationCredentialsFromRepoRoot(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	env, err := loadLiveValidationEnvFile(filepath.Join(repoRoot, ".env.local"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), liveValidationTestTimeout)
	defer cancel()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, liveValidationRemoteRoot), 0o700); err != nil {
		t.Fatalf("create isolated local root: %v", err)
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")

	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	serverName := liveValidationServerName(t)
	client := liveValidationHcloudClient(credentials.HetznerAPIKey)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cleanupCancel()
		cleanupLiveValidationServer(cleanupCtx, t, client, configPath, serverName)
	})

	var authOut bytes.Buffer
	if err := runHetznerAuth(ctx, &authOut, hetznerAuthOptions{token: credentials.HetznerAPIKey}, hetznerAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: func(string) string {
			return ""
		},
		validateToken: newHetznerValidator("").validate,
	}); err != nil {
		t.Fatalf("save isolated Hetzner Credentials: %v", err)
	}
	if strings.Contains(authOut.String(), credentials.HetznerAPIKey) {
		t.Fatal("Hetzner API key was printed during live auth")
	}

	var tailscaleAuthOut bytes.Buffer
	if err := runTailscaleAuth(ctx, &tailscaleAuthOut, tailscaleAuthOptions{
		token:   credentials.TailscaleAPIKey,
		tailnet: credentials.TailscaleTailnet,
	}, tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: func(string) string {
			return ""
		},
		validateCredentials: newTailscaleCloudValidator("").validate,
	}); err != nil {
		t.Fatalf("save isolated Tailscale Credentials: %v", err)
	}
	if strings.Contains(tailscaleAuthOut.String(), credentials.TailscaleAPIKey) {
		t.Fatal("Tailscale API key was printed during live auth")
	}

	prompter := &liveValidationConfigurePrompter{
		locationName:   strings.TrimSpace(env[liveValidationLocationEnv]),
		serverTypeName: strings.TrimSpace(env[liveValidationServerTypeEnv]),
		serverName:     serverName,
		password:       "myn-live-validation-password",
	}
	var out bytes.Buffer
	err = runConfigure(&out, configureOptions{
		localRoot:     liveValidationRemoteRoot,
		localRootSet:  true,
		remoteRoot:    liveValidationRemoteRoot,
		remoteRootSet: true,
	}, configureDeps{
		ctx: ctx,
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		userHomeDir: func() (string, error) {
			return home, nil
		},
		prompter: prompter,
		personalServerProvisioner: personalServerProvisioningGate{
			newCloudClient: func(gotToken string) personalServerCloudClient {
				if gotToken != credentials.HetznerAPIKey {
					t.Fatalf("live configure used unexpected Hetzner token")
				}
				return newHcloudPersonalServerCloudClient(gotToken, "")
			},
			userHomeDir: func() (string, error) {
				return home, nil
			},
			tailnetPolicyEnabled: true,
			openURL: func(string) error {
				return nil
			},
			runTailscaleSSHCheck: liveValidationTailscaleSSHRunner(knownHostsPath),
			bootstrapTimeout:     liveValidationBootstrapLimit,
			currentUsername: func() string {
				return liveValidationUser
			},
			gitConfigValue: func(_ personalServerGitConfigScope, key string) (string, bool) {
				switch key {
				case "user.name":
					return liveValidationGitName, true
				case "user.email":
					return liveValidationGitEmail, true
				default:
					return "", false
				}
			},
		},
	})
	if strings.Contains(out.String(), credentials.HetznerAPIKey) {
		t.Fatal("Hetzner API key was printed during live configure")
	}
	if strings.Contains(out.String(), credentials.TailscaleAPIKey) {
		t.Fatal("Tailscale API key was printed during live configure")
	}

	cfg, loadErr := loadAppConfig(configPath)
	if loadErr != nil {
		t.Fatalf("load isolated config: %v", loadErr)
	}
	if err != nil {
		t.Fatalf("live configure failed: %v\n%s", err, out.String())
	}
	assertLiveValidationConfig(t, cfg)

	server, _, err := client.Server.GetByID(ctx, int64(cfg.PersonalServer.ServerID))
	if err != nil {
		t.Fatalf("load live validation server: %v", err)
	}
	if server == nil {
		t.Fatalf("live validation server %d was not found after configure", cfg.PersonalServer.ServerID)
	}
	assertLiveValidationServer(t, server, cfg, serverName, prompter.selectedLocationName, prompter.selectedServerTypeName)
	assertLiveValidationFirewall(t, ctx, client)
	assertLiveValidationTailscaleDevice(t, ctx, cfg)
	marker := assertLiveValidationBootstrap(t, ctx, knownHostsPath, cfg.PersonalServer.TailscaleHost)
	assertLiveValidationRemoteSetup(t, ctx, knownHostsPath, cfg.PersonalServer.TailscaleHost, marker)
}

func loadLiveValidationCredentialsFromRepoRoot(repoRoot string) (liveValidationCredentials, error) {
	env, err := loadLiveValidationEnvFile(filepath.Join(repoRoot, ".env.local"))
	if err != nil {
		return liveValidationCredentials{}, err
	}
	return liveValidationCredentialsFromEnv(env)
}

func loadLiveValidationEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("live validation requires .env.local at %s with %s, %s, and %s", path, liveValidationHetznerAPIKeyEnv, liveValidationTailscaleAPIKeyEnv, liveValidationTailscaleTailnetEnv)
	}
	if err != nil {
		return nil, fmt.Errorf("read live validation .env.local: %w", err)
	}

	env := make(map[string]string)
	for lineNumber, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse live validation .env.local line %d: expected KEY=VALUE", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("parse live validation .env.local line %d: key is empty", lineNumber+1)
		}
		if len(value) >= 2 {
			quote := value[0]
			if (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
				value = value[1 : len(value)-1]
			}
		}
		env[key] = value
	}
	return env, nil
}

func liveValidationCredentialsFromEnv(env map[string]string) (liveValidationCredentials, error) {
	credentials := liveValidationCredentials{
		HetznerAPIKey:    strings.TrimSpace(env[liveValidationHetznerAPIKeyEnv]),
		TailscaleAPIKey:  strings.TrimSpace(env[liveValidationTailscaleAPIKeyEnv]),
		TailscaleTailnet: strings.TrimSpace(env[liveValidationTailscaleTailnetEnv]),
	}
	switch {
	case credentials.HetznerAPIKey == "":
		return liveValidationCredentials{}, fmt.Errorf("live validation requires %s in .env.local", liveValidationHetznerAPIKeyEnv)
	case credentials.TailscaleAPIKey == "":
		return liveValidationCredentials{}, fmt.Errorf("live validation requires %s in .env.local", liveValidationTailscaleAPIKeyEnv)
	case credentials.TailscaleTailnet == "":
		return liveValidationCredentials{}, fmt.Errorf("live validation requires %s in .env.local", liveValidationTailscaleTailnetEnv)
	default:
		return credentials, nil
	}
}

func findLiveValidationRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find working directory: %w", err)
	}
	for {
		if fileExists(filepath.Join(dir, "prd.md")) && fileExists(filepath.Join(dir, "go", "cli", "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func liveValidationServerName(t *testing.T) string {
	t.Helper()
	var randomBytes [3]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatalf("generate live validation server suffix: %v", err)
	}
	return fmt.Sprintf("myn-live-%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(randomBytes[:]))
}

func liveValidationHcloudClient(token string) *hcloud.Client {
	return hcloud.NewClient(
		hcloud.WithToken(token),
		hcloud.WithPollInterval(5*time.Second),
	)
}

func liveValidationTailscaleSSHRunner(knownHostsPath string) personalServerTailscaleSSHCheckRunner {
	return func(ctx context.Context, out io.Writer, user string, host string, command string) (string, error) {
		if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0o700); err != nil {
			return "", fmt.Errorf("create isolated SSH known_hosts directory: %w", err)
		}
		args := personalServerSSHCommandArgs(user, host,
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "UserKnownHostsFile="+knownHostsPath,
			"-o", "ConnectTimeout=10",
		)
		args = append(args, command)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		return runPersonalServerTailscaleSSHCommand(cmd, out)
	}
}

func TestLiveValidationTailscaleSSHRunnerReturnsOnlyStdout(t *testing.T) {
	prependFakeSSHToPath(t, `#!/bin/sh
printf '%s\n' '{"status":"success","timestamp":"2026-05-10T12:00:00Z"}'
printf '%s\n' 'Tailscale SSH banner on stderr' >&2
`)

	var out bytes.Buffer
	output, err := liveValidationTailscaleSSHRunner(filepath.Join(t.TempDir(), "known_hosts"))(
		context.Background(),
		&out,
		"harish",
		"harish-personal-server",
		"cat /var/lib/myn/personal-server-bootstrap.json",
	)
	if err != nil {
		t.Fatalf("run live validation ssh: %v", err)
	}
	if got, want := strings.TrimSpace(output), `{"status":"success","timestamp":"2026-05-10T12:00:00Z"}`; got != want {
		t.Fatalf("stdout mismatch: want %q, got %q", want, got)
	}
	if strings.Contains(output, "Tailscale SSH banner") {
		t.Fatalf("stderr should not be returned as marker output, got %q", output)
	}
	if !strings.Contains(out.String(), "Tailscale SSH banner") {
		t.Fatalf("stderr should be surfaced to the user, got %q", out.String())
	}
}

func TestAssertLiveValidationBootstrapAllowsHomebrewIPv4PartialFailure(t *testing.T) {
	marker := `{
  "status": "success",
  "timestamp": "2026-05-24T12:00:00Z",
  "toolVersions": {
    "tailscale": "1.82.0",
    "docker": "Docker version 26.1.4",
    "dockerCompose": "Docker Compose version v2.27.1"
  },
  "partialFailures": ["Homebrew tools skipped: IPv4 egress to GitHub/Homebrew infrastructure is unavailable"]
}`
	prependFakeSSHToPath(t, "#!/bin/sh\nprintf '%s\\n' "+shellQuote(marker)+"\n")

	assertLiveValidationBootstrap(t, context.Background(), filepath.Join(t.TempDir(), "known_hosts"), "harish-personal-server")
}

func TestValidateLiveValidationBootstrapMarkerRequiresHomebrewToolsWithoutIPv4PartialFailure(t *testing.T) {
	err := validateLiveValidationBootstrapMarker(personalServerBootstrapMarker{
		Status: "success",
		ToolVersions: map[string]string{
			"tailscale":     "1.82.0",
			"docker":        "Docker version 26.1.4",
			"dockerCompose": "Docker Compose version v2.27.1",
		},
	})
	if err == nil {
		t.Fatal("expected missing Homebrew-managed tool error")
	}
	if !strings.Contains(err.Error(), "missing brew version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertLiveValidationRemoteSetupSkipsNodeChecksWhenHomebrewWasSkipped(t *testing.T) {
	prependFakeSSHToPath(t, `#!/bin/sh
last=
for arg in "$@"; do
  last="$arg"
done

case "$last" in
  true)
    ;;
  "id -un")
    printf '%s\n' 'melive'
    ;;
  getent\ passwd*)
    printf '%s\n' 'melive:/bin/bash'
    ;;
  id\ -nG*)
    printf '%s\n' 'melive sudo docker'
    ;;
  stat\ -c*)
    printf '%s\n' 'melive:melive:directory'
    ;;
  "docker --version && docker compose version")
    printf '%s\n' 'Docker version 26.1.4'
    printf '%s\n' 'Docker Compose version v2.27.1'
    ;;
  *"nvm version default"*)
    printf '%s\n' 'unexpected nvm check' >&2
    exit 40
    ;;
  *"/home/linuxbrew/.linuxbrew/bin/git config"*)
    printf '%s\n' 'unexpected Homebrew git check' >&2
    exit 41
    ;;
  "git config --global user.name")
    printf '%s\n' 'Myn Live Validation'
    ;;
  "git config --global user.email")
    printf '%s\n' 'myn-live@example.invalid'
    ;;
  systemctl\ is-active*)
    printf '%s\n' 'inactive inactive inactive inactive'
    ;;
  "command -v mosh || true")
    ;;
  *)
    printf 'unexpected command: %s\n' "$last" >&2
    exit 42
    ;;
esac
`)

	assertLiveValidationRemoteSetup(t, context.Background(), filepath.Join(t.TempDir(), "known_hosts"), "harish-personal-server", personalServerBootstrapMarker{
		Status:          "success",
		PartialFailures: []string{liveValidationHomebrewIPv4Skip},
	})
}

func TestAssertLiveValidationRemoteSetupChecksNodeWhenHomebrewWasNotSkipped(t *testing.T) {
	sawNVMCheckPath := filepath.Join(t.TempDir(), "saw-nvm-check")
	t.Setenv("SAW_NVM_CHECK", sawNVMCheckPath)
	prependFakeSSHToPath(t, `#!/bin/sh
last=
for arg in "$@"; do
  last="$arg"
done

case "$last" in
  true)
    ;;
  "id -un")
    printf '%s\n' 'melive'
    ;;
  getent\ passwd*)
    printf '%s\n' 'melive:/bin/bash'
    ;;
  id\ -nG*)
    printf '%s\n' 'melive sudo docker'
    ;;
  stat\ -c*)
    printf '%s\n' 'melive:melive:directory'
    ;;
  "docker --version && docker compose version")
    printf '%s\n' 'Docker version 26.1.4'
    printf '%s\n' 'Docker Compose version v2.27.1'
    ;;
  *"nvm version default"*)
    printf '%s\n' 'checked' >"$SAW_NVM_CHECK"
    printf '%s\n' 'v20.12.2 v20.12.2'
    ;;
  "/home/linuxbrew/.linuxbrew/bin/git config --global user.name")
    printf '%s\n' 'Myn Live Validation'
    ;;
  "/home/linuxbrew/.linuxbrew/bin/git config --global user.email")
    printf '%s\n' 'myn-live@example.invalid'
    ;;
  systemctl\ is-active*)
    printf '%s\n' 'inactive inactive inactive inactive'
    ;;
  "command -v mosh || true")
    ;;
  *)
    printf 'unexpected command: %s\n' "$last" >&2
    exit 42
    ;;
esac
`)

	assertLiveValidationRemoteSetup(t, context.Background(), filepath.Join(t.TempDir(), "known_hosts"), "harish-personal-server", personalServerBootstrapMarker{
		Status: "success",
	})
	if !fileExists(sawNVMCheckPath) {
		t.Fatal("expected live validation to check nvm/Node when Homebrew was not skipped")
	}
}

type liveValidationConfigurePrompter struct {
	locationName           string
	serverTypeName         string
	serverName             string
	password               string
	selectedLocationName   string
	selectedServerTypeName string
}

func (p *liveValidationConfigurePrompter) CanPrompt() bool {
	return true
}

func (p *liveValidationConfigurePrompter) Confirm(title string, affirmative bool) (bool, error) {
	switch title {
	case "Allow Myn to edit Tailnet Policy?", "Create Personal Server?":
		return true, nil
	default:
		return affirmative, nil
	}
}

func (p *liveValidationConfigurePrompter) Input(title string, defaultValue string, validate func(string) error) (string, error) {
	value := defaultValue
	switch title {
	case "Personal Server User":
		value = liveValidationUser
	case "Personal Server name":
		value = p.serverName
	}
	if validate != nil {
		if err := validate(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

func (p *liveValidationConfigurePrompter) Password(string) (string, error) {
	return p.password, nil
}

func (p *liveValidationConfigurePrompter) SelectSSHIdentity(choices []sshIdentityPromptChoice, selected int) (sshIdentityPromptChoice, error) {
	if selected < 0 || selected >= len(choices) {
		return sshIdentityPromptChoice{}, fmt.Errorf("SSH selection index out of range")
	}
	return choices[selected], nil
}

func (p *liveValidationConfigurePrompter) SelectPersonalServerLocation(choices []personalServerLocationChoice, selected int) (personalServerLocationChoice, error) {
	index := selected
	if p.locationName != "" {
		index = -1
		for i, choice := range choices {
			if choice.Location.Name == p.locationName {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(choices) {
		return personalServerLocationChoice{}, fmt.Errorf("live validation Location %q was not offered", p.locationName)
	}
	p.selectedLocationName = choices[index].Location.Name
	return choices[index], nil
}

func (p *liveValidationConfigurePrompter) SelectPersonalServerType(choices []personalServerTypeChoice, selected int) (personalServerTypeChoice, error) {
	index := selected
	if p.serverTypeName != "" {
		index = -1
		for i, choice := range choices {
			if choice.ServerType.Name == p.serverTypeName {
				index = i
				break
			}
		}
	} else if len(choices) > 0 {
		index = cheapestLiveValidationServerTypeChoice(choices, p.selectedLocationName)
	}
	if index < 0 || index >= len(choices) {
		return personalServerTypeChoice{}, fmt.Errorf("live validation Server Type %q was not offered", p.serverTypeName)
	}
	p.selectedServerTypeName = choices[index].ServerType.Name
	return choices[index], nil
}

func cheapestLiveValidationServerTypeChoice(choices []personalServerTypeChoice, locationName string) int {
	selected := 0
	selectedPrice, selectedPriced := personalServerTypeMonthlyGross(choices[selected].ServerType, locationName)
	for index := 1; index < len(choices); index++ {
		price, priced := personalServerTypeMonthlyGross(choices[index].ServerType, locationName)
		switch {
		case priced && !selectedPriced:
			selected = index
			selectedPrice = price
			selectedPriced = true
		case priced && selectedPriced && price < selectedPrice:
			selected = index
			selectedPrice = price
		case priced == selectedPriced && price == selectedPrice && choices[index].ServerType.Name < choices[selected].ServerType.Name:
			selected = index
		}
	}
	return selected
}

func cleanupLiveValidationServer(ctx context.Context, t *testing.T, client *hcloud.Client, configPath string, serverName string) {
	t.Helper()
	serverIDs := make(map[int64]struct{})
	if cfg, err := loadAppConfig(configPath); err == nil && cfg.PersonalServer.ServerID != 0 {
		serverIDs[int64(cfg.PersonalServer.ServerID)] = struct{}{}
	}
	if strings.TrimSpace(serverName) != "" {
		server, _, err := client.Server.GetByName(ctx, serverName)
		if err != nil {
			t.Errorf("find live validation server by name for cleanup: %v", err)
		} else if server != nil {
			serverIDs[server.ID] = struct{}{}
		}
	}

	for id := range serverIDs {
		server, _, err := client.Server.GetByID(ctx, id)
		if err != nil {
			t.Errorf("find live validation server %d for cleanup: %v", id, err)
			continue
		}
		if server == nil {
			continue
		}
		result, _, err := client.Server.DeleteWithResult(ctx, server)
		if err != nil {
			t.Errorf("delete live validation server %d: %v", id, err)
			continue
		}
		if result != nil && result.Action != nil {
			if err := client.Action.WaitFor(ctx, result.Action); err != nil {
				t.Errorf("wait for live validation server %d deletion: %v", id, err)
				continue
			}
		}
		t.Logf("deleted live validation server %d (%s)", id, server.Name)
	}
}

func assertLiveValidationConfig(t *testing.T, cfg appConfig) {
	t.Helper()
	if cfg.PersonalServer.ServerID == 0 {
		t.Fatal("live validation did not save Personal Server serverID")
	}
	if got := strings.TrimSpace(cfg.PersonalServer.User); got != liveValidationUser {
		t.Fatalf("live validation Personal Server User mismatch: want %q, got %q", liveValidationUser, got)
	}
	if strings.TrimSpace(cfg.PersonalServer.TailscaleHost) == "" {
		t.Fatal("live validation did not save Personal Server Tailscale Host")
	}
	if strings.TrimSpace(cfg.PersonalServer.IPv4) != "" {
		t.Fatalf("live validation saved unsupported public IPv4 access path: %q", cfg.PersonalServer.IPv4)
	}
	if strings.TrimSpace(cfg.PersonalServer.IPv6) == "" {
		t.Fatal("live validation did not save Personal Server IPv6")
	}
}

func assertLiveValidationServer(t *testing.T, server *hcloud.Server, cfg appConfig, serverName string, locationName string, serverTypeName string) {
	t.Helper()
	if server.Name != serverName {
		t.Fatalf("live validation server name mismatch: want %q, got %q", serverName, server.Name)
	}
	if locationName != "" && (server.Location == nil || server.Location.Name != locationName) {
		t.Fatalf("live validation Location mismatch: want %q, got %#v", locationName, server.Location)
	}
	if serverTypeName != "" && (server.ServerType == nil || server.ServerType.Name != serverTypeName) {
		t.Fatalf("live validation Server Type mismatch: want %q, got %#v", serverTypeName, server.ServerType)
	}
	assertLiveValidationLabels(t, "server", server.Labels)
	if got := strings.TrimSpace(server.Labels["tailscale_host"]); got != cfg.PersonalServer.TailscaleHost {
		t.Fatalf("live server tailscale_host label mismatch: want %q, got %q", cfg.PersonalServer.TailscaleHost, got)
	}
	if !server.PublicNet.IPv4.IsUnspecified() {
		t.Fatalf("live validation server should have public IPv4 disabled, got %q", server.PublicNet.IPv4.IP.String())
	}
	if server.PublicNet.IPv6.IsUnspecified() || server.PublicNet.IPv6.IP.String() != cfg.PersonalServer.IPv6 {
		t.Fatalf("live validation server IPv6 mismatch: config=%q live=%q", cfg.PersonalServer.IPv6, server.PublicNet.IPv6.IP.String())
	}
}

func assertLiveValidationFirewall(t *testing.T, ctx context.Context, client *hcloud.Client) {
	t.Helper()
	after, _, err := client.Firewall.GetByName(ctx, personalServerFirewallName)
	if err != nil {
		t.Fatalf("load live Personal Server Firewall after configure: %v", err)
	}
	if after == nil {
		t.Fatal("live Personal Server Firewall was not found after configure")
	}
	assertLiveValidationLabels(t, "firewall", after.Labels)
	if got, want := firewallRuleSummaries(after.Rules), []string{}; !equalStringSlices(got, want) {
		t.Fatalf("new live Personal Server Firewall rules mismatch: want %v, got %v", want, got)
	}
	t.Logf("Personal Server Firewall %d has no public inbound rules", after.ID)
}

func assertLiveValidationTailscaleDevice(t *testing.T, ctx context.Context, cfg appConfig) {
	t.Helper()
	client, err := newTailscaleAPIClient("", nil, cfg.Auth.Tailscale.Token, cfg.Auth.Tailscale.Tailnet)
	if err != nil {
		t.Fatalf("create live Tailscale client: %v", err)
	}
	deviceClient := personalServerTailscaleAPIClient{client: client}
	device, found, err := (personalServerProvisioningGate{}).findPersonalServerTailscaleDevice(ctx, deviceClient, cfg.PersonalServer.TailscaleHost)
	if err != nil {
		t.Fatalf("find live Personal Server Tailscale device: %v", err)
	}
	if !found {
		t.Fatalf("live Personal Server Tailscale device %q was not found", cfg.PersonalServer.TailscaleHost)
	}
	if err := personalServerTailscaleDeviceReadyError(device, cfg.PersonalServer.TailscaleHost); err != nil {
		t.Fatalf("live Personal Server Tailscale device is not ready: %v", err)
	}
	t.Logf("Tailscale device %q is tagged, authorized, and online", cfg.PersonalServer.TailscaleHost)
}

func assertLiveValidationBootstrap(t *testing.T, ctx context.Context, knownHostsPath string, host string) personalServerBootstrapMarker {
	t.Helper()
	markerOutput := liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "cat "+personalServerBootstrapMarkerPath)
	marker, err := parsePersonalServerBootstrapMarker(markerOutput)
	if err != nil {
		t.Fatalf("parse live Personal Server Bootstrap marker: %v", err)
	}
	if err := validateLiveValidationBootstrapMarker(marker); err != nil {
		t.Fatal(err)
	}

	logLiveValidationOptionalAgentVersions(t, marker)
	if len(marker.PartialFailures) > 0 {
		t.Logf("live Personal Server Bootstrap reported partial failures: %v", marker.PartialFailures)
	}
	return marker
}

func validateLiveValidationBootstrapMarker(marker personalServerBootstrapMarker) error {
	if strings.ToLower(marker.Status) != "success" {
		return fmt.Errorf("live Personal Server Bootstrap marker was not successful: %#v", marker)
	}
	requiredTools := []string{"tailscale", "docker", "dockerCompose"}
	if !liveValidationMarkerHasPartialFailure(marker, liveValidationHomebrewIPv4Skip) {
		requiredTools = append(requiredTools, "brew", "tmux", "jq", "git", "gh", "rustup", "go", "nvm", "node", "npm")
	}
	for _, tool := range requiredTools {
		if strings.TrimSpace(marker.ToolVersions[tool]) == "" {
			return fmt.Errorf("live Personal Server Bootstrap marker is missing %s version: %#v", tool, marker.ToolVersions)
		}
	}
	return nil
}

func logLiveValidationOptionalAgentVersions(t *testing.T, marker personalServerBootstrapMarker) {
	t.Helper()
	for _, optionalAgent := range []string{"codex", "claude"} {
		if strings.TrimSpace(marker.ToolVersions[optionalAgent]) == "" {
			t.Logf("live Personal Server Bootstrap marker has no %s version; partial failures: %v", optionalAgent, marker.PartialFailures)
		}
	}
}

func liveValidationMarkerHasPartialFailure(marker personalServerBootstrapMarker, failure string) bool {
	for _, partialFailure := range marker.PartialFailures {
		if strings.TrimSpace(partialFailure) == failure {
			return true
		}
	}
	return false
}

func assertLiveValidationRemoteSetup(t *testing.T, ctx context.Context, knownHostsPath string, host string, marker personalServerBootstrapMarker) {
	t.Helper()
	homebrewSkipped := liveValidationMarkerHasPartialFailure(marker, liveValidationHomebrewIPv4Skip)

	liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "true")
	if got := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "id -un")); got != liveValidationUser {
		t.Fatalf("live Personal Server User SSH mismatch: want %q, got %q", liveValidationUser, got)
	}

	passwd := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "getent passwd "+shellQuote(liveValidationUser)+" | cut -d: -f1,7"))
	if passwd != liveValidationUser+":/bin/bash" {
		t.Fatalf("live Personal Server User passwd entry mismatch: %q", passwd)
	}
	groups := strings.Fields(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "id -nG "+shellQuote(liveValidationUser)))
	for _, group := range []string{"sudo", "docker"} {
		if !containsString(groups, group) {
			t.Fatalf("live Personal Server User missing %s group: %v", group, groups)
		}
	}

	remoteRoot := "/home/" + liveValidationUser + "/" + liveValidationRemoteRoot
	statCommand := "stat -c '%U:%G:%F' " + shellQuote(remoteRoot)
	if got := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, statCommand)); got != liveValidationUser+":"+liveValidationUser+":directory" {
		t.Fatalf("live remote project root mismatch: %q", got)
	}

	liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "docker --version && docker compose version")
	if !homebrewSkipped {
		nvmOutput := strings.Fields(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "source /etc/profile.d/myn-personal-server.sh && nvm version default && node --version"))
		if len(nvmOutput) < 2 || nvmOutput[0] == "N/A" || nvmOutput[0] != nvmOutput[1] {
			t.Fatalf("live nvm default mismatch: %v", nvmOutput)
		}
	} else {
		t.Logf("skipping live nvm/Node checks because bootstrap reported: %s", liveValidationHomebrewIPv4Skip)
	}

	gitCommand := "/home/linuxbrew/.linuxbrew/bin/git"
	if homebrewSkipped {
		gitCommand = "git"
	}
	if got := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, gitCommand+" config --global user.name")); got != liveValidationGitName {
		t.Fatalf("live Git user.name mismatch: want %q, got %q", liveValidationGitName, got)
	}
	if got := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, gitCommand+" config --global user.email")); got != liveValidationGitEmail {
		t.Fatalf("live Git user.email mismatch: want %q, got %q", liveValidationGitEmail, got)
	}
	if got := strings.Fields(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "systemctl is-active ssh sshd ssh.socket sshd.socket 2>/dev/null || true")); containsString(got, "active") {
		t.Fatalf("system OpenSSH should be disabled, active units: %v", got)
	}
	if got := strings.TrimSpace(liveValidationSSH(t, ctx, knownHostsPath, liveValidationUser, host, "command -v mosh || true")); got != "" {
		t.Fatalf("Mosh should not be installed, found %q", got)
	}
}

func liveValidationSSH(t *testing.T, ctx context.Context, knownHostsPath string, user string, host string, command string) string {
	t.Helper()
	output, err := liveValidationTailscaleSSHRunner(knownHostsPath)(ctx, io.Discard, user, host, command)
	if err != nil {
		t.Fatalf("run live SSH command as %s: %v", user, err)
	}
	return output
}

func assertLiveValidationLabels(t *testing.T, resource string, labels map[string]string) {
	t.Helper()
	for key, value := range personalServerResourceLabels() {
		if labels[key] != value {
			t.Fatalf("live %s label %s mismatch: want %q, got %q", resource, key, value, labels[key])
		}
	}
}

func sameFirewallRules(left []hcloud.FirewallRule, right []hcloud.FirewallRule) bool {
	return equalStringSlices(firewallRuleSummaries(left), firewallRuleSummaries(right))
}

func firewallRuleSummaries(rules []hcloud.FirewallRule) []string {
	summaries := make([]string, 0, len(rules))
	for _, rule := range rules {
		sources := make([]string, 0, len(rule.SourceIPs))
		for _, source := range rule.SourceIPs {
			sources = append(sources, source.String())
		}
		sort.Strings(sources)
		summaries = append(summaries, fmt.Sprintf("%s %s %s %s", rule.Direction, rule.Protocol, stringValue(rule.Port), strings.Join(sources, ",")))
	}
	sort.Strings(summaries)
	return summaries
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
