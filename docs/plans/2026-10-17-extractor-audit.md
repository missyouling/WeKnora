# 结构化提取器（Extractor）现状摸底报告

- **盘点日期**：2026-10-17
- **代码版本**：`daily-affairs-v4`（HEAD `60af53832`）
- **盘点范围**：仅二开提取层（Parser/OCR 由 WeKnora 原项目负责，不在本报告范围）
- **方法**：只读扫描，逐条核对真实代码（未修改任何逻辑）
- **用途**：为「跨模型兼容 / 多层降级 / 前端可视化规则配置」升级方案提供事实基线

---

## 0. 代码地图

| 文件 | 行数 | 职责 |
|---|---:|---|
| `internal/application/service/extract_executor.go` | 132 | 通用执行器（多次尝试 + 超时 + 诊断） |
| `internal/application/service/extract_validate.go` | 262 | 确定性校验 + 一次自愈（仅发票） |
| `internal/application/service/knowledge_invoice_extract.go` | 2021 | 发票提取（含三套前置规则） |
| `internal/application/service/knowledge_contract_extract.go` | 403 | 合同提取 |
| `internal/application/service/knowledge_regulation_extract.go` | 308 | 制度提取 |
| `internal/application/service/knowledge_award_punish_extract.go` | 314 | 奖惩提取 |
| `internal/application/service/knowledge_solar_extract.go` | — | 光伏提取（自带旧重试） |
| `internal/application/service/knowledge_utility_extract.go` | — | 水电提取 |
| `internal/application/service/extract_config_prompt.go` | — | Schema/Prompt 组装 |
| `internal/handler/knowledge_business.go` | 2845 | 各业务提取编排（分批、并发、落库） |
| `internal/handler/extract_config.go` | 873 | 规则读取、子列注入 |
| `internal/types/extract_config.go` | 49 | 规则数据结构 |
| `internal/common/tools.go` | — | `ParseLLMJsonResponse`（JSON 清洗） |
| `frontend/src/views/dailyAffairs/ExtractRulePanel.vue` | — | 提取规则设置面板 |
| `frontend/src/views/dailyAffairs/ExtractTestDialog.test.ts` | — | 测试提取对话框（含单测） |
| `frontend/src/views/dailyAffairs/InvoiceManagement.vue` | ~2600 | 发票主界面 |

---

## 1. 解析（Parse）与提取（Extract）的交接边界

### 1.1 输入从哪来

**答案：直接读 `chunks` 表文本，按 chunk 拼接。不是 Markdown 全文，也没有页码结构。**

`BuildInvoiceExtractionBatches`（`knowledge_invoice_extract.go:858`）：

```go
usable := make([]*types.Chunk, 0, len(chunks))
for _, chunk := range chunks {
    if chunk.ChunkType != types.ChunkTypeText &&
        chunk.ChunkType != types.ChunkTypeImageOCR &&
        chunk.ChunkType != types.ChunkTypeImageCaption {
        continue
    }
    if strings.TrimSpace(chunk.Content) != "" {
        usable = append(usable, chunk)
    }
}
```

**拼装**：`header + chunk.Content`，用 `\n\n` 连接：

```go
header = append(header, "Document name: "+name)
header = append(header, "Existing summary: "+summary)
headStr := strings.Join(header, "\n\n")
```

### 1.2 是否保留物理页码

**没有。** 逐项核实：

| 检查项 | 结果 |
|---|---|
| 文本中 `Page N` / `第 N 页` 标记 | ❌ 无 |
| 文本中 `[chunk N]` / `[片段 N]` 标记 | ❌ 无 |
| 分页符 / `<!-- page -->` 注释 | ❌ 无 |
| `chunk.ChunkIndex` 是否拼进正文 | ❌ 否（只用于 `StartIndex/EndIndex` 记录区间） |
| OCR 引擎输出是否带页标记 | ❌ 无（`img src="resource://..."` 是资源 key；`order=` 是元素顺序；`page-N` 命中数为 0）|

### 1.3 分批策略

已有分批，**双约束**：

```go
const invoiceExtractMaxContentRunes = 24000   // knowledge_invoice_extract.go:21
const InvoiceExtractionBatchSize = 8          // 每批 chunk 数
```

