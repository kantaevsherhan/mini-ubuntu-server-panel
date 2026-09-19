-- Per-object alert state: one row per (event, container/unit/mountpoint) so that
-- two containers failing at once produce two alerts and two independent recoveries.
CREATE TABLE IF NOT EXISTS notification_subject_states (
 event_key TEXT NOT NULL,
 subject TEXT NOT NULL,
 active INTEGER NOT NULL DEFAULT 0,
 active_dedup_key TEXT,
 last_event_id INTEGER,
 last_triggered_at DATETIME,
 last_notified_at DATETIME,
 resolved_at DATETIME,
 PRIMARY KEY(event_key, subject)
);

CREATE TABLE IF NOT EXISTS monitor_settings (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 settings_json TEXT NOT NULL,
 updated_at DATETIME NOT NULL
);
