# 四模块前端页面统一改造方案

> 调研日期：2026-10-12
> 基准：`InvoiceManagement.vue`（2619 行，AGENTS.md 指定的"黄金参考基准"）
> 目标：合同 / 制度 / 奖惩三个管理页与发票页**结构、布局、交互模式**一模一样（业务字段本身保留差异）
> 约定：只做调研与方案，不改代码；实施时每项只 commit，攒到合适时机再请示 push

---

## 一、发票页面组件结构清单（基准）

### 1.1 import 清单（script 424–471 行）

| 组件 | 路径 |
|---|---|
| BusinessKbWizard | `./BusinessKbWizard.vue` |
| FleetUploadHistoryDrawer | `./FleetUploadHistoryDrawer.vue` |
| DashboardKpiGroup | `@/components/business/DashboardKpiGroup.vue` |
| StandardSettingDrawer | `@/components/business/StandardSettingDrawer.vue` |
| FleetUploadDialog | `./FleetUploadDialog.vue` |
| SettingDrawer | `@/components/settings/SettingDrawer.vue` |
| BusinessColumnFilter | `./BusinessColumnFilter.vue` |
| BusinessListToolbar | `./BusinessListToolbar.vue`（**显式 import**） |
| DocumentPreview | `@/components/document-preview.vue` |
| TagEditDialog | `@/views/knowledge/components/TagEditDialog.vue` |
| KbTagManageDrawer | `@/views/knowledge/components/KbTagManageDrawer.vue` |

另：`invoiceFieldLabels.ts`（字段名归一，已接入）、`useBusinessList` / `useBusinessPolling` / `useDocStatus` 三个 composable。

### 1.2 template 布局顺序（自上而下）

```
div.invoice-management-container
├─ div.header：标题「发票管理」+ 副标题（无按钮，上传在工具栏）
├─ div.loading-area（kbId 未加载）
├─ div.empty-area（无 KB：t-empty + 创建知识库按钮）
├─ div.invoice-main（主界面）
│   ├─ ① DashboardKpiGroup（:cards + :active-type + @card-click）
│   ├─ ② BusinessListToolbar（hide-batch-bar）
│   ├─ ③ t-table（sticky-header，row-select 46px + 序号 44px）
│   ├─ ④ div.doc-summary-bar（底部固定汇总）
│   └─ ⑤ div.doc-batch-bar（自有 fixed 悬浮批量条，选中行时出现）
└─ 弹窗/抽屉区（按序）
    ├─ BusinessKbWizard（kb-name="日常事务-发票"）
    ├─ FleetUploadHistoryDrawer（scope="invoice"）
    ├─ StandardSettingDrawer（无 scope，@saved="() => reloadColumns(true)"）
    ├─ FleetUploadDialog（scope="invoice"，唯一上传入口）
    ├─ SettingDrawer 详情抽屉（654px，可拖宽记忆，hide-footer，destroy-on-close）
    ├─ t-dialog（单号重复二次确认）
    ├─ TagEditDialog / KbTagManageDrawer
    └─ teleport 打印预览（print/catalog 双模式）
```

**顺序口诀：KPI → 工具栏 → 表格 → 汇总条 → 自有批量浮条。**

### 1.3 KPI 卡

- 数据源：**后端** `getInvoiceOverviewStats(kbId)` → `{total, current_month, file_count, parse_failed, extract_failed, by_category[]}`，全量聚合不受筛选影响。
- 卡片：①「已收录发票」总卡（sub 本月新增 N 张，点击清空类型筛选）→ ②按 `invoiceCats`（`listFleetCategories({scope:'invoice'})`）动态分类卡（点击联动 `filterInvoiceType`，`active-type` 高亮）→ ③「其它票据」排除式桶（total − 各分类之和，>0 显示）→ ④「历史记录」卡（上传文件总数，异常数标红，点击开 `FleetUploadHistoryDrawer`）。

### 1.4 工具栏（BusinessListToolbar）

