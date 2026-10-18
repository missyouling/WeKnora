import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const panel = readFileSync(new URL('./AiFieldSuggestPanel.vue', import.meta.url), 'utf8')
const cert = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

// 2026-10-18 需求 1+2：字段生成去弹窗（内嵌）、三分组四列、自动补建明细父字段。

test('字段生成是内嵌面板，不再有模态对话框与遮罩', () => {
  assert.doesNotMatch(panel, /<t-dialog/)
  assert.doesNotMatch(panel, /:close-on-overlay-click/)
  assert.match(panel, /<section v-if="visible" class="ai-field-panel"/)
})

test('内嵌面板锚点在「发票类型」下方、「字段配置」上方', () => {
  const formGridEnd = cert.indexOf('<AiFieldSuggestPanel')
  const fieldConfig = cert.indexOf('<div class="field-config">')
  const nameInput = cert.indexOf('v-model="editForm.name"')
  assert.ok(nameInput !== -1 && formGridEnd !== -1 && fieldConfig !== -1)
  assert.ok(nameInput < formGridEnd, 'AI 面板在分类名输入之后')
  assert.ok(formGridEnd < fieldConfig, 'AI 面板在字段配置之前')
})

test('编辑面板左下角不再有「智能生成字段」按钮（入口已移到分类行 ⋮ 菜单）', () => {
  assert.doesNotMatch(cert, /btn-ai-field/, '旧的按钮样式/类名必须消失')
  assert.doesNotMatch(cert, /@click="openAiFieldSuggest"/)
  assert.match(cert, /content: '字段生成', value: 'ai'/, '菜单项必须存在且改名为「字段生成」')
  assert.match(cert, /if \(value === 'ai'\) openAiPanel\(item\)/, '菜单必须能打开内嵌面板')
})

test('新建分类后自动展开编辑面板并打开 AI 生成区', () => {
  const idx = cert.indexOf('const created = res.data || res')
  assert.ok(idx !== -1)
  const after = cert.slice(idx, idx + 900)
  assert.match(after, /openAiPanel\(createdItem\)/, '新建成功后应展开 AI 面板')
})

test('识别结果固定三分组，且使用统一的五列网格', () => {
  assert.match(panel, /title: '标准字段'/)
  assert.match(panel, /title: '扩展字段'/)
  // 组名统一改「明细子项」：与「字段定义」里的明细子项叫法一致
  assert.match(panel, /title: '明细子项'/)
  // 五列：勾选 / 编码 / 名称 / 类型 / 样本
  assert.match(panel, /grid-template-columns: 22px minmax\(110px, 0\.9fr\) minmax\(110px, 1fr\) 96px minmax\(80px, 1fr\)/)
  // 三个分组固定展示（某组为空时给空态，不整块消失）
  assert.match(panel, /本样票未识别到该组字段/)
})

test('字段编码只读展示；字段名称与数据类型可编辑', () => {
  assert.match(panel, /<span class="afp-key"[^>]*>\{\{ r\.key \}\}<\/span>/, '编码必须是纯文本（不可编辑）')
  assert.match(panel, /<t-input v-model="r\.label"/, '中文名可改')
  assert.match(panel, /<t-select v-model="r\.data_type"/, '类型可改')
  assert.match(panel, /label: '数组', value: 'array'/, '类型选项含 array')
})

test('明细子项缺 items 父字段时静默补建，不再弹第二条提示 Toast', () => {
  // 旧实现：一条 info「已自动创建父字段」+ 一条 success「已并入 N 项」叠加，噪音大
  assert.doesNotMatch(panel, /当前分类没有「明细」字段/)
  assert.doesNotMatch(panel, /子列将无法保存/)
  assert.doesNotMatch(cert, /已自动创建「项目明细」父字段/, '父字段提示必须移除')
  assert.doesNotMatch(cert, /已并入 \$\{merged\.addedCount\} 项/, '并入计数 Toast 必须移除')
  assert.match(cert, /mergeAiFieldSuggestions/, '仍由合并层自动补建 items 宿主')
})

// 2026-10-18 实测补齐：内嵌面板高 1444px（含 276px textarea + 924px 结果表），
// 把下方「字段配置」和「保存」顶出视口；内嵌反而比弹窗更难用。两处收口：
test('识别成功后折叠输入区，避免 textarea 与上传区长期占高', () => {
  assert.match(panel, /const inputCollapsed = ref\(false\)/)
  assert.match(panel, /<template v-if="!inputCollapsed">/, '输入区必须可折叠')
  assert.match(panel, /if \(n\) inputCollapsed\.value = true/, '识别成功应自动折叠')
  assert.match(panel, /重新选择样票/)
  assert.match(panel, /inputCollapsed\.value = false/, 'resetState 必须复位折叠态')
})

