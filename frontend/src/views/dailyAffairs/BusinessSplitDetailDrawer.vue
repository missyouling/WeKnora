<!--
  BusinessSplitDetailDrawer —— 详情抽屉中台组件（蓝图 P1-5，单栏重构）。

  布局（单栏垂直流，标准抽屉宽度 ~654px）：
    1. 顶部：提取质量溯源面板（ExtractQualityPanel，无告警/补洞时自动隐藏）；
    2. 中部：双列网格动态字段表单（标准字段 + 扩展字段）；
    3. 中下部：项目明细子项只读表格（仅 supportsSubFields 且存在明细时展示）；
    4. **最底部**：源文件预览（可折叠，**折叠时（默认）不挂载** DocumentPreview，避免
       PDF/图片渲染器初始化；展开时在底部原地展开并按物理页码 pageNo 定位）。

  契约：
    - 表单状态由父级持有（`form`），组件只做绑定与展示；
    - 字段一律按 `DetailFieldMeta[]` 渲染（dataType: text/date/number/array/items），
      items 类型字段不在此处渲染（由下方明细表格区块承载），杜绝「明细见下方表格」占位行；
    - 业务特有控件（类型下拉/状态标签/标签墙）通过 `#field-<name>` 具名插槽注入；
    - 组件内严禁出现任何业务 scope 分支，预览页码、明细标题等全部走 props。

  Slots：title / field-<name>（按字段名覆盖单个控件）/ extra（表单尾部）/ footer-left
