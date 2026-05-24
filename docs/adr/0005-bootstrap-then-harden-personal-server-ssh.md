# Bootstrap Then Harden Personal Server SSH

Superseded by `0006-tailscale-only-personal-server-access.md` for newly
provisioned Personal Servers.

`myn` uses root SSH as the initial bootstrap-time control path, then applies a Myn-owned OpenSSH daemon drop-in at `/etc/ssh/sshd_config.d/20-myn-hardening.conf` so ongoing access is key-only for the Personal Server User. Because applying that profile disables root SSH before the final success marker may be observed, `myn configure` can read the Personal Server Bootstrap marker as the Personal Server User after first trying root. If the hardening profile cannot be validated or applied, bootstrap fails clearly and leaves root SSH available as a recovery path instead of reporting an insecure Personal Server as ready.
