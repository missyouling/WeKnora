<template>
  <div class="fleet-supplier-panel">
    <div class="panel-head">
      <span class="panel-title">供应商</span>
      <t-button size="small" theme="primary" variant="outline" @click="startAdd">
        <template #icon><t-icon name="add" size="14px" /></template>新增供应商
      </t-button>
    </div>
    <div v-if="!suppliers.length" class="panel-empty">暂无供应商，点击新增配置</div>
    <div class="card-list">
      <div v-for="sup in suppliers" :key="sup.id" class="cfg-item">
        <div class="cfg-item-head">
          <span class="cfg-name" @click="toggleExpand(sup)">{{ sup.name || '未命名供应商' }}</span>
          <span class="cfg-type">{{ sup.supplier_type || '' }}</span>
          <div class="cfg-actions">
            <t-button variant="text" size="small" class="icon-btn" @click="toggleExpand(sup)">
              <t-icon :name="expanded.has(sup.id) ? 'chevron-up' : 'chevron-down'" size="14px" />
            </t-button>
            <t-popconfirm theme="warning" content="确定删除该供应商吗？" :confirm-btn="{ content: '删除', theme: 'danger' }"
              :cancel-btn="{ content: '取消' }" placement="top" @confirm="removeSupplier(sup)">
              <t-button variant="text" size="small" theme="danger" class="icon-btn">
                <t-icon name="delete" size="14px" />
              </t-button>
            </t-popconfirm>
          </div>
        </div>
        <transition name="cat-expand">
          <div v-if="expanded.has(sup.id)" class="cfg-body">
            <div class="cfg-grid">
              <div class="cfg-field">
                <label>供应商名称</label>
                <t-input v-model="sup.name" size="small" placeholder="名称" />
              </div>
              <div class="cfg-field">
                <label>供应商类型</label>
                <t-select v-model="sup.supplier_type" size="small" filterable allow-create clearable :options="typeOptions" />
              </div>
              <div class="cfg-field">
                <label>资质等级</label>
                <t-input v-model="sup.qualification" size="small" placeholder="如：一类维修" />
              </div>
              <div class="cfg-field">
                <label>统一社会信用代码</label>
                <t-input v-model="sup.credit_code" size="small" placeholder="代码" />
              </div>
              <div class="cfg-field">
                <label>法定代表人</label>
                <t-input v-model="sup.legal_person" size="small" placeholder="姓名" />
              </div>
              <div class="cfg-field">
                <label>联系人</label>
                <t-input v-model="sup.contact" size="small" placeholder="姓名" />
              </div>
              <div class="cfg-field">
                <label>联系电话</label>
                <t-input v-model="sup.phone" size="small" placeholder="电话" />
              </div>
              <div class="cfg-field">
                <label>联系地址</label>
                <t-input v-model="sup.address" size="small" placeholder="地址" />
              </div>
              <div class="cfg-field">
                <label>合作状态</label>
                <t-select v-model="sup.cooperation_status" size="small" clearable :options="coopOptions" />
              </div>
              <div class="cfg-field">
                <label>合作开始日期</label>
                <t-date-picker v-model="sup.coop_start_date" size="small" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable />
              </div>
              <div class="cfg-field">
                <label>结算方式</label>
                <t-select v-model="sup.settle_method" size="small" clearable :options="settleOptions" />
              </div>
              <div class="cfg-field">
                <label>税率</label>
                <t-input v-model="sup.tax_rate" size="small" placeholder="如：13%" />
              </div>
              <div class="cfg-field">
                <label>发票类型</label>
                <t-select v-model="sup.invoice_type" size="small" clearable :options="invoiceOptions" />
              </div>
              <div class="cfg-field">
                <label>供应商状态</label>
                <t-select v-model="sup.status" size="small" clearable :options="statusOptions" />
              </div>
              <div class="cfg-field">
                <label>启用状态</label>
                <t-switch v-model="sup.enabled" size="small" />
              </div>
              <div class="cfg-field cfg-field--wide">
                <label>备注</label>
                <t-textarea v-model="sup.remark" size="small" :maxlength="300" placeholder="选填" :autosize="{ minRows: 1, maxRows: 3 }" />
              </div>
            </div>
            <div class="cfg-footer">
              <t-button theme="primary" size="small" :loading="savingId === sup.id" @click="saveSupplier(sup)">保存</t-button>
            </div>
          </div>
        </transition>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listFleetSuppliers, createFleetSupplier, updateFleetSupplier, deleteFleetSupplier } from '@/api/fleet'

