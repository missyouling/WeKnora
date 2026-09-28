-- Description: utility_meters 增加表类型 meter_kind（默认 dorm=宿舍，production=生产/非宿舍）
DO $$ BEGIN RAISE NOTICE '[Migration 000101] Adding meter_kind to utility_meters'; END $$;

ALTER TABLE utility_meters ADD COLUMN IF NOT EXISTS meter_kind VARCHAR(20) NOT NULL DEFAULT 'dorm';

DO $$ BEGIN RAISE NOTICE '[Migration 000101] utility_meters.meter_kind ready'; END $$;
