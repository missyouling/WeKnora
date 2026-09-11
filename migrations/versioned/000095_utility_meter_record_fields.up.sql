-- 水/气表计记录新增字段：录入日期(record)、抄表日期/抄表人/倍率快照(item)
ALTER TABLE utility_meter_records ADD COLUMN IF NOT EXISTS record_date VARCHAR(20);
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS reading_date VARCHAR(20);
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS reader VARCHAR(100);
ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS rate NUMERIC(12,4) DEFAULT 1;
