# 发票管理模块 Review 报告与优化方案

> 审查范围：`InvoiceManagement.vue`（2680 行）、`FleetUploadHistoryDrawer.vue`（903 行）、
> `BusinessSplitDetailDrawer.vue`（442 行）、`BusinessSubItemsPopover.vue`（160 行）、
> `businessDetailForm.ts`（513 行）、`invoicePendingGate.ts`、`itemTotals.ts`，
> 以及后端 `knowledge_invoice_*.go` / `knowledge_business.go` 的发票相关路径。
>
> 对照基准：能耗管理（`UtilitiesManagement.vue` + 5 个 `utility*.ts`）、合同管理
> （`ContractManagement.vue`）、中台组件（`BusinessListToolbar` / `BusinessFilterBar` /
> `BusinessSplitDetailDrawer` / `BusinessSubItemsPopover`）、`AGENTS.md` §8.5.4 / §9。
>
> 审查日期：2026-11-03。本报告**只出结论与方案，不改代码**（第 4 项要求）。

---

## 0. 总评

| 维度 | 评级 | 一句话结论 |
|---|---|---|
| 孤儿代码与死代码 | **B+** | 三文件 ESLint 零警告；仅 1 处真孤儿函数 + 2 处「已下线」注释残留 |
| 硬编码与重复造轮子 | **C+** | 字号零硬编码、色值均带 token 兜底；但**批量工具栏绕过中台自写 3 份**、格式化有 3 套并存 |
| 六同步闭环 | **B** | ①②③④⑤ 全闭环；② 字段筛选器只有 3 个固定项、⑥ 打印缺「清单」版式 |
| 视觉与交互统一性 | **B-** | 字号/圆角/间距全走 token；工具栏类名与能耗不一致（`doc-batch-bar` vs `doc-batch-bar-fixed`），空状态文案口径不同 |

**总体：B。** 数据链路与提取质量已达到生产级（本轮三项修复后真机 11/11），
问题集中在**前端结构复用**：三处「复制粘贴式实现」绕开了已经存在的中台能力。

---

## 1. 孤儿代码与死代码

### 1.1 确认无问题的部分

- 三个核心 .vue 文件 `eslint --format json` 输出**零 warning**（全项目 142 条 warning 均在其它模块）。
- `sanitizeNum` 的 `@input` 用法已在第 23 轮移除（那是「抽屉数值输不进去」的真根因），
  当前无任何「边输入边改写」的残留。
- 底部摘要栏（第 21 轮下线）的代码已删除，只剩两行说明性注释。

### 1.2 需清理（P2）

| # | 位置 | 现象 | 建议 |
|---|---|---|---|
| D-1 | `businessDetailForm.ts:63` `sanitizeNum` | **真孤儿**：生产代码零调用，仅 `businessDetailForm.test.ts` 引用。第 23 轮把 `@input` 过滤改成纯 `v-model` 后，它失去了唯一调用点 | 删除函数与其 2 条测试；若担心未来复用，改为在 `itemTotals` 同类工具里保留并注明用途 |
| D-2 | `InvoiceManagement.vue:160`、`:1366` | 「底部摘要栏已下线（第 21 轮）」注释两处 | 合并为一行，或直接删除（git 历史可查） |
| D-3 | `InvoiceManagement.vue:1143-1160` | 文件级占位行的三种分支（unknown / not_invoice / 有票）返回结构高度相似，`...base` 展开重复 3 次 | 抽 `makeFilePlaceholder(kind, status, error)` 单构造器，减少 3 处分支漂移风险 |

> 说明：D-2/D-3 属清洁度问题，不影响功能。按 `AGENTS.md` §8.5.4 的「批量清理三条铁律」，
> 清理 D-1 时必须确认 `sanitizeNum` 不在任何**一行多标识符的解构**里（本处是独立 export，安全）。

---

## 2. 硬编码与重复造轮子

### 2.1 硬编码：基本达标

