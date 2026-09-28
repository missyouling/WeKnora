-- Description: 水电气管理模块数据表：字段配置 + 水/气月度记录与表计子行。
DO $$ BEGIN RAISE NOTICE '[Migration 000091] Creating utility tables'; END $$;

CREATE TABLE IF NOT EXISTS utility_field_configs (
    id               VARCHAR(36)  PRIMARY KEY,
    tenant_id        BIGINT       NOT NULL,
    category         VARCHAR(20)  NOT NULL,
    field_key        VARCHAR(64)  NOT NULL,
    label            VARCHAR(128) NOT NULL,
    field_type       VARCHAR(20)  NOT NULL DEFAULT 'text',
    default_visible  BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_order       INT          NOT NULL DEFAULT 0,
    is_custom        BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_utility_field_configs
    ON utility_field_configs (tenant_id, category, field_key) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS utility_meter_records (
    id            VARCHAR(36)  PRIMARY KEY,
    tenant_id     BIGINT       NOT NULL,
    category      VARCHAR(20)  NOT NULL,
    month         VARCHAR(7)   NOT NULL,
    meter_count   INT          NOT NULL DEFAULT 0,
    total_usage   NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount  NUMERIC(18,2) NOT NULL DEFAULT 0,
    remark        TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_utility_meter_records
    ON utility_meter_records (tenant_id, category, month) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_utility_meter_records_tenant_cat
    ON utility_meter_records (tenant_id, category);

CREATE TABLE IF NOT EXISTS utility_meter_items (
    id             VARCHAR(36)  PRIMARY KEY,
    record_id      VARCHAR(36)  NOT NULL,
    meter_name     VARCHAR(128) NOT NULL,
    start_reading  NUMERIC(18,2) NOT NULL DEFAULT 0,
    end_reading    NUMERIC(18,2) NOT NULL DEFAULT 0,
    unit_price     NUMERIC(18,2) NOT NULL DEFAULT 0,
    usage          NUMERIC(18,2) NOT NULL DEFAULT 0,
    amount         NUMERIC(18,2) NOT NULL DEFAULT 0,
    remark         TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_utility_meter_items_record
    ON utility_meter_items (record_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Utility tables created'; END $$;
