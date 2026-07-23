# Production deployment

## Supported boundary

The panel is installed on Ubuntu and listens on loopback behind a TLS reverse proxy. Do not expose the development Vite server or plain HTTP listener to the public internet. For internet-reachable administration, also require a VPN/Tailscale or an IP allowlist.

Use a dedicated DNS name and bind the application to loopback:

```yaml
# /etc/mini-ubuntu-server/config.yml
listen: 127.0.0.1:8080
```

After changing config:

```bash
sudo systemctl restart mini-ubuntu-server
curl -fsS http://127.0.0.1:8080/api/v1/health
```

## TLS reverse proxy

A minimal Caddy virtual host is:

```caddyfile
panel.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

Caddy must preserve the original `Host` and WebSocket upgrade for `/api/v1/terminal/ws`. Do not rewrite browser `Origin`: the backend intentionally requires it to match `Host`. Validate the deployed path:

```bash
curl -fsS https://panel.example.com/api/v1/health
```

Restrict direct access to port 8080 with UFW/security groups. Expose only HTTPS and the SSH port selected by the operator. The built-in firewall UI is not a substitute for an upstream cloud firewall or VPN policy.

## Least privilege

- Keep the service user `mini-ubuntu-server` without a login shell and without general sudo access.
- Do not broaden `/etc/sudoers.d/mini-ubuntu-server`; only exact embedded helper subcommands are supported.
- Leave Docker integration disabled unless required. Membership in the `docker` group is root-equivalent.
- Keep `allowed_directories` narrow. Never add `/`, `/home`, `/etc`, or `/var` as broad roots.
- Verify secrets and config ownership:

```bash
sudo stat -c '%U:%G %a %n' \
  /etc/mini-ubuntu-server/config.yml \
  /etc/mini-ubuntu-server/secrets.env
sudo visudo -cf /etc/sudoers.d/mini-ubuntu-server
sudo systemd-analyze security mini-ubuntu-server.service
```

Expected secret permissions are `root:mini-ubuntu-server 640`. Never paste `secrets.env`, Telegram Bot Token, JWT secret, temporary passwords, terminal input, or `/etc/shadow` into issue reports.

## Backups and recovery

Before maintenance, stop the service or use the built-in updater so SQLite/WAL/SHM are captured consistently:

```bash
sudo mini-ubuntu-server update
sudo find /var/lib/mini-ubuntu-server/backups -maxdepth 2 -type f -ls
```

Keep encrypted off-host backups with a tested retention policy. A backup is not trusted until a restore drill opens the copied database, runs migrations and passes `/api/v1/health`. The updater automatically restores the previous binary and stopped-service database snapshot if startup or health fails.

## Monitoring and incident response

- Monitor `systemctl is-active mini-ubuntu-server`, health status, disk space and failed login/audit volume.
- Configure Telegram alerts only to explicitly approved user/chat IDs.
- On suspected web-session theft, disable the account or revoke its sessions. Rotating `MINI_UBUNTU_SERVER_JWT_SECRET` invalidates every JWT and requires a service restart.
- On suspected Telegram token exposure, update it in Settings and revoke the old token with BotFather.
- On host compromise, isolate the server, preserve journal/audit/database evidence, rotate every secret from a clean host and rebuild rather than trusting an in-place cleanup.

## Release verification checklist

1. CI, Playwright desktop/mobile smoke, `bun audit`, Go tests/vet/lint and `govulncheck` are green.
2. Release archive SHA-256 matches `checksums.txt`.
3. Config and secrets permissions match the documented ownership.
4. TLS certificate is valid and HTTP is not publicly reachable.
5. Web terminal opens only for admin/operator and closes after session revocation.
6. Backup restore and update rollback have been exercised for the target release.

Checksums protect accidental corruption and mirror mismatch, but do not replace signed provenance. Signed releases/SBOM and an independent penetration test remain recommended before exposing the panel outside a private administrative network.
