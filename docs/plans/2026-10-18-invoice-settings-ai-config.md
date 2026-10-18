# 「发票设置」现状摸底与智能配置设计报告

> 分支：`daily-affairs-v4`（本地 HEAD `585528e95`）
> 本轮为**只读摸底**，未修改任何业务代码、未提交 Commit。
> 所有结论均给出真实文件路径、函数名与行号；VM 实测数据取自 `root@10.10.10.239:5522`。

---

## 0. 结论速览

| # | 结论 | 影响 |
|---|---|---|
| 1 | 「字段定义」= `fleet_categories.subs`（JSONB）；「提取规则」= `knowledge_extract_configs.fields`（JSONB）。**字段名以 subs 为唯一权威源**，规则表只存 desc/rule/type/regex/required 增强 | 双源不是"同步"关系，是"主从 + 覆盖式清洗"关系 |
| 2 | **P0 缺陷**：`FleetCertPanel` 保存 subs 时**丢弃 `sub_fields`**，任何一次字段编辑都会清空明细子列，直到下次进程重启 seed 补回 | 智能字段生成要写 sub_fields，必须先补这条链路 |
| 3 | 沙箱「测试规则」**不支持上传文件**，只能复用 KB 内已解析文档（`knowledge_id` + chunks） | 不能现成复用；需新建上传解析入口 |
| 4 | 但后端**已有完整可复用的临时解析管线** `temporary_document.go`（24h TTL、解析器选择、VLM OCR 兜底、不落知识库） | 智能字段生成可低成本复用 |
| 5 | 沙箱测试走**通用 fleet 提取链路**，与发票正式提取链路（规则三件套 → LLM → 补洞）**不是同一条** | 测试结果不能等价推导正式提取效果 |
| 6 | 前端 `isStandardField` 认可 **22 个**发票标准键，后端真正能补洞的只有 **8 个**（`invoiceFieldOrder`） | AI 生成正则的边界应按后端 8 个来，否则"配了不生效" |

VM 实测分类现状（`GET /api/v1/fleet/categories?scope=invoice`）：

```
普通发票     enabled=False fields=17 items_subfields=8
专用发票     enabled=False fields=17 items_subfields=8
水电发票     enabled=True  fields=17 items_subfields=8
通行费发票    enabled=True  fields=18 items_subfields=8
三联收据     enabled=False fields=11 items_subfields=0
其它票据     enabled=True  fields=17 items_subfields=8
```

---

## 1. 「发票设置」前端组件树与状态流转

### 1.1 组件树与文件定位

```
InvoiceManagement.vue                        # 发票列表页
└─ StandardSettingDrawer                     # src/components/business/StandardSettingDrawer.vue
   ├─ Tab「字段定义」→ FleetCertPanel         # src/views/dailyAffairs/FleetCertPanel.vue
   └─ Tab「提取规则」→ ExtractRulePanel       # src/views/dailyAffairs/ExtractRulePanel.vue
        └─ ExtractTestDialog                 # src/views/dailyAffairs/ExtractTestDialog.vue
             ├─ TestInputPanel.vue
             └─ TestResultPanel.vue
```

- 入口：`frontend/src/views/dailyAffairs/InvoiceManagement.vue:283`
  ```vue
  <StandardSettingDrawer v-model:visible="invoiceSettingsVisible" :kb-id="kbId || ''" @saved="() => reloadColumns(true)" />
  ```
- 抽屉骨架：`StandardSettingDrawer.vue:11-18`（双 Tab）
  - 字段定义 Tab：`StandardSettingDrawer.vue:12-14` → `<FleetCertPanel :scope="scope" :kb-id="kbId" />`
  - 提取规则 Tab：`StandardSettingDrawer.vue:15-17` → `<ExtractRulePanel ref="extractRef" :scope :kb-name :kb-id />`
  - 默认 scope=`'invoice'`、title=`'发票设置'`（`StandardSettingDrawer.vue:41-46`）

### 1.2 「字段定义」Tab 如何编辑一个分类（如水电发票 17 字段）

| 步骤 | 位置 | 行为 |
|---|---|---|
| 1. 加载 | `FleetCertPanel.vue:456-479` `load()` | `listFleetCategories({scope})` → `categories.value[scope]` |
| 2. 合并展示 | `FleetCertPanel.vue:344-370` `buildItems()` | 已入库分类（按 `sort_order`）+ 内置未入库兜底 |
| 3. 点击行展开 | `FleetCertPanel.vue:558-573` `toggleEdit()` | **L563-570**：`fieldsEditable ← item.subs`（无 subs 时回退 `BUILTIN[scope]` 的 base+detail） |
| 4. 字段行编辑 | `FleetCertPanel.vue:91-124` | 名称 / 数据类型 / 宽度 / 默认表头 / 启用；**invoice scope 字段名只读**（L96 `fc-name-static` + `invoiceFieldLabel`） |
| 5. 增 / 删 / 改 | `addField():607`、`removeField():614`、`duplicateField():599` | 仅操作内存数组 `fieldsEditable` |
| 6. 保存 | `FleetCertPanel.vue:618-652` `commitEdit()` | **L623-625** 映射为 subs → **L639** `updateFleetCategory(item.id, { name, subs, enabled })` |
| 7. 通知 | `FleetCertPanel.vue:423-425` `notifyCategoriesChanged()` | `window.dispatchEvent(new CustomEvent('fleet-categories-changed'))` |