flush 条件：`len(curParts) >= batchSize || curRunes + r > invoiceExtractMaxContentRunes`

**并发**：`invoiceBatchConcurrency = 4`（`knowledge_business.go`），预分配切片按批次索引写入，合并阶段回主 goroutine 顺序归位。

### 1.4 页码归位的真实语义（重要）

```go
inv.Page = batches[i].StartIndex + 1 + j
```

`Page` 是**票序号**，不是 PDF 页号。

**实证**（2026-10-17，16 页通行费汇总单）：
- 提取出 16 张发票，`page` = 1..16
- 但该 PDF 实际只有 **12 页**（16 张票排 12 页，部分页含 2 张）
- 结果：前端预览 `#page=15` 超出页数 → 浏览器停在末页 → 用户看到"第 15 张显示第 12 页"

**chunk 粒度随引擎漂移，无法反推页**：

| 引擎 | chunk 数 | 结构 |
|---|---:|---|
| docreader | 32（有效） | 按页切，约 2 chunk/页 |
| paddleocr_vl_cloud | 64 | 按票切，**4 chunk/票**（图头/号码/明细/尾） |

---

## 2. 通用执行器与多模型兼容性

### 2.1 调用阶梯

`DefaultExtractAttempts`（`extract_executor.go:33`）：

```go
return []ExtractAttempt{
    {Label: "stream-free", Stream: true},
    {Label: "nonstream-free"},
    {Label: "nonstream-jsonobject", Schema: schema},
    {Label: "stream-free-2", Stream: true},
}
```

| # | Label | Stream | Format(Schema) | 说明 |
|---|---|---|---|---|
| 1 | `stream-free` | ✅ | 无 | 依赖 prompt 约束 |
| 2 | `nonstream-free` | ❌ | 无 | 换传输方式 |
| 3 | `nonstream-jsonobject` | ❌ | ✅ | 写 `ChatOptions.Format` |
| 4 | `stream-free-2` | ✅ | 无 | 流式再试 |

**每次尝试的参数差异**：

```go
opts := *baseOpts          // 浅拷贝
if len(a.Schema) > 0 {
    opts.Format = a.Schema // 只有第 3 步覆盖这一个字段
}
```

其余参数（`Temperature=0.1`、`MaxTokens=0`、`Thinking=&false`）**四轮完全相同**，由调用方固定传入。

**其他机制**：
- 每次调用 `withExtractCallTimeout(ctx)` 硬超时
- 空 content 单独告警（提示 MaxTokens 可能误设）
- `for` 内 `parse(raw)` 成功即 return，不浪费后续 token
- 失败时保留**最长**的 raw（不是最后一次），供 `onFailure` 诊断

**专用变体**：
- `SingleAttemptExtractAttempts()`：只 1 次，供分批回退路径避免 25 分钟最坏耗时
- `RepairExtractAttempts(schema)`：2 次（非流式 → 非流式+schema），供自愈

### 2.2 响应清洗现状

入口：`parseInvoiceRuleReply`（`knowledge_invoice_extract.go:531`）

```go
normalized := normalizeRuleFieldKeys(raw, cfg)   // 中文 key → 英文
var parsed InvoiceExtractionResult
if err := common.ParseLLMJsonResponse(normalized, &parsed); err != nil { ... }
```

`ParseLLMJsonResponse`（`internal/common/tools.go:103`）**四层兜底**：

| 层 | 处理 | 代码 |
|---|---|---|
| 1 | 直接 `json.Unmarshal` | :105 |
| 2 | Markdown 代码块围栏 | `jsonCodeFenceRE`，:111 |
| 3 | 前后废话剥离（平衡括号扫描） | `ExtractBalancedJSON`，:124 |
| 4 | 截断修复（未闭合围栏 + 丢尾补括号） | `stripUnclosedFencePrefix` + `repairTruncatedJSON`，:135-148 |

**发票额外一层**：`normalizeRuleFieldKeys` 用 `invoiceFieldAliases`（60+ 中文别名，按"最长包含命中"匹配，如「价税合计金额」→ `total_amount` 而非 `amount`）。

### 2.3 兼容缺口（逐项核实）

