# AI Request Payloads 归档方案

> **版本**: v3 — 普通表 + 存储过程归档  
> **最后更新**: 2026-05-21

---

## 概述

`ai_request_payloads` 表用于存储 AI 请求的审计日志（请求/响应正文、协议信息等）。数据特点是**写多读少、只查近期、历史可归档**。

v3 方案将表设计为**普通表**，业务代码只管 INSERT/SELECT，归档由 PostgreSQL 存储过程 + 定时任务处理，实现**业务与运维解耦**。

---

## 架构对比

| | v2（已废弃） | v3（当前） |
|---|---|---|
| 表类型 | 分区表（PARTITION BY RANGE） | 普通表 |
| 分区/归档管理 | Go 代码（Partitioner） | PostgreSQL 存储过程 |
| 启动依赖 | 父表必须是分区表，否则 WARN | 无依赖 |
| 代码耦合 | 业务代码需维护 Partitioner | 业务代码无感知 |
| 运维调整 | 改代码 + 重新部署 | 改 crontab 或存储过程参数即可 |

---

## 归档策略

- **保留周期**: 主表保留最近 6 个月数据
- **归档粒度**: 逐月归档，每个月一张归档表
- **归档表命名**: `ai_request_payloads_archive_YYYY_MM`（如 `ai_request_payloads_archive_2026_01`）
- **归档后操作**: 已归档数据从主表**删除**，主表始终只保留近期数据

---

## 新版本部署操作

v3 变更了 `ai_request_payloads` 表结构（分区表 → 普通表），部署新版本时需执行以下操作：

### 1. 删除旧表

> ⚠️ `init.sql` 只负责建表，不包含删表逻辑。旧表需手动删除。

```sql
DROP TABLE IF EXISTS ai_request_payloads;
```

如果是分区表，会连带删除所有子分区。

### 2. 重新执行 init.sql

```bash
docker exec -i postgres psql -U postgres -d devdb < ai-service/db/init.sql
```

这会创建新的普通表 + 归档存储过程。

### 3. 重启 ai-service

代码中已移除 Partitioner，重启后不再有 `partition pre-check failed` 的 WARN 日志。

### 4. 配置定时归档（可选，建议配置）

使用宿主机 crontab 调度，每月 1 号凌晨 3 点执行：

```bash
crontab -e

# 添加以下行（根据实际容器名和数据库名调整）
0 3 1 * * docker exec postgres psql -U postgres -d devdb -c "SELECT archive_request_payloads(6);" >> /var/log/payload-archive.log 2>&1
```

---

## 归档使用方法

### 手动执行归档

```sql
-- 默认保留 6 个月
SELECT archive_request_payloads();

-- 自定义保留月数（如保留 3 个月）
SELECT archive_request_payloads(3);
```

返回结果示例：

| archived_table | rows_moved |
|---|---|
| ai_request_payloads_archive_2026_01 | 15234 |
| ai_request_payloads_archive_2026_02 | 12876 |

### 查询历史数据

```sql
-- 查询 2026 年 1 月的历史数据
SELECT * FROM ai_request_payloads_archive_2026_01
WHERE request_id = 'req_xxx';

-- 跨月查询
SELECT * FROM ai_request_payloads_archive_2026_01
UNION ALL
SELECT * FROM ai_request_payloads_archive_2026_02
WHERE request_model = 'gpt-4o';
```

### 给归档表加索引（按需）

归档表创建时不带索引以节省空间，需要频繁查询时按需添加：

```sql
CREATE INDEX idx_arp_archive_2026_01_request_id
  ON ai_request_payloads_archive_2026_01 (request_id);
```

### 清理归档表

```sql
DROP TABLE ai_request_payloads_archive_2026_01;
```

---

## FAQ

**Q: 归档过程中会影响业务写入吗？**  
A: 归档是在历史数据上操作的（6个月前），和当前写入的数据不冲突，锁冲突极小。建议在低峰期执行（如凌晨3点）。

**Q: 忘记执行归档会怎样？**  
A: 主表数据会越来越多，查询逐渐变慢，但不会导致错误。几百万行对 PostgreSQL 完全不是问题。

**Q: 可以调整保留周期吗？**  
A: 可以，只需修改 crontab 中的参数：`archive_request_payloads(3)` 改为 `archive_request_payloads(12)` 即可保留12个月。无需改代码、无需重新部署。

**Q: 归档表占用多少磁盘空间？**  
A: 取决于业务量。AI 审计日志通常每条几 KB 到几十 KB，月均百万条约 1-5 GB。归档表是冷数据，不占数据库缓存。
