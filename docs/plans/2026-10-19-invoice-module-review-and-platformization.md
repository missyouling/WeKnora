# 发票管理模块代码质量审查、性能优化与中台化复用蓝图

- 日期：2026-10-19
- 分支：`daily-affairs-v4`（HEAD `8dff1927a`）
- 性质：**只读审查**，本轮未修改任何业务源码
- 覆盖：前端 8 个业务组件 + 6 个配置/工具模块；后端提取器 6 个核心文件 + 解析引擎层
- 实测环境：VM `10.10.10.239:5522`（Supabase session pooler，`DB_MAX_OPEN_CONNS=8`）

---

## 摘要：五个必须立刻知道的事实

| # | 结论 | 严重度 | 证据 |
|---|---|---|---|
| 1 | **5 个业务的提取都只加载 `chunk_type='text'`**，构建函数里对 `image_ocr`/`image_caption` 的放行分支是**死代码**。纯扫描件（PaddleOCR-VL / Simple+VLM）将提取到空文本 | **P0** | `chunk.go:166-177` SQL 硬编码 `chunk_type = ?` → `"text"`；调用点 `knowledge_business.go:320/1049/1575/1920/2275` |
| 2 | **沙箱测试与正式提取是两条不同管线**：沙箱走 `ExtractFleetDocumentFromContent`（通用 map 视图），正式走 `runInvoiceCore`（结构体视图 + 3 套确定性前置规则） | **P0** | `extract_config.go:464` vs `knowledge_business.go:1191` |
| 3 | `/invoices` **每次请求都全量扫库 + 全量解包 JSONB + 内存排序分页**，实测 **1.05–1.40s**（182 文件 / 1109 张票 / 311 KB JSONB），而前端每 3s 轮询一次 | **P0** | 本文 §1.3 实测表 |
| 4 | **样票解析的引擎选择与上传解析不一致**：上传用 KB 的 `ParserEngineRules`，样票用 `ChatParserEngineRules` —— 而后者**全代码库没有任何写入点**（永远是 nil） | **P1** | `knowledge_process.go:4212` vs `extract_suggest_parse.go:170`；`grep ChatParserEngineRules` 只有读、无写 |
| 5 | 发票 `page` 字段**不是物理页码**，是 `NormalizeInvoiceExtractionResult` 里自增的数组序号；真实页码 `Chunk.SourceLocators[].Page` 已存在但**提取侧零引用** | **P2** | `knowledge_invoice_extract.go:1042`；`types/source_locator.go:44` |

---

# 维度 1：代码质量与性能效率深度审查

## 1.1 巨石文件解耦诊断

### 1.1.1 前端 `InvoiceManagement.vue`（3009 行）

| 区块 | 行号 | 说明 |
|---|---|---|
| `<template>` | 1–425 | 顶部 KPI + 自绘表格 + 详情抽屉 + 6 个弹窗全部内联 |
| `<script setup>` | 427–2442 | **2015 行**，34 个 `ref`、30 个 `computed`、7 个 `watch`、180 个顶层绑定 |
| `<style scoped>` | 2442–2932 | 490 行 |
| `<style>`（全局） | 2932–3009 | 77 行 |

**超长函数（按跨度排序）**

| 函数 | 行号 | 跨度 | 混合的职责 |
|---|---|---|---|
| `persistEditForm` | 1799–1952 | **153 行** | 表单脏检查 + 数值清洗 + 税率归一 + 重号确认 + PATCH 请求 + 本地行合并 + 错误提示 |
| `fillEditForm` | 1683–1784 | 101 行 | 行 → 表单映射 + 明细子列展开 + 备注片段拆解 |
| `loadFiles` | 985–1068 | 83 行 | 请求 + 防重（`listSeq`）+ 后端行映射 + KPI 联动触发 |
| `handleBatchPrint` | 2157–2235 | 78 行 | 选中聚合 + 图片/PDF 分流 + iframe 打印 |
| `handleFileExtract` | 2078–2151 | 73 行 | 批量重提 + 分批并发 + 409 冲突识别 + 刷新 |

**坏味道**

1. **状态变量过度耦合**：`keyword / dateRange / taxRateFilter / filterInvoiceType` 四个筛选条件各自 `watch`（L1377/L1383/L1390）又各自调 `applyFilter()`，再加 `applyFilter` 自己也调（L1354）→ 一次用户操作可触发 2–3 次全量重载。
2. **KPI 与列表耦合**：`watch(pendingFiles)`（L1461、L1537）两处，任一变化即 `loadInvoiceOverview()`；而 `pendingFiles` 由 3s 轮询刷新，等价于"每 3s 无条件重算 KPI"。
3. **发票专属逻辑内联在页面**：`fallbackItemSubFields`（L1603）硬编码 8 个明细标准键，`ITEM_STD_KEYS`（L1611）再硬编码一次 —— 与 `extractFieldMeta.ts` 的 `INVOICE_STRUCT_KEYS` 是第三份副本。
4. **`kind: 'invoice'` 字面量出现在 3 处**（L576 类型、L1077 映射、L1896 编辑）。

### 1.1.2 后端 `knowledge_business.go`（2993 行）

| 函数 | 行号 | 跨度 |
|---|---|---|
| `runInvoiceCore` | 991–1495 | **504 行** |
| `runContractCore` | 255–586 | 331 行 |
| `runRegulationCore` | 1855–2163 | 308 行 |
| `runAwardPunishCore` | 2210–2515 | 305 行 |

`runInvoiceCore` 一个函数里塞了 **10 件事**（按行内偏移）：

```
  +21  panic 恢复 + 落 failed 状态
  +66  BuildInvoiceExtractionBatches（分批）
  +72  空文本 → no_text 落库
 +101  解析中状态落库
 +109  ResolveInvoiceCertType（权威分类解析）
 +125  LineRules 前置规则
 +134  MarkdownTableRules 兜底
 +146  FlatLayout 兜底
 +236  LLM 分批调用（并发 4）
 +317  NormalizeInvoiceExtractionResult（去重/页码/税率）
 +381  applyInvoiceUserCategoryPrecedence（分类锁定）
 +464  合并落库
```

**四个 `run*Core` 是同一骨架的四份复制**：都是 `ctx 包装 → panic 兜底 → 取 chunks → 拼内容 → 调模型 → 校验 → 落库`。这是后端最大的重复源，也正是通用提取器要消灭的东西。

### 1.1.3 `knowledge_invoice_extract.go`（2269 行，44 个函数）

一个文件里并存四层：

| 层 | 代表函数 | 行号 |
|---|---|---|
| LLM Prompt 与协议 | `invoiceExtractionSystemPrompt`、`buildInvoiceRuleSystemPrompt` | ~150–230、548 |
| 模型输出清洗 | `cleanInvoiceNullStrings`、`normalizeRuleFieldKeys`（111 行）、`parseInvoiceRuleReply` | 398、428、539 |
| 三套确定性规则 | `ExtractInvoicesByLineRules`（158 行）、`ExtractInvoicesByMarkdownTableRules`（116 行）、`ExtractInvoicesByFlatLayout` | 1349、1584、1831 |
| 文本/HTML 解析工具 | `stripHTMLTags`、`plateLike`、`isCompact8Date`、`htmlTableDataRow`、`htmlItemRowByLabels` | 1965–2245 |

