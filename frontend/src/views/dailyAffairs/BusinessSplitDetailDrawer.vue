<!--
  BusinessSplitDetailDrawer —— 分屏详情抽屉中台组件（蓝图 P1-5）。

  布局：左 = 源文件预览（可折叠，**折叠时不挂载** DocumentPreview，避免 PDF/图片
  渲染器初始化），右 = 提取质量溯源 + 动态字段表单 + 明细子项 + 扩展插槽。

  契约：
    - 表单状态由父级持有（`form`），组件只做绑定与展示——保存链路的口径（载荷组装、
      级联、列表局部更新）仍在父级的可测纯函数里，不在组件里；
    - 字段一律按 `FieldMeta[]` 渲染（dataType: text/date/number/array/items），
      业务特有控件（类型下拉/状态标签/标签墙）通过 `#field-<name>` 具名插槽注入；
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

    <div class="bsd-body">
      <!-- 左：源文件预览 -->
      <div class="bsd-preview" :class="{ 'is-collapsed': !previewOpen }">
        <button type="button" class="bsd-preview-toggle" :title="previewOpen ? '收起预览' : '展开预览'"
          @click="previewOpen = !previewOpen">
          <t-icon :name="previewOpen ? 'chevron-left' : 'chevron-right'" size="14px" />
          <span v-if="!previewOpen" class="bsd-preview-rail">源文件预览</span>
        </button>
        <div v-if="previewOpen" class="bsd-preview-pane">
          <DocumentPreview v-if="row" :knowledge-id="row.knowledgeId" :page="pageNo"
            :file-type="row.fileType || ''" :file-name="row.fileName || ''" :active="true" />
        </div>
      </div>

      <!-- 右：质量溯源 + 字段表单 -->
      <div class="bsd-main">
        <ExtractQualityPanel v-if="quality" :quality="quality" :label-of="labelOf" compact />

        <section class="bsd-block">
          <div class="bsd-block-title">{{ blockTitle }}</div>
          <div class="bsd-fields">
            <template v-for="f in fields" :key="f.name">
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
                  <span v-else-if="f.dataType === 'items'" class="bsd-muted">明细见下方表格</span>
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
      </div>
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
import { sanitizeNum } from './businessDetailForm'

export interface DetailFieldMeta {
  name: string
  label: string
  dataType?: string
  /** 备注片段字段名（有值表示数据存 remark 片段里） */
  remarkPart?: string
}

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
  /** 展开预览时的抽屉宽度（分屏需要一个可用的右栏，否则字段会被挤成省略号） */
  expandedWidth?: string
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
  expandedWidth: '1180px',
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
// 预览默认折叠：展开才挂载 DocumentPreview（重资源，不能随抽屉一起初始化）
const previewOpen = ref(false)
// 展开预览时抽屉加宽：654px 分屏会把右侧字段压到 ~300px，标签/输入全被截断
const drawerWidth = computed(() => (previewOpen.value ? props.expandedWidth : props.width))
watch(() => props.row?.knowledgeId, () => { previewOpen.value = false })

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
const isLongType = (f: DetailFieldMeta) => f.dataType === 'items' || f.dataType === 'array'
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
.bsd-body {
  display: flex;
  align-items: stretch;
  gap: var(--td-comp-margin-s);
  min-height: 0;
  height: 100%;
}
.bsd-preview {
  display: flex;
  flex-direction: column;
  flex: 1 1 50%;
  max-width: 56%;
  min-width: 200px;
  border-right: 1px solid var(--td-component-stroke);
  padding-right: var(--td-comp-margin-s);
}
.bsd-preview.is-collapsed { flex: 0 0 34px; max-width: 34px; padding-right: 0; }
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
.bsd-preview-rail { writing-mode: vertical-rl; letter-spacing: 2px; }
.bsd-preview-pane { flex: 1; min-height: 0; overflow: auto; }
.bsd-main {
  flex: 1 1 50%;
  /* 右栏最小可用宽度：低于这个值两列表单会被压成省略号 */
  min-width: 340px;
  overflow-y: auto;
  padding-right: var(--td-comp-margin-s);
}
.bsd-block { margin-bottom: var(--td-comp-margin-l); }
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
.bsd-items { overflow-x: auto; }
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
.bsd-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--td-comp-margin-s);
  padding-top: var(--td-comp-margin-m);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
