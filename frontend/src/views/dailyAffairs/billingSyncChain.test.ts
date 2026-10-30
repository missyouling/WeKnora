import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 费用清单第 7 轮「六同步断链」回归契约（2026-10-28）。
 *
 * 用户操作：在「设置 → 字段定义」里搜索添加 `[市电账单] 账单周期 / 本期电费 /
 * 本期电量` 并保存 → 工具栏 `[☰ 字段]` 筛选器与列表表头都不出现这些字段。
 *
 * 断点有四处，缺一处就复现：
 *   B）`activeColDefs` 的**租户视角分支**只拼 anchors + 分摊子项，直接 return，
 *      引用字段全部丢失（业主视角正常，所以「换个视角就看得见」）；
 *   B2）即便列拼出来，`buildRows` 的租户分支不写引用字段的值 → 有表头没数据；
 *   C）列可见性存档只记「可见列」，无法区分「用户取消勾选」与「新增列」；
 *   D）保存后 `loadCategorySubs` 只重算列集、不重算行 → 单元格空白。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

// ---- B：两套视角都必须出列 ----

test('租户视角不得丢弃跨分类引用字段（同一循环产出，不按视角分支丢弃）', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('const activeColDefs = computed'), src.indexOf('const colStorageKey'))
  // 单一主循环：内置列与引用列都按 subs 下标顺序 push（第 15 轮起先做大类折叠）
  assert.match(fn, /for \(const row of collapseFeeGroups\(effective\)\)/)
  assert.match(fn, /const src = parseFieldSource\(sub\)/)
  // 视角差异只剩「是否追加租户分摊子项」，不得再过滤/重排 subs 列：
  // 业主视角直接返回主循环产出；租户视角在同一个 out 上追加子项与合计应付。
  assert.match(fn, /if \(isOwnerView\.value\) return out/)
  // 分摊子项（去重后）补进同一个 out：字段定义里没保存过的新子项也要能出列
  assert.match(fn, /for \(const it of ownerView \? \[\] : tenantItemsUnique\.value\)/,
    '租户子项只在租户视角补列（业主视角补了会多出空列）')
  assert.match(fn, /out\.push\(\{\s*key,/, '分摊子项必须追加进同一个 out（不是另起一份列集）')
})

test('引用字段列不得被内置列白名单过滤掉', () => {
  const src = read('TenantBilling.vue')
  const head = src.slice(src.indexOf('const activeColDefs = computed'), src.indexOf('const colStorageKey'))
  assert.ok(!/restrictToBase/.test(head),
    '不得用 restrictToBase 白名单：没写进 COL_DEFS_* 的内置字段会被静默丢弃')
  // 引用字段仍必须由 source 分流（否则会被当成内置列套用错误宽度/口径）
  assert.match(head, /const src = parseFieldSource\(sub\)/)
})

test('引用字段列 key 必须带跨分类前缀，避免与内置同比名（如内置「账单周期」= month）撞键', () => {
  const src = read('FleetCertPanel.vue')
  const fn = src.slice(src.indexOf('function addRefField'), src.indexOf('function groupCount'))
  assert.match(fn, /const name = \['src', src\.category, src\.group, src\.field\]/,
    'key 必须是 src_<分类>_<分组>_<字段>，既唯一又可回溯')
})

test('新增引用字段默认「默认表头」，保存后立即出列', () => {
  const src = read('FleetCertPanel.vue')
  const fn = src.slice(src.indexOf('function addRefField'), src.indexOf('function groupCount'))
  assert.match(fn, /isDefault:\s*true/, '新增字段的默认表头必须默认开启（否则保存后列表毫无变化）')
  assert.match(fn, /enabled:\s*true/, '新增字段必须默认启用')
  assert.match(fn, /source:\s*src/, '必须带上完整 source（category/group/field/label）')
})

// ---- B2：租户视角要有值 ----

test('buildRows 两套视角都要回填引用字段的值与口径说明', () => {
  const src = read('TenantBilling.vue')
  // 第 15 轮起大类汇总列也带 source（mode=fee_group），必须显式跳过，
  // 否则会被当成「引用字段」用 resolveSourceValue 覆盖掉汇总值。
  const fill = src.match(/if \(!c\.source \|\| c\.feeGroup\) continue/g) || []
  assert.ok(fill.length >= 2, `业主/租户都要回填引用字段值，实际命中 ${fill.length} 处`)
  const assign = src.match(/base\[c\.key\] = v/g) || []
  assert.ok(assign.length >= 2, `两套视角都要把值写进行对象，实际命中 ${assign.length} 处`)
  // 提示同理：租户分支不写 sourceFieldTip 的话，hover 出来是空白卡片
  const tipHits = src.match(/sourceFieldTip\(\{/g) || []
  assert.ok(tipHits.length >= 2, `两套视角都要生成引用字段提示，实际命中 ${tipHits.length} 处`)
})

// ---- C：存档要能区分「新列」与「用户关掉」 ----

test('列存档必须是 v2 结构（known + visible），否则区分不出新列与被关掉的列', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /interface ColStore \{ v: 2; known: string\[\]; visible: string\[\] \}/)
  const fn = src.slice(src.indexOf('function loadStoredKeys'), src.indexOf('const persistColumns'))
  assert.match(fn, /!known\.has\(c\.key\) && c\.default/, '新列（不在 known 里）按 default 自动勾选')
  assert.match(fn, /picked\.has\(c\.key\)/, '已勾选列按 visible 还原')
  // 老 v1 数组要能读（否则升级后所有人的列偏好被清空）
  assert.match(src, /Array\.isArray\(parsed\)/, '必须兼容 v1 纯数组存档')
})

test('写存档必须同时写 known 与 visible，且清洗已删除的键', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('const persistColumns'), src.indexOf('const syncColumnKeys'))
  assert.match(fn, /v:\s*2/)
  assert.match(fn, /known:\s*defs\.map\(c => c\.key\)/)
  assert.match(fn, /visible:\s*visibleKeys\.value\.filter\(k => defs\.some/)
})

// ---- D：保存后免刷新联动 ----

test('字段定义保存后必须重算列集**并重建行**（否则新列单元格空白）', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('async function loadCategorySubs'), src.indexOf('const colStorageKey'))
  assert.match(fn, /syncColumnKeys\(\)/)
  assert.match(fn, /if \(dataReady\.value\) buildRows\(\)/,
    '列集变化后要重建行：引用字段的值是 buildRows 写进 row 的')
})

test('必须监听 fleet-categories-changed 并暴露 reloadSubs 供父级显式调用', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /addEventListener\('fleet-categories-changed'/)
  assert.match(src, /const reloadSubs = async \(\) => \{/)
  assert.match(src, /defineExpose\(\{ reloadTenants, reloadSubs \}\)/)
})

// ---- A：后端不得丢 source ----

test('后端 subs 更新不得重建/裁剪 source（结构体字段齐全即可完整往返）', () => {
  const go = readFileSync(new URL('../../../../internal/handler/fleet.go', import.meta.url), 'utf8')
  const fn = go.slice(go.indexOf('func (h *FleetHandler) UpdateFleetCategory'), go.indexOf('func (h *FleetHandler) SortFleetCategories'))
  // 落库 = 原样 json.Marshal(req.Subs)，只做「系统列剥离 + 明细子列去重」
  assert.match(fn, /dataJSON, _ := json\.Marshal\(req\.Subs\)/)
  assert.match(fn, /stripListSystemSubs\(req\.Subs\)/)
  assert.ok(!/FleetCategorySubSource\{/.test(fn), '不得在保存路径里重建 source（会丢 group/label）')
})