**问题**：LLM 协议变更、规则引擎扩展、HTML 变体适配三者互相牵动，任何一处改动都要跑全文件回归。建议按 `prompt / normalize / rules / parseutil` 四包拆分。

### 1.1.4 `extract_config.go`（961 行）

发票专属的四个函数与通用配置 CRUD 混在同一文件：

- `injectInvoiceItemSubFields` L61
- `invoiceRuleDescFor` L530（118 行，全是发票字段文案）
- `mergeInvoiceRuleFields` L689
- `seedInvoiceRuleBackfill` → `SeedInvoiceExtractRuleBackfill` L845
- `defaultInvoiceRuleFields` L779 / `synthesizeInvoiceRuleConfig` L813 / `defaultInvoicePromptTemplate` L648

`grep -c Invoice` = **39 处**。

### 1.1.5 代码坏味道清单

| 坏味道 | 位置 | 说明 |
|---|---|---|
| **两套校验器** | `extract_validate.go:94` `validateInvoiceExtraction` vs `extract_generic_validate.go:340` `ValidateGenericExtraction` | 发票走前者，合同/制度/通知/电费/光伏走后者。同一件事两份实现，必漂移 |
| **两套内容拼装** | `BuildInvoiceExtractionBatches:932`（24000 runes，`\n\n` 分隔）vs `buildFleetExtractionContent:415`（120000 **bytes**，`\n` 分隔） | 沙箱比正式宽 **5 倍**，且截断策略不同 |
| **GET 里写库** | `FleetCertPanel.vue:544-556` `load()` 内调用 `updateFleetCategory` 做"营运证→道路运输经营许可证"迁移 | 每次打开设置面板都可能发一次 UPDATE |
| **硬编码 scope 联合类型** | `StandardSettingDrawer.vue:38`、`FleetCertPanel.vue:262`、`useDocStatus.ts:25`、`extractFieldMeta.ts:27` | 新增业务要改 4 处类型 |
| **三份标准键副本** | `invoiceFieldLabels.INVOICE_FIELD_LABELS`、`extractFieldMeta.INVOICE_STRUCT_KEYS`、`InvoiceManagement.ITEM_STD_KEYS` | 加字段要改 3 处 |
| **`notice` 与 `award_punish` 混淆** | `extractFieldMeta.ts:42` 用 `NOTICE_FIELD_LABELS` 映射 `award_punish`；`LABELERS:139-144` 没有 `notice` | "通知公告"与"奖惩"共用 scope，字段将来会打架 |

---

## 1.2 前后端性能与效率瓶颈

### 1.2.1 `/invoices` 列表：全量扫描 + 内存分页（P0）

**实现路径**：`knowledge_invoice_list.go:42` `ListInvoiceRecords`

```go
// 1. 拉全量 knowledge（分页循环，避免单页上限）
for {
    knowledges := repo.ListPagedKnowledgeByKnowledgeBaseID(page, 1000)   // :68
    repo.GetKnowledgeTags(ctx, ids)                                      // :82
    for _, k := range knowledges { json.Unmarshal(k.CustomMetadata, &meta) }  // :92
}
// 2. 平铺 invoices（每个文件 1..27 张）
// 3. 按发票号去重
// 4. 过滤 → 内存排序 → 内存分页
```

**实测数据（VM，182 文件）**

| 指标 | 值 |
|---|---|
| 知识库文件数 | 182 |
| 发票总行数 | **1109** |
| `custom_metadata` 总字节 | **311 KB**（均 1750 B，最大 6494 B） |
| 纯 SQL 取数耗时 | 144 ms |
| **接口端到端耗时** | **1.05–1.40 s**（5 次：1.396 / 1.084 / 1.136 / 1.104 / 1.069） |
| 响应体大小 | 30 KB（只 20 行） |
| 前端轮询间隔 | 3 s |

**结论**：1.1 s / 3 s ≈ **37% 的时间片**在重复做同一件全量工作。文件数线性增长 → 1820 文件时约 **11 s/次**，届时前端每 3 s 发一次、接口 11 s 才回，请求必然堆积（这正是上一轮 503 的应用侧诱因）。

**数据库侧缺陷**

| 项 | 现状 | 影响 |
|---|---|---|
| `custom_metadata` 索引 | **无任何 GIN 索引**（`grep custom_metadata migrations/*.sql` 为空） | 任何 JSONB 条件查询都要全表扫 |
| 取数投影 | `Find(&knowledges)` = `SELECT *` | 连带 `description`（本库 43 KB）与 `custom_metadata` 全量拉取 |
| 排序 | Go 侧 `sort.Slice` | 数据库索引完全用不上 |
| 分页 | 最后才切片 | `LIMIT 20` 只省传输，不省扫描 |

**优化方案（按性价比排序）**

1. **落一张投影表 `invoice_records`**（推荐，彻底解决）
   ```sql
   CREATE TABLE invoice_records (
     tenant_id bigint, kb_id uuid, knowledge_id uuid,
     page int, invoice_no text, invoice_date date,
     invoice_type text, category text,
     amount numeric, tax numeric, total_amount numeric, tax_rate numeric,
     buyer_name text, seller_name text, buyer_tax_no text, seller_tax_no text,
     plate_no text, travel_from date, travel_to date,
     is_placeholder bool, extract_status text,
     search_text text,          -- 全字段 + items[].extra 拼接，供 ILIKE
     created_at timestamptz,
     PRIMARY KEY (knowledge_id, page)
   );
   CREATE INDEX ON invoice_records (tenant_id, kb_id, invoice_date DESC);
   CREATE INDEX ON invoice_records USING gin (search_text gin_trgm_ops);
   ```
   在 `runInvoiceCore` 落库处同步写入（或 asynq 异步投影）。列表查询变成一条带 `WHERE + ORDER BY + LIMIT` 的 SQL，耗时与文件数脱钩。

2. **过渡方案（1 天内可上线）**
   - 加 `CREATE INDEX ... USING gin (custom_metadata jsonb_path_ops)`；
   - 分页查询加投影：`Select("id, file_name, file_type, parse_status, custom_metadata, created_at, title")`，不取 `description`；
   - `page_size` ≤ 50 时先用 `LIMIT/OFFSET` 取候选文件，只解包候选（但按发票排序会失真，需谨慎）。

3. **前端降频**：`onTick` 只在"有 pending/processing 文件"时才刷列表；无在途文件时轮询周期从 3 s 退到 15 s。

### 1.2.2 前端渲染与轮询开销

| 问题 | 位置 | 说明 |
|---|---|---|
| 轮询固定 3 s，即使无在途任务 | `InvoiceManagement.vue:1438-1454` | 上一轮已加退避，但**成功路径仍是 3 s 恒定** |
| 双 `watch(pendingFiles)` | `:1461`、`:1537` | 每次轮询触发两次回调，其中一次无条件 `loadInvoiceOverview()` |
| 三个筛选 `watch` + `applyFilter` | `:1377/1383/1390/1354` | 一次操作可能连发 2–3 个 `/invoices` |
| 自绘 grid 全量重渲染 | `:70-425` | 虚拟滚动已做（>100 行仅渲染视口），但列定义 `effectiveColumns` 每次 `reloadColumns` 都重建对象 → 触发整表 patch |
| 分类接口无缓存 | 见下表 | 同一面板重复拉 |

