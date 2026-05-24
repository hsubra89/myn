# Tailscale-Only Personal Server PRD

## Problem Statement

Myn's Personal Server provisioning still assumes public SSH access, local SSH key material, and Mosh-oriented network setup. That model is no longer acceptable for new Personal Servers. A user wants every new Personal Server to be reachable only through Tailscale SSH, with no public inbound SSH or Mosh exposure, no local SSH public key requirement, and no manual machine-key copy/paste step during provisioning.

The current flow also makes access setup feel split across several trust boundaries: Hetzner creates the server, cloud-init installs tooling, SSH hardening happens afterward, and connection code later chooses a public host. The desired model is stricter and simpler from the user's perspective: Myn should authenticate to Tailscale once, validate the required tailnet capabilities, prepare policy before any billable server is created, create a short-lived one-off machine auth key automatically, let cloud-init join Tailscale first, disable system OpenSSH, and then connect through ordinary `ssh` to the Tailscale hostname.

## Solution

Myn will make Tailscale mandatory for newly provisioned Personal Servers. The user will run `myn auth tailscale` to save Tailscale Credentials consisting only of an API access token and tailnet identifier. The command will open the Tailscale Keys page for token creation, validate the token through the Tailscale cloud API, and save it only when it has the capabilities required for provisioning.

During interactive `myn configure`, Myn will require both Hetzner Credentials and Tailscale Credentials before creating a Personal Server. It will verify the local Tailscale daemon through LocalAPI, confirm the active local tailnet matches the saved tailnet, derive an editable Personal Server User default from the current Tailscale identity, check for duplicate Hetzner and Tailscale hostnames, compute required Tailnet Policy changes, validate the current or proposed policy, and show the full creation plan before final confirmation.

After final confirmation, Myn will apply required Tailnet Policy changes with ETag protection, create a fresh one-off Tailscale Machine Auth Key, render Tailscale-first cloud-init, and create an IPv6-only Hetzner server with no public inbound firewall rules. Cloud-init will install and join Tailscale first, enable Tailscale SSH, remove the auth key material, disable system OpenSSH, and then run the existing development bootstrap without installing Mosh or injecting local SSH keys.

Myn will save the created Personal Server identity as soon as Hetzner create actions complete, then wait for Tailscale device registration, expected tag, authorization, online status, ordinary SSH reachability through the Tailscale Host, and the existing bootstrap marker. `myn connect`, `myn connect-new`, and `myn sessions` will use ordinary `ssh` to the saved Tailscale Host and will not fall back to public IPv4, public IPv6, or configured SSH identities.

## User Stories

