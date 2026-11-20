/**
 * 第 67 轮单测：
 *   ① 三段分离表格的滚动条宽度补偿（有/无纵向滚动条都要像素级对齐）
 *   ② 账单弹窗标签结构：社保 3 标签、公积金 4 标签（岗位汇总），旧「按岗位汇总」彻底消失
 *   ③ 公积金岗位汇总按花名册真实岗位聚合，金额/人数与部门汇总守恒
 *   ④ 人事全模块六同步：自定义字段必须进入抽屉 / 模板 / 导入 / 列表
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { groupBillCents, isAdjustmentRow } from './hrBill'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (f: string) => fs.readFileSync(path.join(DIR, f), 'utf8')
const readSrc = (rel: string) => fs.readFileSync(path.join(DIR, '..', '..', rel), 'utf8')

test('第67轮：三段表滚动条补偿恒实测，且不再使用 scrollbar-gutter', () => {
  const src = readSrc('components/business/DetailScrollTable.vue')
  // 稳定槽位会让「无纵向滚动」时数据区仍窄一个滚动条宽度 → 必须删除
  assert.ok(!/scrollbar-gutter:\s*stable/.test(src), '不得再使用 scrollbar-gutter: stable')
  // 补偿宽度 = 实测 offsetWidth - clientWidth（无滚动条时天然 0），不再依赖是否溢出
  assert.match(src, /el\.offsetWidth - el\.clientWidth/)
  assert.ok(!/const overflow = el\.scrollHeight/.test(src), '补偿不能以「是否溢出」为前置条件')
  // 三段统一 border-box（padding 计入宽度）
  assert.match(src, /\.detail-table-head,\s*\n\.detail-table-foot \{[\s\S]{0,600}box-sizing: border-box/)
  // 表头 / 合计行仍通过 paddingRight 让出滚动条宽度
  assert.match(src, /paddingRight: scrollbarWidth\.value/)
})

test('第67轮：社保账单弹窗三标签，公积金四标签（岗位汇总）', () => {
  const src = read('HrManagement.vue')
  // 岗位标签只在公积金出现
  assert.match(src, /v-if="billDetailKind === 'fund'" value="post">岗位汇总</)
  // 旧文案彻底删除
  assert.ok(!/按岗位汇总/.test(src), '旧「按岗位汇总」文案必须全部清理')
  // 社保默认个人明细、公积金默认部门汇总
  assert.match(src, /billDetailTab\.value = kind === 'social' \? 'personal' : 'summary'/)
  // 打印 / 导出 / 表格共用一处取数，标签名同步
  assert.match(src, /label: '岗位汇总'/)
  assert.match(src, /function billTabSource/)
})

test('第67轮：岗位汇总只保留搜索框，且按人数降序', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /placeholder="搜索岗位"/)
  assert.match(src, /data-hr-bill-post-count>共 \{\{ billPostRows\.length \}\} 个岗位/)
  // 岗位下拉筛选已删除（需求只要求搜索框）
  assert.ok(!/data-hr-bill-post-select/.test(src), '岗位下拉筛选必须移除')
  assert.ok(!/billPostFilter/.test(src), 'billPostFilter 状态必须清理')
  // 排序走统一聚合器（人数降序 → 合计降序 → 名称）
  assert.match(src, /groupBillCents\(billFiltered\.value/)
  const bill = read('hrBill.ts')
  assert.match(bill, /b\.people - a\.people/)
  assert.match(bill, /b\.personalCents \+ b\.unitCents\) - \(a\.personalCents \+ a\.unitCents\)/)
})

test('第67轮：岗位汇总按花名册真实岗位取值（custom_data 优先于 position）', () => {
  const src = read('HrManagement.vue')
  // position 是「普工/操作员」职务口径，排最后
  const idx = src.indexOf('e.custom_data?.[\'岗位\']')
  assert.ok(idx > 0, '必须优先读 custom_data 岗位')
  const line = src.slice(idx, idx + 160)
  assert.match(line, /e\['岗位'\]/)
  assert.match(line, /e\.position/)
  assert.ok(line.indexOf('e.position') > line.indexOf('e[\'岗位\']'), 'position 必须是最后兜底')
  // 未匹配到岗位的人归入「未设置岗位」
  assert.match(src, /未设置岗位/)
})

test('第67轮：岗位聚合金额/人数与部门汇总守恒（2026-11 口径 74 人）', () => {
  // 构造 74 条真实口径数据：24 个岗位，个人=单位=310，合计 45,880 分/人
  const rows: any[] = []
  const posts = ['管退操作员', '束绞操作员', '主管', '检验员', '过磅员', '保安']
  for (let i = 0; i < 74; i++) {
    rows.push({
      证件号码: `51010119900101${String(1000 + i)}`,
      岗位: posts[i % posts.length],
      个人缴存: 310,
      单位缴存: 310,
    })
  }
  const groups = groupBillCents(rows, {
    keyOf: (r) => r.岗位,
    personalCentsOf: (r) => Math.round(Number(r.个人缴存) * 100),
    unitCentsOf: (r) => Math.round(Number(r.单位缴存) * 100),
    countPerson: (r) => !isAdjustmentRow(r),
  })
  assert.equal(groups.length, posts.length)
  assert.equal(groups.reduce((n, g) => n + g.people, 0), 74, '人数必须守恒')
  assert.equal(groups.reduce((n, g) => n + g.personalCents, 0), 74 * 31000)
  assert.equal(groups.reduce((n, g) => n + g.unitCents, 0), 74 * 31000)
  // 人数降序（同人数再按合计降序 / 名称）
  for (let i = 1; i < groups.length; i++) {
    assert.ok(groups[i - 1].people >= groups[i].people, '必须按人数降序')
  }
})

test('第67轮：补缴行只计金额不计人数（社保恒等口径）', () => {
  const rows = [
    { 证件号码: 'A', 岗位: '操作员', 个人缴存: 100, 单位缴存: 100 },
    { 证件号码: 'A', 岗位: '操作员', 个人缴存: 50, 单位缴存: 50, 是否补缴: '是' },
  ]
  const groups = groupBillCents(rows, {
    keyOf: (r) => r.岗位,
    personalCentsOf: (r) => r.个人缴存 * 100,
    unitCentsOf: (r) => r.单位缴存 * 100,
    countPerson: (r) => !isAdjustmentRow(r),
  })
  assert.equal(groups[0].people, 1)
  assert.equal(groups[0].personalCents, 15000)
})

test('第67轮：抽屉字段以 subs 为唯一驱动源（自定义字段不再被白名单丢弃）', () => {
  const src = read('HrManagement.vue')
  // 旧实现按静态契约过滤，自定义字段永远进不了抽屉
  assert.ok(!/orderedSubPairs\.value\.filter\(\(p\) => !!defOf\(p\)\)/.test(src),
    '不得再按静态契约白名单过滤抽屉字段')
  assert.ok(!/pairs = pairs\.filter\(\(p\) => !!defOf\(p\)\)/.test(src))
  // 未归属固定分组的新增字段进「自定义信息」
  assert.match(src, /groupLabelOf\(p\.key\) \|\| groupLabelOf\(p\.label\) \|\| '自定义信息'/)
  // 离职抽屉同样落到「自定义信息」
  assert.match(src, /groupOfName\.get\(name\) \|\| '自定义信息'/)
  // 自定义字段按字段配置的 data_type 渲染控件
  assert.match(src, /function hrDrawerTypeOfSub/)
  assert.match(src, /dataType: def \? hrDataTypeOf\(def\) : hrDrawerTypeOfSub\(sub\)/)
  // 社保正常/异常记录追加自定义字段
  assert.match(src, /for \(const sub of subsOfTab\('hr-social'\)\)/)
  // 保存按 subs.name 写回 custom_data（改名不丢数据）
  assert.match(src, /const name = p\.key \|\| p\.label/)
})

test('第67轮：人事设置恢复自由新增字段入口（自定义字段六同步闭环）', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /v-if="showFreeFieldAdd" class="field-config-add"/)
  // 人事同时是「引用分类」，必须与引用选择器并列渲染而不是互斥
  assert.match(src, /const showFreeFieldAdd = computed\(\(\) => !directMode\.value && \(props\.singleCategory \|\| !isRefCategory\.value\)\)/)
  // 手输字段名 → 继承当前胶囊分组（否则 subsOfTab 会把它过滤掉）
  assert.match(src, /const g = activeSubGroup\.value[\s\S]{0,300}group: g/)
})

test('第67轮：后端模板/导入按已保存 subs 动态生成，排除跨模块引用', () => {
  const tpl = fs.readFileSync(
    path.join(DIR, '..', '..', '..', '..', 'internal', 'handler', 'hr_template_dynamic.go'), 'utf8')
  // enabled=false / source 非空 / system / readonly 一律不进模板
  assert.match(tpl, /if !s\.Enabled \|\| s\.Source != nil \{/)
  assert.match(tpl, /s\.DataType == "system" \|\| s\.DataType == "readonly"/)
  // 顺序严格沿用 subs（字段设置里的拖动顺序）
  assert.match(tpl, /for _, s := range pick\.Subs \{/)
  // 导入解析同源
  const imp = fs.readFileSync(
    path.join(DIR, '..', '..', '..', '..', 'internal', 'handler', 'hr_import_modules.go'), 'utf8')
  assert.match(imp, /if fields, ok := hrTemplateFieldsFor\(ctx, h\.db, tid, scope, group\); ok \{/)
  assert.match(imp, /custom\[sp\.Header\] = v/)
})

test('第67轮：社保记录保存接受自定义字段并写入 custom_data', () => {
  const go = fs.readFileSync(
    path.join(DIR, '..', '..', '..', '..', 'internal', 'handler', 'hr_social_round56.go'), 'utf8')
  assert.match(go, /siRecordLocked := map\[string\]bool\{/)
  assert.match(go, /row\.CustomData\[key\] = strings\.TrimSpace\(toStringValue\(v\)\)/)
  // 前端把抽屉里非只读的自定义键一并提交
  const src = read('HrManagement.vue')
  assert.match(src, /const extra: Record<string, any> = \{\}/)
  assert.match(src, /\.\.\.extra,/)
})

test('第67轮：社保/公积金列顺序跟随字段配置（只重排不重建）', () => {
  const src = read('HrManagement.vue')
  // 骨架列按 subs 顺序重排，勾选列恒第一
  assert.match(src, /function orderColumnsBySubs\(cols: HrColumn\[\], subs: readonly any\[\]\): HrColumn\[\]/)
  assert.match(src, /const head = cols\.filter\(\(c\) => c\.colKey === 'row-select'\)/)
  // 三个入口都要走同一处排序：新增列合并后 / 公积金月度账单
  assert.match(src, /const merged = extra\.length \? \[\.\.\.fixed, \.\.\.extra\] : fixed\s*\n\s*return orderColumnsBySubs\(merged, subs\)/)
  assert.match(src, /return orderColumnsBySubs\(hrBillColumnDefs\(\), subsOfTab\('hr-fund'\)\)/)
  // 配置里没有的列（派生列）不得丢，排在已知列之后
  assert.match(src, /100000 \+ i/)
})
