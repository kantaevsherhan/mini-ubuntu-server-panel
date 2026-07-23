# Состояние проекта и TODO

Дата актуализации: 24 июля 2026.

## Сделано

- [x] Структура Go/Fiber backend и Vue 3 frontend.
- [x] SQLite WAL schema для panel users, audit, Telegram recipients и notification queue.
- [x] JWT login, bcrypt password hashes и роли admin/operator/viewer.
- [x] Актуальная проверка active/role на защищённых запросах.
- [x] Базовые API пользователей панели и чтение `/etc/passwd`.
- [x] PrimeVue 4, PrimeIcons, Aura/Lara, dark/light и accent colors.
- [x] RU/EN и локализованный Moment.js formatter.
- [x] Desktop Splitter navigation и mobile Drawer navigation.
- [x] ESLint, Prettier, TypeScript check, gofmt, tests/vet pipeline и CI.
- [x] API/login rate limiting, CSP, security headers и SSRF validation.
- [x] Скрипты install/update/uninstall/release и systemd unit.
- [x] SHA-256 release verification и update backup/rollback для binary.
- [x] GitHub Actions для amd64/arm64 releases.
- [x] Централизованный PrimeVue Toast для API и network errors.
- [x] GORM ORM с pure-Go SQLite driver вместо runtime raw SQL.
- [x] Processes: чтение `/proc`, RBAC API, allowlist сигналов через root-helper, аудит и PrimeVue virtual DataTable.
- [x] Systemd services: installed/loaded units, RBAC actions, protected panel unit, audit и PrimeVue virtual DataTable.
- [x] Docker SDK: API negotiation, container list/actions, strict ID/action validation, audit и PrimeVue virtual DataTable.
- [x] Firewall: UFW status/rules, admin-only mutation, root-side allowlist, SSH port protection, audit и PrimeVue UI.
- [x] Logs: journald allowlist query через root-helper, bounded structured response, RBAC и PrimeVue virtual DataTable.
- [x] Files: root-owned directory allowlist, traversal/symlink protection, atomic UTF-8 operations, audit, FileUpload и lazy Monaco editor.

## В работе / следующий приоритет

- [x] Исторические CPU/RAM метрики: SQLite samples, `/proc` collector, range API и ECharts день/неделя/месяц/всё время.
- [x] Полный CRUD panel users, web sessions и обязательная смена временного пароля.
- [x] Транзакционное создание panel + Ubuntu user с compensating rollback и integration test.
- [x] Связь `system_username`: группы, sudo, SSH keys, web/Ubuntu sessions и последний Ubuntu login.
- [x] Telegram getMe/getUpdates/sendMessage, SSRF-safe transport и recipients UI.
- [x] Привилегированное изменение Telegram Bot Token через root-helper stdin без передачи token в argv, лог или SQLite.
- [x] Notification queue worker с delivery status, retry, exponential backoff и dedup.
- [x] Notification rules UI: per-event severity, recipients, cooldown, repeat interval, recovery и delivery history.
- [x] Terminal: unprivileged PTY, xterm.js, resizable/fullscreen workspace и start/end audit без command logging.
- [x] WebSocket: short-lived single-use ticket in subprotocol header, IP/RBAC/origin validation, message/session rate and size limits.
- [ ] Все Settings sections. RBAC-aware navigation, route guards, account identity и admin-only actions уже готовы.
- [x] Добавить минимальные осмысленные micro-interactions: hover/focus для действий и ссылок, обратная связь выбора файла, мягкое раскрытие панелей и изменение статуса; поддержать `prefers-reduced-motion` и исключить декоративные тяжёлые анимации.
- [x] Versioned embedded SQLite migrations with transactional application tests.
- [ ] CLI subcommands `update` и `uninstall` внутри binary.

## Известные ошибки и ограничения

- Backend tests покрывают migrations, auth, Telegram, notification worker, processes, systemd, Docker, firewall, logs, files, terminal tickets/origin, валидацию root-helper и compensating rollback; для остальных security flows покрытие ещё требуется.
- `scripts/update.sh` health-check пока использует порт `8080`, а не читает значение из config.
- Rollback update восстанавливает binary, но не восстанавливает несовместимую SQLite migration.
- Installer использует GitHub API без authenticated token и может попасть под rate limit.
- JWT находится в `sessionStorage`; XSS всё ещё может прочитать его. Нужны HttpOnly cookie sessions.
- Login limiter хранится в памяти и сбрасывается после рестарта.
- Audit login failures может расти при распределённой атаке; нужна retention/aggregation policy.
- Telegram SSRF DNS validation не заменяет защиту от DNS rebinding во время реального HTTP-запроса; transport должен повторно проверять конечный IP.
- Frontend placeholder routes ещё не реализуют реальные модули.
- UI пока не имеет автоматических component/e2e tests и screenshot regression tests.
- Production TLS не настраивается установщиком.

## Рекомендации

- Не публиковать development-версию напрямую в интернет.
- Ставить панель за Caddy/nginx с TLS, firewall allowlist и VPN/Tailscale.
- Добавить HttpOnly sessions, CSRF, TOTP/WebAuthn и session revocation.
- Добавить Prometheus-compatible metrics export и retention tiers.
- Применить downsampling: raw 24–48 часов, 5-minute aggregates 30 дней, hourly aggregates для all-time.
- Добавить CodeQL, Dependabot/Renovate, SBOM, signed releases и secret scanning.
- Добавить backup integrity check и регулярный restore drill.
- Выполнить threat modeling и независимый security audit перед v1.0.0.

## Критерии готовности v1

- [ ] Все ключевые модули имеют backend RBAC и audit.
- [ ] Полный user transaction/rollback покрыт integration tests.
- [ ] Telegram queue выдерживает retry/restart без дублей.
- [ ] Update проверен с успешной миграцией и автоматическим rollback.
- [ ] Mobile и desktop UI покрыты e2e smoke tests.
- [ ] Нет high/critical dependency vulnerabilities.
- [ ] Security review завершён, production deployment документирован.
