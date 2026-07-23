# Расширение Mini Ubuntu Server Panel

Дата: 24 июля 2026

Статус: проектирование
Формат: технический план реализации без AI-функций

Этот документ фиксирует новые задачи после завершения текущего v1 scope. Это roadmap, а не перечень уже реализованных функций.

## 1. Цель и архитектурные границы

Расширить панель, сохранив Go/Fiber, Vue 3/Vite/TypeScript, PrimeVue, SQLite, exact-allowlist root-helper, RBAC `admin|operator|viewer`, audit, безопасные WebSocket-сессии и GitHub Releases update/rollback.

Новые функции переиспользуют текущие auth/RBAC, audit, Toast, ConfirmDialog, PrimeVue Aura/Lara, RU/EN, health-check, backup и rollback.

Backend-пакеты:

```text
backend/internal/
├── search/             ├── resource_map/      ├── server_explorer/
├── status_page/        ├── network/           ├── disk_health/
├── sandbox/            ├── workspace/         ├── ssh_manager/
├── incident/           ├── goals/             ├── maintenance/
├── timeline/           └── plugins/
```

Каждый пакет разделяет handler/service/repository/models/routes/permissions и имеет tests. Frontend-модули располагаются в `frontend/src/modules/` под соответствующими именами и каждый имеет route, Pinia store, API client, PrimeVue page, loading/skeleton, empty/error states, RBAC guard, RU/EN и e2e smoke.

Запрещены shell из пользовательской строки, secrets в argv, прямой root/plugin-helper/Docker socket, произвольные SSH jump/ProxyCommand, sandbox без limits и раскрытие внутренних данных public status page. Все privilege actions проходят точную helper-подкоманду со строгим stdin schema.

## 2. Global Search

`Ctrl+K` ищет Docker containers, systemd services, processes, panel/Ubuntu users, logs, allowlisted files, firewall rules, SSH hosts, plugins, settings и UI commands.

```http
GET /api/search?q=nginx&types=service,container,file&limit=50
```

Результат: `type`, stable `id`, `title`, `subtitle`, deep-link `route`, `score`, RBAC-filtered `actions`. Два уровня: SQLite FTS5 index для сохранённых сущностей и bounded live search по `/proc`, systemd, Docker и allowed directories.

```sql
CREATE TABLE search_documents (
  id TEXT PRIMARY KEY, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
  title TEXT NOT NULL, subtitle TEXT, keywords TEXT, route TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE VIRTUAL TABLE search_documents_fts USING fts5(
  title, subtitle, keywords, content='search_documents', content_rowid='rowid'
);
```

PrimeVue Dialog/Drawer: autofocus, debounce 150–250 ms, type filters, keyboard navigation, Enter=open, Ctrl+Enter=primary action и local recent searches. Поиск не аудируется, действия — аудируются. Local index target <300 ms; никаких недоступных роли объектов; files только внутри allowlist.

## 3. Live Resource Map

Связи: service→process→port, container→process→volume/network/port, nginx upstream→container, user→process/session. Источники: `/proc`, systemd D-Bus/allowlisted show, Docker SDK, `/proc/net`, mounts, разрешённые Nginx configs и SSH sessions.

```http
GET /api/resource-map/snapshot
GET /api/resource-map/stream
```

WebSocket передаёт bounded node updates. В `resource_map_snapshots(captured_at,payload_json)` сохраняются только периодические snapshots.

ECharts Graph или Cytoscape.js: System/Docker/Network/Users/Selected modes, zoom/pan, filters, problem highlighting, details drawer, resource deep links, pause/fullscreen. Нужны server filter, aggregation, node limit, lazy loading и kernel-thread hiding.

## 4. Server Explorer

Mount tree, directory size/growth, top/old files, optional duplicate hashes, usage map, File Manager deep links и безопасный cleanup.

```http
GET  /api/server-explorer/mounts
POST /api/server-explorer/scan
GET  /api/server-explorer/scans/:id
GET  /api/server-explorer/top-files
GET  /api/server-explorer/growth
POST /api/server-explorer/cleanup/preview
POST /api/server-explorer/cleanup/apply
```

