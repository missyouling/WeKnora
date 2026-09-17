DO $$ BEGIN RAISE NOTICE '[Migration 000106] fleet management tables'; END $$;

-- 车队管理：车辆配置
CREATE TABLE IF NOT EXISTS fleet_vehicles (
    id            TEXT PRIMARY KEY,
    tenant_id     BIGINT NOT NULL,
    plate_no      TEXT NOT NULL DEFAULT '',
    vehicle_type  TEXT NOT NULL DEFAULT '',
    brand_model   TEXT NOT NULL DEFAULT '',
    load_tonnage  NUMERIC(12,2) NOT NULL DEFAULT 0,
    seat_count    INTEGER NOT NULL DEFAULT 0,
    purchase_date TEXT NOT NULL DEFAULT '',
    department    TEXT NOT NULL DEFAULT '',
    manager       TEXT NOT NULL DEFAULT '',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    remark        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_vehicles_tenant ON fleet_vehicles (tenant_id, deleted_at);

-- 车队管理：驾驶员配置
CREATE TABLE IF NOT EXISTS fleet_drivers (
    id           TEXT PRIMARY KEY,
    tenant_id    BIGINT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    license_no   TEXT NOT NULL DEFAULT '',
    license_type TEXT NOT NULL DEFAULT '',
    phone        TEXT NOT NULL DEFAULT '',
    hire_date    TEXT NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    remark       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_drivers_tenant ON fleet_drivers (tenant_id, deleted_at);

-- 车队管理：油卡配置
CREATE TABLE IF NOT EXISTS fleet_fuel_cards (
    id         TEXT PRIMARY KEY,
    tenant_id  BIGINT NOT NULL,
    card_no    TEXT NOT NULL DEFAULT '',
    vehicle_id TEXT NOT NULL DEFAULT '',
    driver_id  TEXT NOT NULL DEFAULT '',
    station    TEXT NOT NULL DEFAULT '',
    face_value NUMERIC(18,2) NOT NULL DEFAULT 0,
    balance    NUMERIC(18,2) NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    remark     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_fuel_cards_tenant ON fleet_fuel_cards (tenant_id, deleted_at);

-- 车队管理：通用记录表（8 类记录共用）
CREATE TABLE IF NOT EXISTS fleet_records (
    id           TEXT PRIMARY KEY,
    tenant_id    BIGINT NOT NULL,
    record_type  TEXT NOT NULL,
    vehicle_id   TEXT NOT NULL DEFAULT '',
    record_month TEXT NOT NULL DEFAULT '',
    record_date  TEXT NOT NULL DEFAULT '',
    amount       NUMERIC(18,2) NOT NULL DEFAULT 0,
    mileage      NUMERIC(18,2) NOT NULL DEFAULT 0,
    data         JSONB NOT NULL DEFAULT '{}'::jsonb,
    remark       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fleet_records_tenant ON fleet_records (tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_fleet_records_type ON fleet_records (record_type, record_month, vehicle_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000106] done'; END $$;
