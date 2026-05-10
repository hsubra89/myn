package cli

import (
	"fmt"
	"io"
	"strings"
)

type personalServerProvisioner interface {
	Configure(out io.Writer, cfg appConfig, canPrompt bool) error
}

type personalServerProvisionerFunc func(io.Writer, appConfig, bool) error

func (fn personalServerProvisionerFunc) Configure(out io.Writer, cfg appConfig, canPrompt bool) error {
	return fn(out, cfg, canPrompt)
}

type personalServerProvisioningGate struct{}

func (personalServerProvisioningGate) Configure(out io.Writer, cfg appConfig, canPrompt bool) error {
	if strings.TrimSpace(cfg.SSH.IdentityFile) == "" {
		fmt.Fprintln(out, "Personal Server creation skipped: SSH identity is not configured.")
		return nil
	}
	if strings.TrimSpace(cfg.Auth.Hetzner.Token) == "" {
		fmt.Fprintln(out, "Personal Server creation skipped: Hetzner Credentials are not configured. Run `me auth hetzner` first.")
		return nil
	}
	if !canPrompt {
		fmt.Fprintln(out, "Personal Server creation skipped: configure is running non-interactively.")
		return nil
	}

	fmt.Fprintln(out, "Personal Server provisioning prerequisites are ready.")
	return nil
}