Restart-safe background scanner: allowlisted roots, openat2-equivalent safe paths, no symlinks/special files, CPU/IO limits, cancel/progress, исключения `/proc|/sys|/dev`. Таблицы `filesystem_scans` и `filesystem_usage_snapshots`.

Cleanup v1: old journal, package cache, temp files, stopped containers, dangling images, unused build cache. Apply только после preview, expected bytes, ConfirmDialog, server revalidation и audit.

## 5. Public Status Page

Режимы disabled/public/token/password/IP-allowlist. Публикуются общий status/uptime, custom checks, latency, incident/maintenance messages и recent incidents. Не публикуются internal IP, process/command/path/user/Docker ID и package details; CPU/RAM скрыты по умолчанию.

Admin CRUD управляет config/checks. Public API:

```http
GET /status/api/v1/summary
GET /status/api/v1/incidents
```

Checks: HTTP(S), TCP, systemd, Docker и DNS; local command запрещён. Отдельный минимальный frontend bundle, rate limiter, CSP, strict CORS, без JWT. HTTP checks проверяют фактический IP при каждом connect; metadata/private ranges запрещены без explicit admin allowlist.

## 6. Network Analyzer

Interfaces, RX/TX realtime/history, listeners, TCP/UDP connections, top remote/local endpoints, process/container relation, optional DNS stats, conntrack, errors/drops и CSV export.

Источники: `/proc/net/{dev,tcp,tcp6,udp,udp6}`, `/proc/<pid>/fd`, netlink, Docker SDK, `/sys/class/net`.

```http
GET /api/network/interfaces
GET /api/network/connections
GET /api/network/listeners
GET /api/network/top
GET /api/network/history
GET /api/network/stream
```

`network_samples` хранит interface, rx/tx bytes/errors и time с retention/downsampling. Viewer получает masked remote IP и no command line; payload capture запрещён; raw connection history короткий или off.

## 7. Disk Health

Модель, masked serial, temperature, power-on hours, wear, reallocated/pending sectors, SMART, NVMe used %, filesystem/inodes/read-only/errors и alerts.

`smartctl --json` и `nvme smart-log --output-format=json` вызываются только exact helper operations. Device обязан существовать в `/sys/block`; произвольный path запрещён.

```http
GET  /api/disk-health/devices
GET  /api/disk-health/devices/:id
POST /api/disk-health/devices/:id/self-test
GET  /api/disk-health/history
```

Self-test: admin, confirmation, audit. Alerts: high temperature, SMART fail, wear >80%, filesystem/inodes >90%, read-only transition.

## 8. Sandbox

Временные ограниченные Docker containers, не root shell/VM: allowlisted image, command/env, small uploads, optional read-only allowed bind, mandatory CPU/RAM/PID/disk/TTL, network off, live logs/terminal и auto-delete.

```http
POST   /api/sandboxes
GET    /api/sandboxes
GET    /api/sandboxes/:id
POST   /api/sandboxes/:id/start
POST   /api/sandboxes/:id/stop
DELETE /api/sandboxes/:id
GET    /api/sandboxes/:id/logs
POST   /api/sandboxes/:id/ticket
```

Обязательно: `Privileged=false`, no host PID/network/socket/devices, drop all caps, seccomp/AppArmor, read-only rootfs, tmpfs `/tmp`, bounded resources и no binds `/|/etc|/proc|/sys|/var/run`.

Compose v1 принимает image, command, env, random localhost ports, tmpfs, ephemeral named volumes и limits. Запрещены privileged/devices/host network/PID/cap_add/socket/arbitrary binds.

## 9. Workspace

Private/shared saved layouts с tabs/Splitter для terminal, logs, metrics, Docker/service/file details и notes. Autosave debounce, templates, duplicate, secret-free JSON import/export, reset.

```sql
CREATE TABLE workspaces (
  id TEXT PRIMARY KEY, owner_user_id INTEGER NOT NULL, name TEXT NOT NULL,
  description TEXT, visibility TEXT NOT NULL DEFAULT 'private',
  layout_json TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
);
CREATE TABLE workspace_members (
  workspace_id TEXT NOT NULL, user_id INTEGER NOT NULL, permission TEXT NOT NULL,
  PRIMARY KEY (workspace_id,user_id)
);
```

