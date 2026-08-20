# 审计持久可靠性边界

AI 请求审计使用 PostgreSQL `ai_audit_inbox` 作为 durable inbox。正常完成的请求由
用量完成事务同时写入用量事实、账务事实和审计 inbox；认证、路由或配额阶段提前失败的
请求则在最终处理阶段独立入队。请求线程只负责把 JSONB envelope 交给 PostgreSQL，正文
物化和媒体 blob 写入由后台 worker 完成。

worker 使用 `FOR UPDATE SKIP LOCKED` 领取 `pending` 行，并写入租约标识和领取次数。
进程崩溃或租约过期后，其他实例可以重新领取；写入 payload 和删除 inbox 行在同一事务中
完成，`request_id` 唯一约束保证重试不会产生重复审计记录。

媒体或 payload 写入失败会保留 `last_error`，按指数退避重新进入 `pending`；达到最大
尝试次数后进入 `dead`，不会静默丢失。队列待处理量、最老待处理时长、失败和 dead 数量
通过 `/metrics` 暴露，dead 行应由运维确认根因后再人工重置为 `pending`。

schema 14 的升级脚本是
`internal/db/changes/0014_20260820_durable_audit_inbox.sql`；生产发布必须先完成该
迁移，再启动新版本进程。
