# 发票「知识库文件 ↔ 列表」数据一致性治理 · 结案报告

- **分支**：`daily-affairs-v4`
- **提交**：`e1e8c3b9c`（已推送）
- **验证环境**：VM `10.10.10.239:5522`，后台 `weknora-backend`（新二进制 `build_time=2026-10-19T04:08:34Z`），真实 Supabase 库
- **数据结论**：治理后 167 / 167 全对齐，0 孤儿，0 待清理，668 行发票 0 误占位

---

## 一、取证：文件在库中的真实字段值与隐身原因

### 1.1 目标文件确实在库（并非数据丢失）

| 字段 | 值 |
| --- | --- |
| `id` | `72a6aae5-a980-4e1c-b77c-d48b810db1d1` |
| `file_name` | `通行费电子票据汇总单(发票).pdf` |
| `file_size` | **528.7 KB**（与用户所见一致） |
| `parse_status` | `completed` |
| `custom_metadata.kind` | `invoice` |
| `custom_metadata.extract_status` | `success` |
| `custom_metadata.fleet_cert_type` | **缺失**（未写入，靠逐票 `category` 兜底） |
| 发票张数 | **13**（page 1..13） |
| 发票号 | `64299470, 64042300, 41706947, 58881543, 62566329, 64226922, 64968633, 51554086, 55250939, 62438064, 61334681, 54750159, 48107833` |
| `invoice_date` | 全部 `2025-10-09` |
| `category` / `invoice_type` | 全部 `通行费发票` |
| 车牌号位置 | `invoices[0].fields.plate_no = "渝C87567"`；`invoices[1].items[0].extra.plate_no = "渝C87567"` |

### 1.2 隐身原因判定（三选一）

| 假设 | 结论 | 证据 |
| --- | --- | --- |
| **A. 删行未删源文件 → `invoices:[]` 孤儿** | **不成立**（当前库 0 条） | `kind=invoice` 且 `jsonb_array_length(invoices)=0` 计数 = **0**；166 份文件全部 OK |
| **B. 提取失败/未提取 → 前端过滤丢弃** | **代码路径存在，本库未触发** | 该文件 `extract_status=success` 且有 13 张票；但 `ListInvoiceRecords` 确有「`invoices` 空 → `continue`」的静默跳过分支 |
| **C. 搜索/日期/分类过滤盲区** | **✅ 成立，且为唯一真因（C①）** | 见下 |

**C① 搜索未穿透（真因）**：`invoiceRecordMatchKeyword` 只检索顶层字段
（`invoice_no / invoice_code / invoice_date / invoice_type / buyer_name / seller_name /
issuer / remark / category / file_name`…），**既不含 `invoices[].fields`，也不含
`items[].extra`**。而 `渝C87567` 恰好只存在于这两处 → 搜 `87567` 命中 0 条。

**C② 日期穿透**：本例 `invoice_date = 2025-10-09`，按 `2025-10` 过滤**本就命中**，
故本例不是日期问题；但「次月汇总开票 + 用户按通行月份筛」的同类场景确实会漏（已一并加固）。

**C③ 分类归属**：`category` 逐票为「通行费发票」，分类筛选同样命中，非本例原因。

### 1.3 「重复上传被拦」的真实机制

- 文件确实在库 → `CheckKnowledgeExists` 按 `file_hash` 命中 → 返回 `409 duplicate_file`。
- 更隐蔽的一点：该分支会 `UpdateKnowledgeColumn(existingKnowledge.ID, "created_at", now())`，
  把老文件的 `created_at` **顶到最新**——实测该文件已排到库内第 **2** 位，用户更容易误判成"刚上传的新文件"。
- 前端收到 409 后仅显示「知识库中已有相同文件」，**没有任何下一步操作**；而列表轮询
  `useBusinessPolling.scanOnce()` 只对 `extract_status` 为空的文件自动投递提取，对
  `success` 的文件永不再提交 → **用户被卡死在"看不见、又传不进、也不能重试"的闭环里**。

---

## 二、四项修复

### ① 搜索穿透（`internal/application/service/knowledge_invoice_list.go`）

- `invoiceRecordMatchKeyword` 新增：`invoices[].fields`（key + value）、
  `items[].Name/Spec/Unit`、`items[].Extra`（key + value）。
- `mapContainsKeyword` 覆盖 `string / number / bool / json.Number / array`
  （防模型把车牌写成数字 `12345678` 后再次隐身）；数字按 `%g` 与 `%.2f` 双格式匹配。

### ② 日期筛选兼容通行日期

- 新增 `invoiceRecordDateInRange`：**顶层 `invoice_date` 命中 OR 明细
  `travel_from/travel_to` 与筛选区间有交集**。
