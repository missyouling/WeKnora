/**
 * 第 70 轮单测：
 *   ① 数值格式化：面积/金额去 `.0000`，单价保留到厘
 *   ② 状态语义唯一映射（合同到期 / 五险参保 / 宿舍四态 / 资产报废）
 *   ③ 宿舍费用项目拆分（宿舍只电+水，公租房只水+气）
 *   ④ 住宿记录费用项随房源类型 + 可编辑（不再是只读）
 *   ⑤ 12 类标准配套设施
 *   ⑥ 表计设置只留工商业 + 底部保存/取消
 *   ⑦ 三段表最后一列无右边框、抽屉短竖条手柄
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { cellNum, priceNum } from './formatNumber'
import { resolveStatusTone, statusTagThemeOf, statusTextClassOf } from './hrStatusStyle'
import {
  HR_DORM_FIELDS, HR_DORM_STANDARD_FACILITIES, hrDormDrawerGroups,
} from './hrDorm'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (f: string) => fs.readFileSync(path.join(DIR, f), 'utf8')
const readSrc = (rel: string) => fs.readFileSync(path.join(DIR, '../../../', rel), 'utf8')

test('第70轮：面积/金额去 .0000，单价保留到厘', () => {
  // 后端 numeric(10,4) 原样是 28.0000 / 100.0000 / 1.0000 / 2.1960
  assert.equal(cellNum(28.0), '28')
  assert.equal(cellNum('28.0000'), '28')
  assert.equal(cellNum(49.28), '49.28')
  assert.equal(cellNum('45.6000'), '45.6')
  assert.equal(cellNum(100.0), '100')
  assert.equal(cellNum(''), '')
  assert.equal(cellNum(null), '')
  // 单价：0 显示 0，保留有效小数
  assert.equal(priceNum('1.0000'), '1')
  assert.equal(priceNum('5.0000'), '5')
  assert.equal(priceNum('0.5200'), '0.52')
  assert.equal(priceNum('3.3200'), '3.32')
  assert.equal(priceNum('2.1960'), '2.196')
  assert.equal(priceNum('0.0000'), '0')
  assert.equal(priceNum(''), '')
})

test('第70轮：状态语义唯一映射（列表与抽屉同源）', () => {
  // 合同：到期必须红（旧实现抽屉里是黑字）
  assert.equal(resolveStatusTone('合同状态', '到期'), 'danger')
  assert.equal(resolveStatusTone('合同状态', '已终止'), 'danger')
  assert.equal(resolveStatusTone('合同状态', '已解除'), 'off')
  assert.equal(resolveStatusTone('合同状态', '即将到期'), 'warn')
  assert.equal(resolveStatusTone('合同状态', '未签订'), 'warn')
  assert.equal(resolveStatusTone('合同状态', '正常'), 'ok')
  // 五险参保
  assert.equal(resolveStatusTone('养老参保状态', '参保缴费'), 'ok')
  assert.equal(resolveStatusTone('医疗参保状态', '暂停缴费'), 'warn')
  assert.equal(resolveStatusTone('失业参保状态', '终止参保'), 'off')
  // 宿舍四态 + 住宿 + 资产
  assert.equal(resolveStatusTone('宿舍状态', '满员'), 'danger')
  assert.equal(resolveStatusTone('宿舍状态', '部分入住'), 'warn')
  assert.equal(resolveStatusTone('宿舍状态', '空闲'), 'ok')
  assert.equal(resolveStatusTone('宿舍状态', '维修中'), 'off')
  assert.equal(resolveStatusTone('住宿状态', '在住'), 'ok')
  assert.equal(resolveStatusTone('住宿状态', '已退宿'), 'off')
  assert.equal(resolveStatusTone('资产状态', '正常'), 'ok')
  assert.equal(resolveStatusTone('资产状态', '损坏'), 'warn')
  // 第 70 轮：报废由灰改红
  assert.equal(resolveStatusTone('资产状态', '报废'), 'danger')
  // 列表 Tag 与抽屉文字同 tone
  assert.equal(statusTagThemeOf('宿舍状态', '满员'), 'danger')
  assert.equal(statusTextClassOf('宿舍状态', '满员'), 'is-danger')
  assert.equal(statusTagThemeOf('资产状态', '报废'), 'danger')
  assert.equal(statusTextClassOf('资产状态', '报废'), 'is-danger')
})

test('第70轮：宿舍费用项只含电+水，公租房只含水+气', () => {
  const dorm = HR_DORM_FIELDS.dorm.map((f) => f.name)
  assert.ok(dorm.includes('宿舍电表名称'))
  assert.ok(dorm.includes('宿舍电表号'))
  assert.ok(dorm.includes('电费标准(元/度)'))
  assert.ok(dorm.includes('宿舍水表名称'))
  assert.ok(dorm.includes('宿舍水表号'))
  assert.ok(dorm.includes('水费单价(元/m³)'))
  // 员工宿舍没有燃气/租金/物业/垃圾处理
  for (const banned of ['关联气表', '气费标准(元/m³)', '垃圾处理费(元/套)',
    '二次供水费(元/m³)', '污水处理费(元/m³)', '房屋租金(元/月)', '物业费(元/月)']) {
    assert.ok(!dorm.includes(banned), `宿舍不应含 ${banned}`)
  }
  const pub = HR_DORM_FIELDS.public.map((f) => f.name)
  assert.ok(pub.includes('公租房水表名称'))
  assert.ok(pub.includes('公租房气表名称'))
  assert.ok(pub.includes('房屋租金(元/月)'))
  assert.ok(pub.includes('租期起始月份'))
  // 公租房不定义电表
  assert.ok(!pub.some((n) => n.includes('电表') || n === '电费标准(元/度)'))
  // 成员字段紧跟舍长之后，且只读
  const dormIdx = dorm.indexOf('管理员/舍长')
  assert.equal(dorm[dormIdx + 1], '成员')
  assert.ok(HR_DORM_FIELDS.dorm.find((f) => f.name === '成员')?.readonly)
  // 住宿记录ID 彻底删除
  assert.ok(!HR_DORM_FIELDS.stay.some((f) => f.name === '住宿记录ID'))
})

test('第72轮（第 70 轮已统一口径）：住宿记录费用项全部归「费用项目」且可独立覆写', () => {
  const groups = hrDormDrawerGroups('stay', { roomType: '公司宿舍' })
  const fee = groups.find((g) => g.key === 'fee')?.fields || []
  const names = fee.map((f) => f.name)
  /**
   * 第 72 轮：不论宿舍还是公租房，住宿记录都呈现同一份费用项
   * （短名，与 subs 一致），值落在该条记录的 fee_overrides，绝不反写房源档案。
   */
  for (const n of ['押金金额(元)', '电费标准', '水费单价', '气费标准', '房屋租金',
    '月费用单价', '季度租金', '物业费', '污水处理费', '二次供水费', '垃圾处理费', '分摊比例']) {
    assert.ok(names.includes(n), `住宿费用项缺 ${n}`)
  }
  assert.ok(fee.every((f) => !f.readonly), '住宿记录费用项必须可编辑（独立覆写）')
  // 第 72 轮：3 个「入住时*表度数」已彻底删除
  assert.ok(!names.some((n) => /度数/.test(n)), `度数字段应删除：${names.filter((n) => /度数/.test(n))}`)
})


