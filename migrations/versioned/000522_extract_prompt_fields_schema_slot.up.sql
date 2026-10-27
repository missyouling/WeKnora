DO $$ BEGIN RAISE NOTICE '[Migration 000522] backfill {{fields_schema}} slot in advanced prompt templates'; END $$;

-- 高级模式的字段规则只通过 {{fields_schema}} 插槽注入。
-- 早期版本保存/seed 的模板里常缺这个插槽，导致「字段定义里配的规则」在提示词里
-- 完全不出现：界面上只挂一条黄色告警，提取却静默降级成一份没有字段约束的提示词。
--
-- 这里只做**幂等补齐**，绝不改写模板其它内容，也不会覆盖用户已写的任何文字：
--   · 已含插槽 → 一字不动（WHERE prompt_template NOT LIKE '%{{fields_schema}}%'）；
--   · 不含插槽 → 追加一个【字段与规则】小节承载插槽（读起来与默认模板同构）。
UPDATE knowledge_extract_configs
SET prompt_template = rtrim(prompt_template, E'\n')
    || E'\n\n【字段与规则】\n{{fields_schema}}\n',
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND btrim(prompt_template) <> ''
  AND position('{{fields_schema}}' in prompt_template) = 0;

-- 版本快照同样补齐：回滚到旧版本时不应再退回"没有插槽"的模板。
UPDATE knowledge_extract_config_versions
SET prompt_template = rtrim(prompt_template, E'\n')
    || E'\n\n【字段与规则】\n{{fields_schema}}\n'
WHERE btrim(prompt_template) <> ''
  AND position('{{fields_schema}}' in prompt_template) = 0;
