<!--
  中台通用附件预览器（第 55 轮）。

  与 document-preview.vue 的区别：document-preview 面向「知识库文档」全类型播放，
  本组件面向「人事离职证明」这类单文件附件，强调页内工具栏常用操作：
  放大 / 缩小 / 1:1 / 旋转 / 打印 / 下载 / 关闭，以及滚轮缩放与拖拽平移。

  铁律：严禁 window.open / target="_blank" —— 全部在当前页面内完成。
-->
<template>
  <t-dialog :visible="visible" :header="title || fileName || '附件预览'" width="960px"
    :footer="false" placement="center" class="file-preview-dialog" destroy-on-close
    @close="close" @update:visible="(v: boolean) => { if (!v) close() }">
    <div class="fpd-toolbar">
      <t-button variant="outline" size="small" @click="zoomBy(0.2)">
        <template #icon><t-icon name="zoom-in" size="15px" /></template>放大
      </t-button>
      <t-button variant="outline" size="small" :disabled="scale <= 0.3" @click="zoomBy(-0.2)">
        <template #icon><t-icon name="zoom-out" size="15px" /></template>缩小
      </t-button>
      <t-button variant="outline" size="small" @click="resetView">1:1 实际大小</t-button>
      <t-button variant="outline" size="small" @click="rotate">
        <template #icon><t-icon name="refresh" size="15px" /></template>旋转
      </t-button>
      <span class="fpd-zoom">{{ Math.round(scale * 100) }}%</span>
      <span class="fpd-spacer" />
      <t-button variant="outline" size="small" @click="printFile">
        <template #icon><t-icon name="print" size="15px" /></template>打印
      </t-button>
  <t-button variant="outline" size="small" :disabled="!src" @click="downloadFile">
    <template #icon><t-icon name="download" size="15px" /></template>下载
  </t-button>
  <!-- 第 57 轮：工具栏不再重复渲染「关闭」按钮 —— 关闭入口只保留标题栏右上角的 ×。 -->
</div>

    <div class="fpd-stage" :class="{ 'is-image': kind === 'image' }"
      @wheel.prevent="onWheel" @mousedown="onDragStart" @mousemove="onDragMove"
      @mouseup="onDragEnd" @mouseleave="onDragEnd">
      <img v-if="kind === 'image' && src" :src="src" :alt="fileName" draggable="false"
        :style="imageStyle" @load="resetView" />
      <iframe v-else-if="kind === 'pdf' && src" ref="pdfFrame" :src="src" class="fpd-pdf"
        :title="fileName" />
      <div v-else class="fpd-empty">暂无可预览的附件</div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  visible: boolean
  /** 已创建的 blob URL（父组件负责 URL.createObjectURL + 生命周期回收）。 */
  src?: string
  fileName?: string
  /** 显式指定类型；缺省按文件名后缀推断。 */
  fileType?: string
  title?: string
}>(), { src: '', fileName: '附件', fileType: '', title: '' })

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const kind = computed<'image' | 'pdf' | 'other'>(() => {
  const t = String(props.fileType || '').toLowerCase()
  if (t.includes('image') || t.includes('png') || t.includes('jpg') || t.includes('jpeg')) return 'image'
  if (t.includes('pdf') || t.includes('application/pdf')) return 'pdf'
  const n = String(props.fileName || '').toLowerCase()
  if (/\.(png|jpe?g|webp|gif|bmp)$/.test(n)) return 'image'
  if (/\.pdf$/.test(n)) return 'pdf'
  return 'other'
})

const scale = ref(1)
const rotation = ref(0)
const offset = ref({ x: 0, y: 0 })
const dragging = ref(false)
const dragOrigin = ref({ x: 0, y: 0, ox: 0, oy: 0 })
const pdfFrame = ref<HTMLIFrameElement | null>(null)

const imageStyle = computed(() => ({
  transform: `translate(${offset.value.x}px, ${offset.value.y}px) scale(${scale.value}) rotate(${rotation.value}deg)`,
  cursor: dragging.value ? 'grabbing' : 'grab',
}))

function zoomBy(delta: number) {
  scale.value = Math.min(5, Math.max(0.2, Math.round((scale.value + delta) * 100) / 100))
}
function onWheel(e: WheelEvent) {
  zoomBy(e.deltaY > 0 ? -0.1 : 0.1)
}
function resetView() {
  scale.value = 1
  rotation.value = 0
  offset.value = { x: 0, y: 0 }
}
function rotate() { rotation.value = (rotation.value + 90) % 360 }

function onDragStart(e: MouseEvent) {
  if (kind.value !== 'image') return
  dragging.value = true
  dragOrigin.value = { x: e.clientX, y: e.clientY, ox: offset.value.x, oy: offset.value.y }
}
function onDragMove(e: MouseEvent) {
  if (!dragging.value) return
  offset.value = {
    x: dragOrigin.value.ox + (e.clientX - dragOrigin.value.x),
    y: dragOrigin.value.oy + (e.clientY - dragOrigin.value.y),
  }
}
function onDragEnd() { dragging.value = false }

function printFile() {
  if (!props.src) return
  if (kind.value === 'pdf' && pdfFrame.value?.contentWindow) {
    pdfFrame.value.contentWindow.print()
    return
  }
  // 图片：用隐藏 iframe 打印，绝不跳新窗口
  const frame = document.createElement('iframe')
  frame.style.position = 'fixed'
  frame.style.width = '0'
  frame.style.height = '0'
  frame.style.border = '0'
  frame.onload = () => {
    frame.contentWindow?.focus()
    frame.contentWindow?.print()
    setTimeout(() => frame.remove(), 1000)
  }
  frame.src = props.src
  document.body.appendChild(frame)
}
function downloadFile() {
  if (!props.src) return
  const a = document.createElement('a')
  a.href = props.src
  a.download = props.fileName || '附件'
  document.body.appendChild(a)
  a.click()
  a.remove()
}
function close() { emit('update:visible', false) }

// 每次打开重置视图，避免上一次的缩放/旋转残留
watch(() => props.visible, (v) => { if (v) resetView() })
</script>

<style scoped>
.fpd-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--td-component-stroke);
}
.fpd-zoom {
  min-width: 46px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.fpd-spacer { flex: 1; }
.fpd-stage {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: min(68vh, 680px);
  margin-top: 10px;
  overflow: hidden;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  user-select: none;
}
.fpd-stage.is-image { cursor: grab; }
.fpd-stage img {
  max-width: 92%;
  max-height: 92%;
  object-fit: contain;
  transform-origin: center;
  transition: transform .12s ease-out;
  pointer-events: none;
}
.fpd-pdf {
  width: 100%;
  height: 100%;
  border: 0;
  background: #fff;
  transform: scale(1);
}
.fpd-empty { color: var(--td-text-color-placeholder); }
</style>
