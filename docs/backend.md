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
| GET | `/processes` | authenticated |
| POST | `/processes/:pid/signal` | admin/operator |
| GET | `/services` | admin/operator |
| POST | `/services/:unit/action` | admin/operator |
| GET | `/docker/containers` | admin/operator |
| POST | `/docker/containers/:id/action` | admin/operator |
| GET | `/firewall` | admin/operator |
| POST | `/firewall/rules` | admin |
| DELETE | `/firewall/rules/:number` | admin |
| GET | `/logs?unit=&priority=&range=hour|day|week&limit=` | admin/operator |
| GET | `/users` | authenticated |
| POST | `/users` | admin |
| GET | `/system-users` | admin/operator |
| GET | `/users/:id/system-details` | admin/operator |
| GET/PUT | `/telegram/settings` | admin |
| PUT | `/telegram/token` | admin |
| GET/PUT | `/notifications/rules[/:key]` | admin |
| GET | `/notifications/history` | admin/operator |
| GET | `/audit` | admin |

Ответы об ошибках содержат стабильное поле `error` и не раскрывают внутреннее сообщение Go/SQLite.

HTTP слой разделён по предметным файлам: `auth_handlers.go`, `dashboard_handlers.go`, `user_handlers.go`, `telegram_handlers.go`, `notification_rules.go`, `process_handlers.go` и `audit_handlers.go`. `api.go` содержит только dependency wiring, routes и health-check; новые модули не должны возвращать обработчики в единый большой файл.

`POST /users` поддерживает независимые флаги `create_panel_user` и `create_system_user`. Для Ubuntu-пользователя доступны `system_username`, `home_directory`, `shell`, `system_groups`, `allow_sudo`, `create_home`, `allow_ssh` и `ssh_public_key`. Если системная запись создана, но запись панели сохранить не удалось, backend вызывает компенсирующее удаление системного пользователя и его только что созданной домашней директории.

`DELETE /users/:id` принимает `delete_panel_user`, `delete_system_user`, `delete_home_directory`, `delete_ssh_keys` и `terminate_sessions`. Удаление home разрешено только вместе с Ubuntu-пользователем. Если root-helper отклонил системный шаг после удаления panel-записи, backend транзакционно восстанавливает пользователя панели и snapshot его web-сессий.

`GET /users/:id/system-details` связывает `system_username` с актуальными системными данными, ничего не копируя в SQLite: UID/GID, home, shell и группы берутся из NSS/passwd, sudo определяется по группам, наличие ключей — по `authorized_keys`, активные сессии — через `who --ips`, последний вход — через `last -F`. Временные значения возвращаются в RFC 3339 и форматируются Moment.js на frontend по выбранному языку.

## Root-helper

Основной процесс работает без root. Единственная пользовательская привилегированная команда — `/opt/mini-ubuntu-server/bin/mini-ubuntu-server privileged-user`, закреплённая в sudoers без wildcard-аргументов. Helper:

- доступен только через `sudo -n` и проверяет effective UID;
- принимает не более 32 KiB JSON через stdin и отклоняет неизвестные поля;
- разрешает только allowlist shell, безопасные username/group и home внутри `/home`;
- запускает `useradd`/`userdel` через массив аргументов без shell;
- проверяет формат публичного SSH-ключа и выставляет права `.ssh`/`authorized_keys`;
- не принимает и не хранит Ubuntu-пароли.

Bot Token изменяется отдельным exact subcommand `privileged-secret telegram-token`. Значение проходит allowlist-валидацию, поступает через stdin, атомарно заменяется в `secrets.env` с сохранением owner/mode и никогда не попадает в argv, SQLite, ответ API или аудит. Telegram client перечитывает файл перед запросом, поэтому restart сервиса не нужен.

Список процессов читается непривилегированно из `/proc`. Из исчезнувших или недоступных процессов данные не возвращаются. Управляющий endpoint принимает только числовой PID больше 1 и сигналы `HUP`, `TERM`, `KILL`. Сигнал передаётся JSON через stdin в exact subcommand `privileged-process`, повторно проверяется после перехода к root и отправляется напрямую через `kill(2)` без shell. Успешная операция фиксируется в аудите без содержимого командной строки процесса.

Systemd adapter объединяет `systemctl list-units` и `list-unit-files`, чтобы показать активные и неактивные установленные сервисы. Изменения выполняются exact subcommand `privileged-service`: unit name проверяется строгим шаблоном с обязательным `.service`, доступны только `start`, `stop`, `restart`, `enable`, `disable`, shell не используется. Собственный unit панели заблокирован, успешные действия записываются в аудит.

Docker adapter использует поддерживаемые модули `github.com/moby/moby/client` и `github.com/moby/moby/api` с автоматическим согласованием Engine API. Endpoint возвращает все контейнеры, включая остановленные. Действия принимают только hex container ID длиной 12–64 и allowlist `start`, `stop`, `restart`, `remove`; remove никогда не использует `force` и не удаляет volumes. Все успешные изменения записываются в аудит.

UFW adapter работает только через exact sudoers subcommand `privileged-firewall`. JSON повторно валидируется после root-перехода; разрешены status, добавление inbound `allow`/`deny` для одного TCP/UDP-порта и удаление numbered rule. Source принимает только `any`, IP или CIDR. Deny порта 22, enable/disable/reset и произвольные UFW arguments запрещены. Команды запускаются без shell, изменения доступны только admin и пишутся в аудит.

Journald adapter вызывает exact subcommand `privileged-logs`. Unit принимает только корректное имя `.service`, priority и временной диапазон выбираются из allowlist, limit ограничен 1–2000. Root-helper формирует фиксированный массив аргументов `journalctl` без shell, читает JSON lines, пропускает некорректные записи и обрезает каждое сообщение до 8 KiB. В API не возвращаются произвольные journal fields.

## Очередь уведомлений

Правила seeded для ресурсных, Docker, systemd, security и system событий. Каждое правило задаёт enabled, severity, выбранных Telegram recipients, cooldown, repeat interval и recovery notification. Worker хранит incident state в SQLite:

- повторный сигнал активной проблемы подавляется до repeat interval;
- после recovery новый сигнал подавляется на cooldown, чтобы избежать flapping;
- recovery закрывает активные события и отменяет ещё не отправленные stale deliveries;
- выбранные recipients переопределяют audience defaults;
- delivery использует retry/exponential backoff, terminal failed status и восстановление записей `sending` после restart;
- frontend показывает виртуализированные rules/history таблицы и форматирует время Moment.js по RU/EN locale.

Сетевые ошибки Telegram нормализуются до безопасного сообщения до записи в SQLite; Bot Token не входит в URL/error, API истории возвращает только безопасный delivery error code.

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

Если Go отсутствует на хосте, проверки можно выполнить официальным Docker image `golang:1.24`.
