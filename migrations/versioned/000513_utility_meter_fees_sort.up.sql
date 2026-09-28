-- Description: 水费附加费用(垃圾处置费/二次供水费/污水处理费)列; 表计配置排序 sort_order 列; 公租房气表默认单价 3.4 → 2.196
DO $$ BEGIN RAISE NOTICE '[Migration 000104] utility meter fees & sort'; END $$;

-- 表计配置拖动排序
ALTER TABLE utility_meters ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

-- 水费附加费用（仅水费子行使用，默认 0）
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS garbage_fee NUMERIC(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS secondary_water_fee NUMERIC(18,2) NOT NULL DEFAULT 0;
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS sewage_fee NUMERIC(18,2) NOT NULL DEFAULT 0;

-- 公租房类气表默认单价统一 3.4 → 2.196（仅改未手工改过的 3.4 值）
UPDATE utility_meters SET default_unit_price = 2.196
WHERE category = 'gas' AND meter_kind = 'public' AND default_unit_price = 3.4;

DO $$ BEGIN RAISE NOTICE '[Migration 000104] done'; END $$;