**修改保存在哪里**：`fleet_categories` 表 `subs` 列（`tenant_id=0, scope='invoice'`），一次 PUT 整体覆盖。

### 1.3 「提取规则」Tab 如何感知并合并字段

这是关键链路：

```ts
// ExtractRulePanel.vue:699-721
function authorityFields() {
  const scope = currentScope.value
  const catList = categories.value[scope] || []
  let c = catList.find(x => x.name === certTypeName.value)      // ← 按选中分类取
  if (!isFleetScope.value && !c) c = catList.find(x => Array.isArray(x.subs)) || catList[0]
  if (c && Array.isArray(c.subs) && c.subs.length) {
    return c.subs.map(s => ({ name, enabled, dataType }))         // ← 字段名唯一权威源 = subs
  }
  ...回退 DEFAULT_FIELDS 底稿
}
```

```ts
// ExtractRulePanel.vue:757-805  loadConfig()
const auth = authorityFields()                 // ① 字段名来自 subs
const draft = DEFAULT_RULES[scope][certType]   // ② 底稿 desc/rule
const res = await getExtractConfig(kbId, scope, certType)   // ③ 已保存规则
fields.value = auth.map(a => ({
  name: a.name,
  desc:  c?.desc || d.desc || '',
  type:  c?.type || map(a.dataType),
  rule:  c?.rule || d.rule || '',
  enabled: c?.enabled ?? a.enabled,
  regex:   c?.regex || '',
  required: c?.required ?? false,
}))
```

**合并语义**：`subs`（权威名） ⟵ 叠加 `extract_config`（增强） ⟵ 叠加 `DEFAULT_RULES`（底稿）。
**感知时机**三条：

1. 切 Tab：`StandardSettingDrawer.vue:62-64` → `extractRef.value?.refresh()`
2. 全局事件：`StandardSettingDrawer.vue:66-72` → `onCategoriesChanged()` → `refresh()`（由 `FleetCertPanel` 保存/增删改/排序后派发）
3. `refresh()` 本体：`ExtractRulePanel.vue:871-887`，重新 `listFleetCategories` + `loadConfig()`，并 `defineExpose({ refresh })`（L887）

**保存时的反向清洗**（`ExtractRulePanel.vue:838-845`）：
```ts
const activeNames = new Set(authorityFields().filter(a => a.enabled).map(a => a.name))
const cleanFields = fields.value.map(f => ({...f})).filter(f => activeNames.size === 0 || activeNames.has(f.name))
```
→ 不在「启用 subs」里的旧规则字段会被前端**主动剔除**；后端 `SaveExtractConfig` 还会再剔一次（`internal/handler/extract_config.go:261-272`），对未知/已禁用字段**记 WARNING 后静默丢弃**，不报错。

**⚠️ P0 缺陷（本次摸底最重要的发现）**：
`FleetCertPanel` 两处 subs 映射都**不含 `sub_fields`**：

```ts
// FleetCertPanel.vue:563-570  toggleEdit（读）
fieldsEditable.value = raw.map(s => ({ name, enabled: s.enabled!==false, isDefault: s.is_default===true,
                                       dataType: s.data_type||'text', width: Number(s.width)||0 }))
// FleetCertPanel.vue:623-625  commitEdit（写）
const subs = fieldsEditable.value.map(f => ({ name, enabled, is_default, data_type: f.dataType||'text', width: Number(f.width)||0 }))
```

`FleetCategorySub.SubFields` 是 `subs` JSONB 内的嵌套字段（`internal/types/fleet.go:142-143`）。前端读→写不搬运它，因此**每次「字段定义」保存都会把该分类 items 字段的 `sub_fields` 抹成空**，直到进程重启时 `seedInvoiceItemSubFields` 检测到 `len(sub_fields)==0` 才补回（`internal/handler/fleet.go:1990-1993`）。
`duplicateItem` 同样丢（`FleetCertPanel.vue:727`）。

---

## 2. 双数据源同步机制

### 2.1 两张表 / 两套接口

| | 字段定义 | 提取规则 |
|---|---|---|
| 表 | `fleet_categories`（`internal/types/fleet.go:147-162`） | `knowledge_extract_configs`（`internal/types/extract_config.go:21-37`） |
| 承载列 | `subs jsonb`（`[]FleetCategorySub`） | `fields jsonb`（`[]ExtractFieldConfig`） |
| 维度 | `tenant_id(=0 业务 scope) × scope × name` | `knowledge_base_id × scope × cert_type` |
| 读 | `GET /api/v1/fleet/categories?scope=invoice`<br>`FleetHandler.ListFleetCategories` `internal/handler/fleet.go:898-923` | `GET /api/v1/knowledge-bases/:id/extract-config?scope&cert_type`<br>`BusinessExtractHandler.GetExtractConfig` `internal/handler/extract_config.go:161-186` |
| 写 | `POST/PUT /api/v1/fleet/categories[/:id]`<br>`CreateFleetCategory:928-989` / `UpdateFleetCategory:994-1048` | `POST /api/v1/knowledge-bases/:id/extract-config`<br>`SaveExtractConfig:198-329` |
| 版本 | 无 | `version++` + 快照表 `knowledge_extract_configs_versions`，保留最近 20 版（`extract_config.go:20`, `307-322`） |