**分类接口冗余（实测调用点）**

| 组件 | 位置 | 请求形态 | 频次 |
|---|---|---|---|
| `FleetCertPanel.load()` | `:544-546` | `Promise.all(scopes.map(listFleetCategories))` —— 拉**当前所有分组**的 scope | 每次打开设置抽屉 |
| `ExtractRulePanel.refresh()` | `:1176-1183` | `Promise.all(scopes.map(...))`，fleet scope 还额外拉 aliases | 每次切 Tab + `watch(scope)` |
| `InvoiceManagement.reloadColumns` | `:1244-1253` | 单 scope，**已有缓存**（`invoiceCats`） | 好，可作范本 |
| `ExtractRulePanel:1180` | 同上 | 车队场景一次 3 个 scope | 3× 请求 |

**建议**：建 `useExtractCatalog()` —— 模块级单例 + `Map<scope, Promise>` 去重 + TTL 30 s。发票页 `invoiceCats` 的做法直接升级为全局版本即可。

### 1.2.3 LLM 与解析调用效率

| 项 | 现状 | 评估 |
|---|---|---|
| 分批粒度 | `InvoiceExtractionBatchSize = 8` chunks（`:886`） | 合理（≈4 页） |
| 批间并发 | `invoiceBatchConcurrency = 4`（`knowledge_business.go:81`）+ 信号量 `:1189` | 合理 |
| 单次调用超时 | 5 min（`knowledge_invoice_extract.go:30`） | 合理 |
| 任务总超时 | 30 min（`knowledge_business.go:66`） | **偏紧**：27 张票 → 89 chunks → 12 批 → 3 波 × 5 min = 15 min 上限；若模型抖动到 8 min/次即触顶 |
| 单批内容上限 | **24000 runes**，且超限走 `sampleLongContent`（head 60% + mid 20% + tail 20%） | **危险**：中段 20% 被**静默丢弃**。多票汇总单超限时会"少几张票"，且无任何告警 |
| 规则短路 | `ruleOK && !InvoiceRuleResultIncomplete` 才跳过 LLM | 已加半残守卫，正确 |

**建议**：`BuildInvoiceExtractionBatches` 在"单 chunk 超限"与"内容被采样"时写入 `extract_warnings`，让前端 `ExtractQualityPanel` 显示"本文档因超长被采样，可能有票未提取"。

---

# 维度 2：前端组件全景盘点与中台抽离方案

## 2.1 现有组件盘点

| 组件 | 路径 | 行数 | 职责 | 中台就绪度 |
|---|---|---|---|---|
| `StandardSettingDrawer` | `components/business/StandardSettingDrawer.vue` | 103 | 「字段定义 / 提取规则」双 Tab 骨架 | **★★★☆** 已 `scope` 参数化，仅类型联合硬编码（L38）、默认值写死 `'invoice'`（L43） |
| `FleetCertPanel` | `views/dailyAffairs/FleetCertPanel.vue` | 1486 | 分类/字段/子项 CRUD + AI 字段建议入口 | **★★☆☆** `GROUPS` 7 个 scope 硬编码（L267-278），11 处 `scope === 'invoice'` 三元分支 |
| `AiFieldSuggestPanel` | `views/dailyAffairs/AiFieldSuggestPanel.vue` | 440 | 样票上传 → AI 建议 → 勾选落配置 | **★★★☆** 仅 L201 `scope: props.scope \|\| 'invoice'` 一处兜底 |
| `ExtractRulePanel` | `views/dailyAffairs/ExtractRulePanel.vue` | 1665（38 函数） | 规则表 + 展开编辑 + AI 生成规则 | **★★☆☆** L933 硬编码 `INVOICE_COMMON_RULES`；L1180 全 scope 拉取 |
| `ExtractTestPanel` | `views/dailyAffairs/ExtractTestPanel.vue` | 325 | 沙箱容器（输入 + 结果） | **★★★☆** 已拆分 |
| ├ `TestInputPanel` | `views/dailyAffairs/TestInputPanel.vue` | 132 | 样票/文本输入 | ★★★★ |
| └ `TestResultPanel` | `views/dailyAffairs/TestResultPanel.vue` | 240 | 结果 + 缺失 + issues | ★★★★ |
| `ExtractQualityPanel` | `views/dailyAffairs/ExtractQualityPanel.vue` | 166 | 质量徽章 + 分级告警 + 正则溯源 | **★★★★** 纯展示，`labelOf` 由外部注入 |
| `FleetUploadDialog` | `views/dailyAffairs/FleetUploadDialog.vue` | 717 | 上传 + 解析/提取进度 + 重复自愈 | **★★★☆** L118 `isInvoice` 分支；其余已 `scope` 化 |
| `document-preview` | `components/document-preview.vue` | 1694 | PDF/图片/Office 预览 | **★★★★** 仅需 `knowledge-id` |
| `DashboardKpiGroup` | `components/business/DashboardKpiGroup.vue` | 308 | KPI 卡片组 + 溢出滚动 | **★★★★** `cards` 数组驱动，已是 `BusinessStatCards` 雏形 |
| `BusinessListToolbar` | `views/dailyAffairs/BusinessListToolbar.vue` | 140 | 搜索 + 类型选择 + 批量条 | **★★★☆** 缺日期区间/动态筛选/主按钮插槽命名 |
| `BusinessColumnFilter` | `views/dailyAffairs/BusinessColumnFilter.vue` | 114 | 列显隐 | ★★★★ |
| `BusinessParseStatusBar` | `components/business/BusinessParseStatusBar.vue` | 112 | 解析/提取进度条 | ★★★★ |
| `useBusinessList` | `composables/useBusinessList.ts` | 92 | 列定义 + 可见列持久化 + 多选 | ★★★★ |
| `useDocStatus` | `composables/useDocStatus.ts` | 82 | 文档状态文案 | ★★★☆ L25 scope 联合类型硬编码 |
| `useBusinessPolling` | `composables/useBusinessPolling.ts` | 256 | 轮询 + 防重 + 退避 | ★★★★ |

**结论**：**展示型组件（Quality/Preview/KPI/StatusBar/ColumnFilter）已具备直接复用条件**；
**配置型组件（CertPanel/RulePanel）与 scope 强绑定**，是本次改造的主战场。

## 2.2 硬编码耦合点全清单（含行号）

### 2.2.1 `scope === 'invoice'` 字面判断（必须消灭的 12 处）

