-- Description: 租户档案字段 + 分时电表档案字段(归属单位动态分组)。
DO $$ BEGIN RAISE NOTICE '[Migration 000099] Tenant profile fields'; END $$;

-- 租户档案
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS tenant_no       VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS allocation_mode VARCHAR(32)  NOT NULL DEFAULT '按比例分摊';
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS lease_start     DATE;
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS lease_years     INT          NOT NULL DEFAULT 0;
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS contact         VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE billing_tenants ADD COLUMN IF NOT EXISTS phone           VARCHAR(32)  NOT NULL DEFAULT '';

-- 分时电表档案字段
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS meter_no     VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS meter_kind   VARCHAR(16)  NOT NULL DEFAULT 'time'; -- time | normal
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS owner_unit   VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS use_unit     VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS manager      VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS contact      VARCHAR(64)  NOT NULL DEFAULT '';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS meter_mode   VARCHAR(16)  NOT NULL DEFAULT 'manual';
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS install_date DATE;
ALTER TABLE billing_time_meters ADD COLUMN IF NOT EXISTS remark       TEXT;

-- 存量数据:star -> 归属星达铜业(总表);sub -> 归属租户名(分表)
UPDATE billing_time_meters m SET owner_unit = '星达铜业', meter_kind = 'time' WHERE m.meter_type = 'star';
UPDATE billing_time_meters m SET owner_unit = (SELECT t.name FROM billing_tenants t WHERE t.id = m.billing_tenant_id), meter_kind = 'time' WHERE m.meter_type = 'sub' AND owner_unit = '';

DO $$ BEGIN RAISE NOTICE '[Migration 000099] Tenant profile fields done'; END $$;
