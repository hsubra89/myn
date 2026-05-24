# Myn CLI

**Myn** provisions and operates the user's personal development environment across their local machine and a cloud server.

## Language

**Myn**:
The CLI that provisions and operates the user's personal development environment across their local machine and a cloud server.
_Avoid_: generic personal toolbox

**myn**:
The command name for **Myn**, pronounced "mine".
_Avoid_: generic command aliases

**Myn Release**:
A tagged GitHub Release that distributes downloadable **Myn** CLI binaries.
_Avoid_: GitHub Package, package registry release

**Myn Installer**:
A curl-able shell installer that installs the `myn` command from a **Myn Release**.
_Avoid_: Homebrew formula, Personal Server Bootstrap

**Personal Server**:
A Hetzner Cloud server that **Myn** provisions for the user's cloud-hosted development environment.
_Avoid_: remote machine, default server, Myn Server

**Hetzner Credentials**:
A saved Read & Write Hetzner Cloud API token used by **Myn** for cloud provisioning.
_Avoid_: hcloud context, cloud login

**Tailscale Credentials**:
A saved Tailscale API access token and tailnet identifier used by **Myn** for tailnet policy checks, policy edits, device lookup, and one-off machine auth key creation.
_Avoid_: Tailscale login, local Tailscale session, machine auth key

**Location**:
A Hetzner Cloud location where a **Personal Server** can be created.
_Avoid_: region, datacenter

**Personal Server Configuration**:
The saved identity of a created **Personal Server**: server ID, **Personal Server User**, Tailscale hostname, and public IPv6 inventory address.
_Avoid_: Hetzner config, remote config

**Personal Server Firewall**:
A reusable Hetzner Cloud firewall named `myn-personal-server` that is created by **Myn** for **Personal Server** instances and reconciled to no public inbound rules when it is clearly Myn-managed.
_Avoid_: SSH-only firewall, generated firewall

**Tailscale Host**:
The plain Tailscale hostname assigned to a **Personal Server** and used by **Personal Server Connections**.
_Avoid_: public host, SSH alias

**Tailnet Policy**:
The Tailscale access policy that grants the current Tailscale identity network and Tailscale SSH access to `tag:myn-personal-server`.
_Avoid_: SSH config, local firewall rule

**Tailscale Machine Auth Key**:
A one-off, tagged, pre-approved, non-ephemeral Tailscale auth key that **Myn** creates during **Personal Server** provisioning and passes only to first-boot cloud-init.
_Avoid_: Tailscale Credentials, API token

**Tailscale SSH Access**:
Tailscale-provided SSH access to a **Personal Server** over the tailnet, authorized by **Tailnet Policy**.
_Avoid_: public SSH, OpenSSH hardening profile

**Mosh Access**:
Legacy UDP-based interactive shell access that is not installed or opened for new **Personal Servers**.
_Avoid_: default Personal Server access

**Personal Server Connection**:
An SSH-backed interactive connection over **Tailscale SSH Access** that opens a **Project Session** on the configured **Personal Server**.
_Avoid_: raw SSH session, Mosh session, remote shell

**myn connect**:
The canonical command for starting a **Personal Server Connection**, with `myn c` as its short alias.
_Avoid_: myn ssh, myn tmux, myn shell

**myn connect-new**:
The canonical command for creating a new **Project Session** for the current **Project**, with `myn cn` as its short alias.
_Avoid_: myn connect new, myn new

**myn sessions**:
The canonical command for listing **Project Sessions** for the current **Project**, with `myn s` and `myn l` as short aliases.
_Avoid_: myn list, tmux ls

**Project**:
A connection target rooted at the configured project root itself or at a top-level directory directly under it.
_Avoid_: Git repository, workspace, checkout

**Project Session**:
A tmux-backed interactive workspace scoped to one **Project** on the **Personal Server**.
_Avoid_: raw tmux session, shell, terminal

**Personal Server User**:
A Linux user account created on a **Personal Server** from a normalized form of the current Tailscale identity, editable during provisioning.
_Avoid_: remote user, cloud user

**Server Type**:
A non-deprecated Hetzner Cloud x86_64 server type available in the selected **Location** for a **Personal Server**.
_Avoid_: size, plan, instance type

**Personal Server Bootstrap**:
The first-boot cloud-init process that creates the **Personal Server User** and installs the expected development tools.
_Avoid_: setup script, install script

**Personal Server tmux Profile**:
The standard tmux behavior that **Myn** installs for the **Personal Server User**.
_Avoid_: copied tmux dotfile, local tmux config sync

**Idle Lease**:
A renewable runtime claim that keeps a **Personal Server** awake while there is recent evidence of user-triggered work.
_Avoid_: lock, keepalive, inhibitor

**Stdio Lease**:
An **Idle Lease** for an interactive command whose activity is evidenced by recent terminal input or output.
_Avoid_: prompt lease, terminal lock