| 能力 | 现状 | 触发后果 |
|---|---|---|
| `<think>...</think>` 剥离 | ❌ **无** | 思考模型的思维链进正文，第 3 步可能把 think 内 JSON 片段当 payload |
| 千分位 `"3,258.99"` | ❌ **无** | `Unmarshal` 到 `*float64` 失败 → 字段静默 null |
| 末尾多余逗号 `,]` | ❌ **无** | `repairTruncatedJSON` 只补括号，不删尾逗号 |
| 不支持 `response_format` | ⚠️ 第 3 步失败 | **不中断**：继续第 4 步；只是少一次强约束 |
| 强制思考模型 | ⚠️ `Thinking: &false` 硬编码 | 对 deepseek-reasoner 类无效，显著变慢 |
| 流式不支持 | ✅ 有 `nonstream-free` 兜底 | — |

---

## 3. 校验、自愈与降级兜底

### 3.1 校验范围

`validateInvoiceExtraction`（`extract_validate.go:94`），**仅发票**：

```go
if res.Kind != "invoice" {
    return issues      // :99 其他业务直接跳过
}
```

| # | 规则 | 判定 | severity |
|---|---|---|---|
| 1 | 日期可归一化 | 5 种格式（`2026-09-02` / `2026/9/2` / `2026.9.2` / `2026年9月2日` / `20260902`）| 失败 → `warn` |
| 2 | 标识存在 | 发票号 **或** 发票代码至少一个 | 都空 → **`error`** |
| 3 | 金额存在 | 价税合计 **或** 不含税金额至少一个 | 都空 → **`error`** |
| 4 | 明细合计一致性 | `Σ items[].amount ≈ inv.amount` | 不等 → `warn` |
| 5 | 金额勾稽 | `amount + tax ≈ total_amount` | 不等 → `warn` |

**容差**：`floatEquals`（:81）—— 绝对 0.02 **或** 相对 1%

**副作用**：校验会**就地归一化日期**（`inv.InvoiceDate = norm`）

### 3.2 自愈（Repair）

**触发**：`countIssues(issues, issueSeverityError) > 0`（`knowledge_invoice_extract.go:684`）

**Prompt 组装**（`repairInvoiceExtraction`，:193）：

```go
sb.WriteString("上一次提取结果存在以下问题，请修正后重新输出完整 JSON（结构与字段键名保持不变）：\n")
for _, it := range issues {
    if it.Severity != issueSeverityError { continue }   // 只列 error，warn 不修
    sb.WriteString(fmt.Sprintf("- 第 %d 张票据的字段 %s：%s（%s）\n", it.Index+1, it.Field, it.Detail, it.Kind))
}
sb.WriteString("\n只输出 JSON，不要解释。")
```

**消息序列**：

```go
messages = append(messages, baseMessages...)                       // system + user(原文)
messages = append(messages, chat.Message{Role: "assistant", Content: prev})  // 上次输出(截断 8000 rune)
messages = append(messages, chat.Message{Role: "user", Content: sb.String()}) // 问题清单
```

**采用条件**（:691）：

```go
case countIssues(fixedIssues, error) < countIssues(issues, error):   // 严格减少
    parsed, issues = fixed, fixedIssues
```

> 设计取舍已在注释写明：绝不因为"再问了一次"就丢掉原本可用的数据——空结果比有瑕疵的结果更糟。

### 3.3 降级兜底（关键结论）

| 兜底类型 | 现状 | 证据 |
|---|---|---|
| **正则兜底** | ⚠️ **仅发票有，且是"前置优先"不是"后置兜底"** | 三套规则都在 LLM 之前：`ExtractInvoicesByLineRulesFromChunks` → `ExtractInvoicesByMarkdownTableRulesFromChunks` → `ExtractInvoicesByFlatLayoutFromChunks`，命中即 return，不调模型 |
| 模型 4 次全挂 | 分两种 | 首批失败且无产出 → 整体 failed（可重试）；后续批失败 → 跳过，保留已得 |
| 关键字段始终为空 | ❌ **不阻断** | error 级只触发一次自愈；自愈后仍缺照样入库，`ExtractStatus` 仍为 `success`，问题仅写日志 |
| 半成品入库 | ✅ **允许** | `merged.ExtractError` 写"部分批次未完成（N/M 批失败），已保留其余批次结果，建议复核" |

**实证**：2026-10-17 一号票号码全空却显示"提取成功"——因校验只要求"号码或代码有一个"，且 error 不阻断落库。

