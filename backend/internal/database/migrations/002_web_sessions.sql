CREATE TABLE IF NOT EXISTS web_sessions (
 id TEXT PRIMARY KEY,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 ip_address TEXT NOT NULL,
 user_agent TEXT NOT NULL,
 created_at DATETIME NOT NULL,
 last_seen_at DATETIME NOT NULL,
 expires_at DATETIME NOT NULL,
 revoked_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_web_sessions_user ON web_sessions(user_id, expires_at);
