# Local Database Setup

Local PostgreSQL initialization now uses versioned migrations plus a separate local seed:

- `backend/migrations/*.sql`: schema source of truth.
- `db/local_seed.sql`: local development data.

The local seed uses `tenant-local` and runtime key `sk-ai-local-dev`. The `openai_compatible` provider points to the fake upstream at `http://127.0.0.1:18080/v1` for chat, responses, embeddings, and images. DeepSeek is included only as a real-provider sample.

## Config

Create a private local config from the checked-in example:

```bash
cp backend/config.local.example.yaml backend/config.local.yaml
```

Edit the PostgreSQL DSN if needed. `backend/config.local.yaml` is ignored by git.

## Schema And Seed

Use the Go commands:

```bash
cd backend
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/migrate up
UNI_AI_API_CONFIG=config.local.yaml go run ./cmd/seed
```

If you need manual inspection in Navicat, inspect the database after `migrate up` and `seed`. Do not maintain a second schema SQL snapshot.

## URM Bypass

The seed uses non-zero prices so local smoke calls exercise URM `Freeze -> Confirm/Cancel` against `http://127.0.0.1:6900`.

If URM is not running, temporarily bypass settlement by setting local prices to zero:

```sql
UPDATE ai_model_prices
SET platform_input_price_per_1m = 0,
    platform_output_price_per_1m = 0,
    platform_image_price = 0,
    tenant_input_price_per_1m = 0,
    tenant_output_price_per_1m = 0,
    tenant_image_price = 0
WHERE model_id IN (
  SELECT id FROM ai_models
  WHERE model_code IN ('local-chat-test', 'local-responses-test', 'local-embedding-test', 'local-image-test')
);
```

Re-run `db/local_seed.sql` to restore the URM-on smoke data.
