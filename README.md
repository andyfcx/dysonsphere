# Observer — Distributed Cron & Data Monitoring System

A self-hosted monitoring system for observing scheduled tasks and data volume across multiple Linux hosts, without modifying any existing cronjobs.

## Architecture

```
React Dashboard ──→ Central Server (Go) ──→ PostgreSQL
                          ↑
                    Agents push via HTTP
                          ↑
          ┌───────────────┴───────────────┐
          │                               │
    Host A (agent)                  Host B (agent)
    - Cron scan                     - Cron scan
    - Process watch                 - Process watch
    - Probe runner                  - Probe runner
    - Local bbolt buffer            - Local bbolt buffer
```

**External observation model**: agents read cron configs and observe processes without touching
any existing cronjob. Status is inferred and reported with a `confidence_score`.

## Quick Start

### 1. Clone and copy env

```bash
git clone <repo>
cd dysonsphere
cp .env.example .env
```

### 2. Start via Docker Compose

```bash
docker compose up --build
```

This starts:
- `postgres` — PostgreSQL 16 on port 5432
- `server` — Go API server on http://localhost:8000
- `web` — React dashboard on http://localhost:5173

Dashboard login defaults to `admin` / `admin`. Set `SERVER_LOGIN_USERNAME` and
`SERVER_LOGIN_PASSWORD` in `.env` before production use.

### 2b. Start server host only

If you want the central host to run PostgreSQL, server, and dashboard web, but not seed, use:

```bash
docker compose -f docker-compose.server.yml up --build -d
```

This starts:
- `postgres`
- `server`
- `web`

It does not start `seed`.

### 3. Load sample data (optional)

```bash
docker compose --profile seed up seed
```

Then open http://localhost:5173 to see the dashboard with sample hosts, jobs, and metrics.

## Local Development

### Prerequisites
- Go 1.23+
- Node.js 20+
- Docker + docker-compose

### Start database only

```bash
make dev-db
# or: docker compose up postgres
```

### Run server locally

```bash
# Copy and edit .env
cp .env.example .env

# Apply migrations + start server
make dev-server
# or: cd apps/server && go run ./cmd/server
```

### Run frontend locally

```bash
cd apps/web
npm install
npm run dev
# Opens on http://localhost:5173
```

### Run agent (local test)

```bash
# First initialize against local server
make agent-init
# or:
cd apps/agent && go run ./cmd/agent init \
  --server http://localhost:8000

# Then run
cd apps/agent && go run ./cmd/agent run \
  --config /etc/observer-agent/config.yaml
```

If `/etc/observer-agent/config.yaml` does not exist yet, `observer-agent run`
will now start an interactive setup flow. It asks for the server URL and a
hidden enrollment token, exchanges that for a formal agent credential, then
writes the config and local credential file.

Background mode and local status:

```bash
observer-agent run --daemon --config /etc/observer-agent/config.yaml
observer-agent daemon status --config /etc/observer-agent/config.yaml
observer-agent status --config /etc/observer-agent/config.yaml --since 24h
```

Trigger immediate execution for selected jobs on a host:

```bash
curl -X POST http://localhost:8000/api/v1/hosts/<host-id>/commands/run \
  -H "Authorization: Bearer dev-token" \
  -H "Content-Type: application/json" \
  -d '{"job_ids":["<job-id-1>","<job-id-2>"]}'
```

Login to the dashboard API:

```bash
curl -X POST http://localhost:8000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me"}'
```

## Database Migrations

Migrations run automatically on server startup. To run manually:

```bash
make migrate-up    # apply all pending
make migrate-down  # rollback last
```

Migration files are in `apps/server/migrations/`.

## Testing

```bash
make test           # run all Go tests
make test-verbose   # with output
```

Tests cover:
- Cron parser / command normalizer
- Process matching logic
- Probe runner (file freshness, command)
- API route validation and auth
- Alert rule names

## Agent Configuration

See `infra/examples/agent-config.yaml` for a full annotated config example.

Key sections:
```yaml
server:
  url: "http://your-server:8000"
  token: "your-token"

agent:
  machine_id: "..."    # set by init
  environment: "production"
  tags: [crawler, weibo, tw]

probes:
  - name: "daily_events"
    type: "sql_count"
    sql:
      dsn: "postgres://..."
      query: "SELECT COUNT(*) FROM events WHERE ..."
```

## GitHub Releases

### Download release binaries

Push a version tag to GitHub and the release workflow will publish these assets on the GitHub Release page:

- `observer-agent-darwin-arm64`
- `observer-agent-darwin-amd64`
- `observer-agent-linux-amd64`
- `observer-server-darwin-arm64`
- `observer-server-darwin-amd64`
- `observer-server-linux-amd64`
- `observer-darwin-arm64.tar.gz`
- `observer-darwin-amd64.tar.gz`
- `observer-linux-amd64.tar.gz`
- `checksums.txt`

