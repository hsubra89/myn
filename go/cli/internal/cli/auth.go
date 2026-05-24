package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const tailscaleKeysURL = "https://login.tailscale.com/admin/settings/keys"

type hetznerAuthOptions struct {
	token             string
	fromHcloudContext string
}

type tailscaleAuthOptions struct {
	token   string
	tailnet string
}

type hetznerAuthDeps struct {
	appConfigPath    func() (string, error)
	hcloudConfigPath func() (string, error)
	env              func(string) string
	validateToken    func(context.Context, string) error
	prompter         hetznerAuthPrompter
}

type tailscaleAuthDeps struct {
	appConfigPath       func() (string, error)
	env                 func(string) string
	validateCredentials func(context.Context, tailscaleCredentials) error
	inferTailnets       func(context.Context, string) ([]string, error)
	openURL             func(string) error
	prompter            tailscaleAuthPrompter
}

type tailscaleCredentials struct {
	Token   string
	Tailnet string
}

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	cmd.AddCommand(newHetznerAuthCommand())
	cmd.AddCommand(newTailscaleAuthCommand())

	return cmd
}

func newHetznerAuthCommand() *cobra.Command {
	var opts hetznerAuthOptions

	cmd := &cobra.Command{
		Use:   "hetzner",
		Short: "Configure Hetzner Cloud authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHetznerAuth(cmd.Context(), cmd.OutOrStdout(), opts, defaultHetznerAuthDeps())
		},
	}

	cmd.Flags().StringVar(&opts.token, "token", "", "Hetzner API token to validate and save")
	cmd.Flags().StringVar(&opts.fromHcloudContext, "from-hcloud-context", "", "Import a token from a named hcloud context")

	return cmd
}

func newTailscaleAuthCommand() *cobra.Command {
	var opts tailscaleAuthOptions

	cmd := &cobra.Command{
		Use:   "tailscale",
		Short: "Configure Tailscale authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTailscaleAuth(cmd.Context(), cmd.OutOrStdout(), opts, defaultTailscaleAuthDeps())
		},
	}

	cmd.Flags().StringVar(&opts.token, "token", "", "Tailscale API access token to validate and save")
	cmd.Flags().StringVar(&opts.tailnet, "tailnet", "", "Tailscale tailnet identifier to validate and save")

	return cmd
}

func defaultHetznerAuthDeps() hetznerAuthDeps {
	env := os.Getenv
	validator := newHetznerValidator(env("HCLOUD_ENDPOINT"))

	return hetznerAuthDeps{
		appConfigPath: func() (string, error) {
			return defaultAppConfigPath(env)
		},
		hcloudConfigPath: func() (string, error) {
			return defaultHcloudConfigPath(env)
		},
		env:           env,
		validateToken: validator.validate,
		prompter:      huhPrompter{},
	}
}

func defaultTailscaleAuthDeps() tailscaleAuthDeps {
	env := os.Getenv
	validator := newTailscaleCloudValidator(env("TAILSCALE_ENDPOINT"))
	discoverer := newTailscaleTailnetDiscoverer(env("TAILSCALE_ENDPOINT"))

	return tailscaleAuthDeps{
		appConfigPath: func() (string, error) {
			return defaultAppConfigPath(env)
		},
		env:                 env,
		validateCredentials: validator.validate,
		inferTailnets:       discoverer.inferTailnets,
		openURL:             openBrowserURL,
		prompter:            huhPrompter{},
	}
}

func runHetznerAuth(ctx context.Context, out io.Writer, opts hetznerAuthOptions, deps hetznerAuthDeps) error {
	deps = fillHetznerAuthDeps(deps)

	if opts.token != "" && opts.fromHcloudContext != "" {
		return fmt.Errorf("--token and --from-hcloud-context cannot be used together")
	}

	appConfigPath, err := deps.appConfigPath()
	if err != nil {
		return err
	}

	if opts.token != "" {
		return validateSaveAndReport(ctx, out, appConfigPath, deps, strings.TrimSpace(opts.token), "")
	}

	if opts.fromHcloudContext != "" {
		return importHcloudContextByName(ctx, out, appConfigPath, opts.fromHcloudContext, deps)
	}

	if token := strings.TrimSpace(deps.env("HCLOUD_TOKEN")); token != "" {
		return validateSaveAndReport(ctx, out, appConfigPath, deps, token, "")
	}

	cfg, err := loadAppConfig(appConfigPath)
	if err != nil {
		return err
	}

	if token := strings.TrimSpace(cfg.Auth.Hetzner.Token); token != "" {
		if err := deps.validateToken(ctx, token); err == nil {
			if !deps.prompter.CanPrompt() {
				fmt.Fprintln(out, "Hetzner authentication is already configured.")
				return nil
			}

			keep, err := deps.prompter.Confirm("Hetzner authentication is already configured. Keep existing credentials?", true)
			if err != nil {
				return err
			}
			if keep {
				fmt.Fprintln(out, "Hetzner authentication is already configured.")
				return nil
			}
		} else {
			reportExistingTokenFailure(out, err)
		}
	}

	if imported, err := maybeImportHcloudContext(ctx, out, appConfigPath, deps); err != nil {
		return err
	} else if imported {
		return nil
	}

	if !deps.prompter.CanPrompt() {
		return fmt.Errorf("interactive Hetzner authentication requires a terminal; pass --token, set HCLOUD_TOKEN, or use --from-hcloud-context")
	}

	token, err := deps.prompter.InputToken()
	if err != nil {
		return err
	}
	return validateSaveAndReport(ctx, out, appConfigPath, deps, strings.TrimSpace(token), "")
}