路由注册：`internal/router/routes_knowledge.go:141-143`（extract-config）、`internal/router/routes_knowledge.go:506-510`（categories）。

### 2.2 标准字段清单分别定义在哪

| 位置 | 内容 | 数量 |
|---|---|---|
| **前端** `frontend/src/views/dailyAffairs/invoiceFieldLabels.ts:4-27` | `INVOICE_FIELD_LABELS`（英文契约键 → 中文名）：invoiceNo/invoiceCode/invoiceType/invoiceDate/buyerName/sellerName/buyerTaxNo/sellerTaxNo/amount/tax/taxAmount/taxRate/totalAmount/issuer/remark/extractStatus/tags/fileName/pdfUrl/sourceFile/invoice_category/items | 22 |
| **前端** `extractFieldMeta.ts:27-43` | `STANDARD_KEYS.invoice` = 上面 22 键 + `invoice_no/invoice_code/invoice_date/buyer_tax_no/seller_tax_no/buyer_name/seller_name/tax_amount`；经 `canonicalFieldKey` 归一后去重仍是 **22** 个 canonical 键 | 22（canonical 去重后） |
| **前端** `invoiceFieldLabels.ts:36-64` | `INVOICE_FIELD_KEY_ALIASES`：中文自定义名 → 标准英文键（收据编号/价税合计/车牌号→plate_no…） | ~60 条 |
| **前端** `invoiceFieldLabels.ts:67` | `INVOICE_REMARK_PART_FIELDS`（无标准存储位，并入 remark 片段） | 8 |
| **后端** `internal/application/service/knowledge_invoice_extract.go:38-74` | `InvoiceExtractionItem` 结构体 json tag（真正的落库契约）：invoice_no/invoice_code/invoice_date/invoice_type/total_amount/amount/tax/tax_rate/seller_*/buyer_*/issuer/remark/category/items/void_flag/duplicate/fields/page | 28 |
| **后端** `knowledge_invoice_extract.go:269-347` | `invoiceFieldAliases`：模型输出中文 key → 标准 snake_case（最长包含命中） | ~80 条 |
| **后端（补洞白名单）** `extract_regex_fallback.go:106-111` | `invoiceFieldOrder` = `invoice_no, invoice_code, invoice_date, buyer_name, buyer_tax_no, seller_name, seller_tax_no, issuer` | **8** |
| **后端（补洞读写器）** `extract_regex_fallback.go:71-104` | `invoiceFieldAccessors` | 8 |
| **后端（日期归一）** `extract_generic_validate.go:281-289` | `standardDateFields` | 8 |

**⚠️ 不对称**：前端 `isStandardField(scope,'invoice',name)`（`extractFieldMeta.ts:55-59`）对 22 个键都返回 true，UI 上 `required`/`regex` 输入框**不置灰**；但后端发票正式链路的补洞只认 `invoiceFieldOrder` 的 8 个（`RegexFallbackInvoiceFields` `extract_regex_fallback.go:140-195`）。用户给 `totalAmount`/`taxRate` 配了正则，正式提取**不会执行**（沙箱里会执行，因为沙箱走 `genericRegexPatch` 的 map 视图，见 `extract_generic_validate.go:416-453`）。
→ **AI 生成正则的字段白名单必须取后端 8 个，而不是前端 22 个。**

### 2.3 新增自定义字段 / 明细子列时两表怎么写

**新增一个自定义字段（如「户号」）**：

1. 前端 `FleetCertPanel.addField()`（L607-613）push 进 `fieldsEditable`
2. `commitEdit()`（L623-625）→ `PUT /fleet/categories/:id` `{name, subs:[..., {name:'户号', enabled:true, is_default:false, data_type:'text', width:0}]}`
3. 后端 `UpdateFleetCategory`（`internal/handler/fleet.go:1037-1039`）`updates["subs"] = string(dataJSON)` 整体覆盖
4. 提取规则侧：直到用户在「提取规则」Tab 选到该分类并点保存，`knowledge_extract_configs.fields` 才会出现该字段；**保存时后端校验字段名必须在该分类启用 subs 内**（`extract_config.go:250-273`），否则静默剔除
5. 列表页列：`InvoiceManagement.reloadColumns()`（L1222-1245）按当前分类 subs 重建列；无标准存储位字段走 remark 片段（`isRemarkPartField`）
6. 提取侧：字段名进 Prompt（`buildInvoiceRuleSystemPrompt` `knowledge_invoice_extract.go:548-603`）；模型输出的未知 key 收进 `InvoiceExtractionItem.Fields` 动态桶（`knowledge_invoice_extract.go:65-68`）；中文名经 `invoiceFieldAliases` / `INVOICE_FIELD_KEY_ALIASES` 归一

