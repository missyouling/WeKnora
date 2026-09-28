DO $$ BEGIN RAISE NOTICE '[Migration 000106] drop fleet tables'; END $$;

DROP TABLE IF EXISTS fleet_records;
DROP TABLE IF EXISTS fleet_fuel_cards;
DROP TABLE IF EXISTS fleet_drivers;
DROP TABLE IF EXISTS fleet_vehicles;

DO $$ BEGIN RAISE NOTICE '[Migration 000106] done'; END $$;
