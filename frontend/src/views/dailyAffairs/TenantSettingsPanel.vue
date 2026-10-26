<template>
  <!--
    清单设置面板（费用清单）：租户档案 + 分摊子项。

    来源：原 TenantBilling 工具栏「租户设置」按钮打开的抽屉。
    工具栏收敛为 3 个按钮后，这部分配置作为统一设置抽屉的「清单设置」Tab 内嵌。
  -->
  <div class="tenant-settings-panel">
    <t-tabs v-model="settingsTab" class="settings-tabs">
      <t-tab-panel value="tenant" label="租户信息">
        <div class="settings-panel">
          <div class="panel-head">
            <span class="panel-title">租户档案</span>
            <t-button variant="outline" size="small" @click="openCreateTenant">
              <template #icon><t-icon name="add" /></template>新增租户
            </t-button>
          </div>
          <div class="tenant-cards">
            <!-- 新增租户表单：展开在网格顶部 -->
            <div v-if="tenantFormVisible && !tenantForm.id" class="tenant-form">
              <div class="tenant-form-title">新增租户</div>
              <div class="form-grid">
                <div class="form-item">
                  <label>租户名 <span class="required">*</span></label>
                  <t-input v-model="tenantForm.name" placeholder="如：持睿汽车" />
                </div>
                <div class="form-item">
                  <label>租户编号</label>
                  <t-input v-model="tenantForm.tenant_no" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>分摊方式</label>
                  <t-select v-model="tenantForm.allocation_mode" :options="allocationOptions" />
                </div>
                <div class="form-item">
                  <label>租赁日期</label>
                  <t-date-picker v-model="tenantForm.lease_start" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                </div>
                <div class="form-item">
                  <label>租赁年限（年）</label>
                  <t-input v-model.number="tenantForm.lease_years" type="number" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>单位联系人</label>
                  <t-input v-model="tenantForm.contact" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>联系电话</label>
                  <t-input v-model="tenantForm.phone" placeholder="选填" />
                </div>
                <div class="form-item form-item--full">
                  <label>备注</label>
                  <t-textarea v-model="tenantForm.remark" :maxlength="500" placeholder="选填" />
                </div>
              </div>
              <div class="form-actions">
                <t-button variant="outline" size="small" @click="tenantFormVisible = false">取消</t-button>
                <t-button theme="primary" size="small" :loading="savingTenant" @click="saveTenantForm">保存</t-button>
              </div>
            </div>

            <template v-for="t in tenants" :key="t.id">
              <div class="tenant-card">
                <div class="tenant-card-head">
                  <span class="tenant-card-name">{{ t.name }}</span>
                  <span class="tenant-card-no">{{ t.tenant_no || '—' }}</span>
                  <span class="tenant-card-actions">
                    <t-button variant="text" size="small" @click="openTenantForm(t)">
                      <template #icon><t-icon name="edit" size="15px" /></template>
                    </t-button>
                    <t-popconfirm theme="warning" :content="`确定删除租户「${t.name}」吗？`"
                      :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                      @confirm="removeTenant(t)">
                      <t-button variant="text" size="small" @click.stop>
                        <template #icon><t-icon name="delete" size="15px" /></template>
                      </t-button>
                    </t-popconfirm>
                  </span>
                </div>
                <div class="tenant-card-grid">
                  <div class="tenant-card-item"><span class="k">分摊方式</span><span class="v">{{ t.allocation_mode || '按比例分摊' }}</span></div>
                  <div class="tenant-card-item"><span class="k">联系人</span><span class="v">{{ t.contact || '—' }}</span></div>
                  <div class="tenant-card-item"><span class="k">联系电话</span><span class="v">{{ t.phone || '—' }}</span></div>
                </div>
              </div>
              <!-- 编辑租户表单：展开在当前卡片下方 -->
              <div v-if="tenantFormVisible && tenantForm.id === t.id" class="tenant-form">
                <div class="tenant-form-title">编辑租户</div>
                <div class="form-grid">
                  <div class="form-item">
                    <label>租户名 <span class="required">*</span></label>
                    <t-input v-model="tenantForm.name" placeholder="如：持睿汽车" />
                  </div>
                  <div class="form-item">
                    <label>租户编号</label>
                    <t-input v-model="tenantForm.tenant_no" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>分摊方式</label>
                    <t-select v-model="tenantForm.allocation_mode" :options="allocationOptions" />
                  </div>
                  <div class="form-item">
                    <label>租赁日期</label>
                    <t-date-picker v-model="tenantForm.lease_start" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                  </div>
                  <div class="form-item">
                    <label>租赁年限（年）</label>
                    <t-input v-model.number="tenantForm.lease_years" type="number" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>单位联系人</label>
                    <t-input v-model="tenantForm.contact" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>联系电话</label>
                    <t-input v-model="tenantForm.phone" placeholder="选填" />
                  </div>
                  <div class="form-item form-item--full">
                    <label>备注</label>
                    <t-textarea v-model="tenantForm.remark" :maxlength="500" placeholder="选填" />
                  </div>
                </div>
                <div class="form-actions">
                  <t-button variant="outline" size="small" @click="tenantFormVisible = false">取消</t-button>
                  <t-button theme="primary" size="small" :loading="savingTenant" @click="saveTenantForm">保存</t-button>
                </div>
              </div>
            </template>
            <div v-if="!tenants.length" class="panel-empty">暂无租户，点击右上角「新增租户」开始配置</div>
          </div>
        </div>
      </t-tab-panel>

      <t-tab-panel value="items" label="分摊子项">
        <div class="settings-panel">
          <div class="panel-head">
            <span class="panel-title">分摊子项</span>
            <t-select v-if="tenants.length" v-model="selectedTenantId" size="small" :options="tenantOptions"
              class="tenant-picker" placeholder="选择租户" @change="loadDetail" />
          </div>
          <div class="item-groups">
            <div v-for="g in itemGroups" :key="g.category" class="item-group">
              <div class="item-group-head" @click="toggleItemGroup(g.category)">
                <span class="item-group-name">{{ g.category }}</span>
                <t-icon :name="expandedItemGroup === g.category ? 'chevron-down' : 'chevron-right'" class="item-group-arrow" />
              </div>
              <div v-if="expandedItemGroup === g.category" class="item-group-body">
                <div class="item-row item-row--head">
                  <span class="item-name">子项名称</span>
                  <span class="item-op">分摊</span>
                </div>
                <div v-for="(it, i) in g.items" :key="i" class="item-row">
                  <span class="item-name" :title="it.item_name">{{ it.item_name }}</span>
                  <span class="item-op">
                    <t-switch size="small" :model-value="!!it.enabled" @change="(v: boolean) => toggleItem(it, v)" />
                  </span>
                </div>
                <div v-if="!g.items.length" class="item-empty">暂无子项</div>
              </div>
            </div>
            <div v-if="!itemGroups.length" class="item-empty">暂无子项，生成账单后自动从市电账单引入</div>
          </div>
        </div>
      </t-tab-panel>
    </t-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, createBillingTenant, updateBillingTenant, deleteBillingTenant,
  getBillingTenant, saveBillingTenantItems,
} from '@/api/knowledge-base'

