<template>
  <div class="org-units">
    <div class="org-units__head">
      <span class="org-units__title">单位管理</span>
      <t-button variant="outline" size="small" @click="addUnit">
        <template #icon><t-icon name="add" size="14px" /></template>
        新增单位
      </t-button>
    </div>

    <!-- 单位列表：拖拽排序 + 点击折叠/展开（与字段设置的分类行同一交互） -->
    <div class="org-units__list">
      <div v-for="(u, i) in units" :key="u.id || `new-${i}`" class="org-unit"
        :class="{ 'is-active': activeIndex === i }" draggable="true"
        @dragstart="onUnitDragStart(i)" @dragover.prevent @drop.prevent="onUnitDrop(i)"
        @dragend="dragIndex = -1" @click="toggleUnit(i)">
        <span class="org-unit__drag" title="拖动排序" @click.stop>
          <t-icon name="move" size="14px" />
        </span>
        <span class="org-unit__chevron">
          <t-icon :name="activeIndex === i ? 'chevron-down' : 'chevron-right'" size="14px" />
        </span>
        <span class="org-unit__name">{{ u.name || '未命名单位' }}</span>
        <span class="org-unit__code">{{ u.code }}</span>
        <span class="org-unit__depts">{{ (u.departments || []).length }} 个部门</span>
        <span class="org-unit__ops" @click.stop>
          <t-popconfirm theme="warning" :content="`确定删除单位「${u.name || '未命名单位'}」吗？`"
            @confirm="removeUnit(i)">
            <t-button variant="text" shape="square" size="small" title="删除">
              <DeleteIcon />
            </t-button>
          </t-popconfirm>
        </span>
      </div>
      <div v-if="!units.length" class="org-units__empty">暂无单位</div>
    </div>

    <!-- 选中单位的明细表单（再次点击单位行可折叠） -->
    <template v-if="activeUnit">
      <div class="org-units__title org-units__title--form">单位信息</div>
      <div class="org-form">
        <div class="org-form__row">
          <span class="org-form__label">单位名称</span>
          <t-input v-model="activeUnit.name" placeholder="单位名称" />
        </div>
        <div class="org-form__row">
          <span class="org-form__label">单位编码</span>
          <t-input v-model="activeUnit.code" placeholder="单位编码" />
        </div>
        <div class="org-form__row">
          <span class="org-form__label">统一社会信用代码</span>
          <t-input v-model="activeUnit.credit_code" placeholder="18 位统一社会信用代码" :maxlength="18" />
        </div>
        <div class="org-form__row">
          <span class="org-form__label">住所</span>
          <t-input v-model="activeUnit.address" placeholder="注册 / 经营地址" />
        </div>
        <div class="org-form__row">
          <span class="org-form__label">法定代表人</span>
          <t-input v-model="activeUnit.legal_person" placeholder="法定代表人姓名" />
        </div>
        <!-- 第 76 轮：别名是全局简称唯一真相源（费用清单租户标签 / 核算列表单位标签） -->
        <div class="org-form__row">
          <span class="org-form__label">别名</span>
          <t-input v-model="activeUnit.alias" placeholder="如：持睿汽车 / 重庆星达" data-org-alias />
        </div>
        <div class="org-form__row">
          <span class="org-form__label">是否为租户</span>
          <div class="org-form__switch">
            <t-switch v-model="activeUnit.is_tenant" data-org-tenant-switch />
            <span class="org-form__hint">开启后可维护租户档案与分摊子项</span>
          </div>
        </div>
        <template v-if="activeUnit.is_tenant">
          <div class="org-form__row">
            <span class="org-form__label">租户编码</span>
            <t-input v-model="activeUnit.tenant_no" placeholder="如：Z001" data-org-tenant-no />
          </div>
          <div class="org-form__row">
            <span class="org-form__label">分摊方式</span>
            <t-select v-model="activeUnit.allocation_mode" :options="allocationOptions" />
          </div>
          <div class="org-form__row">
            <span class="org-form__label">联系人</span>
            <t-input v-model="activeUnit.contact" placeholder="单位联系人" />
          </div>
          <div class="org-form__row">
            <span class="org-form__label">联系电话</span>
            <t-input v-model="activeUnit.phone" placeholder="联系电话" />
          </div>
        </template>
      </div>

      <div class="org-units__head org-units__head--dept">
        <span class="org-units__title">下属部门</span>
        <t-button variant="outline" size="small" @click="addDept">
          <template #icon><t-icon name="add" size="14px" /></template>
          新增部门
        </t-button>
      </div>
      <div class="org-depts">
        <div class="org-depts__head">
          <span class="org-depts__drag"></span>
          <span>部门名称</span>
          <span>部门编码</span>
          <span>操作</span>
        </div>
        <div v-for="(d, i) in activeUnit.departments || []" :key="d.id || `newd-${i}`" class="org-dept"
          draggable="true" @dragstart="onDeptDragStart(i)" @dragover.prevent @drop.prevent="onDeptDrop(i)"
          @dragend="deptDragIndex = -1">
          <span class="org-depts__drag" title="拖动排序"><t-icon name="move" size="14px" /></span>
          <t-input v-model="d.name" size="small" placeholder="部门名称" />
          <t-input v-model="d.code" size="small" placeholder="5 位编码" :maxlength="5" />
          <t-popconfirm theme="warning" :content="`确定删除部门「${d.name || '未命名部门'}」吗？`"
            @confirm="removeDept(i)">
            <t-button variant="text" shape="square" size="small" title="删除">
              <DeleteIcon />
            </t-button>
          </t-popconfirm>
        </div>
        <div v-if="!(activeUnit.departments || []).length" class="org-depts__empty">
          暂无下属部门（单位可单独保存）
        </div>
      </div>

      <!-- 租户专属：分摊子项（第 77 轮删除冗余标题栏，直接展示「分摊子项」面板） -->
      <template v-if="activeUnit.is_tenant">
        <TenantSettingsPanel v-if="activeUnit.billing_tenant_id && kbId" ref="allocRef"
          :tenant-id="activeUnit.billing_tenant_id" :kb-id="kbId" items-only embedded
          @changed="onAllocChanged" />
        <div v-else class="org-depts__empty">
          保存单位档案后即可配置分摊子项
        </div>
      </template>
    </template>

    <div class="org-units__footer">
      <span class="org-units__tip">调整顺序后点击保存统一提交</span>
      <span class="org-units__grow" />
      <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { DeleteIcon } from 'tdesign-icons-vue-next'
