-- name: InsertAlert :one
INSERT INTO alerts (target_type, target_id, rule_name, severity, message)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListActiveAlerts :many
SELECT * FROM alerts WHERE status = 'active' ORDER BY triggered_at DESC;

-- name: ListAlerts :many
SELECT * FROM alerts ORDER BY triggered_at DESC LIMIT $1 OFFSET $2;

-- name: ResolveAlert :exec
UPDATE alerts SET status = 'resolved', resolved_at = NOW()
WHERE target_type = $1 AND target_id = $2 AND rule_name = $3 AND status = 'active';

-- name: GetActiveAlertByTarget :one
SELECT * FROM alerts
WHERE target_type = $1 AND target_id = $2 AND rule_name = $3 AND status = 'active'
LIMIT 1;

-- name: CountActiveAlerts :one
SELECT COUNT(*) FROM alerts WHERE status = 'active';
