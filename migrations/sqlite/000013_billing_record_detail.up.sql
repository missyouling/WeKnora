-- Description: 账单明细增强(SQLite 本地部署):新增 billing_record_meters / billing_record_waters。
-- 注: SQLite 本地部署未启用租户核算(billing 主表迁移仅存在于 versioned/Postgres),此处仅保证两新表存在。

CREATE TABLE IF NOT EXISTS billing_record_meters (
    id            VARCHAR(36)  PRIMARY KEY,
    record_id     VARCHAR(36)  NOT NULL,
    meter_name    VARCHAR(128) NOT NULL,
    period        VARCHAR(20)  NOT NULL DEFAULT '',
    prev          REAL         NOT NULL DEFAULT 0,
    curr          REAL         NOT NULL DEFAULT 0,
    rate          REAL         NOT NULL DEFAULT 1,
    usage         REAL         NOT NULL DEFAULT 0,
    line_loss     REAL         NOT NULL DEFAULT 0,
    adjust        REAL         NOT NULL DEFAULT 0,
    bill_kwh      REAL         NOT NULL DEFAULT 0,
    diff_kwh      REAL         NOT NULL DEFAULT 0,
    sort          INTEGER      NOT NULL DEFAULT 0,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_billing_record_meters_record ON billing_record_meters (record_id);

CREATE TABLE IF NOT EXISTS billing_record_waters (
    id            VARCHAR(36)  PRIMARY KEY,
    record_id     VARCHAR(36)  NOT NULL,
    meter_name    VARCHAR(128) NOT NULL,
    meter_kind    VARCHAR(20)  NOT NULL DEFAULT '',
    prev          REAL         NOT NULL DEFAULT 0,
    curr          REAL         NOT NULL DEFAULT 0,
    rate          REAL         NOT NULL DEFAULT 1,
    usage         REAL         NOT NULL DEFAULT 0,
    price         REAL         NOT NULL DEFAULT 0,
    fee           REAL         NOT NULL DEFAULT 0,
    sort          INTEGER      NOT NULL DEFAULT 0,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_billing_record_waters_record ON billing_record_waters (record_id);
