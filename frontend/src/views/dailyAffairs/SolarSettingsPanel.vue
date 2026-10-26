<template>
  <!--
    光伏设置面板（原 SolarSettingsDrawer 的抽屉体）。

    为什么改成面板：工具栏铁律只允许 3 个按钮（字段 / 设置 / 上传账单），
    「光伏设置」不再单独开抽屉，而是作为统一设置抽屉的第 3 个 Tab 内嵌。
    组件本身不持有开关状态，由 StandardSettingDrawer 的 Tab 承载。
  -->
  <div class="solar-settings-panel">
    <div class="ss-section">
      <div class="ss-section-head">
        <span class="ss-section-title">基本户信息</span>
        <span class="ss-state">概览基础信息默认加载</span>
      </div>
      <div class="ss-hint">配置默认光伏发电户档案信息，保存后账单概览「基础信息」直接加载，无需每次解析。</div>
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

    <div class="ss-save-bar">
      <t-button theme="primary" size="small" @click="saveAccount">保存</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

defineProps<{ kbId?: string }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const ACCOUNT_KEY = 'weknora-solar-account-v1'
const account = ref<Record<string, string>>({})

function loadStored() {
  try {
    const acc = localStorage.getItem(ACCOUNT_KEY)
    if (acc) account.value = { ...account.value, ...JSON.parse(acc) }
  } catch { /* ignore */ }
}
onMounted(loadStored)

const saveAccount = () => {
  try { localStorage.setItem(ACCOUNT_KEY, JSON.stringify(account.value)) } catch { /* ignore */ }
  MessagePlugin.success('已保存')
  emit('changed')
}
</script>

<style lang="less" scoped>
.solar-settings-panel { display: flex; flex-direction: column; gap: 16px; }
.ss-section {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 14px 16px;
}
.ss-section-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.ss-section-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.ss-state { font-size: 12px; color: var(--td-text-color-secondary); }
.ss-hint { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 12px; line-height: 1.6; }

.ba-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.ba-item { display: flex; flex-direction: column; gap: 4px; }
.ba-item--full { grid-column: 1 / -1; }
.ba-label { font-size: 12px; color: var(--td-text-color-secondary); }

.ss-save-bar {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 8px;
}
</style>
