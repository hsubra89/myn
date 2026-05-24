package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tailscale/hujson"
	tailscale "tailscale.com/client/tailscale/v2"
)

const (
	personalServerTailscaleTag           = "tag:myn-personal-server"
	personalServerTailnetPolicyTCPSSH    = "tcp:22"
	personalServerTailnetPolicySSHAction = "check"
	tailscaleAccessControlsURL           = "https://login.tailscale.com/admin/acls/file"
)

type personalServerTailnetPolicy struct {
	HuJSON string
	ETag   string
}

type personalServerTailnetPolicyClient interface {
	ReadPolicy(ctx context.Context) (personalServerTailnetPolicy, error)
	ValidatePolicy(ctx context.Context, rawHuJSON string) error
	ApplyPolicy(ctx context.Context, rawHuJSON string, etag string) error
}

type personalServerTailnetPolicyInput struct {
	Identity string
	User     string
	Tag      string
}

type personalServerTailnetPolicyPlan struct {
	NeedsChanges   bool
	Summary        []string
	ProposedHuJSON string
}

func planPersonalServerTailnetPolicy(rawHuJSON string, input personalServerTailnetPolicyInput) (personalServerTailnetPolicyPlan, error) {
	input.Identity = strings.TrimSpace(input.Identity)
	input.User = strings.TrimSpace(input.User)
	input.Tag = strings.TrimSpace(input.Tag)
	if input.Tag == "" {
		input.Tag = personalServerTailscaleTag
	}
	if input.Identity == "" {
		return personalServerTailnetPolicyPlan{}, fmt.Errorf("Tailscale identity is required")
	}
	if input.User == "" {
		return personalServerTailnetPolicyPlan{}, fmt.Errorf("Personal Server User is required")
	}

	normalizedRaw := strings.TrimSpace(rawHuJSON)
	if normalizedRaw == "" {
		normalizedRaw = "{}"
	}

	policy, err := parsePersonalServerTailnetPolicy(normalizedRaw)
	if err != nil {
		return personalServerTailnetPolicyPlan{}, err
	}

	var summary []string
	var patchOps []personalServerTailnetPolicyPatchOperation
	if !personalServerTailnetPolicyHasTagOwner(policy, input.Tag, input.Identity) {
		patchOps = append(patchOps, personalServerTailnetPolicyTagOwnerPatch(policy, input.Tag, input.Identity))
		summary = append(summary, fmt.Sprintf("Allow %s to own %s.", input.Identity, input.Tag))
	}
	if !personalServerTailnetPolicyHasGrant(policy, input.Identity, input.Tag, personalServerTailnetPolicyTCPSSH) {
		patchOps = append(patchOps, personalServerTailnetPolicyArrayPatch(policy.Grants == nil, "/grants", tailscale.Grant{
			Source:      []string{input.Identity},
			Destination: []string{input.Tag},
			IP:          []string{personalServerTailnetPolicyTCPSSH},
		}, "Myn Personal Server: allow the selected identity to reach tagged servers over SSH."))
		summary = append(summary, fmt.Sprintf("Grant %s network access to %s on %s.", input.Identity, input.Tag, personalServerTailnetPolicyTCPSSH))
	}
	if !personalServerTailnetPolicyHasSSHRule(policy, input.Identity, input.Tag, input.User) {
		patchOps = append(patchOps, personalServerTailnetPolicyArrayPatch(policy.SSH == nil, "/ssh", tailscale.ACLSSH{
			Action:      personalServerTailnetPolicySSHAction,
			Source:      []string{input.Identity},
			Destination: []string{input.Tag},
			Users:       []string{input.User},
			CheckPeriod: tailscale.CheckPeriodAlways,
		}, "Myn Personal Server: allow Tailscale SSH only as the selected Linux user."))
		summary = append(summary, fmt.Sprintf("Allow %s to Tailscale SSH to %s as %s with checkPeriod always.", input.Identity, input.Tag, input.User))
	}

	if len(summary) == 0 {
		return personalServerTailnetPolicyPlan{
			Summary:        []string{fmt.Sprintf("Tailnet Policy already allows %s to use %s as %s.", input.Identity, input.Tag, input.User)},
			ProposedHuJSON: normalizedRaw,
		}, nil
	}

	proposed, err := renderPersonalServerTailnetPolicyPatch(normalizedRaw, patchOps)
	if err != nil {
		return personalServerTailnetPolicyPlan{}, err
	}
	return personalServerTailnetPolicyPlan{
		NeedsChanges:   true,
		Summary:        summary,
		ProposedHuJSON: proposed,
	}, nil
}

