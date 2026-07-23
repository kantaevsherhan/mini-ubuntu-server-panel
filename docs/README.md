# Документация Mini Ubuntu Server Panel

Документация описывает текущее состояние проекта. Основной [README](../README.md) содержит быстрый старт и установку.

## Разделы

- [Архитектура](architecture.md) — компоненты, границы и поток данных;
- [Frontend](frontend.md) — Vue, PrimeVue, темы, локализация и запуск;
- [Backend](backend.md) — Fiber API, SQLite, auth и системные интеграции;
- [Безопасность](security.md) — модель угроз и реализованные меры;
- [Security review](security-review.md) — findings, evidence, accepted risks и review decision;
- [Production deployment](production.md) — TLS proxy, least privilege, backup и incident checklist;
- [Эксплуатация](operations.md) — сборка, установка, systemd, update и backup;
- [Состояние и TODO](todo.md) — готовые функции, план, рекомендации и известные проблемы.

## Быстрый запуск frontend

```bash
cd frontend
bun install
bun run dev
```

Откройте `http://localhost:5173/`. Для авторизации и данных должен работать backend на `127.0.0.1:8080`.

## Обязательная проверка изменений

```bash
cd frontend && bun run format && bun run check && bun audit
docker run --rm -u "$(id -u):$(id -g)" -e GOCACHE=/tmp/go-cache \
  -e GOMODCACHE=/tmp/go-mod-cache -v "$PWD:/workspace" \
  -w /workspace/backend golang:1.25 \
  sh -c 'gofmt -w . && go test ./... && go vet ./...'
bash -n scripts/*.sh
```
