-- name: InsertDataMetric :one
INSERT INTO data_metrics (host_id, metric_name, metric_type, dimensions, value, measured_at, status, evidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListDataMetrics :many
SELECT * FROM data_metrics ORDER BY measured_at DESC LIMIT $1 OFFSET $2;

-- name: ListDataMetricsByHost :many
SELECT * FROM data_metrics WHERE host_id = $1 ORDER BY measured_at DESC LIMIT $2;

-- name: ListDataMetricsByName :many
SELECT * FROM data_metrics WHERE metric_name = $1 ORDER BY measured_at DESC LIMIT $2;
