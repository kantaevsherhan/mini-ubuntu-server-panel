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

## Локализация

Поддерживаются только `ru` и `en`. Настройки языка хранятся в `localStorage`. Даты форматируются централизованно через `src/services/dateTime.ts`:

- RU: `DD.MM.YYYY HH:mm`;
- EN: `MM/DD/YYYY h:mm A`.

## Авторизация

JWT временно хранится в `sessionStorage`, а не в `localStorage`. Это уменьшает длительность хранения, но окончательная рекомендуемая схема — короткая HttpOnly Secure SameSite cookie-сессия с серверной ротацией refresh token.

## Проверки

```bash
bun run format
bun run check
bun audit
```

`check` запускает Prettier check, ESLint без warnings, TypeScript и production build. Маршруты загружаются лениво для уменьшения начального bundle.
