# 车队管理模块现状摸底与中台对齐优化方案

> 摸底时间：2026-11-03　范围：`frontend/src/views/dailyAffairs/Fleet*`、`internal/handler/fleet.go`、`internal/application/service/knowledge_fleet_document.go`、`fleet_categories` / `fleet_records`（VM 库）
> 结论均来自代码走查与数据库实测，非推测。

---

## 一、三条硬结论

1. **12 个菜单里只有 3 个是真跑通的**（车辆档案 / 司机档案 / 维保记录：上传 → 解析 → 提取 → 列表 → 编辑）。其余 9 个（轮胎、年检、7 项成本）是「有表单、无数据、无字段配置、无提取规则、无 KPI 口径」的空壳。
2. **9 个记录型菜单目前根本存不进数据**：`POST /api/v1/fleet/records` 在 `vehicle_id` 为空时直接返回 **400「请选择车辆」**（`normalizeFleetRecord`），而 `GET /fleet/vehicles` 返回 `[]` —— 前端车辆下拉永远是空的。实测：`{"record_type":"tire","data":{...}}` → `400 {"code":1000,"message":"请选择车辆"}`。这是「功能看起来有、实际录不进」的死结。
3. **右侧内容区的骨架与发票/能耗完全不是一套**：自绘 KPI（`.stat-card/.type-card`）、自写批量浮条（`.doc-batch-bar-fixed`）、自绘抽屉（`SettingDrawer`）、自绘合计条（`.doc-summary-bar`）、自绘打印弹窗，且 `t-table` 挂着 `max-height="100%"`（第 25 轮已知懒加载坑）。

---

## 二、12 个菜单现状盘点

### 2.1 数据支撑

| # | 菜单 | `record_type` | `fleet_categories` 分类 | 数据来源 | 录入 / 提取方式 | 存量（存活 / 含删） |
|---|---|---|---|---|---|---|
| 1 | 车辆档案 | `vehicle-archive` | ✅ `vehicle` scope 5 类：道路运输经营许可证 / 道路运输证 / 车辆登记证书 / 保险单 / 行驶证 | `knowledges`（日常事务-车队）+ `fleet_records` 镜像 | 上传 → 解析 → 提取（LLM + 分类规则） | 车队库 17 份文件（14 成功 / 2 无状态 / 1 失败）；records 2 / 40 |
| 2 | 司机档案 | `driver-archive` | ✅ `driver` scope 2 类：驾驶证 / 从业资格证 | 同上 | 同上 | 2 / 2 |
| 3 | 维保记录 | `maintain-archive` | ✅ `maintain` scope 2 类：一般维修（工单）/ 合格证 | 同上 | 上传 + **手工录入** | 1 / 1 |
| 4 | 轮胎管理 | `tire` | ❌ 无 | `fleet_records.data` | 手工表单（14 字段，前端硬编码） | 0 |
| 5 | 年检管理 | `inspection` | ❌ 无 | 同上 | 手工表单（15 字段） | 0 |
| 6 | 加油充电 | `fuel-charge` | ❌ 无 | 同上 | 手工表单（8 字段） | 0 |
| 7 | 路桥费 | `road-toll` | ❌ 无 | 同上 | 手工表单（13 字段） | 0 |
| 8 | 维修保养费 | `repair-cost` | ❌ 无 | 同上 | 手工表单（7 字段） | 0 |
| 9 | 保险理赔 | `insurance-claim` | ❌ 无 | 同上 | 手工表单（17 字段） | 0 |
| 10 | 违章处理 | `violation` | ❌ 无 | 同上 | 手工表单（17 字段） | 0 |
| 11 | 辅材记录 | `material` | ❌ 无 | 同上 | 手工表单（4 字段） | 0 |
| 12 | 请车记录 | `car-request` | ❌ 无 | 同上 | 手工表单（5 字段） | 0 |

**后端白名单是齐的**：`fleetRecordTypes` 12 类全部登记，`fleet_categories` 却只有 3 个 scope（`vehicle` 5 条 / `driver` 2 条 / `maintain` 2 条）。也就是说：记录能存，但**没有字段目录 → 六同步的 ①字段定义 ②列表列 ③筛选器 ④提取规则 全部无从派生**，只能靠前端 `TYPES` / `RECORD_COLS` 两份硬编码。

