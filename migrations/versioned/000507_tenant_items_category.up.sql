-- 分摊子项改为子项级(name)开关：新增 category 大类分组列；旧数据为类别级开关，重建为子项级
ALTER TABLE billing_tenant_items ADD COLUMN IF NOT EXISTS category VARCHAR(255) NOT NULL DEFAULT '';
DELETE FROM billing_tenant_items;
