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
      </div>
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

    <div v-if="!embedded" class="ss-save-bar">
      <t-button theme="primary" size="small" @click="saveAccount">保存</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { getUtilityBasicInfo, saveUtilityBasicInfo } from '@/api/knowledge-base'

// 内嵌模式（第 74 轮）：隐藏自带保存栏，保存由抽屉底栏统一提交。
defineProps<{ kbId?: string; embedded?: boolean }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const ACCOUNT_KEY = 'weknora-solar-account-v1'
const account = ref<Record<string, string>>({})

/**
 * 第 75 轮：基本户改为**落库**（utility_basic_accounts, category=solar）。
 *
 * 旧实现只写 localStorage：换浏览器/清缓存即丢，且与账单概览的"基础信息"脱钩。
 * 复用中台已有 basic-info 接口，字段映射：
 *   户号→account_no、户名→account_name、服务单位→supply_unit、并网电压→voltage_level、
 *   消纳方式→usage_category、发电方式→market_attr、发电地址→address、纳税人类型→meter_no
 * 同时保留 localStorage 镜像，兼容旧数据与离线回显。
 */
function payloadOf(): Record<string, unknown> {
  const a = account.value
  return {
    name: '光伏基本户',
    account_no: a.account_no || '',
    account_name: a.account_name || '',
    supply_unit: a.supply_unit || '',
    voltage_level: a.voltage_level || '',
    usage_category: a.consumption_mode || '',
    market_attr: a.generation_mode || '',
    address: a.address || '',
    meter_no: a.taxpayer_type || '',
  }
}
function applyPayload(d: any) {
  const src = d?.data || d || {}
  // 后端还没有基本户时返回 {}：保留 localStorage 镜像，绝不清空用户已填内容
  if (!Object.keys(src).length) return
  account.value = {
    ...account.value,
    account_no: String(src.account_no || ''),
    account_name: String(src.account_name || ''),
    supply_unit: String(src.supply_unit || ''),
    voltage_level: String(src.voltage_level || ''),
    consumption_mode: String(src.usage_category || ''),
    generation_mode: String(src.market_attr || ''),
    address: String(src.address || ''),
    taxpayer_type: String(src.meter_no || ''),
  }
}
function loadStored() {
  try {
    const acc = localStorage.getItem(ACCOUNT_KEY)
    if (acc) account.value = { ...account.value, ...JSON.parse(acc) }
  } catch { /* ignore */ }
}
onMounted(async () => {
  loadStored()
  try {
    applyPayload(await getUtilityBasicInfo('solar'))
  } catch { /* 后端不可用时保留本地回显 */ }
})

const saveAccount = async () => {
  try { localStorage.setItem(ACCOUNT_KEY, JSON.stringify(account.value)) } catch { /* ignore */ }
  try {
    await saveUtilityBasicInfo('solar', payloadOf())
    MessagePlugin.success('已保存')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
    throw e
  }
  emit('changed')
}

// 第 74 轮：抽屉底栏「保存」调用（内嵌模式下不再自带保存按钮）
defineExpose({ commit: saveAccount })
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
