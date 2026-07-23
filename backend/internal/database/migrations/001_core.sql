CREATE TABLE IF NOT EXISTS users (
 id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE,
 display_name TEXT NOT NULL DEFAULT '', password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
 is_active INTEGER NOT NULL DEFAULT 1, must_change_password INTEGER NOT NULL DEFAULT 0,
 system_username TEXT, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
 last_login_at DATETIME
);
CREATE TABLE IF NOT EXISTS audit_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, actor_user_id INTEGER, action TEXT NOT NULL,
 target_type TEXT NOT NULL, target_id TEXT, details_json TEXT NOT NULL DEFAULT '{}',
 ip_address TEXT, created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS telegram_settings (
 id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL DEFAULT 0,
 api_base_url TEXT NOT NULL DEFAULT 'https://api.telegram.org',
 request_timeout_seconds INTEGER NOT NULL DEFAULT 10, retry_count INTEGER NOT NULL DEFAULT 3,
 updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS telegram_recipients (
 id INTEGER PRIMARY KEY AUTOINCREMENT, telegram_user_id INTEGER,
 telegram_chat_id INTEGER NOT NULL, display_name TEXT, enabled INTEGER NOT NULL DEFAULT 1,
 receive_alerts INTEGER NOT NULL DEFAULT 1, receive_audit INTEGER NOT NULL DEFAULT 0,
 receive_updates INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS notification_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, event_key TEXT NOT NULL, severity TEXT NOT NULL,
 payload_json TEXT NOT NULL, dedup_key TEXT, status TEXT NOT NULL DEFAULT 'pending', created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_event_dedup ON notification_events(dedup_key) WHERE dedup_key IS NOT NULL;
CREATE TABLE IF NOT EXISTS notification_deliveries (
 id INTEGER PRIMARY KEY AUTOINCREMENT, event_id INTEGER NOT NULL REFERENCES notification_events(id),
 recipient_id INTEGER NOT NULL REFERENCES telegram_recipients(id), status TEXT NOT NULL DEFAULT 'pending',
 attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, next_attempt_at DATETIME, delivered_at DATETIME,
 created_at DATETIME NOT NULL
);
INSERT OR IGNORE INTO telegram_settings(id, updated_at) VALUES(1, CURRENT_TIMESTAMP);
