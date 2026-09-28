<template>
  <SettingDrawer v-model:visible="drawerVisible" :title="'发票设置'" width="760px"
    :storage-key="'weknora-invoice-settings-drawer-width'" destroy-on-close
    class="invoice-settings-drawer">
    <t-tabs v-model="activeTab" class="invoice-settings-tabs" @change="onTabChange">
      <t-tab-panel value="fields" label="字段定义">
        <div class="invoice-fields-panel">
          <div class="invoice-fields-head">
            <span class="invoice-fields-title">发票列表字段</span>
            <t-button variant="outline" size="small" theme="primary" :loading="saving" @click="saveFields">
              保存
            </t-button>
          </div>
          <div class="invoice-fields-table">
            <div class="invoice-fields-row invoice-fields-row--head">
              <span>字段名称</span>
              <span>启用</span>
              <span>默认显示</span>
            </div>
            <template v-for="(f, i) in fieldsList" :key="f.name + '-' + i">
              <div class="invoice-fields-row">
                <span class="iff-name" :class="{ 'iff-disabled': !f.enabled }">{{ f.label || f.name }}</span>
                <span class="iff-switch">
                  <t-switch v-model="f.enabled" size="small" />
                </span>
                <span class="iff-switch">
                  <t-switch v-model="f.is_default" size="small" />
                </span>
              </div>
            </template>
            <div v-if="!fieldsList.length" class="invoice-fields-empty">
              {{ loading ? '加载中…' : '暂无字段配置' }}
            </div>
          </div>
          <div class="invoice-fields-tip">字段配置保存后立即同步列表表头与字段筛选器</div>
        </div>
      </t-tab-panel>
      <t-tab-panel value="rules" label="提取规则">
        <ExtractRulePanel ref="extractRef" scope="invoice" kb-name="日常事务-发票" :kb-id="kbId || ''" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import ExtractRulePanel from './ExtractRulePanel.vue'
import { listFleetCategories, updateFleetCategory } from '@/api/fleet'

const props = defineProps<{ visible: boolean; kbId?: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

// v-model 桥接：props 不可写，经 computed 转发 update:visible
const drawerVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const activeTab = ref('fields')
const extractRef = ref()

interface InvoiceSub {
  name: string
  label?: string
  is_default?: boolean
  enabled?: boolean
}
const fieldsList = ref<InvoiceSub[]>([])
const loading = ref(false)
const saving = ref(false)
let categoryId = ''
let categoryName = ''
let categoryEnabled = true

async function loadFields() {
  loading.value = true
  try {
    const res: any = await listFleetCategories({ scope: 'invoice' })
    const arr = Array.isArray(res) ? res : res?.data || []
    const cat = arr.find((x: any) => Array.isArray(x.subs)) || arr[0]
    if (cat) {
      categoryId = cat.id
      categoryName = cat.name
      categoryEnabled = cat.enabled !== false
      fieldsList.value = (Array.isArray(cat.subs) ? cat.subs : []).map((s: any) => ({
        name: String(s.name || '').trim(),
        label: String(s.label || s.name || '').trim(),
        is_default: s.is_default === true,
        enabled: s.enabled !== false,
      }))
    } else {
      fieldsList.value = []
    }
  } catch (e) {
    console.error('load invoice fields failed', e)
    fieldsList.value = []
  } finally {
    loading.value = false
  }
}

async function saveFields() {
  if (!categoryId) {
    MessagePlugin.error('发票字段配置不存在')
    return
  }
  const subs = fieldsList.value.map((f) => ({
    name: f.name,
    label: f.label,
    is_default: !!f.is_default,
    enabled: f.enabled !== false,
  }))
  saving.value = true
  try {
    await updateFleetCategory(categoryId, { name: categoryName, enabled: categoryEnabled, subs })
    MessagePlugin.success('字段配置已保存')
    emit('saved')
  } catch (e: any) {
    console.error('save invoice fields failed', e)
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function onTabChange(val: string | number) {
  if (val === 'rules') extractRef.value?.refresh()
}

onMounted(() => {
  loadFields()
})
</script>

<style lang="less" scoped>
.invoice-settings-tabs {
  height: 100%;
  display: flex;
  flex-direction: column;
  :deep(.t-tabs__content) {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding-top: 16px;
  }
}

/* ---- 字段定义面板 ---- */
.invoice-fields-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.invoice-fields-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.invoice-fields-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.invoice-fields-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.invoice-fields-row {
  display: grid;
  grid-template-columns: minmax(140px, 2fr) 80px 100px;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  font-size: 13px;
  border-bottom: 1px solid var(--td-component-stroke);
  &:last-of-type {
    border-bottom: none;
  }
  &--head {
    color: var(--td-text-color-secondary);
    font-weight: 500;
    background: var(--td-bg-color-secondarycontainer);
  }
  .iff-name {
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    &.iff-disabled {
      color: var(--td-text-color-placeholder);
    }
  }
  .iff-switch {
    display: flex;
    align-items: center;
  }
}
.invoice-fields-empty {
  padding: 24px 0;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}
.invoice-fields-tip {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
