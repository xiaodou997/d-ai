-- from_version: 40
-- to_version: 41
-- created_at: 2026-09-14
-- description: independent execution records and financial settlement evidence
BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM dai_schema_metadata WHERE singleton AND version=40) THEN
  RAISE EXCEPTION 'expected D-AI schema version 40';
 END IF;
 IF EXISTS(SELECT 1 FROM bill_charge_outbox WHERE status <> 'done') THEN
  RAISE EXCEPTION 'drain or reconcile legacy charges before cutover';
 END IF;
END $$;
-- Request records and financial evidence have independent retention horizons.
CREATE TABLE bill_record_control (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 epoch uuid NOT NULL DEFAULT gen_random_uuid(),
 accepting boolean NOT NULL DEFAULT true,
 closed_before timestamptz NOT NULL DEFAULT '-infinity',
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO bill_record_control(singleton,accepting) VALUES(true,false);

CREATE TABLE ai_request_keys (
 request_id text PRIMARY KEY,
 registered_at timestamptz NOT NULL DEFAULT now(),
 epoch uuid NOT NULL,
 lease_until timestamptz NOT NULL,
 sealed_at timestamptz,
 tenant_id text NOT NULL DEFAULT '',
 user_id text NOT NULL DEFAULT ''
);
CREATE INDEX ai_request_keys_live ON ai_request_keys(lease_until) WHERE sealed_at IS NULL;

CREATE TABLE ai_requests (
 created_at timestamptz NOT NULL,
 request_id text NOT NULL,
 tenant_id text NOT NULL DEFAULT '',
 user_id text NOT NULL DEFAULT '',
 api_key_id text NOT NULL DEFAULT '',
 model_code text NOT NULL DEFAULT '',
 request_source text NOT NULL DEFAULT '',
 end_reason text NOT NULL DEFAULT 'running',
 delivery_state text NOT NULL DEFAULT 'unknown',
 is_error boolean NOT NULL DEFAULT false,
 completed_at timestamptz,
 facts jsonb NOT NULL DEFAULT '{}',
 PRIMARY KEY(created_at,request_id)
) PARTITION BY RANGE(created_at);
CREATE INDEX ai_requests_tenant_page ON ai_requests(tenant_id,created_at DESC,request_id DESC);
CREATE INDEX ai_requests_user_page ON ai_requests(user_id,created_at DESC,request_id DESC);
CREATE INDEX ai_requests_error_page ON ai_requests(is_error,created_at DESC,request_id DESC);

CREATE TABLE ai_request_attempts (
 created_at timestamptz NOT NULL,
 request_id text NOT NULL,
 ordinal integer NOT NULL,
 facts jsonb NOT NULL,
 PRIMARY KEY(created_at,request_id,ordinal)
) PARTITION BY RANGE(created_at);

CREATE TABLE ai_request_errors (
 created_at timestamptz NOT NULL,
 request_id text NOT NULL,
 origin text NOT NULL,
 stage text NOT NULL,
 code text NOT NULL,
 message text NOT NULL DEFAULT '',
 internal_detail text NOT NULL DEFAULT '',
 PRIMARY KEY(created_at,request_id)
) PARTITION BY RANGE(created_at);

CREATE TABLE bill_settlements (
 created_at timestamptz NOT NULL,
 request_id text NOT NULL,
 tenant_id text NOT NULL,
 user_id text NOT NULL DEFAULT '',
 api_key_id text NOT NULL DEFAULT '',
 subscription_id text NOT NULL DEFAULT '',
 billing_source text NOT NULL DEFAULT 'payg',
 sealed_at timestamptz NOT NULL DEFAULT now(),
 delivery_interrupted boolean NOT NULL DEFAULT false,
 state text NOT NULL CHECK(state IN ('pending','posted','waived','review','refunded')),
 reason text NOT NULL,
 tenant_due bigint NOT NULL DEFAULT 0 CHECK(tenant_due>=0),
 user_due bigint NOT NULL DEFAULT 0 CHECK(user_due>=0),
 key_due bigint NOT NULL DEFAULT 0 CHECK(key_due>=0),
 subscription_due bigint NOT NULL DEFAULT 0 CHECK(subscription_due>=0),
 tenant_charged bigint NOT NULL DEFAULT 0 CHECK(tenant_charged>=0),
 user_charged bigint NOT NULL DEFAULT 0 CHECK(user_charged>=0),
 evidence jsonb NOT NULL DEFAULT '{}',
 pricing jsonb NOT NULL DEFAULT '{}',
 dimensions jsonb NOT NULL DEFAULT '{}',
 subscription_windows jsonb NOT NULL DEFAULT '{}',
 attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '',
 posted_at timestamptz,
 refunded_at timestamptz,
 refund_reason text NOT NULL DEFAULT '',
 refund_operator text NOT NULL DEFAULT '',
 PRIMARY KEY(created_at,request_id),
 CHECK(state NOT IN ('posted','refunded') OR posted_at IS NOT NULL),
 CHECK(state <> 'waived' OR (tenant_due=0 AND user_due=0 AND key_due=0 AND subscription_due=0)),
 CHECK(tenant_charged<=tenant_due AND user_charged<=user_due),
 CHECK(state IN ('posted','refunded') OR (tenant_charged=0 AND user_charged=0)),
 CHECK(state NOT IN ('posted','refunded') OR (tenant_charged=tenant_due AND user_charged=user_due)),
 CHECK(state <> 'refunded' OR refunded_at IS NOT NULL)
) PARTITION BY RANGE(created_at);
CREATE INDEX bill_settlements_work ON bill_settlements(available_at,created_at) WHERE state='pending';
CREATE INDEX bill_settlements_tenant_page ON bill_settlements(tenant_id,created_at DESC,request_id DESC);
CREATE INDEX bill_settlements_user_page ON bill_settlements(user_id,created_at DESC,request_id DESC);

CREATE TABLE bill_journal (
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 operation_id text NOT NULL,
 account_id text NOT NULL,
 meter text NOT NULL CHECK(meter IN ('balance','api_key','subscription_total','subscription_5h','subscription_7d')),
 delta bigint NOT NULL,
 before_value bigint NOT NULL,
 after_value bigint NOT NULL,
 reason text NOT NULL,
 PRIMARY KEY(created_at,id),
 CHECK(after_value=before_value+delta)
) PARTITION BY RANGE(created_at);
CREATE INDEX bill_journal_account ON bill_journal(account_id,meter,created_at,id);
CREATE INDEX bill_journal_operation ON bill_journal(operation_id,created_at);

CREATE TABLE bill_checkpoints (
 account_id text NOT NULL,
 meter text NOT NULL,
 through_at timestamptz NOT NULL,
 value bigint NOT NULL,
 source text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,meter,through_at)
);
CREATE TABLE bill_daily_totals (
 day date NOT NULL,
 tenant_id text NOT NULL,
 user_id text NOT NULL DEFAULT '',
 requests bigint NOT NULL DEFAULT 0,
 errors bigint NOT NULL DEFAULT 0,
 interruptions bigint NOT NULL DEFAULT 0,
 prompt_tokens bigint NOT NULL DEFAULT 0,
 completion_tokens bigint NOT NULL DEFAULT 0,
 tenant_charged bigint NOT NULL DEFAULT 0,
 user_charged bigint NOT NULL DEFAULT 0,
 tenant_refunded bigint NOT NULL DEFAULT 0,
 user_refunded bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(day,tenant_id,user_id)
);