1. As a Myn user, I want new Personal Servers to use Tailscale-only access, so that public SSH is not exposed.
2. As a Myn user, I want Tailscale to be mandatory for new Personal Servers, so that every new server follows the same access model.
3. As a Myn user, I want `myn auth tailscale` to save my Tailscale API access token and tailnet, so that Myn can automate Tailscale provisioning work.
4. As a Myn user, I want `myn auth tailscale` to open the Tailscale Keys page, so that I can create and copy an API access token without hunting for the page.
5. As a Myn user, I want `myn auth tailscale` to print the Tailscale Keys URL if opening a browser fails, so that I still know where to create the token.
6. As a Myn user, I want Tailscale auth to validate the token before saving it, so that I do not discover missing permissions halfway through server creation.
7. As a Myn user, I want Tailscale auth to save only the API token and tailnet, so that one-off machine auth keys are never persisted as long-lived configuration.
8. As a Myn user running automation, I want Tailscale auth to accept environment-provided token and tailnet values, so that setup can run without prompts when inputs are unambiguous.
9. As a Myn user, I want Tailscale auth to reject insufficient-scope tokens with a clear message, so that I know how to create a usable token.
10. As a Myn user, I want `myn configure` to verify the local Tailscale daemon through LocalAPI, so that a macOS GUI Tailscale install works even when the `tailscale` CLI is unavailable.
11. As a Myn user, I want `myn configure` to fail before cloud resource creation when the local Tailscale daemon is unavailable, so that I do not create an unreachable billable server.
12. As a Myn user, I want `myn configure` to verify that my local tailnet matches the saved Tailscale Credentials, so that server access is configured in the tailnet I am actually using.
13. As a Myn user, I want the Personal Server User to default from my current Tailscale identity, so that the remote username naturally matches who is allowed to access the server.
14. As a Myn user, I want the Personal Server User prompt to remain editable, so that I can choose a Linux-safe username that fits my conventions.
15. As a Myn user, I want the Personal Server User to have password-backed sudo, so that privileged access exists without passwordless sudo.
16. As a Myn user, I want Myn to check whether required Tailnet Policy already exists, so that it does not make unnecessary policy changes.
17. As a Myn user, I want Myn to summarize missing Tailnet Policy pieces before editing, so that I understand the access it is about to grant.
18. As a Myn user, I want Myn to ask before editing Tailnet Policy through the API, so that policy mutation remains explicit.
19. As a Myn user, I want Myn to open the Tailscale Access Controls page when policy changes are needed, so that I can inspect the policy in Tailscale directly.
20. As a Myn user, I want Myn to validate the proposed Tailnet Policy before final server creation confirmation, so that invalid policy does not create a stranded server.
21. As a Myn user, I want Tailnet Policy changes applied only after final confirmation, so that reviewing the plan does not mutate my tailnet.
22. As a Myn user, I want Tailnet Policy edits to use grants for network access, so that the new access model uses Tailscale's current policy primitive.
23. As a Myn user, I want the Tailscale SSH rule to allow only my identity and only the selected Personal Server User, so that Tailscale SSH access is narrow.
24. As a Myn user, I want the Tailscale SSH rule to use `checkPeriod: "always"`, so that Tailscale continues to enforce check requirements for SSH sessions.
25. As a Myn user, I want Tailnet Policy edits to preserve unrelated ACLs and policy content, so that Myn does not rewrite my broader tailnet configuration.
26. As a Myn user, I want policy writes to use ETag protection, so that concurrent policy edits are not silently overwritten.
27. As a Myn user, I want Myn to create a fresh one-off Tailscale Machine Auth Key for each provisioning attempt, so that server join credentials are short-lived and single-use.
28. As a Myn user, I want the machine auth key to be tagged, pre-approved, non-ephemeral, and expire after ten minutes, so that the server joins with the right identity and limited credential lifetime.
29. As a Myn user, I want the machine auth key to be created immediately before server creation, so that as much of its ten-minute lifetime as possible is available for bootstrap.
30. As a Myn user, I want the machine auth key to never be printed or saved, so that it does not leak through logs or config files.
31. As a Myn user, I want cloud-init to install and join Tailscale first, so that the server's primary access path exists before the rest of bootstrap runs.
32. As a Myn user, I want cloud-init to enable Tailscale SSH during `tailscale up`, so that Myn can later connect using ordinary SSH through Tailscale.
33. As a Myn user, I want cloud-init to disable system OpenSSH after Tailscale SSH is enabled, so that public SSH is not left running as a fallback path.
34. As a Myn user, I want bootstrap to hard-fail if Tailscale join or OpenSSH disablement fails, so that the success marker never hides a broken access model.
35. As a Myn user, I want no local SSH public key to be required or injected, so that provisioning is tied to Tailscale identity rather than local key material.
36. As a Myn user, I want Mosh removed from new Personal Server provisioning, so that no Mosh package, firewall rule, command, or marker entry remains.
37. As a Myn user, I want the existing development tool bootstrap to continue after Tailscale setup, so that the server is still ready for project work.
38. As a Myn user, I want new Hetzner servers to disable public IPv4 and enable public IPv6, so that public networking is minimal while bootstrap still has outbound connectivity.
39. As a Myn user, I want the public IPv6 address saved only as inventory, so that it is not mistaken for a supported connection endpoint.
40. As a Myn user, I want the Personal Server Firewall to have no public inbound rules, so that the Hetzner perimeter matches the Tailscale-only model.
41. As a Myn user, I want Myn-managed firewalls reconciled safely and unmanaged firewalls protected, so that reuse does not clobber unrelated firewall rules.
42. As a Myn user, I want server labels to include Myn ownership and Tailscale Host inventory, so that cloud resources are understandable in Hetzner.
43. As a Myn user, I want Myn to save the server ID, Personal Server User, Tailscale Host, and IPv6 inventory after Hetzner creation, so that I retain the billable server identity even if bootstrap fails.
44. As a Myn user, I want Myn to wait for Tailscale device registration, tag, authorization, online state, and SSH reachability, so that completion means the server is actually reachable through Tailscale.
45. As a Myn user, I want Myn to read the bootstrap marker over Tailscale SSH as the Personal Server User, so that bootstrap completion is verified through the real access path.
46. As a Myn user, I want separate progress messages for Tailscale registration, SSH reachability, and bootstrap marker polling, so that I can tell where provisioning is waiting.
47. As a Myn user, I want reachability and bootstrap failures to hard-fail without deleting the server, so that I can inspect or clean up the billable resource myself.
48. As a Myn user, I want successful provisioning output to show the Tailscale Host and `myn connect`, so that the next action is clear.
49. As a Myn user, I want `myn connect` to use ordinary `ssh` to the Tailscale Host, so that Tailscale SSH interception and policy enforce access without requiring a special `tailscale ssh` command.
50. As a Myn user, I want `myn connect-new` and `myn sessions` to use the same Tailscale-only connection config, so that all Personal Server entry points behave consistently.
51. As a Myn user, I want connection commands to preserve existing tmux session behavior, so that the access model changes without disrupting project workflow.
52. As a Myn user, I want legacy public-SSH Personal Server configurations to fail with a migration message, so that old access assumptions do not silently continue.
53. As a Myn user, I want `myn configure` to verify existing Tailscale Personal Server Configuration before skipping creation, so that stale saved state is caught.
54. As a Myn user, I want missing Tailscale devices for existing servers to fail without automated repair, so that Myn does not guess how to repair identity-bearing access state.
55. As a Myn maintainer, I want the Tailscale policy planner to be testable apart from CLI prompts, so that policy behavior can be validated without live API calls.
56. As a Myn maintainer, I want the cloud-init renderer to be testable apart from Hetzner creation, so that secret handling and bootstrap ordering can be verified deterministically.
57. As a Myn maintainer, I want Tailscale cloud API, LocalAPI, Hetzner, SSH, and prompt boundaries behind narrow interfaces, so that provisioning orchestration can be tested with fakes.
58. As a Myn maintainer, I want docs and live validation to describe the new access model, so that users and future agents understand the intended behavior.