---

## 4. 各业务模块接入进度

### 4.1 总表（核心结论：只有发票接了）

| 模块 | 文件 | 行数 | 通用执行器 | 校验器 | 独有逻辑 |
|---|---|---:|---|---|---|
| **invoice** 发票 | `knowledge_invoice_extract.go` | **2021** | ✅ | ✅ | 三套前置规则；分批 + 并发 4；购销方单元格解析；明细子列注入；去重；查验 |
| **contract** 合同 | `knowledge_contract_extract.go` | 403 | ❌ | ❌ | `ExtractContractsFromContentWithRules`；自动编号 `MaxAutoContractSeq` |
| **regulation** 制度 | `knowledge_regulation_extract.go` | 308 | ❌ | ❌ | `WithRules` 变体 |
| **award_punish** 奖惩 | `knowledge_award_punish_extract.go` | 314 | ❌ | ❌ | 多一个入参 `measures []string`（奖惩措施清单作为约束） |
| **solar** 光伏 | `knowledge_solar_extract.go` | — | ❌（**自带重试**） | ❌ | :134 `attempts := []struct{...}` + :161 `for i, a := range attempts` |
| **utility** 水电 | `knowledge_utility_extract.go` | — | ❌ | ❌ | 无 |
| **notice** 通知公告 | 内联于 `knowledge_business.go` | — | ❌ | ❌ | 无独立 extract 文件 |

`grep -rln "RunExtractWithRetry"` 结果只有：`extract_executor.go`、`extract_validate.go`、`knowledge_invoice_extract.go`。

### 4.2 关键旁证

`extract_executor.go:16` 注释自述：

> 这套模式最早在 solar（`knowledge_solar_extract.go`）里跑通——4 次尝试……它是**六个业务里唯一有重试的实现**。发票体积最大（1472 行）却只有单次调用。

**现状是"发票迁走了，solar 还留在原地"** —— 通用执行器抽出来之后，solar 自己的 attempt 循环**尚未迁移**，存在双套实现。

### 4.3 各模块入口函数

```
contract:      ExtractContractsFromContent / ExtractContractPageFromContent / ExtractContractsFromContentWithRules
regulation:    ExtractRegulationsFromContent / ExtractRegulationsFromContentWithRules
award_punish:  ExtractAwardPunishesFromContent / ExtractAwardPunishesFromContentWithRules
solar:         ExtractSolarBillsFromContent
utility:       ExtractUtilityBillsFromContent
```

---

## 5. 提取规则数据结构与 Prompt 组装

### 5.1 完整 Struct（`internal/types/extract_config.go` 全文 49 行）

```go
// ExtractFieldConfig 可配置提取字段：名称（输出 JSON key）、描述、数据类型、提取规则。
type ExtractFieldConfig struct {
    Name    string `json:"name"`    // 输出 JSON 的字段 key，如"经营地址"
    Label   string `json:"label"`   // 界面显示名（可为空，缺省与 name 相同）
    Desc    string `json:"desc"`    // 字段描述，如"公司实际经营地址"
    Type    string `json:"type"`    // string | array | date | number
    Rule    string `json:"rule"`    // 提取规则/特殊说明
    Enabled bool   `json:"enabled"` // 是否参与提取
    // SubFields 明细行子列口径（仅 Type=items 时使用）：提取时由 handler 从分类
    // categories.subs 的 items 字段实时注入（单一权威源），用于生成明细行 JSON Schema。
    SubFields []FleetItemSubField `json:"sub_fields,omitempty"`
}

// KbExtractConfig 用户可配置的字段提取规则（知识库 × scope × 证照类型维度）。
type KbExtractConfig struct {
    ID              string               `gorm:"primaryKey" json:"id"`
    TenantID        int64                `gorm:"index" json:"tenant_id"`
    KnowledgeBaseID string               `gorm:"index" json:"knowledge_base_id"`
    Scope           string               `json:"scope"`     // vehicle | driver | maintain | invoice | contract...
    CertType        string               `json:"cert_type"`
    Fields          []ExtractFieldConfig `gorm:"type:jsonb;serializer:json" json:"fields"`
    AdvancedEnabled bool                 `json:"advanced_enabled"`
    PromptTemplate  string               `gorm:"type:text" json:"prompt_template"`
    Version         int                  `json:"version"`
    Enabled         bool                 `json:"enabled"`
    CreatedAt       time.Time            `json:"created_at"`
    UpdatedAt       time.Time            `json:"updated_at"`
    DeletedAt       *time.Time           `gorm:"index" json:"deleted_at"`
}

func (KbExtractConfig) TableName() string { return "knowledge_extract_configs" }
```