-->
<template>
  <!-- 第 69 轮：业务抽屉最小 680px（保证一行两列），可拖到 1400px（三列自适应） -->
  <SettingDrawer v-model:visible="innerVisible" :title="title" :width="drawerWidth" :storage-key="storageKey"
    :min-width="minWidth" :max-width="maxWidth" hide-footer destroy-on-close class="bsd-drawer">
    <template #title>
      <slot name="title">
        <span class="detail-title-main">{{ title }}</span>
        <!-- 物理页码胶囊：与列表 P 标同一套绿底样式。此前用 row-page-badge，
             而该类只在父页面 scoped 样式里定义、命中不到子组件，渲染成裸文本，
             标题变成「发票详情 · 26507903330500014563P9」。 -->
        <span v-if="pageNo >= 1" class="bsd-page-badge">P{{ pageNo }}</span>
      </slot>
    </template>

    <!-- 单栏垂直流：自顶部向下依次为 质量 / 表单 / 明细 / 源预览 -->
    <div class="bsd-body">
      <ExtractQualityPanel v-if="quality" :quality="quality" :label-of="labelOf" compact />

      <!-- 第 58 轮：有 groups / 无 groups 归一化为**同一条**渲染链。
           `normalizedSections` 把「无分组」的 fields 包装成单组，
           表单网格与全部控件分支因此只维护一份 —— 此前 v-if / v-else 两套模板
           近乎逐行重复（约 90 行），改一处漏一处，是六同步漂移的温床。 -->
      <section v-for="sec in normalizedSections" :key="sec.key" class="bsd-block">
        <div class="bsd-block-title">{{ sec.label }}</div>
        <!-- 第 69 轮：容器查询锚点（inline-size 容器只能被子孙查询，不能查自己） -->
        <div class="bsd-fields-wrap">
          <div class="bsd-fields" :class="fieldsClass">
          <template v-for="f in sec.fields" :key="f.name">
            <t-form-item :label="f.label" label-align="top" :class="fieldClassOf(f)">
              <slot :name="`field-${f.name}`" :field="f" :form="form">
                <!-- 只读模式（费用清单等「查看型」抽屉）：所有字段只展示不可改，
                     也不出现保存按钮。费用清单的值是核算结果，编辑只能走「生成/重算」，
                     在查看抽屉里改等于绕过核算口径。 -->
                <div v-if="readonly" class="bsd-readonly-value" :class="{ 'is-long': isLongType(f) }">
                  {{ displayValueOf(f) }}
                </div>
                <!-- 第 55 轮：状态类字段直接渲染彩色徽标，不再是灰底禁用输入框 -->
                <!-- 第 64 轮：宿舍/住宿/资产/账单/维修状态改**纯带色文字**（无 Tag 外框），
                     抽屉里绝不能出现输入框或描边方框（线上「黑框」问题）。 -->
                <span v-else-if="isStatusField(f)" class="bsd-status-text"
                  :class="statusTextClassOf(f.name, form.data[f.name])">
                  {{ form.data[f.name] || '—' }}
                </span>
                <!-- 只读派生字段（年龄 / 工龄 / 系统状态）：disabled 展示，不参与手填 -->
                <t-input v-else-if="f.readonly" :value="String(form.data[f.name] ?? '')" disabled />
                <!-- 备注片段字段：无标准存储位，值存 remark 的「字段名：值」片段 -->
                <t-textarea v-else-if="f.remarkPart && isLongType(f)" v-model="form.remarkParts[f.remarkPart]"
                  :autosize="{ minRows: 2, maxRows: 5 }" placeholder="每行一条" />
                <t-input v-else-if="f.remarkPart" v-model="form.remarkParts[f.remarkPart]" />
                <t-date-picker v-else-if="f.dataType === 'date'" v-model="form.data[f.name]" value-type="YYYY-MM-DD"
                  format="YYYY-MM-DD" clearable allow-input />
                <!-- 第 65 轮：月份型字段强制月份选择器，禁止手输 53q / 5353 -->
                <t-date-picker v-else-if="f.dataType === 'month'" v-model="form.data[f.name]"
                  mode="month" value-type="YYYY-MM" format="YYYY-MM" :allow-input="false" clearable />
                <!-- 第 65 轮：整数型字段（床位数 / 数量 / 入住天数）只允许非负整数 -->
                <t-input-number v-else-if="f.dataType === 'integer'" v-model="form.data[f.name]"
                  :min="0" :decimal-places="0" :allow-input="false" theme="normal" />
                <!-- 第 65 轮：金额 / 小数型字段（押金 / 单价 / 面积 / 资产价值）
                     受控数字控件：只接受数字与小数点，非负、最多 4 位小数。
                     t-input-number 自带非法字符拦截，用户无法输入中文/字母/符号。 -->
                <t-input-number v-else-if="f.dataType === 'number'" v-model="form.data[f.name]"
                  :min="0" :decimal-places="4" :allow-input="false" theme="normal" />
                <!-- 第 52 轮：内置枚举字段统一走可搜索下拉（选项由业务字段契约注入） -->
                <t-select v-else-if="f.options && f.options.length" v-model="form.data[f.name]"
                  clearable filterable placeholder="请选择">
                  <t-option v-for="opt in f.options" :key="opt" :label="opt" :value="opt" />
                </t-select>
                <!-- 第 70 轮：候选为空时**仍然是禁用下拉**，绝不退化成白底输入框。
                     场景：宿舍/公租房暂无在住人员时的「管理员/舍长」、暂无可用表计的关联项。 -->
                <t-select v-else-if="f.selectLike" :model-value="undefined" disabled
                  :placeholder="f.emptyPlaceholder || '暂无可选项'" />
                <!-- 多行文本（字段定义里显式选择「多行文本」）：多行编辑区域，
                     支持换行与自动折行，长内容完整录入与展示不截断。
                     用户显式选择的类型优先于下方的字段名兜底。 -->
                <t-textarea v-else-if="f.dataType === 'textarea'" v-model="form.data[f.name]"
                  :autosize="{ minRows: 2, maxRows: 6 }" />
                <!-- 摘要 / 多行长文本兜底：字段名命中白名单但 data_type 仍是 text
                     （存量数据未迁移 / 用户改回单行前的过渡态）时也用文本域，
                     避免长摘要被单行输入框截断。必须放在 array 分支**之前** ——
                     验收期曾把 summary 的 data_type 误改成 array，落到 tag-input
                     后被顿号拆成 4 个带 × 的灰色标签，与知识库详情里的段落摘要完全两样。 -->
                <t-textarea v-else-if="isLongTextField(f)" v-model="form.data[f.name]"
                  :autosize="{ minRows: 2, maxRows: 5 }" />
                <!-- 数组类型（字段定义里把类型改成「数组」）：回车追加多值标签。
                     存量值可能是字符串（改类型前保存的「甲\n乙」），由 normalizeArrayValue
                     按换行/分号/顿号拆成 string[]，避免整段文本被当成单个标签。 -->
                <t-tag-input v-else-if="f.dataType === 'array'" :value="arrayValueOf(f.name)"
                  @change="(v: any) => (form.data[f.name] = normalizeArrayValue(v))"
                  placeholder="输入后回车添加" clearable />
                <t-input v-else v-model="form.data[f.name]" />
              </slot>
            </t-form-item>
          </template>
          </div>
        </div>
      </section>

      <!-- 明细子项：只读表格（编辑走列表/浮层，避免抽屉里再叠一层可编辑表格） -->
      <section v-if="supportsSubFields && itemRows.length" class="bsd-block">
        <div class="bsd-block-title">{{ itemLabel }}</div>
        <div class="bsd-items">
          <table class="bsd-items-table">
            <thead>
              <tr>
                <th v-for="sf in itemCols" :key="sf.key">{{ sf.label || sf.key }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(it, i) in itemRows" :key="i">
                <td v-for="sf in itemCols" :key="sf.key" :class="itemCellClass(sf)">{{ formatItemCell(it, sf) }}</td>
              </tr>
            </tbody>
            <!--
              合计行（第 24 轮任务三）：与列表悬停浮层（BusinessSubItemsPopover）
              完全同口径 —— 两者都调 itemTotals，保证「浮窗看到的合计 == 抽屉里的合计」。
              首列写「合计」，金额/税额列给精确汇总，其余列留空（不可累加）。
            -->
            <tfoot v-if="hasSumCols">
              <tr class="bsd-items-total">
                <td v-for="(txt, i) in totalRow" :key="i" :class="itemCellClass(itemCols[i])">{{ txt }}</td>
              </tr>
            </tfoot>
          </table>
        </div>
      </section>

      <slot name="extra" />

      <!-- 最底部：源文件预览，默认折叠（不挂载重型资源），展开原地展开 -->
      <section v-if="showPreview !== false" class="bsd-preview-block">
        <button type="button" class="bsd-preview-toggle" :title="previewExpanded ? '收起源文件预览' : '展开源文件预览'"
          @click="previewExpanded = !previewExpanded">
          <t-icon :name="previewExpanded ? 'chevron-down' : 'chevron-right'" size="14px" />
          <span>{{ previewExpanded ? '收起源文件预览' : '展开源文件预览' }}</span>
        </button>
        <div v-if="previewExpanded" class="bsd-preview-pane">
          <DocumentPreview v-if="row" :knowledge-id="row.knowledgeId" :page="pageNo"
            :file-type="row.fileType || ''" :file-name="row.fileName || ''" :active="true" />
        </div>
      </section>
    </div>

    <div class="bsd-footer">
      <slot name="footer-left" />
      <t-popconfirm v-if="showDelete && !readonly" content="确定删除该记录吗？该操作不可恢复" @confirm="emit('delete')">
        <t-button variant="outline" size="small" theme="danger">删除</t-button>
      </t-popconfirm>
      <t-button v-if="showReExtract && !readonly" variant="outline" size="small" @click="emit('re-extract')">重新提取</t-button>
      <t-button variant="outline" size="small" @click="close">{{ readonly ? '关闭' : '取消' }}</t-button>
      <t-button v-if="!readonly" theme="primary" size="small" :loading="saving" :disabled="dirty === false"
        @click="emit('save')">保存</t-button>
    </div>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import DocumentPreview from '@/components/document-preview.vue'
import ExtractQualityPanel from './ExtractQualityPanel.vue'
import { formatItemCell, itemCellClass } from './businessFormat'
import { hasSumColumn, totalRowTexts } from './itemTotals'
import { normalizeArrayValue, type DetailFieldMeta } from './businessDetailForm'
import { formatCellDateTime } from './formatDateTime'
import { STATUS_TEXT_FIELDS, statusTextClassOf } from './hrStatusStyle'

const props = withDefaults(defineProps<{
  visible: boolean
  title?: string
  /** 当前行（提供 knowledgeId / fileName / fileType 供预览） */
  row?: any
  /** 物理页码（预览跳转用；缺省回退 1） */
  /** 第 48 轮：按分组渲染（花名册 11 分组），为空时退回单块渲染 */
  groups?: { key: string; label: string; fields: any[] }[]
  pageNo?: number
  /** 表单状态（父级持有，组件只绑定） */
  form: any
  fields?: DetailFieldMeta[]
  /** 明细行（form.items）；子列定义 */
  subFields?: any[]
  supportsSubFields?: boolean
  itemLabel?: string
  /** 提取质量（ExtractQualityPanel；无则静默） */
  quality?: any
  labelOf?: (f: string) => string
  /** 脏检查：false 时禁用保存；不传则不禁用 */
  dirty?: boolean
  saving?: boolean
  blockTitle?: string
  width?: string
  storageKey?: string
  showDelete?: boolean
  showReExtract?: boolean
  /**
   * 只读模式：字段全部以纯文本展示，底栏不出现「保存」。
   * 用于费用清单这类「核算结果查看」抽屉（编辑只能走生成/重算）。
   */
  readonly?: boolean
  /**
   * 是否渲染底部「源文件预览」区块。
   * 费用清单/核算表是**计算行**，没有源文件可预览，硬留一个空折叠条会让人点了没反应。
   */
  showPreview?: boolean
  /**
   * 第 64 轮：固定两列网格。
   *
   * 默认仍是 `auto-fit minmax(200px,1fr)`（发票/合同等窄抽屉自适应）；
   * 人事全模块抽屉统一 820px 后必须固定两列 —— auto-fit 在 820px 下会落成 3 列，
   * 每列输入控件只剩 150px 左右，18 位身份证号与公司全称被截断（线上问题 1 的根因）。
   */
  twoColumn?: boolean
  /** 第 69 轮：拖拽调宽的下限（px）。业务抽屉一律 680（保证一行两列）。 */
  minWidth?: number
  /** 上限（px）：允许拉宽到三列布局。 */
  maxWidth?: number
}>(), {
  title: '',
  row: null,
  pageNo: 0,
  fields: () => [],
  subFields: () => [],
  supportsSubFields: false,
  itemLabel: '明细',
  quality: undefined,
  labelOf: undefined,
  dirty: undefined,
  saving: false,
  blockTitle: '记录信息',
  width: '654px',
  storageKey: 'weknora-business-detail-drawer-width',
  showDelete: false,
  showPreview: true,
  readonly: false,
  showReExtract: false,
  twoColumn: false,
  minWidth: 680,
  maxWidth: 1400,
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'save'): void
  (e: 'delete'): void
  (e: 're-extract'): void
  (e: 'cancel'): void
}>()

const innerVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})
// 单栏标准宽度；不再随预览展开加宽（预览置底，不影响表单列宽）
const drawerWidth = computed(() => props.width)
// 源预览默认折叠：展开才挂载 DocumentPreview（重资源，不能随抽屉一起初始化）
const previewExpanded = ref(false)
watch(() => props.row?.knowledgeId, () => { previewExpanded.value = false })