const props = defineProps<{ tenantId?: string }>()
const emit = defineEmits<{ (e: 'changed'): void; (e: 'select', id: string): void }>()

// 分摊子项归属哪个租户：面板内自持选择（设置抽屉不依赖列表当前选中行，
// 否则用户在抽屉里切换租户会连带刷新背后的列表）。
const selectedTenantId = ref(String(props.tenantId || ''))

const settingsTab = ref('tenant')
const tenants = ref<any[]>([])
const allocationOptions = [{ label: '按比例分摊', value: '按比例分摊' }]

const tenantFormVisible = ref(false)
const savingTenant = ref(false)
const emptyTenantForm = () => ({ id: '', name: '', tenant_no: '', allocation_mode: '按比例分摊', lease_start: '', lease_years: 0, contact: '', phone: '', remark: '' })
const tenantForm = ref<any>(emptyTenantForm())

const detail = ref<any>(null)
const itemsForm = ref<{ category: string; item_key: string; item_name: string; enabled: boolean }[]>([])
const expandedItemGroup = ref('')

const tenantOptions = computed(() => tenants.value.map((t: any) => ({ label: t.name, value: t.id })))

const itemGroups = computed(() => {
  const g: Record<string, any[]> = {}
  itemsForm.value.forEach((it) => {
    const cat = it.category || '其他费用'
    ;(g[cat] = g[cat] || []).push(it)
  })
  return Object.entries(g).map(([category, items]) => ({ category, items }))
})

const loadTenants = async () => {
  try {
    const res: any = await listBillingTenants()
    tenants.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户失败')
  }
}

const loadDetail = async () => {
  if (!selectedTenantId.value) { itemsForm.value = []; return }
  try {
    const res: any = await getBillingTenant(selectedTenantId.value)
    detail.value = res.data
    const d = res.data
    itemsForm.value = (d.items || []).map((it: any) => ({
      category: it.category || '其他费用', item_key: it.item_key, item_name: it.item_name, enabled: !!it.enabled,
    }))
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户详情失败')
  }
}