CREATE TABLE ai_debug_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 tenant_id text NOT NULL DEFAULT '',
 api_key_id text NOT NULL DEFAULT '',
 model_code text NOT NULL DEFAULT '',
 created_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '24 hours'),
 CHECK(tenant_id<>'' OR api_key_id<>'' OR model_code<>'')
);
CREATE TABLE ai_request_debug_payloads (
 created_at timestamptz NOT NULL,
 request_id text NOT NULL,
 session_id uuid NOT NULL,
 content_gzip bytea NOT NULL,
 original_bytes bigint NOT NULL,
 truncated boolean NOT NULL,
 PRIMARY KEY(created_at,request_id),
 CHECK(octet_length(content_gzip)<=1100000)
) PARTITION BY RANGE(created_at);

-- Identifiers are selected from a closed list; partitions are created ahead of
-- time. There is no DEFAULT partition hiding a failed maintenance worker.
CREATE FUNCTION ensure_request_record_partitions(at_time timestamptz) RETURNS void LANGUAGE plpgsql AS $$
DECLARE base text; lo timestamptz; hi timestamptz; suffix text; n integer;
BEGIN
 FOR base IN SELECT unnest(ARRAY['ai_requests','bill_settlements','bill_journal','ai_request_attempts','ai_request_errors','ai_request_debug_payloads']) LOOP
  FOR n IN -1..2 LOOP
   IF base IN ('ai_requests','bill_settlements','bill_journal') THEN
    lo := date_trunc('month',at_time AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' + n*interval '1 month';
    hi := lo+interval '1 month'; suffix := to_char(lo AT TIME ZONE 'UTC','YYYYMM');
   ELSE
    lo := date_trunc('day',at_time AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' + n*interval '1 day';
    hi := lo+interval '1 day'; suffix := to_char(lo AT TIME ZONE 'UTC','YYYYMMDD');
   END IF;
   EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',base||'_'||suffix,base,lo,hi);
  END LOOP;
 END LOOP;
END $$;
SELECT ensure_request_record_partitions(now());

-- All existing balance writers (payment callbacks, expiry and administrative
-- grants included) participate without duplicating accounting implementations.
CREATE FUNCTION journal_account_balance() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE op text; kind text;
BEGIN
 IF NEW.balance_micro=OLD.balance_micro THEN RETURN NEW; END IF;
 op := NULLIF(current_setting('dai.billing_operation',true),'');
 kind := NULLIF(current_setting('dai.billing_reason',true),'');
 INSERT INTO bill_journal(operation_id,account_id,meter,delta,before_value,after_value,reason)
 VALUES(COALESCE(op,'balance:'||txid_current()::text),NEW.account_id,'balance',NEW.balance_micro-OLD.balance_micro,OLD.balance_micro,NEW.balance_micro,COALESCE(kind,'balance_adjustment'));
 RETURN NEW;
END $$;
CREATE TRIGGER bill_accounts_journal AFTER UPDATE OF balance_micro ON bill_accounts FOR EACH ROW EXECUTE FUNCTION journal_account_balance();
CREATE FUNCTION checkpoint_new_account() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source)
 VALUES(NEW.account_id,'balance',clock_timestamp(),NEW.balance_micro,'account_opening');
 RETURN NEW;
END $$;
CREATE TRIGGER bill_accounts_opening AFTER INSERT ON bill_accounts FOR EACH ROW EXECUTE FUNCTION checkpoint_new_account();
INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source)
SELECT account_id,'balance',now(),balance_micro,'cutover_opening' FROM bill_accounts;