**新增明细子列（`sub_fields`）**：

- 结构：`FleetItemSubField{Key,Label,DataType,Width}`（`internal/types/fleet.go:128-133`），挂在 `subs` 中 `data_type='items'` 那一项的 `sub_fields`
- 后端**唯一权威源**：`subs`；提取时由 `injectInvoiceItemSubFields` 实时读分类注入到 `cfg.Fields[Type=items].SubFields`（`internal/handler/extract_config.go:61-121`，含"items 缺失时自动追加"与"强制 Enabled"）
- 消费：`invoiceItemsSubFieldsInstruction`（`knowledge_invoice_extract.go:610-647`）生成"明细行必须按这些键输出"指令；列表/详情由 `InvoiceManagement.subFieldsOfRow`（L1593-1601）读取
- 写入：**当前前端无编辑入口**（全仓 `grep sub_fields` 只有 `InvoiceManagement.vue` 的读取侧）。只能靠后端 seed（`internal/handler/fleet.go:1921-2010`，模板 `invoiceItemSubFieldsStandard()` 8 列 / `invoiceItemSubFieldsToll()` 通行费 8 列）或直接改库
- ⚠️ 叠加 1.3 的 P0：即使 seed 写进去了，用户只要在「字段定义」点一次保存就会被抹掉

---

## 3. 现有「沙箱测试解析」能力复用盘点

### 3.1 链路与"不污染"是怎么做到的

- 入口：`ExtractRulePanel.vue:10`「测试规则」→ `ExtractTestDialog`（`ExtractRulePanel.vue:129-130`）
- 请求：`ExtractTestDialog.vue:170-200` `runTest()` → `testExtractConfig(kbId, payload)` → `POST /api/v1/knowledge-bases/:id/extract-config/test`
- 后端：`BusinessExtractHandler.TestExtractConfig` `internal/handler/extract_config.go:341-480`

关键实现（`extract_config.go:385-443`）：

```go
// 文本优先，否则复用知识库已解析文件
if strings.TrimSpace(req.Text) != "" {
    content = req.Text
} else if kid := strings.TrimSpace(req.KnowledgeID); kid != "" {
    knowledge, _ := h.kgService.GetKnowledgeByIDOnly(effCtx, kid)
    // 归属校验 + ParseStatus 必须 completed
    chunks, _ := h.chunkService.ListChunksByKnowledgeID(effCtx, kid)
    content = buildFleetExtractionContent(knowledge.FileName, knowledge.Description, chunks)
}
...
modelID := kb.SummaryModelID                       // 复用知识库 summary 模型
chatModel, _ := h.modelService.GetChatModel(effCtx, modelID)
sysPrompt := service.BuildExtractSystemPrompt(ec)  // extract_config_prompt.go:72
result, _ := service.ExtractFleetDocumentFromContent(effCtx, chatModel, content, scope, certType, fieldNames, sysPrompt)
```

**"不污染"的原理 = 只读**：它不创建 knowledge、不写 `knowledge_extract_configs`、不改 `custom_metadata`；只是把已解析 chunk 拼成文本喂给模型。质量闭环（Phase D）在同一函数内跑一遍 `ValidateGenericExtraction`，返回 `extract_issues` / `patched_by_regex` / `regex_patches`（`extract_config.go:445-479`）。

### 3.2 能否直接复用做「智能字段生成」的样票解析？

**不能现成复用**，三点不符：

1. **不支持上传**：`ExtractTestDialog.loadFiles()`（L156-168）只是 `listKnowledgeFiles(kbId, {parse_status:'completed'})` 列 KB 内已有文件；前端没有任何 FormData 上传路径
2. **语义不同**：测试是"用规则跑一次提取"，字段生成是"从票面反推有哪些字段"
3. **链路不同**：见 3.3

### 3.3 ⚠️ 沙箱 ≠ 正式发票链路

`TestExtractConfig` 调的是**通用 fleet 提取** `service.ExtractFleetDocumentFromContent`（`internal/application/service/knowledge_fleet_document.go:347`），输出 `{kind, doc_type, fields}`；
而发票正式提取走 `runInvoiceCore`（`internal/handler/knowledge_business.go:974`）→ `ExtractInvoicesByLineRulesFromChunks` → `ExtractInvoicesByMarkdownTableRulesFromChunks` → `ExtractInvoicesByFlatLayoutFromChunks` → 回退 `ExtractInvoicesFromContentWithRules`（`knowledge_invoice_extract.go:652`），输出 `{kind, invoices[]}`，且规则由 `resolveInvoiceRuleCfg` + `injectInvoiceItemSubFields` 装配（`knowledge_business.go:1090-1093`）。

→ 发票分类点「测试规则」看到的字段结构与正式落库结构**不是一套**，`extract_config/test` 的返回值不能直接当作正式提取的预演。

### 3.4 ✅ 可直接复用的底层：`temporary_document.go`

