package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultHetznerEndpoint = "https://api.hetzner.cloud/v1"
	hetznerValidationLimit = 4 * time.Second
)

type hetznerValidator struct {
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

type hetznerValidationError struct {
	reason  string
	timeout bool
}

func (e hetznerValidationError) Error() string {
	return e.reason
}

func (e hetznerValidationError) Timeout() bool {
	return e.timeout
}

func newHetznerValidator(endpoint string) hetznerValidator {
	if endpoint == "" {
		endpoint = defaultHetznerEndpoint
	}
	return hetznerValidator{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: hetznerValidationLimit},
		timeout:  hetznerValidationLimit,
	}
}

func (v hetznerValidator) validate(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return hetznerValidationError{reason: "token is empty"}
	}

	validateCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(validateCtx, http.MethodGet, v.endpoint+"/locations", nil)
	if err != nil {
		return fmt.Errorf("build Hetzner validation request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		if validateCtx.Err() != nil || os.IsTimeout(err) {
			return hetznerValidationError{reason: "did not validate within 4s", timeout: true}
		}
		return hetznerValidationError{reason: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return hetznerValidationError{reason: fmt.Sprintf("API rejected the token (%s)", resp.Status)}
	default:
		return hetznerValidationError{reason: fmt.Sprintf("Hetzner API returned %s", resp.Status)}
	}
}

func validationTimedOut(err error) bool {
	var validationErr hetznerValidationError
	if err != nil && errors.As(err, &validationErr) {
		return validationErr.Timeout()
	}

	return false
}