### 5.2 存储与读取

- 表：`knowledge_extract_configs`
- `fields` 列：JSONB（`gorm:"type:jsonb;serializer:json"`）
- 唯一维度：`knowledge_base_id + scope + cert_type`
- 读取（`extract_config.go:23`）：

```go
Where("knowledge_base_id = ? AND scope = ? AND enabled = TRUE AND deleted_at IS NULL")
if ct != "" { q = q.Where("cert_type = ?", ct) } else { q = q.Where("cert_type = ? OR cert_type = ?", "", "__all__") }
q.Order("updated_at DESC").First(&ec)
```

**回退链**（`resolveInvoiceRuleCfg`，:43）：`certType` 规则 → 「普通发票」规则 → 整体规则（`''` / `__all__`）

### 5.3 Prompt / Schema 组装

**通用入口** `BuildExtractSystemPrompt`（`extract_config_prompt.go:72`）：

```go
kind := scopeKey + "_document"          // 注释明确"不得内含任何具体业务概念"
schema := BuildFieldsSchema(cfg.Fields)
```

**Schema 生成** `BuildFieldsSchema`（:24）—— **手写 JSON 字符串**，非 `encoding/json`：

```go
sb.WriteString("{\n  \"type\": \"object\",\n  \"properties\": {\n")
for _, f := range fields {
    if !f.Enabled || strings.TrimSpace(f.Name) == "" { continue }
    sb.WriteString("    \"" + strings.TrimSpace(f.Name) + "\": {")
    ...
}
```

**发票专用** `buildInvoiceRuleSystemPrompt`（`knowledge_invoice_extract.go:540`）：

```go
if !hasTemplate && !hasFields { return invoiceExtractionSystemPrompt }  // 内置兜底

if hasTemplate {
    // 支持 {{fields_schema}} 占位
    r := strings.NewReplacer("{{fields_schema}}", BuildFieldsSchema(cfg.Fields))
    sb.WriteString(strings.TrimSpace(r.Replace(cfg.PromptTemplate)))
}
if hasFields {
    sb.WriteString("提取要点（用户配置的字段口径...）:\n")
    for _, f := range cfg.Fields {
        if n == "" || !f.Enabled { continue }        // ← 禁用字段被跳过
        parts = append(parts, "描述："+f.Desc)
        parts = append(parts, "规则："+f.Rule)
        sb.WriteString("· " + n + "（" + strings.Join(parts, "；") + "）\n")
    }
}
// + 分离要求 + 明细子列指令 + 号码口径 + invoiceExtractionSystemPrompt
```

### 5.4 字段级规则 / 正则：**有壳，无执行**

| 能力 | 状态 | 说明 |
|---|---|---|
| `Desc` 字段描述 | ✅ 进 prompt | 纯文字 |
| `Rule` 字段规则 | ✅ 进 prompt | **纯文字提示，不是正则** |
| `PromptTemplate` | ✅ 支持占位符 | `{{fields_schema}}` |
| `AdvancedEnabled` | ✅ 存储 | 控制高级模式 |
| **正则表达式配置** | ❌ **数据结构中完全没有** | 要做"正则兜底"必须新增字段（如 `Regex` / `RegexGroup` / `RegexFlags`） |

---

## 6. 前端「提取设置」与「测试提取」

### 6.1 组件清单

| 组件 | 路径 | 职责 |
|---|---|---|
| `ExtractRulePanel.vue` | `views/dailyAffairs/` | 提取规则设置面板（字段/模板/测试入口） |
| `ExtractTestDialog` | 被 `ExtractRulePanel` 引用 | 测试提取对话框（有单测 `ExtractTestDialog.test.ts`） |
| `InvoiceManagement.vue` | `views/dailyAffairs/` | 发票主界面（列表/详情抽屉/字段配置） |

### 6.2 用户可配置项（两个权威源）