// 表单由父级持有：这里用 computed 代理（读写都落在父级的同一个对象上），
// 既保持"保存链路在父级"的单一职责，也避免组件直接改写 prop。
const form = computed<any>(() => props.form || { data: {}, remarkParts: {}, items: [] })
/** 只读展示值：数组拼顿号、数字按原值、空值显示 —（避免空白行看起来像丢数据）。 */
function displayValueOf(f: any): string {
  const v = f?.remarkPart ? form.value?.remarkParts?.[f.remarkPart] : form.value?.data?.[f.name]
  if (v === null || v === undefined || v === '') return '—'
  if (Array.isArray(v)) return v.length ? v.join('、') : '—'
  if (typeof v === 'object') return JSON.stringify(v)
  // 时间/日期字段统一格式化：不允许 ISO8601 原串（T + 微秒 + +08:00）出现在详情里。
  const label = String(f?.label || f?.name || '')
  if (f?.dataType === 'date' || /时间|日期/.test(label)) {
    return formatCellDateTime(label, v) || '—'
  }
  return String(v)
}

const itemRows = computed<any[]>(() => (Array.isArray(form.value.items) ? form.value.items : []))
// 子列：未配置时按第一行实际键生成（不写死任何业务字段集）
const itemCols = computed<any[]>(() => {
  if (props.subFields?.length) return props.subFields
  const keys = new Set<string>()
  for (const it of itemRows.value) for (const k of Object.keys(it || {})) if (k !== 'extra') keys.add(k)
  return [...keys].map((k) => ({ key: k, label: k }))
})
// 合计行（第 24 轮任务三）：判据与求和都走 itemTotals，
// 与列表悬停浮层（BusinessSubItemsPopover）共用同一实现 —— 两处数字必须一致。
const hasSumCols = computed(() => hasSumColumn(itemCols.value))
const totalRow = computed(() => totalRowTexts(itemRows.value, itemCols.value))

