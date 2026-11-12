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
    </template>

    <div class="org-units__footer">
      <span class="org-units__tip">调整顺序后点击保存统一提交</span>
      <span class="org-units__grow" />
      <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { DeleteIcon } from 'tdesign-icons-vue-next'
import * as hrApi from './hrApi'

const props = withDefaults(defineProps<{ visible: boolean }>(), { visible: false })
const emit = defineEmits<{ (e: 'saved'): void }>()

interface Dept { id?: string; name: string; code: string }
interface Unit { id?: string; name: string; code: string; credit_code: string; address: string; legal_person: string; departments: Dept[] }

const units = ref<Unit[]>([])
const activeIndex = ref(0)
const saving = ref(false)
const dragIndex = ref(-1)
const deptDragIndex = ref(-1)

const activeUnit = computed<Unit | null>(() => units.value[activeIndex.value] || null)

watch(() => props.visible, (v) => { if (v) void load() }, { immediate: true })

async function load() {
  try {
    const res: any = await hrApi.getOrgUnits()
    const d = res?.data || res || {}
    const list = Array.isArray(d.units) ? d.units : []
    units.value = list.map((u: any) => ({
      id: u.id, name: u.name || '', code: u.code || '', credit_code: u.credit_code || '',
      address: u.address || '', legal_person: u.legal_person || '',
      departments: Array.isArray(u.departments) ? u.departments.map((x: any) => ({ id: x.id, name: x.name, code: x.code })) : [],
    }))
    activeIndex.value = 0
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
    name: '', code: String(units.value.length + 1).padStart(2, '0'),
    credit_code: '', address: '', legal_person: '', departments: [],
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
          // 部门顺序 = 数组下标，后端按序写 sort_order
          departments: (u.departments || []).filter((d) => String(d.name || '').trim()),
        })),
    })
    // 第 53 轮：面板从「设置抽屉 → 组织机构」迁到左下角「公司设置」弹窗，
    // 提示文案同步成用户可见的入口名，避免出现已经不存在的位置描述。
    MessagePlugin.success('单位与部门已保存')
    emit('saved')
    await load()
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
