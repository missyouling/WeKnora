# MinerU 引擎切换三故障根治 · 结案报告

- **分支**：`daily-affairs-v4`
- **提交**：`ab21dce41`（已推送）
- **环境**：VM `10.10.10.239:5522`，`weknora-backend` 已换新二进制
- **取证对象**：`通行费电子票据汇总单(发票).pdf` = `ad740ba3-7e8b-4c4e-b479-075d96fa141e`，27 张票，89 chunks

---

## 一、故障 2 取证：MinerU chunk 原文与缺项根因

### 1.1 真实 chunk 结构（脱敏后已写入单测）

**A. 明细行「标签 + 数值」合并进同一个 `<td>`**

```html
<tr>
  <td rowspan="2" colspan="2">项目名称*经营租赁*通行费合 计</td>
  <td rowspan="2">车牌号渝C12345</td>
  <td rowspan="2">类型货车</td>
  <td rowspan="2">通行日期起20250505</td>
  <td rowspan="2" colspan="2">通行日期止20250528</td>
  <td>金额234.25</td>
  <td rowspan="2">税率3%</td>
  <td>税额7.03</td><td></td>
</tr>
```

**B. 购销方单元格：标签带空格 + 四字段无换行首尾相连**

```html
<td>购 买 方</td>
<td colspan="5">名 称:重庆示例铜业有限公司纳税人识别号:91500000MA5U54AJ0A
地址、电话:重庆市示例区示例路106号023-41557211开户行及账号:中国建设银行示例支行50050118370000000047</td>
```

**C. 同一文件内混排两种渲染**：新 `<table>` 与旧 `<table border=1>`，后者单元格内是**字面量** `\n`（两个字符）。

### 1.2 为什么金额/税率/税额全空

`htmlTableDataRow` 的数据行判定是**位置式**的，要求：

```go
if plateLike(vals[1]) && isCompact8Date(vals[3]) && isCompact8Date(vals[4])
```

实测：

| 单元格 | 实际值 | 判定 |
| --- | --- | --- |
| `vals[1]` | `车牌号渝C12345` | `plateLike` 10 runes > 上限 9 → **false** |
| `vals[3]` | `通行日期起20250505` | `isCompact8Date` 13 字符 → **false** |
| `vals[4]` | `通行日期止20250528` | 同上 → **false** |

→ 明细行彻底失配，`inv.Items` 为空 → 金额/税率/税额全丢。
唯一活下来的是 `flatHTMLTotalRe = 小写\D{0,4}([\d,]+(?:\.\d+)?)`，它只认「(小写) ¥241.28」→ **价税合计 27/27 有值，其余全 0**，与库中数据完全一致。

### 1.3 为什么「隔行」丢购销方

`flatPartyCellRe` 原为 `(?is)<td>\s*(购买方|销售方)\s*</td>`，只匹配**连续**中文。
MinerU 对同一份文件输出了两种形态：

| 形态 | 张数 | 结果 |
| --- | --- | --- |
| `<td>购买方</td>` / `<td>销售方</td>` | **12** | 命中 → 有购销方 |
| `<td>购 买 方</td>` / `<td>销 售 方</td>` | **15** | **不命中** → 全空 |

库中实测：有购销方 12 张、空 15 张 —— **与两种形态张数精确互补**，这是「隔行丢失」的真因（不是 chunk 切分边界）。

### 1.4 为什么 buyer_name 变成整块拼接文本

`flatNameLikeRe` 原为 `名\s*称\s*[:：]\s*([^\n\\]+)`。该单元格**没有任何换行**，
`[^\n\\]+` 一直吃到单元格末尾，把税号/地址/开户行全部吞进名称。
（税号本身靠 `flo\tTaxNoLikeRe` 独立命中，所以 `buyer_tax_no` 反而还在。）

### 1.5 另外两个隐藏缺陷

1. `parseFlatInvoiceSegment` 回填发票级字段时只做了：
   ```go
   if line.Amount != nil { inv.Amount = line.Amount }
   if line.TaxAmt != nil { inv.Tax = line.TaxAmt }
   ```
   **漏了 `inv.TaxRate = line.TaxRate`** → 即便明细行解析出税率，发票级 `tax_rate` 仍恒空。
2. 规则命中即**短路跳过 LLM**（`ruleOK` 分支），半成品永久留存，没有任何补救通道。

---

## 二、故障 1 取证：分类为什么落入「其它票据」

同批（04:16）上传 5 个文件，实测：

