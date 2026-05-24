package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
	tailscale "tailscale.com/client/tailscale/v2"
)

func TestPlanPersonalServerTailnetPolicyNoopWhenPolicyIsSufficient(t *testing.T) {
	raw := `{
  "tagOwners": {
    "tag:myn-personal-server": ["harish@example.test"]
  },
  "grants": [
    {
      "src": ["harish@example.test"],
      "dst": ["tag:myn-personal-server"],
      "ip": ["tcp:22"]
    }
  ],
  "ssh": [
    {
      "action": "accept",
      "src": ["harish@example.test"],
      "dst": ["tag:myn-personal-server"],
      "users": ["harish"],
      "checkPeriod": "always"
    }
  ]
}`

	plan, err := planPersonalServerTailnetPolicy(raw, personalServerTailnetPolicyInput{
		Identity: "harish@example.test",
		User:     "harish",
		Tag:      personalServerTailscaleTag,
	})
	if err != nil {
		t.Fatalf("plan policy: %v", err)
	}
	if plan.NeedsChanges {
		t.Fatalf("policy should already be sufficient: %#v", plan)
	}
	if got, want := plan.Summary, []string{"Tailnet Policy already allows harish@example.test to use tag:myn-personal-server as harish."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("summary mismatch: want %#v, got %#v", want, got)
	}
	if got := strings.TrimSpace(plan.ProposedHuJSON); got != strings.TrimSpace(raw) {
		t.Fatalf("no-op plan should preserve raw policy:\nwant %s\ngot  %s", raw, plan.ProposedHuJSON)
	}
}

func TestPlanPersonalServerTailnetPolicyReportsIndividualMissingPieces(t *testing.T) {
	sufficientParts := map[string]string{
		"tag owner": `"tagOwners": {"tag:myn-personal-server": ["harish@example.test"]}`,
		"grant":     `"grants": [{"src": ["harish@example.test"], "dst": ["tag:myn-personal-server"], "ip": ["tcp:22"]}]`,
		"ssh":       `"ssh": [{"action": "accept", "src": ["harish@example.test"], "dst": ["tag:myn-personal-server"], "users": ["harish"], "checkPeriod": "always"}]`,
	}

	tests := []struct {
		name    string
		omit    string
		wantOne string
	}{
		{
			name:    "missing tag owner",
			omit:    "tag owner",
			wantOne: "Allow harish@example.test to own tag:myn-personal-server.",
		},
		{
			name:    "missing grant",
			omit:    "grant",
			wantOne: "Grant harish@example.test network access to tag:myn-personal-server on tcp:22.",
		},
		{
			name:    "missing ssh",
			omit:    "ssh",
			wantOne: "Allow harish@example.test to Tailscale SSH to tag:myn-personal-server as harish with checkPeriod always.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parts []string
			for name, raw := range sufficientParts {
				if name != tt.omit {
					parts = append(parts, raw)
				}
			}
			raw := "{\n" + strings.Join(parts, ",\n") + "\n}"

			plan, err := planPersonalServerTailnetPolicy(raw, personalServerTailnetPolicyInput{
				Identity: "harish@example.test",
				User:     "harish",
				Tag:      personalServerTailscaleTag,
			})
			if err != nil {
				t.Fatalf("plan policy: %v", err)
			}
			if !plan.NeedsChanges {
				t.Fatal("policy should need changes")
			}
			if got, want := plan.Summary, []string{tt.wantOne}; !reflect.DeepEqual(got, want) {
				t.Fatalf("summary mismatch: want %#v, got %#v", want, got)
			}
		})
	}
}

