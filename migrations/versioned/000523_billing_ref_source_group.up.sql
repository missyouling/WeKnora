DO $$ BEGIN RAISE NOTICE '[Migration 000523] backfill source.group for 费用清单 reference fields'; END $$;

-- 费用清单的跨分类引用字段（fleet_categories.subs[].source）在旧版本里只记了
-- category + field，没有记**来源分组**。这带来两个真机可见的问题：
--   ① 徽标只显示「市电账单」，而同名「计费电量」在市电账单的 8 个分组里各有一条，
--      用户无法知道自己引用的是「市场化购电费」还是「输电配电量电费」；
--   ② 取值只能按名称全分类求和 —— 分组的 qty/rate/fee 会被跨组累加，金额放大数倍。
--
-- 回填口径（保守，只补不猜）：
--   · 仅处理 source 存在且 group 为空的条目；
--   · 在**来源分类**的 subs 里找 name 相同的条目，取其 group / group_label / label；
--   · 用 `COUNT(*) = 1` 限定「该字段在来源分类里只属于一个分组」才回填 ——
--     像 qty/rate/fee 这种在 8 个分组都存在的列名，回填哪个都是猜，一律跳过，
--     交由用户在字段配置里重新选择（候选项现在已带分组路径，可精确选）。
-- 幂等：group 已有值的条目一字不动，重复执行无副作用。
WITH src AS (
  SELECT
    c2.id AS cat_id,
    s->>'name'  AS field_name,
    MIN(s->>'group')       AS grp,
    MIN(s->>'group_label') AS grp_label,
    MIN(s->>'label')       AS src_label,
    COUNT(DISTINCT COALESCE(s->>'group', '')) AS group_cnt
  FROM fleet_categories c2,
       jsonb_array_elements(c2.subs) s
  WHERE c2.deleted_at IS NULL
    AND COALESCE(s->>'group', '') <> ''
  GROUP BY c2.id, s->>'name'
),
patched AS (
  SELECT
    fc.id,
    jsonb_agg(
      CASE
        WHEN COALESCE(e->'source'->>'category', '') = ''
          OR COALESCE(e->'source'->>'field', '') = ''
          OR COALESCE(e->'source'->>'group', '') <> ''
        THEN e
        ELSE (
          SELECT jsonb_set(
                   jsonb_set(
                     jsonb_set(e, '{source,group}', to_jsonb(sr.grp)),
                     '{source,group_label}', to_jsonb(COALESCE(sr.grp_label, ''))
                   ),
                   '{source,label}', to_jsonb(COALESCE(sr.src_label, sr.field_name))
                 )
          FROM src sr
          JOIN fleet_categories c3 ON c3.id = sr.cat_id
          WHERE c3.name = e->'source'->>'category'
            AND sr.field_name = e->'source'->>'field'
            -- 只有「该字段在来源分类里唯一属于一个分组」才回填；多个分组 = 有歧义，跳过
            AND sr.group_cnt = 1
          LIMIT 1
        )
      END
      ORDER BY ord
    ) AS new_subs
  FROM fleet_categories fc,
       jsonb_array_elements(fc.subs) WITH ORDINALITY AS t(e, ord)
  WHERE fc.name = '费用清单'
    AND fc.deleted_at IS NULL
  GROUP BY fc.id
)
UPDATE fleet_categories target
SET subs = CASE
      -- jsonb_agg 在「某个元素无法匹配」时会产生 NULL 元素：此时保留原值，
      -- 绝不能用 NULL 覆盖整份 subs（那等于把用户字段定义清空）。
      WHEN patched.new_subs IS NULL THEN target.subs
      ELSE patched.new_subs
    END,
    updated_at = NOW()
FROM patched
WHERE target.id = patched.id
  -- 有元素真的被改过才写回，避免无意义的 updated_at 抖动
  AND patched.new_subs IS NOT NULL
  AND patched.new_subs::text <> target.subs::text;
