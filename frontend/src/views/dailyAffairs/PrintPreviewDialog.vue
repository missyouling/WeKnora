<!--
  PrintPreviewDialog —— 全中台统一的 PDF 打印预览弹窗。

  为什么单独抽组件：市电账单 / 光伏账单 / 电水气核算 / 费用清单 原先各写一份
  mask + dialog + iframe + 底栏（类名分别为 utility- / meter- / tenant-print-*），
  间距、圆角、按钮顺序、是否带「下载」全不一样；用户在四个页面看到四种弹窗。
  现在壳子只有这一份，内容（PDF 字节）由各页面自己生成。

  契约：
    · `data` 是 PDF 字节（父级生成时就有，直接传，**不要再 fetch blob URL**：
      生产环境 CSP 可能禁掉 blob: 的 connect-src，fetch 会静默失败卡在加载态）；
    · `url` 仅用于「下载 / 打印」的 blob 地址，父级负责 revoke；
    · 预览体统一走 PdfSourceViewer（pdf.js 画布），**不用 iframe 内嵌浏览器 PDF 插件**
      —— 插件自带深色工具条、页面居中/缩放不受控，是「打印预览很丑」的直接来源；
    · 「打印」用一次性隐藏 iframe 调原生 print；「下载」用 <a download>。
-->
<template>
  <teleport to="body">
    <div v-if="visible" class="ppd-mask" @click.self="close">
      <div class="ppd-dialog" role="dialog" aria-modal="true">
        <header class="ppd-header">
          <div class="ppd-heading">
            <t-icon name="file-pdf" size="16px" class="ppd-heading-icon" />
            <span class="ppd-title">{{ title }}</span>
          </div>
          <t-button variant="text" size="small" shape="square" class="ppd-close" @click="close">
            <t-icon name="close" size="16px" />
          </t-button>
        </header>

        <div class="ppd-body">
          <PdfSourceViewer v-if="viewerData" :data="viewerData" :file-name="downloadName" class="ppd-viewer" />
          <div v-else class="ppd-loading">
            <t-loading size="small" :text="loadingText" />
          </div>
        </div>

        <footer class="ppd-footer">
          <span class="ppd-hint">A4 · 打印时请在系统对话框中选择「实际大小」</span>
          <div class="ppd-actions">
            <a v-if="url" class="ppd-download" :href="url" :download="downloadName">
              <t-button variant="outline" size="small">
                <template #icon><t-icon name="download" size="14px" /></template>
                下载 PDF
              </t-button>
            </a>
            <t-button theme="default" variant="outline" size="small" :disabled="!url" @click="doPrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
            <t-button theme="primary" size="small" @click="close">关闭</t-button>
          </div>
        </footer>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import PdfSourceViewer from '@/components/source-preview/PdfSourceViewer.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  title: string
  url: string
  data?: ArrayBuffer | Uint8Array | null
  loadingText?: string
  downloadName?: string
}>(), {
  data: null,
  loadingText: '正在生成 PDF…',
  downloadName: 'preview.pdf',
})

const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'close'): void }>()

const viewerData = ref<ArrayBuffer | null>(null)

/** 统一成「独立」ArrayBuffer：pdf.js 只认 ArrayBuffer，不能拿别人的视图/偏移。 */
function toArrayBuffer(v: ArrayBuffer | Uint8Array): ArrayBuffer {
  if (v instanceof ArrayBuffer) return v.slice(0)
  return v.buffer.slice(v.byteOffset, v.byteOffset + v.byteLength) as ArrayBuffer
}

watch(() => props.data, (v) => {
  viewerData.value = v ? toArrayBuffer(v) : null
}, { immediate: true })

function close() {
  emit('update:visible', false)
  emit('close')
}

function doPrint() {
  if (!props.url) return
  // PDF 的打印只能交给浏览器插件：挂一个隐藏 iframe，加载完调 print()
  const frame = document.createElement('iframe')
  frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0;opacity:0'
  frame.src = props.url
  frame.onload = () => {
    try { frame.contentWindow?.focus(); frame.contentWindow?.print() } catch { window.open(props.url, '_blank') }
    window.setTimeout(() => frame.remove(), 60_000)
  }
  document.body.appendChild(frame)
}
</script>

<style lang="less" scoped>
.ppd-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}

.ppd-dialog {
  width: min(1080px, 92vw);
  height: min(820px, 90vh);
  background: var(--td-bg-color-container);
  border-radius: var(--td-radius-large);
  box-shadow: var(--td-shadow-3);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ppd-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.ppd-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ppd-heading-icon { color: var(--td-error-color); }

.ppd-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppd-close { color: var(--td-text-color-secondary); }

.ppd-body {
  flex: 1;
  min-height: 0;
  // 滚动必须落在这一层之内：body 是 dialog 里唯一的弹性区，
  // 高度被 dialog 的定高（min(820px, 90vh)）约束住，子级才可能滚。
  // 旧版这里还叠了 overflow，但真正的坑在 .ppd-viewer（见下）。
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--td-bg-color-page);
}

/*
  ⚠️ 这里**不能**写 `display: block`。
  PdfSourceViewer 的根节点自带 `display:flex; flex-direction:column`，其滚动体
  `.pdf-source-viewer__scroller` 靠 `flex:1 + min-height:0` 拿高度。
  两个 scoped 选择器（.ppd-viewer / .pdf-source-viewer）特异性相同，谁在后面谁赢；
  一旦 block 生效，滚动体就失去 flex 父级，`min-height:0` 也不再兜底，
  高度被内容撑到 6000+px，clientHeight === scrollHeight → **一点都滚不动**
  （真机取证：打印预览 3 页只能看到第 1 页上沿，且内容溢出盖住底部按钮栏）。
  所以这里显式声明成同样的 flex 列，与样式表顺序无关。
*/
.ppd-viewer {
  width: 100%;
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.ppd-loading {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ppd-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-top: 1px solid var(--td-component-stroke);
}

.ppd-hint { font-size: 12px; color: var(--td-text-color-placeholder); }
.ppd-actions { display: flex; align-items: center; gap: 8px; }
.ppd-download { text-decoration: none; }
</style>
