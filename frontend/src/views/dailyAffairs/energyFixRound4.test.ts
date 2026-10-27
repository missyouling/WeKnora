import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 能耗管理第 4 轮真机问题的回归契约（2026-10-27 下午）。
 *
 * 这一轮的坑集中在一类：**框架事件签名与业务假设不一致导致的静默错误**，
 * 以及**硬编码兜底把用户删除的数据又变回来**。两者都不会在类型检查里暴露，
 * 只能靠源码契约钉住：
 *   ① TDesign 组件上的 .stop / .enter 修饰符 → 运行时 TypeError；
 *   ② 列表列派生用 FALLBACK 兜底 → 删掉的字段复活；
 *   ③ 居民读数把所有行的 kwh 相加 → 定比电量算成天文数字后被减成 0；
 *   ④ 勾选态被 buildRows 无条件清空 → 单选生成账单变成全量校验。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const readGo = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

// ---- ① 控制台报错：TDesign 组件上不得有 .stop / 键修饰符 ----

test('TDesign 组件上不得挂 @click.stop（t-input 的 click 首参是 { e } 而非原生事件）', () => {
  const files = [
    'FleetCertPanel.vue', 'ExtractRulePanel.vue', 'TenantSettingsPanel.vue',
    'MeterSettingsPanel.vue', 'UtilitiesManagement.vue', 'SourceFieldPicker.vue',
  ]
  for (const f of files) {
    const src = read(f)
    // 逐行扫：以 <t-input / <t-switch / <t-select / <t-checkbox 开头的标签片段里不能出现 .stop
    const offenders: string[] = []
    const tagRe = /<t-(input|switch|select|checkbox|textarea|input-number)\b[\s\S]{0,400}?\/?>/g
    for (const m of src.matchAll(tagRe)) {
      if (/@(click|mousedown|mouseup|change)\.stop/.test(m[0])) offenders.push(m[0].slice(0, 80))
    }
    assert.deepEqual(offenders, [], `${f} 的 TDesign 组件上仍有 .stop 修饰符`)
  }
})

test('t-input 上不得用 @keydown.xxx / @keyup.xxx（首参是输入值字符串），必须用 @enter', () => {
  const files = ['FleetCertPanel.vue', 'ExtractRulePanel.vue', 'SourceFieldPicker.vue', 'UtilitiesManagement.vue']
  for (const f of files) {
    const src = read(f)
    const tagRe = /<t-input\b[\s\S]{0,400}?\/?>/g
    for (const m of src.matchAll(tagRe)) {
      assert.ok(!/@key(down|up)\./.test(m[0]), `${f} 的 t-input 上仍有键修饰符：${m[0].slice(0, 80)}`)
    }
  }
  // SourceFieldPicker 必须改用 @enter
  assert.match(read('SourceFieldPicker.vue'), /@enter="pickFirst"/)
})

test('PdfSourceViewer 必须静默忽略 pdf.js 的取消异常', () => {
  const src = readFileSync(new URL('../../components/source-preview/PdfSourceViewer.vue', import.meta.url), 'utf8')
  assert.match(src, /function isAbortError/)
  assert.match(src, /Loading aborted/i)
  assert.match(src, /RenderingCancelledException/)
  // catch 里必须先判取消再打日志
  const catchIdx = src.indexOf('} catch (err) {', src.indexOf('async function load()'))
  const body = src.slice(catchIdx, catchIdx + 400)
  assert.ok(body.indexOf('isAbortError') < body.indexOf('console.error'), '取消异常必须先于 console.error 判断')
})

// ---- ② 六同步：删除字段后不得从硬编码兜底复活 ----

