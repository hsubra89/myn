# Tailscale-Only Personal Server Issues

Source: the Tailscale-only Personal Server decisions captured in the domain context and ADR-0006.

## Issue 1: Add Tailscale Credentials with `myn auth tailscale`

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Add first-class Tailscale authentication to Myn. `myn auth tailscale` should collect, validate, and persist Tailscale Credentials consisting of an API access token and tailnet identifier. It should work interactively, with environment variables for automation, and should validate that the token can perform every cloud API operation needed for Tailscale-only Personal Server provisioning.

## Acceptance criteria

- [x] `myn auth tailscale` is available under the existing `auth` command namespace.
- [x] Interactive auth opens the Tailscale Keys page, falls back to printing the URL if opening fails, and prompts for an API access token.
- [x] Non-interactive auth accepts `TAILSCALE_API_TOKEN` and supports `TAILSCALE_TAILNET` or an equivalent tailnet override when inference is ambiguous.
- [x] The command infers the tailnet when possible and prompts interactively when inference is ambiguous.
- [x] The command validates policy read, policy validation, safe no-op policy update, device listing, and auth key creation capability before saving credentials.
- [x] Only `auth.tailscale.token` and `auth.tailscale.tailnet` are saved.
- [x] Invalid, expired, or insufficient-scope tokens fail with specific messages and are not saved.
- [x] Tests cover successful save, invalid token, insufficient capability, env-driven auth, ambiguous tailnet, and config file permissions.

## Blocked by

None - can start immediately.

## Issue 2: Make Personal Server Connection Tailscale-only

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Change Personal Server connection planning so `myn connect`, `myn connect-new`, and `myn sessions` use the saved Tailscale Host directly through ordinary `ssh`. Remove SSH identity and public address requirements from the connection path, and make legacy public-SSH Personal Server Configurations fail with a clear migration message.

## Acceptance criteria

- [x] A complete connection config requires `personalServer.user` and `personalServer.tailscaleHost`.
- [x] `myn connect`, `myn connect-new`, and `myn sessions` call ordinary `ssh` with the Personal Server User and Tailscale Host.
- [x] SSH commands keep `StrictHostKeyChecking=accept-new` and do not pass `-i` or require an SSH identity file.
- [x] Public IPv4 and IPv6 are not used as fallback connection hosts.
- [x] Existing Project Session behavior, tmux handoff, session numbering, terminal validation, and quiet successful handoff behavior are preserved.
- [x] Legacy configs that have public addresses but no Tailscale Host fail with an explicit migration message.
- [x] Tests cover command construction, config validation, legacy migration failure, `connect`, `connect-new`, and `sessions`.

## Blocked by

None - can start immediately.

## Issue 3: Use LocalAPI for configure-time Tailscale preflight

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Before Personal Server creation, `myn configure` should verify the local Tailscale daemon is running and connected to the saved tailnet through Tailscale LocalAPI. The local `tailscale` CLI must not be required. The current Tailscale identity should become the default Personal Server User after Linux-safe normalization, while remaining editable.

## Acceptance criteria

- [x] `myn configure` uses Tailscale LocalAPI, not shelling out to the `tailscale` CLI, for local daemon status.
- [x] Configure fails before cloud resource creation when the local daemon is unavailable, disconnected, or unreadable.
- [x] Configure fails before cloud resource creation when the active local tailnet does not match saved Tailscale Credentials.
- [x] The Personal Server User default is derived from the current Tailscale identity and normalized with the existing Linux username rules.
- [x] The Personal Server User prompt remains editable and validated.
- [x] Configure skips Personal Server creation with a clear message when Tailscale Credentials are missing.
- [x] Tests cover daemon unavailable, disconnected, tailnet mismatch, successful identity-derived default, and editable override.

## Blocked by

- Issue 1: Add Tailscale Credentials with `myn auth tailscale`

## Issue 4: Plan and apply Tailnet Policy for Personal Servers

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Teach `myn configure` to compute, validate, summarize, and optionally apply the minimum Tailnet Policy required for a Tailscale-only Personal Server. The policy should be idempotent, use grants for network access, preserve unrelated policy content where practical, and apply only after final Personal Server creation confirmation.

