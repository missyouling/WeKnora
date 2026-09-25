-- Description: 回滚租户档案字段。
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS tenant_no;
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS allocation_mode;
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS lease_start;
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS lease_years;
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS contact;
ALTER TABLE billing_tenants      DROP COLUMN IF EXISTS phone;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS meter_no;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS meter_kind;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS owner_unit;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS use_unit;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS manager;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS contact;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS meter_mode;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS install_date;
ALTER TABLE billing_time_meters  DROP COLUMN IF EXISTS remark;
