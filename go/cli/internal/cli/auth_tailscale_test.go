package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestAuthTailscaleEnvValidatesAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "myn", "config.json")
	t.Setenv("TAILSCALE_API_TOKEN", "env-token")
	t.Setenv("TAILSCALE_TAILNET", "tailnet-123")

	var validated []tailscaleCredentials
	deps := tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: os.Getenv,
		validateCredentials: func(_ context.Context, credentials tailscaleCredentials) error {
			validated = append(validated, credentials)
			if credentials.Token != "env-token" || credentials.Tailnet != "tailnet-123" {
				return fmt.Errorf("unexpected credentials: %#v", credentials)
			}
			return nil
		},
		prompter: &fakeTailscalePrompter{
			canPrompt: false,
		},
	}

	var out bytes.Buffer
	if err := runTailscaleAuth(context.Background(), &out, tailscaleAuthOptions{}, deps); err != nil {
		t.Fatalf("run auth tailscale: %v", err)
	}

	if got, want := out.String(), "Saved Tailscale credentials.\n"; got != want {
		t.Fatalf("output mismatch:\nwant %q\ngot  %q", want, got)
	}
	if len(validated) != 1 {
		t.Fatalf("validation count mismatch: %d", len(validated))
	}
	assertSavedTailscaleCredentials(t, configPath, "env-token", "tailnet-123")
	assertFileMode(t, configPath, 0o600)
	assertFileMode(t, filepath.Dir(configPath), 0o700)

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var saved map[string]map[string]map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	if _, ok := saved["auth"]["tailscale"]["machineAuthKey"]; ok {
		t.Fatalf("machine auth key should not be saved: %s", data)
	}
	if got, want := len(saved["auth"]["tailscale"]), 2; got != want {
		t.Fatalf("saved Tailscale config should contain only token and tailnet, got %s", data)
	}
}

func TestAuthTailscaleCommandValidatesThroughTailscaleAPIAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	server := newTailscaleValidationServer(t, map[string]testTailscaleTokenPermission{
		"flag-token": testTailscaleFullAccess,
	})

	t.Setenv("MYN_CONFIG", configPath)
	t.Setenv("TAILSCALE_ENDPOINT", server.URL)

	var out bytes.Buffer
	cmd := NewRootCommand(BuildInfo{})
	cmd.SetArgs([]string{"auth", "tailscale", "--token", "flag-token", "--tailnet", "tailnet-123"})
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute auth tailscale: %v", err)
	}

	if got, want := out.String(), "Saved Tailscale credentials.\n"; got != want {
		t.Fatalf("output mismatch:\nwant %q\ngot  %q", want, got)
	}
	if strings.Contains(out.String(), "flag-token") {
		t.Fatal("Tailscale API token was printed")
	}
	assertSavedTailscaleCredentials(t, configPath, "flag-token", "tailnet-123")

	requests := server.Requests()
	gotOperations := make([]string, 0, len(requests))
	for _, request := range requests {
		gotOperations = append(gotOperations, request.Operation)
	}
	wantOperations := []string{
		"policy read",
		"policy validation",
		"safe no-op policy update",
		"device listing",
		"auth key creation",
		"auth key cleanup",
	}
	if !slices.Equal(gotOperations, wantOperations) {
		t.Fatalf("validation operations mismatch:\nwant %#v\ngot  %#v", wantOperations, gotOperations)
	}

	createRequest := requests[4]
	var body struct {
		Capabilities struct {
			Devices struct {
				Create struct {
					Reusable      bool     `json:"reusable"`
					Ephemeral     bool     `json:"ephemeral"`
					Preauthorized bool     `json:"preauthorized"`
					Tags          []string `json:"tags"`
				} `json:"create"`
			} `json:"devices"`
		} `json:"capabilities"`
		ExpirySeconds int64  `json:"expirySeconds"`
		Description   string `json:"description"`
	}
	if err := json.Unmarshal([]byte(createRequest.Body), &body); err != nil {
		t.Fatalf("parse auth key request body: %v\n%s", err, createRequest.Body)
	}
	if body.Capabilities.Devices.Create.Reusable {
		t.Fatal("validation auth key should be one-off")
	}
	if !body.Capabilities.Devices.Create.Ephemeral {
		t.Fatal("validation auth key should be ephemeral")
	}
	if body.Capabilities.Devices.Create.Preauthorized {
		t.Fatal("validation auth key should not be pre-approved")
	}
	if len(body.Capabilities.Devices.Create.Tags) != 0 {
		t.Fatalf("validation auth key should not request tags before policy setup: %#v", body.Capabilities.Devices.Create.Tags)
	}
	if got, want := body.ExpirySeconds, int64(600); got != want {
		t.Fatalf("validation auth key expiry mismatch: want %d, got %d", want, got)
	}
}

func TestAuthTailscaleRejectsInvalidToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	server := newTailscaleValidationServer(t, map[string]testTailscaleTokenPermission{
		"good-token": testTailscaleFullAccess,
	})

	t.Setenv("MYN_CONFIG", configPath)
	t.Setenv("TAILSCALE_ENDPOINT", server.URL)

	var out bytes.Buffer
	cmd := NewRootCommand(BuildInfo{})
	cmd.SetArgs([]string{"auth", "tailscale", "--token", "bad-token", "--tailnet", "tailnet-123"})
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid token error")
	}
	if !strings.Contains(err.Error(), "Tailscale token validation failed: API rejected the token while checking policy read") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "bad-token") || strings.Contains(out.String(), "bad-token") {
		t.Fatal("invalid Tailscale API token was printed")
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config should not exist after invalid token, stat err: %v", statErr)
	}
}

func TestAuthTailscaleRejectsInsufficientCapability(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	server := newTailscaleValidationServer(t, map[string]testTailscaleTokenPermission{
		"limited-token": testTailscaleNoAuthKeyCreate,
	})

	t.Setenv("MYN_CONFIG", configPath)
	t.Setenv("TAILSCALE_ENDPOINT", server.URL)

	var out bytes.Buffer
	cmd := NewRootCommand(BuildInfo{})
	cmd.SetArgs([]string{"auth", "tailscale", "--token", "limited-token", "--tailnet", "tailnet-123"})
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected insufficient capability error")
	}
	if !strings.Contains(err.Error(), "Tailscale token validation failed: token is missing auth key creation capability") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "limited-token") || strings.Contains(out.String(), "limited-token") {
		t.Fatal("limited Tailscale API token was printed")
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config should not exist after insufficient token, stat err: %v", statErr)
	}
}

func TestAuthTailscalePromptsWhenTailnetInferenceIsAmbiguous(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	t.Setenv("TAILSCALE_API_TOKEN", "env-token")

	prompter := &fakeTailscalePrompter{
		canPrompt:       true,
		selectedTailnet: "tailnet-b",
	}
	var validated []tailscaleCredentials
	deps := tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: os.Getenv,
		inferTailnets: func(_ context.Context, token string) ([]string, error) {
			if token != "env-token" {
				return nil, fmt.Errorf("unexpected token %q", token)
			}
			return []string{"tailnet-a", "tailnet-b"}, nil
		},
		validateCredentials: func(_ context.Context, credentials tailscaleCredentials) error {
			validated = append(validated, credentials)
			return nil
		},
		prompter: prompter,
	}

	var out bytes.Buffer
	if err := runTailscaleAuth(context.Background(), &out, tailscaleAuthOptions{}, deps); err != nil {
		t.Fatalf("run auth tailscale: %v", err)
	}

	if !slices.Equal(prompter.selectCandidates, []string{"tailnet-a", "tailnet-b"}) {
		t.Fatalf("select candidates mismatch: %#v", prompter.selectCandidates)
	}
	if len(validated) != 1 || validated[0].Tailnet != "tailnet-b" {
		t.Fatalf("validated credentials mismatch: %#v", validated)
	}
	assertSavedTailscaleCredentials(t, configPath, "env-token", "tailnet-b")
}