CREATE FUNCTION journal_quota_meter() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
DECLARE names text[]; meters text[]; i integer; before_n bigint; after_n bigint; op text; why text;
BEGIN
 IF TG_TABLE_NAME='ai_api_keys' THEN names:=ARRAY['quota_used'];meters:=ARRAY['api_key'];
 ELSE names:=ARRAY['total_used_micro','win5h_used_micro','win7d_used_micro'];meters:=ARRAY['subscription_total','subscription_5h','subscription_7d'];END IF;
 op:=COALESCE(NULLIF(current_setting('dai.billing_operation',true),''),'quota:'||txid_current()::text);
 why:=COALESCE(NULLIF(current_setting('dai.billing_reason',true),''),'quota_adjustment');
 FOR i IN 1..array_length(names,1) LOOP
  after_n:=(to_jsonb(NEW)->>names[i])::bigint;
  IF TG_OP='INSERT' THEN
   INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source) VALUES(NEW.id::text,meters[i],clock_timestamp(),after_n,'meter_opening');
  ELSE
   before_n:=(to_jsonb(OLD)->>names[i])::bigint;
   IF before_n<>after_n THEN INSERT INTO bill_journal(operation_id,account_id,meter,delta,before_value,after_value,reason) VALUES(op,NEW.id::text,meters[i],after_n-before_n,before_n,after_n,why); END IF;
  END IF;
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_api_keys_journal AFTER INSERT OR UPDATE OF quota_used ON ai_api_keys FOR EACH ROW EXECUTE FUNCTION journal_quota_meter();
CREATE TRIGGER ai_subscriptions_journal AFTER INSERT OR UPDATE OF total_used_micro,win5h_used_micro,win7d_used_micro ON ai_sub_subscriptions FOR EACH ROW EXECUTE FUNCTION journal_quota_meter();
INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source) SELECT id::text,'api_key',now(),quota_used,'cutover_opening' FROM ai_api_keys;
INSERT INTO bill_checkpoints(account_id,meter,through_at,value,source)
 SELECT id::text,meter,now(),value,'cutover_opening' FROM ai_sub_subscriptions CROSS JOIN LATERAL (VALUES('subscription_total',total_used_micro),('subscription_5h',win5h_used_micro),('subscription_7d',win7d_used_micro)) m(meter,value);

