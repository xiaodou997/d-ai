# 数据库人工变更

D-AI 使用 `../init.sql` 维护最新完整基线，当前 schema 版本为 `32`。
本目录保存首次发布后的单向升级 SQL，生产发布时必须从当前版本开始按编号连续执行。

## 文件命名

文件名格式为 `NNNN_YYYYMMDD_description.sql`，其中 `NNNN` 是执行后的目标版本：

```text
0002_20260810_add_account_internal_note.sql
0003_20260815_add_payment_order_index.sql
```

从版本 A 升级到版本 B 时，按版本号顺序人工执行 `A+1` 到 `B` 的全部脚本。

## SQL 要求

- 只使用 PostgreSQL SQL 和 PL/pgSQL，不使用迁移框架标记或 `psql` 元命令。
- 每个文件必须写明来源版本、目标版本、日期和描述。
- 默认使用单个事务，开头校验来源版本，最后更新目标版本和 `updated_at`。
- 已发布脚本不可修改；修复必须新增更高版本脚本。
- 不使用 `IF EXISTS` 或 `IF NOT EXISTS` 掩盖结构漂移，确有兼容需要时必须说明原因。
- 同一个改动必须同步更新 `../init.sql` 和 `internal/db/schema.go` 中的期望版本。
- 发布前分别在空数据库和目标版本的数据库副本上验证。

模板：

```sql
-- from_version: 1
-- to_version: 2
-- created_at: 2026-08-10
-- description: add account internal note

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 1
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 1';
    END IF;
END
$$;

ALTER TABLE iam_accounts
    ADD COLUMN internal_note TEXT NOT NULL DEFAULT '';

UPDATE dai_schema_metadata
SET version = 2,
    updated_at = now()
WHERE singleton = TRUE AND version = 1;

COMMIT;
```

应用不会执行这些脚本。生产发布时，从发布附件的 `sql/changes/` 取出所需文件，
使用数据库连接工具在维护窗口人工执行。