## Acceptance criteria

- [x] Configure detects whether `tag:myn-personal-server` is owned by the current Tailscale identity.
- [x] Configure detects whether a grant already allows the current Tailscale identity to reach `tag:myn-personal-server` on port `22`.
- [x] Configure detects whether a Tailscale SSH rule already allows the current Tailscale identity to SSH to `tag:myn-personal-server` only as the selected Personal Server User with `checkPeriod: "always"`.
- [x] Missing policy pieces are represented as a concise semantic summary before confirmation.
- [x] When policy changes are needed, configure opens the Tailscale Access Controls page and falls back to printing the URL.
- [x] The current or proposed policy is validated before final Personal Server creation confirmation without mutating policy.
- [x] After final confirmation, configure re-reads, re-validates, and applies the policy with ETag protection before creating auth keys or cloud resources.
- [x] Policy edits use grants, do not migrate unrelated legacy ACLs, do not add policy tests, and are idempotent.
- [x] HuJSON comments and formatting are preserved where practical, and Myn-added entries include comments when practical.
- [x] Tests cover no-op policy, missing tag owner, missing grant, missing SSH rule, combined changes, validation failure, apply conflict, and declined policy edit.

## Blocked by

- Issue 1: Add Tailscale Credentials with `myn auth tailscale`
- Issue 3: Use LocalAPI for configure-time Tailscale preflight

## Issue 5: Create one-off Tailscale Machine Auth Keys for provisioning

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

After Tailnet Policy is confirmed and applied, `myn configure` should create a fresh one-off Tailscale Machine Auth Key for the new Personal Server. The key should be tagged, pre-approved, non-ephemeral, expire after ten minutes, be created immediately before the Hetzner create request, and never be saved or printed.

## Acceptance criteria

- [x] Configure creates a new auth key for each Personal Server creation attempt.
- [x] The key is one-off, tagged with `tag:myn-personal-server`, pre-approved, non-ephemeral, and expires after ten minutes.
- [x] The key is created only after final confirmation and required Tailnet Policy changes are applied.
- [x] The key is created immediately before rendering cloud-init and creating the Hetzner server.
- [x] The key is passed into bootstrap rendering but never saved to config or printed to command output.
- [x] Configure does not attempt to revoke the key after successful join.
- [x] Tests cover key request shape, ordering relative to policy and server creation, no persistence, no output leakage, and create-key failure before cloud resources.

## Blocked by

- Issue 4: Plan and apply Tailnet Policy for Personal Servers

## Issue 6: Render Tailscale-first Personal Server Bootstrap

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Update Personal Server Bootstrap cloud-init so Tailscale install and join happen first, Tailscale SSH is enabled, system OpenSSH is disabled, and the rest of the development environment installs through the existing bootstrap path. Remove local SSH key injection and Mosh from new bootstrap output.

## Acceptance criteria

- [x] Cloud-init no longer writes `ssh_authorized_keys` for root or the Personal Server User.
- [x] Bootstrap installs Tailscale before Docker, Homebrew, coding agents, and other development tools.
- [x] Bootstrap runs `tailscale up` with the generated Machine Auth Key, selected Tailscale Host, and Tailscale SSH enabled.
- [x] The Machine Auth Key is handled through a root-only file and removed after successful Tailscale join.
- [x] Tailscale install or join failure hard-fails bootstrap.
- [x] System OpenSSH is disabled after Tailscale SSH is enabled, and failure to disable it hard-fails bootstrap.
- [x] Mosh is not installed, not included in tool versions, and not mentioned in the install plan.
- [x] The bootstrap marker includes the installed Tailscale version.
- [x] Existing Docker, Homebrew, tmux, Git, GitHub CLI, rustup, Go, nvm, Node, Codex, Claude Code, Git identity, and tmux profile behavior is preserved.
- [x] Tests cover rendered cloud-init, rendered shell script, secret non-leakage, removal of Mosh and OpenSSH hardening profile behavior, marker versions, and valid Bash.

## Blocked by

- Issue 5: Create one-off Tailscale Machine Auth Keys for provisioning

