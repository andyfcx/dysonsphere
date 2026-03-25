// Package repository provides PostgreSQL-backed storage for all domain entities.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andyfcx/observer/server/internal/domain"
)

// HostRepo handles persistence of Host entities.
type HostRepo struct {
	db *pgxpool.Pool
}

func NewHostRepo(db *pgxpool.Pool) *HostRepo {
	return &HostRepo{db: db}
}

// Upsert inserts or updates a host by machine_id.
func (r *HostRepo) Upsert(ctx context.Context, h *domain.Host) (*domain.Host, error) {
	tags := h.Tags
	if tags == nil {
		tags = []string{}
	}
	var metaRaw []byte
	if h.Metadata != nil {
		metaRaw = h.Metadata
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO hosts (machine_id, hostname, ip_address, environment, tags, agent_version, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (machine_id) DO UPDATE
		    SET hostname = EXCLUDED.hostname,
		        ip_address = EXCLUDED.ip_address,
		        environment = EXCLUDED.environment,
		        tags = EXCLUDED.tags,
		        agent_version = EXCLUDED.agent_version,
		        metadata = EXCLUDED.metadata
		RETURNING id, machine_id, hostname, ip_address, environment, tags,
		          registered_at, last_heartbeat_at, status, agent_version, metadata
	`, h.MachineID, h.Hostname, h.IPAddress, h.Environment, tags, h.AgentVersion, metaRaw)

	return scanHost(row)
}

// GetByID returns a host by UUID.
func (r *HostRepo) GetByID(ctx context.Context, id string) (*domain.Host, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, machine_id, hostname, ip_address, environment, tags,
		       registered_at, last_heartbeat_at, status, agent_version, metadata
		FROM hosts WHERE id = $1
	`, id)
	return scanHost(row)
}

// GetByMachineID returns a host by machine_id.
func (r *HostRepo) GetByMachineID(ctx context.Context, machineID string) (*domain.Host, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, machine_id, hostname, ip_address, environment, tags,
		       registered_at, last_heartbeat_at, status, agent_version, metadata
		FROM hosts WHERE machine_id = $1
	`, machineID)
	return scanHost(row)
}

// List returns all hosts ordered by registration date.
func (r *HostRepo) List(ctx context.Context) ([]*domain.Host, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, machine_id, hostname, ip_address, environment, tags,
		       registered_at, last_heartbeat_at, status, agent_version, metadata
		FROM hosts ORDER BY registered_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("hosts list: %w", err)
	}
	defer rows.Close()
	return collectHosts(rows)
}

// UpdateHeartbeat sets last_heartbeat_at to now and marks host active.
func (r *HostRepo) UpdateHeartbeat(ctx context.Context, id, ipAddress string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE hosts SET last_heartbeat_at = NOW(), status = 'active', ip_address = $2
		WHERE id = $1
	`, id, ipAddress)
	return err
}

// UpdateStatus changes the status of a host.
func (r *HostRepo) UpdateStatus(ctx context.Context, id string, status domain.HostStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE hosts SET status = $2 WHERE id = $1`, id, string(status))
	return err
}

// MarkStaleHosts marks hosts whose heartbeat is older than threshold as stale.
func (r *HostRepo) MarkStaleHosts(ctx context.Context, threshold time.Duration) (int64, error) {
	result, err := r.db.Exec(ctx, `
		UPDATE hosts
		SET status = 'stale'
		WHERE status = 'active'
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < NOW() - $1::INTERVAL)
	`, threshold.String())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// UpsertCurrentState upserts the current_host_states record.
func (r *HostRepo) UpsertCurrentState(ctx context.Context, hostID string, status domain.HostStatus, lastHB *time.Time, activeJobs int) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO current_host_states (host_id, status, last_heartbeat_at, active_jobs, updated_at)
		VALUES ($1,$2,$3,$4,NOW())
		ON CONFLICT (host_id) DO UPDATE
		    SET status = EXCLUDED.status,
		        last_heartbeat_at = EXCLUDED.last_heartbeat_at,
		        active_jobs = EXCLUDED.active_jobs,
		        updated_at = NOW()
	`, hostID, string(status), lastHB, activeJobs)
	return err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanHost(row pgx.Row) (*domain.Host, error) {
	var h domain.Host
	var id uuid.UUID
	var tags []string
	var meta []byte
	var lastHB *time.Time

	err := row.Scan(
		&id, &h.MachineID, &h.Hostname, &h.IPAddress,
		&h.Environment, &tags, &h.RegisteredAt, &lastHB,
		&h.Status, &h.AgentVersion, &meta,
	)
	if err != nil {
		return nil, fmt.Errorf("scan host: %w", err)
	}
	h.ID = id.String()
	h.Tags = tags
	h.LastHeartbeatAt = lastHB
	if meta != nil {
		h.Metadata = json.RawMessage(meta)
	}
	return &h, nil
}

func collectHosts(rows pgx.Rows) ([]*domain.Host, error) {
	var hosts []*domain.Host
	for rows.Next() {
		var h domain.Host
		var id uuid.UUID
		var tags []string
		var meta []byte
		var lastHB *time.Time

		if err := rows.Scan(
			&id, &h.MachineID, &h.Hostname, &h.IPAddress,
			&h.Environment, &tags, &h.RegisteredAt, &lastHB,
			&h.Status, &h.AgentVersion, &meta,
		); err != nil {
			return nil, fmt.Errorf("scan host row: %w", err)
		}
		h.ID = id.String()
		h.Tags = tags
		h.LastHeartbeatAt = lastHB
		if meta != nil {
			h.Metadata = json.RawMessage(meta)
		}
		hosts = append(hosts, &h)
	}
	return hosts, rows.Err()
}
