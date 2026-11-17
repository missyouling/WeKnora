/**
 * 第 61 轮源码级回归：
 *   · 明细弹窗三段分离表（唯一滚动区 = 数据区）；
 *   · 唯一实心绿底主按钮规则（工具栏 + 弹窗底部）与 AGENTS.md 落档；
 *   · 离职证明无后缀预览接口；
 *   · 宿舍管理五标签契约与公租房合计。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { HR_DORM_VIEWS, HR_DORM_FIELDS, HR_DORM_GROUP_OF, dormPublicTotals, hrDormColumns, hrDormBillColumns } from './hrDorm'
import { HR_TABS } from './hrCore'

const read = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')
const readLocal = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const strip = (t: string) => t.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')

test('明细表：三段容器，仅数据区可滚动（表头/合计行 overflow hidden）', () => {
  const src = readLocal('../../components/business/DetailScrollTable.vue')
  assert.match(src, /class="detail-table-head"/)
  assert.match(src, /class="detail-table-body"/)
  assert.match(src, /class="detail-table-foot"/)
  const css = strip(src.slice(src.indexOf('<style scoped>')))
  assert.match(css, /\.detail-table-head,\s*\.detail-table-foot\s*\{[^}]*overflow:\s*hidden/)
  assert.match(css, /\.detail-table-body\s*\{[^}]*overflow-y:\s*auto/)
  assert.match(src, /maxHeight:\s*\$\{props\.maxBodyHeight\}px|max-height: 380px/)
  // 列宽同源：三段各有一份 colgroup
  const tpl = src.slice(src.indexOf('<template>'), src.indexOf('</template>'))
  assert.equal((tpl.match(/<colgroup>/g) || []).length, 3)
  // 出现滚动条时给表头/合计行补 padding-right
  assert.match(src, /scrollbarWidth/, '必须实测滚动条宽度并补 padding')
})

test('账单明细弹窗：改用 DetailScrollTable（不再 sticky-header 单容器）', () => {
  const src = readLocal('HrManagement.vue')
  const block = src.slice(src.indexOf('hr-bill-table-wrap'), src.indexOf('hr-bill-footer'))
  assert.match(block, /<DetailScrollTable/)
  assert.ok(!/sticky-header/.test(block), '明细弹窗不得再用 sticky-header 把滚动条贯穿表头/合计行')
})

test('唯一实心绿底：工具栏导入/生成账单按优先级切换主题', () => {
  const src = readLocal('HrManagement.vue')
  assert.match(src, /const toolbarImportPrimary = computed/)
  assert.match(src, /:theme="toolbarImportPrimary \? 'primary' : 'default'"/)
  assert.match(src, /v-if="toolbarShowGenerateBill" theme="primary"/)
  // 弹窗底部：关闭为唯一实心绿底
  assert.match(src, /data-hr-bill-close @click="billDetailVisible = false"/)
  const agents = read('AGENTS.md')
  assert.match(agents, /主操作按钮唯一实心绿底规范（Single Solid-Green Primary Button Rule）/)
  assert.match(agents, /实心绿底/)
  assert.match(agents, /主操作恒在最右/)
})

test('离职证明：前端走无后缀预览路由，后端保留旧路径兼容', () => {
  const api = readLocal('hrApi.ts')
  assert.match(api, /roster\/resign-proof-view\$\{qs\(\{ id \}\)\}/)
  assert.ok(!/roster\/resign-proof\/\$\{encodeURIComponent\(id\)\}/.test(api), '不得再拼带扩展名的旧路径')
  const go = read('internal/handler/hr_bootstrap.go')
  assert.match(go, /resign-proof-view/)
})

test('宿舍管理：五标签顺序与设置胶囊 1:1', () => {
  assert.deepEqual(HR_DORM_VIEWS.map((v) => v.label), ['宿舍档案', '公租房档案', '住宿记录', '资产档案', '费用信息'])
  assert.deepEqual(HR_DORM_VIEWS.map((v) => v.module), ['dorm', 'public', 'stay', 'asset', 'fee'])
  for (const v of HR_DORM_VIEWS) assert.equal(HR_DORM_GROUP_OF[v.key], v.label)
  assert.equal(HR_TABS.length, 5)
  assert.equal(HR_TABS[4].key, 'hr-dorm')
  assert.equal(HR_TABS[4].label, '宿舍管理')
})

test('宿舍字段契约：五胶囊 15/17/15/10/9，状态列各自唯一', () => {
  assert.equal(HR_DORM_FIELDS.dorm.length, 15)
  assert.equal(HR_DORM_FIELDS.public.length, 17)
  assert.equal(HR_DORM_FIELDS.stay.length, 15)
  assert.equal(HR_DORM_FIELDS.asset.length, 10)
  assert.equal(HR_DORM_FIELDS.fee.length, 9)
  assert.ok(HR_DORM_FIELDS.dorm.some((f) => f.name === '宿舍状态' && f.readonly))
  assert.ok(HR_DORM_FIELDS.stay.some((f) => f.name === '住宿状态' && f.readonly))
  assert.ok(HR_DORM_FIELDS.fee.some((f) => f.name === '账单状态' && f.readonly))
})

test('公租房合计：3,401 / 485.55 / 2,915 / 486 / 10,203（图 7 口径）', () => {
  const rows = [
    { '建筑面积(㎡)': 49.28, '租金(元/月)': 296, '物业(元/月)': 49 },
    { '建筑面积(㎡)': 50.69, '租金(元/月)': 304, '物业(元/月)': 51 },
    { '建筑面积(㎡)': 47.95, '租金(元/月)': 288, '物业(元/月)': 48 },
    { '建筑面积(㎡)': 45.65, '租金(元/月)': 274, '物业(元/月)': 46 },
    { '建筑面积(㎡)': 46.83, '租金(元/月)': 281, '物业(元/月)': 47 },
    { '建筑面积(㎡)': 49.28, '租金(元/月)': 296, '物业(元/月)': 49 },
    { '建筑面积(㎡)': 49.28, '租金(元/月)': 296, '物业(元/月)': 49 },
    { '建筑面积(㎡)': 50.69, '租金(元/月)': 304, '物业(元/月)': 51 },
    { '建筑面积(㎡)': 47.95, '租金(元/月)': 288, '物业(元/月)': 48 },
    { '建筑面积(㎡)': 47.95, '租金(元/月)': 288, '物业(元/月)': 48 },
  ]
  const t = dormPublicTotals(rows)
  assert.equal(t.首月费用, '3401')
  assert.equal(t.建筑面积, '485.55')
  assert.equal(t.租金, '2915')
  assert.equal(t.物业, '486')
  assert.equal(t.季度, '10203')
})

test('宿舍列：费用信息列必须自带 value（否则单元格与合计全空）', () => {
  const cols = hrDormBillColumns() as any[]
  const dataCols = cols.filter((c) => c.colKey !== 'row-select')
  assert.equal(dataCols.length, 9)
  assert.ok(dataCols.every((c) => typeof c.value === 'function'), '每一列都要有取值函数')
  const row = { 账单周期: '2026-10', 房租合计: 570, 账单状态: '已生成' }
  assert.equal(dataCols[0].value(row), '2026-10')
  assert.equal(dataCols.find((c) => c.colKey === '房租合计').value(row), '570.00')
  // 宿舍档案列来自 subs/契约（hrColumnsFromSubs 自带 value）
  const dormCols = hrDormColumns('dorm', []) as any[]
  assert.ok(dormCols.some((c) => c.colKey === '宿舍编号'))
})

test('宿舍导入/生成账单入口：费用信息只有生成（无导入），其余四页可导入', () => {
  const src = readLocal('HrManagement.vue')
  assert.match(src, /if \(activeTab\.value === 'hr-dorm'\) return dormSubView\.value !== 'fee'/)
  assert.match(src, /if \(activeTab\.value === 'hr-dorm'\) return dormSubView\.value === 'fee'/)
  assert.match(src, /办理调宿/)
  assert.match(src, /办理退宿/)
  assert.match(src, /data-hr-dorm-bill-tabs/)
})