test('第70轮：12 类标准配套设施齐全', () => {
  assert.equal(HR_DORM_STANDARD_FACILITIES.length, 12)
  for (const n of ['晾衣杆', '毛巾杆', '衣柜', '床柜', '床头柜', '空调遥控器',
    '空调', '电热水器', '床', '床垫', '钥匙', '窗帘']) {
    assert.ok(HR_DORM_STANDARD_FACILITIES.includes(n), `缺 ${n}`)
  }
  const src = read('HrManagement.vue')
  // 第 71 轮：改为「空 Set + norm() 逐个入集合」的严格去重（兼容资产台账里的空格变体）
  assert.match(src, /const names = new Set<string>\(\)/)
  assert.match(src, /for \(const v of HR_DORM_STANDARD_FACILITIES\)/)
  assert.match(src, /const norm = \(v: any\) => String\(v \?\? ''\)\.replace\(/)
})

test('第70轮：表计设置只保留工商业分组 + 底部保存/取消', () => {
  const src = read('MeterSettingsPanel.vue')
  // 第 77 轮：类型值归一为「工商业」（兼容旧 production），集合随之扩展
  assert.match(src, /const METER_KINDS_IN_SETTINGS70 = new Set\(\['工商业', 'production'\]\)/)
  assert.match(src, /if \(!METER_KINDS_IN_SETTINGS70\.has\(String\(m\.meter_kind \|\| ''\)\)\) continue/)
  assert.match(src, /class="meter-save-bar" data-meter-save-bar/)
  assert.match(src, /data-meter-cancel>取消</)
  assert.match(src, /data-meter-save>保存</)
  // 开关只记脏，保存时统一提交
  assert.match(src, /next\.add\(String\(m\.id\)\)/)
  assert.match(src, /await updateUtilityMeter\(id, meterPayloadOf\(m\)\)/)
})

test('第70轮：三段表最后一列不画右边框（消除贯穿细线）', () => {
  const src = readSrc('src/components/business/DetailScrollTable.vue')
  assert.match(src, /\.detail-table-head tr > \*:last-child,\s*\r?\n\.detail-table-foot tr > \*:last-child \{\s*\r?\n\s*border-right: none;/)
  // 合计行背景比分表头浅一档
  assert.match(src, /\.detail-table-foot \{\s*\r?\n\s*background: var\(--td-bg-color-container-hover\);/)
})

test('第70轮：抽屉拖拽手柄为居中短竖条（对齐原项目，无通顶线）', () => {
  const src = readSrc('src/components/settings/SettingDrawer.vue')
  assert.match(src, /class="setting-drawer-resize-grip"/)
  // 第 74 轮：手柄规格改为上游知识库文档详情抽屉的 2×48 / radius 1px
  assert.match(src, /\.setting-drawer-resize-grip \{[\s\S]{0,200}width: 2px;[\s\S]{0,120}height: 48px;[\s\S]{0,120}border-radius: 1px;/)
  // 第 74 轮：不再用 --grip-offset 把短条推到面板外，改为热区整体 left:-12px
  assert.match(src, /left: -12px;/)
  assert.match(src, /width: 12px;/)
  // 悬停/拖拽点亮品牌绿（tokens，不写死色值）
  assert.match(src, /background: var\(--td-brand-color\);/)
  // 不再有通高线
  assert.ok(!/height: 100%;\s*\r?\n\s*background: var\(--td-component-stroke\)/.test(src), '不得保留通顶竖线')
})

test('第70轮：管理员下拉只取本房源在住人员，空态禁用', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const dormRoomOccupants = computed<string\[\]>\(\(\) => \{[\s\S]{0,240}row\['在住人员'\]/)
  assert.match(src, /'管理员\/舍长': dormRoomOccupants\.value/)
  assert.match(src, /emptyPlaceholder: '暂无在住人员'/)
  assert.match(src, /if \(activeTab\.value === 'hr-dorm'\) normalizeDormNumberFields\(data\)/)
  // 空值必须归一成 ''（否则 JSON 丢键 → 清空保存无效）
  assert.match(src, /if \(d\[k\] === undefined \|\| d\[k\] === null\) d\[k\] = ''/)
})

test('第70轮：抽屉空候选渲染禁用下拉（不退化成输入框）', () => {
  const src = read('BusinessSplitDetailDrawer.vue')
  assert.match(src, /v-else-if="f\.selectLike" :model-value="undefined" disabled/)
  assert.match(src, /:placeholder="f\.emptyPlaceholder \|\| '暂无可选项'"/)
})