后端**已经有一条完整的「上传 → 临时解析拿文本 → 不落知识库」管线**：

- 文件：`internal/application/service/temporary_document.go`
- 实体：`types.TemporaryDocument`（`internal/types/temporary_document.go:24-48`），`ExpiresAt` 默认 **24h TTL**（`temporary_document.go:26`），有 `CleanupExpired`（L824）
- 创建（异步）：`(*temporaryDocumentService).Create` L145-222 —— 校验扩展名/大小 → `fileService.SaveBytes` → 落 `temporary_documents` → 投递 `TypeTemporaryDocumentProcess`（asynq，MaxRetry 2 / Timeout 10min）
- **同步解析核心**：`(*temporaryDocumentService).parse` **L410-522**
  - L455-471：`docparser.NewReader(ctx, parserEngine, ext, false, deps)` + `reader.Read(ctx, request)` → `ReadResult.MarkdownContent`
  - L472-475：行尾归一 + `docparser.NormalizeHTMLTables`
  - L524-575：`applyImageUnderstanding` —— **纯扫描件/图片走 VLM OCR 兜底**（`understandImagesWithVLM` L569，`GetVLMModel`），这正是发票样票（PDF 扫描件/JPG）最需要的
- 已有路由：`POST /api/v1/sessions/:session_id/attachments` 等（`internal/router/routes_chat.go:61-65`），但**绑定聊天会话**

**复用建议**：在 `package service` 内新增一个导出函数（例如 `ParseFileForExtractSuggest`），复用 `parse()` 的解析器选择 + VLM OCR 兜底，但**不写 `temporary_documents` 表、不绑定 session**，直接返回纯文本。因为与 `parse()` 同包，`supportsExtension`（L224）、`applyImageUnderstanding`（L524）、`understandImagesWithVLM`（L569）都可直接调用，成本很低。

---

## 4. 接口契约与 Prompt 设计提案

### 4.1 后端新增接口

注册位置：`internal/router/routes_knowledge.go:140-143` 同组（`kbDirect`，`g.OwnedKBOrAdmin()` + `g.KBAccessWrite("id")`），与 `SaveExtractConfig` 权限一致。

---

#### 接口 A：`POST /api/v1/knowledge-bases/:id/extract-config/ai-suggest-fields`

`Content-Type: multipart/form-data`（样票文件）或 `application/json`（已有文本二选一）。

**Request**

| 字段 | 类型 | 说明 |
|---|---|---|
| `scope` | string | 固定 `invoice`（与 `validateExtractScope` 一致，`extract_config.go:123-131`） |
| `cert_type` | string | 必填，发票分类名（水电发票/通行费发票…） |
| `file` | file | 样票（PDF/JPG/PNG），可选 |
| `text` | string | 已有票面文本，可选；与 `file` 二选一 |
| `include_disabled` | bool | 是否把当前禁用字段也纳入候选，默认 false |

**Response**

```jsonc
{
  "success": true,
  "data": {
    "cert_type": "水电发票",
    "sample_text": "…（解析后的票面纯文本，≤12000 字符截断）",
    "sample_text_truncated": false,
    // ① 标准字段启用建议：键必须是 InvoiceExtractionItem 的契约键
    "standard_fields": [
      { "key": "invoice_no",   "label": "发票号码",       "enable": true,  "sample": "24312000000012345678", "data_type": "text" },
      { "key": "invoice_date", "label": "开票日期",       "enable": true,  "sample": "2026-07-05",           "data_type": "date" },
      { "key": "tax_rate",     "label": "税率",           "enable": false, "sample": "",                     "data_type": "number" }
    ],
    // ② 明细子列建议（data_type=items 那一项）
    "item_sub_fields": [
      { "key": "name", "label": "项目名称", "data_type": "text",   "width": 140 },
      { "key": "account_no", "label": "户号", "data_type": "text",  "width": 120 }
    ],
    // ③ 新增自定义字段建议（无标准存储位 → 走 Fields 动态桶 或 remark 片段）
    "custom_fields": [
      { "name": "户号", "data_type": "text", "sample": "5001234567",
        "store": "fields",            // fields=动态桶 | remark=并入备注片段
        "remark_part": false }
    ]
  }
}
```

**服务端处理顺序（建议）**

1. `validateKnowledgeBaseAccessWithKBID` + 角色 Admin/Editor（对齐 `TestExtractConfig` L348-356）
2. 取文本：`file` → `ParseFileForExtractSuggest`（复用 3.4）；`text` → 直接用
3. 取模型：`kb.SummaryModelID` → `h.modelService.GetChatModel`（对齐 `extract_config.go:419-429`）
4. 取字典：
   - 标准键字典从 **`InvoiceExtractionItem` 的 json tag 反推**（或新增一份常量镜像），保证与落库契约同源；附带 `canonicalFieldKey` 归一
   - 当前分类 subs 从 `fleet_categories`（`tenant_id=0, scope=invoice, name=cert_type`）读出作为"已选"基线
