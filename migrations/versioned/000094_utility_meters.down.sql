-- Description: 回滚水/气表计配置表与 meter_id 列。
DROP INDEX IF EXISTS idx_utility_meters_tenant_cat;
DROP TABLE IF EXISTS utility_meters;

DROP INDEX IF EXISTS idx_utility_meter_items_meter;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS meter_id;
