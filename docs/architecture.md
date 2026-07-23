# Архитектура

## Общая схема

```text
Browser
  │ HTTPS / REST / WebSocket
  ▼
Go + Fiber
  ├─ JWT authentication and RBAC
  ├─ domain services
  ├─ SQLite
  ├─ /proc and /sys readers
  ├─ Docker SDK
  ├─ systemd / firewall adapters
  ├─ unprivileged PTY + ticketed WebSocket
  └─ Telegram delivery worker
```

Production frontend собирается Bun/Vite и копируется в `backend/cmd/mini-ubuntu-server/web`. Go встраивает каталог через `go:embed`, поэтому на Ubuntu-сервере Bun не требуется.

## Каталоги

- `backend/cmd/mini-ubuntu-server` — точка входа и embedded frontend;
- `backend/internal` — закрытые пакеты auth, database, HTTP API и системных интеграций;
- `frontend/src` — Vue-приложение;
- `packaging` — systemd, sudoers и пример конфигурации;
- `scripts` — установка, update, uninstall и release;
- `.github/workflows` — проверки и публикация релизов.

Fiber API организован по доменам, а не одним монолитным handler-файлом: auth, dashboard/metrics, users/system users, processes, systemd services, Docker, firewall, logs, files, terminal, Telegram, notification rules и audit имеют отдельные исходники. Общий `api.go` отвечает только за зависимости, route registration и health endpoint.

## Данные

SQLite хранит пользователей панели, аудит, Telegram-настройки и получателей, события и доставки уведомлений. Ubuntu-пользователи читаются из системных источников и не дублируют пароли в SQLite.

Notification worker использует durable events/deliveries, rule-recipient links и incident state. Cooldown применяется после recovery, repeat interval — пока incident активен; recovery закрывает incident и отменяет pending stale deliveries. После restart записи `sending` возвращаются в `pending`.

Все времена backend должны храниться в UTC. Frontend локализует отображение Moment.js после получения данных.

## Границы привилегий

Основной сервис работает от `mini-ubuntu-server`. Создание и удаление Ubuntu-пользователей проходит через subcommand `privileged-user`: sudoers разрешает только точный путь бинарного файла и точное имя subcommand без wildcard. Запрос передаётся JSON через stdin, повторно валидируется уже после перехода к root и выполняется без shell. Произвольные команды из API запрещены.

Telegram Bot Token обновляется только через exact subcommand `privileged-secret telegram-token`, сигналы процессам — через `privileged-process`, systemd actions — через `privileged-service`, UFW — через `privileged-firewall`, journald — через `privileged-logs`, allowlisted files — через `privileged-files`. systemd unit панели не использует `NoNewPrivileges`, потому что это заблокировало бы проверенный sudo-переход; фактическое ограничение обеспечивают непривилегированный service user, exact sudoers commands и повторная root-side валидация. `ProtectSystem=strict` сохранён, а writable paths открыты для `/etc` и `/home`, необходимые `useradd` и atomic secrets update; сам service user не имеет Unix-прав записи в эти каталоги.

Docker SDK подключается к стандартному daemon socket. Installer не выдаёт такой доступ по умолчанию: только `--enable-docker` добавляет service user в существующую группу `docker`. Это отдельная root-equivalent граница доверия, а не ограниченная sudo-операция; риск явно показывается при установке и описан в security documentation.

Maintenance не проходит через HTTP service: root запускает тот же binary как `mini-ubuntu-server update|uninstall`. Shell-файлы в `scripts/` являются только distribution entrypoints. Update останавливает отдельный systemd process, создаёт stopped-state snapshot базы, атомарно меняет executable и выполняет rollback при неуспешных migrations/start/health.

Web-терминал остаётся по непривилегированную сторону границы: PTY наследует UID/GID systemd-сервиса и не вызывает sudo-helper. От REST/JWT слоя к WebSocket передаётся только короткоживущий одноразовый ticket; terminal input не хранится в SQLite и не включается в audit details.