5. 调 LLM：Prompt 见 4.3-A；要求严格 JSON 输出（复用 `RunExtractWithRetry` 的 attempts 序列，`knowledge_fleet_document.go:362-367`）
6. 后处理（**不信任模型输出**）：
   - 白名单过滤：`standard_fields[].key` 必须命中 `InvoiceExtractionItem` 契约键，否则丢弃
   - `item_sub_fields[].key` 必须匹配 `^[a-z][a-z0-9_]{1,31}$`（要与 `items[].extra` 及 `invoiceLineFieldAliases` 能对上）
   - `custom_fields[].name` 去重 + 排除与标准键 canonical 冲突的
   - `sample` 必须能在 `sample_text` 中原文命中（防幻觉）
7. 返回（**不写库**：不落 `extract_config`、不落 `fleet_categories`；`sample_text` 只回给前端缓存，前端透传给接口 B）

---

#### 接口 B：`POST /api/v1/knowledge-bases/:id/extract-config/ai-suggest-rules`

`Content-Type: application/json`。

**Request**

```jsonc
{
  "scope": "invoice",
  "cert_type": "水电发票",
  "fields": [                       // 当前分类启用字段（前端从 authorityFields() 现取，权威源仍是 subs）
    { "name": "invoiceNo",  "data_type": "text" },
    { "name": "户号",        "data_type": "text" }
  ],
  "sample_text": "…",               // 可选；来自接口 A 缓存
  "overwrite": false                // true=覆盖已有 desc/rule；false=只填空的（默认 false，防覆盖铁律）
}
```

**Response**

```jsonc
{
  "success": true,
  "data": {
    "fields": [
      {
        "name": "invoiceNo",
        "desc": "发票号码",
        "rule": "提取票面“发票号码 / 发票号”栏；票面『机器编号』『校验码』不得写入本字段；无标签时留空",
        "required": true,
        "regex": "\\b\\d{20}\\b",
        // 正则为何被接受/拒绝，前端可直接展示
        "regex_status": "accepted",          // accepted | rejected_not_standard | rejected_compile | rejected_not_unique
        "regex_reason": "样票上唯一命中 1 次"
      },
      {
        "name": "户号",
        "desc": "用电户号",
        "rule": "提取票面“户号 / 用户编号”栏",
        "required": false,
        "regex": "",
        "regex_status": "rejected_not_standard",
        "regex_reason": "自定义扩展字段不参与后置正则补洞"
      }
    ],
    "prompt_template": "你是一个水电发票信息提取助手…{{document_text}}…{{fields_schema}}…"
  }
}
```

**正则安全边界（必须实现，对应 Phase B/D 既有约定）**

```go
// 1) 仅对系统结构体内置标准字段生成 —— 白名单就是后端真正能补洞的 8 个
//    internal/application/service/extract_regex_fallback.go:106-111
patchable := map[string]bool{}   // invoice_no/invoice_code/invoice_date/buyer_name/buyer_tax_no/seller_name/seller_tax_no/issuer

// 2) Go RE2 编译校验：不过就丢弃，绝不下发给前端
re, err := regexp.Compile(pattern)     // 与 genericRegexPatch 同口径（extract_generic_validate.go:428）
if err != nil { drop("rejected_compile") }

// 3) 唯一命中校验（进阶，可选但强烈建议）：
//    复用 service.uniqueRegexCapture（extract_regex_fallback.go:227-250）
//    必须在样票文本上**恰好命中一个值**，否则该正则会误补 —— 直接丢弃
if v, ok := service.UniqueRegexCapture(sampleText, re); !ok { drop("rejected_not_unique") }

// 4) 禁止宽泛表达式：长度上限 + 禁止裸 .* 等
```

前端同样要置灰：非标准键的 `regex` 输入框继续走 `isStdField`（`ExtractRulePanel.vue:658`）；**建议把 `extractFieldMeta.ts` 的 `STANDARD_KEYS.invoice` 收窄到与后端 8 个一致**（或新增 `isRegexPatchable()` 单独判定正则、保留 `isStandardField()` 判定必填），消除 2.2 里"配了不生效"的落差。

**`overwrite=false` 的必要性**：后端启动时 `SeedInvoiceExtractRuleBackfill` 有"防覆盖铁律"（`extract_config.go:767-847`），但 `mergeInvoiceRuleFields`（L656-720）会把 `rule` 为空或等于**泛用兜底文案**（`invoiceGenericRuleFor` L649-654）的字段按底稿升级。AI 生成的规则若恰好等于兜底文案会被重启覆盖——因此接口 B 生成时应避免产出与 `invoiceGenericRuleFor` 完全相同的字符串（可在返回前比对并追加差异化句式）。

### 4.2 Prompt 设计

#### A. 智能字段生成（接口 A）

