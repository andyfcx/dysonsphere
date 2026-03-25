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

// MetricRepo handles persistence of DataMetric records.
type MetricRepo struct {
	db *pgxpool.Pool
}

func NewMetricRepo(db *pgxpool.Pool) *MetricRepo {
	return &MetricRepo{db: db}
}

// Insert writes a new data metric record.
func (r *MetricRepo) Insert(ctx context.Context, m *domain.DataMetric) (*domain.DataMetric, error) {
	hostID, err := uuid.Parse(m.HostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id: %w", err)
	}
	var dimRaw, evidRaw []byte
	if m.Dimensions != nil {
		dimRaw = m.Dimensions
	}
	if m.Evidence != nil {
		evidRaw = m.Evidence
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO data_metrics
		    (host_id, metric_name, metric_type, dimensions, value, measured_at, status, evidence)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, host_id, metric_name, metric_type, dimensions, value,
		          measured_at, status, evidence, created_at
	`, hostID, m.MetricName, string(m.MetricType), dimRaw,
		m.Value, m.MeasuredAt, string(m.Status), evidRaw)

	return scanMetric(row)
}

// List returns paginated metrics ordered by measured_at desc.
func (r *MetricRepo) List(ctx context.Context, limit, offset int) ([]*domain.DataMetric, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, host_id, metric_name, metric_type, dimensions, value,
		       measured_at, status, evidence, created_at
		FROM data_metrics ORDER BY measured_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("metrics list: %w", err)
	}
	defer rows.Close()
	return collectMetrics(rows)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanMetric(row pgx.Row) (*domain.DataMetric, error) {
	var m domain.DataMetric
	var id, hostID uuid.UUID
	var dim, evid []byte
	var measuredAt time.Time

	err := row.Scan(
		&id, &hostID, &m.MetricName, &m.MetricType, &dim,
		&m.Value, &measuredAt, &m.Status, &evid, &m.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan metric: %w", err)
	}
	m.ID = id.String()
	m.HostID = hostID.String()
	m.MeasuredAt = measuredAt
	if dim != nil {
		m.Dimensions = json.RawMessage(dim)
	}
	if evid != nil {
		m.Evidence = json.RawMessage(evid)
	}
	return &m, nil
}

func collectMetrics(rows pgx.Rows) ([]*domain.DataMetric, error) {
	var metrics []*domain.DataMetric
	for rows.Next() {
		var m domain.DataMetric
		var id, hostID uuid.UUID
		var dim, evid []byte
		var measuredAt time.Time

		if err := rows.Scan(
			&id, &hostID, &m.MetricName, &m.MetricType, &dim,
			&m.Value, &measuredAt, &m.Status, &evid, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan metric row: %w", err)
		}
		m.ID = id.String()
		m.HostID = hostID.String()
		m.MeasuredAt = measuredAt
		if dim != nil {
			m.Dimensions = json.RawMessage(dim)
		}
		if evid != nil {
			m.Evidence = json.RawMessage(evid)
		}
		metrics = append(metrics, &m)
	}
	return metrics, rows.Err()
}
