<template>
  <SettingDrawer v-model:visible="innerVisible" :title="title" :width="drawerWidth"
    storage-key="weknora-hr-settings-drawer-width" hide-footer class="hr-settings-drawer"
    @update:visible="(v: boolean) => emit('update:visible', v)">
    <template #title>
      <span class="hrs-title">{{ title }}</span>
    </template>

    <div class="hrs-body">
      <t-tabs v-model="tab" size="small">
        <t-tab-panel value="fields" label="字段设置" />
        <t-tab-panel value="org" label="组织机构" />
      </t-tabs>

      <!-- 字段设置：11 分组矩阵（左分组 + 右字段复选框） -->
      <div v-if="tab === 'fields'" class="hrs-pane">
        <div class="hrs-tip">勾选后点击「保存」生效，列表表头、字段筛选、详情抽屉、导出与打印同步更新。</div>
        <div class="hrs-matrix">
          <div class="hrs-groups">
            <div v-for="g in ROSTER_FIELD_GROUPS" :key="g.key" class="hrs-group-item"
              :class="{ 'is-active': activeKey === g.key }" @click="activeKey = g.key">
              <t-checkbox :checked="stateOf(g) === 'all'" :indeterminate="stateOf(g) === 'partial'"
                @change="(v: boolean) => toggleGroup(g, v)" @click.stop />
              <span class="hrs-group-label">{{ g.label }}</span>
              <span class="hrs-group-count">{{ checkedCountOf(g) }}/{{ g.fields.length }}</span>
            </div>
          </div>
          <div class="hrs-fields">
            <div class="hrs-fields-head">{{ activeGroup?.label }}</div>
            <div class="hrs-field-grid">
              <label v-for="fd in activeGroup?.fields || []" :key="fd.name" class="hrs-field-item">
                <t-checkbox :checked="isChecked(fd)" :disabled="fd.locked"
                  @change="(v: boolean) => setField(fd, v)" />
                <span class="hrs-field-name">{{ fd.name }}</span>
                <t-tag v-if="fd.locked" size="small" variant="light" theme="default">必选</t-tag>
              </label>
            </div>
          </div>
        </div>
        <div class="hrs-footer">
          <t-button variant="outline" size="small" @click="resetFields">恢复默认</t-button>
          <span class="hrs-footer-grow" />
          <t-button theme="primary" size="small" @click="saveFields">保存</t-button>
        </div>
      </div>

      <!-- 组织机构：仅公司 + 部门 -->
      <div v-else class="hrs-pane">
        <div class="org-section">
          <div class="org-section-title">公司定义</div>
          <div class="org-row">
            <span class="org-label">公司名称</span>
            <t-input v-model="company.name" placeholder="公司名称" class="org-control" />
          </div>
          <div class="org-row">
            <span class="org-label">公司编码</span>
            <t-input v-model="company.code" placeholder="公司编码（默认 02）" class="org-control" />
          </div>
        </div>

        <div class="org-section">
          <div class="org-section-title">
            部门定义
            <t-button variant="outline" size="small" class="org-add" @click="addDept">
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
              <t-input v-model="d.code" size="small" placeholder="编码（5 位）" />
              <t-input-number v-model="d.sort_order" size="small" :min="1" theme="normal" />
              <t-button variant="text" size="small" theme="danger" @click="departments.splice(i, 1)">删除</t-button>
            </div>
            <div v-if="!departments.length" class="org-empty">暂无部门</div>
          </div>
        </div>

        <div class="hrs-footer">
          <span class="hrs-footer-grow" />
          <t-button theme="primary" size="small" :loading="savingOrg" @click="saveOrg">保存</t-button>
        </div>
      </div>
    </div>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import {
  ROSTER_FIELD_GROUPS, defaultVisibleFields, lockedRosterFields,
  groupCheckState, type RosterFieldDef, type RosterFieldGroupDef,
} from './hrRosterGroups'
import * as hrApi from './hrApi'

const props = withDefaults(defineProps<{
  visible: boolean
  title?: string
  visibleFields: string[]
}>(), { visible: false, title: '花名册设置', visibleFields: () => [] })

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'save-fields', fields: string[]): void
  (e: 'saved-org'): void
}>()

const drawerWidth = ref('860px')
const innerVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const tab = ref<'fields' | 'org'>('fields')
const activeKey = ref<string>(ROSTER_FIELD_GROUPS[0].key)
const draft = ref<Set<string>>(new Set())