## Implementation Decisions

- New Personal Servers are Tailscale-only. Public IPv4, public IPv6, system OpenSSH, Mosh, and configured SSH identities are not fallback access paths.
- The legacy public-SSH Personal Server model is a hard migration break. Existing configs without a Tailscale Host fail with a migration message rather than being silently repaired, cleared, or used.
- Tailscale Credentials are saved under auth configuration as an API access token and tailnet identifier only. Machine auth keys are separate one-off provisioning secrets and are never saved.
- `myn auth tailscale` validates only cloud API access. It does not require the local Tailscale daemon and does not require the local `tailscale` CLI.
- `myn auth tailscale` must validate policy read, policy validation, safe no-op policy update, device listing, and auth key creation capability before saving credentials.
- `myn configure` requires a running local Tailscale daemon only at Personal Server creation time. The daemon is accessed through Tailscale LocalAPI so macOS GUI installs work without a CLI binary.
- The active local tailnet must match the saved Tailscale Credentials tailnet before any new Personal Server is created.
- The Personal Server User defaults from the current Tailscale identity after Linux-safe normalization and remains editable.
- The Personal Server User has Bash as the login shell, belongs to sudo and docker groups, and uses password-backed sudo. The password is hashed for cloud-init and not saved.
- Tailnet Policy planning is its own deep module. It should accept current policy, current Tailscale identity, selected Personal Server User, and target tag, then report whether the policy is sufficient or which semantic changes are required.
- The required Tailnet Policy owns `tag:myn-personal-server` by the current Tailscale identity, grants that identity network access to `tag:myn-personal-server` on port `22`, and grants Tailscale SSH to that tag only as the selected Personal Server User with `checkPeriod: "always"`.
- Policy network access uses grants. Myn does not migrate unrelated legacy ACL entries and does not add policy tests.
- Policy edits are idempotent, preserve HuJSON comments and formatting where practical, include Myn comments where practical, validate the full proposed policy before mutation, and apply with ETag protection.
- When policy changes are needed, `myn configure` opens the Tailscale Access Controls page, shows a semantic summary, asks whether API editing is allowed, validates before final creation confirmation, and applies only after final confirmation.
- The Tailscale cloud API integration is a narrow client boundary responsible for policy fetch, validation, ETag-protected update, device listing, and one-off machine auth key creation.
- The LocalAPI integration is a narrow client boundary responsible for local daemon status, active tailnet, and current Tailscale identity.
- A fresh Tailscale Machine Auth Key is created for every Personal Server creation attempt after final confirmation and after policy application.
- The machine auth key is one-off, tagged `tag:myn-personal-server`, pre-approved, non-ephemeral, expires after ten minutes, and is created immediately before rendering cloud-init and creating the server.
- The machine auth key is passed only into bootstrap rendering, handled through a root-only file during bootstrap, removed after successful Tailscale join, and never printed or persisted.
- Personal Server Bootstrap is updated to install Tailscale first, run `tailscale up` with the auth key, selected Tailscale Host, and Tailscale SSH enabled, then disable system OpenSSH before writing the success marker.
- Personal Server Bootstrap no longer writes authorized SSH keys for root or the Personal Server User.
- Personal Server Bootstrap no longer installs Mosh, opens Mosh ports, reports Mosh versions, or prints Mosh commands.
- Existing development bootstrap behavior remains: security updates, unattended upgrades, Docker, Homebrew-owned-by-user tooling, tmux profile, Git identity, GitHub CLI, Go, nvm, Node LTS, Codex, Claude Code, and partial-failure reporting for coding agents.
- New Hetzner Personal Servers are created with public IPv4 disabled and public IPv6 enabled. IPv6 is saved as inventory only.
- The Personal Server Firewall remains named `myn-personal-server`, but the desired ruleset has no public inbound rules.
- An existing firewall with the Personal Server Firewall name is reconciled only when it is clearly Myn-managed. An unmanaged firewall with the same name fails provisioning before server creation.
- Hetzner labels include Myn ownership metadata and the Tailscale Host for inventory, but labels are not used to auto-adopt missing Personal Server Configuration.
- Personal Server Configuration for new servers stores server ID, Personal Server User, Tailscale Host, and public IPv6 inventory. It does not store public IPv4, Location, Server Type, SSH identity, desired Tailnet Policy, or machine auth key.
- `myn connect`, `myn connect-new`, and `myn sessions` require Personal Server User and Tailscale Host, call ordinary `ssh`, preserve host-key behavior, and do not pass an identity file.
- Personal Server connection planning continues to preserve existing Project Session and tmux handoff semantics.
- `myn configure` checks duplicate Hetzner server name and duplicate Tailscale Host before policy mutation, auth key creation, or cloud resource creation.
- The final plan shows Location, Server Type, Tailscale Host, IPv6-only public networking, no public inbound firewall, policy summary, install plan, and pricing when available.
- After Hetzner create actions finish, Myn saves server ID, Personal Server User, Tailscale Host, and IPv6 before any Tailscale reachability or bootstrap marker wait.
- Provisioning waits up to eight minutes for Tailscale device registration with hostname match, expected tag, authorization, online state, and ordinary SSH reachability.
- After SSH reachability, provisioning waits five minutes for the existing bootstrap marker over SSH as the Personal Server User.
- Tailscale join, SSH reachability, bootstrap timeout, and bootstrap failure are hard failures, but Myn does not delete the Hetzner server automatically.
- Existing complete Tailscale Personal Server Configuration is verified against both Hetzner server ID and Tailscale Host before `myn configure` skips creation.
- `myn run` remains unaffected by Personal Server access changes.

