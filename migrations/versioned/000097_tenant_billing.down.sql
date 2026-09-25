-- Description: 回滚租户费用核算表。
DO $$ BEGIN RAISE NOTICE '[Migration 000097] Dropping tenant billing tables'; END $$;

DROP TABLE IF EXISTS billing_record_items;
DROP TABLE IF EXISTS billing_records;
DROP TABLE IF EXISTS billing_tenant_meter_refs;
DROP TABLE IF EXISTS billing_tenant_items;
DROP TABLE IF EXISTS billing_time_meter_readings;
DROP TABLE IF EXISTS billing_time_meters;
DROP TABLE IF EXISTS billing_tenant_settings;
DROP TABLE IF EXISTS billing_tenants;

DO $$ BEGIN RAISE NOTICE '[Migration 000097] Tenant billing tables dropped'; END $$;