const openCreateTenant = () => {
  tenantForm.value = emptyTenantForm()
  tenantFormVisible.value = true
}
const openTenantForm = (t: any) => {
  tenantForm.value = {
    id: t.id, name: t.name || '', tenant_no: t.tenant_no || '',
    allocation_mode: t.allocation_mode || '按比例分摊', lease_start: t.lease_start || '',
    lease_years: Number(t.lease_years) || 0, contact: t.contact || '', phone: t.phone || '', remark: t.remark || '',
  }
  tenantFormVisible.value = true
}
const saveTenantForm = async () => {
  if (!tenantForm.value.name.trim()) {
    MessagePlugin.warning('请输入租户名')
    return
  }
  savingTenant.value = true
  try {
    const payload: Record<string, unknown> = {
      name: tenantForm.value.name.trim(),
      tenant_no: tenantForm.value.tenant_no,
      allocation_mode: tenantForm.value.allocation_mode || '按比例分摊',
      lease_start: tenantForm.value.lease_start || null,
      lease_years: Number(tenantForm.value.lease_years) || 0,
      contact: tenantForm.value.contact,
      phone: tenantForm.value.phone,
      remark: tenantForm.value.remark,
    }
    if (tenantForm.value.id) {
      await updateBillingTenant(tenantForm.value.id, payload)
    } else {
      const res: any = await createBillingTenant(payload)
      if (res?.data?.id) emit('select', res.data.id)
    }
    MessagePlugin.success('已保存')
    tenantFormVisible.value = false
    await loadTenants()
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingTenant.value = false
  }
}
const removeTenant = async (t: any) => {
  try {
    await deleteBillingTenant(t.id)
    MessagePlugin.success('已删除')
    await loadTenants()
    if (selectedTenantId.value === t.id) selectedTenantId.value = tenants.value[0]?.id || ''
    if (props.tenantId === t.id) emit('select', tenants.value[0]?.id || '')
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

const toggleItemGroup = (category: string) => {
  expandedItemGroup.value = expandedItemGroup.value === category ? '' : category
}
const toggleItem = async (it: any, v: boolean) => {
  it.enabled = !!v
  await saveItems()
}
const saveItems = async () => {
  if (!selectedTenantId.value) return
  try {
    const items = itemsForm.value
      .filter((it: any) => it.item_name.trim())
      .map((it: any) => ({
        category: it.category || '其他费用',
        item_key: `${(it.category || '其他费用').trim()}|${it.item_name.trim()}`, item_name: it.item_name.trim(), enabled: !!it.enabled,
      }))
    await saveBillingTenantItems(selectedTenantId.value, { items })
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存子项失败')
  }
}

onMounted(async () => {
  await loadTenants()
  if (!selectedTenantId.value) selectedTenantId.value = tenants.value[0]?.id || ''
  await loadDetail()
})
</script>

<style lang="less" scoped>
.tenant-settings-panel { padding: 0 0 16px; }
.settings-tabs {
  :deep(.t-tabs__content) { padding-top: 12px; }
}
.settings-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; }
.panel-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
.tenant-picker { width: 200px; }
.panel-empty { padding: 28px 0; text-align: center; color: var(--td-text-color-placeholder); font-size: 13px; }

.tenant-cards { display: flex; flex-direction: column; gap: 10px; }
.tenant-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 12px 14px;
  background: var(--td-bg-color-container);
}
.tenant-card-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.tenant-card-name { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
.tenant-card-no { font-size: 12px; color: var(--td-text-color-secondary); font-variant-numeric: tabular-nums; }
.tenant-card-actions { margin-left: auto; display: flex; align-items: center; }
.tenant-card-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px 12px; }
.tenant-card-item {
  display: flex; flex-direction: column; gap: 2px;
  .k { font-size: 12px; color: var(--td-text-color-placeholder); }
  .v { font-size: 12px; color: var(--td-text-color-primary); }
}

.tenant-form {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 14px;
  background: var(--td-bg-color-secondarycontainer);
}
.tenant-form-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); margin-bottom: 12px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px 16px; }
.form-item {
  label {
    display: block; font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px;
    .required { color: var(--td-error-color); }
  }
  &--full { grid-column: 1 / -1; }
}
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }

.item-groups { display: flex; flex-direction: column; gap: 8px; }
.item-group { border: 1px solid var(--td-component-stroke); border-radius: 9px; overflow: hidden; }
.item-group-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 9px 12px; cursor: pointer; background: var(--td-bg-color-container);
  .item-group-name { font-size: 13px; color: var(--td-text-color-primary); }
  .item-group-arrow { color: var(--td-text-color-secondary); }
}
.item-group-body { padding: 4px 12px 10px; }
.item-row {
  display: grid; grid-template-columns: 1fr 64px; align-items: center; gap: 8px;
  padding: 6px 0; font-size: 12px; border-bottom: 1px dashed var(--td-component-stroke);
  &:last-child { border-bottom: none; }
  &--head { color: var(--td-text-color-placeholder); border-bottom: 1px solid var(--td-component-stroke); }
  .item-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--td-text-color-primary); }
  &--head .item-name { color: var(--td-text-color-placeholder); }
  .item-op { display: flex; justify-content: flex-start; }
}
.item-empty { padding: 14px 0; text-align: center; color: var(--td-text-color-placeholder); font-size: 12px; }
</style>
