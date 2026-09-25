-- Description: 水电气管理模块数据表回滚。
DO $$ BEGIN RAISE NOTICE '[Migration 000091 down] Dropping utility tables'; END $$;

DROP INDEX IF EXISTS idx_utility_meter_items_record;
DROP TABLE IF EXISTS utility_meter_items;
DROP INDEX IF EXISTS idx_utility_meter_records_tenant_cat;
DROP INDEX IF EXISTS uq_utility_meter_records;
DROP TABLE IF EXISTS utility_meter_records;
DROP INDEX IF EXISTS uq_utility_field_configs;
DROP TABLE IF EXISTS utility_field_configs;