```
System:
你是一个票据字段建模助手。给定一份「{cert_type}」票面的原文，判定该票面上
真实存在哪些业务字段，并将其映射到系统标准字段字典。

【标准字段字典】（只允许从中挑选，不得自造 key）
{标准键列表：key | 中文名 | 类型 | 取值样例说明}

【输出要求】
1. 严格输出一个 JSON 对象，不要任何前后缀文字、不要 Markdown 围栏。
2. standard_fields：票面**真实出现**该信息的字段 enable=true 并给出 sample
   （sample 必须是原文中的子串，禁止改写/臆造）；未出现的 enable=false 且 sample=""。
3. item_sub_fields：仅在票面存在"明细/项目行"表格时输出，key 用小写下划线英文
   （name/spec/unit/qty/price/amount/tax_rate/tax_amount 为通用键；额外的用业务语义英文）。
4. custom_fields：票面上有、但字典里没有的信息（如户号、缴费期、抄表读数），
   给出中文名与 sample。若该信息没有独立存储位，store 填 "remark"。
5. 不要输出字典中不存在、票面上也没出现的字段。

【号码口径】invoice_no 只能取明确标注『发票号码/发票号/No.』标签后的数字串；
『机器编号』『校验码』『密码区』一律不算。

User:
<document>
{sample_text}
</document>
```

#### B. 智能配置规则（接口 B）

```
System:
你是一个票据提取规则配置助手。为「{cert_type}」的每个字段编写描述、自然语言
提取规则、是否必填，并在必要时给出 RE2 兜底正则。

【字段清单】{fields: name, data_type}

【desc 要求】一句话说明该字段的业务含义，不超过 20 字。
【rule 要求】
- 中文自然语言，说明从票面哪里取、如何归一（日期统一 YYYY-MM-DD、金额两位小数、不要货币符号）。
- 原文没有时明确写"无则留空"，禁止让模型臆造。
- 不得与本分类其它字段的 rule 完全相同。
【required 要求】仅对"票面必然存在"的核心字段置 true
  （如 invoice_no / invoice_date / total_amount / seller_name）；可选字段一律 false。
【regex 要求】
- 仅对下列字段给出：invoice_no, invoice_code, invoice_date, buyer_name, buyer_tax_no, seller_name, seller_tax_no, issuer。
- 必须是 Go RE2 语法：可用字符类、量词、\b 边界、捕获组；**禁止**环视 (?=)(?!)、反向引用、递归。
- 必须是"形状唯一"的，能在原文中唯一命中（如 \b\d{20}\b），不要 (?s).* 这类宽泛式。
- 不需要或无法保证唯一时，输出空字符串 ""，不要硬凑。

User:
<document>
{sample_text}
</document>
```

### 4.3 前端交互串联设计

#### 步骤 1：字段定义 Tab 增加「智能字段生成」入口

- `FleetCertPanel.vue:132-135` 的 `.meter-form-actions` 区（编辑态底部）新增按钮「智能生成字段」，仅在 `group.scope === 'invoice'` 且已选分类时可用
- 弹窗：`AiFieldSuggestDialog.vue`（新建）—— 分类名（只读，取当前编辑分类）+ 上传样票（`<t-upload>` multipart）+ 「开始识别」
- 调接口 A → 渲染三块可勾选结果：标准字段启用建议（默认勾选 enable=true）/ 明细子列 / 新增自定义字段（可改字段名）
- 用户确认 → 把结果**并入 `fieldsEditable`**（含 `sub_fields`，见下方"必须同时修的链路"）→ 走既有 `commitEdit()` 保存

#### 步骤 2：保存后弹窗询问跳转

`commitEdit`（`FleetCertPanel.vue:643-644`）成功分支后：

```ts
// 伪代码
const go = await DialogPlugin.confirm({
  header: '字段已保存',
  body: '是否立即前往「提取规则」为「水电发票」智能生成提取规则？',
})
// 把 cert_type 与样票文本透传给抽屉
window.dispatchEvent(new CustomEvent('fleet-categories-changed'))
emit('ai-guide', { certType: name, sampleText, autoSuggest: true })   // 新增事件
```

#### 步骤 3：抽屉切 Tab + 自动选中分类

`StandardSettingDrawer.vue` 需要两处小改（当前无 `defineExpose`，且 `activeTab` 是内部 ref）：

```ts
// StandardSettingDrawer.vue
function goRules(payload?: { certType: string; sampleText?: string; autoSuggest?: boolean }) {
  activeTab.value = 'rules'
  nextTick(() => extractRef.value?.focusCertType(payload))   // 见步骤 4
}
defineExpose({ goRules })
```

父组件 `InvoiceManagement.vue:283` 拿 ref，或直接由 `FleetCertPanel` 通过新事件向上冒泡到抽屉（抽屉是 `FleetCertPanel` 的父级，直接监听即可，无需经过列表页）。

#### 步骤 4：`ExtractRulePanel` 支持外部指定分类 + 一键智能生成

`ExtractRulePanel.vue` 新增：

```ts
const props = defineProps<{ scope: string; kbName?: string; kbId?: string
  autoCertType?: string; seedSampleText?: string; autoSuggest?: boolean }>()

// 被抽屉调用：切到指定分类并（可选）立即触发智能生成
function focusCertType(p?: { certType: string; sampleText?: string; autoSuggest?: boolean }) {
  if (!p?.certType) return
  certType.value = typeComposite(props.scope, p.certType)   // 复合值，见 L572-575
  pendingSampleText.value = p.sampleText || ''
  void loadConfig().then(() => { if (p.autoSuggest) void runAiSuggest() })
}
defineExpose({ refresh, focusCertType })
```

