-- 2026-05-24-api-credits-unit-cleanup.sql
-- API 积分单位统一重构：清空历史数据
--
-- 本次重构后，后端 API 边界统一使用「积分」(float64)，
-- 数据库继续使用「微积分」(int64, 1积分=10000微积分)。
--
-- ⚠️  此文件仅在「清空数据库」场景下执行！
-- ⚠️  用户已确认将清空数据库，因此无需做数据迁移（×10000 / ÷10000）。
-- ⚠️  只需清空所有含价格/quota数据的表即可。
--
-- 执行方式：psql -f this_file.sql

BEGIN;

-- 1. 清空价格相关表
TRUNCATE ai_model_prices CASCADE;
TRUNCATE ai_tenant_model_price_overrides CASCADE;
TRUNCATE ai_tenant_user_prices CASCADE;

-- 2. 清空 API Key（含 quota 数据）
TRUNCATE ai_api_keys CASCADE;

-- 3. 清空使用记录（含 *_cost 微积分数据）
TRUNCATE ai_usage_logs CASCADE;
TRUNCATE ai_usage_rollups_hourly CASCADE;

-- 4. 清空 ledger（含 pending_*_micro 数据）
TRUNCATE ai_user_credit_ledger CASCADE;

-- 注意：以下表无需清空（不含积分/价格字段）：
--   ai_providers, ai_provider_endpoints, ai_models, ai_model_routes,
--   ai_upstream_deployments, ai_tenant_model_grants, ai_runtime_limit_policies,
--   ai_audit_logs, ai_credential_pools, ai_pool_credentials

COMMIT;
