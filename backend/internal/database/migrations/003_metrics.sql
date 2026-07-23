CREATE TABLE IF NOT EXISTS metric_samples (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 sampled_at DATETIME NOT NULL,
 cpu_percent REAL NOT NULL CHECK(cpu_percent >= 0 AND cpu_percent <= 100),
 memory_percent REAL NOT NULL CHECK(memory_percent >= 0 AND memory_percent <= 100),
 memory_used_bytes INTEGER NOT NULL,
 memory_total_bytes INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_metric_samples_time ON metric_samples(sampled_at);
