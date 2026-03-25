# Agent Guide

## Overview

The `observer-agent` is a Go binary that installs on monitored Linux hosts. It:

1. Scans cron configuration files to discover jobs
2. Monitors running processes to infer execution events
3. Reads journald/cron logs for additional signal
4. Runs configured data probes (SQL, file freshness, command)
5. Reports everything to the central server via HTTP

**Key design principle:** The agent never modifies existing cronjobs. All observation is external.

## Installation

### Binary

```bash
# Copy binary to host
scp bin/observer-agent user@host:/usr/local/bin/observer-agent
chmod +x /usr/local/bin/observer-agent
```

### Init (first time)

```bash
observer-agent init \
  --server http://your-server:8000 \
  --token your-secret-token \
  --env production \
  --tags crawler,weibo,tw
```

This will:
1. Read `/etc/machine-id` (or generate a UUID) as `machine_id`
2. Register this host with the central server
3. Write config to `/etc/observer-agent/config.yaml`
4. Write local state DB to `/var/lib/observer-agent/state.db`

### Run

```bash
observer-agent run --config /etc/observer-agent/config.yaml
```

If the config file is missing, `observer-agent run` will enter interactive setup,
register the host, and write the config/state paths you provide.

### Background mode

```bash
observer-agent run --daemon --config /etc/observer-agent/config.yaml
observer-agent daemon status --config /etc/observer-agent/config.yaml
observer-agent daemon stop --config /etc/observer-agent/config.yaml
```

### Local cron status

```bash
observer-agent status --config /etc/observer-agent/config.yaml --since 24h
```

This reads the local bbolt state and shows the latest status seen for each cron
command in the requested time window.

### Systemd service

```bash
# Install service file
cp infra/systemd/observer-agent.service /etc/systemd/system/

# Create dedicated user
useradd --system --no-create-home --shell /bin/false observer-agent
mkdir -p /var/lib/observer-agent
chown observer-agent:observer-agent /var/lib/observer-agent

# Enable and start
systemctl daemon-reload
systemctl enable --now observer-agent
systemctl status observer-agent
```

## Configuration

Config file: `/etc/observer-agent/config.yaml`

```yaml
server:
  url: "http://your-server:8000"
  token: "your-secret-token"

agent:
  machine_id: "auto-set-by-init"
  hostname: "your-hostname"
  environment: "production"
  tags: [crawler, weibo, tw]
  version: "0.1.0"
  state_file: "/var/lib/observer-agent/state.db"
  heartbeat_interval: "30s"
  discovery_interval: "5m"
  process_scan_interval: "30s"
  report_interval: "1m"

probes:
  - name: "daily_events"
    type: "sql_count"
    schedule: "0 * * * *"
    sql:
      dsn: "postgres://..."
      query: "SELECT COUNT(*) FROM events WHERE ..."
```

## Probe Types

### sql_count
Connects to a PostgreSQL database and runs a COUNT query.

### file_freshness
Checks whether a file exists and how old it is.

### command
Runs a shell command and parses the first line as a float.

## Cron Discovery

The agent scans:
- `/etc/crontab`
- `/etc/cron.d/*`
- `/var/spool/cron/crontabs/*` (requires root, graceful fallback)
- `/var/spool/cron/*` (RHEL/CentOS path)

Extra custom directories can be added via config (for testing).

## Process Observation

Every `process_scan_interval`, the agent:
1. Lists all running processes via `gopsutil`
2. Matches processes against known job normalized commands (substring match)
3. Buffers inferred execution events with `confidence_score` and `evidence`

**Limitation:** Process-based observation cannot determine exit codes.
Status is always `running` when first observed; finished state is inferred from
process disappearance. Confidence score reflects the quality of the match.

## Local State

Events are buffered in a `bbolt` embedded database at `state_file`.
If the server is unavailable, events accumulate locally and are flushed
on the next successful connection. This provides resilience for short outages.

## Observability Honesty

All execution statuses observed by the agent include:
- `status`: `running`, `success`, `failed`, `unknown`, `partial`, `missed`
- `confidence_score`: 0.0–1.0 (reflects inference confidence)
- `detection_sources`: which sources contributed (e.g., `["process"]`, `["journal"]`)
- `evidence`: raw data supporting the inference

A `confidence_score` of 1.0 means the status is certain (rare without instrumentation).
A score of 0.4–0.8 means inferred from process/log signals.
