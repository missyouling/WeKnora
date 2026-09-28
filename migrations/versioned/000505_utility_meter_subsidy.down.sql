-- Description: 回滚补差金额列。
DO $$ BEGIN RAISE NOTICE '[Migration 000096] Dropping subsidy column'; END $$;

ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS subsidy;

DO $$ BEGIN RAISE NOTICE '[Migration 000096] subsidy column dropped'; END $$;
