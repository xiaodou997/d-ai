# Schema 发布校验与恢复手册

`deploy/production/schema_release.sh`（构建附件中的 `release/sql/schema_release.sh`）是唯一推荐的发布期 schema 执行入口。
应用启动只验证 `dai_schema_metadata.version`，不会调用它、不会执行 DDL，也不会隐式升级生产库。

当前仓库基线为 schema v30。发布制品应先运行 `scripts/generate_release_metadata.sh release` 生成
SBOM/provenance/checksum，再将同一 `release/sql/` 与二进制交给本手册执行；部署后的
`/health`、`/ready`、Portal、API 和流式 smoke 由 `scripts/smoke_release.sh` 执行。

## 发布前不变量

- 先停止全部旧版本应用实例；迁移期间不允许旧代码继续写入已变更的列或表。
- 发布附件必须来自同一次构建：`SCHEMA_SQL_DIR=release/sql`，不要把工作区的 SQL 与二进制混用。
- 迁移前必须能恢复完整 PostgreSQL 备份；没有可验证的备份就停止发布。
- 迁移脚本按连续目标版本逐个执行，任何一步失败都停止，应用保持下线。
- 新应用只在目标 schema 版本和 readiness 检查都通过后放量；旧应用不能与新 schema 混跑。

## 标准流程

在维护窗口执行，数据库连接通过 secret manager 注入，不要把密码写进 shell 历史：

```bash
export SCHEMA_DATABASE_URL='postgres://dai:${PASSWORD}@db.example/dai?sslmode=require'
export SCHEMA_SQL_DIR="$PWD/release/sql"
export SCHEMA_BACKUP_DIR=/var/backups/dai/schema

# 只读：确认当前版本、目标版本、待执行脚本，并确认没有其他数据库会话
deploy/production/schema_release.sh preflight

# 显式确认后才会创建备份并执行迁移
export SCHEMA_RELEASE_CONFIRM=APPLY
deploy/production/schema_release.sh migrate

# 启动新二进制后再验证 schema 和管理 readiness
export SCHEMA_RELEASE_HEALTH_URL='http://127.0.0.1:19642/ready'
deploy/production/schema_release.sh verify
```

`migrate` 会在执行 SQL 前创建 PostgreSQL custom-format dump，并在同一目录写入：

- `database.dump`：恢复用完整备份；
- `SHA256SUMS`：备份完整性校验；
- `SQL_SHA256SUMS`：本次待执行 SQL 文件哈希；
- `MIGRATION.txt`：源/目标版本、SQL 目录和恢复命令记录。

默认情况下，脚本发现任何其他 client session 都会拒绝迁移。
`SCHEMA_RELEASE_ALLOW_ACTIVE_SESSIONS=1` 只允许在隔离 rehearsal 中绕过，不能用于生产窗口。

## 失败与恢复

迁移脚本自身在事务内执行，失败后当前脚本会回滚；不要因此跳过备份或直接改
`dai_schema_metadata.version`。失败时：

1. 保持应用停止，保留失败数据库和 `MIGRATION.txt` 供排查；
2. 校验 `SHA256SUMS`，把 `database.dump` 恢复到隔离数据库并先验证表结构/版本；
3. 生产恢复使用 DBA 审批的 `pg_restore --clean --if-exists` 或整库替换方案，恢复后重新查询 schema 版本；
4. 只有恢复结果、应用版本和 readiness 都通过，才重新放量。

已经恢复流量后不要使用历史 SQL 回滚脚本。当前仅 `0003_rollback.sql` 在明确的“无流量、无新结算记录”窗口内可用；其他版本统一依赖备份恢复，详见 `internal/db/rollback/README.md`。

## 兼容窗口与发布后观察

- 迁移前记录当前 schema 版本、应用版本和备份校验值。
- 迁移后先启动单个新实例，确认 `/health`、管理 `/ready`、日志中的 schema verified 和关键数据库查询，再扩大副本数。
- 观察 outbox、scheduler、支付补偿和错误率；发现 schema/数据不变量异常立即停止放量并进入恢复流程。
- 发布记录至少保留：SQL 文件哈希、备份哈希、源/目标 schema 版本、执行人、窗口开始/结束时间和 readiness 结果。
- 可通过 `SCHEMA_RELEASE_APP_VERSION` 把应用构建版本写入 `MIGRATION.txt`，避免 schema 与二进制版本无法关联。

## 本地 rehearsal

```bash
bash -n deploy/production/schema_release.sh
deploy/production/schema_release.sh --help
go run ./cmd/checkschema
SCHEMA_REPLAY_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/dai_v29_test?sslmode=disable' \
  bash scripts/replay_schema_chain.sh
```

真实数据库 rehearsal 必须使用临时 PostgreSQL 实例和临时备份目录，不能把生产 URL
交给本地脚本。迁移链专项 PostgreSQL 测试由 `go test ./internal/db` 覆盖；发布前仍需
在目标 PostgreSQL 版本上执行一次完整备份/恢复演练。
