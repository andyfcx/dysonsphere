-- Migration 001: initial schema for observer system

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── hosts ─────────────────────────────────────────────────────────────────────

CREATE TABLE hosts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id      TEXT NOT NULL UNIQUE,
    hostname        TEXT NOT NULL,
    ip_address      TEXT NOT NULL DEFAULT '',
    environment     TEXT NOT NULL DEFAULT 'unknown',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (status IN ('active','stale','offline','unknown')),
    agent_version   TEXT NOT NULL DEFAULT '',
    metadata        JSONB
);

CREATE INDEX idx_hosts_machine_id ON hosts(machine_id);
CREATE INDEX idx_hosts_status ON hosts(status);

-- ── jobs ──────────────────────────────────────────────────────────────────────

CREATE TABLE jobs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id             UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    source_type         TEXT NOT NULL DEFAULT 'discovered'
                            CHECK (source_type IN ('cron','systemd_timer','manual','discovered')),
    schedule            TEXT NOT NULL DEFAULT '',
    timezone            TEXT NOT NULL DEFAULT 'UTC',
    "user"              TEXT NOT NULL DEFAULT '',
    raw_command         TEXT NOT NULL,
    normalized_command  TEXT NOT NULL DEFAULT '',
    command_hash        TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    source_file         TEXT NOT NULL DEFAULT '',
    metadata            JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(host_id, command_hash)
);

CREATE INDEX idx_jobs_host_id ON jobs(host_id);
CREATE INDEX idx_jobs_command_hash ON jobs(command_hash);

-- ── executions ────────────────────────────────────────────────────────────────

CREATE TABLE executions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id                 UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    job_id                  UUID REFERENCES jobs(id) ON DELETE SET NULL,
    scheduled_at            TIMESTAMPTZ,
    detected_started_at     TIMESTAMPTZ,
    detected_finished_at    TIMESTAMPTZ,
    duration_seconds        DOUBLE PRECISION,
    -- Status is inferred from external observation. See confidence_score.
    status                  TEXT NOT NULL DEFAULT 'unknown'
                                CHECK (status IN ('running','success','failed','unknown','partial','missed')),
    confidence_score        DOUBLE PRECISION NOT NULL DEFAULT 0.0
                                CHECK (confidence_score BETWEEN 0.0 AND 1.0),
    detection_sources       TEXT[] NOT NULL DEFAULT '{}',
    evidence                JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_executions_host_id ON executions(host_id);
CREATE INDEX idx_executions_job_id ON executions(job_id);
CREATE INDEX idx_executions_detected_started_at ON executions(detected_started_at);
CREATE INDEX idx_executions_status ON executions(status);

-- ── data_metrics ──────────────────────────────────────────────────────────────

CREATE TABLE data_metrics (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id         UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    metric_name     TEXT NOT NULL,
    metric_type     TEXT NOT NULL DEFAULT 'gauge'
                        CHECK (metric_type IN ('count','gauge','boolean','string')),
    dimensions      JSONB,
    value           DOUBLE PRECISION NOT NULL,
    measured_at     TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (status IN ('ok','warning','critical','unknown')),
    evidence        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_data_metrics_host_id ON data_metrics(host_id);
CREATE INDEX idx_data_metrics_metric_name ON data_metrics(metric_name);
CREATE INDEX idx_data_metrics_measured_at ON data_metrics(measured_at);

-- ── alerts ────────────────────────────────────────────────────────────────────

CREATE TABLE alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_type     TEXT NOT NULL CHECK (target_type IN ('host','job','metric')),
    target_id       UUID NOT NULL,
    rule_name       TEXT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'warning'
                        CHECK (severity IN ('info','warning','critical')),
    status          TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active','resolved')),
    message         TEXT NOT NULL DEFAULT '',
    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ
);

CREATE INDEX idx_alerts_target ON alerts(target_type, target_id);
CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_triggered_at ON alerts(triggered_at);

-- ── current_host_states ───────────────────────────────────────────────────────
-- Materialized current state for fast dashboard access.

CREATE TABLE current_host_states (
    host_id         UUID PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_heartbeat_at TIMESTAMPTZ,
    active_jobs     INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── current_job_states ────────────────────────────────────────────────────────

CREATE TABLE current_job_states (
    job_id              UUID PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    host_id             UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    last_status         TEXT NOT NULL DEFAULT 'unknown',
    last_run_at         TIMESTAMPTZ,
    next_expected_at    TIMESTAMPTZ,
    consecutive_fail    INT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_current_job_states_host_id ON current_job_states(host_id);

-- ── agent_commands ───────────────────────────────────────────────────────────

CREATE TABLE agent_commands (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id         UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    command_hash    TEXT NOT NULL,
    raw_command     TEXT NOT NULL,
    schedule        TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','dispatched','success','failed')),
    message         TEXT NOT NULL DEFAULT '',
    exit_code       INT,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at   TIMESTAMPTZ,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE INDEX idx_agent_commands_host_status ON agent_commands(host_id, status, requested_at);
