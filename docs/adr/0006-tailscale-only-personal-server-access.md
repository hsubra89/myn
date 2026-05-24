# Tailscale-Only Personal Server Access

This ADR supersedes the access model in
`0005-bootstrap-then-harden-personal-server-ssh.md` and supersedes default
Mosh Access for newly provisioned Personal Servers.

`myn` provisions new Personal Servers as Tailscale-only development machines.
New servers use Hetzner IPv6 public networking for outbound bootstrap traffic,
join the user's tailnet during cloud-init, enable Tailscale SSH, and expose no
public inbound services through the Hetzner firewall. `myn connect`,
`myn connect-new`, and `myn sessions` use ordinary `ssh` against the saved
Tailscale hostname; Tailscale SSH and tailnet policy provide the access
boundary.

`myn auth tailscale` stores a Tailscale API access token and tailnet identifier
in `auth.tailscale`. It validates the cloud API token up front, including the
ability to read, validate, and no-op update tailnet policy, list devices, and
create auth keys. Local Tailscale daemon checks are not part of `auth`; they run
during `myn configure`.

During interactive `myn configure`, `myn` verifies that the local Tailscale
daemon is running and connected to the saved tailnet through Tailscale LocalAPI.
The local `tailscale` CLI is not required; macOS GUI installs with a running
daemon must be supported. The implementation should use Tailscale's LocalAPI
client for local daemon status and the official Tailscale cloud API Go client
for policy, device, and auth-key operations.
The Personal Server User defaults from the current Tailscale identity,
normalized as a Linux username, and remains editable. Before any cloud resource
is created, `myn` checks for duplicate Hetzner server names and duplicate
Tailscale hostnames, computes the minimum required tailnet policy, validates the
current or proposed policy, and shows a semantic summary of any changes.

Required tailnet policy is intentionally narrow:

- `tag:myn-personal-server` is owned by the current Tailscale identity.
- a Tailscale grant allows that identity to reach
  `tag:myn-personal-server` on port `22`.
- a Tailscale SSH rule allows that identity to SSH to
  `tag:myn-personal-server` only as the selected Personal Server User, with
  `checkPeriod: "always"`.

If policy changes are needed, `myn configure` opens the Tailscale Access
Controls page, asks before editing through the API, validates the proposed full
policy, and applies it with ETag protection. Policy edits are idempotent and
should preserve HuJSON comments/formatting where practical. `myn` uses grants
for the network access rule and does not migrate unrelated legacy ACL entries.

After final Personal Server creation confirmation, `myn` applies any required
policy changes, creates a fresh one-off, tagged, pre-approved, non-ephemeral
Tailscale machine auth key for `tag:myn-personal-server` with a ten minute
expiry, renders cloud-init, and creates the Hetzner server. The machine auth
key is passed only through cloud-init, handled in a root-only file during
bootstrap, and removed after successful `tailscale up`.

Cloud-init installs Tailscale first, runs
`tailscale up --auth-key ... --hostname ... --ssh`, disables system OpenSSH
after Tailscale SSH is enabled, then continues the existing development
bootstrap. It does not inject local SSH public keys for root or the Personal
Server User. Mosh is not installed, no Mosh UDP firewall rule is created, and
Mosh commands are not printed.

The reusable Hetzner firewall remains named `myn-personal-server`, but its
desired ruleset has no public inbound rules. If an existing firewall with that
name has Myn labels, `myn` reconciles it to no inbound rules. If the existing
firewall is not clearly Myn-managed, provisioning fails rather than clobbering
user-managed rules.

The saved Personal Server Configuration for new servers stores `serverID`,
Personal Server User, `tailscaleHost`, and public `ipv6` as inventory. It does
not store public IPv4, SSH identity, Location, Server Type, machine auth key, or
tailnet policy desired state. New Hetzner servers are created with IPv4
disabled and IPv6 enabled. Hetzner labels include Myn ownership metadata and the
Tailscale hostname for inventory only.

After Hetzner create actions finish, `myn` saves the billable server identity
before waiting for access. It then waits up to eight minutes for Tailscale
device registration, the expected tag, authorization, online status, and
ordinary SSH reachability at the Tailscale hostname. After SSH reachability,
`myn` waits five minutes for the existing bootstrap marker over SSH. Bootstrap
or reachability failure is a hard failure, but `myn` does not automatically
delete the Hetzner server so the user can inspect it.

Existing public-SSH Personal Server configurations are a hard migration break.
`myn connect` does not fall back to IPv4/IPv6 or configured SSH identities. Old
configs should fail with a clear migration message rather than being cleared or
repaired automatically.
