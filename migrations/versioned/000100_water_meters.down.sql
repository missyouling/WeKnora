-- Description: 回滚 000100 水表。
DO $$ BEGIN RAISE NOTICE '[Migration 000100] rollback water meters'; END $$;

DROP TABLE IF EXISTS billing_water_meter_readings;
DROP TABLE IF EXISTS billing_water_meters;

DO $$ BEGIN RAISE NOTICE '[Migration 000100] rollback done'; END $$;
