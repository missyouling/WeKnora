-- Description: 水/气表计配置表 utility_meters；utility_meter_items 增加 meter_id 关联列。
DO $$ BEGIN RAISE NOTICE '[Migration 000092] Creating utility_meters table'; END $$;

CREATE TABLE IF NOT EXISTS utility_meters (
    id                VARCHAR(36)   PRIMARY KEY,
    tenant_id         BIGINT        NOT NULL,
    category          VARCHAR(20)   NOT NULL,
    alias             VARCHAR(128)  NOT NULL,
    meter_no          VARCHAR(64),
    rate              NUMERIC(12,4) NOT NULL DEFAULT 1,
    default_unit_price NUMERIC(18,4) NOT NULL DEFAULT 0,
    use_unit          VARCHAR(64),
    manager           VARCHAR(64),
    contact           VARCHAR(64),
    meter_mode        VARCHAR(20)   NOT NULL DEFAULT 'manual',
    install_date      VARCHAR(20),
    remark            TEXT,
    enabled           BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_utility_meters_tenant_cat
    ON utility_meters (tenant_id, category);

ALTER TABLE utility_meter_items ADD COLUMN IF NOT EXISTS meter_id VARCHAR(36);
CREATE INDEX IF NOT EXISTS idx_utility_meter_items_meter
    ON utility_meter_items (meter_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000092] Utility meters table ready'; END $$;
