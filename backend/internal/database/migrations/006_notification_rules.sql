CREATE TABLE IF NOT EXISTS notification_rule_recipients (
 event_key TEXT NOT NULL REFERENCES notification_rules(event_key) ON DELETE CASCADE,
 recipient_id INTEGER NOT NULL REFERENCES telegram_recipients(id) ON DELETE CASCADE,
 PRIMARY KEY(event_key, recipient_id)
);

CREATE TABLE IF NOT EXISTS notification_rule_states (
 event_key TEXT PRIMARY KEY REFERENCES notification_rules(event_key) ON DELETE CASCADE,
 active INTEGER NOT NULL DEFAULT 0,
 active_dedup_key TEXT,
 last_event_id INTEGER,
 last_triggered_at DATETIME,
 last_notified_at DATETIME,
 resolved_at DATETIME
);

INSERT OR IGNORE INTO notification_rules(event_key, enabled, severity, cooldown_seconds, repeat_interval_seconds, send_recovery, updated_at) VALUES
 ('resource.cpu.high', 1, 'critical', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('resource.memory.high', 1, 'critical', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('resource.swap.high', 1, 'warning', 900, 3600, 1, CURRENT_TIMESTAMP),
 ('resource.disk.full', 1, 'critical', 900, 3600, 1, CURRENT_TIMESTAMP),
 ('resource.temperature.high', 1, 'critical', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('resource.disk_io.high', 1, 'warning', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('resource.network.high', 1, 'warning', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('docker.container.stopped', 1, 'error', 300, 1800, 1, CURRENT_TIMESTAMP),
 ('docker.container.restarted', 1, 'warning', 300, 1800, 0, CURRENT_TIMESTAMP),
 ('docker.container.unhealthy', 1, 'critical', 300, 900, 1, CURRENT_TIMESTAMP),
 ('docker.compose.error', 1, 'error', 300, 1800, 1, CURRENT_TIMESTAMP),
 ('docker.image.pull_failed', 1, 'error', 600, 3600, 0, CURRENT_TIMESTAMP),
 ('docker.storage.low', 1, 'critical', 900, 3600, 1, CURRENT_TIMESTAMP),
 ('systemd.service.stopped', 1, 'error', 300, 1800, 1, CURRENT_TIMESTAMP),
 ('systemd.service.failed', 1, 'critical', 300, 900, 1, CURRENT_TIMESTAMP),
 ('systemd.service.restarted', 1, 'warning', 300, 1800, 0, CURRENT_TIMESTAMP),
 ('systemd.service.disabled', 1, 'warning', 600, 3600, 1, CURRENT_TIMESTAMP),
 ('security.admin_login', 1, 'info', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.login_failures', 1, 'critical', 900, 3600, 1, CURRENT_TIMESTAMP),
 ('security.ssh_login', 1, 'info', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.ubuntu_user_created', 1, 'warning', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.sudo_granted', 1, 'critical', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.firewall_changed', 1, 'critical', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.public_port_opened', 1, 'critical', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.ssh_port_closed', 1, 'critical', 0, 0, 0, CURRENT_TIMESTAMP),
 ('security.ssh_key_changed', 1, 'warning', 0, 0, 0, CURRENT_TIMESTAMP),
 ('system.started', 1, 'info', 0, 0, 0, CURRENT_TIMESTAMP),
 ('system.rebooting', 1, 'warning', 0, 0, 0, CURRENT_TIMESTAMP),
 ('system.shutting_down', 1, 'warning', 0, 0, 0, CURRENT_TIMESTAMP),
 ('panel.updated', 1, 'info', 0, 0, 0, CURRENT_TIMESTAMP),
 ('backup.failed', 1, 'critical', 600, 3600, 1, CURRENT_TIMESTAMP),
 ('sqlite.corrupt', 1, 'critical', 600, 1800, 1, CURRENT_TIMESTAMP),
 ('system.storage.low', 1, 'critical', 900, 3600, 1, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO notification_rule_states(event_key)
SELECT event_key FROM notification_rules;
