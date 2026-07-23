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

Fiber API организован по доменам, а не одним монолитным handler-файлом: auth, dashboard/metrics, users/system users, processes, Telegram, notification rules и audit имеют отдельные исходники. Общий `api.go` отвечает только за зависимости, route registration и health endpoint.

## Данные

SQLite хранит пользователей панели, аудит, Telegram-настройки и получателей, события и доставки уведомлений. Ubuntu-пользователи читаются из системных источников и не дублируют пароли в SQLite.

Notification worker использует durable events/deliveries, rule-recipient links и incident state. Cooldown применяется после recovery, repeat interval — пока incident активен; recovery закрывает incident и отменяет pending stale deliveries. После restart записи `sending` возвращаются в `pending`.

Все времена backend должны храниться в UTC. Frontend локализует отображение Moment.js после получения данных.

## Границы привилегий

Основной сервис работает от `mini-ubuntu-server`. Создание и удаление Ubuntu-пользователей проходит через subcommand `privileged-user`: sudoers разрешает только точный путь бинарного файла и точное имя subcommand без wildcard. Запрос передаётся JSON через stdin, повторно валидируется уже после перехода к root и выполняется без shell. Произвольные команды из API запрещены.

Telegram Bot Token обновляется только через exact subcommand `privileged-secret telegram-token`, а сигналы процессам — через `privileged-process`. systemd не использует `NoNewPrivileges`, потому что это заблокировало бы проверенный sudo-переход; фактическое ограничение обеспечивают непривилегированный service user, exact sudoers commands и повторная root-side валидация. `ProtectSystem=strict` сохранён, а writable paths открыты для `/etc` и `/home`, необходимые `useradd` и atomic secrets update; сам service user не имеет Unix-прав записи в эти каталоги.