| 文件 | fleet_cert_type | 逐票 category | 张数 |
| --- | --- | --- | --- |
| ad740ba3 | 通行费发票 | 通行费发票 | 27 |
| ce8992c6 | 通行费发票 | 通行费发票 | 14 |
| **ce450a5c** | **（空）** | **（空串）** | 20 |
| 2c61b318 | 通行费发票 | 通行费发票 | 17 |
| **7f626b8c** | **（空）** | **（空串）** | 24 |

**交替 3 对 2 错**。两个根因叠加：

**根因 A：`invoice_category` 从未被写入。**
全库发票文件的 `custom_metadata.invoice_category` 一律不存在，只有 `fleet_cert_type`。

**根因 B：整列替换 + 陈旧快照引发的竞态。**
`SaveInvoiceCustomMetadata` 原实现是：

```go
record.CustomMetadata = meta   // 整列替换
```

而 `knowledge := GetKnowledgeByID(...)` 是 handler **入口快照**，之后每次落库都用它重建整列。
而分类是「parse 完成后前端另发一次 `PUT invoice-metadata`」写入的 —— 两者谁写在后谁赢。
若提取先读到旧快照、分类后写，则提取的整列写入把分类**冲成空**。

空串 category 会被「其它票据」的**排除式桶**吞掉：

```go
if filter.InvoiceType == "其它票据" { /* category 不属于启用分类集合 → 归入此桶 */ }
```

---

## 三、故障 3 取证：为什么被强制退出登录

`refreshAccessTokenShared` 的单例锁（`isRefreshing` + `failedQueue`）**本来就正常工作**，
不是 refresh token 轮换竞态。

真实日志（`data/server_vm.log`，12:18 / 12:21 / 12:22 / 12:30，正是解析轮询窗口）：

```
[auth] bearer token rejected: look up token: failed to connect to
  ... aws-0-ap-northeast-2.pooler.supabase.com:5432:
  FATAL: (EMAXCONNSESSION) max clients reached in session mode
  - max clients are limited to pool_size: 15 (SQLSTATE XX000)

POST /api/v1/auth/refresh -> 401 {"code":1001,"message":"Token refresh failed"}
```

**根因：Supabase session 模式连接池上限 15 被打满 → 鉴权 SQL 失败 →
中间件把「基础设施故障」一律映射成 401 → 前端当成登录失效，清 token + 跳登录。**

`ValidateToken` 的注释本来就写着「a failed lookup is an infrastructure error」，
但中间件没有区分，直接 `c.JSON(401, ...)`。这就是「50 秒解析轮询期间被踢数次」的机制。

---

## 四、修复

### 4.1 分类铁律（新增 `knowledge_invoice_category_lock.go`）

| 组件 | 作用 |
| --- | --- |
| `mergeInvoiceMetaPreservingUserKeys(existing, fresh)` | 提取落库改为**合并**：只覆盖提取管理的键；`invoice_category`/`fleet_cert_type`/`auto_deleted_*` 等用户拥有的键自动保留；新值显式携带则优先（人工改分类生效）；提取出真实发票时清掉残留 `pending_file_delete` |
| `ResolveInvoiceCertType(request, meta)` | 权威优先级：请求 `cert_type` > `invoice_category` > `fleet_cert_type` |
| `LockInvoiceCategory(res, certType)` | certType 非空时，每张票 `invoice_type` 与 `category` 强制锁定，模型输出一律无效 |
| `SeedInvoiceCategoryMeta(cat, scope)` | 上传时写入的种子 metadata |

**上传即持久化**：`POST /knowledge-bases/:id/knowledge/file` 新增 `invoice_category`
表单字段，创建记录后**立即**写入（在响应返回之前）。前端 `FleetUploadDialog`
随 upload 一起提交，不再依赖事后 PUT。

**不再留空**：模型返回空大类时，用已启用的细分 `category` 兜底，避免空串被排除式桶吞掉。

### 4.2 MinerU 适配

| 修复 | 说明 |
| --- | --- |
| 标签驱动明细行 | 新增 `htmlItemRowByLabels` / `applyHTMLItemLabels`：**逐单元格锚定标签**取值，与列序解耦；优先于位置式解析 |
| 标签空格容错 | `flatPartyCellRe` → `购\s*买\s*方|销\s*售\s*方`；税号/地址/开户行标签同样容忍内部空格 |
| 名称收口 | 名称/地址/开户行改为「非贪婪吃到下一个标签」，不再吞整块 |
| 税率回填 | 补上 `inv.TaxRate = line.TaxRate` |
| 免税票 | 税率列写「不征税/免税/零税率」→ 税率与税额置 **0**（不是空） |
| 字面量转义 | `cleanLiteralEscapes` 清掉单元格内的 `\n`/`\t` 两字符序列 |
| 半残守卫 | `InvoiceRuleResultIncomplete`：全部票缺核心字段 → 回退 LLM；部分缺 → 保留并把问题写进 `extract_error` 供复核 |