func runTailscaleAuth(ctx context.Context, out io.Writer, opts tailscaleAuthOptions, deps tailscaleAuthDeps) error {
	deps = fillTailscaleAuthDeps(deps)

	appConfigPath, err := deps.appConfigPath()
	if err != nil {
		return err
	}

	token := firstNonEmpty(strings.TrimSpace(opts.token), strings.TrimSpace(deps.env("TAILSCALE_API_TOKEN")))
	tailnet := firstNonEmpty(strings.TrimSpace(opts.tailnet), strings.TrimSpace(deps.env("TAILSCALE_TAILNET")))

	if token == "" {
		if !deps.prompter.CanPrompt() {
			return fmt.Errorf("interactive Tailscale authentication requires a terminal; pass --token or set TAILSCALE_API_TOKEN")
		}
		if err := deps.openURL(tailscaleKeysURL); err != nil {
			fmt.Fprintf(out, "Open %s to create a Tailscale API access token.\n", tailscaleKeysURL)
		}
		inputToken, err := deps.prompter.InputTailscaleToken()
		if err != nil {
			return err
		}
		token = strings.TrimSpace(inputToken)
	}

	if tailnet == "" {
		var inferErr error
		inferred, inferErr := deps.inferTailnets(ctx, token)
		if inferErr == nil {
			switch len(inferred) {
			case 1:
				tailnet = strings.TrimSpace(inferred[0])
			default:
				if len(inferred) > 1 {
					if !deps.prompter.CanPrompt() {
						return fmt.Errorf("Tailscale tailnet is ambiguous; pass --tailnet or set TAILSCALE_TAILNET")
					}
					selected, err := deps.prompter.SelectTailnet(inferred)
					if err != nil {
						return err
					}
					tailnet = strings.TrimSpace(selected)
				}
			}
		}
		if tailnet == "" {
			if !deps.prompter.CanPrompt() {
				if inferErr != nil {
					return fmt.Errorf("Tailscale tailnet could not be inferred: %v; pass --tailnet or set TAILSCALE_TAILNET", inferErr)
				}
				return fmt.Errorf("Tailscale tailnet could not be inferred; pass --tailnet or set TAILSCALE_TAILNET")
			}
			inputTailnet, err := deps.prompter.InputTailnet()
			if err != nil {
				return err
			}
			tailnet = strings.TrimSpace(inputTailnet)
		}
	}

	credentials := tailscaleCredentials{
		Token:   strings.TrimSpace(token),
		Tailnet: strings.TrimSpace(tailnet),
	}
	if err := deps.validateCredentials(ctx, credentials); err != nil {
		return fmt.Errorf("Tailscale token validation failed: %s", err)
	}
	if err := saveTailscaleCredentials(appConfigPath, credentials); err != nil {
		return err
	}

	fmt.Fprintln(out, "Saved Tailscale credentials.")
	return nil
}

func fillHetznerAuthDeps(deps hetznerAuthDeps) hetznerAuthDeps {
	if deps.env == nil {
		deps.env = os.Getenv
	}
	if deps.appConfigPath == nil {
		deps.appConfigPath = func() (string, error) {
			return defaultAppConfigPath(deps.env)
		}
	}
	if deps.hcloudConfigPath == nil {
		deps.hcloudConfigPath = func() (string, error) {
			return defaultHcloudConfigPath(deps.env)
		}
	}
	if deps.validateToken == nil {
		validator := newHetznerValidator(deps.env("HCLOUD_ENDPOINT"))
		deps.validateToken = validator.validate
	}
	if deps.prompter == nil {
		deps.prompter = huhPrompter{}
	}
	return deps
}

func fillTailscaleAuthDeps(deps tailscaleAuthDeps) tailscaleAuthDeps {
	if deps.env == nil {
		deps.env = os.Getenv
	}
	if deps.appConfigPath == nil {
		deps.appConfigPath = func() (string, error) {
			return defaultAppConfigPath(deps.env)
		}
	}
	if deps.validateCredentials == nil {
		validator := newTailscaleCloudValidator(deps.env("TAILSCALE_ENDPOINT"))
		deps.validateCredentials = validator.validate
	}
	if deps.inferTailnets == nil {
		discoverer := newTailscaleTailnetDiscoverer(deps.env("TAILSCALE_ENDPOINT"))
		deps.inferTailnets = discoverer.inferTailnets
	}
	if deps.openURL == nil {
		deps.openURL = openBrowserURL
	}
	if deps.prompter == nil {
		deps.prompter = huhPrompter{}
	}
	return deps
}