import * as hrApi from './hrApi'
import TenantSettingsPanel from './TenantSettingsPanel.vue'
import { listKnowledgeBases } from '@/api/knowledge-base'

const props = withDefaults(defineProps<{ visible: boolean }>(), { visible: false })
const emit = defineEmits<{ (e: 'saved'): void }>()

interface Dept { id?: string; name: string; code: string }
interface Unit {
  id?: string
  name: string
  alias: string
  code: string
  credit_code: string
  address: string
  legal_person: string
  is_tenant: boolean
  tenant_no: string
  allocation_mode: string
  contact: string
  phone: string
  billing_tenant_id: string
  departments: Dept[]
}

const units = ref<Unit[]>([])
const activeIndex = ref(0)
const saving = ref(false)
const dragIndex = ref(-1)
const deptDragIndex = ref(-1)
const allocationOptions = [{ label: '按比例分摊', value: '按比例分摊' }]
const allocRef = ref<any>(null)
/** 分摊子项「导入市电账单字段」需要电费知识库；按名称解析一次即可。 */
const kbId = ref('')
async function resolveKbId() {
  if (kbId.value) return
  try {
    const res: any = await listKnowledgeBases()
    const list: any[] = Array.isArray(res?.data) ? res.data : (res?.data?.items || [])
    kbId.value = String(list.find((k) => String(k?.name || '').includes('电费'))?.id || '')
  } catch { /* 知识库不可用时只影响导入按钮 */ }
}
function onAllocChanged() {
  // 分摊子项落库后会反向同步「费用清单」租户分组字段，仅提示外层刷新即可
}