func TestAuthTailscaleInfersSingleTailnet(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	t.Setenv("TAILSCALE_API_TOKEN", "env-token")

	var validated []tailscaleCredentials
	deps := tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: os.Getenv,
		inferTailnets: func(_ context.Context, token string) ([]string, error) {
			if token != "env-token" {
				return nil, fmt.Errorf("unexpected token %q", token)
			}
			return []string{"tailnet-inferred"}, nil
		},
		validateCredentials: func(_ context.Context, credentials tailscaleCredentials) error {
			validated = append(validated, credentials)
			return nil
		},
		prompter: &fakeTailscalePrompter{
			canPrompt: false,
		},
	}

	var out bytes.Buffer
	if err := runTailscaleAuth(context.Background(), &out, tailscaleAuthOptions{}, deps); err != nil {
		t.Fatalf("run auth tailscale: %v", err)
	}

	if len(validated) != 1 || validated[0].Tailnet != "tailnet-inferred" {
		t.Fatalf("validated credentials mismatch: %#v", validated)
	}
	assertSavedTailscaleCredentials(t, configPath, "env-token", "tailnet-inferred")
}

func TestAuthTailscaleInteractiveBrowserFallbackPrintsKeysURL(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")

	prompter := &fakeTailscalePrompter{
		canPrompt:    true,
		inputToken:   "prompt-token",
		inputTailnet: "tailnet-123",
	}
	var opened []string
	deps := tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		env: func(string) string {
			return ""
		},
		openURL: func(rawURL string) error {
			opened = append(opened, rawURL)
			return fmt.Errorf("browser unavailable")
		},
		inferTailnets: func(_ context.Context, _ string) ([]string, error) {
			return nil, fmt.Errorf("not inferable")
		},
		validateCredentials: func(_ context.Context, credentials tailscaleCredentials) error {
			if credentials.Token != "prompt-token" || credentials.Tailnet != "tailnet-123" {
				return fmt.Errorf("unexpected credentials: %#v", credentials)
			}
			return nil
		},
		prompter: prompter,
	}

	var out bytes.Buffer
	if err := runTailscaleAuth(context.Background(), &out, tailscaleAuthOptions{}, deps); err != nil {
		t.Fatalf("run auth tailscale: %v", err)
	}

	if !slices.Equal(opened, []string{tailscaleKeysURL}) {
		t.Fatalf("opened URLs mismatch: %#v", opened)
	}
	if !strings.Contains(out.String(), tailscaleKeysURL) {
		t.Fatalf("fallback output should include keys URL, got %q", out.String())
	}
	if strings.Contains(out.String(), "prompt-token") {
		t.Fatal("prompted Tailscale API token was printed")
	}
	assertSavedTailscaleCredentials(t, configPath, "prompt-token", "tailnet-123")
}

func TestOpenBrowserURLReportsStartedOpenerFailure(t *testing.T) {
	var gotCommand string
	var gotArgs []string

	err := runBrowserOpener(tailscaleKeysURL, "linux", func(command string, args ...string) *exec.Cmd {
		gotCommand = command
		gotArgs = slices.Clone(args)
		cmd := exec.Command(os.Args[0], "-test.run=TestBrowserOpenerHelperProcess")
		cmd.Env = append(os.Environ(), "MYN_BROWSER_OPENER_HELPER=exit")
		return cmd
	})
	if err == nil {
		t.Fatal("expected opener exit failure")
	}
	if gotCommand != "xdg-open" {
		t.Fatalf("browser opener command mismatch: %q", gotCommand)
	}
	if !slices.Equal(gotArgs, []string{tailscaleKeysURL}) {
		t.Fatalf("browser opener args mismatch: %#v", gotArgs)
	}
}

func TestBrowserOpenerHelperProcess(t *testing.T) {
	if os.Getenv("MYN_BROWSER_OPENER_HELPER") != "exit" {
		return
	}
	os.Exit(7)
}

type fakeTailscalePrompter struct {
	canPrompt        bool
	inputToken       string
	inputTailnet     string
	selectedTailnet  string
	selectCandidates []string
}

func (p *fakeTailscalePrompter) CanPrompt() bool {
	return p.canPrompt
}

func (p *fakeTailscalePrompter) InputTailscaleToken() (string, error) {
	return p.inputToken, nil
}

func (p *fakeTailscalePrompter) InputTailnet() (string, error) {
	return p.inputTailnet, nil
}

func (p *fakeTailscalePrompter) SelectTailnet(candidates []string) (string, error) {
	p.selectCandidates = slices.Clone(candidates)
	return p.selectedTailnet, nil
}

