# Myn

<p align="center">
  <img src="../../logos/myn-seagull-variation-5-icon-palette-04-m-blue-dark-electric-blue.svg" alt="Myn seagull logo" width="180">
</p>

Myn ("mine") provisions and operates your personal development environment.

## Hetzner Personal Server Setup

These commands install `myn`, save the required Hetzner Cloud and Tailscale
credentials, create a Tailscale-only Hetzner Personal Server through the
interactive provisioning flow, and then connect to it:

```sh
curl -fsSL https://raw.githubusercontent.com/hsubra89/myn/master/go/cli/install.sh | sh
myn version

myn auth hetzner
myn auth tailscale
myn configure

myn connect
```

`myn auth hetzner`, `myn auth tailscale`, and `myn configure` are interactive
when flags or environment variables are not supplied. Tailscale authentication
is required before creating a new Personal Server.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/hsubra89/myn/master/go/cli/install.sh | sh
```

This installs the latest stable release to `~/.local/bin/myn` and verifies the
release checksum before replacing any existing binary at that path. To install
a specific release:

```sh
curl -fsSL https://raw.githubusercontent.com/hsubra89/myn/master/go/cli/install.sh | MYN_VERSION=0.1.0 sh
```

## Development

Common development commands are defined in [`justfile`](justfile):

```sh
just run version
just fmt
just test
just build
just tidy
```

## Run

```sh
myn version
```

## Local Dev Launcher

```sh
../../scripts/mount-myn-cli
myn version
```

This creates `~/.local/bin/myn` as a launcher for this checkout. It runs
a temporary dev binary built from this checkout, so local source changes are
picked up automatically while cwd-sensitive commands still see the directory
where `myn` was invoked. Use `../../scripts/mount-myn-cli --unmount` to remove
it.

## Configure

```sh
myn configure
```

The command configures project roots for this machine and the remote Personal
Server. It stores normalized home-relative paths in the `myn` config:

```json
{
  "projects": {
    "localRoot": "projects",
    "remoteRoot": "projects"
  }
}
```

The local root must be an existing directory under the current user's home
directory. The prompt accepts `projects`, `~/projects`, or an absolute path
under the user's home directory, and stores all of them as `projects`.

The remote root must be relative to the remote user's home directory. The
prompt accepts `projects` or `~/projects` and stores `projects`; absolute
remote paths are rejected.

For new Personal Servers, `myn configure` expects saved Hetzner Credentials and
Tailscale Credentials. When both are already present it does not prompt for,
generate, upload, or attach a local SSH identity; access is through Tailscale
SSH. The legacy `--ssh-identity-file` option remains for local configuration
compatibility, but it is not part of new Tailscale-only Personal Server
provisioning.

Non-interactive options:

```sh
myn configure \
  --local-root projects \
  --remote-root projects
