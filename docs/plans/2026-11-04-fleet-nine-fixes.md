# 车队管理 9 项深度修复 — 实施计划（2026-11-04）

## 根因摘要（先定位再动手）

| # | 现象 | 根因 |
|---|---|---|
| 1 | 字段配置无列宽 | `FleetCertPanel.showWidthCol` 显式排除 `vehicle/driver/maintain`；列宽 UI 本身已存在 |
| 2 | 归档类无提取规则 | `DefaultExtractFields` 只覆盖 contract/invoice/regulation/award_punish/energy；`vehicle/driver/maintain` 的 8 个证照类型不在 `BusinessFieldCatalogs()` |
| 3 | 无状态三色/提取状态列 | 种子把「证件状态」当普通文本字段；无派生列、无状态筛选 |
| 4 | 预览背景重影 | `document-preview.vue` 的 loading 是 overlay，与 `v-else-if` 链并存；`previewType` 初值就是 `'unsupported'` → 加载期两个分支同时渲染 |
| 5 | 右侧冻结操作列 | 第 30 轮新增的 `colKey:'operation'` + `fixed:'right'`，发票管理没有 |
| 6 | 两个「暂无数据」 | TDesign `t-empty` 的 `title` 默认就是「暂无数据」，又被 `:description="暂无数据"` 传了一次 |
| 7 | 维保工具栏缺筛选 | 第 30 轮统一极简时把归档类筛选全删了，本轮按新口径补回（仅维保） |
| 8 | 合计行悬空 | 车队页局部样式 `.doc-list-scroll { overflow-y: auto }` 覆盖了中台共享样式（共享样式本身正确：flex 容器 + `overflow:hidden` + 表格 height:100%） |

## 任务拆分（每步 TDD：测试先红）

1. **纯函数层**（`fleetColumns.ts`，可单测）
   - `certStatusOf(data, today)` → `{ key: 'active'|'expiring'|'expired'|'', label, theme }`
   - `certStatusFieldOf(data)` → 命中的截止日期字段名
   - `extractStatusOf(row)` → `{ label: '提取成功'|'未提取', theme }`
   - `buildFleetCertStatusColumn()` / `buildFleetExtractStatusColumn()`
   - `normalizeFieldWidth(v)` → 0（自适应）或 60~600
2. **后端字段目录**（`business_field_catalog.go`）：新增 vehicle/driver/maintain 8 个证照类型的 catalog（Rule + Regex），注册进 `BusinessFieldCatalogs()`；补 `DefaultBusinessPromptTemplate`。
3. **后端提取配置懒建**：`GetExtractConfig` 对 `vehicle/driver/maintain` 走 `ensureBusinessExtractConfigDefaults` 同款路径（未保存过则合成落库）。
4. **前端列**：删操作列；归档类追加「证照状态」「提取状态」两列（只读、Tag 渲染）；列宽读 `subs[].width` 已在 `buildFleetColumns` 生效。
5. **前端筛选**：归档类工具栏 = 搜索 + 证照状态；维保额外加车牌号 + 日期范围。
6. **设置面板**：放开 FleetCertPanel 列宽列，范围 60~600，步长 10。
7. **空状态**：`BusinessEmptyState` 不再向 `t-empty` 重复传 title/description。
8. **合计贴底**：删除车队页对 `.doc-list-scroll` 的覆盖，回归中台共享样式。
9. **预览互斥**：`document-preview.vue` unsupported 分支加 `!loading && !iframePending` 守卫（最小 upstream-touch）。
10. **门禁 + 真机 6 条断言 + 能耗/发票回归截图。**

## 验收
- `go vet` / `go test`（白名单）/ `type-check` / `lint` / `build` 全绿。
- Playwright `probe-round31.mjs`：6 条断言逐项 PASS + 跨模块回归无 console error。
