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

## Данные

SQLite хранит пользователей панели, аудит, Telegram-настройки и получателей, события и доставки уведомлений. Ubuntu-пользователи читаются из системных источников и не дублируют пароли в SQLite.

Все времена backend должны храниться в UTC. Frontend локализует отображение Moment.js после получения данных.

## Границы привилегий

Основной сервис работает от `mini-ubuntu-server`. Операции, требующие root, должны проходить только через узкий sudoers allowlist. Произвольные shell-команды из API запрещены.
