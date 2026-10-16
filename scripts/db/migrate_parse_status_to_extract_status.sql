-- 历史数据迁移：fields 扩展桶里的 parseStatus → extractStatus
--
-- 背景：分类配置的字段名是 extractStatus，但模型常把"提取状态"写成 parseStatus，
-- 该键不在标准契约键集合内 → 原样落入 fields 扩展桶 → 前端按配置键 extractStatus
-- 取值取不到，这一列渲染为空白。
-- 实测（2026-10-16，435 条 knowledges）：parseStatus=20 条，extractStatus=0 条。
--
-- 代码侧已修（invoiceFieldAliases 增加 parseStatus→extractStatus 别名），
-- 但**存量数据不会自动改写**，需要跑本脚本。
--
-- 执行方式（VM 上）：
--   cd /opt/weknora-dev && set -a && . ./.env && set +a && export PGPASSWORD=$DB_PASSWORD
--   psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f scripts/db/migrate_parse_status_to_extract_status.sql
--
-- 安全：整段在事务里，先跑 SELECT 预览确认条数，确认无误后再 COMMIT。

BEGIN;

-- 1) 预览：确认受影响条数
SELECT count(*) AS affected_rows
FROM knowledges
WHERE jsonb_typeof(custom_metadata) = 'object'
  AND jsonb_typeof(custom_metadata -> 'invoices') = 'array'
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements(custom_metadata -> 'invoices') AS inv
    WHERE inv -> 'fields' ? 'parseStatus'
  );

-- 2) 执行改写：把每条 invoice 的 fields.parseStatus 改名为 extractStatus
--    （已存在 extractStatus 时不覆盖，避免丢数据）
UPDATE knowledges k
SET custom_metadata = (
  jsonb_set(
    k.custom_metadata,
    '{invoices}',
    COALESCE((
      SELECT jsonb_agg(
        CASE
          WHEN inv -> 'fields' ? 'parseStatus' AND NOT (inv -> 'fields' ? 'extractStatus') THEN
            jsonb_set(
              inv,
              '{fields}',
              (inv -> 'fields') - 'parseStatus'
                || jsonb_build_object('extractStatus', inv -> 'fields' -> 'parseStatus')
            )
          WHEN inv -> 'fields' ? 'parseStatus' THEN
            jsonb_set(inv, '{fields}', (inv -> 'fields') - 'parseStatus')
          ELSE inv
        END
      )
      FROM jsonb_array_elements(k.custom_metadata -> 'invoices') AS inv
    ), '[]'::jsonb)
  )
)
WHERE jsonb_typeof(k.custom_metadata) = 'object'
  AND jsonb_typeof(k.custom_metadata -> 'invoices') = 'array'
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements(k.custom_metadata -> 'invoices') AS inv
    WHERE inv -> 'fields' ? 'parseStatus'
  );

-- 3) 校验：改写后 parseStatus 应为 0，extractStatus 应等于原 parseStatus 条数
SELECT
  (SELECT count(*) FROM knowledges
    WHERE jsonb_typeof(custom_metadata) = 'object'
      AND jsonb_typeof(custom_metadata -> 'invoices') = 'array'
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(custom_metadata -> 'invoices') inv
                  WHERE inv -> 'fields' ? 'parseStatus')) AS remaining_parse_status,
  (SELECT count(*) FROM knowledges
    WHERE jsonb_typeof(custom_metadata) = 'object'
      AND jsonb_typeof(custom_metadata -> 'invoices') = 'array'
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(custom_metadata -> 'invoices') inv
                  WHERE inv -> 'fields' ? 'extractStatus')) AS now_extract_status;

-- 确认上面两行符合预期（remaining_parse_status = 0）后再执行：
COMMIT;
-- 若不符合预期，把 COMMIT 换成 ROLLBACK。