// 主表单只渲染真实字段：items 类型（明细容器）交由下方明细表格承载，绝不生成占位行
const formFields = computed<DetailFieldMeta[]>(
  () => (props.fields || []).filter((f) => f.dataType !== 'items' && f.name !== 'items' && f.label !== '项目明细'),
)

/**
 * 第 58 轮：模板归一化的唯一数据源。
 *
 * - 有 groups：按分组分块渲染（花名册 11 分组 / 社保多胶囊）；
 * - 无 groups：把 fields 包装成「单组」，label 用 blockTitle（默认「记录信息」），
 *   与旧单块模板的视觉完全一致，但不再复制一整套控件分支。
 */
const normalizedSections = computed<{ key: string; label: string; fields: DetailFieldMeta[] }[]>(() => {
  if (props.groups && props.groups.length) {
    return props.groups.map((g, i) => ({
      key: String(g.key || `group-${i}`),
      label: String(g.label || ''),
      fields: (g.fields || []) as DetailFieldMeta[],
    }))
  }
  return [{ key: 'single', label: props.blockTitle, fields: formFields.value }]
})
const isLongType = (f: DetailFieldMeta) => f.dataType === 'array'

/**
 * 第 70 轮：状态字段名单与配色统一收口到 `hrStatusStyle.ts`。
 *
 * 旧实现这里维护一份白名单 + 一份 `if/else`，列表侧另有 4 个 theme 函数，
 * 于是「合同 `到期` 抽屉里是黑字」「回盘表五险状态在抽屉渲染成 disabled 输入框」
 * 这类不一致反复出现。现在名单与映射只有一份。
 */
