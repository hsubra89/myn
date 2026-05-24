package cli

import (
	"context"
	"fmt"
	"strings"

	"tailscale.com/client/local"
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
}

type personalServerLocalTailscaleClientFunc func(context.Context) (personalServerLocalTailscaleStatus, error)

func (fn personalServerLocalTailscaleClientFunc) Status(ctx context.Context) (personalServerLocalTailscaleStatus, error) {
	return fn(ctx)
}

type personalServerLocalAPIClient struct {
	client *local.Client
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
		if profile, ok := status.User[status.Self.UserID]; ok {
			out.Identity = strings.TrimSpace(profile.LoginName)
			if out.Identity == "" {
				out.Identity = strings.TrimSpace(profile.DisplayName)
			}
		}
	}
	return out
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
	if !personalServerTailnetMatches(cfg.Tailnet, status) {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale daemon is connected to tailnet %q, but saved Tailscale Credentials use %q", status.bestTailnetName(), strings.TrimSpace(cfg.Tailnet))
	}
	if strings.TrimSpace(status.Identity) == "" {
		return personalServerLocalTailscaleStatus{}, fmt.Errorf("local Tailscale identity is unavailable; reconnect to Tailscale before running `myn configure`")
	}

	return status, nil
}

func (gate personalServerProvisioningGate) localTailscaleClient() personalServerLocalTailscaleClient {
	if gate.newLocalTailscaleClient != nil {
		return gate.newLocalTailscaleClient()
	}
	return personalServerLocalAPIClient{}
}

func personalServerTailnetMatches(savedTailnet string, status personalServerLocalTailscaleStatus) bool {
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