| 项 | 结果 | 证据 |
|---|---|---|
| 字号 | **0 处** `font-size: Npx` | `InvoiceManagement.vue` / `FleetUploadHistoryDrawer.vue` 均 0 命中 |
| 色值 | 2 处，但**均带 token 兜底** | `.row-page-badge`：`var(--td-success-color-1, #e8f8f2)`；`BusinessSplitDetailDrawer.vue:308-310` 同款 |
| 内联 style | 各 1 处 | 用于动态宽度，属必要 |

结论：不存在裸色值/裸字号。`.row-page-badge` 的 `#e8f8f2 / #00a870 / #94e3c5` 是
TDesign 成功色的**降级副本**，可接受的写法；若要彻底消除，可换 `--td-success-color-*` 单变量。

### 2.2 重复造轮子（本轮最值得修的 3 项）

#### R-1 批量工具栏：中台已有，三个页面各写一份（P1）

```
中台：BusinessListToolbar.vue:67   <div class="doc-batch-bar-fixed">  ← 内置 + batch-actions slot
发票：InvoiceManagement.vue:162    <div class="doc-batch-bar">        ← 自写，且传 hide-batch-bar 关掉中台
合同：ContractManagement.vue:152   <div class="doc-batch-bar">
奖惩：AwardPunishManagement.vue:147 <div class="doc-batch-bar">
```

能耗页的注释直接写明「复刻合同管理模块」——**复刻的是别人的重复实现，而不是中台能力**。

后果：
- 类名不同（`doc-batch-bar` vs 中台 `doc-batch-bar-fixed`）→ 定位/动画/层级两套；
- 三个页面各自维护按钮顺序、禁用条件、二次确认弹窗；
- 中台的 `batch-actions` slot 与 `clear-selection` 事件形同虚设。

建议：发票页删除自写块，改用
```vue
<BusinessListToolbar ... :selected-count="selectedRowKeys.length" @clear-selection="clearSelection">
  <template #batch-actions> ...现有按钮... </template>
</BusinessListToolbar>
```
合同/奖惩同批迁移。**收益：-120 行/页，工具栏行为一处定义。**

#### R-2 数字格式化：3 套并存（P1）

| 出口 | 使用者 | 能力 |
|---|---|---|
| `formatNumber.ts` | 能耗（市电/光伏/核算/清单） | `round2/money2/num2/rateText/pct/cellNum/numOrZero/grouped`（最完整） |
| `businessFormat.ts` | 发票、制度、奖惩、浮窗 | `formatAmount/formatRate/formatDate...` |
| `ContractManagement.vue:1606/1610` | 合同 | **本地又写了一份 `formatAmount`/`formatRate`** |

风险：同一个金额在合同页与发票页可能显示不同小数位（口径分叉），这正是第 22 轮
「金额列裸浮点」问题的同源隐患。

建议：以 `formatNumber.ts` 为唯一出口，`businessFormat.ts` 改为**薄封装**（委托 + 业务默认值），
删除 `ContractManagement.vue` 的本地实现。

#### R-3 筛选器未从字段定义派生（P2，同时属六同步缺口）

`filterFields`（`InvoiceManagement.vue:671`）固定 3 项：关键词 / 税率 / 开票日期。
税率选项从后端 `listInvoiceTaxRates` 派生（✅ 好设计），但**分类自定义字段**
（车牌号、车辆类型、通行日期起止）无法作为筛选条件 —— 而它们恰恰是通行费发票
最需要筛的维度（运营实际会问「渝A75R1C 这个月跑了几趟」）。

建议：`filterFields` 从分类 `subs` 的 `filterable` 标记派生，与列/抽屉/浮窗同源。
这是把「六同步」从 5/6 补到 6/6 的关键一步。

---

## 3. 六同步闭环

