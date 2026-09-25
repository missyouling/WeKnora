-- Description: revert utility_meters.meter_kind
DO $$ BEGIN RAISE NOTICE '[Migration 000101] Dropping utility_meters.meter_kind'; END $$;

ALTER TABLE utility_meters DROP COLUMN IF EXISTS meter_kind;

DO $$ BEGIN RAISE NOTICE '[Migration 000101] meter_kind dropped'; END $$;
