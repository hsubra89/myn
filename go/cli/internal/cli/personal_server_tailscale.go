package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"tailscale.com/client/local"
	tailscale "tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn/ipnstate"
)

const tailscaleBackendStateRunning = "Running"

type personalServerLocalTailscaleClient interface {
	Status(ctx context.Context) (personalServerLocalTailscaleStatus, error)
}

type personalServerLocalTailscaleStatus struct {
	BackendState   string
	TailnetName    string
	MagicDNSSuffix string
	Identity       string
	SelfNodeID     string
}

type personalServerLocalTailscaleClientFunc func(context.Context) (personalServerLocalTailscaleStatus, error)

func (fn personalServerLocalTailscaleClientFunc) Status(ctx context.Context) (personalServerLocalTailscaleStatus, error) {
	return fn(ctx)
}

type personalServerTailscaleCloudClient interface {
	TailnetContainsNodeID(ctx context.Context, nodeID string) (bool, error)
}

type personalServerTailscaleCloudClientFunc func(context.Context, string) (bool, error)

func (fn personalServerTailscaleCloudClientFunc) TailnetContainsNodeID(ctx context.Context, nodeID string) (bool, error) {
	return fn(ctx, nodeID)
}

type personalServerLocalAPIClient struct {
	client *local.Client
}

type personalServerTailscaleAPIClient struct {
	client *tailscale.Client
}

func (client personalServerLocalAPIClient) Status(ctx context.Context) (personalServerLocalTailscaleStatus, error) {
	localClient := client.client
	if localClient == nil {
		localClient = &local.Client{}
	}

	status, err := localClient.StatusWithoutPeers(ctx)
	if err != nil {
		return personalServerLocalTailscaleStatus{}, err
	}
	return personalServerLocalTailscaleStatusFromIPN(status), nil
}

func personalServerLocalTailscaleStatusFromIPN(status *ipnstate.Status) personalServerLocalTailscaleStatus {
	if status == nil {
		return personalServerLocalTailscaleStatus{}
	}

	out := personalServerLocalTailscaleStatus{
		BackendState: status.BackendState,
	}
	if status.CurrentTailnet != nil {
		out.TailnetName = status.CurrentTailnet.Name
		out.MagicDNSSuffix = status.CurrentTailnet.MagicDNSSuffix
	}
	if status.Self != nil {
		out.SelfNodeID = strings.TrimSpace(string(status.Self.ID))
		if profile, ok := status.User[status.Self.UserID]; ok {
			out.Identity = strings.TrimSpace(profile.LoginName)
			if out.Identity == "" {
				out.Identity = strings.TrimSpace(profile.DisplayName)
			}
		}
	}
	return out
}