Terminal не восстанавливается после restart; layout не хранит shell history.

## 10. SSH Manager

Сохранённые host/port/username/fingerprint/key reference/tags, connection test, web terminal, SFTP и recent sessions. Jump host — позже.

Private keys не хранятся открыто в SQLite: `/etc/mini-ubuntu-server/ssh-keys/`, root-owned 0600, encryption-at-rest от master key; passphrase только explicit opt-in, secrets не возвращаются frontend. SQLite содержит только `ssh_hosts` metadata.

Обязательны destination validation, explicit admin private-network approval, metadata deny, DNS-rebinding protection, session IP pinning, port allowlist, no ProxyCommand и обязательный host-key fingerprint. WebSocket переиспользует local terminal single-use ticket.

## 11. Incident Mode

Инциденты создаются вручную либо Goals, Disk Health, systemd/Docker events и notification rules. States: `open|investigating|monitoring|resolved|closed`.

При открытии собираются bounded CPU/RAM/network/disk snapshots, top processes, failed services, unhealthy/stopped containers, allowlisted logs, firewall/audit changes и Timeline. Таблицы `incidents`, `incident_events`, `incident_notes`.

Frontend: overview, timeline, metrics, logs, resources, notes/actions/status и report export. Автоматизация только deterministic; автоматические root fixes запрещены.

## 12. Goals

Декларативные metric/service/container/port/HTTP/disk/certificate/update/backup conditions в `goals`; `goal_states` хранит state, first failure, last check/value и consecutive failures.

Worker: fixed interval+jitter, timeout, no overlap, recovery, cooldown/dedup, notification linkage и optional incident. Frontend: visual rule builder, preview, current/history, mute/disable и maintenance suppression.

## 13. Maintenance Mode

Panel-wide, status-only или selected services/containers/goals/notifications; manual/scheduled interval, reason/public message, suppression, auto-expiration и audit. Хранение в `maintenance_windows` со scope JSON.

Goals продолжают вычисляться как `suppressed`; ожидаемые notifications/incidents блокируются; Timeline сохраняется; после завершения immediate recheck.

## 14. Resource Timeline и Event Bus

Единая хронология audit, metrics, network/disk, Docker/systemd/firewall/users, updates/backups, goals/incidents/maintenance и plugins.

In-process event bus + durable SQLite outbox используют immutable `SystemEvent` с ID/type/severity, optional ResourceRef/ActorRef, JSON payload и UTC time. `system_events` индексируется по time и resource.

Retention: critical/security/incidents — 1 год, operational — 90 дней, high-frequency transitions — 30 дней; configurable aggregation/vacuum.

```http
GET /api/timeline
GET /api/timeline/resources/:type/:id
GET /api/timeline/export
GET /api/timeline/stream
```

Фильтры range/severity/source/resource/actor/incident/type/text. Frontend: vertical/table/live/group/correlation modes, deep links, add-to-incident, CSV/JSON.

## 15. Plugin SDK

Плагин имеет signed manifest/package, isolated process/user/storage/lifecycle, explicit permissions, health, audit, update/rollback и event subscriptions. Первые official plugins: WireGuard, Tailscale, PostgreSQL, Nginx, extended Docker, Hermes Agent, Nexo Agent и VibeIDE.

Nexo Agent (`https://github.com/kantaevsherhan/nexo-agent`) и VibeIDE (`https://github.com/kantaevsherhan/vibe-ide`) отмечаются «От автора Mini Ubuntu Server Panel».

```text
plugin-package/
├── plugin.json
├── checksums.txt
├── signature.sig
├── backend/plugin-binary
├── frontend/remoteEntry.js + assets
├── migrations/
├── permissions.json
├── locales/{ru,en}.json
└── README.md
```

Manifest schema v1 фиксирует verified publisher/panelAuthor, compatibility, `http-unix` backend, frontend entrypoint/navigation, permissions и declared events.

Plugin process: отдельный user/transient unit, `NoNewPrivileges`, restricted FS/resources/restart, no panel SQLite/secrets/Docker socket/root-helper, только Unix socket + health. Core проксирует `/api/plugins/:pluginId/*`.

