# Frontend

## Запуск

```bash
cd frontend
bun install
bun run dev
```

Vite запускает приложение на `http://localhost:5173` и проксирует `/api` на backend `http://127.0.0.1:8080`.

## UI

Приложение использует PrimeVue 4. Нативные аналоги готовых компонентов не создаются. Верхняя панель — `Menubar`, desktop navigation — `PanelMenu` внутри `Splitter`, mobile navigation — `Drawer` с `PanelMenu`.

Требования к дизайну находятся в [frontend/DESIGN.md](../frontend/DESIGN.md). Темы Aura/Lara, dark/light и accent color переключаются штатным API `@primeuix/themes`.

Все surfaces, semantic states и accent scales берутся из PrimeVue theme tokens. Accent selector переключает ссылки на primitive palettes `{emerald.*}`, `{blue.*}` и `{violet.*}` без отдельной hex-палитры приложения.

## Локализация

Поддерживаются только `ru` и `en`. Настройки языка хранятся в `localStorage`. Даты форматируются централизованно через `src/services/dateTime.ts`:

- RU: `DD.MM.YYYY HH:mm`;
- EN: `MM/DD/YYYY h:mm A`.

## Авторизация

JWT временно хранится в `sessionStorage`, а не в `localStorage`. Это уменьшает длительность хранения, но окончательная рекомендуемая схема — короткая HttpOnly Secure SameSite cookie-сессия с серверной ротацией refresh token.

Login response также содержит username/role. Sidebar, mobile Drawer, routes, settings tabs и admin-only row actions фильтруются по роли; backend остаётся обязательной границей RBAC и повторно проверяет каждый endpoint.

## Проверки

```bash
bun run format
bun run check
bun audit
```

`check` запускает Prettier check, ESLint без warnings, TypeScript и production build. Маршруты загружаются лениво для уменьшения начального bundle.

Dashboard загружает ECharts модульно (`echarts/core`) и показывает CPU/RAM за день, неделю, месяц или всё время. Диапазон выбирается PrimeVue `SelectButton`, а даты оси форматируются Moment.js согласно выбранному языку.

Страница процессов использует PrimeVue `DataTable`, `Column`, `InputText`, `Tag`, `Button` и глобальный `ConfirmDialog`. Таблица виртуализирована, даты запуска форматируются общим Moment.js formatter, а действия управления скрыты для viewer.