| # | 环节 | 同源实现 | 状态 |
|---|---|---|---|
| ① | 字段定义 | 分类 `subs`（后端 `BusinessFieldCatalogs`） | ✅ |
| ② | 字段筛选器 | `filterFields` 固定 3 项 | ⚠️ **未从 subs 派生** |
| ③ | 列表表头/单元格/合计行 | `visibleColDefs` ← `effectiveColumns` ← `customColumns`(subs) 或 `DEFAULT_INVOICE_COLUMNS` 兜底；`invoiceColValue` 做键名归一 + `items[0].extra` 回退；合计行走 `itemTotals`（与浮窗共用） | ✅ |
| ④ | 悬停明细浮窗 | `subFieldsOfRow` → `BusinessSubItemsPopover`（`itemTotals` 合计） | ✅ |
| ⑤ | 编辑详情抽屉 | `buildDetailFieldDefs(subs, ...)` + `BusinessSplitDetailDrawer`（`itemTotals.totalRowTexts`，与浮窗**同一实现**） | ✅ |
| ⑥ | 打印目录/清单 | `catalogValueOf` ← `visibleColDefs` | ⚠️ 只有目录，无清单 |

### 3.1 ③④⑤ 的闭环质量（本轮已验证）

`itemTotals.ts` 是浮窗与抽屉的**唯一**合计实现，本轮真机断言两处数字完全一致
（`51,989.49 / 6,758.63`），且非累加列返回**空串**而非 `0` —— 这是正确设计。

`invoiceColValue` 的三级回退（`row[key]` → camel/snake 归一 → `items[0].extra[key]`）
解决了「分类子列整列为空」的历史问题，逻辑集中在一处，可维护性好。

### 3.2 ② 的缺口（P2，建议纳入重构）

当前无法按「车牌号」筛选，只能在关键词框里手输车牌（能命中，因为
`invoiceRecordMatchKeyword` 穿透了 `fields` 与 `items[].extra`，但那不是筛选器语义）。
补齐后：筛选器、列、抽屉、浮窗、打印全部由同一份 `subs` 派生，改一次处处生效。

### 3.3 ⑥ 的差异说明（非缺陷）

发票只有「打印目录」，能耗有「打印清单 + 打印目录」。发票的「清单」语义
（按租户分组、含期初/期末、费用归属）不适用于票据列表，**属合理差异**，
但应在文档中写明，避免后续误判为遗漏。

---

## 4. 前端视觉与交互统一性

| 检查项 | 发票 | 能耗 | 结论 |
|---|---|---|---|
| 字号 | 全部 token | 全部 token | ✅ 一致 |
| 主色 | `--td-brand-color` | 同 | ✅ |
| 圆角/间距 | `--td-radius-*` / `--td-comp-margin-*` | 同 | ✅ |
| 批量工具栏 | 自写 `.doc-batch-bar` | 自写（另一份） | ❌ **应统一到中台 `.doc-batch-bar-fixed`** |
| 空状态 | `t-empty description="尚未创建…"`（未建库）+ TDesign 表格默认「暂无数据」 | 自定义 `.fg-empty`「暂无数据」 | ⚠️ 文案口径不同，建议统一 |
| 抽屉底部按钮 | 取消 / 保存 | 取消 / 保存 | ✅ |
| 删除二次确认 | 三选一（删除 / 删除此页 / 删除文件） | 单一删除 | ✅ 合理（发票有分页语义） |
| 状态徽标 | 自定义 `.row-page-badge`（P1/P2 页码） | 无此需求 | ✅ 合理 |

### 4.1 需要统一的两处（P3）

1. **空状态文案**：发票列表空时用 TDesign 默认「暂无数据」，能耗用自定义「暂无数据」。
   两处视觉权重不同（一个在表格内居中，一个在容器内左对齐）。建议抽
   `BusinessEmptyState`（含 `description` / `action` slot），全模块统一。
2. **页码徽标配色**：`.row-page-badge` 用成功色，但它表达的是「第 N 页」而非「成功」。
   建议改用中性色（`--td-bg-color-container-hover` + `--td-text-color-secondary`），
   避免与「提取成功」的绿色语义混淆。

---

## 5. 优化方案（按优先级，待确认后执行）

### P1 — 结构与复用（预计 -300 行，风险中）

