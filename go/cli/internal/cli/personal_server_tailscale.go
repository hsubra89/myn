package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"tailscale.com/client/local"
	tailscale "tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn/ipnstate"
)

const (
	tailscaleBackendStateRunning                     = "Running"
	personalServerTailscaleMachineAuthKeyLifetime    = 10 * time.Minute
	personalServerTailscaleMachineAuthKeyDescription = "Myn Personal Server provisioning"
)

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

type personalServerTailscaleDevice struct {
	ID                 string
	NodeID             string
	Name               string
	Hostname           string
	Tags               []string
	Authorized         bool
	ConnectedToControl bool
}

type personalServerTailscaleDeviceClient interface {
	Devices(ctx context.Context) ([]personalServerTailscaleDevice, error)
}

type personalServerTailscaleMachineAuthKey struct {
	Key string
}

type personalServerTailscaleMachineAuthKeyInput struct {
	Tag      string
	Lifetime time.Duration
}

type personalServerTailscaleMachineAuthKeyClient interface {
	CreateMachineAuthKey(ctx context.Context, input personalServerTailscaleMachineAuthKeyInput) (personalServerTailscaleMachineAuthKey, error)
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

func (client personalServerTailscaleAPIClient) Devices(ctx context.Context) ([]personalServerTailscaleDevice, error) {
	cloudClient := client.client
	if cloudClient == nil {
		return nil, fmt.Errorf("Tailscale cloud client is unavailable")
	}
	devices, err := cloudClient.Devices().List(ctx)
	if err != nil {
		return nil, mapTailscaleValidationError(ctx, "Tailscale device listing", err)
	}

	result := make([]personalServerTailscaleDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, personalServerTailscaleDevice{
			ID:                 strings.TrimSpace(device.ID),
			NodeID:             strings.TrimSpace(device.NodeID),
			Name:               strings.TrimSpace(device.Name),
			Hostname:           strings.TrimSpace(device.Hostname),
			Tags:               append([]string(nil), device.Tags...),
			Authorized:         device.Authorized,
			ConnectedToControl: device.ConnectedToControl,
		})
	}
	return result, nil
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

func (client personalServerTailscaleAPIClient) CreateMachineAuthKey(ctx context.Context, input personalServerTailscaleMachineAuthKeyInput) (personalServerTailscaleMachineAuthKey, error) {
	cloudClient := client.client
	if cloudClient == nil {
		return personalServerTailscaleMachineAuthKey{}, fmt.Errorf("Tailscale cloud client is unavailable")
	}
	tag := strings.TrimSpace(input.Tag)
	if tag == "" {
		tag = personalServerTailscaleTag
	}
	lifetime := input.Lifetime
	if lifetime <= 0 {
		lifetime = personalServerTailscaleMachineAuthKeyLifetime
	}

	capabilities := tailscale.KeyCapabilities{}
	capabilities.Devices.Create.Reusable = false
	capabilities.Devices.Create.Ephemeral = false
	capabilities.Devices.Create.Preauthorized = true
	capabilities.Devices.Create.Tags = []string{tag}

	key, err := cloudClient.Keys().CreateAuthKey(ctx, tailscale.CreateKeyRequest{
		Capabilities:  capabilities,
		ExpirySeconds: int64(lifetime.Seconds()),
		Description:   personalServerTailscaleMachineAuthKeyDescription,
	})
	if err != nil {
		return personalServerTailscaleMachineAuthKey{}, mapTailscaleValidationError(ctx, "Tailscale Machine Auth Key creation", err)
	}
	if key == nil || strings.TrimSpace(key.Key) == "" {
		return personalServerTailscaleMachineAuthKey{}, fmt.Errorf("Tailscale Machine Auth Key creation returned no key material")
	}
	return personalServerTailscaleMachineAuthKey{Key: strings.TrimSpace(key.Key)}, nil
}

func (gate personalServerProvisioningGate) createPersonalServerTailscaleMachineAuthKey(ctx context.Context, cfg tailscaleConfig) (personalServerTailscaleMachineAuthKey, error) {
	key, err := gate.tailscaleMachineAuthKeyClient(cfg).CreateMachineAuthKey(ctx, personalServerTailscaleMachineAuthKeyInput{
		Tag:      personalServerTailscaleTag,
		Lifetime: personalServerTailscaleMachineAuthKeyLifetime,
	})
	if err != nil {
		return personalServerTailscaleMachineAuthKey{}, fmt.Errorf("create Tailscale Machine Auth Key: %w", err)
	}
	return key, nil
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

func (gate personalServerProvisioningGate) tailscaleMachineAuthKeyClient(cfg tailscaleConfig) personalServerTailscaleMachineAuthKeyClient {
	if gate.newTailscaleMachineAuthKeyClient != nil {
		return gate.newTailscaleMachineAuthKeyClient(cfg)
	}
	if cloudClient := gate.tailscaleCloudClient(cfg); cloudClient != nil {
		if authKeyClient, ok := cloudClient.(personalServerTailscaleMachineAuthKeyClient); ok {
			return authKeyClient
		}
	}
	return personalServerTailscaleMachineAuthKeyErrorClient{err: fmt.Errorf("Tailscale cloud client cannot create Machine Auth Keys")}
}

func (gate personalServerProvisioningGate) tailscaleDeviceClient(cfg tailscaleConfig) personalServerTailscaleDeviceClient {
	if gate.newTailscaleDeviceClient != nil {
		return gate.newTailscaleDeviceClient(cfg)
	}
	if cloudClient := gate.tailscaleCloudClient(cfg); cloudClient != nil {
		if deviceClient, ok := cloudClient.(personalServerTailscaleDeviceClient); ok {
			return deviceClient
		}
	}
	return personalServerTailscaleDeviceErrorClient{err: fmt.Errorf("Tailscale cloud client cannot list devices")}
}

type personalServerTailscaleMachineAuthKeyErrorClient struct {
	err error
}

func (client personalServerTailscaleMachineAuthKeyErrorClient) CreateMachineAuthKey(context.Context, personalServerTailscaleMachineAuthKeyInput) (personalServerTailscaleMachineAuthKey, error) {
	return personalServerTailscaleMachineAuthKey{}, client.err
}

type personalServerTailscaleDeviceErrorClient struct {
	err error
}

func (client personalServerTailscaleDeviceErrorClient) Devices(context.Context) ([]personalServerTailscaleDevice, error) {
	return nil, client.err
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
