package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CredentialRepo stores issued agent credentials.
type CredentialRepo struct {
	db *pgxpool.Pool
}

func NewCredentialRepo(db *pgxpool.Pool) *CredentialRepo {
	return &CredentialRepo{db: db}
}

func (r *CredentialRepo) Upsert(ctx context.Context, hostID, tokenHash string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO agent_credentials (host_id, token_hash, created_at, rotated_at)
		VALUES ($1,$2,NOW(),NOW())
		ON CONFLICT (host_id) DO UPDATE
		    SET token_hash = EXCLUDED.token_hash,
		        rotated_at = NOW()
	`, hostID, tokenHash)
	return err
}

func (r *CredentialRepo) Validate(ctx context.Context, hostID, tokenHash string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM agent_credentials
		WHERE host_id = $1 AND token_hash = $2
	`, hostID, tokenHash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate credential: %w", err)
	}
	return count == 1, nil
}