| 文件 | 行号 | 代码 | 改为 |
|---|---|---|---|
| `extractFieldMeta.ts` | **111** | `if (scope !== 'invoice') return false` | `if (!registry.get(scope)?.regexPatchable) return false` |
| `extractFieldMeta.ts` | **117** | `return '本模块暂未接入后置正则补洞（当前仅发票生效）'` | 从 registry 读 `disabledReason` |
| `ExtractRulePanel.vue` | **933** | `currentScope.value === 'invoice' ? draftByKeyForms(INVOICE_COMMON_RULES, a.name) : undefined` | `registry.get(scope)?.commonRules` |
| `FleetCertPanel.vue` | **124** | `group.scope === 'invoice' ? invoiceFieldLabel(fd.name)` | `labelOf(group.scope, fd.name)` |
| `FleetCertPanel.vue` | **214** | 四层三元（maintain/invoice/contract/…） | `registry.get(scope)?.emptyText` |
| `FleetCertPanel.vue` | **273** | `invoice: [{ key:'invoice', label:'发票', ... }]` | 由 registry 生成 |
| `FleetCertPanel.vue` | **281** | `scopeName()` 五层三元 | `registry.get(scope)?.categoryLabel` |
| `FleetCertPanel.vue` | **605/607** | `addLabel` / `addRowLabel` | 同上 |
| `FleetCertPanel.vue` | **611/612/789** | 三处提示文案三元 | `registry.get(scope)?.namePlaceholder` |
| `FleetCertPanel.vue` | **674** | `if (group.value.scope !== 'invoice') return false` | `registry.get(scope)?.supportsSubFields` |
| `FleetCertPanel.vue` | **735** | `fieldLabelOf` 内 `scope === 'invoice'` | 同上 `labelOf` |
| `FleetUploadDialog.vue` | **118** | `isInvoice = computed(() => props.scope === 'invoice')` | `registry.get(scope)?.kind === 'invoice'` |

### 2.2.2 发票专属字段字典（3 份副本）

| 文件 | 行号 | 内容 |
|---|---|---|
| `invoiceFieldLabels.ts` | 4–44 | `INVOICE_FIELD_LABELS`（中文名） |
| `invoiceFieldLabels.ts` | 75–104 | `INVOICE_FIELD_KEY_ALIASES`（中文→英文别名） |
| `invoiceFieldLabels.ts` | **106** | `INVOICE_REMARK_PART_FIELDS = ['收款方式','收款事由','审核',…]`（**发票独有的"备注片段当字段存"hack**） |
| `extractFieldMeta.ts` | 28–39 | `STANDARD_KEYS.invoice` |
| `extractFieldMeta.ts` | 74–97 | `INVOICE_STRUCT_KEYS`（22 键） |
| `InvoiceManagement.vue` | 1603–1612 | `fallbackItemSubFields` + `ITEM_STD_KEYS`（第三份） |

### 2.2.3 发票专属 API / 常量

| 文件 | 行号 | 内容 |
|---|---|---|
| `InvoiceManagement.vue` | **1250** | `listFleetCategories({ scope: 'invoice' })` |
| `InvoiceManagement.vue` | **1442** | `extractBusinessDocument(kid, fileId, 'invoice', …)` |
| `InvoiceManagement.vue` | **1551** | `useDocStatus({ scope: 'invoice', notLabel: '非发票' })` |
| `AiFieldSuggestPanel.vue` | **201** | `scope: props.scope \|\| 'invoice'` |
| `StandardSettingDrawer.vue` | 38 / 43 | scope 联合类型 / 默认 `'invoice'` |
| `useDocStatus.ts` | 25 | `scope: 'contract' \| 'invoice' \| 'regulation' \| 'award_punish'` |
| `ExtractRulePanel.vue` | 1180 | `isFleetScope ? ['vehicle','driver','maintain'] : [props.scope]` |

## 2.3 `BusinessScopeRegistry` 设计（可直接编码）

### 2.3.1 契约（TS）

```ts
// frontend/src/registry/businessScope.ts
export type ScopeKey =
  | 'invoice' | 'contract' | 'regulation' | 'award_punish' | 'notice'
  | 'vehicle' | 'driver' | 'maintain' | 'utility' | 'solar'

export interface FieldMeta {
  key: string                 // snake_case 契约键（后端 json tag）
  label: string               // 中文显示名
  dataType: 'text' | 'number' | 'date' | 'array'
  hint?: string               // 喂给 AI 的语义提示
  aliases?: string[]          // 中文别名（正则/LLM 归一用）
  remarkPart?: boolean        // 无标准存储位，数据在 remark 片段里（发票专属能力）
  subFields?: FieldMeta[]     // 该字段是明细数组时，子列定义
}

export interface ScopeDescriptor {
  key: ScopeKey
  kind: 'invoice' | 'document' | 'archive'
  categoryLabel: string        // “发票类型 / 合同类型 / 制度名称”
  namePlaceholder: string      // 弹窗输入占位
  emptyText: string            // 空态文案
  itemLabel: string            // 明细子项标题
  supportsSubFields: boolean   // 是否展示明细子项
  supportsRemarkParts: boolean // 是否用 remark 片段存自定义字段
  regexPatchable: boolean      // 是否接入后置正则补洞
  standardFields: FieldMeta[]  // 标准契约字段（唯一真源）
  commonRules?: { name: string; regex: string; hint: string }[]  // 内置正则预设
  api: {
    extract: (kbId: string, kid: string, certType?: string) => Promise<any>
    deletePage?: (kbId: string, kid: string, page: number) => Promise<any>
    listRows: 'invoices' | 'documents' | 'records'
  }
  columns: ColumnDef[]         // 列表默认列
  kpiCards?: KpiCardSpec[]     // 顶部统计卡
}
```

### 2.3.2 注册与消费

```ts
// registry/index.ts
const registry = new Map<ScopeKey, ScopeDescriptor>()
export const registerScope = (d: ScopeDescriptor) => registry.set(d.key, d)
export const getScope = (k: string) => registry.get(k as ScopeKey)
export const labelOf = (k: string, field: string) => {
  const d = getScope(k)
  return d?.standardFields.find(f => canonical(f.key) === canonical(field))?.label ?? field
}

// registry/invoice.ts —— 发票只需声明，不再写分支
registerScope({
  key: 'invoice', kind: 'invoice',
  categoryLabel: '发票类型', namePlaceholder: '请输入分类名称',
  emptyText: '暂无发票类型', itemLabel: '项目明细',
  supportsSubFields: true, supportsRemarkParts: true, regexPatchable: true,
  standardFields: INVOICE_FIELDS,           // 从 invoiceFieldLabels 迁移，只此一份
  commonRules: INVOICE_COMMON_RULES,
  api: { extract: extractBusinessDocument, deletePage: deleteInvoicePage, listRows: 'invoices' },
  columns: DEFAULT_INVOICE_COLUMNS,
})
```

**迁移收益**：新增 `notice` 模块只需 1 个 `registry/notice.ts`（约 80 行），
`FleetCertPanel` / `ExtractRulePanel` / `extractFieldMeta` **零改动**。

## 2.4 待提炼的新中台组件

### 2.4.1 `BusinessStatCards.vue`（现有 `DashboardKpiGroup` 升级）

| 项 | 设计 |
|---|---|
| Props | `cards: KpiCard[]`、`loading?: boolean`、`activeKey?: string`、`columns?: number` |
| Emits | `card-click(card)`、`overflow-change(dir: 'left'\|'right')` |
| 数据来源 | `useBusinessOverview(scope)` —— 内部按 `scope.api.listRows` 选 `/invoice-overview-stats` 或通用聚合 |
| 工作量 | **0.5 人日**（`DashboardKpiGroup` 已实现溢出滚动，仅补数据 hook 与空态） |

### 2.4.2 `BusinessFilterBar.vue`