| 来源 | 配置项 |
|---|---|
| `knowledge_extract_configs.fields` | 字段名 `Name`、显示名 `Label`、描述 `Desc`、类型 `Type`、提取规则 `Rule`（文字）、启用 `Enabled` |
| `knowledge_extract_configs` | `advanced_enabled`（高级模式）、`prompt_template`（含 `{{fields_schema}}`） |
| `fleet_categories.subs` | 分类字段（`name`/`width`/`enabled`/`is_default`/`data_type`）+ **明细子列 `sub_fields`**（`key`/`label`/`data_type`） |

> **双源失配是已知事故点**（2026-10-17）：`KbExtractConfig.fields` 为空数组 + `fleet_categories.subs[items].enabled=false` → prompt 跳过明细子列 → 车牌号/车辆类型/通行日期全部丢失。

### 6.3 校验结果的前端可见性：**完全不可见**

`ExtractValidationIssues` 注释（`extract_validate.go:258`）：

> 当前仅由服务内部使用；导出是为了后续接可观测面板时不必再改签名

**前端能看到**：`extract_status`（success/processing/failed）、`extract_error`（批次级提示）
**前端看不到**：warn/error 级字段问题、自愈是否触发、自愈前后的差异

---

## 7. 缺口清单（按对升级方案的影响排序）

| # | 缺口 | 位置 | 影响 |
|---|---|---|---|
| 1 | 仅 invoice 接入通用执行器，5 个模块裸调 | 各 `Extract*FromContent` | 换模型时只有发票能自愈 |
| 2 | 无 `<think>` 标签剥离 | `common.ParseLLMJsonResponse` | 思考模型基本不可用 |
| 3 | 无千分位 / 尾逗号清洗 | 同上 | 金额字段静默变 null |
| 4 | 输入无页码，`Page` 是票序号 | `BuildInvoiceExtractionBatches` | 预览跳页错位；页码归位语义错误 |
| 5 | 字段级规则只有文字，无正则 | `ExtractFieldConfig` | 无法做确定性兜底 |
| 6 | 校验结果不进前端 | `ExtractValidationIssues` 未接 API | 用户看不到质量问题 |
| 7 | 校验逻辑仅发票 | `validateInvoiceExtraction:99` | 其他模块无质量闭环 |
| 8 | solar 自带旧重试未迁移 | `knowledge_solar_extract.go:134` | 双套实现，维护分叉 |
| 9 | 明细子列双权威源 | `KbExtractConfig.fields` vs `fleet_categories.subs` | 易失配，已造成事故 |

---

## 附录：关键常量速查

| 常量 | 值 | 位置 |
|---|---|---|
| `InvoiceExtractionBatchSize` | 12 → 8（当前 8） | `knowledge_invoice_extract.go:841` |
| `invoiceExtractMaxContentRunes` | 24000 | `:21` |
| `invoiceBatchConcurrency` | 4 | `knowledge_business.go:74` |
| `invoiceBatchBudget` | 8 分钟 | `knowledge_business.go` |
| `extractionTimeout` | 30 分钟 | `knowledge_business.go:59` |
| 默认 Temperature | 0.1 | 各 extract 文件 |
| 默认 MaxTokens | 0（不限制） | 同上 |
| 思考开关 | `Thinking: &false` 硬编码 | 同上 |

---

# Phase C 执行记录（2026-10-17）：全量收编执行器 + 通用校验扩面

## 目标（对应缺口清单 #1 / #7 / #8）

1. 剩余业务模块全部接入 `extract_executor.go`，消除"每个业务一份重试循环"的分叉；
2. 校验与补洞能力从 `kind == "invoice"` 扩到全业务，但发票专属业务规则不外泄；
3. 发票前置规则路径（Line / MarkdownTable / FlatLayout）补齐质量元数据。

## 决策

