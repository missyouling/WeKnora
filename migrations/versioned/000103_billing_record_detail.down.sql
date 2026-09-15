DROP TABLE IF EXISTS billing_record_waters;
DROP TABLE IF EXISTS billing_record_meters;
ALTER TABLE billing_record_items DROP COLUMN IF EXISTS category;
