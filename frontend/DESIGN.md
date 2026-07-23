# Mini Ubuntu Server Panel — UI design system

This document is the single source of truth for frontend design. Read it before creating or changing any visual UI.

## Product direction

- Desktop-first Ubuntu server administration panel.
- Desktop workspace is optimized for 1200px and wider; mobile navigation and content must remain usable from 320px.
- Default appearance: dark, compact, technical, calm.
- Visual character: between VS Code and a modern infrastructure dashboard.
- Prefer information density, predictable placement, and fast scanning over decorative UI.

## PrimeVue-first rule

Always check PrimeVue 4 before writing UI markup. Use the library component when it exists.

| Requirement       | PrimeVue component                                             |
| ----------------- | -------------------------------------------------------------- |
| Top navigation    | `Menubar`                                                      |
| Side navigation   | `PanelMenu`                                                    |
| Resizable areas   | `Splitter`, `SplitterPanel`                                    |
| Content container | `Card`, `Panel`                                                |
| Text field        | `InputText`, `Textarea`                                        |
| Password          | `Password`                                                     |
| Number            | `InputNumber`                                                  |
| Selectors         | `Select`, `MultiSelect`, `DatePicker`                          |
| Form width/layout | `Fluid`, `FloatLabel`, `IftaLabel`                             |
| Action            | `Button`, `SplitButton`                                        |
| Data              | `DataTable`, `TreeTable`, `Tree`, `VirtualScroller`            |
| Overlay           | `Dialog`, `Drawer`, `Popover`, `ContextMenu`, `Menu`           |
| Feedback          | `Toast`, `Message`, `ConfirmDialog`, `ProgressBar`, `Skeleton` |
| Status            | `Tag`, `Badge`                                                 |
| Upload            | `FileUpload`                                                   |
| Sections          | `Tabs`, `Accordion`                                            |

Do not create native buttons, inputs, selects, textareas, menus, dialogs, cards, tabs, badges, or progress bars. Custom components may compose PrimeVue components for panel-specific behavior.

## Icons

- Use PrimeIcons only, with `pi pi-*` classes.
- Do not add another icon library or hand-written SVG icons.
- Pair unfamiliar icons with a text label or tooltip.
- Standard actions: add `pi-plus`, edit `pi-pencil`, delete `pi-trash`, save `pi-save`, refresh `pi-refresh`, settings `pi-cog`, security `pi-shield`, user `pi-user`, terminal `pi-desktop`.

## Themes and tokens

- Use `@primeuix/themes` with the Aura and Lara presets.
- Default preset: Aura. Default mode: dark. Default accent: emerald.
- Supported accents: emerald, blue, violet.
- Change themes through `usePreset` and `updatePrimaryPalette`; do not hardcode component colors.
- Use PrimeVue semantic CSS variables such as `--p-content-background`, `--p-content-border-color`, `--p-text-color`, and `--p-text-muted-color`.
- Tailwind is for layout, spacing, sizing, overflow, and typography utilities. It must not override PrimeVue control padding, borders, focus rings, or interaction states.

## Spacing and shape

- Base spacing unit: 4px.
- Page padding: 20px (`p-5`).
- Standard component gap: 12–16px.
- Form vertical gap: 24px for floating labels, 16px elsewhere.
- Card padding comes from PrimeVue. Do not wrap cards in duplicated custom padding unless the layout requires it.
- Border radius: small, typically 4–8px. Avoid pill-shaped containers except tags and status badges.
- Tables use `size="small"` and compact rows.

## Layout

- Header uses `Menubar` and remains visible.
- Workspace uses `Splitter`; sidebar size is saved in `localStorage`.
- Sidebar uses `PanelMenu` and PrimeIcons.
- Main workspace scrolls independently from navigation.
- Below the `lg` breakpoint, replace the persistent sidebar with PrimeVue `Drawer` and open it with an icon-only PrimeVue `Button`.
- On mobile, stack dashboard cards and form grids vertically, keep touch targets at least 40px high, and allow data tables to scroll horizontally.
- Terminal height and editor/terminal fullscreen state must be persisted when implemented.
- Heavy screens should lazy-load Monaco, xterm.js, and ECharts.

## Forms

- Wrap full-width forms in `Fluid`.
- Prefer `FloatLabel variant="on"` for login and compact settings forms.
- Every control must have an associated accessible label.
- Show validation next to the field with PrimeVue `Message`; use `Toast` for operation-level results.
- Primary submit action appears last. Destructive actions use danger severity and `ConfirmDialog`.
- Password fields use `Password` with toggle-mask. Never expose secrets after save; show masked values.
- Do not use placeholder text as the only label.

## Tables and large datasets

- Use `DataTable`/`TreeTable`, never hand-built HTML tables for application data.
- Processes, logs, files, containers, and audit lists require virtual scrolling.
- Keep the first identifying column visible where practical.
- Put row actions at the right edge using PrimeVue `Button`, `Menu`, or `ContextMenu`.
- Use `Skeleton` during initial loading and a clear empty state when no rows exist.
- Format status with semantic `Tag`: success, info, warn, danger, secondary.

## Dates, time, and language

- Supported frontend languages are Russian (`ru`) and English (`en`) only.
- Every user-visible string must exist in both locale dictionaries.
- Use the shared Moment.js service for all dates and times.
- Russian date/time format: `DD.MM.YYYY HH:mm`.
- English date/time format: `MM/DD/YYYY h:mm A`.
- Never call `Date.toLocaleString`, `Intl.DateTimeFormat`, or ad hoc formatting directly in components.
- Persist locale in `localStorage`; do not use it for backend audit timestamps, which remain UTC.

## States and feedback

- Loading: `Skeleton` for page content, component loading props for actions and tables.
- Success/error: `Toast`; persistent form errors: `Message`.
- Destructive confirmation: `ConfirmDialog`.
- Offline state must be visible in the header and must not look like success.
- Disable duplicate submissions while a request is running.
- Errors must be actionable and must never reveal backend internals.

## Motion and micro-interactions

- Use a minimal amount of motion only to explain hover, focus, selection, expansion, upload/file selection, status changes, and successful actions.
- Prefer PrimeVue's built-in transitions and short CSS transitions of 120–180ms for color, background, border, opacity, and small transforms.
- Do not animate large layout regions, charts continuously, or add decorative looping motion.
- Never delay an operation to play an animation.
- Respect `prefers-reduced-motion: reduce`; disable non-essential transitions and smooth movement for those users.

## Accessibility

- Keyboard navigation must work for all controls and menus.
- Preserve PrimeVue focus rings; never remove outlines without an equivalent.
- Maintain readable contrast in both dark and light modes.
- Icon-only buttons require `aria-label` and a tooltip.
- Do not communicate severity using color alone; include icon or text.
- Dialog focus must remain trapped and return to the trigger on close.

## Review checklist

Before considering a UI task complete:

1. Existing PrimeVue components were used instead of native replicas.
2. PrimeIcons are the only icons.
3. Aura and Lara both render correctly in dark and light modes.
4. Russian and English strings and date formats work.
5. Form controls have correct theme padding and accessible labels.
6. Loading, empty, error, success, and disabled states are covered.
7. Layout remains usable at 320px, 768px, and 1200px widths.
8. `bun run format` and `bun run check` pass.