- `hide-batch-bar`（隐藏中台内置批量条，用自有 fixed 条）；`v-model:keyword`（300ms 防抖）+ `@refresh` + `:selected-count` + `@clear-selection`。
- slots：`#type-extra`（税率下拉 + 开票日期范围；**无类型下拉**，类型筛选只靠 KPI 点击）、`#columns`（BusinessColumnFilter）、`#right-extra`（设置 + 上传发票）；**不用 `#batch-actions`**。

### 1.5 表格

- 首列复选框 `{colKey:'row-select', type:'multiple', width:46}` + 序号列 44px；服务端排序；`@row-click` 开详情（checkbox 区除外）。
- 发票号码列带**页码徽标**（多页文件显示 `P{n}`/`n/m`）；税率多档 tag；状态 tag；标签可点击编辑。

### 1.6 自有批量浮条（`.doc-batch-bar`）

重新解析（`reparseKnowledge`，物理重拆页） / 重新提取（`extractBusinessDocument` 带当前分类） / 单页打印 / 列表打印 / 删除（二选一气泡：删除此页 `deleteInvoicePage` / 删除文件 `batchDeleteKnowledge`，多页先删大页码）。

### 1.7 其它

- 详情抽屉：先 `fillEditForm()` 再开抽屉（防闪烁），按分类 subs 动态字段 + items 明细可编辑，源文件预览默认折叠**不挂载**，底部手动保存。
- 保存：手动；`persistEditForm` 整包写回 `custom_metadata.invoices`；同文件类型级联；409 `DUPLICATE_INVOICE_NO` 二次确认。
- 发票独有（**不复制**）：按页提取/删除、remark 片段字段、自定义 fields 扩展桶、明细 items。

---

## 二、三模块差异对比表

| 维度 | 发票（基准） | 合同 | 制度 | 奖惩 |
|---|---|---|---|---|
| 行数 | 2619 | 2192 | 1791 | 1770 |
| 布局顺序 | KPI→工具栏→表格 | KPI→工具栏→表格 ✓ | 工具栏→KPI→表格 ✗ | 工具栏→KPI→表格 ✗ |
| KPI 数据源 | 后端 overview | 后端 `listContractOverview` ✓ | 前端本地计算 ✗ | 前端本地计算 ✗ |
| KPI 可点击联动 | 是（+active-type） | 是 | 否 | 否 |
| BusinessListToolbar import | 有 | 有 | **无（P0）** | **无（P0）** |
| hide-batch-bar（自有浮条） | 是 | 否（用内置） | 否（用内置） | 否（内置，但无复选框→死功能） |
| 表格复选框列 | 有 | 有 | **无（P0）** | **无（P0）** |
| 工具栏类型下拉 | 无（KPI 联动） | 有 `:type-options` | 有 | 有 |
| 重新解析按钮 | 有（独立） | 无（只有重新提取） | 无 | 无 |
| 上传入口 | FleetUploadDialog 弹窗 | FleetUploadDialog 弹窗 ✓ | header 隐藏 input 直传 ✗ | header 隐藏 input 直传 ✗ |
| 历史抽屉 | FleetUploadHistoryDrawer（上传历史） | 同左 ✓ | DeletedKnowledgeDrawer（删除历史） | DeletedKnowledgeDrawer（删除历史） |
| StandardSettingDrawer | 无 scope，有 @saved | scope='contract'，有 @saved ✓ | scope='regulation'，**无 @saved** | scope='award_punish'，**无 @saved** |
| 详情抽屉宽度/结构 | 654px，动态字段+折叠预览 | 700px，6 硬编码分组+折叠预览 | 700px，4 分组+预览直接挂载 | 700px，分组+预览+手动保存栏 |
| 打印按钮文案 | 单页打印/列表打印 | 单页打印/列表打印 ✓ | 打印/目录 ✗ | 打印/目录 ✗ |
| 重复单号确认 | 有 | 有 | 有 | 有 |
| 标签编辑/管理 | 有 | 有 | 有 | 有 |
| 各自特有（保留） | 按页操作 | 履约状态派生列、即将到期 KPI、固定三类卡 | 目录批量 PDF、编制日期筛选 | 措施多选联动、恢复后自动开编辑 |

**结论：合同最接近发票（约 85% 对齐），制度/奖惩约 60% 对齐，且各有两个 P0 级残缺。**

---