const activeUnit = computed<Unit | null>(() => units.value[activeIndex.value] || null)

watch(() => props.visible, (v) => { if (v) { void load(); void resolveKbId() } }, { immediate: true })

async function load() {
  try {
    const res: any = await hrApi.getOrgUnits()
    const d = res?.data || res || {}
    const list = Array.isArray(d.units) ? d.units : []
    const keepId = units.value[activeIndex.value]?.id || ''
    units.value = list.map((u: any) => ({
      id: u.id, name: u.name || '', alias: u.alias || '', code: u.code || '',
      credit_code: u.credit_code || '', address: u.address || '', legal_person: u.legal_person || '',
      is_tenant: !!u.is_tenant, tenant_no: u.tenant_no || '',
      allocation_mode: u.allocation_mode || '按比例分摊', contact: u.contact || '', phone: u.phone || '',
      billing_tenant_id: u.billing_tenant_id || '',
      departments: Array.isArray(u.departments) ? u.departments.map((x: any) => ({ id: x.id, name: x.name, code: x.code })) : [],
    }))
    // 保存后重载必须停在原单位（否则配置完分摊子项会被弹回第一家公司）
    const keep = keepId ? units.value.findIndex((u) => u.id === keepId) : -1
    activeIndex.value = keep >= 0 ? keep : 0
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载单位失败')
    units.value = []
  }
}

/**
 * 点击单位行切换折叠/展开（第 50 轮）。
 *
 * 与「字段设置」的分类行一致：点已选中的行就收起，再点展开。
 * 收起时 activeUnit 变 null，下方「单位信息 + 下属部门」整块隐藏。
 */
function toggleUnit(i: number) {
  activeIndex.value = activeIndex.value === i ? -1 : i
}

function addUnit() {
  units.value.push({
    name: '', alias: '', code: String(units.value.length + 1).padStart(2, '0'),
    credit_code: '', address: '', legal_person: '', is_tenant: false, tenant_no: '',
    allocation_mode: '按比例分摊', contact: '', phone: '', billing_tenant_id: '', departments: [],
  })
  activeIndex.value = units.value.length - 1
}

function removeUnit(i: number) {
  units.value.splice(i, 1)
  if (activeIndex.value >= units.value.length) activeIndex.value = Math.max(0, units.value.length - 1)
}

function addDept() {
  if (!activeUnit.value) return
  if (!activeUnit.value.departments) activeUnit.value.departments = []
  activeUnit.value.departments.push({ name: '', code: '' })
}

function removeDept(i: number) {
  const u = activeUnit.value
  if (!u) return
  ;(u.departments || []).splice(i, 1)
}

// ---- 单位拖拽排序 ----
function onUnitDragStart(i: number) { dragIndex.value = i }
function onUnitDrop(i: number) {
  const from = dragIndex.value
  if (from < 0 || from === i) { dragIndex.value = -1; return }
  const list = units.value.slice()
  const [moved] = list.splice(from, 1)
  list.splice(i, 0, moved)
  units.value = list
  activeIndex.value = i
  dragIndex.value = -1
}

// ---- 部门拖拽排序（替代数字输入框） ----
function onDeptDragStart(i: number) { deptDragIndex.value = i }
function onDeptDrop(i: number) {
  const u = activeUnit.value
  const from = deptDragIndex.value
  if (!u || from < 0 || from === i) { deptDragIndex.value = -1; return }
  const list = (u.departments || []).slice()
  const [moved] = list.splice(from, 1)
  list.splice(i, 0, moved)
  u.departments = list
  deptDragIndex.value = -1
}