## Testing Decisions

- Tests should focus on external behavior and stable contracts: saved config shape, command output, request shapes, orchestration ordering, rendered cloud-init content, policy semantic changes, and error behavior. Tests should not assert incidental helper structure.
- Tailscale auth tests should cover successful save, invalid token, insufficient capability, ambiguous tailnet handling, environment-driven auth, browser-open fallback, and config file permissions.
- Tailscale cloud API tests should cover policy read, validation, no-op update validation, ETag-protected apply, device listing, auth key request shape, and API failure mapping.
- LocalAPI tests should cover daemon unavailable, daemon disconnected, unreadable LocalAPI, tailnet mismatch, current identity discovery, and identity-derived username defaults.
- Tailnet Policy planner tests should cover already-sufficient policy, missing tag owner, missing grant, missing Tailscale SSH rule, combined changes, unrelated legacy ACL preservation, validation failure, and ETag conflict.
- Personal Server connection tests should cover ordinary SSH command construction, no identity file, Tailscale Host selection, legacy migration failure, missing config validation, `connect`, `connect-new`, and `sessions`.
- Bootstrap renderer tests should parse rendered cloud-init and inspect rendered shell behavior for Tailscale-first ordering, no authorized keys, auth key non-leakage, Tailscale SSH enablement, OpenSSH disablement, Mosh removal, marker content, and valid Bash.
- Hetzner provisioning tests should cover IPv4 disabled, IPv6 enabled, saved IPv6 inventory, labels, no public inbound firewall rules, managed firewall reconciliation, unmanaged firewall failure, and duplicate server name failure.
- Configure orchestration tests should cover final confirmation decline, ordering of policy apply, auth key creation, cloud-init rendering, server creation, early config save, device polling, SSH reachability, marker polling, and non-deletion on failure.
- Existing-server verification tests should cover valid saved Tailscale config, missing Hetzner server, missing Tailscale device, incomplete Tailscale config, and legacy public-SSH migration messages.
- Documentation and live validation should include at least one human-run smoke test with real Hetzner and Tailscale credentials because it provisions a billable server and depends on live tailnet policy behavior.
- Prior test patterns already exist for auth command validation, config persistence, Personal Server provisioning fakes, SSH command construction, cloud-init rendering, and command-output assertions. New tests should extend those patterns instead of introducing a separate harness unless a deep module needs an isolated table-test suite.

