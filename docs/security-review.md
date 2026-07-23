# Security review — 24 July 2026

## Scope and method

This internal v1 review covers authentication/session checks, RBAC routes, audit coverage, privileged helpers, terminal WebSocket, files, Telegram SSRF/secrets, Docker/systemd/UFW adapters, updater/uninstaller, SQLite migrations, frontend secret handling, dependencies and deployment controls.

Evidence used:

- route-by-route inspection of every mutating Fiber endpoint;
- unit/integration tests for auth, users rollback, helpers, module actions, notification restart/dedup and update rollback;
- Playwright smoke at desktop 1440×900 and Pixel 7 viewport;
- `bun audit`, `govulncheck`, Go tests/vet, golangci-lint and shell syntax checks;
- manual trust-boundary review against [architecture](architecture.md), [security controls](security.md) and [production deployment](production.md).

## Findings resolved for v1

| Severity | Finding | Resolution/evidence |
|---|---|---|
| High | Reachable Fiber/JWT/x.text vulnerabilities | Upgraded to Fiber 2.52.12, JWT 5.2.2, current `x/*` and Go 1.25; `govulncheck` reports zero reachable/imported-package vulnerabilities. |
| High | Arbitrary root command execution | No generic command endpoint; exact sudoers subcommands, stdin JSON, root-side validation and shell-free argv execution. |
| High | File traversal/symlink escape | Root index + relative paths, canonical validation at both layers, symlink rejection, bounded content and atomic writes. |
| High | Reusable/leaked terminal authentication | 30-second SHA-256-stored one-time ticket in WebSocket subprotocol, IP/web-session binding, exact origin and periodic revoke/role checks. |
| High | Update archive path or failed migration takeover | Fixed repository/version/asset, SHA-256, extraction of one exact regular entry, process lock, atomic replacement and binary+SQLite/WAL/SHM rollback. |
| Medium | Duplicate responsive workspaces doubled requests/toasts | Replaced two concurrently mounted `RouterView` instances with a single `matchMedia`-selected workspace; desktop/mobile e2e proves behavior. |
| Medium | Security-state session mutations lacked audit | Login/password/logout/session revoke now produce redacted audit events. |
| Medium | Notification restart could duplicate delivery | Durable `sending → pending` recovery is followed by exactly one delivery in restart integration coverage. |

## Open accepted risks

There are no known open Critical/High reachable dependency findings in the reviewed build. The following risks are accepted only for private/VPN deployment:

- JWT remains in `sessionStorage`; CSP and no third-party script origins reduce XSS exposure, but HttpOnly Secure SameSite cookie sessions plus CSRF protection are the preferred next design.
- TLS is terminated by Caddy/nginx rather than the installer. Loopback binding and the production checklist are mandatory.
- Docker socket access is root-equivalent and remains explicit opt-in.
- Login throttling is per-process memory state and resets on restart; upstream rate limits/VPN are required for public ingress.
- SHA-256 files are downloaded from the same GitHub release trust domain. Signed provenance and SBOM remain recommended.
- `govulncheck -show verbose` reports the unmaintained `x/crypto/openpgp` module advisory, but the project imports bcrypt only; no vulnerable OpenPGP package or symbol is imported/reachable and the upstream module has no fixed version for that advisory.
- No internal review can substitute for an independent penetration test. Public exposure is not approved without one.

## Decision

The internal v1 security review is complete for a loopback-bound, TLS-proxied, private administrative deployment following [production.md](production.md). Deployment that ignores those controls, enables Docker for untrusted operators or directly publishes port 8080 is outside the reviewed boundary.
