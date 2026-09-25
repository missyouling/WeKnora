-- 水电气用途配置：自定义用途（公租房等）持久化，跨端共享
CREATE TABLE IF NOT EXISTS utility_kinds (
    id          TEXT PRIMARY KEY,
    tenant_id   BIGINT NOT NULL DEFAULT 0,
    scope       VARCHAR(20) NOT NULL DEFAULT 'electricity', -- water | gas | electricity
    value       VARCHAR(50) NOT NULL DEFAULT '',
    label       VARCHAR(50) NOT NULL DEFAULT '',
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_utility_kinds_tenant_scope ON utility_kinds (tenant_id, scope, deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_utility_kinds_scope_value ON utility_kinds (tenant_id, scope, value) WHERE deleted_at IS NULL;
