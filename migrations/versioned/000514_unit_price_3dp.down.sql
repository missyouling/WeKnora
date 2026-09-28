DO $$ BEGIN RAISE NOTICE '[Migration 000105] down: revert unit price to 2 decimals'; END $$;

-- 回退列精度（3 位小数会再次截断为 2 位，属预期回滚行为）
ALTER TABLE utility_meter_items ALTER COLUMN unit_price TYPE NUMERIC(18,2);
ALTER TABLE utility_meters ALTER COLUMN default_unit_price TYPE NUMERIC(18,2);

DO $$ BEGIN RAISE NOTICE '[Migration 000105] down done'; END $$;
