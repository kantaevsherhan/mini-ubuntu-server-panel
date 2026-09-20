# Frontend build: Vite writes straight into the Go embed directory.
# Node is required next to Bun because vue-tsc silently degrades to plain tsc without it
# and then fails to resolve .vue modules.
FROM node:22-bookworm-slim AS web
RUN npm install --global bun@1.3.14
WORKDIR /app
COPY frontend/package.json frontend/bun.lock ./frontend/
RUN cd frontend && bun install --frozen-lockfile
COPY frontend ./frontend
RUN cd frontend && bun run build

# Backend build with the frontend embedded into a single static binary.
FROM golang:1.25 AS api
ARG VERSION=dev
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
COPY --from=web /app/backend/cmd/mini-ubuntu-server/web ./cmd/mini-ubuntu-server/web
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/mini-ubuntu-server ./cmd/mini-ubuntu-server

FROM ubuntu:24.04
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --system mini-ubuntu-server \
  && useradd --system --gid mini-ubuntu-server --home-dir /var/lib/mini-ubuntu-server --shell /usr/sbin/nologin mini-ubuntu-server \
  && install -d -o mini-ubuntu-server -g mini-ubuntu-server -m 0750 /var/lib/mini-ubuntu-server /var/log/mini-ubuntu-server
COPY --from=api /out/mini-ubuntu-server /usr/local/bin/mini-ubuntu-server
COPY packaging/config.example.yml /etc/mini-ubuntu-server/config.yml
USER mini-ubuntu-server
EXPOSE 8080
VOLUME ["/var/lib/mini-ubuntu-server"]
# The JWT secret and the temporary administrator are created automatically on first start.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s \
  CMD curl -fsS http://127.0.0.1:8080/api/v1/health || exit 1
ENTRYPOINT ["/usr/local/bin/mini-ubuntu-server", "--config", "/etc/mini-ubuntu-server/config.yml"]
