-- TOTP two-factor authentication for panel users. Secrets are sealed with AES-GCM
-- (key derived from the server secret); recovery codes are stored as SHA-256 hashes.
CREATE TABLE IF NOT EXISTS user_totp (
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 secret_sealed TEXT,
 pending_secret_sealed TEXT,
 pending_created_at DATETIME,
 enabled_at DATETIME,
 last_step INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_recovery_codes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 code_hash TEXT NOT NULL,
 used_at DATETIME,
 created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_recovery_codes_user ON user_recovery_codes(user_id);

INSERT OR IGNORE INTO notification_rules(event_key, enabled, severity, cooldown_seconds, repeat_interval_seconds, send_recovery, updated_at) VALUES
 ('security.mfa_disabled', 1, 'warning', 0, 0, 0, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO notification_rule_states(event_key) VALUES ('security.mfa_disabled');
