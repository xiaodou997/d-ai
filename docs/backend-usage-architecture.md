# Backend Usage Architecture

The backend now uses a two-layer usage model:

- `ai_usage_logs`: request-level audit records
- `ai_usage_rollups_hourly`: hourly statistical aggregates

## Source Of Truth

- Audit, debugging, recent error inspection, and request drill-down use `ai_usage_logs`.
- Dashboard, summary, ranking, and chart queries use `ai_usage_rollups_hourly`.

This split is intentional. Raw logs preserve request detail. Rollups provide stable and cheaper statistical queries.

## Write Path

Usage settlement is strongly consistent. One transaction performs:

1. usage log insert
2. hourly rollup upsert
3. quota confirmation or cancellation

If any step fails, the whole usage write is rolled back.

## Read Path

Use raw logs when the caller needs a concrete request record.

Use hourly rollups when the caller needs:

- dashboard totals
- model rankings
- tenant rankings
- user or tenant statistical summaries
- chart time buckets

## Time Semantics

- Rollups are bucketed by hour.
- `days` and similar filters apply to bucket timestamps.
- `days=0` means all available history.

## Schema Management

- `ai-service/migrations/*.sql` is the only schema source of truth.
- `ai-service/cmd/migrate` records applied versions in `schema_migrations`.
- `migrate up` applies only unapplied migrations.
- `migrate down` rolls back only the latest applied migration.
- `db/local_seed.sql` is sample development data, not schema.