Permissions granular для system/metrics/processes/services/docker/files/network/firewall/notifications/audit/events/secrets и domain-specific операций. Они показываются до install, хранятся в core metadata и проверяются каждый request.

Root flow: Plugin → Plugin API → permission/RBAC/schema/scope/rate checks → Core service → exact helper → audit. Прямой helper запрещён.

Frontend SDK предоставляет core theme tokens, Aura/Lara, RU/EN, единственную копию PrimeVue, current user/RBAC, Toast/ConfirmDialog, navigation/API/events/audit и shared states.

Каждый plugin получает `/var/lib/mini-ubuntu-server/plugins/<id>/plugin.db`; core хранит только plugin metadata/manifest/permission grants.

Install: bounded archive, safe extract, structure/checksum/signature/publisher/compatibility checks, permission confirmation, backup/temp install, migrations, process health, frontend activation и audit; rollback при любой ошибке. Update показывает release notes/permission diff, auto-update off. Uninstall по умолчанию сохраняет config/DB/backups.

Event delivery только declared: durable retry/timeout/rate-limit/dead-letter/health. Secrets выдаются как `secret://plugins/...`, root-owned, stdin-updated/masked, не в SQLite/API и только plugin process.

Marketplace v1: local `.tar.gz`, official signed plugins, manual updates. Registry/categories/search/ratings/verified publishers/count/screenshots/changelog/compatibility — позже.

## 16. Интеграция

```text
Collectors / Docker / systemd / network / disk
        ↓
Internal Event Bus → Resource Timeline → Goals evaluator → Incident Mode
        ↓
Notifications → Public Status Page → Plugin event subscribers
```

Общие типы: `ResourceRef{Type,ID,Name}`, `ActorRef`, severity `info|warning|critical`, immutable SystemEvent.

## 17. RBAC

| Модуль | Viewer | Operator | Admin |
|---|---|---|---|
| Global Search / Resource Map | read | read | read |
| Server Explorer | read | scan/preview | cleanup |
| Public Status | read | incidents | full config |
| Network Analyzer | masked | read | export/extended |
| Disk Health | read | read | self-test |
| Sandbox | none/limited | create | policy |
| Workspace | own | own/shared | all |
| SSH Manager | none | approved hosts | hosts/keys/policy |
| Incidents | read | manage | policy/delete |
| Goals | read | mute | CRUD |
| Maintenance | read | approved start | CRUD |
| Timeline | read | export | retention |
| Plugins | use | use | install/update/remove |

## 18. Очерёдность реализации

1. **Инфраструктура:** Event Bus, durable outbox, Timeline, ResourceRef, jobs framework, retention, permissions, e2e infrastructure.
2. **Наблюдаемость:** Disk Health, Network Analyzer, Server Explorer, Live Resource Map, Global Search.
3. **Состояние:** Goals, Maintenance, Incident Mode, Public Status Page.
4. **Инструменты:** Workspace, SSH Manager, Sandbox.
5. **Plugin SDK:** manifest, registry, supervisor, permission gateway, frontend SDK, events, installer/update/rollback, signing, official plugins.

## 19. Definition of Done каждого модуля

- backend RBAC;
- опасные действия только exact allowlisted root-helper;
- audit;
- API validation/rate limits;
- restart-safe jobs;
- loading/empty/error frontend;
- RU/EN и dark/light Aura/Lara;
- unit/integration + desktop/mobile e2e;
- documented security model;
- tested update/rollback;
- no high/critical reachable vulnerabilities.

## 20. Не входит в первую версию расширения

- AI assistant;
- packet capture;
- arbitrary shell plugins;
- Kubernetes/multi-node orchestration;
- automatic root fixes;
- unsigned third-party marketplace;
- privileged Docker templates;
- SSH ProxyCommand;
- browser-stored private keys;
- direct plugin Docker socket;
- plugin auto-update без подтверждения.

## 21. Главный принцип

Панель развивается в локальную Ubuntu Server management platform: monitoring, безопасные admin actions, Timeline, Goals/Incidents/Maintenance, Explorer, Network/Disk, Workspaces, SSH, Sandbox, Public Status и Plugin SDK.

Каждый core module/plugin получает минимальные permissions; любое опасное действие проходит server validation, RBAC, exact allowlist и audit.