### 2.2 存量数据

| 表 | 条数 | 说明 |
|---|---|---|
| `fleet_records` | 5（存活） | `vehicle-archive` 2 / `driver-archive` 2 / `maintain-archive` 1；另有 38 条已软删、1 条历史 `maintain` |
| `fleet_vehicles` | **0** | 所有记录型菜单的必填外键 —— 直接导致 400 |
| `fleet_drivers` | **0** | — |
| `fleet_fuel_cards` | **0** | 油卡面板空转 |
| `fleet_suppliers` | 1 | — |
| `fleet_etc_cards` | 1 | — |
| 车队知识库文件 | 17 | 14 提取成功 / 2 无提取状态 / 1 提取失败 |
| 文件 `doc_type` 分布 | 行驶证 5、保险单 3、道路运输经营许可证 2、空 2、从业资格证 1、维修工单 1、车辆登记证书 1、道路运输证 1、驾驶证 1 |

### 2.3 与发票模块的联动空间

| 事实 | 价值 |
|---|---|
| 发票库已提取 **1530 张通行费票**，明细带 `plate_no`（车牌号）、`travel_from/to`、金额/税率/税额 | 「路桥费」台账可 100% 由发票派生，无需重录 |
| 发票库存在 **9 个不同车牌**（`items[].plate_no` 去重） | 可自动初始化 `fleet_vehicles`，解开 400 死结 |
| 发票「能耗发票」62 张（油费/加油） | 「加油充电」可派生 |
| 车辆档案里的「保险单」分类 3 份文件 | 「保险理赔」可反向关联保单号 |

---

## 三、旧代码清理与中台组件复用映射

### 3.1 必删（重复造轮子 / 死代码）

| 旧实现 | 位置 | 规模 | 替代 |
|---|---|---|---|
| 自绘 KPI：`.archive-overview` + `.stat-card` + `.type-card`（两排大白框） | `FleetRecordList.vue` L59–113 | ~55 行模板 + ~180 行样式 | **`DashboardKpiGroup`**（`@/components/business/`） |
| 自写批量浮条 `.doc-batch-bar-fixed` | L185–215 | ~31 行 + 样式 | `BusinessListToolbar` 的 **`#batch-actions`** 插槽（第 26 轮契约：页面禁止自写） |
| 自绘详情抽屉 `SettingDrawer` + `.rec-grid` 三套模板 | L218–340 | ~123 行 + 样式 | **`BusinessSplitDetailDrawer`**（左 PDF 预览 / 右表单） |
| 自绘打印弹窗 `.meter-print-mask` | L342–360 | ~19 行 + 样式 | **`PrintPreviewDialog`** |
| 自绘合计条 `.doc-summary-bar`（单行「总金额」） | L178–183 | ~6 行 + 样式 | **`listSummary.ts`** + `t-table` 的 `footData` / `footSpan` |
| `fmtMoney` / `fmtNum` / `fmtVal` | L591–593 | 3 函数 | **`formatNumber.ts`**（`money2` / `num2` / `cellNum`） |
| `TYPES`（9 类 × 手写字段） | L439–589 | ~150 行 | 分类 `subs` 派生 |
| `RECORD_COLS`（9 类 × 手写列） | L647–777 | ~130 行 | 分类 `subs` 派生 + `columnWidth.ts` |
| `BILLING_COLS` + `isBilling` 整条分支 | L762 起 + L404 | ~20 行 | **删除**：`billing` 不在 12 个菜单的 `validTabs` 里，永不可达 |
| `FleetSettingsDrawer.vue`（`PANEL_MAP` 12 分支 + 自绘 resize） | 整文件 | 184 行 | **`StandardSettingDrawer`**（已支持 `vehicle/driver/maintain` scope、`initialCategory`、手动保存） |
| `t-table` 的 `max-height="100%"` | L130 | 1 处 | 删除（内层滚动容器会让懒加载监听失效） |
| 无门禁：`displayRows` / `overviewCards` 直接吃全量 | L860 / L880 | — | `invoicePendingGate` 同款三条件门禁 |

