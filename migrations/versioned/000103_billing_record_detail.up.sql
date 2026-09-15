-- Description: 账单明细增强: billing_record_items 增加 category 大类列;新增 billing_record_meters(电量明细,表×时段)与 billing_record_waters(逐表水费清单)。
DO $$ BEGIN RAISE NOTICE '[Migration 000103] Billing record detail tables'; END $$;

-- 费用明细子行增加大类列(用于 UI 按大项分组)
ALTER TABLE billing_record_items ADD COLUMN IF NOT EXISTS category VARCHAR(128) NOT NULL DEFAULT '';

-- 账单电量明细(分时电表 × 尖峰/峰/平/谷 时段)
CREATE TABLE IF NOT EXISTS billing_record_meters (
    id            VARCHAR(36)  PRIMARY KEY,
    record_id     VARCHAR(36)  NOT NULL,
    meter_name    VARCHAR(128) NOT NULL,
    period        VARCHAR(20)  NOT NULL DEFAULT '',
    prev          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 起度
    curr          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 止度
    rate          NUMERIC(12,4) NOT NULL DEFAULT 1,  -- 倍率
    usage         NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 使用电量
    line_loss     NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 损耗
    adjust        NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 加减电量
    bill_kwh      NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 计费电量
    diff_kwh      NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 差额分摊电量
    sort          INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_billing_record_meters_record ON billing_record_meters (record_id);

-- 账单水费清单(逐表)
CREATE TABLE IF NOT EXISTS billing_record_waters (
    id            VARCHAR(36)  PRIMARY KEY,
    record_id     VARCHAR(36)  NOT NULL,
    meter_name    VARCHAR(128) NOT NULL,
    meter_kind    VARCHAR(20)  NOT NULL DEFAULT '',  -- total | sub | fire
    prev          NUMERIC(18,2) NOT NULL DEFAULT 0,
    curr          NUMERIC(18,2) NOT NULL DEFAULT 0,
    rate          NUMERIC(12,4) NOT NULL DEFAULT 1,
    usage         NUMERIC(18,2) NOT NULL DEFAULT 0,
    price         NUMERIC(18,4) NOT NULL DEFAULT 0,
    fee           NUMERIC(18,2) NOT NULL DEFAULT 0,
    sort          INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_billing_record_waters_record ON billing_record_waters (record_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000103] done'; END $$;
