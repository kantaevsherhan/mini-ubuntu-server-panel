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
| GET | `/metrics/history?range=day|week|month|all` | authenticated |
| GET | `/users` | authenticated |
| POST | `/users` | admin |
| GET | `/system-users` | admin/operator |
| GET/PUT | `/telegram/settings` | admin |
| GET | `/audit` | admin |

Ответы об ошибках содержат стабильное поле `error` и не раскрывают внутреннее сообщение Go/SQLite.

`POST /users` поддерживает независимые флаги `create_panel_user` и `create_system_user`. Для Ubuntu-пользователя доступны `system_username`, `home_directory`, `shell`, `system_groups`, `allow_sudo`, `create_home`, `allow_ssh` и `ssh_public_key`. Если системная запись создана, но запись панели сохранить не удалось, backend вызывает компенсирующее удаление системного пользователя и его только что созданной домашней директории.

`DELETE /users/:id` принимает `delete_panel_user`, `delete_system_user`, `delete_home_directory`, `delete_ssh_keys` и `terminate_sessions`. Удаление home разрешено только вместе с Ubuntu-пользователем. Если root-helper отклонил системный шаг после удаления panel-записи, backend транзакционно восстанавливает пользователя панели и snapshot его web-сессий.

## Root-helper

Основной процесс работает без root. Единственная пользовательская привилегированная команда — `/opt/mini-ubuntu-server/bin/mini-ubuntu-server privileged-user`, закреплённая в sudoers без wildcard-аргументов. Helper:

- доступен только через `sudo -n` и проверяет effective UID;
- принимает не более 32 KiB JSON через stdin и отклоняет неизвестные поля;
- разрешает только allowlist shell, безопасные username/group и home внутри `/home`;
- запускает `useradd`/`userdel` через массив аргументов без shell;
- проверяет формат публичного SSH-ключа и выставляет права `.ssh`/`authorized_keys`;
- не принимает и не хранит Ubuntu-пароли.

## SQLite и ORM

Runtime data access использует GORM и pure-Go SQLite driver `github.com/glebarez/sqlite`, поэтому release сохраняет `CGO_ENABLED=0`. SQLite работает в WAL mode, с foreign keys, prepared statements и одним writer connection. Raw SQL разрешён только в embedded versioned migration-файлах.

Collector раз в минуту читает aggregate CPU counters из `/proc/stat` и `MemTotal`/`MemAvailable` из `/proc/meminfo`. Исторический API группирует точки на стороне SQLite и ограничивает ответ 1000 точками.

## Проверки

```bash
gofmt -w .
go test ./...
go vet ./...
golangci-lint run
```

Если Go отсутствует на хосте, проверки можно выполнить официальным Docker image `golang:1.23`.
