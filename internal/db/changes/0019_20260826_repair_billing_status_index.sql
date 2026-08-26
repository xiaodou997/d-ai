-- from_version: 18
-- to_version: 19
-- created_at: 2026-08-26
-- description: repair billing-status index missing from the historical v1-v18 chain
--
-- The index is already present in the v19 empty-database baseline. Existing
-- databases that reached v18 through the forward chain may not have it because
-- it was added by an unversioned pre-release baseline edit. Repair that drift
-- without modifying the published 0008 migration.

BEGIN;

SELECT pg_advisory_xact_lock(82624001);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata
        WHERE singleton = TRUE AND version = 18
    ) THEN
        RAISE EXCEPTION 'expected D-AI schema version 18';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class index_class
        JOIN pg_namespace index_namespace ON index_namespace.oid = index_class.relnamespace
        JOIN pg_index index_info ON index_info.indexrelid = index_class.oid
        WHERE index_namespace.nspname = current_schema()
          AND index_class.relname = 'idx_ai_usage_logs_billing_status'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_class index_class
            JOIN pg_namespace index_namespace ON index_namespace.oid = index_class.relnamespace
            JOIN pg_index index_info ON index_info.indexrelid = index_class.oid
            WHERE index_namespace.nspname = current_schema()
              AND index_class.relname = 'idx_ai_usage_logs_billing_status'
              AND index_info.indrelid = 'ai_usage_logs'::regclass
              AND index_info.indpred IS NULL
              AND pg_get_indexdef(index_info.indexrelid) LIKE '%(billing_status, created_at DESC)%'
        ) THEN
            RAISE EXCEPTION 'idx_ai_usage_logs_billing_status exists with an unexpected definition';
        END IF;
    ELSE
        EXECUTE 'CREATE INDEX idx_ai_usage_logs_billing_status
                 ON ai_usage_logs (billing_status, created_at DESC)';
    END IF;

    -- Dropping settled_event_id in 0008 removed the dependent check from
    -- historical v18 databases. Restore the same invariant that v19 init.sql
    -- has, while refusing to overwrite an existing differently-defined check.
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint constraint_row
        JOIN pg_class table_class ON table_class.oid = constraint_row.conrelid
        JOIN pg_namespace table_namespace ON table_namespace.oid = table_class.relnamespace
        WHERE table_namespace.nspname = current_schema()
          AND table_class.relname = 'ledger_credit_leases'
          AND constraint_row.conname = 'ledger_credit_leases_check8'
    ) THEN
        EXECUTE $constraint$
            ALTER TABLE ledger_credit_leases
            ADD CONSTRAINT ledger_credit_leases_check8 CHECK (
                (settlement_state = 'unsettled'
                    AND settlement_id IS NULL
                    AND actual_tenant_micro IS NULL
                    AND actual_user_micro IS NULL
                    AND tenant_deducted_micro = 0
                    AND user_deducted_micro = 0
                    AND tenant_debt_added_micro = 0
                    AND user_debt_added_micro = 0
                    AND settled_at IS NULL)
                OR
                (settlement_state = 'settled'
                    AND settlement_id IS NOT NULL
                    AND actual_tenant_micro IS NOT NULL
                    AND actual_user_micro IS NOT NULL
                    AND tenant_deducted_micro + tenant_debt_added_micro = actual_tenant_micro
                    AND user_deducted_micro + user_debt_added_micro = actual_user_micro
                    AND settled_at IS NOT NULL
                    AND escrow_state = 'released')
            )
        $constraint$;
    ELSE
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint constraint_row
            JOIN pg_class table_class ON table_class.oid = constraint_row.conrelid
            JOIN pg_namespace table_namespace ON table_namespace.oid = table_class.relnamespace
            WHERE table_namespace.nspname = current_schema()
              AND table_class.relname = 'ledger_credit_leases'
              AND constraint_row.conname = 'ledger_credit_leases_check8'
              AND pg_get_constraintdef(constraint_row.oid, true) LIKE '%settlement_state%'
              AND pg_get_constraintdef(constraint_row.oid, true) LIKE '%escrow_state%'
        ) THEN
            RAISE EXCEPTION 'ledger_credit_leases_check8 exists with an unexpected definition';
        END IF;
    END IF;
END
$$;

UPDATE dai_schema_metadata
SET version = 19,
    updated_at = now()
WHERE singleton = TRUE AND version = 18;

COMMIT;
