# Architecture

## Overview

Observer is a distributed cron monitoring system built around an **external observation** model:
agents install on monitored hosts without modifying any existing cronjobs. They observe
process activity, cron configuration files, and system logs, then report findings to a
central server.

```
┌─────────────────────────────────────────────────────┐
│                     Dashboard (React)               │
│                   http://localhost:5173             │
└─────────────────────┬───────────────────────────────┘
                      │ REST API
┌─────────────────────▼───────────────────────────────┐
│                Central Server (Go)                  │
│                http://localhost:8000                │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │
│  │  API     │  │ Service  │  │ Alert Evaluator   │ │
│  │ Handlers │→ │  Layer   │→ │ (runs every 1min) │ │
│  └──────────┘  └────┬─────┘  └───────────────────┘ │
│                     │                               │
│              ┌──────▼──────┐                        │
│              │ Repository  │                        │
│              │  (pgx/v5)   │                        │
│              └──────┬──────┘                        │
└─────────────────────┼───────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────┐
│                  PostgreSQL                         │
└─────────────────────────────────────────────────────┘
                      ▲
     push via HTTP    │
┌────────────────┐    │    ┌────────────────┐
│  Agent Host A  │────┘    │  Agent Host B  │
│                │         │                │
│ ┌────────────┐ │         │ ┌────────────┐ │
│ │ Cron Scan  │ │         │ │ Cron Scan  │ │
│ │ Proc Obs   │ │         │ │ Proc Obs   │ │
│ │ Journal Obs│ │         │ │ Probe Run  │ │
│ │ Probe Run  │ │         │ └────────────┘ │
│ └────────────┘ │         └────────────────┘
│  bbolt state   │
└────────────────┘
```

## Data Flow

1. **Agent init**: Agent registers with server → receives host UUID → writes config + local state
2. **Heartbeat loop**: Agent pings server every 30s → server updates `last_heartbeat_at`
3. **Cron discovery**: Agent scans `/etc/crontab`, `/etc/cron.d/*`, user crontabs every 5min → upserts job records on server
4. **Process scan**: Agent scans running processes every 30s → matches against known job commands → buffers inferred execution events
5. **Probe run**: Agent runs configured probes (SQL, file, command) → buffers metric results
6. **Report flush**: Every 1min, agent flushes buffered events/metrics to server via batch API
7. **Alert evaluation**: Server runs alert rules every 1min → creates/resolves alert records
8. **Dashboard**: React SPA polls server API every 30s to display current state

## Why External Observation?

The system intentionally does not wrap cronjobs in execution hooks. This means:

- **No modifications** to existing cron configurations
- **Lower risk** — agent failure doesn't affect production jobs
- **Easier adoption** — just install agent, run init

**Trade-off**: Without instrumentation, exact exit codes are unavailable. The system uses
`confidence_score` (0.0–1.0) and `evidence_json` to be transparent about inference quality.

## Database Schema

```
hosts                  current_host_states
  id ──────────────┐     host_id → hosts.id
  machine_id       │
  hostname         │   jobs (discovered)
  ...              └──→  id
                          host_id → hosts.id
                          command_hash (UNIQUE per host)
                          schedule, timezone, user, ...

                       current_job_states
                          job_id → jobs.id
                          last_status, last_run_at, ...

                       executions (events)
                          id
                          host_id, job_id
                          status, confidence_score
                          detection_sources, evidence

                       data_metrics (probe results)
                          id
                          host_id, metric_name
                          value, measured_at, status

                       alerts
                          id
                          target_type, target_id
                          rule_name, severity, status
```

## Package Structure

```
apps/server/
  cmd/server/       main.go — entry point, wiring
  internal/
    api/            HTTP handlers + router + request/response types
    alerting/       alert rule evaluation
    config/         env-based configuration
    db/             pgx pool + golang-migrate wrapper
    domain/         canonical domain types (no DB deps)
    repository/     pgx-based persistence layer
    service/        business logic, orchestrates repositories
  migrations/       SQL migration files (golang-migrate format)

apps/agent/
  cmd/agent/        main.go — cobra CLI (init, run)
  internal/
    app/            main loop, orchestrates all subsystems
    client/         HTTP client for central server
    config/         YAML config loader
    discovery/      cron file scanner + command normalizer
    observer/       process scanner + journal reader
    probes/         SQL/file/command probe runner
    reporter/       batch upload + bbolt buffering
    state/          bbolt local persistence
```
