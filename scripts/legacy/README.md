# scripts/legacy — 历史一次性脚本存档

本目录存放 2026-10-16 仓库清理时从项目根目录归档的**一次性数据修复 / 备份 / 诊断脚本**。
它们已完成使命，保留仅为备查与复用参考，**不参与任何构建或 CI 流程**。

> ⚠️ 其中部分脚本含硬编码的库表名、租户 ID 或历史数据假设，直接执行前务必先读代码核对。

## 数据修复类

| 脚本 | 用途 |
|------|------|
| `backfill_buyer_seller.py` | 回填买卖方字段 |
| `fix_multi_tax.py` | 修复多税率提取结果 |
| `tmp_fix_rules.py` | 修复提取规则配置 |
| `reextract_failed.py` | 对失败知识库触发重新提取 |

## 备份 / 恢复类

| 脚本 | 用途 |
|------|------|
| `backup_9d3f1b3a.py` | 生成备份（配套 `backup_9d3f1b3a.json`） |
| `restore_knowledge.py` | 从备份恢复知识库 |
| `backup_rules_20261014.json` | 2026-10-14 提取规则快照 |

## 诊断 / 排查类

| 脚本 | 用途 |
|------|------|
| `check_cos.py` / `debug_cos_sig.py` | COS 配置与签名排查 |
| `check_extract.py` | 提取状态检查 |
| `check_state.py` | 通用状态检查 |
| `cmp_sha1.py` | 文件 SHA1 比对 |
| `tmp_presign_check.go` | COS 预签名 URL 校验（Go） |

## 使用建议

- 需要类似能力时，**优先参考本目录旧实现再写新的**，避免重复踩坑
- 确认无用后可整体删除本目录
