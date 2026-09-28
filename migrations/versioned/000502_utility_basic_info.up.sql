-- 000093_utility_basic_info.up.sql
-- 电费基本户信息：按租户+分类各一份，概览基础信息直接读取
CREATE TABLE IF NOT EXISTS utility_basic_info (
    tenant_id      bigint       NOT NULL,
    category       text         NOT NULL,
    account_no     text         NOT NULL DEFAULT '',
    account_name   text         NOT NULL DEFAULT '',
    usage_category text         NOT NULL DEFAULT '',
    voltage_level  text         NOT NULL DEFAULT '',
    market_attr    text         NOT NULL DEFAULT '',
    supply_unit    text         NOT NULL DEFAULT '',
    address        text         NOT NULL DEFAULT '',
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, category)
);
