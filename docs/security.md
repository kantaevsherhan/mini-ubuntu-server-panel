# Безопасность

## Модель угроз

Основные риски панели: подбор пароля, кража web-сессии, XSS, CSRF при cookie-auth, SSRF через Telegram endpoint, command injection, path traversal, чрезмерные sudo-права, утечка секретов, вредоносные WebSocket сообщения и supply-chain зависимости.

## Реализовано

- bcrypt cost 12 для паролей панели;
- JWT HS256 с обязательным секретом не короче 32 символов и сроком 8 часов;
- проверка `is_active` и актуальной роли из SQLite на каждом запросе;
- глобальный API rate limit и отдельный login limit;
- лимит тела запроса, read/write/idle timeouts;
- security headers, CSP, запрет framing и ограниченный Permissions Policy;
- generic API errors без внутренних деталей;
- username/role/password validation;
- Telegram URL validation и SSRF-защита для private/link-local адресов;
- параметризованный SQL;
- аудит входов и административных операций;
- `sessionStorage` вместо долговременного хранения JWT;
- `bun audit`, Go checks и CI;
- systemd hardening и отдельный системный пользователь.
- Docker socket недоступен по умолчанию; opt-in `--enable-docker` документирован как root-equivalent доступ, container IDs/actions проверяются allowlist, remove не использует force и не удаляет volumes.
- UFW changes проходят двойную allowlist-валидацию, не используют shell, доступны только admin и записываются в аудит; deny порта 22 и enable/disable через web запрещены.

## Обязательные меры для production

1. Завершить TLS termination через Caddy/nginx или встроенный TLS.
2. Перейти на HttpOnly Secure SameSite cookie-сессии, CSRF token и refresh rotation.
3. Хранить session identifiers в SQLite и добавить revoke/logout-all.
4. Добавить TOTP/WebAuthn для admin.
5. Реализовать account lockout с временным окном и безопасной очисткой audit flood.
6. Ограничить trusted reverse proxies и не доверять входящим forwarding headers по умолчанию.
7. Добавить WebSocket origin check, message size/rate limits и per-operation RBAC.
8. Подписывать релизы и публиковать SBOM.
9. Выполнять dependency scanning, CodeQL, secret scanning и fuzz tests.
10. Провести независимый security review до размещения панели в публичной сети.

Ни одна web-панель не может обещать абсолютную защиту. Не публикуйте текущую development-версию напрямую в интернет.