```

Non-interactive `configure` saves local settings but never creates a Personal
Server, so scripts do not create billable cloud resources.

## Project Roots

`myn` uses the local project root and remote project root to map the directory
you are in locally to the matching directory on the Personal Server.

The local root is an existing directory under your home directory that contains
your projects, such as `~/projects`. The remote root is a path relative to the
Personal Server user's home directory, also commonly `projects`.

For example, with both roots configured as `projects`:

```text
Local:  ~/projects/cool-vibe-coded-project/app
Remote: ~/projects/cool-vibe-coded-project/app
```

When you run `myn connect` from inside
`~/projects/cool-vibe-coded-project/app`, it connects to the Personal Server
and starts or attaches to the Project Session for `cool-vibe-coded-project`,
using `~/projects/cool-vibe-coded-project/app` as the preferred remote working
directory. If you run `myn connect` from `~/projects`, it targets the remote
root itself. Running outside the configured local root fails before SSH.

## Hetzner Authentication

```sh
myn auth hetzner
```

The command validates that a Hetzner Cloud API token is a Read & Write token and
saves it in the `myn` config. It first checks `/locations`, then probes
`DELETE /ssh_keys/0` against a non-existent key so no key is created. By
default the config path is
`${XDG_CONFIG_HOME}/myn/config.json` or the platform equivalent from Go's
`os.UserConfigDir`; set `MYN_CONFIG` to override it.

Non-interactive options:

```sh
myn auth hetzner --token "$HCLOUD_TOKEN"
myn auth hetzner --from-hcloud-context warptech
```

When no token is supplied, the command checks an existing `myn` token first, then
looks for hcloud contexts in `~/.config/hcloud/cli.toml` or `HCLOUD_CONFIG`.
Set `HCLOUD_ENDPOINT` to override the validation endpoint.

## Tailscale Authentication

```sh
myn auth tailscale
```

The command opens the Tailscale Keys page when it can, prompts for a Tailscale
API access token, infers the tailnet when the token can see exactly one
tailnet, and saves only:

```json
{
  "auth": {
    "tailscale": {
      "token": "tskey-api-...",
      "tailnet": "example.com"
    }
  }
}
```

The token is validated before it is saved. It must be able to read Tailnet
Policy, validate Tailnet Policy, perform a safe no-op policy update, list
devices, and create Machine Auth Keys. The local Tailscale daemon is not checked
by `myn auth tailscale`; `myn configure` checks the daemon later through
Tailscale LocalAPI.

Non-interactive options:

```sh
myn auth tailscale --token "$TAILSCALE_API_TOKEN" --tailnet "$TAILSCALE_TAILNET"
# or
TAILSCALE_API_TOKEN=... TAILSCALE_TAILNET=... myn auth tailscale
```

Saved Tailscale Credentials are long-lived Myn configuration. The one-off
Tailscale Machine Auth Key used by a new server during bootstrap is created
only after final creation confirmation, expires after ten minutes, is consumed
through a root-only cloud-init file, and is never saved or printed.

## Personal Server Provisioning

Run `myn auth hetzner` and `myn auth tailscale` before provisioning a Personal
Server. `myn configure` uses saved credentials; it does not collect Hetzner or
Tailscale tokens inside the configure flow.

Interactive `myn configure` configures project roots, verifies the local
Tailscale daemon through LocalAPI, confirms the active local tailnet matches
the saved Tailscale Credentials, and then offers to create a Hetzner Cloud
Personal Server. The local `tailscale` CLI is not required.

The Personal Server User defaults from the current Tailscale identity after
Linux username normalization and remains editable. The selected user has Bash
as the login shell, belongs to the `sudo` and `docker` groups, and uses
password-backed sudo. The password is hashed for cloud-init and is not saved.

When a Personal Server already exists in config, `configure` verifies the saved
Hetzner server ID and the saved Tailscale Host. If both still exist, it reports
the saved server ID, Tailscale Host, public IPv6 inventory, and current Hetzner
state, then skips creation. If the Tailscale device is missing, configure fails
with a manual repair or recreate message.

The saved Personal Server Configuration is a top-level config section that
stores the created server identity, Personal Server User, Tailscale Host, and
public IPv6 inventory:

```json
{
  "personalServer": {
    "serverID": 123456,
    "user": "harish",
    "tailscaleHost": "harish-personal-server",
    "ipv6": "2001:db8::1"
  }
}
```

The public IPv6 address is inventory only. `myn connect`, `myn connect-new`,
and `myn sessions` require the Personal Server User and Tailscale Host and do
not use public IPv4, public IPv6, or a configured SSH identity as fallback
connection paths.

Location, Server Type, server name, password, install choices, and pricing are
transient provisioning inputs. They are not saved as desired state.

Before billable resources are created, `configure` checks for duplicate Hetzner
server names and duplicate Tailscale Hosts, computes the required Tailnet
Policy, validates the current or proposed policy, and shows the full creation
plan. If policy changes are needed, it opens the Tailscale Access Controls page
when possible, summarizes the missing pieces, and asks before API editing.
Policy writes happen only after the final server creation confirmation and use
ETag protection.

The required Tailnet Policy is narrow:

- the current Tailscale identity can own `tag:myn-personal-server`;
- the current Tailscale identity can reach `tag:myn-personal-server` on
  `tcp:22` through a grant;
- the current Tailscale identity can Tailscale SSH to
  `tag:myn-personal-server` only as the selected Personal Server User, with
  `checkPeriod: "always"`.

During creation, `configure` prompts for a Hetzner Location, then lists
eligible non-deprecated x86_64 Server Types available in that Location. The
Location selector defaults to `ash` when Hetzner offers it, otherwise the first
Location sorted by code. Server Type options show dedicated/shared compute,
vCPU, RAM, disk, storage type, and API name. Prices are not shown in the Server
Type selector; the final confirmation shows one maximum monthly gross EUR price
when pricing is available, or clearly says the price is unavailable.

Before creating cloud resources, `configure` shows an install plan grouped as:

- System services: security updates, unattended security upgrades, Docker
  Engine, Docker Compose, Tailscale, Tailscale SSH, system OpenSSH disablement,
  the Personal Server User, and the remote project root.
- Homebrew tools: `tmux`, `jq`, `git`, `gh`, `rustup`, `go`, `nvm`, latest LTS
  Node.js, and npm.
- Coding agents: Codex and Claude Code.

Git identity values are copied from available local Git config values, with
global config preferred over repo-local config. Missing `user.name` or
`user.email` values are reported and skipped. GitHub CLI is installed, but
GitHub authentication and credentials are not configured.

After final confirmation, `configure` applies any required Tailnet Policy
changes, creates a fresh one-off Tailscale Machine Auth Key, renders
Tailscale-first cloud-init, creates an IPv6-only Hetzner server, and attaches a
Myn-managed firewall with no public inbound rules. Hetzner public IPv4 is
disabled and public IPv6 is enabled for outbound bootstrap connectivity.

Cloud-init installs and joins Tailscale before the development bootstrap,
enables Tailscale SSH, removes Machine Auth Key material, disables system
OpenSSH, and then installs the development environment. It does not inject
local SSH public keys, install Mosh, open Mosh ports, or print Mosh commands.

After Hetzner create actions finish, `configure` saves the server ID, Personal
Server User, Tailscale Host, and public IPv6 inventory before waiting for
access. It then waits for Tailscale device registration, expected tag,
authorization, online state, ordinary SSH reachability through the Tailscale
Host, and the cloud-init Personal Server Bootstrap marker. Reachability and
bootstrap failures are hard failures, but Myn does not delete the billable
server automatically.

When provisioning finishes successfully, `configure` prints the Tailscale Host,
public IPv6 inventory, installed tool versions, any partial bootstrap failures,
and `myn connect`.

Existing public-SSH Personal Server configurations are a hard migration break.
Configs that have public addresses but no Tailscale Host fail with:

```text
legacy public-SSH Personal Server Configuration is no longer supported; recreate the Personal Server with Tailscale-only provisioning
```

Personal Server provisioning does not create SSH config aliases, clone or sync
projects, copy dotfiles, copy GitHub credentials, authenticate `gh`, install a
Rust toolchain, or automatically migrate legacy public-SSH servers.

## Connect

```sh
myn connect
myn c
myn c 2
myn connect-new
myn cn
myn sessions
myn s
myn l
```

`myn connect` starts a Personal Server Connection using the saved Personal
Server Configuration. The current working directory is the input. The command
requires a configured local project root, remote project root, Personal Server
User, and Tailscale Host. The local project root must exist locally, and stdin
and stdout must be terminal-backed.

The current working directory is mapped lexically under the configured local
project root to the matching path under the configured remote project root. If
the command runs from the local project root itself, it targets the remote
project root. If it runs from a subdirectory, the target Project is the first
path segment under the configured local project root; Git repository boundaries
do not affect this. Running outside the configured local project root fails
before SSH.

The command uses ordinary `ssh` to the saved Tailscale Host. Tailscale SSH and
Tailnet Policy enforce access; Myn does not run `tailscale ssh`, does not pass
`-i`, and does not fall back to public IPv4 or public IPv6. SSH requests one TTY
allocation for interactive connects, and host key checking uses
`StrictHostKeyChecking=accept-new`.

On the Personal Server, `myn connect` runs a Bash login-shell tmux handoff. Each
Project can have multiple numbered Project Sessions. Session `1` uses the stable
project tmux session name, and sessions `2`, `3`, and later append `:2`, `:3`,
and so on to that name. Tmux normalizes the colon to `_` in its stored session
name, so Myn treats that tmux-safe spelling as the same Project Session. Bare
`myn connect` attaches to the lowest-numbered existing Project Session for the
current Project. When none exist, it creates Project Session `1`. Numbered
`myn connect N` attaches only to existing Project Session `N` and fails rather
than creating a missing session.

`myn connect-new` creates a new Project Session for the current Project and
attaches to it. The new session number is one greater than the highest existing
Project Session number. When none exist, it creates Project Session `1`.

New Project Sessions start in the exact mapped remote directory when that
directory exists, then fall back to the remote Project root, then the Personal
Server User home directory. Only existing directories are used, and the command
does not create missing remote project directories.

`myn sessions` lists Project Sessions for the current Project. It runs as a
non-interactive SSH command without requesting a TTY, prints one row per session
sorted by session number, and adds `attached` when tmux reports attached
clients:

```text
1  attached
2
3  attached
```

When the current Project has no Project Sessions, `myn sessions` prints no rows
and exits successfully.

`myn connect`, `myn connect-new`, and `myn sessions` do not require Hetzner or
Tailscale API access at connection time, do not verify the saved server before
connecting, and do not create an Idle Lease or Stdio Lease. Successful handoff
prints no Myn-specific output; local validation failures are reported before
SSH, and SSH or tmux exit statuses are preserved.

## Test

```sh
just test
# or
go test ./...
```

### Live Tailscale Validation

The Tailscale-only Personal Server validation is human-run because it creates a
billable Hetzner Cloud server, mutates Tailnet Policy when required, depends on
a running local Tailscale daemon, and uses real Tailscale SSH access.

Use a repository-root `.env.local` file for local secrets. That file is ignored
by git.

```sh
HETZNER_API_KEY=...
TAILSCALE_API_TOKEN=...
TAILSCALE_TAILNET=...
MYN_LIVE_HETZNER_LOCATION=ash
MYN_LIVE_HETZNER_SERVER_TYPE=cax11
```

`HETZNER_API_KEY` must be a Hetzner Read & Write token. `TAILSCALE_API_TOKEN`
must be able to read, validate, and update Tailnet Policy, list devices, and
create Machine Auth Keys for `TAILSCALE_TAILNET`. The local Tailscale daemon
must be running and connected to the same tailnet.

The gated live test exercises the same Tailscale-only provisioning path and
cleans up the billable Hetzner server it creates:

```sh
MYN_LIVE_TAILSCALE=1 go test ./internal/cli -run TestLivePersonalServerProvisioning -count=1 -v
```

For a smoke test, use an isolated config and run the product flow:

```sh
export MYN_CONFIG="$(mktemp -d)/config.json"
set -a
. ./.env.local
set +a

myn auth hetzner --token "$HETZNER_API_KEY"
myn auth tailscale --token "$TAILSCALE_API_TOKEN" --tailnet "$TAILSCALE_TAILNET"
myn configure
```

During validation, confirm that the created server is IPv6-only publicly, the
`myn-personal-server` firewall has no public inbound rules, the Tailscale device
registers with `tag:myn-personal-server`, ordinary
`ssh -l <user> <tailscale-host>` reaches the server, system OpenSSH is disabled
on the server, the bootstrap marker exists at
`/var/lib/myn/personal-server-bootstrap.json`, and `myn connect` reaches the
Project Session over the Tailscale Host. Delete the billable Hetzner server
after the smoke test.

## Format

```sh
just fmt
# or
go fmt ./...
```

## Build

```sh
just build
# or
mkdir -p ./bin
go build -o ./bin/myn ./cmd/myn
```

## Release Metadata

```sh
just release 0.1.0
# or
mkdir -p ./bin
go build \
  -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o ./bin/myn ./cmd/myn
```
