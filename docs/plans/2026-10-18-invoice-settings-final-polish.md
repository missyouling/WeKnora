# 发票设置 · 10 项验收整改实施计划（2026-10-18 终轮）

> 承接 `2026-10-18-invoice-settings-polish.md`（9 项）。本轮为真机逐页验收后的收口。

## 一、问题到文件的映射

| # | 问题 | 主要文件 | 关键点 |
|---|------|----------|--------|
| 1 | 非标准建议字段被丢弃 + 橙色告警框 | `internal/application/service/extract_suggest_ai.go`、`AiFieldSuggestPanel.vue` | `postProcessAISuggestFields` 把白名单外 key 降级为 custom_fields（英文键→中文名映射），前端删除 skipped 告警块 |
| 2 | 内置字段范围收敛到 3 个 | `internal/handler/fleet.go`、`FleetCertPanel.vue` | 仅 `invoiceType`/`extractStatus`/`tags` 标 builtin；明细子项一律可删 |
| 3 | 明细浮层列随子项启用联动 + 宽度自适应；items 行去掉「默认表头」 | `InvoiceManagement.vue`、`FleetCertPanel.vue` | `subFieldsOfRow` 过滤 `enabled !== false`；浮层宽度 = 启用列宽之和 clamp；items 行不渲染默认表头开关 |
| 4 | 删除统一冒泡确认 | `FleetCertPanel.vue` | 明细子项删除补 `<t-popconfirm>`（分类/字段/分组已有） |
| 5 | items 行不显示箭头 | `FleetCertPanel.vue`（`ExtractRulePanel.vue` 已无） | 去掉 `.fc-caret`，保留「项目明细 + 计数」可点 |
| 6 | 六同步 | `ExtractRulePanel.vue`、`invoiceFieldLabels.ts`、`extractFieldMeta.ts`、`StandardSettingDrawer.vue` | 规则面板监听 `fleet-categories-changed`；label 取分类 `subs[].label`；键名归一 |
| 7 | 粘贴文本按钮对齐 + 图标 | `AiFieldSuggestPanel.vue` | 同尺寸 outline 按钮 + `paste` 图标，容器 inline-flex |
| 8 | 工具栏 ticket 图标 + 悬停绿 + 样票缓存常驻绿 | `ExtractRulePanel.vue` | `ticket` 图标；`.tb-icon-btn:hover` 绿；`sampleTextAvailable` → `.is-hot` |
| 9 | 测试面板整改 | `ExtractTestPanel.vue`、`TestInputPanel.vue`、`TestResultPanel.vue`、`FleetCertPanel.vue`、`StandardSettingDrawer.vue` | 右栏滚动 + 对齐；删「收起/关闭」；文本 Tab 不渲染 VLM 原文；字段映射中文名；保存后直接跳转 + Toast，移除引导弹窗 |
| 10 | 裸露 `invoice_code` | `invoiceFieldLabels.ts`、`extractFieldMeta.ts`、`extract_suggest_ai.go`、`aiFieldMerge.ts`、`ExtractRulePanel.vue` | snake_case→中文标签；custom key 与契约键 canonical 冲突时归入标准字段；DEFAULT_RULES 三形态查找 |

## 二、键名归一策略（六同步的根）

三层统一，缺一不可：

1. **展示层**：`invoiceFieldLabel(name)` 依次尝试 原名 → camelCase → snake_case；
   `INVOICE_FIELD_LABELS` 补齐全部 snake_case 键。
2. **合并层**：`aiFieldMerge` 对扩展字段做 canonical 比对，
   命中契约键则按标准字段（camelCase 契约键）并入，不产生 `invoice_code` 与 `invoiceCode` 两行。
3. **后端建议层**：`postProcessAISuggestFields` 里 custom 字段的 key/name canonical 命中契约键
   → 提升为 standard_fields。

## 三、执行顺序（每步先写测试）

1. 后端 B1：建议字段降级/提升 + 单测。
2. 后端 B2：builtin 收敛到 3 + 单测。
3. 前端契约 C1：`invoiceFieldLabels`（snake_case）+ `extractFieldMeta`（canonical 兜底）+ 单测。
4. 前端契约 C2：`aiFieldMerge` canonical 归入标准 + 单测。
5. `FleetCertPanel`：内置 3、去箭头、hide 默认表头、子项删除确认、保存后直跳。
6. `AiFieldSuggestPanel`：删告警框、按钮对齐加图标。
7. `ExtractRulePanel`：ticket 图标/绿态、label 取分类、监听广播、DEFAULT_RULES 三形态。
8. 测试面板三件套 + 明细浮层联动宽度。
9. 门禁：`go vet` + CI 白名单、`vue-tsc`/`eslint`/单测；VM 部署 + Playwright 10 项截图。

## 四、验收口径

- 内置字段 = 3 个；其余全部可删（含明细子项）。
- 识别结果不再出现橙色「未被采纳」框；被降级的字段出现在「扩展字段」组且中文名正确。
- 明细浮层只显示启用的子项，宽度随列数与列宽变化。
- 提取规则表格无裸英文（含 `invoice_code`）。
- 保存字段后直接停在「提取规则」并只弹一条 Toast。