const company = ref<Record<string, any>>({ name: '', code: '' })
const departments = ref<any[]>([])
const savingOrg = ref(false)

const activeGroup = computed<RosterFieldGroupDef | undefined>(
  () => ROSTER_FIELD_GROUPS.find((g) => g.key === activeKey.value),
)

watch(() => props.visible, (v) => {
  if (!v) return
  tab.value = 'fields'
  activeKey.value = ROSTER_FIELD_GROUPS[0].key
  // 第 48 轮：首次打开按「11 分组默认勾选」初始化；若父级已有用户保存过的
  // 显隐集合（localStorage），以它为草稿，保证六同步。
  draft.value = new Set(props.visibleFields.length ? props.visibleFields : defaultVisibleFields())
  void loadOrg()
})

async function loadOrg() {
  try {
    const res: any = await hrApi.getOrganization()
    const d = res?.data || res || {}
    company.value = d.company ? { ...d.company } : { name: '', code: '' }
    departments.value = Array.isArray(d.departments) ? d.departments.map((x: any) => ({ ...x })) : []
  } catch {
    company.value = { name: '', code: '' }
    departments.value = []
  }
}

function isChecked(fd: RosterFieldDef): boolean {
  return fd.locked || draft.value.has(fd.name)
}
function setField(fd: RosterFieldDef, checked: boolean) {
  if (fd.locked) return
  if (checked) draft.value.add(fd.name)
  else draft.value.delete(fd.name)
}
function stateOf(g: RosterFieldGroupDef): 'all' | 'none' | 'partial' {
  return groupCheckState(g, draft.value)
}
function checkedCountOf(g: RosterFieldGroupDef): number {
  return g.fields.filter((fd) => isChecked(fd)).length
}
function toggleGroup(g: RosterFieldGroupDef, checked: boolean) {
  for (const fd of g.fields) {
    if (fd.locked) continue
    if (checked) draft.value.add(fd.name)
    else draft.value.delete(fd.name)
  }
}
function resetFields() {
  draft.value = new Set(defaultVisibleFields())
  MessagePlugin.info('已恢复默认勾选，点击「保存」生效')
}
function saveFields() {
  const out = new Set(draft.value)
  for (const name of lockedRosterFields()) out.add(name)
  emit('save-fields', Array.from(out))
  MessagePlugin.success('字段设置已保存')
}

function addDept() {
  const n = departments.value.length + 1
  departments.value.push({ name: '', code: '', sort_order: n })
}

async function saveOrg() {
  savingOrg.value = true
  try {
    await hrApi.saveOrganization({
      company: company.value,
      departments: departments.value.filter((d) => String(d.name || '').trim()),
    })
    MessagePlugin.success('组织机构已保存')
    emit('saved-org')
    await loadOrg()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingOrg.value = false
  }
}
</script>

<style lang="less" scoped>
.hrs-title { font-weight: 600; }
.hrs-body { display: flex; flex-direction: column; gap: 12px; height: 100%; min-height: 0; }
.hrs-pane { display: flex; flex-direction: column; gap: 10px; min-height: 0; }
.hrs-tip { color: var(--td-text-color-secondary); }

.hrs-matrix {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 12px;
  min-height: 380px;
}
.hrs-groups {
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  padding: 6px;
  max-height: 420px;
  overflow: auto;
}
.hrs-group-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;

  &:hover { background: var(--td-bg-color-secondarycontainer); }
  &.is-active { background: var(--td-brand-color-light); }
}
.hrs-group-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hrs-group-count { color: var(--td-text-color-placeholder); }

.hrs-fields {
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  padding: 10px 12px;
  max-height: 420px;
  overflow: auto;
}
.hrs-fields-head { font-weight: 600; margin-bottom: 8px; color: var(--td-text-color-primary); }
.hrs-field-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 6px 12px;
}
.hrs-field-item { display: flex; align-items: center; gap: 6px; padding: 3px 0; cursor: pointer; }
.hrs-field-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.hrs-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--td-component-border);
}
.hrs-footer-grow { flex: 1; }

.org-section { display: flex; flex-direction: column; gap: 8px; margin-bottom: 12px; }
.org-section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.org-add { font-weight: 400; }
.org-row { display: flex; align-items: center; gap: 10px; }
.org-label { flex: 0 0 84px; color: var(--td-text-color-secondary); }
.org-control { flex: 1; min-width: 0; }
.org-list { max-height: 320px; overflow: auto; }
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
</style>
