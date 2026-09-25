ALTER TABLE utility_meter_records DROP COLUMN IF EXISTS record_date;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS reading_date;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS reader;
ALTER TABLE utility_meter_items DROP COLUMN IF EXISTS rate;