-- Read-only projection for cross-domain dashboards. All financial amounts
-- come from the independent settlement, never from a request log.

-- Financial evidence can only be retired by verified partition lifecycle.
CREATE FUNCTION protect_bill_journal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'financial journal is append-only'; END $$;
CREATE TRIGGER bill_journal_immutable BEFORE UPDATE OR DELETE ON bill_journal FOR EACH ROW EXECUTE FUNCTION protect_bill_journal();
CREATE FUNCTION protect_settlement_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.created_at,NEW.request_id,NEW.tenant_id,NEW.user_id,NEW.api_key_id,NEW.subscription_id,NEW.billing_source,NEW.sealed_at,NEW.reason,NEW.tenant_due,NEW.user_due,NEW.key_due,NEW.subscription_due,NEW.evidence,NEW.pricing,NEW.dimensions)
 IS DISTINCT FROM ROW(OLD.created_at,OLD.request_id,OLD.tenant_id,OLD.user_id,OLD.api_key_id,OLD.subscription_id,OLD.billing_source,OLD.sealed_at,OLD.reason,OLD.tenant_due,OLD.user_due,OLD.key_due,OLD.subscription_due,OLD.evidence,OLD.pricing,OLD.dimensions)
 THEN RAISE EXCEPTION 'sealed settlement evidence is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER bill_settlement_evidence_immutable BEFORE UPDATE ON bill_settlements FOR EACH ROW EXECUTE FUNCTION protect_settlement_evidence();

