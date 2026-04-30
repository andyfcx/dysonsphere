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

// JobRepo handles persistence of Job entities.
type JobRepo struct {
	db *pgxpool.Pool
}

func NewJobRepo(db *pgxpool.Pool) *JobRepo {
	return &JobRepo{db: db}
}

// Upsert inserts or updates a job by (host_id, command_hash).
func (r *JobRepo) Upsert(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	var metaRaw []byte
	if j.Metadata != nil {
		metaRaw = j.Metadata
	}
	hostID, err := uuid.Parse(j.HostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id %q: %w", j.HostID, err)
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO jobs
		    (host_id, source_type, schedule, timezone, "user", raw_command,
		     normalized_command, command_hash, enabled, source_file, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (host_id, command_hash) DO UPDATE
		    SET schedule = EXCLUDED.schedule,
		        timezone = EXCLUDED.timezone,
		        "user" = EXCLUDED.user,
		        raw_command = EXCLUDED.raw_command,
		        normalized_command = EXCLUDED.normalized_command,
		        enabled = EXCLUDED.enabled,
		        source_file = EXCLUDED.source_file,
		        metadata = EXCLUDED.metadata,
		        updated_at = NOW()
		RETURNING id, host_id, source_type, schedule, timezone, "user",
		          raw_command, normalized_command, command_hash, enabled,
		          source_file, metadata, created_at, updated_at
	`, hostID, string(j.SourceType), j.Schedule, j.Timezone, j.User,
		j.RawCommand, j.NormalizedCommand, j.CommandHash,
		j.Enabled, j.SourceFile, metaRaw)

	return scanJob(row)
}

// GetByID returns a job by UUID string.
func (r *JobRepo) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	jobID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid job id: %w", err)
	}
	row := r.db.QueryRow(ctx, `
		SELECT id, host_id, source_type, schedule, timezone, "user",
		       raw_command, normalized_command, command_hash, enabled,
		       source_file, metadata, created_at, updated_at
		FROM jobs WHERE id = $1
	`, jobID)
	return scanJob(row)
}

// List returns all jobs.
func (r *JobRepo) List(ctx context.Context) ([]*domain.Job, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, host_id, source_type, schedule, timezone, "user",
		       raw_command, normalized_command, command_hash, enabled,
		       source_file, metadata, created_at, updated_at
		FROM jobs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("jobs list: %w", err)
	}
	defer rows.Close()
	return collectJobs(rows)
}

// ListByHost returns jobs for a specific host.
func (r *JobRepo) ListByHost(ctx context.Context, hostID string) ([]*domain.Job, error) {
	hid, err := uuid.Parse(hostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host id: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, host_id, source_type, schedule, timezone, "user",
		       raw_command, normalized_command, command_hash, enabled,
		       source_file, metadata, created_at, updated_at
		FROM jobs WHERE host_id = $1 ORDER BY created_at DESC
	`, hid)
	if err != nil {
		return nil, fmt.Errorf("jobs by host: %w", err)
	}
	defer rows.Close()
	return collectJobs(rows)
}

// GetByHostAndHash returns the job for a host+command hash pair.
func (r *JobRepo) GetByHostAndHash(ctx context.Context, hostID, commandHash string) (*domain.Job, error) {
	hid, err := uuid.Parse(hostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host id: %w", err)
	}
	row := r.db.QueryRow(ctx, `
		SELECT id, host_id, source_type, schedule, timezone, "user",
		       raw_command, normalized_command, command_hash, enabled,
		       source_file, metadata, created_at, updated_at
		FROM jobs WHERE host_id = $1 AND command_hash = $2
	`, hid, commandHash)
	return scanJob(row)
}

// UpsertCurrentState upserts current_job_states record.
func (r *JobRepo) UpsertCurrentState(ctx context.Context, jobID, hostID string,
	status domain.ExecutionStatus, lastRunAt, nextExpectedAt *time.Time, consecutiveFail int) error {

	jid, err := uuid.Parse(jobID)
	if err != nil {
		return err
	}
	hid, err := uuid.Parse(hostID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO current_job_states
		    (job_id, host_id, last_status, last_run_at, next_expected_at, consecutive_fail, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())
		ON CONFLICT (job_id) DO UPDATE
		    SET last_status = EXCLUDED.last_status,
		        last_run_at = EXCLUDED.last_run_at,
		        next_expected_at = EXCLUDED.next_expected_at,
		        consecutive_fail = EXCLUDED.consecutive_fail,
		        updated_at = NOW()
	`, jid, hid, string(status), lastRunAt, nextExpectedAt, consecutiveFail)
	return err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanJob(row pgx.Row) (*domain.Job, error) {
	var j domain.Job
	var id, hostID uuid.UUID
	var meta []byte
	var updatedAt time.Time

	err := row.Scan(
		&id, &hostID, &j.SourceType, &j.Schedule, &j.Timezone, &j.User,
		&j.RawCommand, &j.NormalizedCommand, &j.CommandHash, &j.Enabled,
		&j.SourceFile, &meta, &j.CreatedAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.ID = id.String()
	j.HostID = hostID.String()
	j.UpdatedAt = updatedAt
	if meta != nil {
		j.Metadata = json.RawMessage(meta)
	}
	return &j, nil
}

func collectJobs(rows pgx.Rows) ([]*domain.Job, error) {
	jobs := make([]*domain.Job, 0)
	for rows.Next() {
		var j domain.Job
		var id, hostID uuid.UUID
		var meta []byte
		var updatedAt time.Time

		if err := rows.Scan(
			&id, &hostID, &j.SourceType, &j.Schedule, &j.Timezone, &j.User,
			&j.RawCommand, &j.NormalizedCommand, &j.CommandHash, &j.Enabled,
			&j.SourceFile, &meta, &j.CreatedAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		j.ID = id.String()
		j.HostID = hostID.String()
		j.UpdatedAt = updatedAt
		if meta != nil {
			j.Metadata = json.RawMessage(meta)
		}
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}
