package cli

import (
	"context"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

func TestPersonalServerLocalTailscaleStatusFromIPN(t *testing.T) {
	status := &ipnstate.Status{
		BackendState: tailscaleBackendStateRunning,
		CurrentTailnet: &ipnstate.TailnetStatus{
			Name:           "tailnet-123",
			MagicDNSSuffix: "example.ts.net",
		},
		Self: &ipnstate.PeerStatus{
			ID:     tailcfg.StableNodeID("n123456CNTRL"),
			UserID: tailcfg.UserID(42),
		},
		User: map[tailcfg.UserID]tailcfg.UserProfile{
			tailcfg.UserID(42): {
				LoginName:   "harish@example.test",
				DisplayName: "Harish Subra",
			},
		},
	}

	got := personalServerLocalTailscaleStatusFromIPN(status)
	want := personalServerLocalTailscaleStatus{
		BackendState:   tailscaleBackendStateRunning,
		TailnetName:    "tailnet-123",
		MagicDNSSuffix: "example.ts.net",
		Identity:       "harish@example.test",
		SelfNodeID:     "n123456CNTRL",
	}
	if got != want {
		t.Fatalf("status mismatch: want %#v, got %#v", want, got)
	}
}

func TestVerifyLocalTailscaleMatchesSavedTailnetByCloudNodeID(t *testing.T) {
	var checkedNodeID string
	gate := personalServerProvisioningGate{
		newLocalTailscaleClient: func() personalServerLocalTailscaleClient {
			return personalServerLocalTailscaleClientFunc(func(context.Context) (personalServerLocalTailscaleStatus, error) {
				return personalServerLocalTailscaleStatus{
					BackendState:   tailscaleBackendStateRunning,
					TailnetName:    "Example Team",
					MagicDNSSuffix: "tail123.ts.net",
					Identity:       "harish@example.test",
					SelfNodeID:     "n123456CNTRL",
				}, nil
			})
		},
		newTailscaleCloudClient: func(cfg tailscaleConfig) personalServerTailscaleCloudClient {
			if cfg.Token != "tailscale-token" || cfg.Tailnet != "tn-api-id" {
				t.Fatalf("unexpected Tailscale credentials: %#v", cfg)
			}
			return personalServerTailscaleCloudClientFunc(func(_ context.Context, nodeID string) (bool, error) {
				checkedNodeID = nodeID
				return nodeID == "n123456CNTRL", nil
			})
		},
	}

	status, err := gate.verifyLocalTailscaleForPersonalServerCreation(context.Background(), tailscaleConfig{
		Token:   "tailscale-token",
		Tailnet: "tn-api-id",
	})
	if err != nil {
		t.Fatalf("verify local Tailscale: %v", err)
	}
	if checkedNodeID != "n123456CNTRL" {
		t.Fatalf("checked node ID mismatch: %q", checkedNodeID)
	}
	if status.Identity != "harish@example.test" {
		t.Fatalf("identity mismatch: %#v", status)
	}
}

func TestVerifyLocalTailscaleFailsWhenSavedTailnetDoesNotContainLocalNode(t *testing.T) {
	gate := personalServerProvisioningGate{
		newLocalTailscaleClient: func() personalServerLocalTailscaleClient {
			return personalServerLocalTailscaleClientFunc(func(context.Context) (personalServerLocalTailscaleStatus, error) {
				return personalServerLocalTailscaleStatus{
					BackendState:   tailscaleBackendStateRunning,
					TailnetName:    "Example Team",
					MagicDNSSuffix: "tail123.ts.net",
					Identity:       "harish@example.test",
					SelfNodeID:     "n123456CNTRL",
				}, nil
			})
		},
		newTailscaleCloudClient: func(tailscaleConfig) personalServerTailscaleCloudClient {
			return personalServerTailscaleCloudClientFunc(func(context.Context, string) (bool, error) {
				return false, nil
			})
		},
	}

	_, err := gate.verifyLocalTailscaleForPersonalServerCreation(context.Background(), tailscaleConfig{
		Token:   "tailscale-token",
		Tailnet: "tn-other",
	})
	if err == nil {
		t.Fatal("expected tailnet mismatch error")
	}
	if got, want := err.Error(), `local Tailscale daemon is connected to tailnet "Example Team", but saved Tailscale Credentials use "tn-other"`; got != want {
		t.Fatalf("error mismatch:\nwant %q\ngot  %q", want, got)
	}
}
