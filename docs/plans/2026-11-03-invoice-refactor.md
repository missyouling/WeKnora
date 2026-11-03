# 发票管理 O-1~O-8 重构实施计划

> 来源：`docs/invoice-module-review-2026-11-03.md`（用户已确认全部决策）
> 日期：2026-11-03
> 铁律：不动 `docreader` / 知识库 pipeline / `internal/models` / `internal/core`

## 前置结论（取证）

- 三页工具栏结构**完全同源**（合同页注释写「发票基准同款」），是复制粘贴产物。
- 三页实际用的是 `BusinessFilterBar`（内部再包 `BusinessListToolbar`），
  `hide-batch-bar` / `clear-selection` 已透传，但 **`batch-actions` slot 未透传**。
  这是 O-1 的唯一阻塞点，必须先补。
- 中台 `.doc-batch-bar-fixed` 用 `--wk-batch-bar-bottom` / `--wk-batch-bar-z` 变量定位，
  三页自写的 `.doc-batch-bar` 是另一套 `bottom: 24px`（需核对，避免位置跳动）。

## 任务分解（每项：写测试 → 失败 → 实现 → 通过 → 提交）

| # | 任务 | 文件 | 验收 |
|---|---|---|---|
| T1 | `BusinessFilterBar` 透传 `batch-actions` slot | `BusinessFilterBar.vue` | 源断言：存在 `<slot name="batch-actions">` 且包在 BusinessListToolbar 内 |
| T2 | 发票页工具栏迁移中台（移除 `hide-batch-bar` + 自写壳） | `InvoiceManagement.vue` | 源断言：无 `class="doc-batch-bar"`；有 `#batch-actions`；`hide-batch-bar` 消失 |
| T3 | 合同页同上 | `ContractManagement.vue` | 同上 |
| T4 | 奖惩页同上 | `AwardPunishManagement.vue` | 同上 |
| T5 | `businessFormat` 委托 `formatNumber` | `businessFormat.ts` | 单测：`formatAmount(1234.5) === money2(1234.5)`；`formatRate(0.09) === rateText(0.09)` |
| T6 | 删合同页本地 `formatAmount/formatRate` | `ContractManagement.vue` | 源断言：无本地定义；type-check 通过 |
| T7 | `filterFields` 从 subs 派生（车牌号试点） | `InvoiceManagement.vue` + 新 `invoiceFilterFields.ts` | 单测：`buildInvoiceFilterFields(subs)` 对含 `plate_no` 的分类产出 select；不含则不产出 |
| T8 | 删 `sanitizeNum` 与其测试 | `businessDetailForm.ts` + test | 源断言：全局无 `sanitizeNum` |
| T9 | 抽 `makeFilePlaceholder` | `InvoiceManagement.vue` | 单测：三种 kind 的 status/error 映射正确 |
| T10 | 抽 `BusinessEmptyState` 并接 4 页 | 新组件 + 4 页 | 源断言：4 页均引用；真机视觉一致 |
| T11 | `.row-page-badge` 中性色 | `InvoiceManagement.vue` | 源断言：不含 `--td-success-color`；含 `--td-bg-color-container-hover` |
| T12 | 清理 2 处「已下线」注释 | `InvoiceManagement.vue` | 源断言：无「底部摘要栏已下线」 |
| T13a | 后端：保存时重算 `extract_issues` | `knowledge_business.go` / 新 service 函数 | 单测：保存后字段补全 → issues 清空；仍有缺失 → 保留对应项 |
| T13b | 存量：确定性重算全库 issues（不调模型） | 新运维脚本 `scripts/refresh_invoice_issues.py` | dry-run 报告 + 写库后 `extract_issues` 与当前字段一致 |
| T14 | 全量门禁 + VM build + 真机 3 项断言 | — | go vet / 单测 / type-check / lint / 761+ / build EXIT=0 / Playwright |

## 关键约束（踩过的坑，实施时必须遵守）

1. **不用 `slice` 删模板块**：`doc-batch-bar` 内部有嵌套 `t-popconfirm`，必须按
   `<div class="doc-batch-bar"` → 配对的 `</div>` 精确定位（用缩进 + 下一个兄弟节点判断），
   否则会截断弹窗。
2. **`clearSelectionSafe` vs `clearSelection`**：发票页有三处删除确认弹窗状态
   （`delPopVisible`），清除选择时走 `clearSelectionSafe`（会关弹窗）。迁移后
   `@clear-selection` 必须绑 `clearSelectionSafe`，不能绑裸 `clearSelection`。
3. **筛选器派生要保留原有 3 项**：关键词/税率/开票日期是通用项，分类专属字段追加在
   `taxRate` 之后、`dateRange` 之前（视觉顺序：搜索｜分类字段｜税率｜日期）。
   无分类或分类无 `filterable` 字段时不产出，保持向后兼容。
4. **`extract_issues` 重算必须用「已落库字段」而非重新解析**：不跑模型、不读 PDF，
   只对 `custom_metadata.invoices` 现有值跑 `validateInvoiceExtraction`。
   锚点类告警按 `SuppressInvoiceAnchorWarning` 同口径抑制（单票必抑制）。
5. **保存联动放在后端**，不在前端算：前端只提交字段，后端保存后重算并回写
   `extract_issues`，返回给前端刷新详情。否则多端不一致。

## 视觉核对点

- 中台 `.doc-batch-bar-fixed`：`bottom: var(--wk-batch-bar-bottom)`；
  三页自写 `.doc-batch-bar` 需确认 `bottom` 值一致，迁移后真机对比截图。
- 中台「清除」按钮 `variant="text"` / `theme="default"`，与三页原实现一致（已核对）。
- 中台有 `batch-bar-fade` transition，三页原实现可能没有 → 迁移后多一个淡入，属可接受增强。
