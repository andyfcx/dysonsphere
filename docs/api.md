# API Reference

Base URL: `http://localhost:8000`

All protected `/api/v1/*` endpoints require the header:
```
Authorization: Bearer <token>
```

There are two token types:
- Enrollment token: configured with `SERVER_ENROLLMENT_TOKEN`
- Agent token: issued by `POST /api/v1/agents/enroll`
- Dashboard session token: returned by `POST /api/v1/auth/login`

Agent endpoints that act on behalf of a specific host also require:
```
X-Host-ID: <host-uuid>
```

---

## Dashboard Auth

### POST /api/v1/auth/login

Login with the dashboard username/password configured on the server.

**Request:**
```json
{
  "username": "admin",
  "password": "change-me"
}
```

**Response 200:**
```json
{
  "token": "session-token",
  "username": "admin"
}
```

### POST /api/v1/auth/logout

Invalidates the current dashboard session token.

**Response 200:**
```json
{ "ok": true }
```

---

## Agent Endpoints

### POST /api/v1/agents/enroll

Bootstrap a new agent using the one-time enrollment token.

### POST /api/v1/agents/register

Register a new host (or re-register to update metadata).

**Request:**
```json
{
  "machine_id": "abc-unique-id",
  "hostname": "web-prod-01",
  "ip_address": "10.0.1.10",
  "environment": "production",
  "tags": ["web", "nginx"],
  "agent_version": "0.1.0"
}
```

**Response 201:**
```json
{
  "id": "uuid",
  "machine_id": "abc-unique-id",
  "hostname": "web-prod-01",
  "status": "active",
  "registered_at": "2025-01-01T00:00:00Z"
}
```

---

### POST /api/v1/agents/heartbeat

Update last heartbeat timestamp. Header `X-Host-ID` required.

**Request:**
```json
{ "ip_address": "10.0.1.10" }
```

**Response 200:**
```json
{ "ok": true }
```

---

## Job Discovery

### POST /api/v1/jobs/discovery

Submit discovered cron jobs for a host. Header `X-Host-ID` required.

**Request:**
```json
{
  "jobs": [
    {
      "source_type": "cron",
      "schedule": "0 2 * * *",
      "timezone": "UTC",
      "user": "root",
      "raw_command": "/usr/local/bin/backup.sh --full > /dev/null 2>&1",
      "normalized_command": "/usr/local/bin/backup.sh --full",
      "command_hash": "a1b2c3d4",
      "enabled": true,
      "source_file": "/etc/cron.d/backup"
    }
  ]
}
```

**Response 200:**
```json
{ "upserted": 1, "jobs": [...] }
```

---

## Executions

### POST /api/v1/executions/batch

Report observed execution events. Header `X-Host-ID` required.

**Note:** Status is inferred from external observation. Use `confidence_score` (0.0–1.0)
and `evidence` to document the basis for the inferred status.

**Request:**
```json
{
  "executions": [
    {
      "job_id": "optional-job-uuid",
      "detected_started_at": "2025-01-01T02:00:05Z",
      "detected_finished_at": "2025-01-01T02:00:45Z",
      "duration_seconds": 40.0,
      "status": "success",
      "confidence_score": 0.8,
      "detection_sources": ["process", "journal"],
      "evidence": {
        "pid": 12345,
        "exit_code": null,
        "note": "inferred from process lifecycle"
      }
    }
  ]
}
```

**Response 200:**
```json
{ "accepted": 1 }
```

---

### GET /api/v1/executions

Returns paginated execution history.

**Query params:** `limit` (default 50, max 500), `offset`

---

## Metrics

### POST /api/v1/metrics/batch

Submit probe results. Header `X-Host-ID` required.

**Request:**
```json
{
  "metrics": [
    {
      "metric_name": "daily_records_inserted",
      "metric_type": "count",
      "dimensions": {"table": "events", "db": "app_prod"},
      "value": 4821,
      "measured_at": "2025-01-01T00:00:00Z",
      "status": "ok",
      "evidence": {"query": "SELECT COUNT(*) FROM events ..."}
    }
  ]
}
```

**Response 200:**
```json
{ "accepted": 1 }
```

---

### GET /api/v1/metrics

Returns paginated metrics ordered by `measured_at` desc.

---

## Read Endpoints

### GET /api/v1/hosts
Returns all registered hosts.

### GET /api/v1/jobs
Returns all discovered jobs.

### GET /api/v1/jobs/{id}
Returns job detail + recent executions.
```json
{
  "job": { ... },
  "executions": [ ... ]
}
```

### GET /api/v1/alerts
Returns all alerts (active + resolved), newest first.

### GET /api/v1/stats
Returns dashboard overview stats.
```json
{
  "total_hosts": 3,
  "active_hosts": 2,
  "total_jobs": 12,
  "active_alerts": 1,
  "recent_failed": 3
}
```

### GET /health
Health check, no auth required.
```json
{ "status": "ok" }
```
