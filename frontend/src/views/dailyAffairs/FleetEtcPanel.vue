<template>
  <div class="fleet-etc-panel">
    <div class="panel-head">
      <span class="panel-title">ETC管理</span>
      <t-button size="small" theme="primary" variant="outline" @click="startAdd">
        <template #icon><t-icon name="add" size="14px" /></template>新增ETC卡
      </t-button>
    </div>
    <div v-if="!cards.length" class="panel-empty">暂无ETC卡，点击新增配置</div>
    <div class="card-list">
      <div v-for="card in cards" :key="card.id" class="cfg-item">
        <div class="cfg-item-head">
          <span class="cfg-name" @click="toggleExpand(card)">{{ card.alias || card.card_no || '未命名ETC卡' }}</span>
          <div class="cfg-actions">
            <t-button variant="text" size="small" class="icon-btn" @click="toggleExpand(card)">
              <t-icon :name="expanded.has(card.id) ? 'chevron-up' : 'chevron-down'" size="14px" />
            </t-button>
            <t-popconfirm theme="warning" content="确定删除该ETC卡吗？" :confirm-btn="{ content: '删除', theme: 'danger' }"
              :cancel-btn="{ content: '取消' }" placement="top" @confirm="removeCard(card)">
              <t-button variant="text" size="small" theme="danger" class="icon-btn">
                <t-icon name="delete" size="14px" />
              </t-button>
            </t-popconfirm>
          </div>
        </div>
        <transition name="cat-expand">
          <div v-if="expanded.has(card.id)" class="cfg-body">
            <div class="cfg-grid">
              <div class="cfg-field">
                <label>别名</label>
                <t-input v-model="card.alias" size="small" placeholder="如：主车ETC" />
              </div>
              <div class="cfg-field">
                <label>ETC卡号</label>
                <t-input v-model="card.card_no" size="small" placeholder="卡号" />
              </div>
              <div class="cfg-field">
                <label>卡类型</label>
                <t-select v-model="card.card_type" size="small" clearable :options="typeOptions" />
              </div>
              <div class="cfg-field">
                <label>发卡方</label>
                <t-input v-model="card.issuer" size="small" placeholder="发卡机构" />
              </div>
              <div class="cfg-field">
                <label>开户行</label>
                <t-input v-model="card.bank" size="small" placeholder="开户银行" />
              </div>
              <div class="cfg-field">
                <label>开户日期</label>
                <t-date-picker v-model="card.open_date" size="small" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable />
              </div>
              <div class="cfg-field">
                <label>有效期</label>
                <t-date-picker v-model="card.expire_date" size="small" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable />
              </div>
              <div class="cfg-field">
                <label>绑定车辆</label>
                <t-select v-model="card.vehicle_id" size="small" :options="vehicleOptions" filterable clearable />
              </div>
              <div class="cfg-field">
                <label>卡状态</label>
                <t-select v-model="card.card_status" size="small" clearable :options="statusOptions" />
              </div>
              <div class="cfg-field">
                <label>启用状态</label>
                <t-switch v-model="card.enabled" size="small" />
              </div>
              <div class="cfg-field cfg-field--wide">
                <label>备注</label>
                <t-textarea v-model="card.remark" size="small" :maxlength="200" placeholder="选填" :autosize="{ minRows: 1, maxRows: 3 }" />
              </div>
            </div>
            <div class="cfg-footer">
              <t-button theme="primary" size="small" :loading="savingId === card.id" @click="saveCard(card)">保存</t-button>
            </div>
          </div>
        </transition>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listFleetETCCards, createFleetETCCard, updateFleetETCCard, deleteFleetETCCard, listFleetVehicles } from '@/api/fleet'

const typeOptions = ['主卡', '副卡', '子卡'].map((v) => ({ label: v, value: v }))
const statusOptions = ['正常', '挂失', '冻结', '注销', '过期'].map((v) => ({ label: v, value: v }))

const cards = ref<any[]>([])
const vehicles = ref<any[]>([])
const expanded = ref<Set<string>>(new Set())
const savingId = ref('')
const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.id })))

async function load() {
  try {
    const [cr, vr] = await Promise.all([
      listFleetETCCards(),
      listFleetVehicles().catch(() => ({ data: [] })),
    ])
    cards.value = cr.data || []
    vehicles.value = vr.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  }
}
onMounted(load)

function toggleExpand(card: any) {
  if (expanded.value.has(card.id)) expanded.value.delete(card.id)
  else expanded.value.add(card.id)
  expanded.value = new Set(expanded.value)
}

function startAdd() {
  const tmp: any = { id: '__new__', alias: '', card_no: '', card_type: '', issuer: '', bank: '', open_date: '', expire_date: '', vehicle_id: '', card_status: '正常', enabled: true, remark: '' }
  cards.value.unshift(tmp)
  expanded.value.add(tmp.id)
}

async function saveCard(card: any) {
  const isNew = card.id === '__new__'
  if (!(card.card_no || '').trim()) { MessagePlugin.warning('请填写卡号'); return }
  savingId.value = card.id
  try {
    const payload = {
      alias: card.alias || '', card_no: card.card_no, card_type: card.card_type || '',
      issuer: card.issuer || '', bank: card.bank || '', open_date: card.open_date || '',
      expire_date: card.expire_date || '', vehicle_id: card.vehicle_id || '',
      card_status: card.card_status || '正常', enabled: card.enabled !== false, remark: card.remark || '',
    }
    if (isNew) {
      const res = await createFleetETCCard(payload)
      const created = res.data || res
      cards.value = cards.value.map((c: any) => (c.id === '__new__' ? created : c))
      MessagePlugin.success('已创建')
    } else {
      await updateFleetETCCard(card.id, payload)
      MessagePlugin.success('已保存')
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingId.value = ''
  }
}

async function removeCard(card: any) {
  try {
    await deleteFleetETCCard(card.id)
    cards.value = cards.value.filter((c: any) => c.id !== card.id)
    expanded.value.delete(card.id)
    MessagePlugin.success('已删除')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}
</script>

<style lang="less" scoped>
.fleet-etc-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; }
.panel-title { font-size: 14px; font-weight: 600; }
.panel-empty { padding: 24px 0; text-align: center; color: var(--td-text-color-placeholder); font-size: 13px; }
.card-list { display: flex; flex-direction: column; gap: 8px; }
.cfg-item { border: 1px solid var(--td-component-border); border-radius: 8px; overflow: hidden; background: var(--td-bg-color-container); }
.cfg-item-head { display: flex; align-items: center; gap: 10px; padding: 10px 12px; }
.cfg-name { flex: 1; font-size: 14px; cursor: pointer; }
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
