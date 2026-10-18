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
- GORM с параметризованными runtime queries; raw SQL разрешён только в versioned migrations;
- аудит входов и административных операций;
- TOTP 2FA (RFC 6238, SHA-1/6 цифр/30 с, окно ±1 шаг): секрет хранится в SQLite зашифрованным AES-256-GCM ключом, производным от JWT secret; повтор кода блокируется сохранением последнего шага; пароль выдаёт только 5-минутный challenge-токен, подписанный отдельным ключом и непригодный как access token; на один challenge не больше 5 попыток и отдельный rate limit `/auth/login/mfa`; 10 одноразовых кодов восстановления хранятся как SHA-256; отключение 2FA и перевыпуск кодов требуют пароль и второй фактор; включение завершает остальные сессии, сброс администратором — все сессии пользователя. Смена JWT secret делает сохранённые TOTP-секреты нечитаемыми — перед ротацией 2FA нужно сбросить;
- apt-операции доступны только admin (просмотр — admin/operator) через exact `privileged-apt`: allowlist операций `status`/`refresh`/`upgrade`, валидация имён пакетов по Debian policy и повторная root-side проверка, что каждый выбранный пакет действительно обновляем; apt запускается без shell в transient unit `mini-ubuntu-server-apt` через `systemd-run`, все запуски пишутся в аудит;
- `sessionStorage` вместо долговременного хранения JWT;
- `bun audit`, `govulncheck`, CodeQL, Go checks и CI;
- systemd hardening и отдельный системный пользователь;
- Docker socket недоступен по умолчанию; opt-in `--enable-docker` документирован как root-equivalent доступ, container IDs/actions проверяются allowlist, remove не использует force и не удаляет volumes.
- UFW changes проходят двойную allowlist-валидацию, не используют shell, доступны только admin и записываются в аудит; deny порта 22 и enable/disable через web запрещены.
- Journald доступен только admin/operator через allowlist query и exact root-helper; объём ответа и размер сообщения ограничены, дополнительные journal fields отбрасываются.
- File operations привязаны к root-owned `allowed_directories`, повторно проверяются root-helper, запрещают absolute/parent/symlink traversal, ограничивают объём и используют атомарную запись; file content не попадает в аудит.
- WebSocket terminal требует admin/operator RBAC и одноразовый ticket в subprotocol header; ticket имеет 30-секундный TTL, SHA-256 in-memory storage, IP/web-session binding и single-use semantics. Upgrade требует exact same-origin; active user, role и web-session revoke/expiry повторно проверяются каждые 30 секунд. Message size/rate, terminal geometry, session duration и concurrent sessions ограничены. PTY непривилегированный, команды и ввод не журналируются.
- Update check доступен только admin, использует фиксированный HTTPS GitHub API endpoint, 10-секундный timeout, response limit и allowlist version/release URL. Browser не может передать backend произвольный download URL или запустить замену binary через read-only endpoint.
- Root CLI updater блокирует параллельный запуск, принимает только strict release version, проверяет SHA-256 до extraction, не распаковывает произвольные tar paths, атомарно меняет binary и восстанавливает согласованный stopped-service SQLite/WAL/SHM snapshot при failed start/health. Uninstall ограничен фиксированными путями и default-No confirmations.

## Обязательные меры для production

1. Завершить TLS termination через Caddy/nginx или встроенный TLS.
2. Перейти на HttpOnly Secure SameSite cookie-сессии, CSRF token и refresh rotation.
3. Добавить TOTP/WebAuthn для admin.
4. Реализовать distributed account lockout с временным окном и безопасной очисткой audit flood.
5. Ограничить trusted reverse proxies и не доверять входящим forwarding headers по умолчанию.
6. Подписывать релизы и публиковать SBOM.
7. Добавить secret scanning и fuzz tests к существующим dependency/CodeQL scans.
8. Провести независимый penetration test до размещения панели в публичной сети.

Ни одна web-панель не может обещать абсолютную защиту. Не публикуйте текущую development-версию напрямую в интернет.

Результат внутреннего review и обязательная production boundary описаны в [security-review.md](security-review.md) и [production.md](production.md).
