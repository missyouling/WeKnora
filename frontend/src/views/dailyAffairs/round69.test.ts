/**
 * 第 69 轮单测：
 *   ① 抽屉自适应排版（同行优先 / 长文案换行 / 2~3 列容器查询 / 最小宽度）
 *   ② 拖拽锚点贴抽屉左边框（不再是 fixed + 推算 right）
 *   ③ 设置抽屉默认胶囊对齐当前子视图
 *   ④ 「全选」与「重置」语义分离，重置基准唯一（后端 is_default）
 *   ⑤ 离职引用字段元数据 + 来源胶囊（前后端契约）
 *   ⑥ 账单变动红点 + 批量重新生成（三模块接口各自正确）
 *   ⑦ 明细弹窗高度链逐级闭合（滚动条恢复）
 *   ⑧ Toast 统一右下角
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fromSubsPayload, toSubsPayload } from './fleetCategorySubs'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (f: string) => fs.readFileSync(path.join(DIR, f), 'utf8')
// DIR = frontend/src/views/dailyAffairs → 回三级到 frontend
const readSrc = (rel: string) => fs.readFileSync(path.join(DIR, '../../../', rel), 'utf8')

test('第69轮：抽屉同行优先 + 长文案按需换行（不是一刀切置顶）', () => {
  const src = read('BusinessSplitDetailDrawer.vue')
  // 同行：item 横向 flex + label 固定 84px 右对齐
  assert.match(src, /\.bsd-fields :deep\(\.t-form__item\) \{[\s\S]{0,160}flex-direction: row;[\s\S]{0,200}margin-bottom: 0 !important;/)
  assert.match(src, /flex: 0 0 84px;/)
  assert.match(src, /text-align: right;/)
  // 长标签单独换行（>=7 字或含单位括号）
  assert.match(src, /const LONG_LABEL_MIN = 7/)
  assert.match(src, /'bsd-field--long-label': isLongLabel\(f\)/)
  assert.match(src, /\.bsd-field--long-label > \.t-form__label[\s\S]{0,200}justify-content: flex-start;/)
  // 容器查询 2 列 → 3 列 → 极窄单列
  assert.match(src, /container-type: inline-size;/)
  assert.match(src, /@container \(min-width: 1020px\)[\s\S]{0,120}repeat\(3, minmax\(0, 1fr\)\)/)
  assert.match(src, /@container \(max-width: 600px\)/)
  // 紧凑间距：row-gap 10、分组标题 14/8
  assert.match(src, /row-gap: 10px;/)
  assert.match(src, /margin: 14px 0 8px;/)
  // 最小宽度 680，可拉到三列
  assert.match(src, /minWidth: 680/)
  assert.match(src, /maxWidth: 1400/)
  assert.match(src, /:min-width="minWidth" :max-width="maxWidth"/)
})

test('第69轮：拖拽热区挂到抽屉面板内、贴左边框（不再 fixed 推算）', () => {
  const src = readSrc('src/components/settings/SettingDrawer.vue')
  // teleport 目标是面板自身
  assert.match(src, /<teleport v-if="handleHost && drawerVisible && resizable && !maximized" :to="handleHost">/)
  assert.match(src, /const resolveHandleHost = \(\): HTMLElement \| null/)
  assert.match(src, /querySelector\(`\[data-setting-drawer-id="\$\{drawerDomId\}"\]`\)/)
  // 绝对定位（面板是 absolute，天然是包含块）。
  // 第 74 轮起几何与上游知识库文档详情抽屉 1:1：热区 12px、left:-12px（整块落在左缘外）。
  assert.match(src, /\.setting-drawer-resize-handle \{[\s\S]{0,200}position: absolute;/)
  assert.match(src, /position: absolute;\s*\r?\n\s*left: -12px;\s*\r?\n\s*top: 0;\s*\r?\n\s*bottom: 0;\s*\r?\n\s*width: 12px;/)
  assert.match(src, /z-index: 20;/)
  // 不再用固定的 right 推算，也不再凭 drawerWidthPx 起步
  assert.ok(!/right: `\$\{drawerWidthPx\}px`/.test(src), '禁止再用推算的 right 值')
  assert.match(src, /resizeStartWidth = measured > 0 \? clampWidth\(measured\) : drawerWidthPx\.value/)
})

test('第69轮：设置抽屉默认激活当前子视图胶囊（花名册在职/离职不再错位）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const settingsInitialGroup = computed\(\(\) => \{[\s\S]{0,420}rosterStatusView\.value === '离职' \? '离职人员' : '在职人员'/)
  assert.match(src, /:initial-group="settingsInitialGroup"/)
})

test('第69轮：全选与重置分离，重置基准唯一（后端 is_default）', () => {
  const src = read('HrManagement.vue')
  // 全选 = 所有启用字段
  // 行尾兼容 CRLF（Windows 检出后同步到 VM 的文件是 \r\n）
  assert.match(src, /function selectAllColumns\(\) \{\r?\n {2}const allKeys = businessColumns\.value\.map\(\(c\) => c\.colKey\)/)
  assert.ok(!/function selectAllColumns\(\) \{ resetColumns\(\) \}/.test(src), '全选不得复用重置')
  // 重置只认 configKeys.defaults（后端已落库的 enabled && is_default）
  assert.match(src, /const defaults = configKeys\.value\.defaults\.length/)
  // 保存后走同一个 resetColumns，不再维护第二套重建逻辑
  assert.match(src, /await loadFieldConfig\(activeTab\.value\)[\s\S]{0,1400}resetColumns\(\)\s*\r?\n\s*await loadList/)
})

test('第69轮：离职引用字段带 ref_source 元数据并可往返', () => {
  const raw = [
    { name: '工号', enabled: true, is_default: true, data_type: 'text', group: '在职人员' },
    {
      name: '司龄', enabled: false, is_default: false, data_type: 'text',
      group: '离职人员', ref_source: '在职人员', source_module: 'roster_active',
    },
  ]
  const parsed = fromSubsPayload(raw)
  const resign = parsed.find((f) => f.name === '司龄')!
  assert.equal(resign.refSource, '在职人员')
  assert.equal(resign.sourceModule, 'roster_active')
  assert.equal(resign.enabled, false)
  assert.equal(resign.isDefault, false)
  // 往返不丢
  const back = toSubsPayload(parsed)
  const backSub = back.find((s) => s.name === '司龄')!
  assert.equal(backSub.ref_source, '在职人员')
  assert.equal(backSub.source_module, 'roster_active')
  assert.equal(backSub.is_default, false)
})

test('第69轮：设置面板渲染「在职人员」来源胶囊且仍可编辑名称', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /v-else-if="row\.fd\.refSource" class="fc-name-wrap fc-name-wrap--ref"/)
  assert.match(src, /<span class="fc-ref-badge fc-ref-badge--view" data-fc-ref-source>\{\{ row\.fd\.refSource \}\}<\/span>/)
  // 名称输入框仍可编辑（ref_source 不是跨分类取值，不做只读）
  assert.match(src, /fc-ref-badge--view[\s\S]{0,400}<t-input :model-value="fieldEditOf\(row\.fd\)"/)
})

test('第69轮：后端给离职胶囊的在职字段打 ref_source 标记', () => {
  const src = fs.readFileSync(path.join(DIR, '../../../../internal/handler/hr_round68_resign.go'), 'utf8')
  assert.match(src, /mirror\.RefSource = hrRosterGroupActive/)
  assert.match(src, /mirror\.SourceModule = "roster_active"/)
  // 历史合并的字段也要补标记（幂等），但不动已有 Source
  assert.match(src, /if core\[strings\.TrimSpace\(sub\.Name\)\] \|\| sub\.Source != nil \{[\s\S]{0,120}continue/)
})

test('第69轮：账单周期用小圆点表达变动，不再挤压文字', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /<span class="dirty-dot" data-hr-bill-dirty-flag aria-label="原始记录已变动" \/>/)
  assert.ok(!/class="hr-dirty-flag"/.test(src), '旧的挤压文字标签必须删除')
  assert.match(src, /\.dirty-dot \{[\s\S]{0,160}width: 7px;[\s\S]{0,80}height: 7px;[\s\S]{0,80}border-radius: 50%;[\s\S]{0,120}margin-right: 6px;/)
  assert.match(src, /\.hr-bill-month-text \{ white-space: nowrap; \}/)
})

test('第69轮：勾选浮动栏支持批量重新生成（三模块各自接口）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /v-if="toolbarShowGenerateBill"[\s\S]{0,400}@confirm="handleRegenerateBills"/)
  assert.match(src, /data-hr-regenerate-bills/)
  assert.match(src, /确定重新生成所选 \$\{selectedRowKeys\.length\} 个月份的账单吗？/)
  // 三模块分别调用正确的生成接口
  assert.match(src, /await hrApi\.createSocialBill\(\{ month_label: m, overwrite: true \}\)/)
  assert.match(src, /await hrApi\.createFundBill\(\{ month_label: m, overwrite: true \}\)/)
  assert.match(src, /await hrApi\.generateDormCharges\(\{ month: m, overwrite: true \}\)/)
  // 成功后清选择、刷新列表、单条汇总提示
  assert.match(src, /已重新生成 \$\{label\} 账单/)
})

test('第69轮：明细弹窗高度链逐级闭合（滚动条恢复可用）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /\.hr-bill-table-wrap \{[\s\S]{0,200}min-height: 0;[\s\S]{0,80}overflow: hidden;/)
  assert.match(src, /\.hr-bill-table-wrap > \* \{\s*\r?\n\s*flex: 1 1 0;\s*\r?\n\s*min-height: 0;\s*\r?\n\s*height: auto !important;/)
  // 末尾不得再有旧简写覆盖（同优先级后者生效 → 会把 basis 改回 auto）
  assert.equal(src.match(/\.hr-bill-table-wrap \{/g)?.length, 1, 'wrap 规则只能有一处')
  assert.match(src, /\.hr-bill-detail \{[\s\S]{0,400}overflow: hidden;/)
  // 滚动条滑块可见（占位式 10px）
  assert.match(src, /\.hr-bill-detail \.detail-table-body::-webkit-scrollbar \{ width: 10px; height: 10px; \}/)
  assert.match(src, /::-webkit-scrollbar-thumb \{/)
})

test('第69轮：Toast 统一右下角、分级时长且不遮挡顶部卡片', () => {
  const src = readSrc('src/utils/toastDefaults.ts')
  // TDesign 第二参是 duration 数字，placement/offset 必须进 params 对象
  assert.match(src, /const PLACEMENT = 'bottom-right' as const/)
  assert.match(src, /const OFFSET: \[number, number\] = \[-20, -20\]/)
  assert.match(src, /placement: \(extra\.placement as string\) \|\| PLACEMENT/)
  assert.match(src, /offset: \(extra\.offset as \[number, number\]\) \|\| OFFSET/)
  assert.match(src, /success: 2500/)
  assert.match(src, /warning: 4000/)
  assert.match(src, /closeBtn: extra\.closeBtn \?\? \(kind === 'warning' \|\| kind === 'error'\)/)
  // 必须覆盖 $message 出口，否则模板里的调用仍弹顶部
  assert.match(src, /if \(app\) app\.config\.globalProperties\.\$message = MessagePlugin/)
  assert.match(readSrc('src/main.ts'), /installToastDefaults\(app\);/)
  const css = readSrc('src/assets/theme/toast.less')
  assert.match(css, /max-width: 400px;/)
  assert.match(css, /white-space: normal;/)
})
