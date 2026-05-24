package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	tailscale "tailscale.com/client/tailscale/v2"
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

func TestPersonalServerTailscaleAPIClientCreatesOneOffMachineAuthKey(t *testing.T) {
	var gotRequest tailscale.CreateKeyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method mismatch: want %s, got %s", want, got)
		}
		if got, want := r.URL.Path, "/api/v2/tailnet/tailnet-123/keys"; got != want {
			t.Fatalf("path mismatch: want %q, got %q", want, got)
		}
		token, _, ok := r.BasicAuth()
		if !ok || token != "tailscale-token" {
			t.Fatalf("unexpected auth token: ok=%t token=%q", ok, token)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode auth key request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"k-personal-server","key":"tskey-auth-personal-server"}`)
	}))
	t.Cleanup(server.Close)

	tailscaleClient, err := newTailscaleAPIClient(server.URL, server.Client(), "tailscale-token", "tailnet-123")
	if err != nil {
		t.Fatalf("new Tailscale client: %v", err)
	}
	key, err := (personalServerTailscaleAPIClient{client: tailscaleClient}).CreateMachineAuthKey(context.Background(), personalServerTailscaleMachineAuthKeyInput{
		Tag:      personalServerTailscaleTag,
		Lifetime: personalServerTailscaleMachineAuthKeyLifetime,
	})
	if err != nil {
		t.Fatalf("create machine auth key: %v", err)
	}
	if got, want := key.Key, "tskey-auth-personal-server"; got != want {
		t.Fatalf("auth key mismatch: want %q, got %q", want, got)
	}

	create := gotRequest.Capabilities.Devices.Create
	if create.Reusable {
		t.Fatal("Machine Auth Key should be one-off")
	}
	if create.Ephemeral {
		t.Fatal("Machine Auth Key should be non-ephemeral")
	}
	if !create.Preauthorized {
		t.Fatal("Machine Auth Key should be pre-approved")
	}
	if got, want := create.Tags, []string{personalServerTailscaleTag}; !reflect.DeepEqual(got, want) {
		t.Fatalf("auth key tags mismatch: want %#v, got %#v", want, got)
	}
	if got, want := gotRequest.ExpirySeconds, int64(600); got != want {
		t.Fatalf("auth key expiry mismatch: want %d, got %d", want, got)
	}
	if got, want := gotRequest.Description, personalServerTailscaleMachineAuthKeyDescription; got != want {
		t.Fatalf("auth key description mismatch: want %q, got %q", want, got)
	}
}