func importHcloudContextByName(ctx context.Context, out io.Writer, appConfigPath string, name string, deps hetznerAuthDeps) error {
	hcloudConfigPath, err := deps.hcloudConfigPath()
	if err != nil {
		return err
	}

	candidates, err := loadHcloudTokenCandidates(hcloudConfigPath)
	if err != nil {
		return err
	}

	candidate, ok := findHcloudTokenCandidate(candidates, name)
	if !ok {
		return fmt.Errorf("hcloud context %q was not found", name)
	}

	return validateSaveAndReport(ctx, out, appConfigPath, deps, candidate.Token, candidate.Name)
}

func maybeImportHcloudContext(ctx context.Context, out io.Writer, appConfigPath string, deps hetznerAuthDeps) (bool, error) {
	hcloudConfigPath, err := deps.hcloudConfigPath()
	if err != nil {
		fmt.Fprintf(out, "Could not check hcloud config: %v\n", err)
		return false, nil
	}

	candidates, err := loadHcloudTokenCandidates(hcloudConfigPath)
	if err != nil {
		fmt.Fprintf(out, "Could not check hcloud config: %v\n", err)
		return false, nil
	}
	if len(candidates) == 0 {
		return false, nil
	}

	if !deps.prompter.CanPrompt() {
		return false, nil
	}

	candidate, selected, err := chooseHcloudContext(candidates, deps.prompter)
	if err != nil {
		return false, err
	}
	if !selected {
		return false, nil
	}

	if err := deps.validateToken(ctx, candidate.Token); err != nil {
		reportHcloudTokenFailure(out, candidate.Name, err)
		return false, nil
	}

	if err := saveHetznerToken(appConfigPath, candidate.Token); err != nil {
		return false, err
	}
	fmt.Fprintf(out, "Saved Hetzner credentials from hcloud context %q.\n", candidate.Name)
	return true, nil
}

func chooseHcloudContext(candidates []hcloudTokenCandidate, prompter hetznerAuthPrompter) (hcloudTokenCandidate, bool, error) {
	if len(candidates) == 1 {
		candidate := candidates[0]
		use, err := prompter.Confirm(fmt.Sprintf("Use hcloud context %q token?", candidate.Name), true)
		if err != nil {
			return hcloudTokenCandidate{}, false, err
		}
		if !use {
			return hcloudTokenCandidate{}, false, nil
		}
		return candidate, true, nil
	}

	return prompter.SelectHcloudContext(candidates)
}

func validateSaveAndReport(ctx context.Context, out io.Writer, appConfigPath string, deps hetznerAuthDeps, token string, hcloudContextName string) error {
	if err := deps.validateToken(ctx, token); err != nil {
		return fmt.Errorf("Hetzner token validation failed: %s", err)
	}

	if err := saveHetznerToken(appConfigPath, token); err != nil {
		return err
	}

	if hcloudContextName != "" {
		fmt.Fprintf(out, "Saved Hetzner credentials from hcloud context %q.\n", hcloudContextName)
	} else {
		fmt.Fprintln(out, "Saved Hetzner credentials.")
	}

	return nil
}

func saveHetznerToken(path string, token string) error {
	cfg, err := loadAppConfig(path)
	if err != nil {
		return err
	}

	cfg.Auth.Hetzner.Token = token
	return saveAppConfig(path, cfg)
}

func saveTailscaleCredentials(path string, credentials tailscaleCredentials) error {
	cfg, err := loadAppConfig(path)
	if err != nil {
		return err
	}

	cfg.Auth.Tailscale.Token = credentials.Token
	cfg.Auth.Tailscale.Tailnet = credentials.Tailnet
	return saveAppConfig(path, cfg)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func openBrowserURL(rawURL string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{rawURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command = "xdg-open"
		args = []string{rawURL}
	}
	return exec.Command(command, args...).Start()
}

func reportExistingTokenFailure(out io.Writer, err error) {
	if validationTimedOut(err) {
		fmt.Fprintln(out, "Tried the existing Hetzner token, but it did not validate within 4s.")
		return
	}
	fmt.Fprintln(out, "Tried the existing Hetzner token, but it did not validate.")
}

func reportHcloudTokenFailure(out io.Writer, contextName string, err error) {
	if validationTimedOut(err) {
		fmt.Fprintf(out, "Tried the hcloud context %q token, but it did not validate within 4s.\n", contextName)
		return
	}
	fmt.Fprintf(out, "Tried the hcloud context %q token, but it did not validate.\n", contextName)
}