## Relationships

- A **Personal Server** joins the user's tailnet during **Personal Server Bootstrap** and uses **Tailscale SSH Access** as the only supported interactive access path.
- A **Personal Server** does not trust a configured local SSH identity for bootstrap or ongoing login.
- A **Personal Server** does not expose public inbound SSH or **Mosh Access**.
- A **Personal Server** disables system OpenSSH after `tailscale up --ssh` succeeds.
- A **Personal Server** is tagged `tag:myn-personal-server` in Tailscale.
- A **Tailnet Policy** grants the current Tailscale identity network access to `tag:myn-personal-server` on port `22`.
- A **Tailnet Policy** grants Tailscale SSH access only as the configured **Personal Server User** with `checkPeriod: "always"`.
- A **Personal Server Connection** uses ordinary `ssh` to the saved **Tailscale Host** and relies on Tailscale SSH interception and policy.
- A **Project** can have multiple **Project Sessions**.
- A **Personal Server Connection** opens a **Project Session** for the target **Project**.
- A **Project** has a stable default **Project Session** plus optional numbered sibling **Project Sessions**.
- A default **Project Session** name is derived from the remote **Project** root path using a stable `myn-` prefixed tmux-safe name.
- A default **Project Session** name lowercases the remote **Project** root path, keeps ASCII letters and digits, converts every other character run to one hyphen, trims edge hyphens, prefixes `myn-`, and uses `myn-project` if the normalized project path is empty.
- A numbered sibling **Project Session** appends `:2`, `:3`, and so on to the default **Project Session** name.
- Because tmux normalizes colons in session names to underscores, **Myn** treats the tmux-stored `_2`, `_3`, and so on spelling as the same numbered **Project Session** as `:2`, `:3`, and so on.
- **Project Session** `1` maps to the default **Project Session** name without a `-1` suffix.
- Bare `myn connect` opens the lowest-numbered existing **Project Session**, creating **Project Session** `1` only when none exist for the target **Project**.
- `myn connect` accepts an optional **Project Session** number and attaches only to that existing **Project Session**.
- `myn connect` with a **Project Session** number fails rather than creating a missing **Project Session**.
- `myn connect-new` creates a new numbered **Project Session** for the target **Project** and opens it.
- `myn connect-new` uses one greater than the highest existing **Project Session** number.
- `myn connect-new` creates **Project Session** `1` when the target **Project** has no existing **Project Sessions**.
- `myn sessions` lists **Project Sessions** for the target **Project**.
- `myn sessions` sorts **Project Sessions** by session number.
- `myn sessions` prints one plain row per **Project Session** with the session number and an `attached` marker when tmux reports attached clients.
- `myn sessions` prints no rows and exits successfully when the target **Project** has no **Project Sessions**.
- `myn sessions` runs as a non-interactive command without requiring terminal-backed stdin or stdout.
- `myn sessions` does not attach to or create **Project Sessions**.
- A **Personal Server Connection** fails rather than falling back to plain SSH when tmux is unavailable.
- A **Personal Server Connection** relies on the **Personal Server User** login shell PATH to find tmux.
- A **Personal Server Connection** runs its remote tmux handoff through Bash login-shell command evaluation.
- A **Personal Server Connection** trusts the saved **Personal Server Configuration** and does not require **Hetzner Credentials** or Hetzner API verification before connecting.
- A **Personal Server Connection** uses the saved **Tailscale Host** directly and does not fall back to public IPv4 or IPv6.
- A **Personal Server Connection** passes the **Personal Server User** to SSH separately with `-l`.
- A **Personal Server Connection** does not create an **Idle Lease** in the initial implementation.
- A **Personal Server Connection** requires terminal-backed stdin and stdout.
- A **Personal Server Connection** requests one SSH TTY allocation.
- A **Personal Server Connection** does not pass a configured SSH identity to SSH.
- A **Personal Server Connection** uses SSH `StrictHostKeyChecking=accept-new`.
- A **Personal Server Connection** preserves the SSH/tmux process exit status.
- A **Personal Server Connection** validates saved configuration and local project root existence before attempting SSH.
- A **Personal Server Connection** stays quiet on successful handoff and prints clear messages only for local validation failures.
- A **Personal Server Connection** derives the target **Project** from the first path segment under the configured local project root, not from the Git repository root.
- A **Personal Server Connection** maps local paths lexically under the configured local project root and does not resolve symlink targets for project containment.
- `myn connect` accepts no path arguments in the initial implementation and maps from the current working directory.
- When `myn connect` is run from the configured local project root itself, the configured local project root is the target **Project**.
- A **Personal Server Connection** starts in the exact mapped remote directory when it exists, otherwise the remote **Project** root when it exists, otherwise the **Personal Server User** home directory.
- A **Personal Server Connection** treats only existing remote directories as valid starting directories.
- A **Personal Server Connection** does not create missing remote project directories.
- Directory fallback affects only newly created **Personal Server Connection** tmux sessions; existing project sessions are attached as-is.
- `myn connect` fails with clear messaging when run outside the configured local project root.
- `myn configure` does not modify the user's SSH config for **Personal Server** aliases.
- A **Myn Release** publishes CLI binaries as GitHub Release assets rather than GitHub Packages.
- A **Myn Release** is created from an explicit pushed version tag matching `v*`.
- A **Myn Release** publishes binaries for Linux amd64, Linux arm64, macOS amd64, and macOS arm64.
- Windows is not a **Myn Release** target until Windows workflows are supported.
- A **Myn Release** packages each platform binary in a `.tar.gz` archive.
- A **Myn Release** archive contains only the `myn` executable at the archive root.
- A **Myn Release** includes a `checksums.txt` asset with SHA-256 sums for all release archives.
- A **Myn Release** is published automatically after release tests pass for a pushed release tag.
- A **Myn Release** uses GitHub-generated release notes.
- A **Myn Release** tag keeps its leading `v`, but the embedded CLI version and release archive names drop the leading `v`.
- A **Myn Release** with a semver prerelease suffix is marked as a GitHub prerelease.
- A **Myn Release** validates the pushed tag as semver before publishing.
- A **Myn Release** publishes only when the tagged commit is reachable from the default branch.
- A **Myn Release** is built and published by a dedicated release workflow separate from ordinary Go CLI CI.
- A **Myn Release** workflow uses plain GitHub Actions shell steps rather than GoReleaser.
- A **Myn Release** workflow uses the built-in `GITHUB_TOKEN` with scoped release permissions.
- A **Myn Release** workflow fails rather than overwriting an existing release.
- A **Myn Release** is published immediately after release validation, tests, packaging, and checksum generation pass.
- **Myn** documents how to install a **Myn Release** binary from the published release assets.
- A **Myn Installer** installs the local `myn` command from GitHub Release assets rather than from a Homebrew tap or formula.
- A **Myn Installer** installs `myn` to `$HOME/.local/bin/myn` by default.
- A **Myn Installer** does not require `sudo` for its default install path.
- A **Myn Installer** reports when `$HOME/.local/bin` is not on `PATH` rather than editing shell profiles.
- A **Myn Installer** installs the latest stable **Myn Release** by default.
- A **Myn Installer** can install an explicitly pinned release version.
- A **Myn Installer** does not install prereleases unless a prerelease version is explicitly pinned.
- A **Myn Installer** does not support changing the install directory.
- A **Myn Installer** runs non-interactively.
- A **Myn Installer** requires only common shell tools and a release checksum tool, not `jq`, `gh`, Git, Homebrew, or Go.
- A **Myn Installer** fails rather than installing without verifying the matching release checksum.
- A **Myn Installer** supports only the operating systems and CPU architectures published by **Myn Release** assets.
- The canonical **Myn Installer** script lives at `go/cli/install.sh`.
- The documented **Myn Installer** command fetches the installer script from `master`.
- A **Myn Installer** installs released binaries rather than building from `master`.
- A **Myn Installer** overwrites `$HOME/.local/bin/myn` after the downloaded binary is verified.
- A **Myn Installer** resolves the latest stable release from public GitHub release URLs without requiring GitHub authentication.
- A **Myn Installer** prints concise progress for version selection, platform detection, download, checksum verification, install path, and PATH warnings.
- A **Myn Installer** runs the installed `myn version` after installation.
- A **Myn Installer** has automated coverage for version resolution, pinned installs, platform asset naming, checksum failure, unsupported platforms, overwrite behavior, and PATH warnings.
- A **Myn Installer** is tested by the existing Go CLI CI.
- A **Myn Installer** is tested from Go tests rather than a separate shell test framework.
- All user-visible namespaces use `myn`, including config paths, environment variables, runtime lease directories, cloud resource names, Hetzner labels, bootstrap files, shell profile files, and local development launchers.
- Documentation uses **Myn** in prose and `myn` for the command name; pronunciation belongs in introductory documentation, not command help.
- Planning and decision documents keep their technical rationale but use the **Myn** namespace consistently.
- **Myn** uses the command structure `auth hetzner`, `auth tailscale`, `configure`, `connect` (`c`), `connect-new` (`cn`), `sessions` (`s`, `l`), `idle status`, `run`, and `version`.
- `myn auth tailscale` opens the Tailscale Keys page, validates a Tailscale API access token through the cloud API, discovers or accepts a tailnet identifier, and saves **Tailscale Credentials**.
- `myn auth tailscale` validates policy read, policy validate, no-op policy update, device list, and auth key creation capabilities before saving **Tailscale Credentials**.
- `myn auth tailscale` accepts `TAILSCALE_API_TOKEN` and `TAILSCALE_TAILNET` for automation.
- `myn auth tailscale` does not require the local Tailscale daemon to be running.
- **Personal Server** prompts run only after local roots, **Hetzner Credentials**, **Tailscale Credentials**, and local Tailscale daemon connectivity are available.
- **Personal Server** provisioning requires a running local Tailscale daemon but does not require the local `tailscale` CLI on `PATH`.
- The active local tailnet from Tailscale LocalAPI must match the saved **Tailscale Credentials** tailnet before **Personal Server** creation.
- **Personal Server** creation is skipped unless **Hetzner Credentials** and **Tailscale Credentials** are configured.
- **Personal Server** provisioning does not clone or sync projects from the local root.
- **Personal Server Bootstrap** creates the configured remote project root exactly, including spaces, owned by the **Personal Server User**.
- A **Personal Server** can only be provisioned after **Hetzner Credentials** already exist.
- **Personal Server** provisioning respects the `HCLOUD_ENDPOINT` override.
- A **Personal Server** is created in exactly one **Location**.
- A **Location** selector shows the Hetzner code plus human geography and uses the code for API calls.
- The **Location** selector defaults to `ash` when Hetzner offers it, otherwise the first available **Location** sorted by code.
- A **Personal Server** is created with exactly one non-deprecated x86_64 **Server Type** available in its **Location**, based on explicit Hetzner availability metadata.
- If the selected **Location** has no eligible **Server Type**, interactive `configure` returns to **Location** selection.
- Hetzner `Recommended` metadata is ignored for **Location** and **Server Type** defaulting.
- A **Personal Server** defaults its API-safe name to `<Personal Server User>-personal-server`.
- A **Personal Server** default server name does not include `myn`; supporting cloud resources use `myn` because they are product-managed automation artifacts.
- A **Personal Server** name is validated locally as a DNS-label-style value before creation.
- A **Personal Server** is not created when a Hetzner server with the chosen name already exists.
- A **Personal Server** is not created when a Tailscale device already uses the chosen **Tailscale Host**.
- A **Personal Server** uses Hetzner's latest non-deprecated Ubuntu system image, selected by highest Ubuntu version.
- A **Personal Server** is created with public IPv4 disabled and public IPv6 enabled through server creation public network options.
- Hetzner resources created for a **Personal Server** are labeled `managed_by=myn` and `role=personal_server`.
- Hetzner server labels include the **Tailscale Host** for inventory only.
- Hetzner labels are for visibility only and are not used to auto-adopt missing **Personal Server Configuration**.
- **Myn** waits for Hetzner create actions to finish before Tailscale device registration, Tailscale SSH reachability, and bootstrap marker polling.
- Hetzner API calls, Tailscale API calls, Tailscale SSH polling, and bootstrap polling respect command cancellation.
- A **Personal Server Configuration** belongs in its own top-level config section.
- A **Personal Server Configuration** stores `serverID`, **Personal Server User**, `tailscaleHost`, and the assigned `ipv6` address, not ongoing desired state for mutable server details.
- A **Personal Server Configuration** is incomplete for connection without **Personal Server User** and `tailscaleHost`.
- A **Personal Server Configuration** is incomplete as a provisioned server record without `serverID`, **Personal Server User**, `tailscaleHost`, and the assigned `ipv6` address.
- **Location**, **Server Type**, and proposed server name selections are transient until a **Personal Server** is created.
- When **Personal Server Configuration** is complete for connection, `myn configure` skips creation by default and reports the saved server ID, **Tailscale Host**, and public IPv6 inventory address.
- A complete saved **Personal Server Configuration** server ID is verified against Hetzner and its **Tailscale Host** is verified against Tailscale before creation is skipped.
- If the Hetzner server exists but the Tailscale device is missing, `myn configure` fails without attempting automated repair.
- Legacy public-SSH **Personal Server Configurations** fail with a migration message instead of being cleared automatically.
- If a saved **Personal Server** server ID is missing in Hetzner, interactive `configure` asks before clearing it and non-interactive `configure` fails.
- **Personal Server** creation is interactive-only; non-interactive `configure` does not create a server.
- Missing **Hetzner Credentials** do not block saving local configuration; they only skip **Personal Server** creation.
- Missing **Tailscale Credentials** do not block saving local configuration; they only skip **Personal Server** creation.
- `myn configure` computes the required **Tailnet Policy** before final confirmation and validates the current or proposed policy without mutating it.
- If **Tailnet Policy** changes are needed, `myn configure` opens the Tailscale Access Controls page, shows a semantic summary, and asks before editing through the Tailscale API.
- `myn configure` applies **Tailnet Policy** changes only after final **Personal Server** creation confirmation, and before creating the **Tailscale Machine Auth Key** or Hetzner server.
- **Tailnet Policy** edits use grants for network access, preserve unrelated legacy ACL entries, are idempotent, and use ETag protection.
- **Tailnet Policy** edits do not add policy tests.
- `myn configure` creates a fresh one-off **Tailscale Machine Auth Key** with a ten minute expiry immediately before the Hetzner server create request.
- Interactive `configure` asks for explicit final confirmation before creating a **Personal Server**.
- A created **Personal Server** is saved in **Personal Server Configuration** even if the **Personal Server Bootstrap** fails or times out.
- `myn configure` reports public IPv6 only as inventory and does not print public SSH or **Mosh Access** commands.
- After successful **Personal Server Bootstrap**, `myn configure` reports the saved **Tailscale Host** and `myn connect`.
- If **Personal Server Bootstrap** fails or times out, `myn configure` does not report root SSH commands as a recovery path.
- `myn configure` saves local roots before attempting **Personal Server** creation, then saves **Personal Server Configuration** after server creation succeeds.
- After **Personal Server Bootstrap** completes, `myn configure` reports installed tool versions from the completion marker.
- **Personal Server** provisioning is implemented separately from local root configuration.
- A **Personal Server Firewall** is created by **Myn** with no public inbound rules.
- A **Personal Server Firewall** can be reused by multiple **Personal Server** instances over time.
- An existing **Personal Server Firewall** with Myn labels is reconciled to no public inbound rules.
- An existing `myn-personal-server` firewall without Myn labels is not modified; provisioning fails with a clear message.
- **Mosh Access** is not installed, opened, or printed for new **Personal Servers**.
- Supporting resources created for a **Personal Server** are not automatically cleaned up if server creation fails.
- A **Personal Server User** defaults from the current Tailscale identity as a lowercase letters, digits, and hyphens value.
- If the current Tailscale identity cannot be normalized into a valid **Personal Server User**, `myn configure` prompts for one.
- The **Personal Server User** prompt remains editable even when the Tailscale-derived default is valid.
- A **Personal Server User** starts with Bash as the login shell.
- A **Personal Server User** belongs to the `sudo` and `docker` groups.
- A **Personal Server User** has sudo access, but sudo requires a password.
- Membership in the `docker` group is accepted as root-equivalent development-machine access.
- A **Personal Server User** password is collected for provisioning, confirmed, required to be non-empty, sent to cloud-init as a SHA-512 crypt hash, and not saved in config.
- A **Server Type** default is the available option closest to 21 EUR monthly gross price, breaking ties by dedicated over shared, then RAM, then vCPU.
- A **Server Type** label reflects Hetzner's actual storage type and includes the Hetzner API name.
- Final **Personal Server** creation confirmation shows one maximum monthly gross EUR total from Hetzner live pricing for billable resources **Myn** is about to provision when pricing can be fetched, and explicitly says price is unavailable otherwise.
- A **Personal Server Bootstrap** runs through cloud-init during server creation.
- **Personal Server Bootstrap** cloud-init user data is rendered through a typed Go render function using a YAML library.
- A **Personal Server Bootstrap** installs the latest Ubuntu security updates after the **Personal Server** is created, enables unattended security upgrades, and reboots automatically if required.
- **Personal Server Bootstrap** installs and joins Tailscale before the rest of the development tool bootstrap.
- **Personal Server Bootstrap** runs `tailscale up` with a one-off **Tailscale Machine Auth Key**, the selected **Tailscale Host**, and Tailscale SSH enabled.
- **Personal Server Bootstrap** handles the **Tailscale Machine Auth Key** through a root-only file and removes it after successful Tailscale join.
- **Personal Server Bootstrap** disables system OpenSSH after Tailscale SSH is enabled.
- **Personal Server Bootstrap** does not write `ssh_authorized_keys` for root or the **Personal Server User**.
- **Personal Server Bootstrap** does not install `mosh`.
- A **Personal Server Bootstrap** installs Docker Engine and the compose plugin from Docker's official apt repository.
- **Personal Server** creation is not considered complete until the **Personal Server Bootstrap** finishes.
- The **Personal Server Bootstrap** writes a completion marker with status, timestamp, reboot information, and installed tool versions that **Myn** polls for over SSH.
- `myn configure` waits up to eight minutes for Tailscale device registration, expected tag, authorization, online state, and SSH reachability through the **Tailscale Host**.
- `myn configure` reads the **Personal Server Bootstrap** completion marker over Tailscale SSH as the **Personal Server User**.
- The **Personal Server Bootstrap** completion marker is readable by the **Personal Server User**.
- The **Personal Server Bootstrap** completion marker reports the installed `tailscale` version.
- The **Personal Server Bootstrap** must finish within five minutes after the **Personal Server** first accepts Tailscale SSH.
- Automatic reboot downtime after Tailscale SSH reachability counts against the five-minute **Personal Server Bootstrap** timeout.
- The **Personal Server Bootstrap** install plan is shown before final creation confirmation.
- The **Personal Server Bootstrap** install plan groups system services, Homebrew tools, and coding agents separately.
- The **Personal Server Bootstrap** install plan mentions Tailscale-only access, Tailscale SSH, IPv6-only public networking, no public inbound firewall rules, and disabled system OpenSSH.
- Homebrew and Homebrew-installed tools belong to the **Personal Server User**, not root.
- **Personal Server Bootstrap** does not copy local dotfiles or shell configuration beyond required Homebrew, nvm, Git identity setup, and the repo-owned **Personal Server tmux Profile**.
- **Personal Server Bootstrap** installs the **Personal Server tmux Profile** for the **Personal Server User** instead of reading the user's local `~/.tmux.conf` at provisioning time.
- Every new **Personal Server** receives the **Personal Server tmux Profile** without an extra prompt or saved configuration flag.
- **Personal Server Bootstrap** writes the **Personal Server tmux Profile** during creation; this does not define a later repair or update policy for user-edited tmux configuration.
- The **Personal Server tmux Profile** is installed as `/home/<Personal Server User>/.tmux.conf`, owned by the **Personal Server User**, not as system-wide tmux configuration.
- The **Personal Server tmux Profile** is a literal snapshot of the intended tmux settings, not a curated subset.
- The repo-owned **Personal Server tmux Profile** is the source of truth after it is seeded; tests and provisioning do not compare against the user's live local `~/.tmux.conf`.
- The **Personal Server Bootstrap** install plan does not call out the **Personal Server tmux Profile** separately from the standard tool setup.
- `tmux` is installed through Homebrew as part of the **Personal Server Bootstrap**.
- `rustup` is installed through Homebrew, but no Rust toolchain is installed during the **Personal Server Bootstrap**.
- Go is installed through the Homebrew `go` formula during the **Personal Server Bootstrap**.
- GitHub CLI is installed through Homebrew, but GitHub authentication is not configured by **Personal Server Bootstrap**.
- Git identity on the **Personal Server** matches available local Git identity values, read from global Git config first and repo-local Git config second, and reports skipped missing values.
- `nvm` is installed through Homebrew, initialized for the **Personal Server User**, and used to install and default the latest LTS Node.js.
- Codex is installed as the **Personal Server User** with the nvm-managed LTS Node.js npm.
- Claude Code is installed as the **Personal Server User** with its official installer script.
- Coding agent installation failures do not fail the **Personal Server Bootstrap**; they are reported as partial failures.
- **Personal Server Bootstrap** hard-fails system update/security setup, user creation, Tailscale install/join, remote project root creation, Homebrew, Docker, core Homebrew tools, nvm, and LTS Node/npm setup.
- **Personal Server Bootstrap** hard-fails writing the **Personal Server tmux Profile**.
- **Personal Server Bootstrap** hard-fails disabling system OpenSSH after Tailscale SSH is enabled.
- A **Stdio Lease** is active only while the wrapped command still exists and terminal input or output is recent.
- A quiet **Stdio Lease** can become idle while the wrapped command is still waiting at a prompt.
- A completed **Stdio Lease** is removed on normal wrapper exit.
- An expired **Idle Lease** is stale, not merely idle.
- **Idle Lease** heartbeat is distinct from terminal activity.
- A **Stdio Lease** defaults to a 30 minute idle window.
- `myn idle status` reports **Idle Lease** state without removing lease files.
- A **Stdio Lease** requires a terminal-backed stdin and stdout.