func TestRunConfigureValidatesProposedTailnetPolicyBeforeFinalConfirmation(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, "projects"))
	identity := seedTestSSHIdentity(t, home, ".ssh/id_ed25519", "existing@host", 0o600)
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	if err := saveAppConfig(configPath, appConfig{
		Auth: testPersonalServerAuthConfig(),
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cloud := successfulPersonalServerCloudClient()
	policy := &fakePersonalServerTailnetPolicyClient{
		rawPolicy:   `{}`,
		rawETag:     "etag-before",
		validateErr: errors.New("Tailscale says no"),
	}
	opened := []string{}
	prompter := &fakeConfigurePrompter{
		canPrompt: true,
		inputs:    []string{"harish", "harish-personal-server"},
		passwords: []string{"server-secret", "server-secret"},
		confirms:  []bool{true},
	}

	var out bytes.Buffer
	err := runConfigure(&out, configureOptions{
		localRoot:          "projects",
		localRootSet:       true,
		remoteRoot:         "projects",
		remoteRootSet:      true,
		sshIdentityFile:    identity.PrivatePath,
		sshIdentityFileSet: true,
	}, configureDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		userHomeDir: func() (string, error) {
			return home, nil
		},
		sshPublicKey: testSSHPublicKeyFunc(identity),
		prompter:     prompter,
		personalServerProvisioner: personalServerProvisioningGate{
			newLocalTailscaleClient: testPersonalServerLocalTailscaleClient,
			tailnetPolicyEnabled:    true,
			newCloudClient: func(string) personalServerCloudClient {
				return cloud
			},
			newTailnetPolicyClient: func(tailscaleConfig) personalServerTailnetPolicyClient {
				return policy
			},
			openURL: func(rawURL string) error {
				opened = append(opened, rawURL)
				return fmt.Errorf("browser unavailable")
			},
		},
	})
	if err == nil {
		t.Fatal("expected policy validation error")
	}
	if !strings.Contains(err.Error(), "validate proposed Tailnet Policy: Tailscale says no") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := prompter.confirmCalls, []string{"Allow Myn to edit Tailnet Policy?"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("confirm calls mismatch: want %#v, got %#v", want, got)
	}
	if got, want := opened, []string{tailscaleAccessControlsURL}; !reflect.DeepEqual(got, want) {
		t.Fatalf("opened URLs mismatch: want %#v, got %#v", want, got)
	}
	if len(policy.validatedPolicies) != 1 {
		t.Fatalf("expected exactly one validation, got %d", len(policy.validatedPolicies))
	}
	if len(policy.appliedPolicies) != 0 {
		t.Fatalf("policy should not be applied before validation succeeds: %#v", policy.appliedPolicies)
	}
	if cloud.serverCreateRequest.Name != "" {
		t.Fatalf("cloud resources should not be created after policy validation failure: %#v", cloud.serverCreateRequest)
	}
	output := out.String()
	for _, want := range []string{
		"Tailnet Policy changes:",
		"Allow harish to own tag:myn-personal-server.",
		"Open https://login.tailscale.com/admin/acls/file to inspect Tailscale Access Controls.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}

func TestRunConfigureDeclinedTailnetPolicyEditDoesNotCreateCloudResources(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, "projects"))
	identity := seedTestSSHIdentity(t, home, ".ssh/id_ed25519", "existing@host", 0o600)
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	if err := saveAppConfig(configPath, appConfig{
		Auth: testPersonalServerAuthConfig(),
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cloud := successfulPersonalServerCloudClient()
	policy := &fakePersonalServerTailnetPolicyClient{rawPolicy: `{}`, rawETag: "etag-before"}
	prompter := &fakeConfigurePrompter{
		canPrompt: true,
		inputs:    []string{"harish", "harish-personal-server"},
		passwords: []string{"server-secret", "server-secret"},
		confirms:  []bool{false},
	}

	var out bytes.Buffer
	if err := runConfigure(&out, configureOptions{
		localRoot:          "projects",
		localRootSet:       true,
		remoteRoot:         "projects",
		remoteRootSet:      true,
		sshIdentityFile:    identity.PrivatePath,
		sshIdentityFileSet: true,
	}, configureDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		userHomeDir: func() (string, error) {
			return home, nil
		},
		sshPublicKey: testSSHPublicKeyFunc(identity),
		prompter:     prompter,
		personalServerProvisioner: personalServerProvisioningGate{
			newLocalTailscaleClient: testPersonalServerLocalTailscaleClient,
			tailnetPolicyEnabled:    true,
			newCloudClient: func(string) personalServerCloudClient {
				return cloud
			},
			newTailnetPolicyClient: func(tailscaleConfig) personalServerTailnetPolicyClient {
				return policy
			},
			openURL: func(string) error {
				return nil
			},
		},
	}); err != nil {
		t.Fatalf("run configure: %v", err)
	}

	if got, want := prompter.confirmCalls, []string{"Allow Myn to edit Tailnet Policy?"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("confirm calls mismatch: want %#v, got %#v", want, got)
	}
	if len(policy.validatedPolicies) != 0 || len(policy.appliedPolicies) != 0 {
		t.Fatalf("declined edit should not validate or apply policy, got validates=%d applies=%d", len(policy.validatedPolicies), len(policy.appliedPolicies))
	}
	if cloud.createdFirewall.ID != 0 || cloud.createdSSHKey.ID != 0 || cloud.serverCreateRequest.Name != "" {
		t.Fatalf("declined policy edit should stop before cloud resources, got firewall=%#v sshKey=%#v request=%#v", cloud.createdFirewall, cloud.createdSSHKey, cloud.serverCreateRequest)
	}
	if !strings.Contains(out.String(), "Tailnet Policy edit declined. No cloud resources were created.") {
		t.Fatalf("expected decline output, got %q", out.String())
	}
}

func TestRunConfigureAppliesTailnetPolicyWithETagBeforeCloudResources(t *testing.T) {
	home := t.TempDir()
	mkdirAll(t, filepath.Join(home, "projects"))
	identity := seedTestSSHIdentity(t, home, ".ssh/id_ed25519", "existing@host", 0o600)
	configPath := filepath.Join(t.TempDir(), "myn", "config.json")
	if err := saveAppConfig(configPath, appConfig{
		Auth: testPersonalServerAuthConfig(),
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cloud := successfulPersonalServerCloudClient()
	policy := &fakePersonalServerTailnetPolicyClient{
		rawPolicy: `{}`,
		rawETag:   "etag-before-apply",
		applyErr:  errors.New("etag conflict"),
	}
	prompter := &fakeConfigurePrompter{
		canPrompt: true,
		inputs:    []string{"harish", "harish-personal-server"},
		passwords: []string{"server-secret", "server-secret"},
		confirms:  []bool{true, true},
	}

	var out bytes.Buffer
	err := runConfigure(&out, configureOptions{
		localRoot:          "projects",
		localRootSet:       true,
		remoteRoot:         "projects",
		remoteRootSet:      true,
		sshIdentityFile:    identity.PrivatePath,
		sshIdentityFileSet: true,
	}, configureDeps{
		appConfigPath: func() (string, error) {
			return configPath, nil
		},
		userHomeDir: func() (string, error) {
			return home, nil
		},
		sshPublicKey: testSSHPublicKeyFunc(identity),
		prompter:     prompter,
		personalServerProvisioner: personalServerProvisioningGate{
			newLocalTailscaleClient: testPersonalServerLocalTailscaleClient,
			tailnetPolicyEnabled:    true,
			newCloudClient: func(string) personalServerCloudClient {
				return cloud
			},
			newTailnetPolicyClient: func(tailscaleConfig) personalServerTailnetPolicyClient {
				return policy
			},
			openURL: func(string) error {
				return nil
			},
		},
	})
	if err == nil {
		t.Fatal("expected Tailnet Policy apply error")
	}
	if !strings.Contains(err.Error(), "apply Tailnet Policy: etag conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := prompter.confirmCalls, []string{
		"Allow Myn to edit Tailnet Policy?",
		"Create Personal Server?",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("confirm calls mismatch: want %#v, got %#v", want, got)
	}
	if got, want := policy.readPolicies, 2; got != want {
		t.Fatalf("policy read count mismatch: want %d, got %d", want, got)
	}
	if got, want := len(policy.validatedPolicies), 2; got != want {
		t.Fatalf("policy validation count mismatch: want %d, got %d", want, got)
	}
	if got, want := policy.appliedETags, []string{"etag-before-apply"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("applied ETags mismatch: want %#v, got %#v", want, got)
	}
	if cloud.createdFirewall.ID != 0 || cloud.createdSSHKey.ID != 0 || cloud.serverCreateRequest.Name != "" {
		t.Fatalf("policy apply conflict should stop before cloud resources, got firewall=%#v sshKey=%#v request=%#v", cloud.createdFirewall, cloud.createdSSHKey, cloud.serverCreateRequest)
	}
	if strings.Contains(out.String(), "etag conflict") {
		t.Fatalf("low-level policy apply error should not be printed to stdout, got %q", out.String())
	}
}

type fakePersonalServerTailnetPolicyClient struct {
	rawPolicy         string
	rawETag           string
	readPolicies      int
	validatedPolicies []string
	validateErr       error
	appliedPolicies   []string
	appliedETags      []string
	applyErr          error
	events            *[]string
	applyEvent        string
}

func (c *fakePersonalServerTailnetPolicyClient) ReadPolicy(context.Context) (personalServerTailnetPolicy, error) {
	c.readPolicies++
	return personalServerTailnetPolicy{HuJSON: c.rawPolicy, ETag: c.rawETag}, nil
}

func (c *fakePersonalServerTailnetPolicyClient) ValidatePolicy(_ context.Context, rawHuJSON string) error {
	c.validatedPolicies = append(c.validatedPolicies, rawHuJSON)
	return c.validateErr
}

func (c *fakePersonalServerTailnetPolicyClient) ApplyPolicy(_ context.Context, rawHuJSON string, etag string) error {
	c.appliedPolicies = append(c.appliedPolicies, rawHuJSON)
	c.appliedETags = append(c.appliedETags, etag)
	if c.events != nil {
		event := c.applyEvent
		if event == "" {
			event = "policy.apply"
		}
		*c.events = append(*c.events, event)
	}
	return c.applyErr
}

func TestPlanPersonalServerTailnetPolicyAddsMissingPiecesAndPreservesUnrelatedPolicy(t *testing.T) {
	raw := `{
  // Existing policy owned outside Myn.
  "groups": {
    "group:ops": ["ops@example.test"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["group:ops"],
      "dst": ["tag:db:5432"]
    }
  ],
  "tagOwners": {
    "tag:db": ["group:ops"]
  },
  "tests": [
    {
      "src": "ops@example.test",
      "accept": ["tag:db:5432"]
    }
  ]
}`

	plan, err := planPersonalServerTailnetPolicy(raw, personalServerTailnetPolicyInput{
		Identity: "harish@example.test",
		User:     "harish",
		Tag:      personalServerTailscaleTag,
	})
	if err != nil {
		t.Fatalf("plan policy: %v", err)
	}
	if !plan.NeedsChanges {
		t.Fatal("policy should need changes")
	}
	if !strings.Contains(plan.ProposedHuJSON, "Existing policy owned outside Myn.") {
		t.Fatalf("proposed policy should preserve unrelated comments:\n%s", plan.ProposedHuJSON)
	}
	if !strings.Contains(plan.ProposedHuJSON, "Myn Personal Server") {
		t.Fatalf("proposed policy should include Myn comments for added entries:\n%s", plan.ProposedHuJSON)
	}
	wantSummary := []string{
		"Allow harish@example.test to own tag:myn-personal-server.",
		"Grant harish@example.test network access to tag:myn-personal-server on tcp:22.",
		"Allow harish@example.test to Tailscale SSH to tag:myn-personal-server as harish with checkPeriod always.",
	}
	if !reflect.DeepEqual(plan.Summary, wantSummary) {
		t.Fatalf("summary mismatch:\nwant %#v\ngot  %#v", wantSummary, plan.Summary)
	}

	standardJSON, err := hujson.Standardize([]byte(plan.ProposedHuJSON))
	if err != nil {
		t.Fatalf("standardize proposed policy: %v\n%s", err, plan.ProposedHuJSON)
	}
	var proposed tailscale.ACL
	if err := json.Unmarshal(standardJSON, &proposed); err != nil {
		t.Fatalf("parse proposed policy: %v\n%s", err, plan.ProposedHuJSON)
	}
	if got, want := proposed.Groups["group:ops"], []string{"ops@example.test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated groups should be preserved: want %#v, got %#v", want, got)
	}
	if got, want := proposed.ACLs, []tailscale.ACLEntry{{
		Action:      "accept",
		Source:      []string{"group:ops"},
		Destination: []string{"tag:db:5432"},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated ACLs should be preserved: want %#v, got %#v", want, got)
	}
	if got, want := proposed.Tests, []tailscale.ACLTest{{
		Source: "ops@example.test",
		Accept: []string{"tag:db:5432"},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated tests should be preserved: want %#v, got %#v", want, got)
	}
	if got, want := proposed.TagOwners[personalServerTailscaleTag], []string{"harish@example.test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tag owners mismatch: want %#v, got %#v", want, got)
	}
	if got, want := proposed.Grants, []tailscale.Grant{{
		Source:      []string{"harish@example.test"},
		Destination: []string{personalServerTailscaleTag},
		IP:          []string{"tcp:22"},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("grants mismatch: want %#v, got %#v", want, got)
	}
	if got, want := proposed.SSH, []tailscale.ACLSSH{{
		Action:      "accept",
		Source:      []string{"harish@example.test"},
		Destination: []string{personalServerTailscaleTag},
		Users:       []string{"harish"},
		CheckPeriod: tailscale.CheckPeriodAlways,
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ssh rules mismatch: want %#v, got %#v", want, got)
	}
}
