.PHONY: web build check clean
VERSION ?= dev
web:
	cd frontend && bun install --frozen-lockfile && bun run build
build: web
	cd backend && go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o ../dist/mini-ubuntu-server ./cmd/mini-ubuntu-server
check:
	cd frontend && bun run typecheck
	cd backend && go test ./...
clean:
	rm -rf dist frontend/dist
