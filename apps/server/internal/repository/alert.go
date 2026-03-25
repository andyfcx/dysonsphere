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

// AlertRepo handles persistence of Alert records.
type AlertRepo struct {
	db *pgxpool.Pool
}

func NewAlertRepo(db *pgxpool.Pool) *AlertRepo {
	return &AlertRepo{db: db}
}

// Insert creates a new alert.
func (r *AlertRepo) Insert(ctx context.Context, a *domain.Alert) (*domain.Alert, error) {
	targetID, err := uuid.Parse(a.TargetID)
	if err != nil {
		return nil, fmt.Errorf("invalid target_id: %w", err)
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO alerts (target_type, target_id, rule_name, severity, message)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, target_type, target_id, rule_name, severity, status,
		          message, triggered_at, resolved_at
	`, a.TargetType, targetID, a.RuleName, string(a.Severity), a.Message)
	return scanAlert(row)
}

// GetActiveByTarget returns the active alert for a target+rule combination, or nil.
func (r *AlertRepo) GetActiveByTarget(ctx context.Context, targetType, targetID, ruleName string) (*domain.Alert, error) {
	tid, err := uuid.Parse(targetID)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRow(ctx, `
		SELECT id, target_type, target_id, rule_name, severity, status,
		       message, triggered_at, resolved_at
		FROM alerts
		WHERE target_type = $1 AND target_id = $2 AND rule_name = $3 AND status = 'active'
		LIMIT 1
	`, targetType, tid, ruleName)
	a, err := scanAlert(row)
	if err != nil {
		if err.Error() == "scan alert: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// Resolve marks an alert as resolved.
func (r *AlertRepo) Resolve(ctx context.Context, targetType, targetID, ruleName string) error {
	tid, err := uuid.Parse(targetID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE alerts SET status = 'resolved', resolved_at = NOW()
		WHERE target_type = $1 AND target_id = $2 AND rule_name = $3 AND status = 'active'
	`, targetType, tid, ruleName)
	return err
}

// ListActive returns all active alerts ordered by triggered_at desc.
func (r *AlertRepo) ListActive(ctx context.Context) ([]*domain.Alert, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, target_type, target_id, rule_name, severity, status,
		       message, triggered_at, resolved_at
		FROM alerts WHERE status = 'active' ORDER BY triggered_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("active alerts: %w", err)
	}
	defer rows.Close()
	return collectAlerts(rows)
}

// List returns all alerts (paginated).
func (r *AlertRepo) List(ctx context.Context, limit, offset int) ([]*domain.Alert, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, target_type, target_id, rule_name, severity, status,
		       message, triggered_at, resolved_at
		FROM alerts ORDER BY triggered_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("alerts list: %w", err)
	}
	defer rows.Close()
	return collectAlerts(rows)
}

// CountActive returns the number of active alerts.
func (r *AlertRepo) CountActive(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM alerts WHERE status = 'active'`).Scan(&count)
	return count, err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanAlert(row pgx.Row) (*domain.Alert, error) {
	var a domain.Alert
	var id, targetID uuid.UUID
	var resolvedAt *time.Time

	err := row.Scan(
		&id, &a.TargetType, &targetID, &a.RuleName, &a.Severity, &a.Status,
		&a.Message, &a.TriggeredAt, &resolvedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan alert: %s", err.Error())
	}
	a.ID = id.String()
	a.TargetID = targetID.String()
	a.ResolvedAt = resolvedAt
	return &a, nil
}

func collectAlerts(rows pgx.Rows) ([]*domain.Alert, error) {
	var alerts []*domain.Alert
	for rows.Next() {
		var a domain.Alert
		var id, targetID uuid.UUID
		var resolvedAt *time.Time

		if err := rows.Scan(
			&id, &a.TargetType, &targetID, &a.RuleName, &a.Severity, &a.Status,
			&a.Message, &a.TriggeredAt, &resolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan alert row: %w", err)
		}
		a.ID = id.String()
		a.TargetID = targetID.String()
		a.ResolvedAt = resolvedAt
		alerts = append(alerts, &a)
	}
	return alerts, rows.Err()
}