- 同义键覆盖：`travel_from/travelfrom/通行日期起/入口日期/上站日期`，
  `travel_to/travelto/通行日期止/出口日期/下站日期`。
- `dateRangeOverlaps` 做了缺端（单点）与乱序区间归一。

### ③ 铁律一：文件级占位行（消灭隐身文件）

`types.InvoiceRecord` 新增 `IsPlaceholder` / `PlaceholderReason`。
`ListInvoiceRecords` 中以下**五类**文件不再静默跳过，一律渲染占位行：

| 形态 | 占位原因文案 |
| --- | --- |
| `invoices: []` + `extract_status=failed` | `提取失败：<err>` |
| `invoices: []` + 其它状态 | `未提取` |
| `invoices` 非空但每张都是空白提取（LLM 空对象） | 同上（按状态） |
| `kind = not_invoice` | `未识别为发票`（带 err 时拼接） |
| 无 `custom_metadata` / metadata 损坏 | `未提取` / `文件记录异常` |
| `parse_status = failed` | `解析失败` |
| `parse_status = deleting`（删空待清理） | `文件删除中` |

> 保留既有契约：`pending / processing / finalizing`（解析/提取中）仍由列表顶部状态条展示，
> **不**以占位行占用列表。

前端 `InvoiceManagement.vue`：`mapInvoiceRecord` 透传 `is_placeholder` / `placeholder_reason`；
发票号列渲染 `t-tag` 警告徽章；占位行复用已存在的「重新提取 / 删除」入口。

### ④ 铁律二：级联删除可靠化

新增 `service.RemoveInvoicePage(ctx, kbID, knowledgeID, page) (deletedFile, remaining, err)`：

1. **删单张**（仍有剩余）：只更新 `custom_metadata.invoices`，页码前移重排，**绝不动源文件**；
2. **删空**：先用**一次** `UpdateKnowledgeColumns` 同时写
   `invoices=[] + pending_file_delete=true + parse_status=deleting`，再 `repo.DeleteKnowledge`。

这样即使后续异步清理 worker 丢任务，记录也已进入 `deleting`（`CheckKnowledgeExists` 已排除该状态）
→ **不再占哈希锁**，且 `pending_file_delete` 让 `SweepStalePendingFileDeletes` 能兜底识别。

**同时修掉一个会静默废掉整条链路的真 bug**：原 handler 尾部遗留
`SaveInvoiceCustomMetadata(metaBytes)`，把**函数开头解析出的删除前全量 meta** 又写回库，
导致级联和逐张删除全部失效（真机表现为「3 张删 1 张，库里仍是 3 张」）。已删该回写。

### ⑤ 重复上传 → 智能恢复

后端新增 `types.SummarizeDuplicateRecovery(knowledge) *DuplicateRecovery`，
409 响应体追加 `recovery` 字段（`data` 固定为错误里的 knowledge，含 `custom_metadata`）：

| 场景 | `can_reextract` | `normal` | 前端行为 |
| --- | --- | --- | --- |
| 未提取 / 提取失败 / 解析出 0 张 | `true` | `false` | **改分类为本次所选 + 自动 `extract-business`**，任务态 `recovered` |
| 已有正常票据 | `false` | `true` | 提示「已提取 N 张（分类：X，开票日期：Y），可在列表中查看或重新提取」 |
| `deleting` / 解析失败 / `not_invoice` / metadata 损坏 | `false` | `false` | 给明确原因（如「文件正在删除中，请稍后重试」），不再无意义重传 |

前端 `FleetUploadDialog.vue` 同时删除了「翻前 100 条按文件名猜 knowledge id」的脆弱兜底
（发票库已有 167 份文件时必然漏检），改用响应体里的 `recovery` / `data.id`。

### ⑥ 存量巡检（只读，不自动删数据）

| 接口 | 作用 |
| --- | --- |
| `GET /api/v1/knowledge-bases/{id}/invoice-alignment-summary` | 返回 `total_files / invoice_files / placeholder_rows / pending_deletes / not_invoice_files / in_progress_files / orphan_knowledge[]` |
| `GET /api/v1/knowledge-bases/{id}/invoice-pending-delete-sweep?max_age_hours=24` | 返回超时未清理的删空残留 `stale_ids[]` |

两个接口**均只读**。处置（重新提取 / 删除）一律由用户在列表页显式触发——避免误删有效票据。

---

## 三、真机实测结果

### 3.1 搜索与筛选（后端 API，真实库）