| 项 | 设计 |
|---|---|
| Props | `modelValue: FilterState`、`fields: FilterFieldSpec[]`、`columns: ColumnDef[]`、`visibleKeys: string[]`、`primaryAction?: { text, icon }` |
| Emits | `update:modelValue`、`filter-change`、`columns-change`、`primary`、`settings`、`upload` |
| `FilterFieldSpec` | `{ key, type: 'keyword'\|'date-range'\|'select'\|'number-range', label, options?, clearable? }` |
| 内置 | 搜索框 + 动态筛选区 + 列显隐 + 设置按钮 + 主按钮（全部走 spec，不写业务分支） |
| 工作量 | **1.5 人日**（在 `BusinessListToolbar` 上加通用筛选渲染；发票页现有 4 个筛选降为 spec 配置） |

### 2.4.3 `BusinessSubItemsPopover.vue`

| 项 | 设计 |
|---|---|
| Props | `items: Record<string, any>[]`、`subFields: FieldMeta[]`、`minWidth?: number`、`maxWidth?: number` |
| 内部 | 复用 `InvoiceManagement.vue:1653-1682` 的宽度算法与单元格分类（`itemCellClass` / `itemValOf` / `formatItemCell` 原样搬入） |
| 取值策略 | `standardKeys` 直接取行属性；否则 `row[key] ?? row.extra?.[key]` |
| 工作量 | **1 人日**（逻辑已存在，属搬运 + 抽 Props） |

### 2.4.4 `BusinessSplitDetailDrawer.vue`

| 项 | 设计 |
|---|---|
| Props | `visible`、`scope`、`row`、`fields: FieldMeta[]`、`subFields?: FieldMeta[]`、`quality?: ExtractQuality` |
| 布局 | 左：`document-preview`（折叠态默认不挂载）；右：动态表单 + 顶部 `ExtractQualityPanel` |
| Emits | `save(payload)`、`delete`、`re-extract`、`update:visible` |
| 复用点 | 直接替换 `InvoiceManagement.vue:300-425` 的抽屉模板 + `fillEditForm/persistEditForm`（233 行） |
| 工作量 | **3 人日**（表单渲染引擎 + 脏检查 + 数值/日期清洗需参数化） |

### 2.4.5 `useBusinessListSync.ts`

| 项 | 设计 |
|---|---|
| 职责 | 源文件 ↔ 列表 100% 对齐（占位行）、级联删除、上传去重自愈、乐观删除 |
| 入参 | `{ kbId, scope, listFn, deletePageFn?, extractFn }` |
| 出参 | `{ rows, loading, reload, removeRows, syncPending }` |
| 来源 | `InvoiceManagement.vue:2311-2397`（`optimisticRemoveInvoiceRows/schedulePostDeleteRefresh/doPageDelete/doFileDelete`）+ `loadFiles` 的占位行分支 |
| 工作量 | **2 人日** |

**前端总工作量估算：约 8 人日**（不含联调）。

---

# 维度 3：通用提取器跨业务复用审查

## 3.1 四个能力的发票耦合度

| 能力 | 入口 | 发票耦合点 | 耦合度 |
|---|---|---|---|
| **智能字段生成** | `AISuggestFields` `extract_config_ai.go:57` → `suggestFieldsFromText` `extract_suggest_ai.go:364` | `InvoiceStandardFieldDictionary()` `:273`（硬编码 16 条 spec）<br>`invoiceContractKeys()` `:254`（reflect `InvoiceExtractionItem`）<br>`postProcessAISuggestFields` `:412` 里 `contract := invoiceContractKeys()` `:422`<br>`demotedFieldLabel` `:646` 用发票字典 | **高**（字典与契约写死） |
| **智能配置规则** | `AISuggestRules` `extract_config_ai.go:139` → `buildRuleSuggestPrompt` `extract_suggest_ai.go:856` | 同上 + `guardAIRegexPatchable` `:101`（已退化为恒通过，注释仍写"发票补洞器"） | **中**（prompt 已按 dict 参数化，仅契约来源写死） |
| **后置正则补洞** | `RegexFallbackInvoiceFields` `extract_regex_fallback.go:232` | **53 处** `InvoiceExtractionItem` 引用：<br>`invoiceFieldAccessor` `:76`、`invoiceAccessorFor` `:210`、`patchStandardInvoiceFields` `:253`、`patchConfiguredInvoiceFields` `:305`、`patchInvoiceItemSubFields` `:361`、`customInvoiceFieldValue` `:599`、`invoiceFieldOrder` `:197` | **极高**（完全结构体绑定） |
| **沙箱测试** | `TestExtractConfig` `extract_config.go:367` | 走 `ExtractFleetDocumentFromContent` `:464` + `ValidateGenericExtraction` `:480` —— **反而不走发票管线** | **低**（但口径与生产不一致，见 §3.3） |

## 3.2 已有的通用底座（不要重造）

后端其实已经具备通用化的全部零件：

| 零件 | 位置 | 现使用者 |
|---|---|---|
| `BuildExtractSystemPrompt(cfg)` | `extract_config_prompt.go:72` | 合同/制度/通知/电费/光伏/沙箱 |
| `EnabledExtractFieldNames(fields)` | `:10` | 同上 |
| `BuildFieldsSchema(fields)` | `:24` | 同上 |
| `ExtractFleetDocumentFromContent(...)` | `knowledge_fleet_document.go:347` | 同上 |
| `ValidateGenericExtraction(records, text, cfg)` | `extract_generic_validate.go:340` | **合同 :419 / 制度 :2015 / 通知 :2371 / 电费 / 光伏 / 沙箱** |
| `genericRegexPatch(...)` | `:416` | 同上 |
| `NewGenericRecords[T](items)` | `:109` | 泛型反射，任意结构体可用 |
| `RunExtractWithRetry` / `ExtractAttempt` | `extract_executor.go:102/25` | 通用重试 |

**缺的只有一件事**：把发票的「确定性前置规则」与「明细子项」纳入这套 schema 驱动体系。

## 3.3 两套管线并存的具体后果（P0）

| 维度 | 正式提取（invoice） | 沙箱测试 | 后果 |
|---|---|---|---|
| 内容拼装 | `BuildInvoiceExtractionBatches`（24000 **runes**，`\n\n`） | `buildFleetExtractionContent`（120000 **bytes**，`\n`） | 沙箱能提取到的尾部发票，生产会被截断 |
| 前置规则 | LineRules → MarkdownTable → FlatLayout → LLM | **无** | 沙箱看不到规则短路/规则漏抓的真实表现 |
| 结果视图 | `InvoiceExtractionItem` 结构体 | `map[string]any` | 同名字段两套归一规则 |
| 校验 | `validateInvoiceExtraction` + `FinalizeInvoiceRuleResult` | `ValidateGenericExtraction` | issues 文案与等级不一致 |
| 分类锁定 | `LockInvoiceCategory` | **无** | 沙箱不校验分类铁律 |
| 分页 | `page=1..N` | 单块 | — |

**修复方向（P0，1 人日）**：让 `TestExtractConfig` 在 `scope == "invoice"` 时也走 `runInvoiceCore` 的纯函数部分（抽成 `ExtractInvoiceByRulesAndLLM(ctx, chunks, cfg) (*InvoiceExtractionResult, error)`），沙箱只替换"模型调用"这一层。这样"沙箱通过 = 生产通过"。

## 3.4 `ScopeSchema` 驱动的通用化设计

