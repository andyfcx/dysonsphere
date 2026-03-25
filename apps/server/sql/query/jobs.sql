-- name: UpsertJob :one
INSERT INTO jobs (host_id, source_type, schedule, timezone, "user", raw_command, normalized_command, command_hash, enabled, source_file, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
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
RETURNING *;

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListJobs :many
SELECT * FROM jobs ORDER BY created_at DESC;

-- name: ListJobsByHost :many
SELECT * FROM jobs WHERE host_id = $1 ORDER BY created_at DESC;