## 三、改造清单（按优先级）

### P0：运行时阻断（先修，否则页面残缺）

| # | 模块 | 内容 | 说明 |
|---|---|---|---|
| P0-1 | 制度、奖惩 | 补 `import BusinessListToolbar from './BusinessListToolbar.vue'` | template 在用但 script 未 import，也无全局注册/自动导入插件；运行时工具栏无法解析 |
| P0-2 | 制度、奖惩 | `tableColumns` 首列补 `{colKey:'row-select', type:'multiple', width:46}` | 无复选框列则用户无法选中行，全部批量操作（重新提取/打印/删除）无 UI 入口，当前是死功能 |
| P0-3 | 奖惩 | 修 KPI「本月新增」恒为 0：`r.issueDate \|\| r.apDate` → `r.signDate` | 字段名写错，`issueDate`/`apDate` 在行对象上根本不存在 |

### P1：结构对齐（达到"一模一样"的核心）

**三个模块通用：**
| # | 内容 | 参照发票 |
|---|---|---|
| P1-1 | 布局顺序改为 KPI → 工具栏 → 表格 | 发票 1.2 |
| P1-2 | 传 `hide-batch-bar`，改用自有 fixed 批量浮条（含"已选 N 项/清除"） | 发票 1.6；删除只保留"删除文件"，不要按页二选一 |
| P1-3 | 上传统一用 `FleetUploadDialog`（scope=regulation/award_punish），删除 header 隐藏 input 直传 | 发票 1.2；需配 `:type-options`（`regulation__`/`award_punish__` 复合值） |
| P1-4 | `StandardSettingDrawer` 补 `@saved="() => reloadColumns(true)"` | 发票 1.2（制度、奖惩缺） |
| P1-5 | 增加独立的"重新解析"按钮（`reparseKnowledge`），与"重新提取"分离 | 发票 1.6（合同的"重新提取"实际调的是 reparse，可直接拆分为两个按钮） |
| P1-6 | 打印按钮文案统一为「单页打印 / 列表打印」 | 制度、奖惩当前是「打印/目录」 |
| P1-7 | KPI 改后端统计 + 卡片可点击联动（`@card-click` + `:active-type`） | 见后端清单 |
| P1-8 | 详情抽屉源文件预览改默认折叠、折叠时不挂载 DocumentPreview | 发票 1.7（制度当前直接挂载） |

**后端清单（配合 P1-7）：**
| # | 内容 |
|---|---|
| P1-B1 | 新增 `listRegulationOverview` 接口（仿 `listContractOverview`）：total / current_month / by_type[] 全量聚合 |
| P1-B2 | 新增 `listAwardPunishOverview` 接口：同上口径 |
| P1-B3 | 路由注册 + 前端 API 函数 |

**合同特有：**
| # | 内容 |
|---|---|
| P1-C1 | 删 `@print="handleBatchPrint"` 死绑定（BusinessListToolbar 无打印按钮，该 handler 永不触发） |
| P1-C2 | 接入 `contractFieldLabels.ts`（文件已存在 114 行但零引用，Phase 3 命名统一待办） |
| P1-C3 | 删 `delKnowledgeDetails` 未使用 import（按页删除已废弃） |

**奖惩特有：**
| # | 内容 |
|---|---|
| P1-A1 | `primary-action-text="新建奖惩"` 改名或改行为：当前点击打开的是**建库向导**，文案误导（制度页同名按钮同样问题，建议统一改为"创建知识库"或去掉 primary-action） |

### P2：细节打磨

| # | 模块 | 内容 |
|---|---|---|
| P2-1 | 制度 | 删 stale 文案「字段修改后将自动保存」（详情抽屉 329–332 行，与手动保存矛盾） |
| P2-2 | 三模块 | 详情抽屉宽度统一为 654px（跟发票）或统一为 700px（二选一，需定） |
| P2-3 | 合同、发票 | header 注释"标题+上传按钮"与实际不符（上传在工具栏），更新注释 |
| P2-4 | 三模块 | 详情抽屉字段分组保持业务硬编码（合同 6 分组/制度 4 分组/奖惩分组），**不**改成发票的动态 subs 渲染——见风险点 |

