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
  <SettingDrawer v-model:visible="innerVisible" :title="title" :width="drawerWidth" :storage-key="storageKey"
    hide-footer destroy-on-close class="bsd-drawer">
    <template #title>
      <slot name="title">
        <span class="detail-title-main">{{ title }}</span>
        <span v-if="pageNo >= 1" class="row-page-badge">P{{ pageNo }}</span>
      </slot>
    </template>

    <!-- 单栏垂直流：自顶部向下依次为 质量 / 表单 / 明细 / 源预览 -->
    <div class="bsd-body">
      <ExtractQualityPanel v-if="quality" :quality="quality" :label-of="labelOf" compact />

      <section class="bsd-block">
        <div class="bsd-block-title">{{ blockTitle }}</div>
        <div class="bsd-fields">
          <template v-for="f in formFields" :key="f.name">
            <t-form-item :label="f.label" label-width="110px" :class="fieldClassOf(f)">
              <slot :name="`field-${f.name}`" :field="f" :form="form">
                <!-- 备注片段字段：无标准存储位，值存 remark 的「字段名：值」片段 -->
                <t-textarea v-if="f.remarkPart && isLongType(f)" v-model="form.remarkParts[f.remarkPart]"
                  :autosize="{ minRows: 2, maxRows: 5 }" placeholder="每行一条" />
                <t-input v-else-if="f.remarkPart" v-model="form.remarkParts[f.remarkPart]" />
                <t-date-picker v-else-if="f.dataType === 'date'" v-model="form.data[f.name]" value-type="YYYY-MM-DD"
                  format="YYYY-MM-DD" clearable allow-input />
                <t-input v-else-if="f.dataType === 'number'" v-model="form.data[f.name]"
                  @input="(v: string) => (form.data[f.name] = sanitizeNum(v))" />
                <t-textarea v-else-if="f.dataType === 'array'" v-model="form.data[f.name]"
                  :autosize="{ minRows: 2, maxRows: 5 }" placeholder="每行一条" />
                <t-input v-else v-model="form.data[f.name]" />
              </slot>
            </t-form-item>
          </template>
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
          </table>
        </div>
      </section>

      <slot name="extra" />

      <!-- 最底部：源文件预览，默认折叠（不挂载重型资源），展开原地展开 -->
      <section class="bsd-preview-block">
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
      <t-popconfirm v-if="showDelete" content="确定删除该记录吗？该操作不可恢复" @confirm="emit('delete')">
        <t-button variant="outline" size="small" theme="danger">删除</t-button>
      </t-popconfirm>
      <t-button v-if="showReExtract" variant="outline" size="small" @click="emit('re-extract')">重新提取</t-button>
      <t-button variant="outline" size="small" @click="close">取消</t-button>
      <t-button theme="primary" size="small" :loading="saving" :disabled="dirty === false"
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
import { sanitizeNum, type DetailFieldMeta } from './businessDetailForm'

const props = withDefaults(defineProps<{
  visible: boolean
  title?: string
  /** 当前行（提供 knowledgeId / fileName / fileType 供预览） */
  row?: any
  /** 物理页码（预览跳转用；缺省回退 1） */
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
  showReExtract: false,
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
const itemRows = computed<any[]>(() => (Array.isArray(form.value.items) ? form.value.items : []))
// 子列：未配置时按第一行实际键生成（不写死任何业务字段集）
const itemCols = computed<any[]>(() => {
  if (props.subFields?.length) return props.subFields
  const keys = new Set<string>()
  for (const it of itemRows.value) for (const k of Object.keys(it || {})) if (k !== 'extra') keys.add(k)
  return [...keys].map((k) => ({ key: k, label: k }))
})
// 主表单只渲染真实字段：items 类型（明细容器）交由下方明细表格承载，绝不生成占位行
const formFields = computed<DetailFieldMeta[]>(
  () => (props.fields || []).filter((f) => f.dataType !== 'items' && f.name !== 'items' && f.label !== '项目明细'),
)
const isLongType = (f: DetailFieldMeta) => f.dataType === 'array'
const fieldClassOf = (f: DetailFieldMeta) => ({
  'bsd-field--full': isLongType(f) || f.name === 'fileName',
})
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
  gap: var(--td-comp-margin-m);
  min-height: 0;
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
  padding: var(--td-comp-margin-m) var(--td-comp-margin-m) 0;
}
.bsd-block { margin-bottom: var(--td-comp-margin-s); }
.bsd-block-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: var(--td-comp-margin-s);
}
.bsd-fields {
  display: grid;
  /* 自适应列数：抽屉窄（用户拖小/小窗）时自动落成单列，避免标签和输入被截断 */
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 var(--td-comp-margin-m);
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
.bsd-items-table :deep(.item-cell--num) { text-align: right; }
.bsd-muted { color: var(--td-text-color-placeholder); font-size: 12px; }
/* 源文件预览：永远位于最底部；折叠态不挂载重型资源 */
.bsd-preview-block {
  margin-top: var(--td-comp-margin-s);
  border-top: 1px solid var(--td-component-stroke);
  padding-top: var(--td-comp-margin-s);
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
.bsd-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--td-comp-margin-s);
  padding: var(--td-comp-margin-m);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