Use `arm64` for Apple Silicon Macs, `amd64` for Intel Macs, and `linux-amd64` for most 64-bit x86 Linux hosts.

### Create a release

The workflow file lives at `.github/workflows/release-binaries.yml` and runs automatically when a tag matching `v*` is pushed.

```bash
git tag v0.1.0
git push origin v0.1.0
```

After the workflow finishes, GitHub will create a Release with downloadable binaries and archives for macOS (`arm64`, `amd64`) and Linux (`amd64`).

## Installing Agent on a Remote Host

```bash
# Build the agent binary
make build-agent
# Outputs: bin/observer-agent

# Copy to remote host
scp bin/observer-agent user@host:/usr/local/bin/

# Initialize
ssh user@host
observer-agent init \
  --server http://your-server:8000

# Install as systemd service
sudo cp infra/systemd/observer-agent.service /etc/systemd/system/
sudo useradd --system observer-agent
sudo mkdir -p /var/lib/observer-agent
sudo chown observer-agent: /var/lib/observer-agent
sudo systemctl enable --now observer-agent
```

## Dashboard Pages

| Page | Description |
|------|-------------|
| Overview | System-wide stats: hosts, jobs, alerts, recent failures |
| Hosts | All registered hosts with heartbeat status and tags |
| Jobs | All discovered cron jobs with schedules |
| Job Detail | Execution history, confidence scores, detection evidence |
| Metrics | Probe results with values and statuses |
| Alerts | Active and resolved alerts |

## Probe Types

| Type | Description |
|------|-------------|
| `sql_count` | Run a COUNT query against PostgreSQL |
| `file_freshness` | Check file existence and age |
| `command` | Run a command, parse output as a number |

## Known Limitations

1. **No exact exit codes** — The agent observes externally without modifying cronjobs. Job success/failure is inferred from process lifecycle and cron log signals. Confidence scores reflect this uncertainty.

2. **Process matching is approximate** — Matching uses command substring comparison. A `confidence_score < 1.0` indicates an inferred (not certain) match.

3. **Journald observation is MVP** — The journal observer calls `journalctl` as a subprocess. A production implementation would use the native C journal API or a Go binding.

4. **Auth is minimal** — Agents still use a static bearer token, and dashboard users use a single username/password with in-memory sessions. Production should use proper user management and durable sessions.

5. **No TLS** — The MVP runs over plain HTTP. Add a reverse proxy (Caddy, nginx) with TLS for production.

6. **SQL probe requires PostgreSQL** — The `sql_count` probe uses the `lib/pq` driver. Other databases require adding a driver dependency.

## Makefile Reference

```
make dev          Start all via docker-compose
make dev-db       Start only postgres
make dev-server   Run server locally
make dev-web      Run frontend dev server
make build        Build all binaries
make test         Run all Go tests
make migrate-up   Apply pending migrations
make migrate-down Rollback last migration
make seed         Insert sample data
make tidy         Tidy all go.mod files
make sqlc-gen     Regenerate sqlc (if using sqlc)
make agent-init   Init agent against local server
```

## Project Structure

```
dysonsphere/
  README.md
  Makefile
  docker-compose.yml
  .env.example
  go.work                   Go workspace (server + agent + shared)
  apps/
    server/
      cmd/server/           Entry point
      internal/
        api/                HTTP handlers, router, request/response types
        alerting/           Alert rule evaluator
        config/             Config from environment
        db/                 PostgreSQL pool + migrations
        domain/             Canonical domain models
        repository/         pgx persistence
        service/            Business logic
      migrations/           SQL migration files
    agent/
      cmd/agent/            CLI entry point (init, run)
      internal/
        app/                Main loop
        client/             HTTP client
        config/             YAML config
        discovery/          Cron file scanner
        observer/           Process + journal observer
        probes/             Probe runner
        reporter/           Batch upload
        state/              bbolt local state
    web/
      src/
        api/                API client + TypeScript types
        components/         Reusable UI components
        pages/              Route pages
  packages/shared/          Shared constants
  infra/
    systemd/                Agent service template
    examples/               Sample configs and fixtures
  docs/
    architecture.md
    api.md
    agent.md
```

## Roadmap

- [ ] Per-agent JWT tokens (replace static bearer token)
- [ ] TLS support / HTTPS
- [ ] Real-time streaming via SSE or WebSocket
- [ ] MySQL / SQLite probe drivers
- [ ] Native journald integration (via go-systemd)
- [ ] Alert notification webhooks (Slack, PagerDuty)
- [ ] Missed schedule detection (calculate expected next run from cron expression)
- [ ] Historical trend charts in dashboard
- [ ] Multi-tenant / org support
- [ ] Agent auto-update mechanism