func (gate personalServerProvisioningGate) prepareTailnetPolicyForPersonalServer(ctx context.Context, out io.Writer, cfg tailscaleConfig, identity string, user string, prompter configurePrompter) (personalServerTailnetPolicyPlan, bool, error) {
	if !gate.tailnetPolicyEnabled {
		return personalServerTailnetPolicyPlan{}, true, nil
	}

	client := gate.tailnetPolicyClient(cfg)
	rawPolicy, err := client.ReadPolicy(ctx)
	if err != nil {
		return personalServerTailnetPolicyPlan{}, false, fmt.Errorf("read Tailnet Policy: %w", err)
	}

	input := personalServerTailnetPolicyInput{
		Identity: identity,
		User:     user,
		Tag:      personalServerTailscaleTag,
	}
	plan, err := planPersonalServerTailnetPolicy(rawPolicy.HuJSON, input)
	if err != nil {
		return personalServerTailnetPolicyPlan{}, false, err
	}

	if plan.NeedsChanges {
		writePersonalServerTailnetPolicySummary(out, plan)
		if err := gate.personalServerOpenURL(tailscaleAccessControlsURL); err != nil {
			fmt.Fprintf(out, "Open %s to inspect Tailscale Access Controls.\n", tailscaleAccessControlsURL)
		}
		allowEdit, err := prompter.Confirm("Allow Myn to edit Tailnet Policy?", false)
		if err != nil {
			return personalServerTailnetPolicyPlan{}, false, err
		}
		if !allowEdit {
			fmt.Fprintln(out, "Tailnet Policy edit declined. No cloud resources were created.")
			return personalServerTailnetPolicyPlan{}, false, nil
		}
		if err := client.ValidatePolicy(ctx, plan.ProposedHuJSON); err != nil {
			return personalServerTailnetPolicyPlan{}, false, fmt.Errorf("validate proposed Tailnet Policy: %w", err)
		}
		return plan, true, nil
	}

	if err := client.ValidatePolicy(ctx, plan.ProposedHuJSON); err != nil {
		return personalServerTailnetPolicyPlan{}, false, fmt.Errorf("validate current Tailnet Policy: %w", err)
	}
	return plan, true, nil
}

func (gate personalServerProvisioningGate) applyTailnetPolicyForPersonalServer(ctx context.Context, cfg tailscaleConfig, input personalServerTailnetPolicyInput, acceptedPlan personalServerTailnetPolicyPlan) error {
	if !gate.tailnetPolicyEnabled {
		return nil
	}

	client := gate.tailnetPolicyClient(cfg)
	rawPolicy, err := client.ReadPolicy(ctx)
	if err != nil {
		return fmt.Errorf("re-read Tailnet Policy before apply: %w", err)
	}
	plan, err := planPersonalServerTailnetPolicy(rawPolicy.HuJSON, input)
	if err != nil {
		return err
	}
	if !plan.NeedsChanges {
		if err := client.ValidatePolicy(ctx, plan.ProposedHuJSON); err != nil {
			return fmt.Errorf("re-validate current Tailnet Policy before apply: %w", err)
		}
		return nil
	}
	if !acceptedPlan.NeedsChanges {
		return fmt.Errorf("Tailnet Policy changed after preview; rerun configure so Myn can request policy edit consent")
	}
	if err := client.ValidatePolicy(ctx, plan.ProposedHuJSON); err != nil {
		return fmt.Errorf("re-validate proposed Tailnet Policy before apply: %w", err)
	}
	if err := client.ApplyPolicy(ctx, plan.ProposedHuJSON, rawPolicy.ETag); err != nil {
		return fmt.Errorf("apply Tailnet Policy: %w", err)
	}
	return nil
}