CREATE VIEW ai_consumption_projection AS
SELECT
 (md5(s.request_id)::uuid)::UUID AS id,
 (s.request_id)::TEXT AS request_id,
 (COALESCE((s.dimensions->>'trace_id')::TEXT,''))::TEXT AS trace_id,
 (NULLIF(s.api_key_id,'')::uuid)::UUID AS api_key_id,
 (COALESCE((s.dimensions->>'key_owner_type')::TEXT,''))::TEXT AS key_owner_type,
 (COALESCE((s.dimensions->>'auth_method')::TEXT,''))::TEXT AS auth_method,
 (COALESCE((s.dimensions->>'request_source')::TEXT,''))::TEXT AS request_source,
 (s.tenant_id)::TEXT AS tenant_id,
 (NULLIF(s.user_id,''))::TEXT AS user_id,
 (COALESCE((s.dimensions->>'client_user_agent')::TEXT,''))::TEXT AS client_user_agent,
 (COALESCE((s.dimensions->>'external_user_id')::TEXT,''))::TEXT AS external_user_id,
 ((s.dimensions->>'group_id')::UUID)::UUID AS group_id,
 (COALESCE((s.dimensions->>'group_name_snapshot')::TEXT,''))::TEXT AS group_name_snapshot,
 (COALESCE((s.dimensions->>'billing_group_label_snapshot')::TEXT,''))::TEXT AS billing_group_label_snapshot,
 (COALESCE((s.dimensions->>'model_code')::TEXT,''))::TEXT AS model_code,
 (COALESCE((s.dimensions->>'requested_model')::TEXT,''))::TEXT AS requested_model,
 ((s.dimensions->>'matched_dispatch_rule_id')::UUID)::UUID AS matched_dispatch_rule_id,
 (COALESCE((s.dimensions->>'matched_dispatch_rule_summary')::TEXT,''))::TEXT AS matched_dispatch_rule_summary,
 (COALESCE((s.dimensions->>'resolved_logical_model')::TEXT,''))::TEXT AS resolved_logical_model,
 (COALESCE((s.dimensions->>'resolved_provider_family')::TEXT,''))::TEXT AS resolved_provider_family,
 (COALESCE((s.dimensions->>'capability_type')::TEXT,''))::TEXT AS capability_type,
 ((s.dimensions->>'group_target_id')::UUID)::UUID AS group_target_id,
 ((s.dimensions->>'upstream_account_id')::UUID)::UUID AS upstream_account_id,
 ((s.dimensions->>'endpoint_id')::UUID)::UUID AS endpoint_id,
 (s.dimensions->>'provider_code')::TEXT AS provider_code,
 (COALESCE((s.dimensions->>'upstream_model')::TEXT,''))::TEXT AS upstream_model,
 (COALESCE((s.dimensions->>'provider_format')::TEXT,''))::TEXT AS provider_format,
 (COALESCE((s.dimensions->>'conversation_id')::TEXT,''))::TEXT AS conversation_id,
 (COALESCE((s.dimensions->>'stream')::BOOLEAN,false))::BOOLEAN AS stream,
 (COALESCE((s.dimensions->>'prompt_tokens')::INTEGER,0))::INTEGER AS prompt_tokens,
 (COALESCE((s.dimensions->>'completion_tokens')::INTEGER,0))::INTEGER AS completion_tokens,
 (COALESCE((s.dimensions->>'cache_write_tokens')::INTEGER,0))::INTEGER AS cache_write_tokens,
 (COALESCE((s.dimensions->>'cache_read_tokens')::INTEGER,0))::INTEGER AS cache_read_tokens,
 (COALESCE((s.dimensions->>'reasoning_tokens')::INTEGER,0))::INTEGER AS reasoning_tokens,
 (COALESCE((s.dimensions->>'reasoning_effort')::TEXT,''))::TEXT AS reasoning_effort,
 (COALESCE((s.dimensions->>'total_tokens')::INTEGER,0))::INTEGER AS total_tokens,
 (COALESCE((s.dimensions->>'billable_unit_type')::TEXT,''))::TEXT AS billable_unit_type,
 (COALESCE((s.dimensions->>'billable_units')::BIGINT,0))::BIGINT AS billable_units,
 (COALESCE((s.evidence->>'reference_cost_micro')::bigint,0))::BIGINT AS catalog_base,
 (s.tenant_charged)::BIGINT AS tenant_payable,
 (COALESCE((s.dimensions->>'retail_base')::BIGINT,0))::BIGINT AS retail_base,
 (s.user_due)::BIGINT AS user_payable,
 (s.user_charged)::BIGINT AS user_charged,
 (CASE WHEN s.state IN ('posted','refunded') THEN s.key_due ELSE 0 END)::BIGINT AS api_key_quota_cost,
 (COALESCE((s.dimensions->>'service_tier')::TEXT,''))::TEXT AS service_tier,
 (jsonb_build_object('usage_evidence',s.evidence,'pricing',s.pricing,'reason',s.reason))::JSONB AS billing_breakdown,
 (COALESCE((s.dimensions->>'billing_window_id')::TEXT,''))::TEXT AS billing_window_id,
 ((s.dimensions->>'settlement_batch_id')::UUID)::UUID AS settlement_batch_id,
 (s.posted_at)::TIMESTAMPTZ AS settled_at,
 (CASE s.state WHEN 'posted' THEN 'settled' WHEN 'refunded' THEN 'settled' WHEN 'review' THEN 'failed' WHEN 'waived' THEN 'void' ELSE 'pending' END)::TEXT AS billing_status,
 (s.last_error)::TEXT AS settlement_error,
 (CASE WHEN s.state='refunded' THEN 'refunded' ELSE 'none' END)::TEXT AS refund_status,
 (s.refund_reason)::TEXT AS refund_reason,
 (s.refund_operator)::TEXT AS refund_operator_id,
 (s.refunded_at)::TIMESTAMPTZ AS refunded_at,
 (CASE WHEN COALESCE(r.is_error,s.reason IN ('request_error_waived','unconfirmed_execution')) THEN 'failed' ELSE 'success' END)::TEXT AS request_status,
 (COALESCE((s.dimensions->>'http_status')::INTEGER,0))::INTEGER AS http_status,
 (s.dimensions->>'upstream_status')::INTEGER AS upstream_status,
 (COALESCE((s.dimensions->>'latency_ms')::INTEGER,0))::INTEGER AS latency_ms,
 (COALESCE((s.dimensions->>'first_token_latency_ms')::INTEGER,0))::INTEGER AS first_token_latency_ms,
 (COALESCE((s.dimensions->>'request_total_ms')::INTEGER,0))::INTEGER AS request_total_ms,
 (COALESCE((s.dimensions->>'request_setup_ms')::INTEGER,0))::INTEGER AS request_setup_ms,
 (COALESCE((s.dimensions->>'first_response_byte_ms')::INTEGER,0))::INTEGER AS first_response_byte_ms,
 (COALESCE((s.dimensions->>'response_tail_ms')::INTEGER,0))::INTEGER AS response_tail_ms,
 (COALESCE((s.dimensions->>'final_attempt_header_ms')::INTEGER,0))::INTEGER AS final_attempt_header_ms,
 (COALESCE((s.dimensions->>'final_attempt_total_ms')::INTEGER,0))::INTEGER AS final_attempt_total_ms,
 (e.code)::TEXT AS error_code,
 (e.message)::TEXT AS error_message,
 ((s.dimensions->>'oauth_credential_id')::UUID)::UUID AS oauth_credential_id,
 ((s.dimensions->>'credential_pool_id')::UUID)::UUID AS credential_pool_id,
 (COALESCE((s.dimensions->>'attempts_count')::INT,0))::INT AS attempts_count,
 ((s.dimensions->>'final_route_id')::UUID)::UUID AS final_route_id,
 (COALESCE((s.dimensions->>'client_protocol')::TEXT,''))::TEXT AS client_protocol,
 (COALESCE((s.dimensions->>'resolution')::TEXT,''))::TEXT AS resolution,
 (COALESCE((s.dimensions->>'protocol_conversion_enabled')::BOOLEAN,false))::BOOLEAN AS protocol_conversion_enabled,
 (COALESCE((s.dimensions->>'upstream_model_mapping_applied')::BOOLEAN,false))::BOOLEAN AS upstream_model_mapping_applied,
 (COALESCE((s.dimensions->>'public_response_model')::TEXT,''))::TEXT AS public_response_model,
 (false)::BOOLEAN AS usage_estimated,
 (COALESCE((s.dimensions->>'token_usage_source')::TEXT,''))::TEXT AS token_usage_source,
 (COALESCE((s.dimensions->>'provider_terminal_state')::TEXT,''))::TEXT AS provider_terminal_state,
 (COALESCE(r.delivery_state,'unknown'))::TEXT AS client_delivery_state,
 (COALESCE((s.dimensions->>'cancellation_origin')::TEXT,''))::TEXT AS cancellation_origin,
 (s.reason)::TEXT AS billing_reason,
 (COALESCE((s.dimensions->>'response_summary_state')::TEXT,''))::TEXT AS response_summary_state,
 (s.billing_source)::TEXT AS billing_source,
 (NULLIF(s.subscription_id,'')::uuid)::UUID AS subscription_id,
 (s.created_at)::TIMESTAMPTZ AS created_at,
 ((s.dimensions->>'group_default_user_multiplier_snapshot')::NUMERIC(10,4))::NUMERIC(10,4) AS group_default_user_multiplier_snapshot,
 ((s.dimensions->>'user_multiplier_override_snapshot')::NUMERIC(10,4))::NUMERIC(10,4) AS user_multiplier_override_snapshot,
 ((s.dimensions->>'effective_user_multiplier_snapshot')::NUMERIC(10,4))::NUMERIC(10,4) AS effective_user_multiplier_snapshot
