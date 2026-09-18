DO $$ BEGIN RAISE NOTICE '[Migration 000107] fleet management v2 - archives, categories, suppliers, etc'; END $$;

-- 车队记录表扩展：档案类型（证照解析）专用列
ALTER TABLE fleet_records
    ADD COLUMN IF NOT EXISTS doc_type           TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS file_name          TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS doc_knowledge_id   TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_fleet_records_doctype ON fleet_records (tenant_id, record_type, doc_type, deleted_at);

-- 油卡配置扩展：别名、卡类型、品牌、卡状态
ALTER TABLE fleet_fuel_cards
    ADD COLUMN IF NOT EXISTS alias       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS card_type   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS brand       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS card_status TEXT NOT NULL DEFAULT '';

-- 档案分类配置：车辆档案 / 司机档案 / 维保管理 的大项-小项（scope=vehicle|driver|maintain）
CREATE TABLE IF NOT EXISTS fleet_categories (
    id         TEXT PRIMARY KEY,
    tenant_id  BIGINT NOT NULL,
    scope      TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    subs       JSONB NOT NULL DEFAULT '[]'::jsonb,
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_categories_tenant ON fleet_categories (tenant_id, scope, deleted_at);

-- 供应商
CREATE TABLE IF NOT EXISTS fleet_suppliers (
    id                TEXT PRIMARY KEY,
    tenant_id         BIGINT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    supplier_type     TEXT NOT NULL DEFAULT '',
    qualification      TEXT NOT NULL DEFAULT '',
    credit_code       TEXT NOT NULL DEFAULT '',
    legal_person      TEXT NOT NULL DEFAULT '',
    contact           TEXT NOT NULL DEFAULT '',
    phone             TEXT NOT NULL DEFAULT '',
    address           TEXT NOT NULL DEFAULT '',
    cooperation_status TEXT NOT NULL DEFAULT '',
    coop_start_date   TEXT NOT NULL DEFAULT '',
    settle_method     TEXT NOT NULL DEFAULT '',
    tax_rate          NUMERIC(6,2) NOT NULL DEFAULT 0,
    invoice_type      TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    remark            TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_suppliers_tenant ON fleet_suppliers (tenant_id, deleted_at);

-- ETC 卡
CREATE TABLE IF NOT EXISTS fleet_etc_cards (
    id         TEXT PRIMARY KEY,
    tenant_id  BIGINT NOT NULL,
    alias      TEXT NOT NULL DEFAULT '',
    card_no    TEXT NOT NULL DEFAULT '',
    card_type  TEXT NOT NULL DEFAULT '',
    issuer     TEXT NOT NULL DEFAULT '',
    bank       TEXT NOT NULL DEFAULT '',
    open_date  TEXT NOT NULL DEFAULT '',
    expire_date TEXT NOT NULL DEFAULT '',
    vehicle_id TEXT NOT NULL DEFAULT '',
    card_status TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    remark     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_etc_tenant ON fleet_etc_cards (tenant_id, deleted_at);

DO $$ BEGIN RAISE NOTICE '[Migration 000107] done'; END $$;
