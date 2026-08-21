package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

func translatePersistenceError(err error) error {
	if err == nil {
		return nil
	}
	var persistenceErr *domain.PersistenceError
	if errors.As(err, &persistenceErr) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.PersistenceError{Kind: domain.PersistenceNotFound, Cause: err}
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	var kind domain.PersistenceErrorKind
	switch pgErr.Code {
	case "23505":
		kind = domain.PersistenceConflict
	case "23503":
		kind = domain.PersistenceReferenceNotFound
	case "23514":
		kind = domain.PersistenceInvalidField
	case "22P02":
		kind = domain.PersistenceInvalidFormat
	default:
		return err
	}
	return &domain.PersistenceError{Kind: kind, Cause: err}
}

// NewQueries wraps sqlc's DBTX so row and exec errors cross the adapter
// boundary as domain errors. Query iteration errors remain operational errors.
func NewQueries(db dbgen.DBTX) *dbgen.Queries {
	return dbgen.New(translatingDBTX{db: db})
}

func queriesWithTx(tx pgx.Tx) *dbgen.Queries {
	return NewQueries(tx)
}

type translatingPool struct {
	*pgxpool.Pool
}

func newTranslatingPool(pool *pgxpool.Pool) *translatingPool {
	return &translatingPool{Pool: pool}
}

func (p *translatingPool) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tag, err := p.Pool.Exec(ctx, query, args...)
	return tag, translatePersistenceError(err)
}

func (p *translatingPool) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	rows, err := p.Pool.Query(ctx, query, args...)
	return rows, translatePersistenceError(err)
}

func (p *translatingPool) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return translatingRow{row: p.Pool.QueryRow(ctx, query, args...)}
}

func (p *translatingPool) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, translatePersistenceError(err)
	}
	return translatingTx{Tx: tx}, nil
}

func (p *translatingPool) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, translatePersistenceError(err)
	}
	return translatingTx{Tx: tx}, nil
}

func (p *translatingPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	conn, err := p.Pool.Acquire(ctx)
	return conn, translatePersistenceError(err)
}

func (p *translatingPool) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return translatingBatchResults{BatchResults: p.Pool.SendBatch(ctx, batch)}
}

type translatingTx struct {
	pgx.Tx
}

type translatingBatchResults struct {
	pgx.BatchResults
}

func (r translatingBatchResults) Exec() (pgconn.CommandTag, error) {
	tag, err := r.BatchResults.Exec()
	return tag, translatePersistenceError(err)
}

func (r translatingBatchResults) Query() (pgx.Rows, error) {
	rows, err := r.BatchResults.Query()
	return rows, translatePersistenceError(err)
}

func (r translatingBatchResults) QueryRow() pgx.Row {
	return translatingRow{row: r.BatchResults.QueryRow()}
}

func (r translatingBatchResults) Close() error {
	return translatePersistenceError(r.BatchResults.Close())
}

func (t translatingTx) Commit(ctx context.Context) error {
	return translatePersistenceError(t.Tx.Commit(ctx))
}

func (t translatingTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tag, err := t.Tx.Exec(ctx, query, args...)
	return tag, translatePersistenceError(err)
}

func (t translatingTx) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	rows, err := t.Tx.Query(ctx, query, args...)
	return rows, translatePersistenceError(err)
}

func (t translatingTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return translatingRow{row: t.Tx.QueryRow(ctx, query, args...)}
}

type translatingDBTX struct {
	db dbgen.DBTX
}

func (t translatingDBTX) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tag, err := t.db.Exec(ctx, query, args...)
	return tag, translatePersistenceError(err)
}

func (t translatingDBTX) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	rows, err := t.db.Query(ctx, query, args...)
	return rows, translatePersistenceError(err)
}

func (t translatingDBTX) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return translatingRow{row: t.db.QueryRow(ctx, query, args...)}
}

type translatingRow struct {
	row pgx.Row
}

func (r translatingRow) Scan(dest ...any) error {
	return translatePersistenceError(r.row.Scan(dest...))
}
