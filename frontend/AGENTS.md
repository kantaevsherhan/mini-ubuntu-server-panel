# Frontend rules

These rules apply to all files under `frontend/`.

Before changing any visual UI, read and follow `DESIGN.md`. It is the single source of truth for component choice, layout, spacing, themes, states, and responsive behavior.

## Stack and design

- Use Vue 3 Composition API with `<script setup lang="ts">`.
- Keep TypeScript strict. Avoid `any`; when unavoidable, isolate it at an API boundary and document why.
- Use PrimeVue 4 components and PrimeIcons (`pi pi-*`) for interface icons.
- Before building UI, check the PrimeVue component catalog. Use an existing PrimeVue component whenever it covers the requirement; do not recreate buttons, inputs, menus, navigation, cards, dialogs, drawers, tables, splitters, tabs, toasts, confirmations, selectors, uploaders, progress indicators, tags, badges, skeletons, or virtual scrolling.
- Custom components are allowed only for project-specific composition or behavior that PrimeVue does not provide. Compose PrimeVue primitives inside them.
- Use `@primeuix/themes` presets Aura and Lara through PrimeVue's theme API.
- Tailwind CSS is for layout and utilities; do not duplicate PrimeVue component styling unnecessarily.
- Support exactly two frontend locales: Russian (`ru`) and English (`en`). New user-facing text must be added to both locales.
- Format user-visible dates and times through the shared Moment.js service. Use `DD.MM.YYYY HH:mm` for Russian and `MM/DD/YYYY h:mm A` for English; never format dates ad hoc inside components.
- Preserve the dense desktop-first workspace from 1200px upward and provide a fully usable mobile layout from 320px upward with PrimeVue Drawer navigation.
- Persist user-only UI preferences in `localStorage`; never store secrets there.
- Large process, log, file, and container datasets must use virtual scrolling.

## Code organization

- Pages belong in `src/pages`, reusable UI in `src/components`, layouts in `src/layouts`.
- API access belongs in `src/services`; authentication and shared state belong in Pinia stores.
- Route access rules belong in `src/router`.
- Prefer lazy-loaded routes for large pages and heavy libraries such as Monaco, ECharts, and xterm.js.
- All destructive actions require PrimeVue ConfirmDialog and visible error/success feedback.

## Mandatory post-task commands

Run after every change:

```bash
bun run format
bun run format:check
bun run lint
bun run typecheck
bun run build
```

`bun run check` may replace the last four commands. Fix warnings as well as errors; ESLint uses `--max-warnings=0`.
