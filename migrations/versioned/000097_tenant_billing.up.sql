-- Description: 租户费用核算(持睿汽车等租户电费/水费分摊)数据表。
DO $$ BEGIN RAISE NOTICE '[Migration 000097] Creating tenant billing tables'; END $$;

-- 租户
CREATE TABLE IF NOT EXISTS billing_tenants (
    id            VARCHAR(36)  PRIMARY KEY,
    tenant_id     BIGINT       NOT NULL,
    name          VARCHAR(128) NOT NULL,
    remark        TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

-- 租户核算参数(宿舍电价、水价等)
CREATE TABLE IF NOT EXISTS billing_tenant_settings (
    id                 VARCHAR(36)  PRIMARY KEY,
    billing_tenant_id  VARCHAR(36)  NOT NULL,
    dorm_price         NUMERIC(18,4) NOT NULL DEFAULT 1,
    water_price        NUMERIC(18,4) NOT NULL DEFAULT 5.22,
    bill_kb_id         VARCHAR(36)  NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 分时电表(星达分表 star / 持睿工业分表 sub,按尖峰平谷四时段录入)
CREATE TABLE IF NOT EXISTS billing_time_meters (
    id                 VARCHAR(36)  PRIMARY KEY,
    billing_tenant_id  VARCHAR(36)  NOT NULL,
    meter_type         VARCHAR(20)  NOT NULL,  -- star | sub
    name               VARCHAR(128) NOT NULL,
    rate               NUMERIC(12,4) NOT NULL DEFAULT 1,
    enabled            BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ
);

-- 分时电表月度读数(尖峰平谷四时段起止度)
CREATE TABLE IF NOT EXISTS billing_time_meter_readings (
    id            VARCHAR(36)  PRIMARY KEY,
    meter_id      VARCHAR(36)  NOT NULL,
    month         VARCHAR(7)   NOT NULL,
    deep_prev     NUMERIC(18,2) NOT NULL DEFAULT 0,
    deep_curr     NUMERIC(18,2) NOT NULL DEFAULT 0,
    peak_prev     NUMERIC(18,2) NOT NULL DEFAULT 0,
    peak_curr     NUMERIC(18,2) NOT NULL DEFAULT 0,
    flat_prev     NUMERIC(18,2) NOT NULL DEFAULT 0,
    flat_curr     NUMERIC(18,2) NOT NULL DEFAULT 0,
    valley_prev   NUMERIC(18,2) NOT NULL DEFAULT 0,
    valley_curr   NUMERIC(18,2) NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_time_reading
    ON billing_time_meter_readings (meter_id, month);

-- 分摊子项开关(市电账单各费用子项是否参与分摊)
CREATE TABLE IF NOT EXISTS billing_tenant_items (
    id                 VARCHAR(36)  PRIMARY KEY,
    billing_tenant_id  VARCHAR(36)  NOT NULL,
    item_key           VARCHAR(64)  NOT NULL,
    item_name          VARCHAR(128) NOT NULL,
    enabled            BOOLEAN      NOT NULL DEFAULT TRUE,
    sort               INT          NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_tenant_items
    ON billing_tenant_items (billing_tenant_id, item_key);

-- 租户引用表计(宿舍电表 category=electricity / 水表 category=water,引用 utility_meters)
CREATE TABLE IF NOT EXISTS billing_tenant_meter_refs (
    id                 VARCHAR(36)  PRIMARY KEY,
    billing_tenant_id  VARCHAR(36)  NOT NULL,
    category           VARCHAR(20)  NOT NULL,  -- electricity | water
    meter_id           VARCHAR(36)  NOT NULL,
    meter_name         VARCHAR(128) NOT NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_tenant_meter_refs
    ON billing_tenant_meter_refs (billing_tenant_id, category, meter_id);

-- 月度账单(生成时固化金额)
CREATE TABLE IF NOT EXISTS billing_records (
    id                 VARCHAR(36)  PRIMARY KEY,
    billing_tenant_id  VARCHAR(36)  NOT NULL,
    month              VARCHAR(7)   NOT NULL,
    status             VARCHAR(20)  NOT NULL DEFAULT 'generated',  -- generated | adjusted
    total_kwh          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 星达分表总电量
    line_loss          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 线损
    ratio              NUMERIC(18,6) NOT NULL DEFAULT 0,  -- 分摊比例
    bill_total_amount  NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 市电本期电费(展示)
    dorm_kwh           NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 宿舍度数
    dorm_fee           NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 宿舍电费
    water_usage        NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 水用量
    water_fee          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 水费
    industrial_fee     NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 厂区电费
    total_fee          NUMERIC(18,2) NOT NULL DEFAULT 0,  -- 总应付
    remark             TEXT,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_records
    ON billing_records (billing_tenant_id, month) WHERE deleted_at IS NULL;

-- 账单子行(固化明细:子项/时段/电量/单价/费用)
CREATE TABLE IF NOT EXISTS billing_record_items (
    id            VARCHAR(36)  PRIMARY KEY,
    record_id     VARCHAR(36)  NOT NULL,
    kind          VARCHAR(20)  NOT NULL,   -- fee(分摊子项) | dorm | water | base | pf
    name          VARCHAR(128) NOT NULL,
    period        VARCHAR(20)  NOT NULL DEFAULT '',
    qty           NUMERIC(18,2) NOT NULL DEFAULT 0,
    rate          NUMERIC(18,6) NOT NULL DEFAULT 0,
    fee           NUMERIC(18,2) NOT NULL DEFAULT 0,
    sort          INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

DO $$ BEGIN RAISE NOTICE '[Migration 000097] Tenant billing tables created'; END $$;