### 3.4.1 契约（Go）

```go
// internal/types/extract_schema.go
type FieldType string // "text" | "number" | "date" | "array"

type FieldDef struct {
    Key       string      `json:"key"`        // 契约键（= 结构体 json tag）
    Label     string      `json:"label"`
    Type      FieldType   `json:"type"`
    Hint      string      `json:"hint"`       // 喂 LLM
    Aliases   []string    `json:"aliases"`    // 中文别名
    Required  bool        `json:"required"`   // 来自 KbExtractConfig，运行期覆盖
    Regex     string      `json:"regex"`
    SubFields []FieldDef  `json:"sub_fields"` // type == array 时
}

type ScopeSchema struct {
    Scope            string      `json:"scope"`
    Kind             string      `json:"kind"`          // invoice | document | archive
    CategoryLabel    string      `json:"category_label"`
    StandardFields   []FieldDef  `json:"standard_fields"`
    DynamicBucket    string      `json:"dynamic_bucket"` // 扩展字段落点："fields" | "extra"
    ItemBucket       string      `json:"item_bucket"`    // 明细落点："items"
    RegexPatchable   bool        `json:"regex_patchable"`
    Deterministic    []string    `json:"deterministic"`  // 启用的前置规则："line"|"md_table"|"flat"
    RecordFactory    func() any  `json:"-"`              // 生成一条空记录（结构体或 map）
}

type ScopeRegistry interface {
    Register(s ScopeSchema)
    Get(scope string) (ScopeSchema, bool)
    List() []ScopeSchema
}
```

### 3.4.2 四个能力的改造点

| 能力 | 现状 | 改造后 |
|---|---|---|
| `ai-suggest-fields` | `InvoiceStandardFieldDictionary()` 硬编码 | `BuildStandardFieldDictionary(schema ScopeSchema)` —— 从 schema 生成；发票 schema 由 `reflect` 取 `InvoiceExtractionItem` 的 json tag 自动填充（**已被 `invoiceContractKeys` 验证可行**） |
| `ai-suggest-rules` | prompt 接收 `dict []InvoiceStandardField` | 改名 `FieldDef`，逻辑不变；`guardAIRegexPatchable` 改读 `schema.RegexPatchable` |
| 正则补洞 | 53 处 `InvoiceExtractionItem` | 复用 `GenericExtractRecord` + `ValidateGenericExtraction`：把发票补洞搬到 `genericRegexPatch`，删除 `extract_regex_fallback.go` 的 400 行结构体专用实现 |
| 沙箱 | 独立管线 | 统一走 `schema.Deterministic` 声明的规则链 + `RunExtractWithRetry` |

### 3.4.3 落地顺序（关键：先做等价性测试）

```
Step 1  用 InvoiceExtractionItem 生成等价 ScopeSchema（不删旧代码）
Step 2  写「双跑测试」：同一 chunk 输入，旧 validateInvoiceExtraction 与新 ValidateGenericExtraction
        的 issues/patches 必须逐项相等（golden test，样本用现有 knowledge_invoice_mineru_test.go）
Step 3  等价后切 runInvoiceCore 到新路径，删除 extract_regex_fallback.go 的发票专用实现
Step 4  新增 contract/notice 只需注册 schema
```

**风险**：发票有 `remark` 片段字段（`INVOICE_REMARK_PART_FIELDS`）这类"无标准列"的写回逻辑，`GenericExtractRecord` 的 map 视图可覆盖，但需在 schema 里标 `DynamicBucket: "remark_parts"` 分流。

**工作量**：约 **6 人日**（含等价性测试 2 人日）。

---

# 维度 4：多解析引擎全链路兼容性专项审查

## 4.1 「10 个引擎」的真实构成（API 实测）

`GET /api/v1/system/parser-engines` 在 VM 上返回 **10 条**：

```
builtin              available=true    (DocReader)
simple               available=true    (Go 原生)
anydoc               available=true    (进程内)
weknoracloud         available=false
mineru               available=false
mineru_cloud         available=true
paddleocr_vl         available=false
paddleocr_vl_cloud   available=true
markitdown           available=true    ← 来自 docreader 的远程引擎
opendataloader       available=true    ← 来自 docreader 的远程引擎
```

**结构真相**：Go 侧 `RegisterEngine` 只注册 **8 个**（`docparser/engines.go:33-41`），
`markitdown` 与 `opendataloader` 是 **Python docreader 内部后端**，由 `ListAllEngines(..., remoteEngines)` 合并进来（`engine_registry.go:100-130`）。
因此「按引擎分支」的代码只能依赖 **engine name 字符串**，不能依赖 Go 类型。

## 4.2 解析配置优先级一致性（发现 P1 缺口）

| 场景 | 引擎选择来源 | 参数覆盖来源 | 代码 |
|---|---|---|---|
| **正式上传解析**（asynq） | KB 的 `ChunkingConfig.ParserEngineRules` | 租户 `ParserEngineConfig.ToOverridesMap()` | `knowledge_process.go:4212`；`knowledge.go:428` |
| **样票临时解析** | `tenant.ParserEngineConfig.ResolveChatParserEngine(ext)` → 读 **`ChatParserEngineRules`** | 同上 | `extract_suggest_parse.go:170`；`:126` |
| **聊天附件** | agent / tenant `ChatParserEngineRules` | 同上 | `temporary_document.go:425` |

**问题**：`ChatParserEngineRules` 在**全代码库没有任何写入点**：

```
grep -rn "ChatParserEngineRules" internal/ --include=*.go | grep -v _test
  types/config_redaction.go:177-178   （保留旧值）
  types/tenant.go:309                 （字段定义）
  types/tenant.go:382                 （读取）
```

而「系统设置 → 解析引擎」面板（`views/settings/ParserEngineSettings.vue`，`api/system/index.ts:115-170`）**只保存各引擎的端点/密钥/参数**，不保存引擎选择规则。

**结论**：样票解析永远走 `DefaultParserEngine(fileType)` → anydoc / ppt→markitdown / 其余 builtin。
若用户把某 KB 配成 MinerU，**样票解析仍用 builtin/anydoc** → 字段生成看到的文本形态与正式提取不同 → "AI 生成的字段正式提取时抓不到"。

**修复建议（P1，0.5 人日）**：
1. `extract_suggest_parse.go` 增加可选 `KBID`，优先读该 KB 的 `ResolveParserEngine(ext)`；
2. `bindSuggestSampleRequest` 接收 `kb_id`，前端 `AiFieldSuggestPanel` 已持有 `props.kbId`，透传即可；
3. 若确实要保留"系统面板统一指定样票引擎"，则必须在面板里补 `ChatParserEngineRules` 的编辑与保存链路。

## 4.3 Chunk 类型适配缺口（**P0，本次最重要的发现**）

### 4.3.1 事实

`chunkRepository.ListChunksByKnowledgeID`：

```go
// internal/application/repository/chunk.go:166-177
Where("tenant_id = ? AND knowledge_id = ? and chunk_type = ?", tenantID, knowledgeID, "text")
```

而 5 个业务的提取入口**全部**调用它：

| 业务 | 调用点 |
|---|---|
| contract | `knowledge_business.go:320` |
| invoice | `:1049` |
| invoice page | `:1575` |
| regulation | `:1920` |
| notice | `:2275` |

