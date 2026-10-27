-- 回滚：从模板尾部移除本次补入的【字段与规则】小节。
-- 只删「恰好是本次追加形态」的尾部块，用户自己写过的小节不受影响。
UPDATE knowledge_extract_configs
SET prompt_template = regexp_replace(
        prompt_template,
        E'\n\n【字段与规则】\n\\{\\{fields_schema\\}\\}\n$',
        E'\n'
    ),
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND prompt_template LIKE E'%\\n\\n【字段与规则】\\n{{fields_schema}}\\n';

UPDATE knowledge_extract_config_versions
SET prompt_template = regexp_replace(
        prompt_template,
        E'\n\n【字段与规则】\n\\{\\{fields_schema\\}\\}\n$',
        E'\n'
    )
WHERE prompt_template LIKE E'%\\n\\n【字段与规则】\\n{{fields_schema}}\\n';
