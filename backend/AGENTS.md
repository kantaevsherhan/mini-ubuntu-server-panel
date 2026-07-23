# Backend rules

These rules apply to all files under `backend/`.

## Stack and boundaries

- Use Go and Fiber for HTTP and WebSocket endpoints.
- Keep handlers thin. Put domain logic in focused packages under `internal/`.
- Use GORM with the pure-Go SQLite driver for all runtime data access. Keep models and reusable repository queries in `internal/database`; handlers must not use `database/sql` or handwritten runtime SQL.
- Raw SQL is allowed only in versioned `internal/database/migrations/*.sql` files and the isolated migration executor. Never build SQL from untrusted strings.
- Keep migrations deterministic and backward-compatible. Updates must back up SQLite before migration.
- Return stable machine-readable API error codes; do not expose internal errors or secrets.
- Pass `context.Context` through slow I/O, Docker, Telegram, and system operations.
- Use timeouts for all external calls and bounded workers for background jobs.
- Never execute arbitrary shell strings. Prefer Go APIs or explicit executable/argument allowlists.

## Authentication and authorization

- Hash panel passwords with bcrypt or a stronger approved password KDF.
- Verify JWT signature, expiration, active-user state, role, and session policy.
- Enforce `admin`, `operator`, and `viewer` authorization in backend handlers, never only in frontend.
- Audit authentication, user, firewall, systemd, Docker, Telegram, update, and privileged system changes.
- Redact secrets before logging or writing audit details.

## System operations

- Treat panel users and Ubuntu users as independent entities linked only by `system_username`.
- Never read or persist Ubuntu password hashes from `/etc/shadow`.
- Validate usernames, groups, shells, home directories, SSH keys, and sudo intent.
- Multi-step user creation/deletion must use compensating rollback actions.
- Privileged commands must match the installed sudoers allowlist.

## Mandatory post-task commands

Run after every backend change:

```bash
gofmt -w .
go test ./...
go vet ./...
golangci-lint run
```

Add tests for new domain behavior and regression fixes. If Go or golangci-lint is unavailable, report the skipped check explicitly and rely on CI only as a secondary check.
