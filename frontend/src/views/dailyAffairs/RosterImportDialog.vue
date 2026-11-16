<!--
  花名册 / 离职管理 / 劳动合同 / 公积金 导入弹窗（第 52 轮）。

  第 58 轮：视觉与交互全部下沉到中台 `StandardImportDialog`，
  本文件只保留「模块 → 模板 / 接口 / 文案」的业务映射，杜绝各处私造上传 UI。
-->
<template>
  <StandardImportDialog :visible="visible" :title="`导入${moduleLabel}`" :module-label="moduleLabel"
    :modes="modes" default-mode="merge" :on-download="downloadTemplate" :on-import="doImport"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @imported="(n: number) => emit('imported', n)" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StandardImportDialog from '@/components/business/StandardImportDialog.vue'
import * as hrApi from './hrApi'
import { HR_TEMPLATE_NAMES, type HrModule } from './hrFieldSchema'

const props = withDefaults(defineProps<{
  visible: boolean
  /** 三套独立模板（花名册 / 离职管理 / 劳动合同）+ 公积金。 */
  module?: HrModule
}>(), { visible: false, module: 'roster' })

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'imported', count: number): void
}>()

const moduleLabel = computed(() => ({
  roster: '花名册', resign: '离职管理', contract: '劳动合同',
  fund: '公积金', social: '社保回盘',
}[props.module] || '花名册'))

const templateName = computed(() => HR_TEMPLATE_NAMES[props.module] || HR_TEMPLATE_NAMES.roster)

/**
 * 公积金网厅导出文件是纯 upsert，没有合并/新增/更新之分；其余模块保留三模式。
 */
const modes = computed(() => (props.module === 'fund'
  ? [{ label: '合并更新', value: 'merge' }]
  : [
    { label: '合并更新', value: 'merge' },
    { label: '仅新增', value: 'insert' },
    { label: '仅更新', value: 'update' },
  ]))

const downloadTemplate = async (): Promise<Blob> => {
  const blob = await hrApi.getHrTemplate(props.module)
  return new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
}

const doImport = async (files: File[], mode: string): Promise<number> => {
  const fd = new FormData()
  fd.append('file', files[0])
  fd.append('mode', mode)
  fd.append('default_status', '在职')
  const res: any = await hrApi.importHrModule(props.module, fd)
  const data = res?.data || res || {}
  return Number(data.inserted || 0) + Number(data.updated || 0)
}

// 保留导出给单测/外部引用的模板名（行为与第 52 轮一致）
defineExpose({ templateName })
</script>
