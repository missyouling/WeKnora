-- Description: utility_meter_items 增加补差金额列 subsidy（可正负，计入自动计算金额）。
DO $$ BEGIN RAISE NOTICE '[Migration 000096] Adding subsidy column'; END $$;

ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS subsidy NUMERIC(18,2) NOT NULL DEFAULT 0;

DO $$ BEGIN RAISE NOTICE '[Migration 000096] subsidy column added'; END $$;
