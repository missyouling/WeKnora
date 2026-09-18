-- 回滚：恢复 numeric
ALTER TABLE fleet_suppliers ALTER COLUMN tax_rate TYPE numeric(6,2) USING NULLIF(tax_rate, '')::numeric;
