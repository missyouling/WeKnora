-- 聚合列表（发票 / 合同 / 制度 / 奖惩）与文档列表都要按「知识库 + 时间倒序」
-- 翻页拉全量 knowledges：WHERE tenant_id/knowledge_base_id + deleted_at IS NULL
-- ORDER BY created_at DESC（或 updated_at DESC）, id ASC。
--
-- 既有 idx_knowledges_base_id 只覆盖单列 knowledge_base_id，每翻一页都要对命中行
-- 重新排序（KB 内有数百行时，N 页 = N 次 sort + offset 扫描）。
-- 组合索引把「过滤 + 排序」合成一次有序扫描，页内零排序。
--
-- 用部分索引（WHERE deleted_at IS NULL）：软删行走任何列表查询都不可见，
-- 剔掉它们能让索引小一圈；与既有 idx_knowledges_kb_metadata_external_id 同风格。
--
-- file_name 排序（LOWER(COALESCE(NULLIF(file_name,''),...))）是表达式排序，
-- 使用频率低，暂不建索引——默认路径（created_at/updated_at）才是本次要解决的。
CREATE INDEX IF NOT EXISTS idx_knowledges_base_created_at
    ON knowledges (knowledge_base_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_knowledges_base_updated_at
    ON knowledges (knowledge_base_id, updated_at DESC)
    WHERE deleted_at IS NULL;
