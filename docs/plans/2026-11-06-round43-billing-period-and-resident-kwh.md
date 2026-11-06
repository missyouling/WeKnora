# 第 43 轮实施计划：账单周期倒序 / 电量（定比）权威取数 / 重新核算打通

## 根因结论（已用真机库验证，非推测）

| # | 现象 | 根因（证据） |
|---|---|---|
| 1 | 市电账单列表月份穿插错乱（2026-10、2026-09、2025-06、2025-07…） | `knowledge_utility_bill_list.go:168` 与 `:308` 用 `sort.SliceStable(... CreatedAt.After ...)` 排序 —— 按**上传时间**而非账单周期。光伏列表同一文件同口径。费用清单 `TenantBilling.vue:1252` 已按 `months.sort().reverse()` 正确。 |
| 2 | 费用清单「电量（定比）」出现 147.32 / 0 / 149.79 等脏值 | `residential_readings[].bill_kwh` 存量脏（11/17 月错），而 `TenantBilling.vue:1089` 的 `residentInfoOf` **优先取 bill_kwh**。权威源 `fee_items` 的 `(5)目录电费` qty 在 17 个月全部正确（2026-10=9354 … 2025-06=14363）。 |
| 3 | 「重新核算」修不回 147.32 | 租户视角按钮走 `generateBillingRecord`（只写 `billing_records`，不碰 `knowledges` 的 `residential_readings`）；业主视角只 `loadAllData()` 本地重算，脏源不动。且上游数据变更无任何自动重算触发。 |

**权威取数规则**：`电量（定比）` = 该月市电账单 `fee_items` 中 `(5)目录电费` 各行 `qty` 之和（2025-07 有两行 4501+11953=16454，其余月份单行）。`residential_readings[].bill_kwh` 是派生缓存，必须与之对齐。

## 任务分解（TDD，每步先写失败测试）

### T1 账单周期严格倒序（后端为主）
1. 新增 `internal/application/service/bill_period_sort.go`：`billPeriodKey(raw)` 归一化为 `YYYY-MM`（复用 `NormalizeBillingPeriod`），空值排最后。
2. 测试：`TestBillPeriodSortDesc` —— 混入 `"2026.10"`/`"2026年7月"`/`"2026-7"`/`""` 的列表排序后严格递减。
3. 改 `knowledge_utility_bill_list.go` 两处 `sort.SliceStable`：主键 `billPeriodKey(Item.BillPeriod)` DESC，次键 `CreatedAt` DESC，稳定。
4. 前端 `UtilitiesManagement.vue` `displayRows` 兜底再排一次（防御接口未升级/本地合并行）。

### T2 电量（定比）权威取数 + 17 月洗数
1. 新增 `internal/application/service/utility_resident_kwh.go`：
   - `ResidentBillingKwh(item)` 取 `(5)目录电费` qty 之和；无该分类时退回主行 `bill_kwh`。
   - `RepairResidentReadingBillKwh(item) (changed bool)` 把主行 `bill_kwh` 对齐到权威值（多行时不改，避免破坏表计行）。
2. 测试：`TestResidentBillingKwhFromCatalogFee`（9354/9821/2025-07 两行合计）、`TestRepairResidentReadingBillKwh`（147.32→9821、0→9683、已是正确值时不改动）。
3. 前端 `residentInfoOf` 改为：先读 `(5)目录电费` qty → 再 `bill_kwh` → 再 `kwh×ratio`。
4. 新增启动幂等回填 `RepairResidentBillingKwh`（handler，纯 db 读写，跳过 manual），覆盖 17 个月。

### T3 重新核算（勾选 / 全量）+ 数据源自动重算
1. 后端新增 `RegenerateBillingByPeriod(period)`：按月份重算所有使用单位（复用 `GenerateBillingRecord` 内核）。
2. 触发点：市电/光伏账单保存与删除、电费/水费/气费核算记录增删改。
3. 前端：`handleGenerateAll` 按 `selectedKeys.size` 区分 Toast（`已重新核算选中的 k 条记录` / `已完成全部 N 条记录重新核算`）；`notifyEnergyDataChanged` 已有，补上「切回费用清单即最新」。

## 门禁
`npm run type-check` / `lint` / `test` / `build`；`go vet ./...` + 白名单；Playwright 4 断言。
