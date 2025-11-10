-- +goose Up
-- +goose StatementBegin

-- events stores all security events processed by Sentinel.
--
-- Design decisions:
--   - event_id is the primary key (set by the agent, used for dedup)
--   - event_type and severity are text, not enums, for flexibility
--   - raw_data stores the full Protobuf-encoded event as binary (compact)
--   - created_at tracks when the processor ingested the event
--   - indexes on timestamp, hostname, source_ip for forensics queries
CREATE TABLE IF NOT EXISTS events (
    event_id       TEXT PRIMARY KEY,
    event_type     TEXT NOT NULL,
    severity       TEXT NOT NULL DEFAULT 'SEVERITY_UNSPECIFIED',
    hostname       TEXT NOT NULL DEFAULT '',
    source_ip      TEXT NOT NULL DEFAULT '',
    destination_ip TEXT NOT NULL DEFAULT '',
    service        TEXT NOT NULL DEFAULT '',
    timestamp      TIMESTAMPTZ NOT NULL,
    raw_data       BYTEA,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for time-range queries (forensics: "show events from last 24h").
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events (timestamp);

-- Index for filtering by source (forensics: "all events from this IP").
CREATE INDEX IF NOT EXISTS idx_events_source_ip ON events (source_ip)
    WHERE source_ip != '';

-- Index for filtering by host (forensics: "all events from this server").
CREATE INDEX IF NOT EXISTS idx_events_hostname ON events (hostname)
    WHERE hostname != '';

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS events;
