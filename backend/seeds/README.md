# Local Seeds

Manual initialization for a local PostgreSQL database on port `5432`.

Use the built-in Go migration command when `psql` is not installed:

```bash
UNI_AI_API_POSTGRES_DSN='postgres://<user>:<password>@127.0.0.1:5432/uni_ai_api?sslmode=disable&search_path=public' \
UNI_AI_API_PROVIDER_KEY_MASTER='change-me-32-byte-minimum-secret' \
go run ./cmd/migrate up
```

Or run the SQL manually with `psql`:

```bash
createdb -h 127.0.0.1 -p 5432 -U postgres uni_ai_api
sed '/^-- +goose Down$/,$d' backend/migrations/00001_init.sql | \
  psql 'postgres://<user>:<password>@127.0.0.1:5432/uni_ai_api?sslmode=disable&search_path=public'
```

Current local DSN:

```text
postgres://<user>:<password>@127.0.0.1:5432/uni_ai_api?sslmode=disable&search_path=public
```

The database name is `uni_ai_api`. The schema is controlled by PostgreSQL `search_path`; use `search_path=public` for the default schema or replace it with another schema name after creating that schema.

Do not run the whole goose migration file directly with `psql -f`; it also contains the Down section.

Edit `backend/seeds/local_dev.sql` and replace provider keys and Ali model names:

- `REPLACE_ME_DEEPSEEK_API_KEY`
- `REPLACE_ME_ALI_CODING_PLAN_API_KEY`
- `REPLACE_ME_ALI_TOKEN_PLAN_API_KEY`
- `REPLACE_ME_ALI_CODING_PLAN_MODEL`
- `REPLACE_ME_ALI_TOKEN_PLAN_MODEL`

Then seed local providers, endpoints, models, deployments, grants, and one tenant runtime key:

Use the built-in Go seed command when `psql` is not installed:

```bash
UNI_AI_API_POSTGRES_DSN='postgres://<user>:<password>@127.0.0.1:5432/uni_ai_api?sslmode=disable&search_path=public' \
UNI_AI_API_PROVIDER_KEY_MASTER='change-me-32-byte-minimum-secret' \
go run ./cmd/seed
```

Or run the SQL manually with `psql`:

```bash
psql 'postgres://<user>:<password>@127.0.0.1:5432/uni_ai_api?sslmode=disable&search_path=public' \
  -f backend/seeds/local_dev.sql
```

The seeded runtime key is:

```text
sk-ai-local-dev
```

Useful checks:

```bash
curl http://127.0.0.1:13010/v1/models \
  -H 'Authorization: Bearer sk-ai-local-dev'

curl http://127.0.0.1:13010/v1/chat/completions \
  -H 'Authorization: Bearer sk-ai-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "hello"}],
    "stream": false
  }'
```

DashScope Coding Plan and Token Plan OpenAI-compatible endpoints are seeded as separate endpoints:

| Endpoint | Base URL |
| --- | --- |
| DashScope Coding Plan OpenAI | `https://coding.dashscope.aliyuncs.com/v1` |
| DashScope Token Plan OpenAI | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` |

Their Anthropic-compatible endpoints are seeded for later adapter support, but runtime forwarding currently supports only `openai_chat_completions`.
