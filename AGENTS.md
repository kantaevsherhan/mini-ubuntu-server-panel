# Project rules

These rules apply to the entire Mini Ubuntu Server Panel repository.

## Architecture

- Keep the backend in `backend/` and the frontend in `frontend/`.
- The production frontend must be embedded in the Go binary with `go:embed`.
- Use Bun for all frontend dependency and script operations. Do not introduce npm, pnpm, or Yarn commands.
- Keep installation, update, uninstall, and release scripts in `/scripts`; do not add duplicate root scripts.
- Use the product names consistently: repository `mini-ubuntu-server-panel`, CLI `mini-ubuntu-server`, service `mini-ubuntu-server.service`.
- Never commit credentials, generated secrets, databases, release archives, or `node_modules`.

## Security

- Never log passwords, JWT secrets, Telegram tokens, SSH private keys, or `/etc/shadow` data.
- Panel users and Ubuntu users are separate domains. Do not store Ubuntu passwords in SQLite.
- Privileged operations must use explicit allowlists, validation, audit records, and least-privilege sudoers rules.
- Validate all user-controlled paths, URLs, usernames, IDs, commands, and WebSocket input.
- Any multi-step panel/system-user operation must support rollback.

## Required checks after every task

Run checks after every request that changes project files, even when the change appears documentation-only.

1. Run `cd frontend && bun run format`.
2. Run `cd frontend && bun run check`.
3. Run `gofmt -w backend`.
4. Run `cd backend && go test ./... && go vet ./...`.
5. Run `cd backend && golangci-lint run`.
6. Run `bash -n scripts/*.sh` when shell scripts exist.

If a required tool is unavailable, run all remaining checks and explicitly report the skipped command and reason. Never claim an unavailable check passed.

## Change discipline

- Preserve unrelated user changes.
- Update README when commands, configuration, architecture, or installation behavior changes.
- Add or update tests for backend behavior and important frontend logic.
- Keep the working tree reviewable; generated frontend assets remain ignored and are rebuilt before Go release builds.
