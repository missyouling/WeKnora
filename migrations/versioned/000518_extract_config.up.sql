DO $$ BEGIN RAISE NOTICE '[Migration 000109] user-configurable extract rules - configs + versions'; END $$;

-- 可配置字段提取规则：结构化字段配置 + 高级 Prompt 模板（按知识库+scope+证照类型维度）
CREATE TABLE IF NOT EXISTS knowledge_extract_configs (
    id                TEXT PRIMARY KEY,
    tenant_id         BIGINT NOT NULL,
    knowledge_base_id TEXT NOT NULL DEFAULT '',
    scope             TEXT NOT NULL DEFAULT '',
    cert_type         TEXT NOT NULL DEFAULT '',
    fields            JSONB NOT NULL DEFAULT '[]'::jsonb,
    advanced_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    prompt_template   TEXT NOT NULL DEFAULT '',
    version           INTEGER NOT NULL DEFAULT 1,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_extract_config_tenant ON knowledge_extract_configs (tenant_id, knowledge_base_id, scope, cert_type, deleted_at);

-- 版本快照：每次保存 version+1 并落快照，支持回滚
CREATE TABLE IF NOT EXISTS knowledge_extract_config_versions (
    id               TEXT PRIMARY KEY,
    config_id        TEXT NOT NULL,
    version          INTEGER NOT NULL DEFAULT 1,
    fields           JSONB NOT NULL DEFAULT '[]'::jsonb,
    advanced_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    prompt_template  TEXT NOT NULL DEFAULT '',
    remark           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_extract_config_ver ON knowledge_extract_config_versions (config_id, version);

DO $$ BEGIN RAISE NOTICE '[Migration 000109] done'; END $$;
