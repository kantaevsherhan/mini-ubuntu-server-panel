# Mini Ubuntu Server Panel

Тёмная desktop-first панель управления Ubuntu Server. Backend написан на Go и Fiber, frontend — Vue 3, TypeScript, PrimeVue 4 и Tailwind CSS. Интерфейс использует PrimeIcons, штатные темы Aura/Lara и поддерживает русский и английский языки.

> Проект находится в активной разработке. Авторизация, SQLite-схема, dashboard, пользователи панели, чтение Ubuntu-пользователей, базовые настройки Telegram, аудит и UI-каркас уже заложены. Docker, terminal, files, firewall, updater и notification worker пока представлены точками расширения.

## Стек

- Go 1.23, Fiber 2, REST API, JWT и SQLite (`modernc.org/sqlite`, без CGO);
- Vue 3, Vite, TypeScript, Vue Router и Pinia;
- PrimeVue 4, `@primeuix/themes` (Aura/Lara), PrimeIcons и Tailwind CSS;
- Bun для установки зависимостей и сборки frontend;
- ECharts, xterm.js и Monaco Editor подготовлены как frontend-зависимости;
- systemd и ограниченный sudoers для Ubuntu.

## Быстрый старт для разработки

Требуются Go 1.23+ и Bun 1.3+.

```bash
cd frontend
bun install
bun run dev
```

В другом терминале:

```bash
export MINI_UBUNTU_SERVER_JWT_SECRET="$(openssl rand -hex 32)"
export MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=admin
export MINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD='change-this-password'
cd backend
go run ./cmd/mini-ubuntu-server --config ../packaging/config.example.yml
```

Vite проксирует `/api` на `127.0.0.1:8080`. Первый администратор создаётся только в пустой базе и получает флаг обязательной смены пароля. Пароль никогда не выводится backend в лог.

Production-сборка с frontend внутри Go binary:

```bash
make build VERSION=v0.1.0
```

## Установка

После замены `OWNER` на владельца публичного GitHub-репозитория:

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/mini-ubuntu-server-panel/main/install.sh | sudo bash
```

Определённая версия и параметры:

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/mini-ubuntu-server-panel/main/install.sh \
  | sudo bash -s -- --version v1.0.0 --port 8080 --username admin
```

Более безопасный вариант — сначала проверить скрипт:

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/mini-ubuntu-server-panel/main/install.sh -o install.sh
less install.sh
sudo bash install.sh
rm install.sh
```

Установщик поддерживает Ubuntu `amd64`/`arm64`, проверяет SHA-256 релизного архива, создаёт отдельного пользователя, конфигурацию, секреты и включает `mini-ubuntu-server.service`. После успешного health-check bootstrap-переменные удаляются из `secrets.env`; в SQLite остаётся только bcrypt-хеш временного пароля.

## Управление сервисом

```bash
sudo systemctl status mini-ubuntu-server
sudo systemctl restart mini-ubuntu-server
sudo journalctl -u mini-ubuntu-server -f
```

Данные находятся в `/var/lib/mini-ubuntu-server`, конфигурация — в `/etc/mini-ubuntu-server`, бинарный файл — `/opt/mini-ubuntu-server/bin/mini-ubuntu-server`.

## Темы и языки

В `Настройки → Интерфейс` можно переключать Aura/Lara, dark/light, accent color и RU/EN. Выбор хранится только в браузере в `localStorage`. Для всех действий интерфейса используются классы PrimeIcons `pi pi-*`.

## Безопасность

- пользователи панели отделены от пользователей `/etc/passwd`;
- пароль панели хранится как bcrypt-хеш, Ubuntu-пароли не читаются;
- JWT подписывается секретом из `/etc/mini-ubuntu-server/secrets.env` с правами `0640`;
- Telegram Bot Token должен передаваться через `MINI_UBUNTU_SERVER_TELEGRAM_BOT_TOKEN` и не хранится открытым текстом в SQLite;
- административные операции фиксируются в `audit_events`, секреты маскируются;
- системный сервис использует systemd hardening.

## Лицензия

Добавьте выбранный файл лицензии перед публичным релизом.
