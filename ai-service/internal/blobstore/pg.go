package blobstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore is the PostgreSQL-backed BlobStore backed by the ai_audit_blobs table.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates a PGStore backed by pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// Put inserts a blob row. Duplicate sha256 values are silently ignored.
func (s *PGStore) Put(ctx context.Context, sha256 string, data []byte, contentType string) error {
	const q = `
		INSERT INTO ai_audit_blobs (sha256, content, content_type, size_bytes)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (sha256) DO NOTHING`
	_, err := s.pool.Exec(ctx, q, sha256, data, contentType, len(data))
	return err
}
