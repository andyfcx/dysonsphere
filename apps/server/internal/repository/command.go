package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andyfcx/observer/server/internal/domain"
)

// CommandRepo stores server-side agent command requests.
type CommandRepo struct {
	db *pgxpool.Pool
}

func NewCommandRepo(db *pgxpool.Pool) *CommandRepo {
	return &CommandRepo{db: db}
}

func (r *CommandRepo) Insert(ctx context.Context, cmd *domain.AgentCommand) (*domain.AgentCommand, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO agent_commands (host_id, job_id, command_hash, raw_command, schedule, status, message)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, host_id, job_id, command_hash, raw_command, schedule, status, message,
		          exit_code, requested_at, dispatched_at, started_at, finished_at
	`, cmd.HostID, cmd.JobID, cmd.CommandHash, cmd.RawCommand, cmd.Schedule, string(cmd.Status), cmd.Message)
	return scanCommand(row)
}

func (r *CommandRepo) ClaimPending(ctx context.Context, hostID string, limit int) ([]*domain.AgentCommand, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(ctx, `
		WITH picked AS (
			SELECT id
			FROM agent_commands
			WHERE host_id = $1 AND status = 'pending'
			ORDER BY requested_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE agent_commands
		SET status = 'dispatched', dispatched_at = NOW()
		WHERE id IN (SELECT id FROM picked)
		RETURNING id, host_id, job_id, command_hash, raw_command, schedule, status, message,
		          exit_code, requested_at, dispatched_at, started_at, finished_at
	`, hostID, limit)
	if err != nil {
		return nil, fmt.Errorf("claim commands: %w", err)
	}
	defer rows.Close()
	return collectCommands(rows)
}

func (r *CommandRepo) Complete(ctx context.Context, id string, status domain.AgentCommandStatus, message string, startedAt, finishedAt *time.Time, exitCode *int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE agent_commands
		SET status = $2, message = $3, started_at = COALESCE($4, started_at),
		    finished_at = COALESCE($5, finished_at), exit_code = $6
		WHERE id = $1
	`, id, string(status), message, startedAt, finishedAt, exitCode)
	return err
}

func scanCommand(row pgx.Row) (*domain.AgentCommand, error) {
	var cmd domain.AgentCommand
	var id, hostID, jobID uuid.UUID
	if err := row.Scan(
		&id, &hostID, &jobID, &cmd.CommandHash, &cmd.RawCommand, &cmd.Schedule, &cmd.Status, &cmd.Message,
		&cmd.ExitCode, &cmd.RequestedAt, &cmd.DispatchedAt, &cmd.StartedAt, &cmd.FinishedAt,
	); err != nil {
		return nil, err
	}
	cmd.ID = id.String()
	cmd.HostID = hostID.String()
	cmd.JobID = jobID.String()
	return &cmd, nil
}

func collectCommands(rows pgx.Rows) ([]*domain.AgentCommand, error) {
	var items []*domain.AgentCommand
	for rows.Next() {
		item, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