**预估**：`FleetRecordList.vue` 2540 行 → 约 1200–1400 行（-45%），`FleetSettingsDrawer.vue` 184 行整体下线。

### 3.2 必留（本次不动）

| 文件 | 行数 | 理由 |
|---|---|---|
| `FleetManagement.vue` | 192 | 左侧 12 菜单容器，按需求原样保留 |
| `FleetCertPanel.vue` | 2610 | 字段定义面板，已被 `StandardSettingDrawer` 内嵌 |
| `ExtractRulePanel.vue` | — | 提取规则面板，同上 |
| `FleetUploadDialog.vue` | 748 | 上传（分类选择 + 拖拽 + 粘贴 + 进度） |
| `FleetUploadHistoryDrawer.vue` | 903 | 历史记录抽屉（一次拉全 + 本地过滤，已合规） |
| `fleetCategorySubs.ts` | 449 | `subs` 双向映射唯一层 |
| `FleetOilCardPanel / FleetEtcPanel / FleetSupplierPanel` | 575 | 基础配置，降级为设置抽屉的 `extra-tab`（不在 12 菜单内，不占主界面） |

### 3.3 组件复用映射表（目标 → 现成组件）

| 目标区块 | 复用组件 / 工具 | 接入成本 |
|---|---|---|
| 顶部 KPI 卡片组 | `DashboardKpiGroup`（`KpiCard{key,label,icon,value,unit,sub,cls,action,typeValue}`） | 低：现有 `overviewCards` 只需把 `num→value`、`value→typeValue`、`action` 归一 |
| 最右「历史记录」卡 | 同上 + `FleetUploadHistoryDrawer` | 低：三卡合并为 1 卡，副标题 `无异常 / N 份异常` |
| 筛选栏 | `BusinessFilterBar`（`FilterFieldSpec[]` + `settingsText` + `primaryAction`） | 中：需从 `subs` 派生 `fields`（现有 `invoiceFilterFields.ts` 可参数化复用） |
| 列显隐 | `BusinessColumnFilter` | 低（已在用） |
| 设置抽屉 | `StandardSettingDrawer`（`scope` + `initialCategory` + `extraTabLabel`） | 低（已支持车队 scope） |
| 解析进度条 | `BusinessParseStatusBar`（`progress` + `pendingFiles` 轮播） | 低：替换自写 `.archive-pending-bar` |
| 数据列表 | `t-table` + `columnWidth.ts`（`colWidthOf / tableScrollOf / cellClassOf`） | 中：列对象需补 `dataType` / `cell` 渲染器 |
| 勾选感知合计行 | `listSummary.ts`（`buildFootRow / footSpanOf / summaryScopeRows`） | 中：需把 `columnDefs` 转成 `SummaryColumn` |
| 详情抽屉 | `BusinessSplitDetailDrawer`（`row/page-no/form/fields/sub-fields/quality`） | 中高：需把 `form` 收敛成 `{data, remarkParts, items}` 契约 |
| 明细合计行 | `itemTotals.ts`（`hasSumColumn / totalRowTexts`） | 低（抽屉内建） |
| 目录打印 | `useCatalogPdf.ts` + `PrintPreviewDialog` | 低（已在用 `generateCatalogPdf`，只换预览壳） |
| 格式化 | `formatNumber.ts` | 低 |

---

## 四、目标架构（右侧 100% 对齐发票/能耗）

```
FleetManagement.vue（左侧 12 菜单保留，右侧只渲染 <FleetRecordList :record-type>）
└─ FleetRecordList.vue（重写为三段式）
   ├─ ① 顶部：DashboardKpiGroup
   │    · 第 1 张：已收录证照（action=all，默认激活）
   │    · 第 2..N 张：当前菜单的启用分类（车辆档案=5 张、司机=2 张、维保=2 张、成本=7 张）
   │    · 最后 1 张：历史记录（合并原「11 已上传 / 0 解析失败 / 0 提取失败」）
   │    · 点击分类卡 → 只切数据源与表头，不跳二级页
   ├─ ② 中部：BusinessParseStatusBar + BusinessFilterBar
   │    · 左：关键词 + 分类 subs 派生的 filterable 字段 + 刷新
   │    · 右：[字段] BusinessColumnFilter → [设置] StandardSettingDrawer → [上传/新增] 主按钮
   │    · #batch-actions：重新解析 | 重新提取 | 打印目录 | 删除
   ├─ ③ 下部：t-table + listSummary 合计 + BusinessSplitDetailDrawer
   │    · 列：subs 同源（colKey/dataType/cell）+ columnWidth
   │    · 合计：勾选感知（选中时只汇总选中行）
   │    · 抽屉：左 PDF 预览 + 右表单 + 明细合计行
   └─ 门禁：解析中/提取中的文件 —— 不进列表、不参与合计、不计入 KPI
```