### 4.3.2 后果

1. 所有构建函数里这三行判断**永远为真**，是**死代码**：
   ```go
   if chunk.ChunkType != types.ChunkTypeText &&
      chunk.ChunkType != types.ChunkTypeImageOCR &&
      chunk.ChunkType != types.ChunkTypeImageCaption { continue }
   ```
   位置：`knowledge_invoice_extract.go:218-220`、`:938-940`、`:1513`、`:1706`、`:1750-1752`、`:1938-1940`；
   `knowledge_contract_extract.go:102-104/221-223`；
   `knowledge_regulation_extract.go:59-61/140-142`；
   `knowledge_award_punish_extract.go:72-74/153-155`；
   `knowledge_fleet_document.go:432-434`。

2. **纯扫描件提取必失败**：Simple/VLM 或 PaddleOCR-VL 若把正文写进 `image_ocr`（`image_multimodal.go:392`），提取侧拿到 0 个 chunk → `BuildInvoiceExtractionBatches` 退化为 `{Content:""}` → `runInvoiceCore` 走 "no_text" 分支落库。

### 4.3.3 实测数据（证明是"潜伏缺陷"而非"正在发生"）

发票知识库（182 文件）chunk 类型分布：

| chunk_type | 条数 | 涉及文件 |
|---|---|---|
| text | 3743 | 182 |
| summary | 185 | 181 |
| image_caption | **6** | 3 |
| image_ocr | **3** | 2 |

- `image_ocr`/`image_caption` **确实存在**（9 条），但**永远不会进入提取**；
- `files_without_text_chunks = 0` → 当前没有文件因此失败，属**潜伏缺陷**。

### 4.3.4 修复（P0，0.5 人日）

```go
// repository 已有现成方法（chunk.go:183）
chunks, err := h.chunkService.ListChunksByKnowledgeIDAndTypes(
    extractCtx, knowledgeID,
    []types.ChunkType{types.ChunkTypeText, types.ChunkTypeImageOCR, types.ChunkTypeImageCaption},
)
```
5 个调用点统一替换；同时在 `Build*ExtractionContent` 里去重（同一图的 OCR 与 caption 可能重复描述同一内容）。

**注意**：替换后 OCR chunk 的 `chunk_index` 与 text chunk 混排，需要确认 `Order("chunk_index ASC")` 仍然成立（`chunk.go:172`）。

## 4.4 表格形态统一

| 引擎 | 表格产物 | 归一化点 |
|---|---|---|
| DocReader (builtin) | Markdown / HTML `<table>` | `knowledge_process.go:4030` → `NormalizeHTMLTables` |
| MinerU / MinerU Cloud | HTML `<table>`（含行列合并） | 同上 + `mineru_layout.go` 对齐 |
| PaddleOCR-VL（自建/云） | HTML `<table>` | `paddleocr_vl_converter.go:71`、`paddleocr_vl_cloud_converter.go:91` **各自又调一次** |
| anydoc / MarkItDown / OpenDataLoader | GFM Markdown 表格（`|` 分隔） | **无转换**（Markdown 才能被 LLM 直接读） |
| Simple | 原样文本 | 无 |

**结论**：归一化点在**解析入库前**（`knowledge_process.go:4030`）覆盖了 HTML 表格；
但**提取侧没有任何二次归一** —— 若 chunk 是历史遗留或引擎绕过（如 `temporary_document.go:388/471` 只覆盖样票路径），提取器会拿到裸 HTML。

**建议（P2）**：在 `BuildInvoiceExtractionBatches` / `buildFleetExtractionContent` 入口统一调一次 `docparser.NormalizeHTMLTables(chunk.Content)`（幂等，`html_table_normalizer.go:88` 注释已确认可重复调用）。

**空白行/噪声清洗现状**：

| 函数 | 位置 | 行为 |
|---|---|---|
| `cleanInvoiceTemplateNoise` | `knowledge_invoice_extract.go:1170` | 仅发票，规则提取用 |
| `mdCleanText` | `:1569` | 仅 Markdown 规则 |
| `stripHTMLTags` | `:1965` | 仅 FlatLayout |
| **通用清洗** | **无** | 合同/通知/制度直接送原文 |

## 4.5 物理页码透传协议（设计）

### 4.5.1 现状

| 层 | 能力 |
|---|---|
| 解析产物 | `ReadResult.SourceBlocks`（`types/docparser.go:30`） |
| 落库 | `Chunk.SourceLocators`（`types/chunk.go:200`），`SourceLocator.Page` 1-based（`source_locator.go:44`） |
| 产出引擎 | **MinerU / MinerU Cloud**（`mineru_layout.go:141/211`）、**PaddleOCR-VL / Cloud**（`paddleocr_vl_layout.go:66`）、**DocReader**（若 Python 侧返回 `source_blocks`，`grpc_parser.go:174/213`） |
| 不产出 | anydoc、simple、markitdown、opendataloader（除 docreader 转出） |
| **提取侧消费** | **零引用**（`grep SourceLocators` 在 5 个 extract 文件中无命中） |
| 发票 `page` 字段 | `NormalizeInvoiceExtractionResult:1042` 自增序号；`FlatLayout:1851` 用 `len+1` |

### 4.5.2 建议协议

```go
// 1) 页码随 chunk 传给模型
type InvoiceExtractionBatch struct {
    Content    string
    StartIndex int
    EndIndex   int
    PageHint   string // 新增："chunks 3-8 correspond to pages 2,3,4"
}

// 2) 提示词要求模型回填物理页码
// "若原文含页码标记，请为每张发票输出 page_no（整数）；无标记时省略。"

// 3) 落库时优先用模型给的 page_no，缺失则回退序号
if inv.PageNo == 0 { inv.PageNo = seq }
```

**输出**：`InvoiceExtractionItem` 增 `PageNo int \`json:"page_no,omitempty"\``，前端详情抽屉可用它做"跳转到第 N 页"。

**工作量**：**2 人日**（含 5 个引擎的回归）。

---

# 维度 5：重构与中台化落地行动清单

## P0 — 高优 Bug 隐患与性能（进入下一模块前必须完成）

| # | 事项 | 文件 | 验收标准 | 工作量 |
|---|---|---|---|---|
| P0-1 | **提取改用 `ListChunksByKnowledgeIDAndTypes`**，纳入 image_ocr/caption | `internal/application/repository/chunk.go`（加服务方法）+ `knowledge_business.go:320/1049/1575/1920/2275` | 构造只有 image_ocr 的 fixture，提取能拿到文本；原有 golden test 全绿 | 0.5 人日 |
| P0-2 | **沙箱与生产同管线**：抽 `ExtractInvoiceByRulesAndLLM` | `knowledge_invoice_extract.go`（新增）+ `extract_config.go:464` | 同一 chunk 下沙箱结果 == 正式提取结果（golden 断言） | 1 人日 |
| P0-3 | **`/invoices` 投影表 `invoice_records`** | 新增 migration + `runInvoiceCore` 落库处 + `knowledge_invoice_list.go:42` | 182 文件下 P95 < 150 ms；1109 行结果不变 | 3 人日 |
| P0-4 | **超长采样告警**：`sampleLongContent` 触发时写 `extract_warnings` | `knowledge_invoice_extract.go:975`、`BuildInvoiceExtractionBatches` | 前端质量面板可见"本文档被采样，可能漏票" | 0.5 人日 |
| P0-5 | 提取总超时 30 min → 45 min（或按批数动态） | `knowledge_business.go:66` | 27 票文档 12 批不触顶 | 0.2 人日 |
| P0-6 | 前端筛选去抖：3 个 `watch` 合并为 1 个 `debounce(300ms)` | `InvoiceManagement.vue:1377/1383/1390` | 一次操作只发 1 次 `/invoices` | 0.3 人日 |

