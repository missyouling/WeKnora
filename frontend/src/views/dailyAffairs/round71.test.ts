// 第 71 轮单测：持睿表房合一的契约、空表过滤、配套去重、费用别名链路。
//
// 说明：后端迁移逻辑由 Go 侧 TestRound71* 覆盖（真库行为）；
// 这里锁住前端契约（字段名/去重/别名/合计行），防止后续改动悄悄破坏。
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (rel: string) => fs.readFileSync(path.join(DIR, '..', '..', '..', '..', rel), 'utf8')
const readLocal = (name: string) => fs.readFileSync(path.join(DIR, name), 'utf8')

test('第71轮：持睿 11 间房的表计字段契约（E021–E031 / W021–W031）', () => {
  const go = read('internal/handler/hr_round71.go')
  // 只认持睿前缀 + 只动宿舍档案 + JSONB 必须走 serializer
  assert.match(go, /alias LIKE \?/)
  assert.match(go, /"持睿%"/)
  assert.match(go, /Select\("custom_data"\)\.Updates\(room\)/)
  // 覆盖的四个字段（电/水各两名一号 + 单价 + 倍率）
  for (const k of ['宿舍电表名称', '宿舍电表号', '宿舍水表名称', '宿舍水表号',
    '电费标准(元/度)', '电表倍率', '水费单价(元/m³)', '水表倍率', '所属公司']) {
    assert.ok(go.includes(k), `迁移必须写入 ${k}`)
  }
  // 幂等：值相同直接跳过（不产生无意义写入）
  assert.match(go, /if old == want \{\s*\n\s*return\s*\n\s*\}/)
})

test('第71轮：空表号的房源不得进入核算表计源', () => {
  const go = read('internal/handler/utility_round70.go')
  assert.match(go, /if no == "" \|\| seen\[strings\.ToUpper\(no\)\] \{\s*\n\s*return\s*\n\s*\}/)
  assert.ok(go.includes('第 71 轮'), '需要保留空表过滤的说明')
})

test('第71轮：配套设施下拉严格按名称去重（避免 485 条台账撑爆选项）', () => {
  const src = readLocal('HrManagement.vue')
  // 归一化：折叠空白 + trim
  assert.match(src, /const norm = \(v: any\) => String\(v \?\? ''\)\.replace\(\/\\s\+\/g, ' '\)\.trim\(\)/)
  // 三路来源全部入 Set
  assert.match(src, /for \(const v of HR_DORM_STANDARD_FACILITIES\) \{\s*\n\s*const k = norm\(v\)/)
  assert.match(src, /for \(const v of dormFacilityOptions\.value\) \{\s*\n\s*const k = norm\(v\)/)
  assert.match(src, /return \[\.\.\.names\]\.map\(\(v\) => \(\{ label: v, value: v \}\)\)/)
})

test('第71轮：住宿记录费用项长短名双向别名仍然生效', () => {
  const go = read('internal/handler/hr_dorm.go')
  // 长名（房档口径）与短名（住宿记录 subs 口径）都要在可覆写键里
  for (const k of ['"宿舍押金(元)"', '"押金金额(元)"', '"电费标准(元/度)"', '"电费标准"']) {
    assert.ok(go.includes(k), `覆写键必须包含 ${k}`)
  }
  assert.match(go, /var hrDormFeeAliases70 = map\[string\]\[\]string\{/)
  // 核算取价：住宿记录优先 → 房档基准
  const charges = read('internal/handler/hr_dorm_charges.go')
  assert.match(charges, /if v, ok := hrDormFeeLookup70\(stay\.CustomData, key\); ok \{/)
  assert.match(charges, /if v, ok := hrDormFeeLookup70\(room\.CustomData, key\); ok \{/)
})

test('第71轮：资产档案合计行只保留首列文案（485 条不汇总数值）', () => {
  const src = readLocal('HrManagement.vue')
  // 宿舍四档案（含资产）统一 onlyLabel；费用信息仍保留金额汇总
  assert.match(src, /activeTab\.value === 'hr-dorm' && dormSubView\.value !== 'fee'/)
  assert.match(src, /onlyLabel: footOnlyLabel\.value/)
})

test('第71轮：d71 真机脚本覆盖四组关键断言', () => {
  const src = read('tests/e2e/d71.mjs')
  for (const needle of ['持睿汽车部件有限公司', 'E021', '88,147.20', '配套设施']) {
    assert.ok(src.includes(needle), `d71 必须断言 ${needle}`)
  }
})