const isStatusField = (f: DetailFieldMeta): boolean =>
  STATUS_TEXT_FIELDS.has(String(f.name || '').trim())

// 摘要 / 多行长文本字段：与知识库文档摘要 100% 同源同形（段落文本域）。
// 按字段名与中文名双路判定，不受 data_type 残留值影响。
const LONG_TEXT_FIELD_KEYS = new Set(['summary', 'abstract', 'description'])
const LONG_TEXT_FIELD_LABELS = new Set(['摘要', '内容摘要', '概述', '说明'])
const isLongTextField = (f: DetailFieldMeta): boolean => {
  const name = String(f?.name || '').trim()
  const label = String(f?.label || '').trim()
  return LONG_TEXT_FIELD_KEYS.has(name) || LONG_TEXT_FIELD_LABELS.has(label)
}
/**
 * 数组控件的值：存量字符串按换行/分号/顿号拆成 string[]。
 * 不做 computed 缓存——它随 form.data 的编辑实时变化，直接函数调用最省心
 * （t-tag-input 的 :value 每次重渲染都会取最新值）。
 */
const arrayValueOf = (name: string): string[] => normalizeArrayValue(form.value.data?.[name])
/**
 * 第 68 轮：字段 Label 一律置顶（`label-align="top"`），不再按最长标签算左栏宽度。
 *
 * 左右排版时短标签（如「岗位」）与输入框之间会留下一大片空白，18 位身份证号
 * 还会被挤压截断；改为上下结构后，每个控件独占所在栅格列的 100% 宽度。
 * 原 `labelWidthPx`（左栏宽度）契约已删除 —— 上下结构下它没有意义。
 */