async function save() {
  saving.value = true
  try {
    await hrApi.saveOrgUnits({
      units: units.value
        .filter((u) => String(u.name || '').trim())
        .map((u) => ({
          ...u,
          alias: String(u.alias || '').trim(),
          tenant_no: u.is_tenant ? String(u.tenant_no || '').trim() : '',
          allocation_mode: u.is_tenant ? (u.allocation_mode || '按比例分摊') : '',
          contact: u.is_tenant ? String(u.contact || '').trim() : '',
          phone: u.is_tenant ? String(u.phone || '').trim() : '',
          // 部门顺序 = 数组下标，后端按序写 sort_order
          departments: (u.departments || []).filter((d) => String(d.name || '').trim()),
        })),
    })
    // 第 53 轮：面板从「设置抽屉 → 组织机构」迁到左下角「公司设置」弹窗，
    // 提示文案同步成用户可见的入口名，避免出现已经不存在的位置描述。
    // 单位先落库（新开的租户会拿到 billing_tenant_id），再提交分摊子项草稿
    await load()
    await nextTick()
    await allocRef.value?.commitAll?.()
    MessagePlugin.success('单位与部门已保存')
    emit('saved')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<style lang="less" scoped>
.org-units { display: flex; flex-direction: column; gap: 12px; }

.org-units__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.org-units__head--dept { margin-top: 4px; }
.org-units__title { font-weight: 600; color: var(--td-text-color-primary); }
.org-units__title--form { margin-top: 4px; }

.org-units__list {
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  overflow: hidden;
}

.org-unit {
  display: grid;
  grid-template-columns: 20px 18px 1fr 80px 90px 56px;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  cursor: pointer;
  border-bottom: 1px solid var(--td-component-border);

  &:last-child { border-bottom: none; }
  &:hover { background: var(--td-bg-color-secondarycontainer); }
  &.is-active { background: var(--td-brand-color-light); }
}

.org-unit__drag,
.org-depts__drag {
  color: var(--td-text-color-placeholder);
  cursor: move;
  display: inline-flex;
  align-items: center;
}

.org-unit__chevron {
  display: inline-flex;
  align-items: center;
  color: var(--td-text-color-secondary);
}

.org-unit__name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.org-unit__code,
.org-unit__depts { color: var(--td-text-color-secondary); }
.org-unit__ops { display: inline-flex; justify-content: flex-end; }
.org-unit__ops :deep(.t-button) { color: var(--td-text-color-placeholder); }
.org-unit__ops :deep(.t-button:hover) { color: var(--td-error-color); }

.org-units__empty,
.org-depts__empty { padding: 16px; text-align: center; color: var(--td-text-color-placeholder); }

.org-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
}

.org-form__row { display: flex; align-items: center; gap: 8px; }
.org-form__label { flex: 0 0 120px; color: var(--td-text-color-secondary); }
.org-form__row :deep(.t-input) { flex: 1; min-width: 0; }
.org-form__switch { display: flex; align-items: center; gap: 10px; }
.org-form__hint { font-size: 12px; color: var(--td-text-color-placeholder); }
.org-units__head--alloc { margin-top: 6px; }
.org-units__sub { font-size: 12px; color: var(--td-text-color-secondary); }

.org-depts {
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  overflow: hidden;
}

.org-depts__head,
.org-dept {
  display: grid;
  grid-template-columns: 24px 1fr 140px 56px;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
}

.org-depts__head {
  color: var(--td-text-color-placeholder);
  border-bottom: 1px solid var(--td-component-border);
  background: var(--td-bg-color-secondarycontainer);
}

.org-dept { border-bottom: 1px solid var(--td-component-border); }
.org-dept:last-child { border-bottom: none; }
.org-dept :deep(.t-button) { color: var(--td-text-color-placeholder); }
.org-dept :deep(.t-button:hover) { color: var(--td-error-color); }

.org-units__footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--td-component-border);
}

.org-units__tip { color: var(--td-text-color-placeholder); }
.org-units__grow { flex: 1; }
</style>
