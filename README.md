# Mini Ubuntu Server Panel

Mini Ubuntu Server Panel — web-панель управления Ubuntu Server с backend на Go/Fiber и frontend на Vue 3. Проект ориентирован на Ubuntu 24.04, тёмный desktop-first интерфейс и установку из GitHub Releases.

Проект находится в активной разработке. Готов фундамент авторизации, SQLite, аудита, транзакционного создания panel/Ubuntu-пользователей, Telegram-настроек, очереди уведомлений, dashboard и production-упаковки. Docker, terminal, files, firewall и updater worker развиваются поэтапно.

Dashboard сохраняет минутные CPU/RAM samples из Linux `/proc` в SQLite и показывает ECharts-график за день, неделю, месяц или всё время с серверным downsampling.

## Имена проекта

| Назначение | Имя |
|---|---|
| GitHub-репозиторий | `mini-ubuntu-server-panel` |
| CLI и бинарный файл | `mini-ubuntu-server` |
| systemd service | `mini-ubuntu-server.service` |

GitHub: `https://github.com/kantaevsherhan/mini-ubuntu-server-panel`.

## Технологии

Backend:

- Go 1.23 и Fiber 2;
- REST API, JWT и bcrypt;
- SQLite без CGO;
- Linux `/proc`, `/sys` и системные API;
- systemd и ограниченный sudoers.

Frontend:

- Vue 3, Vite, TypeScript, Vue Router и Pinia;
- PrimeVue 4 и готовые компоненты PrimeVue;
- PrimeIcons для всех интерфейсных иконок;
- `@primeuix/themes` с Aura и Lara;
- Tailwind CSS только для layout и utility-классов;
- Moment.js для локализованного отображения даты и времени;
- Bun для зависимостей, проверок и сборки;
- ECharts, xterm.js и Monaco Editor для профильных модулей.

## Интерфейс

Интерфейс использует готовые PrimeVue-компоненты. Layout построен на `Menubar`, `PanelMenu`, `Splitter` и `SplitterPanel`; формы используют `Fluid`, `FloatLabel`, `InputText`, `Password`, `Select` и `Button`; данные отображаются через `DataTable`, `Tag`, `ProgressBar`, `Skeleton`, `Toast` и `ConfirmDialog`.

В `Настройки → Интерфейс` доступны:

- Aura или Lara;
- dark или light режим;
- accent color;
- русский или английский язык.

Настройки интерфейса сохраняются в `localStorage`. Даты на русском отображаются как `DD.MM.YYYY HH:mm`, на английском — как `MM/DD/YYYY h:mm A`.

## Локальная разработка

Требуются Go 1.23+ и Bun 1.3+.

Frontend:

```bash
cd frontend
bun install
bun run dev
```

Backend:

```bash
export MINI_UBUNTU_SERVER_JWT_SECRET="$(openssl rand -hex 32)"
export MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME=admin
export MINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD='change-this-password'
cd backend
go run ./cmd/mini-ubuntu-server --config ../packaging/config.example.yml
```

Vite проксирует `/api` на `127.0.0.1:8080`. Первый администратор создаётся только для пустой базы. Backend не выводит пароль в лог.

Production-сборка, встраивающая frontend в Go binary:

```bash
make build VERSION=v0.1.0
```

## Форматирование и проверки

После каждого изменения frontend обязательно запускаются форматирование, ESLint, TypeScript и production build:

```bash
cd frontend
bun run format
bun run check
```

Для Go используются `gofmt`, `go test`, `go vet` и `golangci-lint`. Полная проверка репозитория:

```bash
make check
```

Такие же проверки запускаются в GitHub Actions при push и pull request.

## Скрипты

Все рабочие скрипты находятся только в `scripts/`:

| Файл | Назначение |
|---|---|
| `scripts/install.sh` | установка из GitHub Release и проверка SHA-256 |
| `scripts/update.sh` | backup, обновление, health-check и rollback |
| `scripts/uninstall.sh` | интерактивное безопасное удаление |
| `scripts/release.sh` | проверка и сборка архивов amd64/arm64 |