/** 第 64 轮：长字段/多选/附件/状态说明类一律独占整行。 */
const FULL_WIDTH_FIELDS = new Set([
  // 第 68 轮：收紧到「真正需要整行」的地址 / 多行说明类。
  // 旧集合把 工作邮箱 / 个人邮箱 / 服装尺寸 / 户口性质 / 手机号码 也塞进整行，
  // 双列抽屉里每出现一个就留下一个孤立半行（右半栏大片空白）。
  '配套设施', '备注', '户籍地址', '现居住地', '现居住地址', '紧急联系人地址',
  '离职原因', '离职原因说明', '维修说明', '地址', '详细地址',
])
/**
 * 第 69 轮：字段名是否「长到放不进同行」。
 *
 * 同行排布下 label 固定 84px（约 6 个 13px 汉字）。超过 6 字、或带单位括号
 * （`垃圾处理费(元/套)`、`二次供水费(元/m³)`）的字段名在这一格里必然被截断，
 * 所以**只让这些字段**切换成上下结构 —— 短标签（工号/姓名/部门）继续同行，
 * 不再一刀切全部置顶（第 68 轮上下结构把抽屉拉得很长）。
 */
const LONG_LABEL_MIN = 7
const isLongLabel = (f: DetailFieldMeta): boolean => {
  const label = String(f?.label || f?.name || '').trim()
  const len = Array.from(label).length
  if (len >= LONG_LABEL_MIN) return true
  // 带括号单位的短名（`电梯费(元)` = 6 字）在 84px 里也放不下
  return /[(（][^)）]{1,10}[)）]/.test(label)
}
const fieldClassOf = (f: DetailFieldMeta) => ({
  'bsd-field--full': isLongType(f) || f.dataType === 'textarea' || f.dataType === 'attachment'
    || isLongTextField(f) || f.name === 'fileName'
    || FULL_WIDTH_FIELDS.has(String(f.name || ''))
    || (f as any).dataType === 'multi',
  // 长字段名（含单位）单独换行；其余短字段保持同行紧凑排布
  'bsd-field--long-label': isLongLabel(f),
})
/** 第 64 轮：两列布局的网格类（CSS 侧只做列定义切换，行距同源）。 */
const fieldsClass = computed(() => ({ 'bsd-fields--two': props.twoColumn }))
function close() {
  emit('cancel')
  innerVisible.value = false
}
</script>

