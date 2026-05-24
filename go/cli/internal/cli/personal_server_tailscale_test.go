package cli

import (
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
	}
	if got != want {
		t.Fatalf("status mismatch: want %#v, got %#v", want, got)
	}
}
