# 发票设置 UI 现状简报 + 重构设计（2026-10-18）

## 一、UI 现状简报（供全局视觉统一参考）

### 1. 层级结构

```
SettingDrawer（@/components/settings/SettingDrawer，width 760px，hide-footer）
└── t-tabs(.standard-settings-tabs)
    ├── Tab「字段定义」→ FleetCertPanel(.meter-settings-body)
    │   ├── .group-headers        分组胶囊（标题 + 数量 + 新增/删除）
    │   ├── .meter-table-wrap
    │   │   ├── .meter-table-head grid: 30px 1.6fr 0.9fr 0.6fr 0.6fr 0.7fr
    │   │   ├── .meter-table-row × N     行点击 = 展开编辑
    │   │   │   └── .meter-row-actions   ⋮ 下拉（复制 / 删除）
    │   │   ├── .meter-form--inline      行内展开编辑区
    │   │   │   ├── .form-grid           分类名（t-input）
    │   │   │   ├── .field-config        字段表（拖拽/类型/宽度/默认/启用/⋮）
    │   │   │   ├── .sub-fields-block    明细子列（仅 data_type=items）
    │   │   │   └── .meter-form-actions  [智能生成字段] ←spacer→ [取消][保存]
    │   │   └── .cert-add-row            末尾「+ 新增分类」
    │   └── AiFieldSuggestDialog(t-dialog 720px，全屏遮罩)
    └── Tab「提取规则」→ ExtractRulePanel
        ├── .extract-toolbar   [cert 下拉][spacer][高级模式][智能生成规则][无样票][测试规则]
        ├── .ai-suggest-panel  绿色中间预览框（勾选后应用）
        ├── .rule-table        表头 + 行（点击展开 .rule-form--inline）
        └── ExtractTestDialog  (t-dialog 62%，全屏遮罩)
```

### 2. 间距 / 字号 / 配色

| 项 | 现状值 |
| --- | --- |
| 行内边距 | `7px 12px`（表格行）、`12px`（表单块） |
| 列间距 | `var(--wk-field-grid-gap)`（语义变量，随密度切换） |
| 字号 | 表格头/行 `12px`；表单标题 `13px/600`；提示文案 `12px` |
| 配色 | 全部走 TDesign token：`--td-text-color-primary/secondary/placeholder`、`--td-component-stroke`、`--td-brand-color-light`（编辑行底色）、`--td-bg-color-container` |
| 圆角 | 卡片 `8px`；按钮/输入框继承 TDesign 默认 |
| 危险态 | `theme="danger"` + `--td-error-color`；提示气泡 `theme="warning"` |

### 3. 问题清单（本次改造动因）

1. 「智能生成字段」按钮落在编辑面板左下角（`meter-form-actions` 最左），与「取消/保存」同排但语义完全不同，视觉上像主操作。
2. 字段生成是**模态框**：遮罩层盖住整页，用户看不到自己正在改哪个分类的字段；上传→识别→并入要跨三层视线。
3. 识别结果三分组列结构不统一：标准字段 `[勾选|中文名|key tag|sample]`，子列 `[勾选|key 输入|label 输入]`，扩展字段 `[勾选|name 输入|store tag|sample]`，同一份数据三种节奏。
4. 提取规则页「智能生成规则」产出落在**中间绿色预览框**，用户还要二次勾选→应用，才回到下方表格，动线绕。
5. 测试规则是独立模态框，改一行正则想验证要开关弹窗，无法同屏对照。
6. 后端 `postProcessAISuggestFields` 用 `canonicalFieldKey(name)` 作为 custom 去重键，纯中文名归一为 `""`，第一个中文扩展字段把 `seenCustom[""]` 置真，其余全部被判「与前面建议的扩展字段重名」丢弃（实测 7 项全丢）。

---

## 二、重构设计

### 2.1 后端：扩展字段 key 生成 + 去重修正

