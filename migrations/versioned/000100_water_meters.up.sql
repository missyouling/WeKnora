-- Description: 租户核算水表(总表/工业/宿舍/消防)与月度读数。
DO $$ BEGIN RAISE NOTICE '[Migration 000100] Water meters'; END $$;

CREATE TABLE IF NOT EXISTS billing_water_meters (
    id                VARCHAR(64)  PRIMARY KEY,
    billing_tenant_id VARCHAR(64)  NOT NULL DEFAULT '',
    name              VARCHAR(128) NOT NULL DEFAULT '',
    meter_no          VARCHAR(64)  NOT NULL DEFAULT '',
    meter_kind        VARCHAR(16)  NOT NULL DEFAULT 'total', -- total | industry | dorm | fire
    owner_unit        VARCHAR(128) NOT NULL DEFAULT '',
    use_unit          VARCHAR(128) NOT NULL DEFAULT '',
    manager           VARCHAR(64)  NOT NULL DEFAULT '',
    contact           VARCHAR(64)  NOT NULL DEFAULT '',
    meter_mode        VARCHAR(16)  NOT NULL DEFAULT 'manual',
    install_date      DATE,
    remark            TEXT,
    rate              NUMERIC(12,4) NOT NULL DEFAULT 1,
    price             NUMERIC(18,4) NOT NULL DEFAULT 0,
    enabled           BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMP     NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP     NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_water_meters_tenant ON billing_water_meters (billing_tenant_id, deleted_at);

CREATE TABLE IF NOT EXISTS billing_water_meter_readings (
    id         VARCHAR(64)  PRIMARY KEY,
    meter_id   VARCHAR(64)  NOT NULL,
    month      VARCHAR(16)  NOT NULL,
    prev       NUMERIC(18,2) NOT NULL DEFAULT 0,
    curr       NUMERIC(18,2) NOT NULL DEFAULT 0,
    price      NUMERIC(18,4) NOT NULL DEFAULT 0,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_water_meter_month UNIQUE (meter_id, month)
);

DO $$ BEGIN RAISE NOTICE '[Migration 000100] Water meters done'; END $$;