## Example dialogue

> **Dev:** "Should `configure` create the **Personal Server** immediately?"
> **Domain expert:** "Interactive setup can create it after confirmation, but non-interactive setup must opt in explicitly."
> **Dev:** "Should `configure` ask for the Hetzner token?"
> **Domain expert:** "No — run authentication first, then `configure` can use the saved **Hetzner Credentials**."
> **Dev:** "Should the prompt ask for a region?"
> **Domain expert:** "No — Hetzner calls this a **Location**, so the CLI should too."
> **Dev:** "Where should selected cloud details be saved?"
> **Domain expert:** "In a top-level **Personal Server Configuration**, separate from auth and project roots."
> **Dev:** "Should **Personal Server Configuration** store selected Location and Server Type?"
> **Domain expert:** "No — store only the server ID, **Personal Server User**, **Tailscale Host**, and public IPv6 inventory address."
> **Dev:** "Should cloud choices be saved if final creation is declined?"
> **Domain expert:** "No — they are transient unless a **Personal Server** is actually created."
> **Dev:** "Should `configure` replace an existing configured **Personal Server**?"
> **Domain expert:** "No — skip creation by default and report the saved server ID, **Tailscale Host**, and public IPv6 inventory address."
> **Dev:** "Should `configure` trust a saved server ID?"
> **Domain expert:** "No — verify it exists in Hetzner before skipping creation."
> **Dev:** "Should stale **Personal Server Configuration** be cleared automatically?"
> **Domain expert:** "No — ask in interactive mode and fail in non-interactive mode."
> **Dev:** "Should non-interactive `configure` create a **Personal Server** with flags?"
> **Domain expert:** "No — skip non-interactive creation for this flow."
> **Dev:** "Should missing **Hetzner Credentials** fail all of `configure`?"
> **Domain expert:** "No — save local configuration and skip **Personal Server** creation with a clear message."
> **Dev:** "Should `configure` create a **Personal Server** automatically after prompts?"
> **Domain expert:** "No — ask explicitly before creating cloud resources."
> **Dev:** "Should `myn configure` delete the server if bootstrap fails?"
> **Domain expert:** "No — save the created server ID, **Tailscale Host**, and public IPv6 inventory address so the user can inspect it."
> **Dev:** "Should the firewall name describe only SSH?"
> **Domain expert:** "No — keep the reusable `myn-personal-server` name, but the desired rule set has no public inbound rules."
> **Dev:** "If the firewall already exists, should **Myn** restore SSH-only rules?"
> **Domain expert:** "Yes, but only when the existing firewall is clearly Myn-managed; otherwise fail rather than clobbering user-managed rules."
> **Dev:** "Should **Mosh Access** be installed by default?"
> **Domain expert:** "No — new **Personal Servers** use Tailscale SSH only."
> **Dev:** "Should public SSH be available as a recovery path?"
> **Domain expert:** "No — no public inbound SSH, no root SSH, no local SSH key injection."
> **Dev:** "Can we rely on cloud-init for Tailscale access?"
> **Domain expert:** "Yes — cloud-init installs Tailscale first, joins with a one-off tagged key, enables Tailscale SSH, then disables system OpenSSH."
> **Dev:** "Should Tailscale policy be edited through the API?"
> **Domain expert:** "Check whether the policy already grants the required access; if not, show a semantic summary, ask, validate, and update with ETag protection."
> **Dev:** "Should **Myn** create the server's Tailscale auth key?"
> **Domain expert:** "Yes — create a fresh one-off, tagged, pre-approved key with a ten minute expiry immediately before the Hetzner create request."
> **Dev:** "Does 'same name as the local user' mean verbatim?"
> **Domain expert:** "No — default the **Personal Server User** from a Linux-safe normalized current Tailscale identity, and keep the prompt editable."
> **Dev:** "Which login shell should the **Personal Server User** start with?"
> **Domain expert:** "Bash; the user can change it later."
> **Dev:** "Should the **Personal Server User** have passwordless sudo?"
> **Domain expert:** "No — sudo must require a password."
> **Dev:** "Where do we save the **Personal Server User** password?"
> **Domain expert:** "We do not save it — collect it during provisioning and send only a password hash to cloud-init."
> **Dev:** "Can the **Personal Server User** password be empty?"
> **Domain expert:** "No — require confirmation and a non-empty password."
> **Dev:** "Should cloud-init receive the plaintext password?"
> **Domain expert:** "No — hash it locally with SHA-512 crypt and send only the hash."
> **Dev:** "Can the user choose any Hetzner server type?"
> **Domain expert:** "No — list only **Server Type** options available in the selected **Location**."
> **Dev:** "Should every **Server Type** label say NVMe SSD?"
> **Domain expert:** "No — display the storage type Hetzner reports for that **Server Type**."
> **Dev:** "Should the **Server Type** selector show price?"
> **Domain expert:** "No — price is only used internally to choose the default."
> **Dev:** "Should final confirmation show the monthly price?"
> **Domain expert:** "Yes — show the maximum possible monthly price for the resources **Myn** is about to provision."
> **Dev:** "Should **Myn** SSH into the server to install tools?"
> **Domain expert:** "No — install them through the **Personal Server Bootstrap** and wait for cloud-init to finish."
> **Dev:** "Can `configure` return once Hetzner accepts the create request?"
> **Domain expert:** "No — block and poll until the **Personal Server Bootstrap** completes or times out."
> **Dev:** "How should **Myn** know the **Personal Server Bootstrap** is done?"
> **Domain expert:** "Poll over ordinary `ssh` to the **Tailscale Host** for the bootstrap completion marker written by cloud-init."
> **Dev:** "Should `myn configure` wait for Tailscale before marker polling?"
> **Domain expert:** "Yes — wait separately for device registration, expected tag, authorization, online state, and SSH reachability."
> **Dev:** "Should system OpenSSH remain available after Tailscale SSH is enabled?"
> **Domain expert:** "No — disable system OpenSSH before writing the success marker."
> **Dev:** "How long should **Myn** wait for bootstrap?"
> **Domain expert:** "At most eight minutes for Tailscale join and SSH reachability, then five minutes for the marker after SSH is reachable."
> **Dev:** "When should users see the software list?"
> **Domain expert:** "Before final creation confirmation, so it is part of what they approve."
> **Dev:** "Should Homebrew run as root?"
> **Domain expert:** "No — install and run Homebrew as the **Personal Server User**."
> **Dev:** "Should `tmux` be installed through apt or Homebrew?"
> **Domain expert:** "Homebrew — it belongs with the user-owned development tools."
> **Dev:** "Should the **Personal Server tmux Profile** come from the user's live local `~/.tmux.conf`?"
> **Domain expert:** "No — **Myn** should install a repo-owned tmux profile so Personal Server provisioning stays reproducible."
> **Dev:** "Should the **Personal Server tmux Profile** be curated or a literal snapshot of the intended local tmux settings?"
> **Domain expert:** "Use a literal snapshot."
> **Dev:** "Should installing the **Personal Server tmux Profile** be optional during creation?"
> **Domain expert:** "No — every new Personal Server should receive it as part of the standard development environment."
> **Dev:** "If a later bootstrap-like flow finds an existing tmux config, should this decision require preserving it?"
> **Domain expert:** "No — this decision only says creation writes the standard profile; future repair or update behavior is separate."
> **Dev:** "Should the **Personal Server tmux Profile** be system-wide?"
> **Domain expert:** "No — install it as the Personal Server User's own `.tmux.conf`."
> **Dev:** "Should the creation plan mention the **Personal Server tmux Profile** separately?"
> **Domain expert:** "No — it does not need to mention the specific tmux configuration."
> **Dev:** "Should failing to write the **Personal Server tmux Profile** be treated as a partial setup issue?"
> **Domain expert:** "No — bootstrap should hard-fail because the expected development environment was not created."
> **Dev:** "Should failing to join Tailscale or disable system OpenSSH be treated as a partial setup issue?"
> **Domain expert:** "No — bootstrap should hard-fail because Tailscale SSH is the only access model."
> **Dev:** "After seeding the snapshot, should tests keep comparing it to the user's live local `~/.tmux.conf`?"
> **Domain expert:** "No — the repo-owned snapshot is canonical."
> **Dev:** "Should bootstrap install a Rust stable toolchain?"
> **Domain expert:** "No — install only the Homebrew `rustup` package."
> **Dev:** "Should the package be called `golang`?"
> **Domain expert:** "No — use the Homebrew formula name `go`."
> **Dev:** "Should `nvm` use the upstream install script?"
> **Domain expert:** "No — install Homebrew `nvm`, initialize it for the user, then install and default the latest LTS Node.js."
> **Dev:** "Should Codex be installed with system npm?"
> **Domain expert:** "No — install it with the **Personal Server User**'s nvm-managed LTS npm."
> **Dev:** "Should Claude Code be installed as root?"
> **Domain expert:** "No — run the official installer as the **Personal Server User**."
> **Dev:** "Should the image be limited to Ubuntu LTS?"
> **Domain expert:** "No — use Hetzner's latest Ubuntu image and apply the latest security updates during bootstrap."
> **Dev:** "Should an open Codex prompt keep the **Personal Server** awake forever?"
> **Domain expert:** "No — protect it with a **Stdio Lease**, and let the lease become idle when terminal input and output have both been quiet long enough."
> **Dev:** "When a **Project** already has multiple **Project Sessions**, should bare `myn connect` open the newest one?"
> **Domain expert:** "No — open the lowest-numbered existing **Project Session**, and create **Project Session** `1` only when none exist."
> **Dev:** "Should `myn connect-new` fill gaps in **Project Session** numbers?"
> **Domain expert:** "No — create one greater than the highest existing **Project Session** number."
> **Dev:** "Should `myn connect 2` create **Project Session** `2` if it is missing?"
> **Domain expert:** "No — numbered `myn connect` attaches only to an existing **Project Session**."
> **Dev:** "Should numbered **Project Session** names append `-2`, `-3`, and so on?"
> **Domain expert:** "No — append `:2`, `:3`, and so on so numbered **Project Sessions** cannot collide with another **Project**'s default **Project Session** name."

## Flagged ambiguities

- "remote machine" and "default server" both refer to **Personal Server** in this context.
- "region" was used to mean **Location** — resolved: the CLI uses **Location** everywhere.
- "lease" does not mean a permanent lock — resolved: an **Idle Lease** is renewable evidence of recent activity.
- "same tmux settings" does not mean live dotfile sync — resolved: **Personal Server Bootstrap** installs a repo-owned **Personal Server tmux Profile**.
- "GitHub package" was used to mean downloadable CLI assets — resolved: use **Myn Release** assets, not GitHub Packages.
- "Homebrew-like installer" means **Myn Installer**, not a Homebrew tap or **Personal Server Bootstrap** behavior.
- "session" now means **Project Session** when discussing `myn connect`, `myn connect-new`, or `myn sessions`.
- "SSH access" for a new **Personal Server** means **Tailscale SSH Access**, not public OpenSSH access or local SSH key injection.
- "Tailscale key" can mean either **Tailscale Credentials** or a **Tailscale Machine Auth Key** — resolved: **Tailscale Credentials** are saved API credentials; **Tailscale Machine Auth Keys** are one-off server join credentials and are not saved.