const typeOptions = ['维修厂', '配件商', '轮胎商', '油品商', '保险公司', '年检代办', '洗车', '救援', '租赁'].map((v) => ({ label: v, value: v }))
const coopOptions = ['潜在', '合作中', '暂停', '终止'].map((v) => ({ label: v, value: v }))
const settleOptions = ['月结', '现结', '季度结'].map((v) => ({ label: v, value: v }))
const invoiceOptions = ['增值税专票', '增值税普票'].map((v) => ({ label: v, value: v }))
const statusOptions = ['正常', '停用', '黑名单'].map((v) => ({ label: v, value: v }))

const suppliers = ref<any[]>([])
const expanded = ref<Set<string>>(new Set())
const savingId = ref('')

async function load() {
  try {
    const res = await listFleetSuppliers()
    suppliers.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  }
}
onMounted(load)

function toggleExpand(sup: any) {
  if (expanded.value.has(sup.id)) expanded.value.delete(sup.id)
  else expanded.value.add(sup.id)
  expanded.value = new Set(expanded.value)
}

function startAdd() {
  const tmp: any = {
    id: '__new__', name: '', supplier_type: '', qualification: '', credit_code: '', legal_person: '',
    contact: '', phone: '', address: '', cooperation_status: '合作中', coop_start_date: '',
    settle_method: '', tax_rate: '', invoice_type: '', status: '正常', enabled: true, remark: '',
  }
  suppliers.value.unshift(tmp)
  expanded.value.add(tmp.id)
}

async function saveSupplier(sup: any) {
  const isNew = sup.id === '__new__'
  if (!(sup.name || '').trim()) { MessagePlugin.warning('请填写供应商名称'); return }
  savingId.value = sup.id
  try {
    const payload = {
      name: sup.name, supplier_type: sup.supplier_type || '', qualification: sup.qualification || '',
      credit_code: sup.credit_code || '', legal_person: sup.legal_person || '', contact: sup.contact || '',
      phone: sup.phone || '', address: sup.address || '', cooperation_status: sup.cooperation_status || '',
      coop_start_date: sup.coop_start_date || '', settle_method: sup.settle_method || '', tax_rate: sup.tax_rate || '',
      invoice_type: sup.invoice_type || '', status: sup.status || '正常', enabled: sup.enabled !== false, remark: sup.remark || '',
    }
    if (isNew) {
      const res = await createFleetSupplier(payload)
      const created = res.data || res
      suppliers.value = suppliers.value.map((c: any) => (c.id === '__new__' ? created : c))
      MessagePlugin.success('已创建')
    } else {
      await updateFleetSupplier(sup.id, payload)
      MessagePlugin.success('已保存')
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingId.value = ''
  }
}

async function removeSupplier(sup: any) {
  try {
    await deleteFleetSupplier(sup.id)
    suppliers.value = suppliers.value.filter((c: any) => c.id !== sup.id)
    expanded.value.delete(sup.id)
    MessagePlugin.success('已删除')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}
</script>

<style lang="less" scoped>
.fleet-supplier-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; }
.panel-title { font-size: 14px; font-weight: 600; }
.panel-empty { padding: 24px 0; text-align: center; color: var(--td-text-color-placeholder); font-size: 13px; }
.card-list { display: flex; flex-direction: column; gap: 8px; }
.cfg-item { border: 1px solid var(--td-component-border); border-radius: 8px; overflow: hidden; background: var(--td-bg-color-container); }
.cfg-item-head { display: flex; align-items: center; gap: 10px; padding: 10px 12px; }
.cfg-name { flex: 1; font-size: 14px; cursor: pointer; }
.cfg-type { font-size: 12px; color: var(--td-text-color-placeholder); }
.cfg-actions { display: flex; align-items: center; gap: 4px; }
.icon-btn { width: 28px; height: 28px; padding: 0; display: flex; align-items: center; justify-content: center; }
.cfg-body { border-top: 1px dashed var(--td-component-border); padding: 12px; background: var(--td-bg-color-container-hover); }
.cfg-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
.cfg-field { display: flex; flex-direction: column; gap: 4px; }
.cfg-field--wide { grid-column: 1 / -1; }
.cfg-field label { font-size: 12px; color: var(--td-text-color-placeholder); }
.cfg-footer { display: flex; justify-content: flex-end; margin-top: 12px; }
.cat-expand-enter-active, .cat-expand-leave-active { transition: opacity .18s ease; }
.cat-expand-enter-from, .cat-expand-leave-to { opacity: 0; }
</style>