test('结果区限高内滚，字段多时仍能看到下方字段配置', () => {
  assert.match(panel, /class="afp-scroll" tabindex="0"/)
  assert.match(panel, /max-height: min\(46vh, 420px\)/)
  assert.match(panel, /overflow-y: auto/)
  // flex 子项缺 min-height:0 时 max-height 形同虚设（此前"滚不动"的根因）
  assert.match(panel, /min-height: 0;/)
})

// ---- 2026-10-18 二轮打磨（9 项 UI/交互整改）----

test('标题统一为「字段生成」，并去掉右上角「收起」按钮', () => {
  assert.match(panel, /<span class="afp-title"><AiSparkIcon[^>]*\/> 字段生成<\/span>/)
  assert.doesNotMatch(panel, /智能字段生成/)
  assert.doesNotMatch(panel, />收起</, '右上角「收起」按钮必须删除')
  // 结果区仍有「取消」，避免用户无路可退
  assert.match(panel, /<t-button variant="outline" size="small" @click="onClose">取消<\/t-button>/)
})

test('「粘贴文本」与「选择文件」同尺寸并带图标，点击展开轻量输入框', () => {
  assert.match(panel, /<t-button variant="outline" size="medium" class="afp-paste-btn"/)
  assert.match(panel, /<template #icon><t-icon name="paste" \/><\/template>/, '补前置剪贴板图标')
  assert.match(panel, /粘贴文本/, '文案精简为「粘贴文本」')
  assert.doesNotMatch(panel, /粘贴票面文本/)
  // 按钮位于 t-upload 之后（并排），textarea 仍由 textMode 控制
  const upload = panel.indexOf('<t-upload')
  const paste = panel.indexOf('afp-paste-btn')
  assert.ok(upload !== -1 && paste !== -1 && upload < paste, '粘贴按钮在「选择文件」右侧')
  assert.match(panel, /<t-textarea v-if="textMode"/)
  // 容器 inline-flex + flex-start（t-upload 高度含下方提示文案，center 会让按钮低半行）+ 32px 同高
  assert.match(panel, /display: inline-flex;/)
  assert.match(panel, /align-items: flex-start;/)
  assert.match(panel, /align-self: flex-start;/)
  assert.match(panel, /height: 32px;/)
})

test('移除绿色长提示与三分组技术黑话', () => {
  assert.doesNotMatch(panel, /识别结果仅供参考，需你勾选并点「保存」后才会写入字段配置/)
  assert.doesNotMatch(panel, /映射到系统契约键，编码不可改/)
  assert.doesNotMatch(panel, /编码由系统生成且只读/)
  assert.doesNotMatch(panel, /afp-group-hint/)
})

test('主按钮改「保存字段」并在点击后直接调后端保存落库', () => {
  assert.match(panel, /保存字段/)
  assert.doesNotMatch(panel, /并入字段配置/)
  // 面板只发 apply，落库由父组件在 apply 里衔接 commitEdit
  const apply = cert.slice(cert.indexOf('function onAiFieldApply'), cert.indexOf('function onAiFieldApply') + 1400)
  assert.match(apply, /void commitEdit\(\)/, '并入后必须直接保存，不能要求用户再点一次')
})

test('AI 图标统一：菜单/标题/工具栏不再出现五角星', () => {
  assert.doesNotMatch(panel, /name="star"/)
  assert.doesNotMatch(cert, /StarIcon/)
  assert.match(cert, /import AiSparkIcon from '\.\/AiSparkIcon\.vue'/)
  assert.match(panel, /import AiSparkIcon from '\.\/AiSparkIcon\.vue'/)
})

test('「发票类型」右侧有 AI 图标开关，可切换展开/收起并高亮', () => {
  assert.match(cert, /class="form-item-inline"/)
  assert.match(cert, /class="ai-toggle-btn"/)
  assert.match(cert, /ai-toggle-btn--on/)
  assert.match(cert, /function toggleAiPanel\(\)/)
  assert.match(cert, /aiPanelOpen\.value = !aiPanelOpen\.value/)
})

test('保存字段后直接切到「提取规则」并只弹一条 Toast（不再有引导弹窗）', () => {
  // 2026-10-18 终轮：确认弹窗被移除——字段已落库，下一步必然是配规则
  assert.doesNotMatch(cert, /InvoiceRulesGuide/)
  assert.doesNotMatch(cert, /rulesGuide/)
  assert.doesNotMatch(cert, /DialogPlugin/)
  const fn = cert.slice(cert.indexOf('function goRulesAfterAiSave'), cert.indexOf('function goRulesAfterAiSave') + 600)
  assert.match(fn, /emit\('ai-guide'/, '直接冒泡跳转事件')
  assert.ok(fn.indexOf('cacheSampleText') < fn.indexOf("emit('ai-guide'"), '先缓存样票再跳转')
  // 单条 Toast：AI 路径下不再叠加「已保存」
  assert.match(cert, /字段已保存，已为您切换至提取规则/)
  assert.match(cert, /if \(!aiContext\) MessagePlugin\.success\('已保存'\)/)
})
