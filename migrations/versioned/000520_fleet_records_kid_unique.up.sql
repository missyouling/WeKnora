-- 同一知识文件(doc_knowledge_id)在同一租户、同一 record_type 下只允许一条有效记录，
-- 杜绝上传弹窗与列表轮询并发触发 extract 时重复落库。空 doc_knowledge_id（手动新增）不参与唯一约束。
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_records_kid_uq
    ON fleet_records (tenant_id, record_type, doc_knowledge_id)
    WHERE doc_knowledge_id <> '' AND deleted_at IS NULL;