**六同步落点**（对齐第 26 轮契约）：

| 环节 | 车队现状 | 目标 |
|---|---|---|
| ① 字段定义 | 只有 3 个 scope | 成本 7 项补 scope（见待拍板 Q2） |
| ② 列表列 | `RECORD_COLS` 硬编码 | `subs` 派生 |
| ③ 筛选器 | 只有月份/证照类型两个固定下拉 | `subs` 派生 `filterable` |
| ④ 提取规则 | 成本 7 项无入口 | 设置抽屉内 `ExtractRulePanel` |
| ⑤ 抽屉字段 | `rec-grid` 手写三套 | `buildDetailFieldDefs(subs)` |
| ⑥ 打印列 | `useCatalogPdf` 已用 | 保持，列源换成 `subs` |

---

## 五、后端改造清单

| # | 改动 | 现状 | 说明 |
|---|---|---|---|
| B1 | `fleet_categories` 扩 scope | 仅 vehicle/driver/maintain | 成本 7 项需要分类（见 Q2），否则六同步无源 |
| B2 | `GET /fleet/records` 补分页/过滤/排序/`total` | 一次全量返回，无 `total`、无关键词、无字段筛选 | 对齐发票 `/invoices` 契约 |
| B3 | KPI 聚合口径 | `FleetOverviewStats` 只按 `doc_type` 统计文件 | 需补「已收录 / 在途 / 异常」三态，且**排除在途** |
| B4 | 车辆主数据初始化 | `fleet_vehicles` 0 条 | 从发票 `items[].plate_no` 回填 9 个车牌（幂等、只增不删） |
| B5 | `normalizeFleetRecord` 的车辆必填 | 硬性 400 | 改为：允许 `vehicle_id` 为空时用 `data.plate_no` 落库并自动建车辆 |
| B6 | 成本类汇总 | `GetFleetSummary` 只按 `vehicle_id` 聚合 `amount` | 补按 `record_type` / `record_month` 的合计，供合计行与 KPI |

---

## 六、分阶段实施计划

| Phase | 内容 | 交付物 | 风险 |
|---|---|---|---|
| **P0 数据打底**（0.5d） | 成本 7 项 scope + 默认 subs seed；车牌主数据从发票回填；`normalizeFleetRecord` 车辆兜底 | 后端 seed + 单测；`fleet_vehicles` 从 0 → 9 | 低（幂等 seed） |
| **P1 骨架对齐**（1d） | 删旧 KPI/旧合计/旧浮条/`max-height`；接 `DashboardKpiGroup` + `BusinessParseStatusBar` + `BusinessFilterBar` + `#batch-actions`；`FleetManagement` 去掉 `:key="activeTab"`（切菜单不再整页重建，只换数据源） | `FleetRecordList` 模板重构，视觉与发票一致 | 中（KPI 点击态与 `activeType` 映射；去 key 后需确保各菜单状态复位） |
| **P2 列表六同步**（1–1.5d） | `subs → 列/筛选/抽屉字段`；`columnWidth` + `formatNumber`；`listSummary` 勾选感知合计；门禁 | 六同步断言单测 + 真机 | 中（列持久化 key 迁移，需做旧存档兜底） |
| **P3 抽屉与打印**（1d） | `BusinessSplitDetailDrawer` 替换自绘抽屉（`form={data,remarkParts,items}`）；`PrintPreviewDialog` 替换 iframe；`StandardSettingDrawer` 替换 `FleetSettingsDrawer` | 抽屉/设置/打印三处 | 中高（`form` 契约改造，易踩「扁平 editForm」坑） |
| **P4 成本联动**（1.5d，待拍板） | 加油充电 / 路桥费 与发票联动（按车牌 + 月份聚合派生，或独立台账） | 派生规则 + 去重口径 | 高（需明确「发票已录，台账是否再录一遍」） |
| **P5 验收**（0.5d） | `go vet` + 白名单单测；`type-check` / `lint` / 全量单测 / VM build；真机 12 菜单逐项截图断言 | 验收报告 | 低 |

