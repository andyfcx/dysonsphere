package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andyfcx/observer/server/internal/domain"
)

// EnrollmentTokenRepo manages one-time enrollment tokens in the database.
type EnrollmentTokenRepo struct {
	db *pgxpool.Pool
}

func NewEnrollmentTokenRepo(db *pgxpool.Pool) *EnrollmentTokenRepo {
	return &EnrollmentTokenRepo{db: db}
}

func (r *EnrollmentTokenRepo) Create(ctx context.Context, tokenHash, label string, expiresAt time.Time) (*domain.EnrollmentToken, error) {
	var t domain.EnrollmentToken
	err := r.db.QueryRow(ctx, `
		INSERT INTO enrollment_tokens (token_hash, label, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, label, used, used_at, expires_at, created_at
	`, tokenHash, label, expiresAt).Scan(
		&t.ID, &t.Label, &t.Used, &t.UsedAt, &t.ExpiresAt, &t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create enrollment token: %w", err)
	}
	return &t, nil
}

// Consume marks a token as used. Returns an error if the token is not found,
// already used, or expired.
func (r *EnrollmentTokenRepo) Consume(ctx context.Context, tokenHash string) error {
	var used bool
	var expiresAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT used, expires_at FROM enrollment_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&used, &expiresAt)
	if err == pgx.ErrNoRows {
		return nil // not found — caller falls back to static token
	}
	if err != nil {
		return fmt.Errorf("lookup enrollment token: %w", err)
	}
	if used {
		return fmt.Errorf("enrollment token has already been used")
	}
	if time.Now().After(expiresAt) {
		return fmt.Errorf("enrollment token has expired")
	}
	_, err = r.db.Exec(ctx, `
		UPDATE enrollment_tokens SET used = TRUE, used_at = NOW() WHERE token_hash = $1
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("consume enrollment token: %w", err)
	}
	return nil
}

// Found reports whether a token hash exists in the DB (regardless of used/expired state).
func (r *EnrollmentTokenRepo) Found(ctx context.Context, tokenHash string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM enrollment_tokens WHERE token_hash = $1`, tokenHash).Scan(&count)
	return count > 0, err
}

func (r *EnrollmentTokenRepo) List(ctx context.Context) ([]*domain.EnrollmentToken, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, label, used, used_at, expires_at, created_at
		FROM enrollment_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list enrollment tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]*domain.EnrollmentToken, 0)
	for rows.Next() {
		var t domain.EnrollmentToken
		if err := rows.Scan(&t.ID, &t.Label, &t.Used, &t.UsedAt, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}
