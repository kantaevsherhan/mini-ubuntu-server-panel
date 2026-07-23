.PHONY: web build check clean
VERSION ?= dev
web:
	rm -rf backend/cmd/mini-ubuntu-server/web/assets backend/cmd/mini-ubuntu-server/web/index.html
	cd frontend && bun install --frozen-lockfile && bun run build
build: web
	cd backend && go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o ../dist/mini-ubuntu-server ./cmd/mini-ubuntu-server
check:
	cd frontend && bun run check
	test -z "$$(gofmt -l backend)"
	cd backend && go test ./... && go vet ./...
	cd backend && golangci-lint run
	bash -n scripts/*.sh

format:
	cd frontend && bun run format
	gofmt -w backend
clean:
	rm -rf dist frontend/dist
