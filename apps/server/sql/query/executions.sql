-- name: InsertExecution :one
INSERT INTO executions (host_id, job_id, scheduled_at, detected_started_at, detected_finished_at, duration_seconds, status, confidence_score, detection_sources, evidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListExecutions :many
SELECT * FROM executions ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: ListExecutionsByJob :many
SELECT * FROM executions WHERE job_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: ListExecutionsByHost :many
SELECT * FROM executions WHERE host_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: CountRecentFailuresByJob :one
SELECT COUNT(*) FROM executions
WHERE job_id = $1
  AND status IN ('failed','unknown')
  AND created_at > NOW() - INTERVAL '24 hours';