func (gate personalServerProvisioningGate) tailnetPolicyClient(cfg tailscaleConfig) personalServerTailnetPolicyClient {
	if gate.newTailnetPolicyClient != nil {
		return gate.newTailnetPolicyClient(cfg)
	}
	if cloudClient := gate.tailscaleCloudClient(cfg); cloudClient != nil {
		if policyClient, ok := cloudClient.(personalServerTailnetPolicyClient); ok {
			return policyClient
		}
	}
	return personalServerTailnetPolicyErrorClient{err: fmt.Errorf("Tailscale cloud client cannot manage Tailnet Policy")}
}

func (gate personalServerProvisioningGate) personalServerOpenURL(rawURL string) error {
	if gate.openURL != nil {
		return gate.openURL(rawURL)
	}
	return openBrowserURL(rawURL)
}

func writePersonalServerTailnetPolicySummary(out io.Writer, plan personalServerTailnetPolicyPlan) {
	if len(plan.Summary) == 0 {
		return
	}
	if plan.NeedsChanges {
		fmt.Fprintln(out, "Tailnet Policy changes:")
	} else {
		fmt.Fprintln(out, "Tailnet Policy:")
	}
	for _, line := range plan.Summary {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(out, "- %s\n", line)
	}
}

type personalServerTailnetPolicyErrorClient struct {
	err error
}

func (client personalServerTailnetPolicyErrorClient) ReadPolicy(context.Context) (personalServerTailnetPolicy, error) {
	return personalServerTailnetPolicy{}, client.err
}

func (client personalServerTailnetPolicyErrorClient) ValidatePolicy(context.Context, string) error {
	return client.err
}

func (client personalServerTailnetPolicyErrorClient) ApplyPolicy(context.Context, string, string) error {
	return client.err
}

func parsePersonalServerTailnetPolicy(rawHuJSON string) (tailscale.ACL, error) {
	standardJSON, err := hujson.Standardize([]byte(rawHuJSON))
	if err != nil {
		return tailscale.ACL{}, fmt.Errorf("parse Tailnet Policy: %w", err)
	}
	var policy tailscale.ACL
	if err := json.Unmarshal(standardJSON, &policy); err != nil {
		return tailscale.ACL{}, fmt.Errorf("decode Tailnet Policy: %w", err)
	}
	return policy, nil
}

type personalServerTailnetPolicyPatchOperation struct {
	Path    string
	Value   any
	Comment string
}

func renderPersonalServerTailnetPolicyPatch(rawHuJSON string, ops []personalServerTailnetPolicyPatchOperation) (string, error) {
	if len(ops) == 0 {
		return rawHuJSON, nil
	}
	value, err := hujson.Parse([]byte(rawHuJSON))
	if err != nil {
		return "", fmt.Errorf("parse Tailnet Policy: %w", err)
	}
	patch, err := renderPersonalServerTailnetPolicyPatchOperations(ops)
	if err != nil {
		return "", err
	}
	if err := value.Patch([]byte(patch)); err != nil {
		return "", fmt.Errorf("patch Tailnet Policy: %w", err)
	}
	value.Format()
	return string(append(value.Pack(), '\n')), nil
}

func renderPersonalServerTailnetPolicyPatchOperations(ops []personalServerTailnetPolicyPatchOperation) (string, error) {
	var b strings.Builder
	b.WriteString("[\n")
	for i, op := range ops {
		if i > 0 {
			b.WriteString(",\n")
		}
		path, err := json.Marshal(op.Path)
		if err != nil {
			return "", fmt.Errorf("encode Tailnet Policy patch path: %w", err)
		}
		value, err := json.MarshalIndent(op.Value, "    ", "  ")
		if err != nil {
			return "", fmt.Errorf("encode Tailnet Policy patch value: %w", err)
		}
		b.WriteString("  {\n")
		b.WriteString(`    "op": "add",` + "\n")
		b.WriteString(`    "path": ` + string(path) + ",\n")
		if strings.TrimSpace(op.Comment) != "" {
			b.WriteString("    // " + strings.TrimSpace(op.Comment) + "\n")
			b.WriteString("    \"value\": ")
			b.Write(value)
			b.WriteByte('\n')
		} else {
			b.WriteString(`    "value": `)
			b.Write(value)
			b.WriteByte('\n')
		}
		b.WriteString("  }")
	}
	b.WriteString("\n]")
	return b.String(), nil
}

