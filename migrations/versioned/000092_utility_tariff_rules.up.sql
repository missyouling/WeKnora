-- 分时电价规则：尖峰平谷单价按月份配置，不同月份可用不同计价方式
-- 例如 7、8 月尖峰与峰分开计价，其它月份尖峰与峰同价（尖=峰 填相同值即可）
CREATE TABLE IF NOT EXISTS utility_tariff_rules (
    id             VARCHAR(36) PRIMARY KEY,
    tenant_id      BIGINT      NOT NULL DEFAULT 10000,
    category       VARCHAR(32) NOT NULL DEFAULT 'electricity',
    name           VARCHAR(128) NOT NULL DEFAULT '',
    months         VARCHAR(64) NOT NULL DEFAULT '',      -- 适用月份，逗号分隔，如 "7,8"
    deep_peak_rate NUMERIC(18,4) NOT NULL DEFAULT 0,      -- 尖峰单价 元/千瓦时
    peak_rate      NUMERIC(18,4) NOT NULL DEFAULT 0,      -- 峰单价
    flat_rate      NUMERIC(18,4) NOT NULL DEFAULT 0,      -- 平单价
    valley_rate    NUMERIC(18,4) NOT NULL DEFAULT 0,      -- 谷单价
    is_default     BOOLEAN     NOT NULL DEFAULT FALSE,    -- 无匹配月份时的兜底规则
    sort_order     INT         NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_utility_tariff_rules_tenant ON utility_tariff_rules(tenant_id, category);