## Out of Scope

- Migrating existing public-SSH Personal Servers automatically.
- Repairing or re-tagging missing Tailscale devices for existing servers.
- Supporting public SSH or Mosh as a recovery path.
- Requiring, generating, uploading, or injecting local SSH keys for Personal Server access.
- Using `tailscale ssh` as the connection command.
- Saving Tailscale Machine Auth Keys.
- Revoking the one-off machine auth key after successful join.
- Adding Tailnet Policy tests to the user's policy file.
- Migrating unrelated legacy ACL entries to grants.
- Running non-interactive Personal Server creation.
- Changing `myn run` behavior.
- Copying local dotfiles or live shell configuration beyond the existing explicit bootstrap setup.
- Automatically cleaning up billable Hetzner resources after provisioning failure.
- Building a full legacy-server migration workflow in this PRD.

## Further Notes

- This PRD is governed by the Tailscale-only Personal Server ADR, which supersedes the previous bootstrap-then-harden public SSH model for newly provisioned Personal Servers.
- The implementation is intentionally split into deep modules around Tailscale cloud API, LocalAPI, Tailnet Policy planning, bootstrap rendering, connection planning, Hetzner provisioning, and configure orchestration.
- The highest-risk areas are policy mutation, secret handling, and the transition from system OpenSSH to Tailscale SSH during first boot. These areas need targeted tests and clear user-facing error messages.
- The live validation slice should remain human-in-the-loop because it needs real Tailscale API access, a running local daemon, tailnet policy permissions, and a billable Hetzner server.