func assertSavedTailscaleCredentials(t *testing.T, configPath string, wantToken string, wantTailnet string) {
	t.Helper()

	cfg, err := loadAppConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.Auth.Tailscale.Token; got != wantToken {
		t.Fatalf("saved token mismatch: want %q, got %q", wantToken, got)
	}
	if got := cfg.Auth.Tailscale.Tailnet; got != wantTailnet {
		t.Fatalf("saved tailnet mismatch: want %q, got %q", wantTailnet, got)
	}
}

type testTailscaleTokenPermission string

const (
	testTailscaleFullAccess      testTailscaleTokenPermission = "full"
	testTailscaleNoAuthKeyCreate testTailscaleTokenPermission = "no-auth-key-create"
)

type testTailscaleRequest struct {
	Operation string
	Method    string
	Path      string
	Body      string
}

type testTailscaleValidationServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []testTailscaleRequest
}

func (s *testTailscaleValidationServer) Requests() []testTailscaleRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *testTailscaleValidationServer) appendRequest(request testTailscaleRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
}

func newTailscaleValidationServer(t *testing.T, tokens map[string]testTailscaleTokenPermission) *testTailscaleValidationServer {
	t.Helper()

	const tailnet = "tailnet-123"
	server := &testTailscaleValidationServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}

		token, _, ok := r.BasicAuth()
		permission, allowed := tokens[token]
		if !ok || !allowed {
			writeTailscaleAPIError(w, http.StatusUnauthorized, "invalid API access token")
			return
		}

		operation, ok := classifyTailscaleValidationRequest(r.Method, r.URL.Path, tailnet)
		if !ok {
			t.Errorf("unexpected Tailscale validation request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}

		server.appendRequest(testTailscaleRequest{
			Operation: operation,
			Method:    r.Method,
			Path:      r.URL.Path,
			Body:      string(body),
		})

		if permission == testTailscaleNoAuthKeyCreate && operation == "auth key creation" {
			writeTailscaleAPIError(w, http.StatusForbidden, "forbidden")
			return
		}

		switch operation {
		case "policy read":
			if got, want := r.Header.Get("Accept"), "application/hujson"; got != want {
				t.Errorf("policy read Accept mismatch: want %q, got %q", want, got)
			}
			w.Header().Set("Content-Type", "application/hujson")
			w.Header().Set("Etag", "policy-etag")
			fmt.Fprint(w, "{}")
		case "policy validation":
			if got, want := r.Header.Get("Content-Type"), "application/hujson"; got != want {
				t.Errorf("policy validation Content-Type mismatch: want %q, got %q", want, got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		case "safe no-op policy update":
			if got, want := r.Header.Get("Content-Type"), "application/hujson"; got != want {
				t.Errorf("policy update Content-Type mismatch: want %q, got %q", want, got)
			}
			if got, want := r.Header.Get("If-Match"), `"policy-etag"`; got != want {
				t.Errorf("policy update If-Match mismatch: want %q, got %q", want, got)
			}
			w.WriteHeader(http.StatusOK)
		case "device listing":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"devices":[]}`)
		case "auth key creation":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"k-validation","key":"tskey-auth-validation"}`)
		case "auth key cleanup":
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unhandled operation: %s", operation)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func classifyTailscaleValidationRequest(method string, path string, tailnet string) (string, bool) {
	switch {
	case method == http.MethodGet && path == "/api/v2/tailnet/"+tailnet+"/acl":
		return "policy read", true
	case method == http.MethodPost && path == "/api/v2/tailnet/"+tailnet+"/acl/validate":
		return "policy validation", true
	case method == http.MethodPost && path == "/api/v2/tailnet/"+tailnet+"/acl":
		return "safe no-op policy update", true
	case method == http.MethodGet && path == "/api/v2/tailnet/"+tailnet+"/devices":
		return "device listing", true
	case method == http.MethodPost && path == "/api/v2/tailnet/"+tailnet+"/keys":
		return "auth key creation", true
	case method == http.MethodDelete && path == "/api/v2/tailnet/"+tailnet+"/keys/k-validation":
		return "auth key cleanup", true
	default:
		return "", false
	}
}

func writeTailscaleAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"message":%q}`, message)
}
