DO $$ BEGIN RAISE NOTICE '[Migration 000105] unit price to 3 decimal places'; END $$;

-- 气费/水费/电费单价与表计默认单价升级为 3 位小数（2.196 不再被 numeric(18,2) 截断为 2.20）
ALTER TABLE utility_meters ALTER COLUMN default_unit_price TYPE NUMERIC(18,3);
ALTER TABLE utility_meter_items ALTER COLUMN unit_price TYPE NUMERIC(18,3);

-- 恢复公租房气表默认单价 2.196（000104 时因列精度被截断为 2.20）
UPDATE utility_meters SET default_unit_price = 2.196
WHERE category = 'gas' AND meter_kind = 'public' AND default_unit_price IN (3.4, 2.20);

-- 同步恢复公租房气表记录的单价 2.196（同样由截断产生）
UPDATE utility_meter_items SET unit_price = 2.196
WHERE meter_id IN (
  SELECT id FROM utility_meters
  WHERE category = 'gas' AND meter_kind = 'public'
) AND unit_price = 2.20;

DO $$ BEGIN RAISE NOTICE '[Migration 000105] done'; END $$;