| 决策 | 理由 |
|---|---|
| 所有模块 `MaxTokens: 0` | 显式传 max_tokens 是部分供应商长输出返回空 content 的直接诱因 |
| 车队保留"json_object 优先"的策略顺序 | 该顺序是为"模型输出非 JSON / 自造字段名"专门优化的，收编只换执行引擎、不换策略 |
| 单份重提取（contract page）用 `SingleAttemptExtractAttempts` | 用户主动定点修复，4 次尝试会把成本翻四倍；仍获得清洗层 + 硬超时 |
| 通用校验用反射按 json tag 读写强类型结构体 | 各业务结果是强类型 struct（可读性/编译期校验），不可能退化成 map |
| 正则补洞只在**单条记录**时启用 | 多条记录同时缺同一字段时无法判定匹配值归属，按序猜测会串号（与 Phase B 同口径） |
| 内置日期字段归一失败**静默**，用户声明 `Type=date` 才告警 | 内置字段常装区间值（"2026-08-01至2026-08-31"）或自然语言，告警纯噪音 |
| 发票规则路径的收尾放在 `OverlayDeterministicInvoiceFields` **之后** | overlay 会纠正号码/明细，先校验会报"已被修正"的假问题 |

## 代码落点

- 收编：`knowledge_solar_extract.go`、`knowledge_utility_extract.go`、`knowledge_contract_extract.go`（标准版 + WithRules + page）、`knowledge_regulation_extract.go`（标准版 + WithRules）、`knowledge_award_punish_extract.go`（标准版 + WithRules，保留 measures 参数）、`knowledge_fleet_document.go`。
- 通用闭环：新增 `extract_generic_validate.go`（`NewGenericRecords` / `ValidateGenericExtraction` / `NormalizeDateValue` / `PatchedFieldNames`）。
- 发票规则路径收尾：`extract_validate.go` 新增 `FinalizeInvoiceRuleResult`，`extractInvoicesWithRulesAttempts` 改为复用它（LLM 路径与规则路径共用同一套收尾）。
- 元数据：`contractCustomMetadata` / `regulationCustomMetadata` / `awardPunishCustomMetadata` / `solarBillCustomMetadata` / `utilityBillCustomMetadata` 均新增 `extract_issues` / `patched_by_regex` / `regex_patches`（与发票同键名）。
- handler 接入点：`knowledge_business.go`（contract / regulation / notice）、`knowledge_solar_bill.go`、`knowledge_utility_bill.go`——均在 `Normalize*` 之后、构造 meta 之前调用 `ValidateGenericExtraction`。

## 新增测试（19 个，全部进 CI 白名单前缀 `TestExtract*`）

- `extract_module_executor_test.go`：contract / regulation / award / solar / utility / fleet 各自"首次空 → 换策略成功"；utility 的 `<think>` + 千分位 + `¥` 金额；utility 全部失败必须报错；断言每次调用 `MaxTokens == 0`。
- `extract_generic_validate_test.go`：标准日期归一化、区间值不被截断、用户声明 date 的 format 告警、Required + Regex 补洞、补不到记 error、结构体不存在的字段不误报、多记录禁止补洞、不覆盖已有值、补洞后日期仍归一、发票规则路径的日期归一化与补洞、`NormalizeDateValue` 表驱动。

## 门禁结果

| 门禁 | 结果 |
|---|---|
| `go vet` handler + service | 通过 |
| CI 白名单测试（`TestBusiness|TestExtract|TestDailyAffairs|TestGolden`） | 通过 |
| 全量基线对照（service + handler） | 基线 47 失败 → 本版 47 失败，**新增 0 / 修复 0** |
| 前端 `npm run type-check` | 通过 |
| 前端 `npm run lint` | 0 error / 130 warning（存量） |
| VM 冒烟（16 张通行费发票，规则路径） | 16 张、`success`，`rule path finalized, issues=0`，日期全部 `2025-11-04` |
| VM 冒烟（合同重提取，LLM 路径） | `success`，1 份合同，`sign_date=2025-05-13`，issues=0 |

## 已知边界（本轮未解）

1. 通用 Required/Regex 只对**结构体真实存在**的 json 字段生效；用户自造的中文 key 仍只进 Prompt，不参与校验（避免误报）。
2. 明细子列 `items[].extra`、账单 `fee_items` 等数组子项不做字段级校验（与 Phase B 边界一致）。
3. 光伏/电费账单没有字段级规则配置（无 scope），故只享受日期归一化，不含 Required/Regex。
4. 全量基线仍有 47 个与提取器无关的红用例（SQLite 迁移 / repository / 模板 / 摘要严格性 / tenant secret）。
5. 页码语义问题未根治（需 docreader 透传 page index）。
