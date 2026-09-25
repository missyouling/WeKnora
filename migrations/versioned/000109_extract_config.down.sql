DO $$ BEGIN RAISE NOTICE '[Migration 000109] dropping extract config tables'; END $$;

DROP TABLE IF EXISTS knowledge_extract_config_versions;
DROP TABLE IF EXISTS knowledge_extract_configs;

DO $$ BEGIN RAISE NOTICE '[Migration 000109] done'; END $$;