## Установка

Одна команда:

```bash
curl -fsSL https://raw.githubusercontent.com/kantaevsherhan/mini-ubuntu-server-panel/main/scripts/install.sh | sudo bash
```

Определённая версия:

```bash
curl -fsSL https://raw.githubusercontent.com/kantaevsherhan/mini-ubuntu-server-panel/main/scripts/install.sh \
  | sudo bash -s -- --version v1.0.0
```

Дополнительные параметры:

```bash
sudo bash scripts/install.sh --port 8080 --username admin --data-dir /var/lib/mini-ubuntu-server
```

Более безопасный способ:

```bash
curl -fsSL https://raw.githubusercontent.com/kantaevsherhan/mini-ubuntu-server-panel/main/scripts/install.sh -o install.sh
less install.sh
sudo bash install.sh
rm install.sh
```

Установщик поддерживает Ubuntu `amd64` и `arm64`, сверяет SHA-256, создаёт системного пользователя, конфигурацию, секреты и включает `mini-ubuntu-server.service`. Временный пароль показывается один раз. После health-check bootstrap-переменные удаляются из `secrets.env`, а в SQLite остаётся только bcrypt-хеш.

## Управление сервисом

```bash
sudo systemctl status mini-ubuntu-server
sudo systemctl restart mini-ubuntu-server
sudo systemctl stop mini-ubuntu-server
sudo journalctl -u mini-ubuntu-server -f
```

Основные пути:

- `/opt/mini-ubuntu-server/bin/mini-ubuntu-server`;
- `/etc/mini-ubuntu-server/config.yml`;
- `/etc/mini-ubuntu-server/secrets.env`;
- `/var/lib/mini-ubuntu-server/mini-ubuntu-server.db`;
- `/var/lib/mini-ubuntu-server/backups`;
- `/var/log/mini-ubuntu-server`.

## Обновление и удаление

```bash
sudo bash scripts/update.sh v1.1.0
sudo bash scripts/uninstall.sh
```

Обновление проверяет checksum, создаёт резервную копию бинарного файла и SQLite, перезапускает сервис и выполняет health-check. При ошибке восстанавливается предыдущий бинарный файл. Uninstall по умолчанию сохраняет данные, пока пользователь явно не подтвердит их удаление.

## Безопасность

- пользователи панели отделены от Ubuntu-пользователей;
- Ubuntu-пароли и `/etc/shadow` не сохраняются в SQLite;
- совместное создание panel/Ubuntu-пользователя компенсирует уже созданную системную запись при ошибке SQLite;
- при удалении можно независимо выбрать panel-запись, Ubuntu-пользователя, home, SSH-ключи и завершение сессий; ошибка root-helper восстанавливает panel-запись и web-сессии;
- пароли панели хранятся как bcrypt-хеши;
- JWT и Telegram Bot Token передаются через защищённый `secrets.env`;
- секреты маскируются в логах и аудите;
- роли `admin`, `operator`, `viewer` проверяются backend;
- системные пользователи создаются через один root-helper с точным sudoers-правилом; параметры передаются JSON через stdin, валидируются и не интерпретируются shell;
- карточка связанного Ubuntu-пользователя показывает UID/GID, home, shell, группы, sudo, наличие SSH-ключей, активные login-сессии и последний вход;
- привилегированные операции и изменения пользователей записываются в аудит;
- systemd unit использует hardening-параметры.

## Структура

```text
backend/    Go/Fiber API, доменные пакеты и SQLite
frontend/   Vue 3, PrimeVue 4 и Tailwind CSS
packaging/  systemd, sudoers и пример конфигурации
scripts/    install, update, uninstall и release
.github/    CI и GitHub Release workflows
```

## Лицензия

Перед публичным релизом добавьте выбранный файл `LICENSE`.