---


## 主人决策（2026-10-12 18:09）

1. **历史抽屉**：移除各页工具栏的独立历史入口，统一走发票模式——"历史记录"KPI 卡即历史抽屉入口。
2. **类型筛选**：发票只靠 KPI 卡点击，其它模块也一样——删工具栏类型下拉。
3. **详情抽屉**：暂时只统一外壳（宽度/footer/预览折叠/保存流程），字段分组后面再细化。
4. **目录 PDF**：四个模块都加上（含发票、合同）。

## 四、风险点与需主人决策事项

### 需主人拍板

1. **历史抽屉取舍**：发票/合同用 `FleetUploadHistoryDrawer`（上传文件的解析/提取状态），制度/奖惩用 `DeletedKnowledgeDrawer`（删除文件的查看与恢复）。两者功能不同，不互斥。
   - 推荐：**都保留**，并给发票/合同也加上 `DeletedKnowledgeDrawer`（删除历史是 Phase 3 统一的二开标准，发票页反而缺了）；或维持现状不折腾。请主人定。
2. **类型筛选位置**：发票=只靠 KPI 卡点击（工具栏无类型下拉）；三模块=工具栏有 `:type-options` 下拉。完全照搬发票要删下拉，但合同的"其它合同"是排除式选项，下拉里天然没有对应项，删下拉改动较大。
   - 推荐：**渐进式**——先给三模块 KPI 卡加上点击联动（P1-7），工具栏下拉暂时保留，两者双向同步。等用顺了再决定是否删下拉。
3. **详情抽屉字段渲染**：发票是按分类动态字段 + items 明细；三模块是硬编码分组。完全统一 = 把三模块的表单改成动态渲染，约等于重写详情抽屉，风险高收益低。
   - 推荐：**只统一抽屉 chrome**（宽度、footer、预览折叠、保存流程），字段分组保持业务现状（P2-4）。
4. **目录 PDF 反向对齐**：制度/奖惩的「目录」批量功能（按可见列生成 A4 清单 PDF）发票没有。是否给发票/合同也加上？请主人定（P3，不阻塞）。

### 风险点

- **KPI 后端接口是最大工作量**：`listRegulationOverview` / `listAwardPunishOverview` 要仿 `listContractOverview` 做全量聚合（total/current_month/by_type），口径必须与前端现有卡片（总数/月新增/类型分布）对齐；上线前用真实数据核对一次。
- **批量条改造注意**：自有 fixed 条照搬发票 `.doc-batch-bar` 样式与逻辑即可，但**不要**复制删除二选一气泡（三模块无按页删除，确认框走普通二次确认）。
- **FleetUploadDialog 接入**：需要 `type-options` 与后端分类体系打通（`listFleetCategories({scope})`）；制度/奖惩目前上传走直传，需确认分类体系是否已建，否则上传弹窗的分类下拉是空的。
- **P0-1 的运行时验证**：制度/奖惩工具栏未 import，理论上运行时不渲染。建议修之前先在隧道里打开两页肉眼确认一次（如果 actually 渲染正常，说明有未知注册机制，import 补上也无害）。
- **回归范围**：三个页面都动工具栏/表格/抽屉，改完必须 `go vet` + 前端 `type-check`（4GB 堆）+ 隧道走查一遍主流程（上传→提取→编辑保存→删除）。

---

## 五、实施顺序建议

1. **P0 修复**（import + 复选框列 + 本月新增 bug）→ commit（约 0.5h）
2. **合同对齐**（最接近发票：批量条、重新解析按钮、死绑定清理、fieldLabels 接入）→ commit（约 1h）
3. **后端 overview 接口**（regulation/award_punish）→ commit（约 1–1.5h）
4. **制度对齐**（布局、KPI、上传弹窗、批量条）→ commit（约 1.5h）
5. **奖惩对齐**（同上 + primary 按钮文案）→ commit（约 1.5h）
6. **P2 打磨** → commit；全量 `go vet` + `type-check` + 隧道走查 → 汇报 push 时机

> 决策事项（第四节 1–4）请主人在实施前或实施中拍板，其中第 1、2 条影响 P1 的写法。
