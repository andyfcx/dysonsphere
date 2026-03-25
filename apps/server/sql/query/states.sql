-- name: UpsertCurrentHostState :exec
INSERT INTO current_host_states (host_id, status, last_heartbeat_at, active_jobs, updated_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (host_id) DO UPDATE
    SET status = EXCLUDED.status,
        last_heartbeat_at = EXCLUDED.last_heartbeat_at,
        active_jobs = EXCLUDED.active_jobs,
        updated_at = NOW();

-- name: GetCurrentHostState :one
SELECT * FROM current_host_states WHERE host_id = $1;

-- name: UpsertCurrentJobState :exec
INSERT INTO current_job_states (job_id, host_id, last_status, last_run_at, next_expected_at, consecutive_fail, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (job_id) DO UPDATE
    SET last_status = EXCLUDED.last_status,
        last_run_at = EXCLUDED.last_run_at,
        next_expected_at = EXCLUDED.next_expected_at,
        consecutive_fail = EXCLUDED.consecutive_fail,
        updated_at = NOW();

-- name: GetCurrentJobState :one
SELECT * FROM current_job_states WHERE job_id = $1;

-- name: ListCurrentJobStatesByHost :many
SELECT * FROM current_job_states WHERE host_id = $1;

-- name: CountStaleHosts :one
SELECT COUNT(*) FROM current_host_states
WHERE status = 'active'
  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < NOW() - INTERVAL '5 minutes');