### 4.3 401 → 503

| 层 | 改动 |
| --- | --- |
| types | 新增 `ErrTokenStoreUnavailable` |
| service | `ValidateToken` / `RefreshToken` 的查询失败用 `%w` 打标 |
| middleware | 该错误 → **503 + `code=auth_backend_unavailable`**，其余才 401 |
| handler | `/auth/refresh` 同样返回 503 |
| frontend | `refreshAccessTokenShared`：5xx/网络错误只 reject，**不清凭据不跳登录**；仅 401/400 按失效处理 |

---

## 五、真机实测

### 5.1 目标文件 `ad740ba3`（27 张票）

| 字段 | 修复前 | 修复后 |
| --- | --- | --- |
| 金额 | 0/27 | **27/27** |
| 税率 | 0/27 | **27/27** |
| 税额 | 0/27 | **27/27** |
| 价税合计 | 27/27 | 27/27 |
| 购买方 | 15/27 | **27/27** |
| 购买方税号 | 15/27 | **27/27** |
| 销售方 | 15/27 | **27/27** |
| 销售方税号 | 15/27 | **27/27** |
| 分类 / 类型 = 通行费发票 | 27/27 | 27/27 |

抽查 `49367445`（原缺全部金额与购销方）：

```
票号=49367445  日期=2025-06-04  类型=通行费发票  分类=通行费发票
金额=234.25  税率=0.03  税额=7.03  价税合计=241.28
购买方=重庆星达铜业有限公司 / 91500227MA5U54AJ0A
销售方=重庆沪渝高速公路有限公司 / 91500108696569675Y
```

免税票 `34013247`：`amt=73.28 rate=0 tax=0 total=73.28`（原 `rate`/`tax` 为空）。

### 5.2 分类订正

| 项 | 结果 |
| --- | --- |
| 原错归文件 `7f626b8c`（24 张）/ `ce450a5c`（20 张） | 全部改回 `通行费发票`，`invoice_category=通行费发票` |
| 全库「逐票 category 为空」 | **0** |
| 全库回填 `invoice_category` | **130** 个文件 |
| 「有车牌号却不在通行费分类」的票 | **0 条** |

### 5.3 全库缺项复查

```
缺金额=0   缺购销方=0   通行费文件缺项数=0
剩余：4 张电费/水费票税率或税额为空
```

剩余 4 张均为**电费/水费**票（`26502000001863021406` 等），票面本身没有该列或是 0 税率，
不属于本次「通行费缺项」范围。

### 5.4 状态机

`TestAuthBackendUnavailableReturns503Not401`：
- 鉴权存储不可用 → **503** + `auth_backend_unavailable`
- token 真失效 / 无效签名 → **401**
- 未携带 Bearer → **401**（不因存储故障误报 503）

前端 `authRefresh.test.ts` 新增 4 项：
- 503 → 保留 access/refresh token，不跳登录
- 网络错误 → 保留凭据
- 401 → 清凭据 + 跳登录
- 缺 refresh token → 按失效处理

---

## 六、门禁

| 项 | 结果 |
| --- | --- |
| `go vet ./internal/...` | **0** |
| 后端白名单（Business/Extract/DailyAffairs/Golden/Invoice/Auth/Token/Duplicate） | **TEST=0** |
| `go build ./internal/...` + anydoc 全量构建 | **0** |
| 新增后端单测 | `TestExtractInvoiceMineru*` 7 项 + `TestExtractInvoiceCategory*/Rule*` 5 项 + `TestAuthBackendUnavailable*` 2 项，全绿 |
| 前端 `src/utils/*.test.ts` | **441 pass / 0 fail** |
| `vue-tsc --build --force` | **0 error** |
| `eslint`（改动文件） | **0 error** |

---

## 七、遗留与建议

1. **连接池是环境级隐患**（未在本轮改）：Supabase **session 模式上限 15** 对 3s 轮询
   + 并发提取偏紧。建议改用 **transaction 模式 pooler**（`:6543`）或把后端最大连接数
   压到 10 以下并给鉴权查询单独的连接预算。3 秒轮询间隔也可考虑在解析长任务时退避到 5~8s。
2. **`invoice_category` 与 `fleet_cert_type` 双字段并存**：本轮保留兼容（历史数据只有后者），
   读取时前者优先。建议数据稳定一个版本后收敛为单字段。
3. **4 张电费/水费票仍缺税率/税额**：属票面本身无该列，若业务上要求「0 也要落值」，
   需要在电费/光伏的规则里加同样的免税归一逻辑（本轮只改通行费路径）。
