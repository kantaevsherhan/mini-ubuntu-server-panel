# Backend

## Запуск

```bash
export MINI_UBUNTU_SERVER_JWT_SECRET="$(openssl rand -hex 32)"
export MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=admin
export MINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD='a-long-temporary-password'
cd backend
go run ./cmd/mini-ubuntu-server --config ../packaging/config.example.yml
```

## API

Базовый prefix: `/api/v1`.

| Method | Endpoint | Доступ |
|---|---|---|
| GET | `/health` | public |
| POST | `/auth/login` | public, rate-limited |
| GET | `/me` | authenticated |
| GET | `/dashboard` | authenticated |
| GET | `/users` | authenticated |
| POST | `/users` | admin |
| GET | `/system-users` | admin/operator |
| GET/PUT | `/telegram/settings` | admin |
| GET | `/audit` | admin |

Ответы об ошибках содержат стабильное поле `error` и не раскрывают внутреннее сообщение Go/SQLite.

## SQLite

SQLite работает в WAL mode, с foreign keys и одним writer connection. SQL-запросы параметризованы. Миграции сейчас выполняются идемпотентной схемой при старте; перед стабильным релизом их нужно перевести на версионированные файлы.

## Проверки

```bash
gofmt -w .
go test ./...
go vet ./...
golangci-lint run
```

Если Go отсутствует на хосте, проверки можно выполнить официальным Docker image `golang:1.23`.
