<template>
  <div class="us-wrap">
    <div v-if="visible" class="us-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" :header="'设置 · 光伏'" :size="`${drawerWidth}px`" :footer="false"
      class="solar-settings-drawer" :close-on-overlay-click="true" @close="onClose"
      @update:visible="(v: boolean) => (v || onClose())">
      <div class="us-body">
        <!-- 基本户信息 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">基本户信息</span>
            <span class="us-state">概览基础信息默认加载</span>
          </div>
          <div class="us-hint">配置默认光伏发电户档案信息，保存后账单概览「基础信息」直接加载，无需每次解析。</div>
          <div class="ba-grid">
            <div class="ba-item">
              <label class="ba-label">户号</label>
              <t-input v-model="account.account_no" size="small" placeholder="发电户号" />
            </div>
            <div class="ba-item">
              <label class="ba-label">户名</label>
              <t-input v-model="account.account_name" size="small" placeholder="户名" />
            </div>
            <div class="ba-item">
              <label class="ba-label">服务单位</label>
              <t-input v-model="account.supply_unit" size="small" placeholder="服务单位" />
            </div>
            <div class="ba-item">
              <label class="ba-label">并网电压</label>
              <t-input v-model="account.voltage_level" size="small" placeholder="并网电压等级" />
            </div>
            <div class="ba-item">
              <label class="ba-label">消纳方式</label>
              <t-input v-model="account.consumption_mode" size="small" placeholder="消纳方式" />
            </div>
            <div class="ba-item">
              <label class="ba-label">发电方式</label>
              <t-input v-model="account.generation_mode" size="small" placeholder="发电方式" />
            </div>
            <div class="ba-item ba-item--full">
              <label class="ba-label">发电地址</label>
              <t-input v-model="account.address" size="small" placeholder="发电地址" />
            </div>
            <div class="ba-item">
              <label class="ba-label">纳税人类型</label>
              <t-input v-model="account.taxpayer_type" size="small" placeholder="纳税人类型" />
            </div>
          </div>
        </div>

        <!-- 手动保存 -->
        <div class="us-save-bar">
          <t-button theme="primary" size="small" @click="saveAccount">保存</t-button>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

const props = defineProps<{ visible: boolean; kbId: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const ACCOUNT_KEY = 'weknora-solar-account-v1'
const DRAWER_WIDTH_KEY = 'weknora-solar-settings-width'

const drawerWidth = ref(Number(localStorage.getItem(DRAWER_WIDTH_KEY)) || 640)
const account = ref<Record<string, string>>({})

function loadStored() {
  try {
    const acc = localStorage.getItem(ACCOUNT_KEY)
    if (acc) account.value = { ...account.value, ...JSON.parse(acc) }
  } catch { /* ignore */ }
}
loadStored()

watch(() => props.visible, (v) => {
  if (v) loadStored()
})

const saveAccount = () => {
  try { localStorage.setItem(ACCOUNT_KEY, JSON.stringify(account.value)) } catch { /* ignore */ }
  MessagePlugin.success('已保存')
  emit('changed')
}

// 拖动调整宽度（与删除历史抽屉一致）
const onResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  const startX = e.clientX
  const startW = drawerWidth.value
  const onMove = (ev: MouseEvent) => {
    const w = Math.min(960, Math.max(480, startW - (ev.clientX - startX)))
    drawerWidth.value = w
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

const onClose = () => {
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.us-wrap { position: relative; }
.us-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2100;
  cursor: col-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  .us-resize-line {
    width: 2px;
    height: 40px;
    border-radius: 1px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity 0.15s ease, height 0.15s ease;
  }
  &:hover .us-resize-line, .us-resize-line:hover {
    opacity: 1;
    height: 80px;
  }
}
.us-body { display: flex; flex-direction: column; gap: 16px; }
.us-section {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 14px 16px;
}
.us-section-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.us-section-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.us-state { font-size: 12px; color: var(--td-text-color-secondary); }
.us-hint { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 12px; line-height: 1.6; }

.ba-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.ba-item { display: flex; flex-direction: column; gap: 4px; }
.ba-item--full { grid-column: 1 / -1; }
.ba-label { font-size: 12px; color: var(--td-text-color-secondary); }

.us-save-bar {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 8px;
}
</style>
