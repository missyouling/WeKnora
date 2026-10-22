# 文本字段细分为单行/多行两种模式 — 设计文档

日期：2026-10-22
状态：已确认（用户选定方案 A + 自动迁移）
关联提交：待定

## 1. 问题

真机截图暴露：`备注` 字段（`data_type=text`）在详情抽屉里渲染成**单行输入框**，
长内容（如 `业务流水号:2026102215...`）被截断显示，无法完整录入与查看。

根因：类型体系只有一种 `text`，抽屉靠**硬编码字段名白名单**
（`isLongTextField`：summary/摘要/abstract/description/概述/说明）决定是否用
textarea。白名单之外的字段（备注、说明、内容摘要…）一律单行，用户无从选择。

## 2. 目标

1. 字段定义下拉把「文本」细分为 **单行文本 / 多行文本** 两个选项；
2. 多行文本字段在详情抽屉用 `<t-textarea>`（自动折行、不截断）；
3. 列表页该列**换行展示完整内容**（不再 nowrap+ellipsis 截断）；
4. 打印导出（目录 PDF）完整输出（现有 `wrapText` 已支持折行，需确认列宽策略）；
5. 数据库存储与前端渲染一致：`data_type` 是唯一权威，后端 Schema 同步识别；
6. 存量已知长文本字段自动迁移为多行文本（幂等，重启执行一次）。

## 3. 方案（已确认）

### 3.1 新增类型值 `textarea`

| 层 | 文件 | 改动 |
|---|---|---|
| 后端枚举 | `internal/types/extract_schema.go` | 新增 `FieldTypeTextarea FieldType = "textarea"` |
| 后端 catalog | `internal/application/service/business_field_catalog.go` | `Type` 字符串映射补 `textarea` → `FieldTypeTextarea` |
| 前端面板 | `FleetCertPanel.vue` | `dataTypeOptions` 把 `文本` 改名为 `单行文本`，新增 `多行文本 → textarea`；子项选项同步 |
| 前端抽屉 | `BusinessSplitDetailDrawer.vue` | 新增 `dataType === 'textarea'` 分支渲染 `<t-textarea :autosize>` |
| 前端表单 | `businessDetailForm.ts` | `DetailFieldMeta.dataType` 透传（无需改）；`isLongTextField` 白名单保留作 summary 兜底 |
| 列表列 | 四个页面 `tableColumns` | 按列的 `dataType === 'textarea'` 关掉 `ellipsis`、单元格换行 |

### 3.2 控件分支顺序（抽屉）

```
remarkPart 片段 → date → number → textarea(新) → isLongTextField(summary 兜底) → array → text
```

`textarea` 分支放在 `isLongTextField` **之前**：用户显式选择的类型优先于字段名兜底。

### 3.3 列表单元格

- `tableColumns` 对 `textarea` 列：`ellipsis: false`，宽度上限放宽（长文本需要更多宽度）；
- 单元格 class 用 `white-space: pre-wrap; word-break: break-word;`（沿用现有
  `.summary_content` 的折行策略），高度按内容自适应；
- 为避免行高失控，加 `max-height` + 悬浮展开（复用现有 summary 折叠模式的思路），
  但**默认展示全部内容**（需求是"不出现截断或显示不全"）。

### 3.4 存量自动迁移

启动幂等任务 `MigrateLongTextFieldsToTextarea`（挂在
`routes_knowledge.go` 与现有 `RepairInvoicePhysicalPages` / `CleanupOrphanExtractConfigs`
同一处）：

- 目标 scope：`contract` / `regulation` / `award_punish`（发票模块的备注类字段一并处理）；
- 迁移字段名清单（按 key 与中文名双路匹配）：
  `remark`/`备注`、`summary`/`摘要`/`内容摘要`、`description`/`说明`/`概述`、`comment`/`评论`；
- 只把 `data_type === 'text'` 改成 `'textarea'`，**不动**已是其它类型的字段；
- 同步更新 `knowledge_extract_configs.fields[].type`（`text` → `textarea`），
  保证提取 Schema 与字段定义一致；
- 迁移写日志（条数），便于验收。

### 3.5 数据库存储一致性

`data_type` 只是**渲染提示**，不改变存储格式（长文本与短文本都是 string）。
因此无需数据迁移脚本改值，只需改类型声明。列表/抽屉/导出三处都从同一个
`subs[].data_type` 取值，从构造上保证一致。

## 4. 边界与风险

| 边界 | 处理 |
|---|---|
| 存量 `text` 字段被自动迁成 `textarea` 后用户想改回单行 | 下拉可选，随时改回 |
| `textarea` 值里含换行，导出 PDF | 现有 `wrapText` 按字符宽折行，已支持 |
| `textarea` 参与关键词搜索 | 字符串包含匹配，不受影响 |
| `textarea` 走正则补洞 | `genericRegexPatch` 按字符串写入，`uniqueRegexCapture` 单值语义不变 |
| 明细子项（`sub_fields`）也加多行文本 | 子项下拉同步加选项（子项渲染走 `formatItemCell`，折行即可） |
| 发票模块 | 同链路处理（`remark`/`summary`） |
| 迁移任务重复执行 | 幂等（只改 `text` → `textarea`，第二次无行受影响） |

## 5. 验收

1. 字段定义下拉出现「单行文本 / 多行文本」，备注改选多行文本后保存；
2. 六同步：抽屉该字段变 textarea、列表该列换行完整显示、规则面板类型同步；
3. 长文本（含换行）在抽屉可完整录入、列表完整显示、目录 PDF 完整输出；
4. 重启后迁移日志显示已迁移条数，库内 `data_type` 为 `textarea`；
5. 门禁：`go vet`、后端白名单、`type-check`、`eslint`、前端单测、VM `build`。