## Issue 7: Provision IPv6-only Hetzner servers with no public ingress

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Change Hetzner provisioning so new Personal Servers are IPv6-only from the public network perspective and attach to a Myn-managed firewall with no public inbound rules. Save IPv6 as inventory only, add the Tailscale Host as an inventory label, and reconcile the reusable firewall safely.

## Acceptance criteria

- [x] Server creation disables public IPv4 and enables public IPv6.
- [x] Public IPv6 is saved in Personal Server Configuration as inventory only.
- [x] Public IPv4 is not saved for new Personal Servers.
- [x] Hetzner server labels include Myn ownership labels and the Tailscale Host for inventory.
- [x] Newly created `myn-personal-server` firewalls have no inbound rules.
- [x] Existing `myn-personal-server` firewalls with Myn labels are reconciled to no inbound rules.
- [x] Existing `myn-personal-server` firewalls without Myn labels cause provisioning to fail before server creation.
- [x] Output and final plan describe IPv6-only public networking and no public inbound access.
- [x] Tests cover server create request, saved config, labels, new firewall rules, managed firewall reconciliation, unmanaged firewall failure, and output.

## Blocked by

None - can start immediately.

## Issue 8: Orchestrate Tailscale-only configure completion

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Wire the full Tailscale-only provisioning sequence into `myn configure`: conflict checks, final plan, policy validation and mutation, auth key creation, Hetzner create, early config save, Tailscale device polling, SSH reachability, and bootstrap marker polling. Fail hard on Tailscale reachability or bootstrap failure while preserving the saved billable server identity.

## Acceptance criteria

- [x] Configure checks for duplicate Hetzner server name and duplicate Tailscale Host before policy mutation or auth key creation.
- [x] Final plan includes Location, Server Type, Tailscale Host, IPv6-only public networking, no public inbound firewall rules, Tailscale policy summary, and install plan.
- [x] After final confirmation, configure applies any required policy changes, creates the Machine Auth Key, renders cloud-init, and creates the Hetzner server in that order.
- [x] Configure saves `serverID`, Personal Server User, Tailscale Host, and public IPv6 immediately after Hetzner create actions finish.
- [x] Configure prints separate progress for Tailscale device registration, Tailscale SSH reachability, and bootstrap marker polling.
- [x] Configure waits up to eight minutes for device registration with hostname match, expected tag, authorized state, and online state.
- [x] Configure then verifies ordinary `ssh` reachability to the Tailscale Host as the Personal Server User.
- [x] After SSH reachability, configure waits five minutes for the existing bootstrap marker over SSH.
- [x] Tailscale join, SSH reachability, bootstrap timeout, and bootstrap failure all return errors without deleting the Hetzner server.
- [x] Successful completion prints Tailscale Host, public IPv6 inventory, installed tool versions, partial failures if any, and `myn connect`.
- [x] Tests cover happy path, duplicate conflicts, final confirmation decline, policy failure before cloud resources, auth key failure before cloud resources, early save, device timeout, unauthorized device, offline device, SSH timeout, marker failure, and successful report.

## Blocked by

- Issue 4: Plan and apply Tailnet Policy for Personal Servers
- Issue 5: Create one-off Tailscale Machine Auth Keys for provisioning
- Issue 6: Render Tailscale-first Personal Server Bootstrap
- Issue 7: Provision IPv6-only Hetzner servers with no public ingress

## Issue 9: Verify existing Tailscale Personal Server configuration

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Update existing-server verification in `myn configure` for Tailscale-only Personal Server Configuration. A configured server should be considered reusable only when the Hetzner server exists and the matching Tailscale device exists. Missing Tailscale devices should fail without automated repair, while legacy public-SSH configs should fail with a migration message.

## Acceptance criteria

- [x] Existing complete Tailscale Personal Server Configuration verifies the saved Hetzner server ID before skipping creation.
- [x] Existing complete Tailscale Personal Server Configuration verifies the saved Tailscale Host exists in the saved tailnet.
- [x] Configure reports saved server ID, Tailscale Host, public IPv6 inventory, and current Hetzner state when skipping creation.
- [x] If the Hetzner server exists but the Tailscale device is missing, configure fails with a manual repair or recreate message.
- [x] If the saved server ID is missing in Hetzner, existing stale-config behavior remains explicit and non-destructive.
- [x] Legacy public-SSH configs without Tailscale Host fail with a clear migration message and are not auto-cleared.
- [x] `myn connect` and `myn sessions` do not perform Hetzner or Tailscale API verification before connecting.
- [x] Tests cover verified existing server, missing Tailscale device, missing Hetzner server, legacy config migration, incomplete Tailscale config, and non-interactive behavior.