test('市电账单列派生不得用 FALLBACK_COLUMNS 兜底复活已删字段', () => {
  const src = read('UtilitiesManagement.vue')
  assert.ok(!/if \(!s\) return \{ \.\.\.fb \}/.test(src), 'subs 缺字段时不得回退硬编码列')
  assert.match(src, /subs 是唯一权威源/)
  // 派生入口必须是遍历 subs
  assert.match(src, /for \(const s of subs\) \{[\s\S]{0,400}?primary\.push\(/)
})

test('四个能耗视图的列存档必须清洗已不存在的 key（重置不得复活）', () => {
  for (const f of ['UtilitiesManagement.vue', 'SolarManagement.vue', 'UtilityMeterTab.vue', 'TenantBilling.vue']) {
    const src = read(f)
    assert.match(src, /const valid = .*filter\(.*columnDefs\.value\.some|const valid = .*filter\(.*activeColDefs\.value\.some/,
      `${f} 必须按当前列集过滤存档`)
    assert.match(src, /localStorage\.setItem\((COLUMN_STORAGE_KEY|STORAGE_KEY\.value|colStorageKey\.value)/,
      `${f} 必须把清洗后的存档回写`)
  }
})

// ---- ③ 定比电量：只取主行 bill_kwh，不能把各比例行的 kwh 累加 ----

test('定比电量取自主行的计费电量，而不是所有行 kwh 求和', () => {
  const src = read('TenantBilling.vue')
  const body = src.slice(src.indexOf('const residentInfoOf ='), src.indexOf('const waterReadingMap'))
  assert.ok(!/rows\.reduce\(\(s: number, r: any\) => s \+ \(Number\(r\.kwh\)/.test(body),
    '仍在对 residential_readings 的所有行 kwh 求和')
  assert.match(body, /const main = rows\.find/)
  assert.match(body, /main\?\.bill_kwh/)
})

// ---- ④ 生成账单：校验范围必须等于所选范围，且选中态不被重建清空 ----

test('buildRows 不得无条件清空勾选（否则单选生成退化成全量校验）', () => {
  const src = read('TenantBilling.vue')
  const body = src.slice(src.indexOf('const buildRows = () => {'), src.indexOf('const loadRecords ='))
  // 只匹配真实调用（行首缩进 + 不是注释），注释里提到旧实现不算违规
  const calls = body.split('\n').filter((l) => /^\s*clearSelection\(\)\s*$/.test(l))
  assert.deepEqual(calls, [], 'buildRows 里不得无条件 clearSelection')
  assert.match(body, /const alive = new Set\(out\.map/)
})

test('生成账单的校验只针对 generateTargets', () => {
  const src = read('TenantBilling.vue')
  const body = src.slice(src.indexOf('const handleGenerateAll = async'), src.indexOf('// 数据齐全时默认自动生成'))
  assert.match(body, /readinessOf\(targets\)/)
  assert.ok(!/readinessOf\(displayRows\.value\)/.test(body), '不得对全部行做校验')
})

// ---- ⑤ 上传弹窗：默认分类必须与选项 value 对得上 ----

test('上传弹窗要按 typeOptions 实际 value 解析默认分类', () => {
  const src = read('FleetUploadDialog.vue')
  assert.match(src, /function resolveDefaultType/)
  assert.ok(!/props\.scope \+ '__' \+ props\.defaultType/.test(src), '不得无条件硬拼 scope__name')
})

// ---- ⑥ 高级模板：必须幂等补 {{fields_schema}} ----

test('模板渲染前必须幂等补齐 {{fields_schema}} 插槽', () => {
  const svc = readGo('internal/application/service/extract_config_prompt.go')
  assert.match(svc, /func EnsureFieldsSchemaSlot/)
  assert.match(svc, /\{\{fields_schema\}\}/)
  // 5 个业务的构建函数都要经过它
  for (const f of [
    'knowledge_award_punish_extract.go', 'knowledge_contract_extract.go',
    'knowledge_energy_extract.go', 'knowledge_invoice_extract.go',
    'knowledge_regulation_extract.go',
  ]) {
    assert.match(readGo('internal/application/service/' + f), /EnsureFieldsSchemaSlot\(cfg\.PromptTemplate\)/, `${f} 未补插槽`)
  }
})

test('存量配置有幂等迁移补齐插槽（且只补不含插槽的记录）', () => {
  const up = readGo('migrations/versioned/000522_extract_prompt_fields_schema_slot.up.sql')
  assert.match(up, /\{\{fields_schema\}\}/)
  assert.match(up, /position\('\{\{fields_schema\}\}' in prompt_template\) = 0/, '必须只命中缺插槽的记录')
})

// ---- ⑦ 市电账单：居民电量明细分组已下线 ----

test('后端不再生产「电量明细（居民）」分组', () => {
  const src = readGo('internal/handler/utility.go')
  assert.ok(!/case "resident-meter":/.test(src))
  assert.ok(!/case "resident-meter-rows":/.test(src))
  assert.ok(!/"resident-meter"/.test(src))
})
