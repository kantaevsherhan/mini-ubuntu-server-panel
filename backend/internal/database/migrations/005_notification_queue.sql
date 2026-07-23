ALTER TABLE notification_events ADD COLUMN updated_at DATETIME;
ALTER TABLE notification_events ADD COLUMN resolved_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_notification_events_status ON notification_events(status, created_at);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_due ON notification_deliveries(status, next_attempt_at);

CREATE TABLE IF NOT EXISTS notification_rules (
 event_key TEXT PRIMARY KEY,
 enabled INTEGER NOT NULL DEFAULT 1,
 severity TEXT NOT NULL DEFAULT 'warning',
 cooldown_seconds INTEGER NOT NULL DEFAULT 600,
 repeat_interval_seconds INTEGER NOT NULL DEFAULT 1800,
 send_recovery INTEGER NOT NULL DEFAULT 1,
 updated_at DATETIME NOT NULL
);
