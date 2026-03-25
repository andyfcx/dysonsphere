-- name: CreateHost :one
INSERT INTO hosts (machine_id, hostname, ip_address, environment, tags, agent_version, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetHostByID :one
SELECT * FROM hosts WHERE id = $1;

-- name: GetHostByMachineID :one
SELECT * FROM hosts WHERE machine_id = $1;

-- name: ListHosts :many
SELECT * FROM hosts ORDER BY registered_at DESC;

-- name: UpdateHostHeartbeat :one
UPDATE hosts
SET last_heartbeat_at = NOW(), status = 'active', ip_address = $2
WHERE id = $1
RETURNING *;

-- name: UpdateHostStatus :exec
UPDATE hosts SET status = $2 WHERE id = $1;

-- name: UpsertHost :one
INSERT INTO hosts (machine_id, hostname, ip_address, environment, tags, agent_version, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (machine_id) DO UPDATE
    SET hostname = EXCLUDED.hostname,
        ip_address = EXCLUDED.ip_address,
        environment = EXCLUDED.environment,
        tags = EXCLUDED.tags,
        agent_version = EXCLUDED.agent_version,
        metadata = EXCLUDED.metadata
RETURNING *;