func personalServerTailnetPolicyTagOwnerPatch(policy tailscale.ACL, tag string, identity string) personalServerTailnetPolicyPatchOperation {
	tagKey, tagExists := personalServerTailnetPolicyTagOwnerKey(policy, tag)
	switch {
	case policy.TagOwners == nil:
		return personalServerTailnetPolicyPatchOperation{
			Path: "/tagOwners",
			Value: map[string][]string{
				tag: []string{identity},
			},
			Comment: "Myn Personal Server: allow this user to assign the Personal Server tag.",
		}
	case !tagExists:
		return personalServerTailnetPolicyPatchOperation{
			Path:    "/tagOwners/" + escapeTailnetPolicyJSONPointerToken(tag),
			Value:   []string{identity},
			Comment: "Myn Personal Server: allow this user to assign the Personal Server tag.",
		}
	default:
		return personalServerTailnetPolicyPatchOperation{
			Path:  "/tagOwners/" + escapeTailnetPolicyJSONPointerToken(tagKey) + "/-",
			Value: identity,
		}
	}
}

func personalServerTailnetPolicyArrayPatch(sectionMissing bool, sectionPath string, entry any, comment string) personalServerTailnetPolicyPatchOperation {
	if sectionMissing {
		return personalServerTailnetPolicyPatchOperation{
			Path:    sectionPath,
			Value:   []any{entry},
			Comment: comment,
		}
	}
	return personalServerTailnetPolicyPatchOperation{
		Path:    sectionPath + "/-",
		Value:   entry,
		Comment: comment,
	}
}

func personalServerTailnetPolicyTagOwnerKey(policy tailscale.ACL, tag string) (string, bool) {
	for existingTag := range policy.TagOwners {
		if policyPrincipalMatches(existingTag, tag) {
			return existingTag, true
		}
	}
	return "", false
}

func escapeTailnetPolicyJSONPointerToken(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

func personalServerTailnetPolicyHasTagOwner(policy tailscale.ACL, tag string, identity string) bool {
	for existingTag, owners := range policy.TagOwners {
		if !policyPrincipalMatches(existingTag, tag) {
			continue
		}
		return policyPrincipalListContains(owners, identity)
	}
	return false
}

func personalServerTailnetPolicyHasGrant(policy tailscale.ACL, identity string, tag string, capability string) bool {
	for _, grant := range policy.Grants {
		if policyGrantSourcesAllow(grant.Source, identity) &&
			policyGrantDestinationsAllow(grant.Destination, tag) &&
			policyGrantCapabilitiesAllow(grant.IP, capability) {
			return true
		}
	}
	return false
}

func personalServerTailnetPolicyHasSSHRule(policy tailscale.ACL, identity string, tag string, user string) bool {
	for _, rule := range policy.SSH {
		if !policyPrincipalMatches(rule.Action, personalServerTailnetPolicySSHAction) {
			continue
		}
		if !policyPrincipalListExactly(rule.Source, []string{identity}) {
			continue
		}
		if !policyPrincipalListExactly(rule.Destination, []string{tag}) {
			continue
		}
		if !policyPrincipalListExactly(rule.Users, []string{user}) {
			continue
		}
		if rule.CheckPeriod != tailscale.CheckPeriodAlways {
			continue
		}
		return true
	}
	return false
}

func appendPolicyPrincipal(values []string, value string) []string {
	if policyPrincipalListContains(values, value) {
		return values
	}
	return append(values, value)
}

func policyGrantSourcesAllow(values []string, identity string) bool {
	return policyPrincipalListContains(values, identity) || policyPrincipalListContains(values, "*")
}

func policyGrantDestinationsAllow(values []string, tag string) bool {
	return policyPrincipalListContains(values, tag) || policyPrincipalListContains(values, "*")
}

func policyGrantCapabilitiesAllow(values []string, capability string) bool {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		switch normalized {
		case "*", "22", capability:
			return true
		}
	}
	return false
}

func policyPrincipalListContains(values []string, want string) bool {
	for _, value := range values {
		if policyPrincipalMatches(value, want) {
			return true
		}
	}
	return false
}

func policyPrincipalListExactly(values []string, want []string) bool {
	if len(values) != len(want) {
		return false
	}
	for i := range values {
		if !policyPrincipalMatches(values[i], want[i]) {
			return false
		}
	}
	return true
}

func policyPrincipalMatches(value string, want string) bool {
	return strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want))
}