FROM bill_settlements s LEFT JOIN ai_requests r ON r.created_at=s.created_at AND r.request_id=s.request_id
LEFT JOIN ai_request_errors e ON e.created_at=s.created_at AND e.request_id=s.request_id;

CREATE OR REPLACE VIEW tenant_usage_projection AS
SELECT e.tenant_id,
       e.user_id,
       COALESCE(NULLIF(u.username, ''), '已删除用户') AS username,
       COALESCE(e.request_source, '') AS request_source,
       e.created_at,
       e.billing_status,
       COALESCE(e.user_charged, 0)::bigint AS user_charged
FROM ai_consumption_projection e
LEFT JOIN iam_accounts u ON u.user_id = e.user_id AND u.user_type = 4;

CREATE OR REPLACE VIEW system_usage_projection AS
SELECT tenant_id,
       request_source,
       billing_status,
       created_at,
       tenant_payable,
       user_charged
FROM ai_consumption_projection;

CREATE OR REPLACE VIEW system_account_stats_projection AS
SELECT t.tenant_id,
       (SELECT COUNT(*)
        FROM iam_accounts u
        WHERE u.tenant_id = t.tenant_id
          AND u.user_type = 4)::bigint AS end_user_count,
       (SELECT COUNT(*)
        FROM iam_invitation_codes i
        WHERE i.tenant_id = t.tenant_id)::bigint AS invite_code_count,
       COALESCE((SELECT SUM(u.user_charged)
                 FROM ai_consumption_projection u
                 WHERE u.tenant_id = t.tenant_id
                   AND u.billing_status = 'settled'), 0)::bigint AS user_deduction_micro
FROM iam_tenants t;

UPDATE dai_schema_metadata SET version=41,updated_at=now() WHERE singleton AND version=40;
COMMIT;