func (client personalServerTailscaleAPIClient) TailnetContainsNodeID(ctx context.Context, nodeID string) (bool, error) {
	nodeID = normalizePersonalServerTailnet(nodeID)
	if nodeID == "" {
		return false, nil
	}

	cloudClient := client.client
	if cloudClient == nil {
		return false, fmt.Errorf("Tailscale cloud client is unavailable")
	}
	devices, err := cloudClient.Devices().List(ctx)
	if err != nil {
		return false, mapTailscaleValidationError(ctx, "saved Tailscale tailnet device listing", err)
	}
	for _, device := range devices {
		for _, candidate := range []string{device.NodeID, device.ID} {
			if normalizePersonalServerTailnet(candidate) == nodeID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (client personalServerTailscaleAPIClient) ReadPolicy(ctx context.Context) (personalServerTailnetPolicy, error) {
	cloudClient := client.client
	if cloudClient == nil {
		return personalServerTailnetPolicy{}, fmt.Errorf("Tailscale cloud client is unavailable")
	}
	rawPolicy, err := cloudClient.PolicyFile().Raw(ctx)
	if err != nil {
		return personalServerTailnetPolicy{}, mapTailscaleValidationError(ctx, "Tailnet Policy read", err)
	}
	policy := strings.TrimSpace(rawPolicy.HuJSON)
	if policy == "" {
		policy = tailscaleValidationEmptyPolicy
	}
	return personalServerTailnetPolicy{
		HuJSON: policy,
		ETag:   rawPolicy.ETag,
	}, nil
}

func (client personalServerTailscaleAPIClient) ValidatePolicy(ctx context.Context, rawHuJSON string) error {
	cloudClient := client.client
	if cloudClient == nil {
		return fmt.Errorf("Tailscale cloud client is unavailable")
	}
	if strings.TrimSpace(rawHuJSON) == "" {
		rawHuJSON = tailscaleValidationEmptyPolicy
	}
	if err := cloudClient.PolicyFile().Validate(ctx, rawHuJSON); err != nil {
		return mapTailscaleValidationError(ctx, "Tailnet Policy validation", err)
	}
	return nil
}

func (client personalServerTailscaleAPIClient) ApplyPolicy(ctx context.Context, rawHuJSON string, etag string) error {
	cloudClient := client.client
	if cloudClient == nil {
		return fmt.Errorf("Tailscale cloud client is unavailable")
	}
	if strings.TrimSpace(rawHuJSON) == "" {
		rawHuJSON = tailscaleValidationEmptyPolicy
	}
	if err := cloudClient.PolicyFile().Set(ctx, rawHuJSON, etag); err != nil {
		return mapTailscaleValidationError(ctx, "Tailnet Policy update", err)
	}
	return nil
}

func (gate personalServerProvisioningGate) verifyLocalTailscaleForPersonalServerCreation(ctx context.Context, cfg tailscaleConfig) (personalServerLocalTailscaleStatus, error) {
	status, err := gate.localTailscaleClient().Status(ctx)
	if err != nil {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale daemon is unavailable or unreadable: %w", err)
	}

	state := strings.TrimSpace(status.BackendState)
	if state == "" {
		state = "unknown"
	}
	if state != tailscaleBackendStateRunning {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale daemon is not connected (state: %s); connect to Tailscale before running `myn configure`", state)
	}
	matches, err := gate.localTailscaleMatchesSavedCredentials(ctx, cfg, status)
	if err != nil {
		return personalServerLocalTailscaleStatus{}, err
	}
	if !matches {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale daemon is connected to tailnet %q, but saved Tailscale Credentials use %q", status.bestTailnetName(), strings.TrimSpace(cfg.Tailnet))
	}
	if strings.TrimSpace(status.Identity) == "" {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale identity is unavailable; reconnect to Tailscale before running `myn configure`")
	}

	return status, nil
}

func (gate personalServerProvisioningGate) localTailscaleMatchesSavedCredentials(ctx context.Context, cfg tailscaleConfig, status personalServerLocalTailscaleStatus) (bool, error) {
	selfNodeID := strings.TrimSpace(status.SelfNodeID)
	if selfNodeID == "" {
		return personalServerTailnetNamesMatch(cfg.Tailnet, status), nil
	}

	matches, err := gate.tailscaleCloudClient(cfg).TailnetContainsNodeID(ctx, selfNodeID)
	if err != nil {
		return false, fmt.Errorf("verify local Tailscale tailnet against saved credentials: %w", err)
	}
	return matches, nil
}

func (gate personalServerProvisioningGate) localTailscaleClient() personalServerLocalTailscaleClient {
	if gate.newLocalTailscaleClient != nil {
		return gate.newLocalTailscaleClient()
	}
	return personalServerLocalAPIClient{}
}

func (gate personalServerProvisioningGate) tailscaleCloudClient(cfg tailscaleConfig) personalServerTailscaleCloudClient {
	if gate.newTailscaleCloudClient != nil {
		return gate.newTailscaleCloudClient(cfg)
	}
	client, err := newTailscaleAPIClient(os.Getenv("TAILSCALE_ENDPOINT"), nil, cfg.Token, cfg.Tailnet)
	if err != nil {
		return personalServerTailscaleCloudClientFunc(func(context.Context, string) (bool, error) {
			return false, err
		})
	}
	return personalServerTailscaleAPIClient{client: client}
}

func personalServerTailnetNamesMatch(savedTailnet string, status personalServerLocalTailscaleStatus) bool {
	saved := normalizePersonalServerTailnet(savedTailnet)
	if saved == "" {
		return false
	}
	for _, candidate := range []string{status.TailnetName, status.MagicDNSSuffix} {
		if normalizePersonalServerTailnet(candidate) == saved {
			return true
		}
	}
	return false
}

func normalizePersonalServerTailnet(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
}

func (status personalServerLocalTailscaleStatus) bestTailnetName() string {
	if strings.TrimSpace(status.TailnetName) != "" {
		return strings.TrimSpace(status.TailnetName)
	}
	if strings.TrimSpace(status.MagicDNSSuffix) != "" {
		return strings.TrimSpace(status.MagicDNSSuffix)
	}
	return "unknown"
}