## Blocked by

- Issue 1: Add Tailscale Credentials with `myn auth tailscale`
- Issue 2: Make Personal Server Connection Tailscale-only
- Issue 8: Orchestrate Tailscale-only configure completion

## Issue 10: Update documentation and live validation for Tailscale-only provisioning

Type: HITL

Suggested label: `ready-for-human`

Status: Ready for Human (docs updated; live validation pending)

## What to build

Update user-facing documentation and live validation so the Tailscale-only provisioning flow is documented and tested against real Hetzner and Tailscale behavior. This slice includes the real smoke test because it needs valid Tailscale API access, tailnet policy permissions, local Tailscale daemon connectivity, and a billable Hetzner server.

## Acceptance criteria

- [x] Setup docs describe `myn auth hetzner`, `myn auth tailscale`, and `myn configure` in the new required order.
- [x] Docs explain that new Personal Servers are Tailscale-only, IPv6-only publicly, have no public inbound SSH or Mosh, and use Tailscale SSH through ordinary `ssh`.
- [x] Docs explain saved Tailscale Credentials versus one-off Tailscale Machine Auth Keys.
- [x] Docs explain required Tailscale API token capabilities and tailnet policy changes in user-facing language.
- [x] Docs describe the legacy public-SSH migration break and expected migration message.
- [ ] Live validation provisions an IPv6-only Tailscale Personal Server, verifies Tailscale device registration and SSH reachability, verifies bootstrap marker/tool setup, and cleans up the billable server.
- [ ] Live validation verifies the firewall has no public inbound rules and no public SSH or Mosh access is documented as available.
- [x] Live validation instructions document required Hetzner and Tailscale environment variables or secrets.

## Blocked by

- Issue 1: Add Tailscale Credentials with `myn auth tailscale`
- Issue 2: Make Personal Server Connection Tailscale-only
- Issue 3: Use LocalAPI for configure-time Tailscale preflight
- Issue 4: Plan and apply Tailnet Policy for Personal Servers
- Issue 5: Create one-off Tailscale Machine Auth Keys for provisioning
- Issue 6: Render Tailscale-first Personal Server Bootstrap
- Issue 7: Provision IPv6-only Hetzner servers with no public ingress
- Issue 8: Orchestrate Tailscale-only configure completion
- Issue 9: Verify existing Tailscale Personal Server configuration

## Issue 11: Remove local SSH identity from Personal Server provisioning

Type: AFK

Suggested label: `ready-for-agent`

Status: Done

## What to build

Finish removing the legacy local SSH identity dependency from new Personal Server creation. `myn configure` should not require or generate a local SSH identity before entering Personal Server provisioning, and Hetzner server creation should not upload or attach a Myn SSH Key. This completes the PRD objective that provisioning is tied to Tailscale identity rather than local key material.

## Acceptance criteria

- [x] Personal Server creation is not skipped when `ssh.identityFile` is empty, as long as Hetzner Credentials, Tailscale Credentials, and LocalAPI preflight are valid.
- [x] Interactive configure no longer prompts to generate, select, add, or validate an SSH identity solely for Personal Server provisioning.
- [x] Hetzner server creation does not create, reuse, upload, or attach a `myn-personal-server` SSH Key for new Tailscale-only Personal Servers.
- [x] The final creation plan does not show an SSH key as part of the Personal Server access model.
- [x] Personal Server Bootstrap input has no SSH public key field and does not depend on configured SSH identity state.
- [x] Existing project root, Git identity, Personal Server User, Tailscale Host, Tailnet Policy, Machine Auth Key, and cloud-init rendering behavior is preserved.
- [x] Tests cover missing SSH identity with successful preview, server create request without SSH key, no SSH-key output, and no regression to connection command behavior.

## Blocked by

- Issue 6: Render Tailscale-first Personal Server Bootstrap
