# 发票设置 9 项验收整改 — 实施计划（2026-10-18 第二轮）

> 前置：第一轮重构落地（内嵌字段生成、三分组、规则直接回填）见
> `docs/plans/2026-10-18-invoice-settings-ui-refactor.md`。本轮为真机验收后的精修。

## 数据结构扩展（前后端契约）

`FleetItemSubField`（`internal/types/fleet.go`）新增：

| 字段 | 类型 | 语义 |
|---|---|---|
| `enabled` | `*bool` | 子项启用开关；`nil` = 启用（兼容存量 JSON 无该键） |
| `desc` | `string` | 子项业务含义（进 Prompt） |
| `rule` | `string` | 子项自然语言提取规则（进 Prompt） |
| `regex` | `string` | 子项后置正则兜底（RE2） |

`ExtractFieldConfig.SubFields` 复用同一结构，因此规则面板保存到 extract_config 的
子项规则会随字段配置持久化。

**关键约束**：`injectInvoiceItemSubFields` 每次提取前会用分类配置覆盖
`cfg.Fields[i].SubFields`。必须改成「结构取分类、规则取配置」的按 key 合并，
否则用户在规则面板配的子项规则下一次提取就被抹掉。

## 后端任务

- **B1 契约扩展**：`FleetItemSubField` 加 enabled/desc/rule/regex；加 `IsEnabled()`。
- **B2 注入合并**：`injectInvoiceItemSubFields` 按 key 保留 desc/rule/regex/enabled。
- **B3 Prompt**：`invoiceItemsSubFieldsInstruction` 把子项 desc/rule 写进明细行要求。
- **B4 全字段补洞**：
  - 文本标准字段：invoice_type/remark/buyer_*/seller_*/category…
  - 数值标准字段：total_amount/amount/tax/tax_rate —— 捕获值走统一清洗
    （去 ¥￥$,空格，千分位）再 `strconv.ParseFloat`，失败则放弃。
  - 自定义扩展字段：`InvoiceExtractionItem.Fields[name]`（map 桶）。
  - 明细子项：单票**单行**明细时写 `Items[0].Extra[key]`；多行不猜。
  - 铁律不变：只补空、同批唯一、值未被他票占用。
- **B5 有无样本差异化**：`ai-suggest-rules` 无 `sample_text` → Prompt 不要求正则，
  且所有 `regex` 强制清空 + `rejected_no_sample`；有样本才走 compile+唯一命中。
- **B6 新建分类默认字段集**：invoice scope 且 subs 为空时注入标准字段集
  （含 extractStatus/tags，均启用）。
- **B7 内置子项标记**：seed 的子列写 `enabled=true`；新增幂等回填给存量分类
  打 builtin 标记（供前端禁用删除）。

## 前端任务

- **F1 项目明细行内展开**：删除底部 `.sub-fields-block`；items 字段行点击展开，
  子项行复用主字段网格（拖动列占位 / 名称 / 类型 / 宽度 / 默认表头留空 / 启用 / 删除），
  隐藏 key 输入，新增子项自动生成 `sub_N` 键。
- **F2 内置字段/子项禁删**：标准契约键与外置 builtin 子项隐藏删除入口，锁定图标 + Tooltip。
- **F3 字段生成面板**：「粘贴文本」并入按钮行（选择文件右侧）；标题与菜单项统一
  「字段生成」；删除「收起」；删除大段绿色提示与分组技术黑话。
- **F4 保存字段一步落库**：主按钮「保存字段」→ 合并 → 直接调保存接口 →
  单个成功提示 → 引导弹窗；删除双 Toast。
- **F5 AI 图标统一**：`star` → `ai-1`（TDesign AI 系列，含四角星），
  用于菜单项、面板标题、AI 开关按钮、规则页工具栏与行内按钮。
- **F6 滚动修复**：面板 flex 子项 `min-height: 0`，结果区可滚。
- **F7 引导弹窗**：复刻创建知识库向导风格（圆角卡片、13px/600 标题、次级说明、
  主次按钮 8px 间距），替换 DialogPlugin 默认样式。
- **F8 规则页**：解除 8 字段限制（去掉告警条与置灰）；items 行展开区增「明细子项规则」
  分组（子项 desc/rule/regex）；工具栏去边框（variant=text，hover 柔和底色）、
  清空图标换 `refresh`；删除行尾箭头；高级模式改「左侧文字 + 右侧 medium 开关」。
- **F9 文案精简**：字段定义/字段生成/提取规则三处说明文字收敛到一行以内。

## 测试与门禁

- Go：`extract_regex_fallback_test.go` 覆盖全字段/数值清洗/自定义桶/子项单行；
  `extract_suggest_ai_test.go` 覆盖无样本不生正则；`fleet_test.go` 覆盖默认字段集与内置标记。
- 前端：`fleetCategorySubs.test.ts`（子项新字段往返）、`FleetCertPanel.test.ts`
  （行内展开、禁删、AI 开关、单次 Toast）、`ExtractRulePanel.test.ts`（去告警/子项规则/无箭头）、
  `AiFieldSuggestPanel.test.ts`（粘贴按钮、保存字段、无提示框）。
- 门禁：VM `go vet` + CI 白名单；前端 `vue-tsc` 0 error / `eslint` 0 error / 单测。
- 部署 VM + Playwright 截图核验 9 项。
