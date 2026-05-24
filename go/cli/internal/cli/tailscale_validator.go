package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	tailscale "tailscale.com/client/tailscale/v2"
)

const (
	defaultTailscaleEndpoint        = "https://api.tailscale.com"
	tailscaleValidationLimit        = 10 * time.Second
	tailscaleValidationKeyLifetime  = 10 * time.Minute
	tailscaleValidationKeyComment   = "myn auth validation"
	tailscaleValidationEmptyPolicy  = "{}"
	tailscaleValidationUserAgent    = "myn"
	tailscaleValidationAuthKeyScope = "auth key creation"
)

type tailscaleCloudValidator struct {
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

type tailscaleTailnetDiscoverer struct {
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

func newTailscaleCloudValidator(endpoint string) tailscaleCloudValidator {
	return tailscaleCloudValidator{
		endpoint: normalizeTailscaleEndpoint(endpoint),
		client:   &http.Client{Timeout: tailscaleValidationLimit},
		timeout:  tailscaleValidationLimit,
	}
}

func newTailscaleTailnetDiscoverer(endpoint string) tailscaleTailnetDiscoverer {
	return tailscaleTailnetDiscoverer{
		endpoint: normalizeTailscaleEndpoint(endpoint),
		client:   &http.Client{Timeout: tailscaleValidationLimit},
		timeout:  tailscaleValidationLimit,
	}
}

func normalizeTailscaleEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return defaultTailscaleEndpoint
	}
	return strings.TrimRight(endpoint, "/")
}

func (v tailscaleCloudValidator) validate(ctx context.Context, credentials tailscaleCredentials) error {
	if strings.TrimSpace(credentials.Token) == "" {
		return fmt.Errorf("token is empty")
	}
	if strings.TrimSpace(credentials.Tailnet) == "" {
		return fmt.Errorf("tailnet is empty")
	}

	validateCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	client, err := v.newClient(credentials.Token, credentials.Tailnet)
	if err != nil {
		return err
	}

	rawPolicy, err := client.PolicyFile().Raw(validateCtx)
	if err != nil {
		return mapTailscaleValidationError(validateCtx, "policy read", err)
	}

	policy := rawPolicy.HuJSON
	if strings.TrimSpace(policy) == "" {
		policy = tailscaleValidationEmptyPolicy
	}

	if err := client.PolicyFile().Validate(validateCtx, policy); err != nil {
		return mapTailscaleValidationError(validateCtx, "policy validation", err)
	}

	if err := client.PolicyFile().Set(validateCtx, policy, rawPolicy.ETag); err != nil {
		return mapTailscaleValidationError(validateCtx, "safe no-op policy update", err)
	}

	if _, err := client.Devices().List(validateCtx); err != nil {
		return mapTailscaleValidationError(validateCtx, "device listing", err)
	}

	if err := validateTailscaleAuthKeyCreation(validateCtx, client); err != nil {
		return mapTailscaleValidationError(validateCtx, tailscaleValidationAuthKeyScope, err)
	}

	return nil
}

func (v tailscaleCloudValidator) newClient(token string, tailnet string) (*tailscale.Client, error) {
	return newTailscaleAPIClient(v.endpoint, v.client, token, tailnet)
}

func (d tailscaleTailnetDiscoverer) inferTailnets(ctx context.Context, token string) ([]string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("token is empty")
	}

	inferCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	client, err := newTailscaleAPIClient(d.endpoint, d.client, token, "-")
	if err != nil {
		return nil, err
	}
	users, err := client.Users().List(inferCtx, nil, nil)
	if err != nil {
		return nil, mapTailscaleValidationError(inferCtx, "tailnet inference", err)
	}

	tailnetsByID := make(map[string]struct{})
	for _, user := range users {
		tailnetID := strings.TrimSpace(user.TailnetID)
		if tailnetID != "" {
			tailnetsByID[tailnetID] = struct{}{}
		}
	}

	tailnets := make([]string, 0, len(tailnetsByID))
	for tailnetID := range tailnetsByID {
		tailnets = append(tailnets, tailnetID)
	}
	slices.Sort(tailnets)
	return tailnets, nil
}

func newTailscaleAPIClient(endpoint string, httpClient *http.Client, token string, tailnet string) (*tailscale.Client, error) {
	baseURL, err := url.Parse(normalizeTailscaleEndpoint(endpoint))
	if err != nil {
		return nil, fmt.Errorf("parse Tailscale endpoint: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: tailscaleValidationLimit}
	}

	return &tailscale.Client{
		BaseURL:   baseURL,
		APIKey:    strings.TrimSpace(token),
		Tailnet:   strings.TrimSpace(tailnet),
		HTTP:      httpClient,
		UserAgent: tailscaleValidationUserAgent,
	}, nil
}

func validateTailscaleAuthKeyCreation(ctx context.Context, client *tailscale.Client) error {
	capabilities := tailscale.KeyCapabilities{}
	capabilities.Devices.Create.Reusable = false
	capabilities.Devices.Create.Ephemeral = true
	capabilities.Devices.Create.Preauthorized = false

	key, err := client.Keys().CreateAuthKey(ctx, tailscale.CreateKeyRequest{
		Capabilities:  capabilities,
		ExpirySeconds: int64(tailscaleValidationKeyLifetime.Seconds()),
		Description:   tailscaleValidationKeyComment,
	})
	if err != nil {
		return err
	}
	if key != nil && strings.TrimSpace(key.ID) != "" {
		_ = client.Keys().Delete(ctx, key.ID)
	}
	return nil
}

func mapTailscaleValidationError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil || os.IsTimeout(err) {
		return fmt.Errorf("%s did not validate within 10s", operation)
	}

	var apiErr tailscale.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusUnauthorized:
			if strings.TrimSpace(apiErr.Message) != "" {
				return fmt.Errorf("API rejected the token while checking %s: %s", operation, apiErr.Message)
			}
			return fmt.Errorf("API rejected the token while checking %s", operation)
		case http.StatusForbidden:
			if strings.TrimSpace(apiErr.Message) != "" {
				return fmt.Errorf("token is missing %s capability: %s", operation, apiErr.Message)
			}
			return fmt.Errorf("token is missing %s capability", operation)
		default:
			if strings.TrimSpace(apiErr.Message) != "" {
				return fmt.Errorf("%s failed: %s (%d)", operation, apiErr.Message, apiErr.Status)
			}
			return fmt.Errorf("%s failed with HTTP %d", operation, apiErr.Status)
		}
	}

	return fmt.Errorf("%s failed: %v", operation, err)
}