| # | 查询 | 修复前 | 修复后 |
| --- | --- | --- | --- |
| ① | `q=87567` | **0 条** | **269 条**（首行 `64299470 / 2025-10-09 / 通行费发票 / plate=渝C87567`） |
| ② | `q=87567` + `2025-10-01..31` + `invoice_type=通行费发票` | **0 条** | **29 条** |
| ③ | 仅 `2025-10-01..31` | — | 32 条（含通行日期穿透） |
| ④ | `q=渝C87567` | **0 条** | 269 条 |
| ⑤ | `q=ZZZ-NO-SUCH-PLATE` | 0 条 | **0 条**（无误命中） |

### 3.2 目标文件在列表中完整可见

遍历全部 668 行，`knowledge_id = 72a6aae5…` 的发票行 = **13 行**，页码 `1..13`，发票号与库内一致。

### 3.3 占位行（铁律一）

- 上传一份无 `custom_metadata` 的探针文件 → 列表返回
  `total=1, is_placeholder=True, placeholder_reason="未识别为发票"`（修复前该文件完全隐身）。
- 健康文件：搜「通行费电子票据汇总单」共 668 行，其中 `is_placeholder=true` 的行数 = **0**（无误占位）。

### 3.4 级联删除（铁律二）

| 步骤 | 结果 |
| --- | --- |
| 造 3 张票的探针文件，删 page 3 | 剩余 **2** 张，页码重排为 `1,2`，**文件保留** |
| 再删 page 2 | 剩余 1 张 |
| 再删 page 1（最后一张） | 响应 `{"deleted_file":true,"message":"Invoice removed, source file deleted"}`；`deleted_at IS NULL` 残留 = **0**，`invoices:[]` 空壳 = **0** |
| 单票文件删最后一张 | 同样 `deleted_file:true`，残留 0 |

### 3.5 重复上传（智能恢复）

对已存在文件二次上传：

```
HTTP=409
code    = duplicate_file
recovery= {"can_reextract": true, "normal": false, "invoice_count": 0}
data.id = <已存在文件的 knowledge_id>
```

→ 前端据此自动改分类并重新提取，不再把用户挡在错误提示上。

### 3.6 存量巡检

```
total_files 167 | invoice_files 167 | placeholder_rows 0
pending_deletes 0 | not_invoice_files 0 | in_progress_files 0
orphan_knowledge []
```

---

## 四、门禁

| 项 | 结果 |
| --- | --- |
| `go vet ./internal/...` | **0** |
| 后端 CI 白名单（`TestBusiness\|TestExtract\|TestDailyAffairs\|TestGolden`） | **TEST=0**，新增 18 个 `TestBusiness*` 全绿 |
| `go build ./internal/...` | **0** |
| 前端 dailyAffairs 全量 | 219 pass / **6 存量红**（`git stash` 复现一致，零回归） |
| `vue-tsc --build --force` | **0 error** |
| `eslint`（改动文件） | **0 error** |
| VM 生产构建 `vite build` | **BUILD_EXIT=0** |

**6 项存量红说明**（非本轮引入）：`extractInFlight` 2 项、轮播 1 项、`FleetUploadDialog` 3 项
（断言静态 `placeholder="请选择证照类型"`，但实现早已改为动态 `:placeholder="typeFieldLabel"`）。

---

## 五、附带发现的环境问题

1. **VM 装了两套 Node**：`/usr/local/node22/bin`（v22.23.2，必需）与 `/usr/bin/node`（18.20.4）。
   自定义 PATH 时若把 `/usr/bin` 放前面，`vite build` 直接失败 `crypto.hash is not a function`。
2. **VM 前端 `src` 与本地漂移**：曾有旧版 `extractFieldMeta.test.ts` 引用已删除的
   `REGEX_PATCHABLE_KEYS` → `vue-tsc` 假红。已整包同步 `src` 修复。
3. **鉴权需查库**：`ValidateToken` 除验签还要查 `auth_tokens` 表（token 为明文列）；
   自签 JWT 会报 `look up token: token not found`。取证请直接取现成 access_token。
4. **测试数据已清零**：所有探针文件（`cascade-probe-*`、`dupcheck_*`）已硬删，
   发票库恢复为 **167** 个文件，与治理前一致。

---

## 六、遗留建议（未在本轮实施）

1. **409 分支不应改写 `created_at`**：`knowledge_create.go:117` 把老文件顶到最新，
   会持续误导用户「这是刚上传的文件」。建议改为只更新 `updated_at` 或记录 `last_upload_attempt_at`。
2. **`fleet_cert_type` 未写入**：该文件 13 张票靠逐票 `category` 兜底才归类正确；
   建议上传后提取成功时回填文件级 `fleet_cert_type`，让分类筛选更稳。
3. **前端占位行视觉可再细化**：当前用「发票号码」列的警告徽章承载，若占位行较多，
   建议增加独立的「文件状态」列或分组（本轮未做，避免改动表格列宽契约）。