**总计约 5.5–6 人日**（不含 P4 业务口径反复）。

---

## 七、需要拍板的业务问题

| # | 问题 | 影响 | 我的建议 |
|---|---|---|---|
| **Q1** | 加油充电 / 路桥费：**独立台账**，还是从发票（油费票 / 通行费票）**派生**？ | 决定 P4 是否存在，以及是否要写「发票 → 台账」同步任务 | **派生为主 + 手工补录**：发票是原始凭据，台账是汇总视图；两者按「车牌 + 月份 + 金额 + 票号」去重 |
| **Q2** | 成本 7 项的分类粒度：**每菜单一个 scope（7 个）**，还是**统一 `fleet-cost` scope + 7 个分类**？ | 决定 `fleet_categories` 表结构与 `FleetCertPanel` 的 scope union 扩展量 | **统一 `fleet-cost` + 7 个分类**：一次扩展，六同步天然成立，设置抽屉复用零改动 |
| **Q3** | 车辆主数据权威：`fleet_vehicles` 为唯一源（从发票回填），还是允许列表直接用发票明细里的车牌？ | 决定 B4/B5 的实现与「车牌改名」时的联动 | **`fleet_vehicles` 为唯一源**，发票车牌只用于「自动补建」，不直接当外键 |
| **Q4** | 「已收录证照」卡口径：全部文件数，还是**已提取成功**份数？ | 影响首屏数字与用户判断 | **已提取成功**为主数字，副标题给「在途 N / 异常 N」 |
| **Q5** | 轮胎 / 年检：是否也要支持「上传证照自动提取」（现在是纯手填）？ | 决定要不要给这两个 scope 配 `subs` 与提取规则 | **轮胎保持手填**（无票据）；**年检支持上传**（有检测报告/发票） |
| **Q6** | 左侧菜单是否允许「成本管理」默认折叠 / 记忆展开状态？ | 交互细节 | 保持现状（默认展开），只加 `localStorage` 记忆 |

---

## 八、验收口径（提交前）

1. **门禁**：解析中/提取中的文件不出现在列表、不计入合计行、不计入 KPI（用临时改 `parse_status='parsing'` 做受控实验，验证行数与 KPI 同步变化后还原）。
2. **六同步**：切换 12 个菜单，表头序列 == 当前分类 `subs` 启用项序列；筛选器数量 == `filterable` 字段数。
3. **零重复造轮子**：`grep -c "doc-batch-bar-fixed\|stat-card\|meter-print-mask\|fmtMoney"` 在 `FleetRecordList.vue` 必须为 0。
4. **真机**：12 菜单逐项截图；KPI 点击切换后列表列/数据同步；抽屉左预览可翻页、右表单可保存。
5. **门禁命令**：`go vet` + 白名单单测、`npm run type-check`、`npm run lint`、全量前端单测、VM `npm run build` EXIT=0。

---

## 附：本次摸底的关键代码坐标

| 内容 | 位置 |
|---|---|
| 12 菜单定义 | `FleetManagement.vue` L60–76 |
| 自绘 KPI | `FleetRecordList.vue` L59–113、L879–960 |
| 自绘批量浮条 | 同上 L185–215 |
| 自绘抽屉 | 同上 L218–340 |
| 硬编码字段/列 | 同上 L439–589（`TYPES`）、L647–777（`RECORD_COLS`） |
| 不可达 `billing` 分支 | 同上 L404、L762 |
| 车辆必填 400 | `internal/handler/fleet.go` L601–607 |
| 分类 seed（仅 3 scope） | 同上 `seedBusinessCategories` L2072 |
| 费用汇总口径 | 同上 `GetFleetSummary` L759 |