**P0 合计：约 5.5 人日**

## P1 — 前端中台组件解耦与提炼

| # | 事项 | 文件 | 验收标准 | 工作量 |
|---|---|---|---|---|
| P1-1 | `BusinessScopeRegistry` + `registry/invoice.ts` | 新增 `frontend/src/registry/*` | 12 处 `scope === 'invoice'` 全部消除；lint 禁新增字面比较 | 2 人日 |
| P1-2 | `extractFieldMeta.ts` 改为 registry 驱动 | `extractFieldMeta.ts:27-43/110-120/139-144` | `isRegexPatchable` / `extractFieldLabel` 不再有 `'invoice'` 字面量 | 0.5 人日 |
| P1-3 | `BusinessFilterBar.vue` | 由 `BusinessListToolbar` 升级 | 发票页 4 个筛选降为 spec 配置，模板减少 ~25 行 | 1.5 人日 |
| P1-4 | `BusinessSubItemsPopover.vue` | 搬运 `InvoiceManagement.vue:1603-1682` | 明细浮层可独立 Storybook 渲染 | 1 人日 |
| P1-5 | `BusinessSplitDetailDrawer.vue` | 搬运 `:300-425` + `fillEditForm/persistEditForm` | 抽屉可被合同页直接复用（传 fields 即可） | 3 人日 |
| P1-6 | `useBusinessListSync.ts` | 搬运 `:2311-2397` + 占位行分支 | 合同页接入后占位行/级联删除行为一致 | 2 人日 |
| P1-7 | 分类接口缓存 `useExtractCatalog` | `FleetCertPanel.vue:546`、`ExtractRulePanel.vue:1180` | 打开设置抽屉只发 1 次请求/scope（30s 内复用） | 1 人日 |
| P1-8 | `FleetCertPanel.load()` 去掉写库副作用 | `FleetCertPanel.vue:549-556` | GET 路径零写操作；迁移逻辑挪到一次性 job | 0.5 人日 |

**P1 合计：约 11.5 人日**

## P2 — 后端提取器泛型化与多引擎统一适配

| # | 事项 | 文件 | 验收标准 | 工作量 |
|---|---|---|---|---|
| P2-1 | `ScopeSchema` + `ScopeRegistry` | 新增 `internal/types/extract_schema.go` + `internal/application/service/extract_schema_registry.go` | 发票/合同/制度/通知各注册一份 | 2 人日 |
| P2-2 | `ai-suggest-fields` 改成 schema 驱动 | `extract_suggest_ai.go:254/273/412` | 新增 scope 零代码改动即有字段建议 | 1.5 人日 |
| P2-3 | `ai-suggest-rules` 同上 | `extract_suggest_ai.go:856` | 同上 | 1 人日 |
| P2-4 | 正则补洞泛型化 + 等价性测试 | `extract_regex_fallback.go`（删 400 行）+ `extract_generic_validate.go:416` | 发票 golden test 的 patches 逐项相等 | 2 人日 |
| P2-5 | 引擎能力矩阵 + 页码透传 | `BuildInvoiceExtractionBatches:932`、`normalizeSuggestText`、`types.InvoiceExtractionItem` | MinerU/PaddleOCR 的 `page_no` 正确回填；其余 6 引擎回退序号 | 2 人日 |
| P2-6 | 提取内容二次归一 + 统一清洗 | 5 个 `Build*ExtractionContent` | 裸 HTML chunk 进入时自动转 Markdown 表 | 1 人日 |
| P2-7 | 拆 `knowledge_invoice_extract.go` 为 4 个文件 | 同上 | 单文件 ≤ 600 行；`go test ./...` 全绿 | 2 人日 |
| P2-8 | 拆 `runInvoiceCore`（504 行）为 6 个阶段函数 | `knowledge_business.go:991-1495` | 单函数 ≤ 120 行；行为不变（golden） | 2 人日 |

**P2 合计：约 13.5 人日**

## 总工作量与建议排期

```
P0（5.5 人日）  ← 阻塞下一模块，必须先做
     ↓
P1（11.5 人日） ← 与合同模块开发并行（先用 registry 承接）
     ↓
P2（13.5 人日） ← 合同模块上线后再收口，但 P2-1/P2-2 要提前到 P1 期间做（否则合同又要复制一套）
```

**关键依赖**：`P2-1 ScopeSchema` 必须在**合同模块动工前**完成 —— 否则合同会再复制一套
`ai-suggest-*` 与补洞代码，等于制造第二个"发票债务"。

---

# 附录 A：本次实测命令与原始数据

```sql
-- 数据规模
SELECT count(*) files, pg_size_pretty(sum(pg_column_size(custom_metadata))::bigint) meta_total
FROM knowledges WHERE deleted_at IS NULL
  AND knowledge_base_id='3919e8d5-190e-4e1f-adca-77af7f117cfc';
-- 182 | 311 kB

SELECT sum(jsonb_array_length(COALESCE(custom_metadata->'invoices','[]'::jsonb))) FROM knowledges ...;
-- 1109

-- chunk 类型分布
SELECT chunk_type, count(*), count(DISTINCT knowledge_id) FROM chunks
WHERE knowledge_id IN (SELECT id FROM knowledges WHERE knowledge_base_id='...')
GROUP BY 1 ORDER BY 2 DESC;
-- text 3743/182 | summary 185/181 | image_caption 6/3 | image_ocr 3/2
```

```bash
# 接口耗时（5 次）
curl -w "t=%{time_total}\n" .../invoices?page=1&page_size=20
# 1.396 / 1.084 / 1.136 / 1.104 / 1.069 s

# 引擎清单（10 个，含 docreader 远程引擎）
curl .../system/parser-engines | jq '.data | length'   # 10
```

# 附录 B：`ScopeSchema` 落地前必须先做的等价性 Golden 测试

现有 `engineering_invoice_mineru_test.go` 已积累 3 个真实 MinerU 变体
（`variantA` 48627547 / `variantB` 49367445 / `taxExempt` 34013247）。
把它们扩展为"双跑对照"：

```go
func TestScopeSchemaEquivalence_Invoice(t *testing.T) {
    for _, tc := range mineruFixtures() {
        oldRes, _ := ExtractInvoicesByFlatLayout(tc.content)
        oldIssues := validateInvoiceExtraction(oldRes, tc.cfg)

        recs := NewGenericRecords(oldRes.Invoices)
        newIssues, newPatches := ValidateGenericExtraction(recs, tc.content, tc.cfg)

        assert.Equal(t, normalizeIssues(oldIssues), normalizeIssues(newIssues))
        assert.Equal(t, oldPatches, newPatches)
    }
}
```

只有这个测试绿了，P2-4 的"删除 400 行发票专用补洞"才允许执行。