- `AISuggestCustomFieldRaw` 增加 `key`（模型可给）；`AISuggestCustomField` 增加 `key`（必给，前端只读展示）。
- Prompt 第 4 条要求模型为每个 custom_field 给出 `key`（`^[a-z][a-z0-9_]{1,31}$`）。
- 后处理 `assignCustomFieldKey`：
  1. 模型 key 合法 → 直接用；
  2. 否则按中文名查 `customFieldKeyHints`（地址/电话/开户行/账号/户号/业务名称/车牌/办理时间/复核…）→ 命中映射；
  3. 再否则 `custom_field`，按 `_2/_3` 递增到唯一。
  4. 唯一性同时避开：契约键、子列键、其它 custom key。
- 去重身份：`dedupKey = "k:"+key`（有 key 时）或 `"n:"+name`（无 key 时），**空 key 绝不当成"重复"**；同名（name 完全相等）才算重复。
- 新增 `FleetCategorySub.Label`（`json:"label,omitempty"`），承载用户自定义中文表头；旧数据为空串，前端回退静态字典。

### 2.2 前端：字段定义入口内嵌化

- `AiFieldSuggestDialog.vue` → `AiFieldSuggestPanel.vue`（同逻辑，根节点由 `t-dialog` 换成 `<section>`，去掉遮罩）。
- 锚点：`.meter-form--inline` 内，`发票类型` 行之后、`.field-config` 之前。
- 入口：分类行 `⋮` 菜单新增「智能生成字段」→ 展开该行编辑面板 + 自动展开 AI 面板。
- 新增分类：`commitAdd` 成功后自动 `toggleEdit(新记录)` + AI 面板默认展开。
- 移除 `meter-form-actions` 里的按钮。

### 2.3 前端：三分组四列

统一网格 `grid-template-columns: 22px minmax(120px,.8fr) minmax(120px,1fr) 96px minmax(96px,1fr)`：
`勾选 | 字段编码(只读) | 字段名称(t-input) | 数据类型(t-select) | 样本(tooltip 淡字)`

- 标准字段：key = 契约键只读；label 可改 → 写 `EditableField.label`（契约键不变）。
- 扩展字段：key = 后端生成的 snake_case 只读；label 可改。
- 明细字段：key 只读；label/type 可改。

勾选明细子列且当前无 `items` 字段时：`mergeAiFieldSuggestions` 自动补 `{name:'items', dataType:'items', enabled:true}` 父字段，子列挂上去，不再出现「无法保存」警告。

### 2.4 前端：提取规则页

- 工具栏：`智能生成规则`/`测试规则` 改为 icon button + tooltip（`magic` / `beaker` 图标 + 短标签），保留高级模式开关。
- 批量生成：结果**直接回填** `fields`（只填空值），更新行加 `ai-fill` 高亮 + `AI` 徽章，自动展开第一行更新项，提示「已填充 N 项，核对后保存生效」。
- 单字段生成：每行 ⋮ 前后加 `✨` icon button（`aiFieldBusy` 按行 key），传单字段 + `overwrite:true`，只刷新该行。
- 测试：`ExtractTestDialog` → `ExtractTestPanel.vue`（内嵌折叠面板，复用 TestInputPanel/TestResultPanel），锚点在工具栏与规则表之间。

---

## 三、任务拆解（TDD）

| # | 任务 | 测试 |
| --- | --- | --- |
| B1 | 后端 custom key 生成 + 去重 | `TestExtractAIConfig_CustomFieldsChineseKeysUnique`（7 中文项全存活且 key 各异）、模型 key 合法直用、契约冲突加后缀 |
| B2 | `FleetCategorySub.Label` | `go vet`（omitempty 向后兼容） |
| F1 | fleetCategorySubs label 往返 | `FleetCertPanel.test.ts` 往返断言含 label |
| F2 | merge：custom.key + 自动 items 父字段 + label/type | `aiFieldMerge.test.ts` |
| F3 | 内嵌 AI 面板（去弹窗） | 源码断言：无 `t-dialog`、存在 `ai-field-panel`、菜单含「智能生成字段」 |
| F4 | 三分组四列 | 源码断言：统一 grid 类 + 只读 key + 可编辑 label/type |
| F5 | 规则页直接回填 / 单字段 / 内嵌测试 | `ExtractRulePanel.test.ts` 断言无 `ai-suggest-panel`、有 `ai-field-gen`、无 `ExtractTestDialog` |
| V | VM 同步 + Playwright 截图 + 门禁 | vue-tsc / eslint / 单测 / VM go test |
