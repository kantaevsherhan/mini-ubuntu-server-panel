.PHONY: web build check clean up down logs image
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
image:
	docker build -t mini-ubuntu-server-panel:$(VERSION) --build-arg VERSION=$(VERSION) .
up:
	docker compose up -d --build
	@echo "Panel: http://localhost:8080 (temporary password in: make logs)"
down:
	docker compose down
logs:
	docker compose logs -f panel
clean:
	rm -rf dist frontend/dist
