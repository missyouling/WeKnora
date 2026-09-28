DO $$ BEGIN RAISE NOTICE '[Migration 000107] rollback fleet v2'; END $$;

DROP TABLE IF EXISTS fleet_etc_cards;
DROP TABLE IF EXISTS fleet_suppliers;
DROP TABLE IF EXISTS fleet_categories;

ALTER TABLE fleet_fuel_cards
    DROP COLUMN IF EXISTS alias,
    DROP COLUMN IF EXISTS card_type,
    DROP COLUMN IF EXISTS brand,
    DROP COLUMN IF EXISTS card_status;

ALTER TABLE fleet_records
    DROP COLUMN IF EXISTS doc_type,
    DROP COLUMN IF EXISTS file_name,
    DROP COLUMN IF EXISTS doc_knowledge_id;

DO $$ BEGIN RAISE NOTICE '[Migration 000107] rollback done'; END $$;
