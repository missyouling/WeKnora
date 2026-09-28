-- 表计类型迁移:utility_meters 新增计量层级(meter_type)与归属单位(owner_unit)
-- meter_type 按类别解释:
--   electricity: normal(普通,默认) | time(分时)
--   water:       total(总表) | sub(分表,默认) | fire(消防)
--   gas:         normal(默认)
-- meter_kind(用途,已有): dorm(宿舍,默认) | production(生产)
ALTER TABLE utility_meters ADD COLUMN IF NOT EXISTS meter_type varchar(16) NOT NULL DEFAULT 'normal';
ALTER TABLE utility_meters ADD COLUMN IF NOT EXISTS owner_unit varchar(200) NOT NULL DEFAULT '';

-- 表计月度记录子行支持分时四时段起止读数(仅分时电表使用;普通表计仍用 start_reading/end_reading)
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS deep_prev numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS deep_curr numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS peak_prev numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS peak_curr numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS flat_prev numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS flat_curr numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS valley_prev numeric(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS valley_curr numeric(18,2) NOT NULL DEFAULT 0;
