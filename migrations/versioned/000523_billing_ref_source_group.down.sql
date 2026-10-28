DO $$ BEGIN RAISE NOTICE '[Migration 000523 down] source.group is additive; keeping backfilled values'; END $$;

-- 回滚策略：**不删**已回填的 source.group。
-- 理由：它只是把「本来就能从来源分类推出来的信息」补进引用描述，
-- 删掉会让取值退化成「跨分组合并求和」（金额被放大），属于制造故障而非回滚。
-- 若确实要回到旧结构，由用户在「字段配置」里逐条删除引用字段即可。
SELECT 1;