| 编号 | 动作 | 影响面 | 验证方式 |
|---|---|---|---|
| O-1 | 发票/合同/奖惩的批量工具栏迁移到 `BusinessListToolbar` 的 `batch-actions` slot | 3 页 | 真机逐页断言按钮顺序、禁用态、二次确认、位置一致 |
| O-2 | 格式化收敛到 `formatNumber.ts`；`businessFormat.ts` 改薄封装；删合同页本地实现 | 发票/合同/制度/奖惩 | 单测锁 `money2/rateText` 口径；真机抽查 4 页金额列无裸浮点 |

### P2 — 六同步补齐（预计 +80 行，风险低）

| 编号 | 动作 | 影响面 | 验证方式 |
|---|---|---|---|
| O-3 | `filterFields` 从 `subs` 的 `filterable` 派生（先补「车牌号」单字段试点） | 发票列表 | 真机断言「按车牌筛选」返回正确行数；列/抽屉/浮窗/打印不回归 |
| O-4 | 删除 `sanitizeNum`（孤儿）与其测试 | `businessDetailForm.ts` | type-check + 单测全绿 |
| O-5 | 抽 `makeFilePlaceholder` 统一占位行构造 | 发票列表 | 单测覆盖 3 种 kind |

### P3 — 视觉收口（预计 +40 行，风险低）

| 编号 | 动作 | 影响面 |
|---|---|---|
| O-6 | 抽 `BusinessEmptyState`，统一 4 个业务页空状态 | 发票/合同/制度/奖惩 |
| O-7 | 页码徽标改中性色，与「成功」语义解耦 | 发票列表 |
| O-8 | 清理 D-2 的两处「已下线」注释 | 发票列表 |

### 不做（明确排除）

- **不合并 `InvoiceManagement.vue` 与 `UtilitiesManagement.vue` 的列表逻辑**：两者数据模型
  （发票行 vs 账单行）、KPI 口径、打印版式差异大，强行抽象会产生参数爆炸。
- **不给发票加「打印清单」**：语义不适用（见 §3.3）。
- **不动 `docreader` / 知识库 pipeline**：铁律。

---

## 6. 本轮修复的回归防线（已落地）

| 防线 | 文件 | 锁定内容 |
|---|---|---|
| `HasUsableInvoiceRecords` | `extract_failure_state.go` | 「有票号或金额」即视为可用记录，失败不降级状态 |
| `SuppressInvoiceAnchorWarning` | 同上 | 只提到 1 张票时绝不再报「还有一张未提取」 |
| `mergeFailedStateMeta` 分支 | `knowledge_business.go` | 有成果 → 保状态 + 记 `last_extract_error`；无成果 → 落 failed |
| `cleanIssuerName` 后缀规则 | `knowledge_invoice_field_complete.go` | `璧山1` 完整保留；纯数字/字母、中缀数字一律拒绝 |
| `SyncDocumentIssuer` | `extract_failure_state.go` | 票级 → 文档级 issuer/drawer 同源 |
| `itemTotals` | `itemTotals.ts` | 浮窗与抽屉合计**唯一实现**，按分累加，非累加列空串 |
| `invoicePendingGate` | `invoicePendingGate.ts` | 解析/提取中的文件整组剔除（前端第二道门禁） |

测试：`invoice_state_consistency_test.go`（14 条）+ `itemTotals.test.ts`（12 条）
+ `invoicePendingGate.test.ts`（6 条）+ 既有 761 条。

---

## 7. 附：真机验收证据（本轮）

| 断言 | 结果 |
|---|---|
| 璧山天然气 10 张票开票人全部 `璧山1` | ✅ 10/10 |
| 详情抽屉无「2 张发票锚点」警告 | ✅ |
| 抽屉「提取状态」= 列表「提取状态」= 提取成功 | ✅ |
| 历史抽屉「异常文件」= 0 | ✅ |
| 历史抽屉滚动加载 20 → 100 条 | ✅ |
| 控制台 0 业务 error | ✅ |

合计 **11/11**。
