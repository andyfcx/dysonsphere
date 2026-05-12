package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andyfcx/observer/server/internal/domain"
)

// ExecutionFilter specifies optional filters for ListFiltered.
type ExecutionFilter struct {
	HostID string
	JobID  string
	Status string
	Since  *time.Time
}

// ExecutionRepo handles persistence of Execution events.
type ExecutionRepo struct {
	db *pgxpool.Pool
}

func NewExecutionRepo(db *pgxpool.Pool) *ExecutionRepo {
	return &ExecutionRepo{db: db}
}

// Insert writes a new execution record. Returns the created record.
func (r *ExecutionRepo) Insert(ctx context.Context, e *domain.Execution) (*domain.Execution, error) {
	hostID, err := uuid.Parse(e.HostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id: %w", err)
	}
	var jobID *uuid.UUID
	if e.JobID != nil {
		jid, err := uuid.Parse(*e.JobID)
		if err != nil {
			return nil, fmt.Errorf("invalid job_id: %w", err)
		}
		jobID = &jid
	}
	var evidenceRaw []byte
	if e.Evidence != nil {
		evidenceRaw = e.Evidence
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO executions
		    (host_id, job_id, scheduled_at, detected_started_at, detected_finished_at,
		     duration_seconds, status, confidence_score, detection_sources, output_text, evidence)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, host_id, job_id, scheduled_at, detected_started_at,
		          detected_finished_at, duration_seconds, status, confidence_score,
		          detection_sources, output_text, evidence, created_at
	`, hostID, jobID, e.ScheduledAt, e.DetectedStartedAt, e.DetectedFinishedAt,
		e.DurationSeconds, string(e.Status), e.ConfidenceScore,
		e.DetectionSources, e.OutputText, evidenceRaw)

	return scanExecution(row)
}

// List returns paginated executions.
func (r *ExecutionRepo) List(ctx context.Context, limit, offset int) ([]*domain.Execution, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, host_id, job_id, scheduled_at, detected_started_at,
		       detected_finished_at, duration_seconds, status, confidence_score,
		       detection_sources, output_text, evidence, created_at
		FROM executions ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("executions list: %w", err)
	}
	defer rows.Close()
	return collectExecutions(rows)
}

// ListByJob returns recent executions for a job.
func (r *ExecutionRepo) ListByJob(ctx context.Context, jobID string, limit int) ([]*domain.Execution, error) {
	jid, err := uuid.Parse(jobID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, host_id, job_id, scheduled_at, detected_started_at,
		       detected_finished_at, duration_seconds, status, confidence_score,
		       detection_sources, output_text, evidence, created_at
		FROM executions WHERE job_id = $1 ORDER BY created_at DESC LIMIT $2
	`, jid, limit)
	if err != nil {
		return nil, fmt.Errorf("executions by job: %w", err)
	}
	defer rows.Close()
	return collectExecutions(rows)
}

// ListFiltered returns paginated executions matching the given filter.
// Zero-value filter fields are ignored.
func (r *ExecutionRepo) ListFiltered(ctx context.Context, f ExecutionFilter, limit, offset int) ([]*domain.Execution, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []any{}
	conditions := []string{}
	n := 1

	if f.HostID != "" {
		hid, err := uuid.Parse(f.HostID)
		if err != nil {
			return nil, fmt.Errorf("invalid host_id: %w", err)
		}
		conditions = append(conditions, fmt.Sprintf("host_id = $%d", n))
		args = append(args, hid)
		n++
	}
	if f.JobID != "" {
		jid, err := uuid.Parse(f.JobID)
		if err != nil {
			return nil, fmt.Errorf("invalid job_id: %w", err)
		}
		conditions = append(conditions, fmt.Sprintf("job_id = $%d", n))
		args = append(args, jid)
		n++
	}
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", n))
		args = append(args, f.Status)
		n++
	}
	if f.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", n))
		args = append(args, *f.Since)
		n++
	}

	q := `SELECT id, host_id, job_id, scheduled_at, detected_started_at,
	             detected_finished_at, duration_seconds, status, confidence_score,
	             detection_sources, output_text, evidence, created_at
	      FROM executions`
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("filtered executions: %w", err)
	}
	defer rows.Close()
	return collectExecutions(rows)
}

// CountRecentFailures counts failed/unknown executions for a job in the past 24 hours.
func (r *ExecutionRepo) CountRecentFailures(ctx context.Context, jobID string) (int64, error) {
	jid, err := uuid.Parse(jobID)
	if err != nil {
		return 0, err
	}
	var count int64
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM executions
		WHERE job_id = $1
		  AND status IN ('failed','unknown')
		  AND created_at > NOW() - INTERVAL '24 hours'
	`, jid).Scan(&count)
	return count, err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanExecution(row pgx.Row) (*domain.Execution, error) {
	var e domain.Execution
	var id, hostID uuid.UUID
	var jobID *uuid.UUID
	var evidence []byte
	var sources []string
	var finishedAt *time.Time
	var startedAt *time.Time
	var scheduledAt *time.Time
	var durSec *float64

	err := row.Scan(
		&id, &hostID, &jobID, &scheduledAt, &startedAt,
		&finishedAt, &durSec, &e.Status, &e.ConfidenceScore,
		&sources, &e.OutputText, &evidence, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan execution: %w", err)
	}
	e.ID = id.String()
	e.HostID = hostID.String()
	if jobID != nil {
		s := jobID.String()
		e.JobID = &s
	}
	e.ScheduledAt = scheduledAt
	e.DetectedStartedAt = startedAt
	e.DetectedFinishedAt = finishedAt
	e.DurationSeconds = durSec
	e.DetectionSources = sources
	if evidence != nil {
		e.Evidence = json.RawMessage(evidence)
	}
	return &e, nil
}

func collectExecutions(rows pgx.Rows) ([]*domain.Execution, error) {
	execs := make([]*domain.Execution, 0)
	for rows.Next() {
		var e domain.Execution
		var id, hostID uuid.UUID
		var jobID *uuid.UUID
		var evidence []byte
		var sources []string
		var finishedAt, startedAt, scheduledAt *time.Time
		var durSec *float64

		if err := rows.Scan(
			&id, &hostID, &jobID, &scheduledAt, &startedAt,
			&finishedAt, &durSec, &e.Status, &e.ConfidenceScore,
			&sources, &e.OutputText, &evidence, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution row: %w", err)
		}
		e.ID = id.String()
		e.HostID = hostID.String()
		if jobID != nil {
			s := jobID.String()
			e.JobID = &s
		}
		e.ScheduledAt = scheduledAt
		e.DetectedStartedAt = startedAt
		e.DetectedFinishedAt = finishedAt
		e.DurationSeconds = durSec
		e.DetectionSources = sources
		if evidence != nil {
			e.Evidence = json.RawMessage(evidence)
		}
		execs = append(execs, &e)
	}
	return execs, rows.Err()
}