工具栏（`ExtractRulePanel.vue:3-11`）在「测试规则」左侧新增「智能生成规则」按钮：

```ts
async function runAiSuggest() {
  const res = await aiSuggestRules(kbId, {
    scope: currentScope.value,
    cert_type: certTypeName.value,
    fields: fields.value.map(f => ({ name: f.name, data_type: f.type })),
    sample_text: pendingSampleText.value || aiCachedText.value,
    overwrite: false,
  })
  // 合并：只回填空值（overwrite=false 语义），保留用户已手改内容
  // regex 仅接受 regex_status === 'accepted'
  // 渲染为「AI 建议预览」可勾选列表 → 用户确认 → 写入 fields → 用户点保存
}
```

**`sample_text` 透传三种来源（优先级）**：
1. 接口 A 返回的 `sample_text` → 存 `sessionStorage['weknora-invoice-sample-text']`（按 `cert_type` 分键，TTL 30min，形态对齐既有的 `weknora-extract-inflight` 做法）
2. 用户未走接口 A 直接点「智能生成规则」 → 可让用户从 KB 已解析文件里选一个（复用 `listKnowledgeFiles`），或留空（Prompt 只按字段语义生成，不保证正则唯一命中 → 此时正则一律 `rejected_not_unique` 丢弃）

#### 必须同时修的链路（否则新功能产出会被后续操作抹掉）

1. **P0：`sub_fields` 需端到端搬运**
   - `FleetCertPanel.vue:563-570`（读）与 `L623-625`（写）都要带上 `sub_fields`
   - `FleetCertPanel.vue:727`（`duplicateItem`）同样
   - 建议：把 subs 的读写收敛成一个 `toSubsPayload()` / `fromSubsPayload()` 纯函数，避免第三处漂移
2. **建议：`sub_fields` 补一个编辑入口**（在字段行 `data_type === 'items'` 时展开子列编辑器），否则「智能字段生成」给的子列用户无法微调
3. **`extractFieldMeta.ts` 正则白名单收窄**：新增 `isRegexPatchable(scope, name)`，UI 用它置灰正则输入/预设（`ExtractRulePanel.vue:31,38,61,68,79`），避免"配了不生效"
4. **沙箱与正式链路不一致**（3.3）：若希望「测试规则」能预演正式提取，需让 `TestExtractConfig` 在 `scope=invoice` 时改走 `ExtractInvoicesFromContentWithRules`；这是独立议题，建议单独立项

---

## 5. 建议落地顺序

| 阶段 | 内容 | 风险 |
|---|---|---|
| P0-1 | 修 `FleetCertPanel` 的 `sub_fields` 读写丢失（含 `duplicateItem`） | 低；纯前端，回归用 `FleetCertPanel.test.ts` 源码断言 |
| P0-2 | 后端 `ParseFileForExtractSuggest`（复用 `temporary_document.parse` + VLM OCR） | 中；需确认解析引擎/VLM 模型来源（建议取 `kb.SummaryModelID`，与 `TestExtractConfig` 同口径） |
| P1-1 | 接口 A + `AiFieldSuggestDialog.vue` | 中；Prompt 需真机调优 |
| P1-2 | 接口 B + 正则三重校验（白名单 / `regexp.Compile` / `uniqueRegexCapture`） | 中；白名单取后端 8 键 |
| P1-3 | 前端串联：`StandardSettingDrawer.goRules` + `ExtractRulePanel.focusCertType` + sample_text 缓存 | 低 |
| P2 | `sub_fields` 编辑入口、`extractFieldMeta` 正则白名单收窄、沙箱链路对齐 | 低 |

---

## 附录：本次摸底涉及的关键文件清单

**前端**
- `src/components/business/StandardSettingDrawer.vue`
- `src/views/dailyAffairs/FleetCertPanel.vue`
- `src/views/dailyAffairs/ExtractRulePanel.vue`
- `src/views/dailyAffairs/ExtractTestDialog.vue` / `TestInputPanel.vue` / `TestResultPanel.vue`
- `src/views/dailyAffairs/InvoiceManagement.vue`
- `src/views/dailyAffairs/extractFieldMeta.ts` / `invoiceFieldLabels.ts`
- `src/api/fleet.ts`

**后端**
- `internal/handler/extract_config.go`
- `internal/handler/fleet.go`
- `internal/handler/knowledge_business.go`
- `internal/application/service/extract_config_prompt.go`
- `internal/application/service/extract_regex_fallback.go`
- `internal/application/service/extract_generic_validate.go`
- `internal/application/service/knowledge_invoice_extract.go`
- `internal/application/service/knowledge_fleet_document.go`
- `internal/application/service/temporary_document.go`
- `internal/types/fleet.go` / `extract_config.go` / `temporary_document.go` / `docparser.go`
- `internal/infrastructure/docparser/engine_registry.go`
- `internal/router/routes_knowledge.go` / `routes_chat.go`
