-- Description: 回滚: 水费附加费用列、表计排序列、公租房气表单价恢复
DO $$ BEGIN RAISE NOTICE '[Migration 000104] rollback'; END $$;

UPDATE utility_meters SET default_unit_price = 3.4 WHERE category = 'gas' AND meter_kind = 'public' AND default_unit_price = 2.196;

ALTER TABLE utility_meters DROP COLUMN IF EXISTS sort_order;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS garbage_fee;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS secondary_water_fee;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS sewage_fee;

DO $$ BEGIN RAISE NOTICE '[Migration 000104] done'; END $$;
