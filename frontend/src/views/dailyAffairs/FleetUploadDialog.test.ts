import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

import '../../registry'
import { getScope } from '../../registry'

const dialog = readFileSync(new URL('./FleetUploadDialog.vue', import.meta.url), 'utf8')

test('上传弹窗证照类型必选：删除「自动识别」，占位符为「请选择类型」', () => {
  assert.ok(!dialog.includes('自动识别'), '不再提供自动识别选项')
  assert.ok(!/t-select[^>]*clearable/.test(dialog), '证照类型选择框不可清空')
  // 占位文案由 registry 驱动（车队=证照类型，发票=发票类型）
  assert.match(dialog, /:placeholder="typeFieldLabel"/)
  assert.equal(getScope('vehicle')?.uploadTypeLabel, '证照类型')
  assert.equal(getScope('invoice')?.uploadTypeLabel, '发票类型')
})

test('上传提示文案明确必选：必须选择证照类型，按所选类型归类提取', () => {
  // 文案收进 registry：车队保留「必须选择…」口径，发票为「选择发票类型…」
  assert.equal(getScope('vehicle')?.uploadTypeTip, '必须选择证照类型，上传后按所选类型归类提取')
  assert.equal(getScope('invoice')?.uploadTypeTip, '选择发票类型，上传后按所选类型归档归类')
  assert.match(dialog, /uploadTypeTip/)
})

test('未选择证照类型时禁止添加文件并提示', () => {
  assert.match(dialog, /if \(!selectedType\.value\)/)
  assert.match(dialog, /MessagePlugin\.warning\(`请先选择\$\{typeFieldLabel\.value\}`\)/)
  assert.equal(`请先选择${getScope('vehicle')?.uploadTypeLabel}`, '请先选择证照类型')
  assert.ok(dialog.indexOf('!selectedType.value') < dialog.indexOf('const allowed'), '类型校验先于文件类型校验')
})

test('打开弹窗时继承父组件筛选的证照类型（defaultType）', () => {
  assert.match(dialog, /props\.defaultType/)
  // 不能无条件硬拼：父级可能给裸名（行驶证），选项 value 是复合值（vehicle__行驶证），
  // 直接赋值会让 t-select 选不中 → 弹窗里类型显示为空（真机问题）。
  assert.match(dialog, /selectedType\.value = resolveDefaultType\(\)/)
  assert.match(dialog, /function resolveDefaultType/)
})

test('上传任务携带所选证照类型：提取请求带 cert_type', () => {
  assert.match(dialog, /cert_type: certType/)
})

test('证照类型选项响应式：computed 依赖 props.typeOptions，分类加载/变更后实时更新（非一次性快照）', () => {
  assert.match(dialog, /const typeOptions = computed\(/)
  assert.match(dialog, /props\.typeOptions && props\.typeOptions\.length/)
})

test('任务进度实时上报：深度监听任务列表，emit progress 携带进行中任务', () => {
  assert.match(dialog, /\(e: 'progress', items: UploadProgressItem\[\]\)/)
  assert.match(dialog, /watch\(tasks, \(list\) => \{/)
  assert.match(dialog, /emitProgress\(\)/)
  assert.match(dialog, /stage: t\.status as UploadProgressItem\['stage'\]/)
  assert.match(dialog, /percent: Math\.round\(t\.progress\)/)
})

test('全部任务结束后自动关闭弹窗：延迟 emit update:visible false，进行中取消定时', () => {
  assert.match(dialog, /setTimeout\(\(\) => \{/)
  assert.match(dialog, /emit\('update:visible', false\)/)
  assert.match(dialog, /\}, 1200\)/)
  assert.match(dialog, /clearTimeout\(autoCloseTimer\)/)
  // 终态含 duplicate（重复上传）与 recovered（命中库内已有文件并自动恢复），
  // 二者也算"结束"，否则弹窗会永远停在最后一条重复任务上不关闭。
  assert.match(dialog, /list\.every\(\(t\) => t\.status === 'success' \|\| t\.status === 'failed' \|\| t\.status === 'duplicate' \|\| t\.status === 'recovered'\)/)
  // 仅真正新增/自动恢复时才通知父组件刷新，纯重复不扰动当前列表
  assert.match(dialog, /list\.some\(\(t\) => t\.status === 'success' \|\| t\.status === 'recovered'\)/)
})