<style scoped lang="less">
.bsd-drawer :deep(.t-drawer__body) { padding: 0; }
/* 单栏垂直流：自上而下排布；横向不溢出（消除外层横向滚动条），纵向按需滚动 */
.bsd-body {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: var(--td-comp-margin-s);
  min-height: 0;
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
  padding: var(--td-comp-margin-m) var(--td-comp-margin-m) 0;
}
/* 第 69 轮：分组标题上下间距收紧（14 / 8），去掉整块的大段留白 */
.bsd-block { margin: 0 0 6px; }
.bsd-page-badge {
  display: inline-flex;
  align-items: center;
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
  background: var(--td-success-color-1, #e8f8f2);
  color: var(--td-success-color, #00a870);
  border: 1px solid var(--td-success-color-3, #94e3c5);
  white-space: nowrap;
  vertical-align: middle;
}
.detail-title-main { font-weight: 600; }
.bsd-status-tag {
  border: 0 !important;
  font-weight: 500;
}
/* 第 64 轮：状态纯带色文字 —— 无边框、无背景，绝不再出现「黑框」 */
.bsd-status-text {
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  font-size: var(--td-font-size-body-medium);
  font-weight: 500;
  border: 0 !important;
  background: transparent !important;
  white-space: nowrap;

  &.is-ok { color: var(--td-success-color); }
  &.is-danger { color: var(--td-error-color); }
  &.is-warn { color: var(--td-warning-color); }
  &.is-off { color: var(--td-text-color-placeholder); }
  &.is-muted { color: var(--td-text-color-secondary); }
}
/* 第 63 轮：状态 Tag 一律无黑框，浅色底由 TDesign theme=light 提供 */
.bsd-fields :deep(.t-tag) {
  border: 0 !important;
}
.bsd-block-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  /* 第 69 轮：上下 14 / 8，与表单 row-gap 10 一起把纵向空隙收到紧凑 */
  margin: 14px 0 8px;

  /* 第 53 轮：分组标题前的绿色竖条（4×14、圆角 2px）。
     与能耗打印预览/原项目小节标题同一视觉语言，颜色走设计令牌，
     不新增硬编码主题色（fallback 与 .bsd-page-badge 保持一致）。 */
  &::before {
    content: '';
    flex: none;
    width: 4px;
    height: 14px;
    border-radius: 2px;
    background: var(--td-success-color, #00a870);
  }
}
/* 第 69 轮：容器查询锚点 + 自适应列数（2 列 / 3 列）。
   inline-size 容器只能被子孙查询 —— 所以必须多包一层 .bsd-fields-wrap，
   把 .bsd-fields 自身的列数交给 @container 决定。 */
.bsd-fields-wrap {
  container-type: inline-size;
  min-width: 0;
}
.bsd-fields {
  display: grid;
  /* 默认 / 窄态：一行两列（抽屉 min-width 680，内容区 ~648 → 每列 ~314） */
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 20px;
  row-gap: 10px;
  /* dense：整行字段后面的空位由后续短字段回填，杜绝孤立半行空洞 */
  grid-auto-flow: dense;
  align-items: start;
}
/* 拉宽到 1020px 以上：自动扩展为三列（820 默认宽仍是两列，绝不挤压控件） */
@container (min-width: 1020px) {
  .bsd-fields { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
/* 极窄态（用户把抽屉拖到 min-width 以下 / 小窗 <600px）：单列，控件不再被挤 */
@container (max-width: 600px) {
  .bsd-fields { grid-template-columns: minmax(0, 1fr); }
}
/* 每列允许收缩：minmax(0,1fr) + min-width:0，长内容只截断自己、不撑破栅格 */
.bsd-fields :deep(.t-form__item) {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  min-width: 0;
  /* 第 69 轮：清掉 TDesign 默认的 margin-bottom:24px（与 row-gap 叠加后行距翻倍） */
  margin-bottom: 0 !important;
}
/**
 * 第 69 轮：**同行优先**的字段名（label 在左、控件在右，同一水平线）。
 *
 * 84px 能放下 6 个 13px 汉字（`紧急联系人` / `参保险种` 都不折行）；
 * 超过 6 字或带单位括号的由 .bsd-field--long-label 切上下结构，见下。
 */
.bsd-fields :deep(.t-form__label) {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  width: 84px !important;
  max-width: 84px;
  flex: 0 0 84px;
  margin: 0 !important;
  padding: 0 !important;
  font-size: 13px;
  line-height: 20px;
  font-weight: 400;
  color: var(--td-text-color-secondary, #4b5563);
  white-space: nowrap;
  overflow: visible;
  text-align: right;
}
.bsd-fields :deep(.t-form__label) > label { cursor: default; }
.bsd-fields :deep(.t-form__controls) {
  flex: 1 1 auto;
  margin-left: 0 !important;
  min-width: 0;
}
.bsd-fields :deep(.t-form__controls-content) { min-width: 0; }
/**
 * 长字段名：单个字段切换上下结构（label 在上、控件在下并独占整列宽）。
 *
 * 只影响命中的那一个字段，同组其它短字段仍保持同行 —— 这样
 * `是否本地首次缴纳社保` / `垃圾处理费(元/套)` 不被截断，
 * 而 `工号` / `姓名` / `部门` 不会白占半行高度。
 */
.bsd-fields :deep(.bsd-field--long-label) {
  flex-direction: column;
  align-items: stretch;
  gap: 3px;
}
.bsd-fields :deep(.bsd-field--long-label > .t-form__label) {
  width: auto !important;
  max-width: none;
  flex: 0 0 auto;
  justify-content: flex-start;
  text-align: left;
  line-height: 18px;
}
/* 极窄态兜底：单列时（每列 <290px）所有字段统一换行，避免控件被压扁 */
@container (max-width: 600px) {
  .bsd-fields :deep(.t-form__item) { flex-direction: column; align-items: stretch; gap: 3px; }
  .bsd-fields :deep(.t-form__label) {
    width: auto !important;
    max-width: none;
    flex: 0 0 auto;
    justify-content: flex-start;
    text-align: left;
    line-height: 18px;
  }
}
/* 对齐红线：左/右两列所有输入控件的左、右边缘必须垂直方向 100% 对齐。
   TDesign 的 t-date-picker / t-select 等带有自身固有宽度（min-width 内联样式），
   在栅格里会撑破列宽，导致「开票日期」比下方的「税率/价税合计」更宽 ——
   统一强制 100% + min-width:0（允许收缩）+ border-box（padding/border 计入宽度）。 */
.bsd-fields :deep(.t-date-picker),
.bsd-fields :deep(.t-select),
.bsd-fields :deep(.t-input-number),
.bsd-fields :deep(.t-input),
.bsd-fields :deep(.t-textarea),
.bsd-fields :deep(.t-input__wrap),
.bsd-fields :deep(.t-tag-input) {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}
.bsd-fields :deep(.bsd-field--full) { grid-column: 1 / -1; }
/* 明细表格：只保留本容器自身的横向滚动条，且不超过父级宽度（杜绝双滚动条） */
.bsd-items { overflow-x: auto; max-width: 100%; }
.bsd-items-table {
  border-collapse: collapse;
  font-size: 12px;
  line-height: 1.7;
  width: 100%;
}
.bsd-items-table th,
.bsd-items-table td {
  padding: 2px 10px;
  text-align: left;
  white-space: nowrap;
  border-bottom: 1px solid var(--td-component-stroke);
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bsd-items-table th { font-weight: 500; color: var(--td-text-color-secondary); }
.bsd-items-table td { color: var(--td-text-color-primary); }
/* 合计行：加粗 + 上边框 + 略深背景，与明细行明确分层（与列表浮层同一套视觉） */
.bsd-items-table tfoot .bsd-items-total td {
  font-weight: 600;
  border-top: 1px solid var(--td-component-border);
  border-bottom: none;
  background: var(--td-bg-color-secondarycontainer);
}
.bsd-items-table :deep(.item-cell--num) { text-align: right; }
/* 明细里的多行文本子列（如「说明」）：默认 nowrap + ellipsis 会把长内容裁掉，
   看起来像数据丢了。改成折行完整显示，与列表/打印同一呈现口径。 */
.bsd-items-table :deep(.item-cell--multiline) {
  white-space: pre-wrap;
  word-break: break-word;
  overflow: visible;
  text-overflow: clip;
  max-width: 240px;
}
.bsd-muted { color: var(--td-text-color-placeholder); font-size: 12px; }
/* 源文件预览：永远位于最底部；折叠态不挂载重型资源。
   分割线对齐：.bsd-body 有左右内边距，若直接画 border-top，这条线会比抽屉底部
   操作栏的分割线（贯穿全宽）短一截，形成长短错位。用等量负 margin 把边界拉到
   抽屉内容区全宽，再用等量 padding 把内容推回对齐位置。 */
.bsd-preview-block {
  margin-top: var(--td-comp-margin-s);
  margin-left: calc(-1 * var(--td-comp-margin-m));
  margin-right: calc(-1 * var(--td-comp-margin-m));
  border-top: 1px solid var(--td-component-stroke);
  padding: var(--td-comp-margin-s) var(--td-comp-margin-m) 0;
}
.bsd-preview-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  padding: 2px 0;
  font-size: 12px;
}
.bsd-preview-pane {
  margin-top: var(--td-comp-margin-s);
  width: 100%;
  max-width: 100%;
  overflow: auto;
}
/* 只读字段值：与输入框同高同左内距，视觉上仍是一张「表单」，只是不可编辑 */
.bsd-readonly-value {
  width: 100%;
  min-height: 32px;
  line-height: 32px;
  padding: 0 10px;
  border-radius: var(--td-radius-default);
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-primary);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  box-sizing: border-box;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &.is-long {
    height: auto;
    line-height: 20px;
    padding: 6px 10px;
    white-space: pre-wrap;
    word-break: break-word;
  }
}

.bsd-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--td-comp-margin-s);
  padding: var(--td-comp-margin-m);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
