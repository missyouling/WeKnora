<template>
  <t-dialog :visible="visible" header="组织机构" width="880px" :footer="false" placement="center"
    class="hr-org-dialog" @close="close" @update:visible="(v: boolean) => { if (!v) close() }">
    <div class="org-body">
      <t-tabs v-model="tab" size="small">
        <t-tab-panel value="company" label="公司" />
        <t-tab-panel value="department" label="部门" />
      </t-tabs>

      <!-- 内容用 v-if 自绘：TDesign 的 tab-panel 内容默认懒渲染且只保留当前面板，
           直接写在 panel 里会导致切换前 DOM 为空（真机断言与首屏都受影响）。 -->
      <div v-if="tab === 'company'" class="org-pane">
        <div class="org-row">
          <span class="org-label">公司名称</span>
          <t-input v-model="company.name" placeholder="公司名称" class="org-control" />
        </div>
        <div class="org-row">
          <span class="org-label">公司编码</span>
          <t-input v-model="company.code" placeholder="公司编码（默认 02）" class="org-control" />
        </div>
      </div>

      <div v-else class="org-pane">
        <div class="org-toolbar">
          <span class="org-tip">已按花名册自动生成，可改名 / 编码 / 排序 / 删除</span>
          <t-button variant="outline" size="small" @click="addDept">
            <template #icon><t-icon name="add" size="14px" /></template>
            新增部门
          </t-button>
        </div>
        <div class="org-list">
          <div class="org-head">
            <span>部门名称</span>
            <span>部门编码</span>
            <span>排序</span>
            <span>操作</span>
          </div>
          <div v-for="(d, i) in departments" :key="d.id || i" class="org-item">
            <t-input v-model="d.name" size="small" placeholder="部门名称" />
            <t-input v-model="d.code" size="small" placeholder="编码" />
            <t-input-number v-model="d.sort_order" size="small" :min="1" theme="normal" />
            <t-button variant="text" size="small" theme="danger" @click="departments.splice(i, 1)">删除</t-button>
          </div>
          <div v-if="!departments.length" class="org-empty">暂无部门</div>
        </div>
      </div>

      <div class="org-footer">
        <t-button variant="outline" size="small" @click="close">取消</t-button>
        <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import * as hrApi from './hrApi'

const props = withDefaults(defineProps<{ visible: boolean }>(), { visible: false })
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

const tab = ref<'company' | 'department'>('company')
const saving = ref(false)
const company = ref<Record<string, any>>({ name: '', code: '' })
const departments = ref<any[]>([])

async function load() {
  try {
    const res: any = await hrApi.getOrganization()
    const d = res?.data || res || {}
    company.value = d.company ? { ...d.company } : { name: '', code: '' }
    departments.value = Array.isArray(d.departments) ? d.departments.map((x: any) => ({ ...x })) : []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载组织机构失败')
  }
}

watch(() => props.visible, (v) => { if (v) void load() })

function addDept() {
  const n = departments.value.length + 1
  departments.value.push({ name: '', code: `D${String(n).padStart(2, '0')}`, sort_order: n })
}

async function save() {
  saving.value = true
  try {
    await hrApi.saveOrganization({
      company: company.value,
      departments: departments.value.filter((d) => String(d.name || '').trim()),
    })
    MessagePlugin.success('组织机构已保存')
    emit('saved')
    close()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function close() { emit('update:visible', false) }
</script>

<style lang="less" scoped>
.org-body { display: flex; flex-direction: column; gap: 12px; }
.org-row { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.org-label { flex: 0 0 84px; color: var(--td-text-color-secondary); }
.org-control { flex: 1; min-width: 0; }
.org-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 8px; }
.org-tip { color: var(--td-text-color-secondary); }
.org-list { max-height: 380px; overflow: auto; }
.org-pane { display: flex; flex-direction: column; gap: 8px; }
.org-head,
.org-item {
  display: grid;
  grid-template-columns: 1.4fr 1fr 0.7fr 0.6fr;
  gap: 8px;
  align-items: center;
  padding: 4px 0;
}
.org-head { color: var(--td-text-color-placeholder); border-bottom: 1px solid var(--td-component-border); }
.org-item :deep(.t-input-number) { width: 100%; }
.org-empty { padding: 16px; text-align: center; color: var(--td-text-color-placeholder); }
.org-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--td-component-border);
}
</style>
