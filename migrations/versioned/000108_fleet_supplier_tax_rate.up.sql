-- 000108: fleet_suppliers.tax_rate 由 numeric 调整为 varchar，支持 "13%" 或 0.13 写法
ALTER TABLE fleet_suppliers ALTER COLUMN tax_rate TYPE varchar(20) USING tax_rate::varchar;
