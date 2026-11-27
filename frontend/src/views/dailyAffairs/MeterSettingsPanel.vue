<template>
  <!--
    表计设置面板（电表 / 水表 / 气表）。

    来源：原 UtilityMeterTab 工具栏「表计配置」按钮打开的抽屉。
    为什么独立成组件：工具栏铁律只允许 3 个按钮（字段 / 设置 / 新增记录），
    表计档案与单价规则改为统一设置抽屉的「表计设置」Tab 内嵌渲染。
    组件只依赖 category，自身负责表计与用途的读写，变更后 emit('changed')
    让外层列表重新拉一次表计（记录表单的「表计」下拉要同步）。
  -->
  <div class="meter-settings-body">
    <!-- 第 76 轮：介质标签与「新增」按钮同一行左右两端对齐（按钮在右上角）。
         介质切换由本面板自持（父级 v-model:category），既保证同一行布局，
         也让切换时整块重置草稿（三介质互不串写）。 -->
    <div class="settings-head" data-meter-toolbar>
      <t-radio-group :model-value="props.category" variant="default-filled" size="small"
        data-meter-tabs @change="onCategoryChange">
        <t-radio-button value="electricity">电表</t-radio-button>
        <t-radio-button value="water">水表</t-radio-button>
        <t-radio-button value="gas">气表</t-radio-button>
      </t-radio-group>
      <t-button theme="primary" size="small" @click="openMeterForm(null)" data-meter-add>
        <template #icon><t-icon name="add" /></template>
        新增{{ meterLabel }}
      </t-button>
    </div>

    <div class="meter-table-wrap">
      <!-- 新增表计：表单展开在顶部 -->
      <div v-if="meterFormVisible && !meterForm.id" class="meter-form">
        <div class="meter-form-title">新增{{ meterLabel }}</div>
        <div class="form-grid">
          <div class="form-item">
            <label>别名 <span class="required">*</span></label>
            <t-input ref="meterAliasInput" v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
          </div>
          <div class="form-item">
            <label>表号</label>
            <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
          </div>
          <div class="form-item">
            <label>类型</label>
            <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" placeholder="选择类型" />
          </div>
          <div class="form-item">
            <label>表计类型</label>
            <t-input v-model="meterForm.meter_kind" placeholder="如：工商业" data-meter-kind-input />
          </div>
          <div class="form-item">
            <label>归属单位</label>
            <t-select v-model="meterForm.owner_unit" :options="companyOptions" filterable clearable
              placeholder="选择单位设置中的公司" />
          </div>
          <div class="form-item">
            <label>倍率</label>
            <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
          </div>
          <div class="form-item">
            <label>默认单价</label>
            <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
          </div>
          <div class="form-item">
            <label>使用单位</label>
            <t-select v-model="meterForm.use_unit" :options="companyOptions" filterable clearable
              placeholder="选择单位设置中的公司" />
          </div>
          <div class="form-item">
            <label>管理人员</label>
            <t-input v-model="meterForm.manager" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>联系方式</label>
            <t-input v-model="meterForm.contact" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>抄表方式</label>
            <t-radio-group v-model="meterForm.meter_mode">
              <t-radio-button value="auto">自动抄表</t-radio-button>
              <t-radio-button value="manual">手动抄表</t-radio-button>
            </t-radio-group>
          </div>
          <div class="form-item">
            <label>安装日期</label>
            <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD"
              placeholder="选填" clearable style="width: 100%" />
          </div>
        </div>
        <div class="form-item form-item--full">
          <label>备注</label>
          <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
        </div>
        <!-- 第 77 轮：内嵌抽屉时不再有第二套保存/取消，只留「收起」把草稿并回列表 -->
        <div class="meter-form-actions">
          <template v-if="props.embedded">
            <t-button variant="text" size="small" @click="foldMeterForm">收起</t-button>
          </template>
          <template v-else>
            <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
            <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
          </template>
        </div>
      </div>

      <!-- 分组标签：宿舍 / 工商业 / 自定义用途（仅导航，表格单表渲染保证拖拽稳定） -->
      <!-- 第 73 轮：不再有「工商业 N」子标签栏（只剩一类，标签没有信息量） -->

      <div v-if="activeGroupItems.length" :ref="setMeterSortableRef" class="meter-table">
        <div class="meter-table-head">
          <span class="meter-drag-th"><t-icon name="move" size="14px" /></span><span>别名</span><span>表号</span><span>表计类型</span><span>倍率</span><span>单价</span><span>归属单位</span><span>状态</span><span>操作</span>
        </div>
        <template v-for="m in activeGroupItems" :key="m.id">
          <div class="meter-table-row" :data-id="m.id" :class="{ editing: meterFormVisible && meterForm.id === m.id }" @click="toggleMeterEdit(m)">
            <span class="meter-drag-handle" title="拖动排序" @click.stop><t-icon name="move" size="14px" /></span>
            <span class="mtr-alias">{{ m.alias }}</span>
            <span class="mtr-mono">{{ m.meter_no || '—' }}</span>
            <span class="mtr-type" :title="meterTypeLabel(m.meter_type)">{{ meterKindLabel(m.meter_kind) }}</span>
            <span class="mtr-mono">{{ fmtNum(m.rate) }}</span>
            <span class="mtr-mono">{{ fmtUnitPrice(m.default_unit_price) }} 元/{{ unitLabel }}</span>
            <span class="mtr-owner">{{ normalizeCompanyName(m.owner_unit, companyNames) || '—' }}</span>
            <span class="mtr-switch" @click.stop>
              <t-switch :model-value="!!m.enabled" size="small" @update:model-value="(v: any) => toggleEnabled(m, !!v)" />
            </span>
            <span class="meter-row-actions" @click.stop>
              <t-popconfirm theme="warning" :content="`确定删除${meterLabel}「${m.alias}」吗？`"
                :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="deleteMeter(m)">
                <t-button variant="text" size="small" class="biz-del-btn" @click.stop>
                  <template #icon><t-icon name="delete" size="16px" /></template>
                </t-button>
              </t-popconfirm>
            </span>
          </div>
          <!-- 编辑表计：点击行展开，再点折叠 -->
          <div v-if="meterFormVisible && meterForm.id === m.id" class="meter-form meter-form--inline">
            <div class="meter-form-title">编辑{{ meterLabel }}</div>
            <div class="form-grid">
              <div class="form-item">
                <label>别名 <span class="required">*</span></label>
                <t-input v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
              </div>
              <div class="form-item">
                <label>表号</label>
                <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
              </div>
              <div class="form-item">
                <label>类型</label>
                <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" placeholder="选择类型" />
              </div>
              <div class="form-item">
                <label>表计类型</label>
                <t-input v-model="meterForm.meter_kind" placeholder="如：工商业" data-meter-kind-input />
              </div>
              <div class="form-item">
                <label>归属单位</label>
                <t-select v-model="meterForm.owner_unit" :options="companyOptions" filterable clearable
                  placeholder="选择单位设置中的公司" />
              </div>
              <div class="form-item">
                <label>倍率</label>
                <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
              </div>
              <div class="form-item">
                <label>默认单价</label>
                <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
              </div>
              <div class="form-item">
                <label>使用单位</label>
                <t-select v-model="meterForm.use_unit" :options="companyOptions" filterable clearable
                  placeholder="选择单位设置中的公司" />
              </div>
              <div class="form-item">
                <label>管理人员</label>
                <t-input v-model="meterForm.manager" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>联系方式</label>
                <t-input v-model="meterForm.contact" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>抄表方式</label>
                <t-radio-group v-model="meterForm.meter_mode">
                  <t-radio-button value="auto">自动抄表</t-radio-button>
                  <t-radio-button value="manual">手动抄表</t-radio-button>
                </t-radio-group>
              </div>
              <div class="form-item">
                <label>安装日期</label>
                <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD"
              placeholder="选填" clearable style="width: 100%" />
              </div>
            </div>
            <div class="form-item form-item--full">
              <label>备注</label>
              <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
            </div>
            <div class="meter-form-actions">
              <template v-if="props.embedded">
                <t-button variant="text" size="small" @click="foldMeterForm">收起</t-button>
              </template>
              <template v-else>
                <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
              </template>
            </div>
          </div>
        </template>
      </div>
      <!-- 唯一空态：只有「加载完成且确实一条都没有」才渲染。
           第 76 轮修复：旧实现上方还有一块 !meters.length 的空态，
           请求中两块同时出现（双图标 + 两段「暂无」），切介质时频繁闪跳。 -->
      <div v-if="!metersLoading && !activeGroupItems.length" class="meter-empty" data-meter-empty>
        <t-icon name="setting" size="28px" class="meter-empty-icon" />
        <span class="meter-empty-text">暂无{{ meterLabel }}</span>
      </div>
      <div v-else-if="metersLoading" class="meter-loading" data-meter-loading>
        <t-loading size="small" />
      </div>
    </div>

    <!-- 第 73 轮：用途管理整体下线。工业表计只剩「工商业」一类，
         保留入口却无处可用只会增加误操作面。 -->
    <!-- 第 70 轮：底部固定操作栏（与「字段定义」Tab 的保存/取消 100% 同款）。
         表计的新增/编辑/排序/启用全部改为**草稿态**，点「保存」一次性落库。 -->
    <div v-if="!props.embedded" class="meter-save-bar" data-meter-save-bar>
      <t-button theme="default" variant="outline" size="small" @click="cancelAllMeters(true)"
        data-meter-cancel>取消</t-button>
      <t-button theme="primary" size="small" :loading="savingAll" @click="saveAllMeters(true)"
        data-meter-save>保存</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import Sortable from 'sortablejs'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeters,
  createUtilityMeter,
  updateUtilityMeter,
  deleteUtilityMeter,
  sortUtilityMeters,
  listUtilityKinds,
  createUtilityKind,
  updateUtilityKind,
  deleteUtilityKind,
} from '@/api/knowledge-base'
import { getOrgUnits } from './hrApi'
import { companyNamesOf, normalizeCompanyName } from './companyScope'

type MeterCategory = 'electricity' | 'water' | 'gas'
const props = defineProps<{
  category: MeterCategory
  /**
   * 第 74 轮：内嵌在统一设置抽屉的「工业表计」平级标签里。
   * 底部唯一的取消/保存由抽屉底栏承担，本面板不再渲染自己的保存栏。
   */
  embedded?: boolean
}>()
const emit = defineEmits<{
  (e: 'changed'): void
  (e: 'close'): void
  (e: 'update:category', v: MeterCategory): void
}>()

const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtUnitPrice = (v: any) => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return String(Math.round(n * 1000) / 1000)
}

const meters = ref<any[]>([])
/**
 * 加载态：切介质或首次进入时置 true。
 * 空态必须等 loading=false 才允许渲染，否则先闪一下「暂无电表」再出数据。
 */
const metersLoading = ref(true)
function onCategoryChange(v: any) {
  const next = String(v || '') as MeterCategory
  if (!next || next === props.category) return
  emit('update:category', next)
}
const panelSearch = ref('')
const savingMeter = ref(false)
const meterFormVisible = ref(false)
const kindManageVisible = ref(false)
const meterForm = ref<any>({})
// 第 63 轮：归属 / 使用单位改为「单位设置」下拉（不再自由文本），存量简称归一
const orgUnits = ref<any[]>([])
const companyNames = computed(() => companyNamesOf(orgUnits.value))
const companyOptions = computed(() => companyNames.value.map((v) => ({ label: v, value: v })))
async function loadCompanyOptions() {
  try {
    const res: any = await getOrgUnits()
    const d = res?.data ?? res
    const list = Array.isArray(d) ? d : (Array.isArray(d?.units) ? d.units : [])
    orgUnits.value = list
  } catch { /* 单位接口失败时下拉为空，不阻断表计维护 */ }
}

// 计量层级按类别: 电表 普通/分时; 水表 总表/分表/消防; 气表 普通
const meterTypeOptions = computed(() => {
  if (props.category === 'electricity') return [
    { label: '普通', value: 'normal' },
    { label: '分时', value: 'time' },
  ]
  if (props.category === 'water') return [
    { label: '总表', value: 'total' },
    { label: '分表', value: 'sub' },
    { label: '消防', value: 'fire' },
  ]
  return [{ label: '普通', value: 'normal' }]
})
const meterTypeLabel = (t: string) => meterTypeOptions.value.find(o => o.value === t)?.label || '普通'
const meterAliasInput = ref()

// 用途体系:内置 宿舍/工商业(锁定 value 驱动租户核算计算,label 可改名) + 用户自定义
const KIND_BUILTIN = reactive([
  { value: 'dorm', label: '宿舍', builtin: true },
  { value: 'production', label: '工商业', builtin: true },
])
type KindItem = { id?: string; value: string; label: string }
const customKinds = ref<KindItem[]>([])
const meterKinds = computed(() => [...KIND_BUILTIN, ...customKinds.value])
/**
 * 第 70 轮：表计设置只管理工商业表计（宿舍/公租房表在房源档案里定义）。
 * 第 77 轮：表计类型改为自由文本输入（默认「工商业」），不再用下拉枚举，
 * 因此这里只保留 label 映射用于展示历史行。
 */
const meterKindLabel = (k: string) => meterKinds.value.find(o => o.value === k)?.label || String(k || '').trim() || '未分类'

const filteredMeters = computed(() => {
  const kw = panelSearch.value.trim().toLowerCase()
  if (!kw) return meters.value
  return meters.value.filter((m: any) =>
    (m.alias || '').toLowerCase().includes(kw) || (m.meter_no || '').toLowerCase().includes(kw))
})
/**
 * 第 70 轮：水电气表的定义权已移回「宿舍 / 公租房档案」。
 *
 * 表计设置**只保留工商业分组**（工业总表 / 分表 / 消防水表 / 食堂气表）——
 * 宿舍电表、宿舍水表、公租房水表/气表在房源档案里维护，核算侧实时引用。
 * utility_meters 里的历史记录**不删除**：抄表与费用核算仍按 meter_no 引用它，
 * 这里只是把它们从设置界面移出（改价去房源档案）。
 */
const METER_KINDS_IN_SETTINGS70 = new Set(['工商业', 'production'])
const meterGroups = computed(() => {
  const groups = meterKinds.value
    .filter((k) => METER_KINDS_IN_SETTINGS70.has(k.value))
    .map(k => ({ key: k.value, label: k.label, items: [] as any[] }))
  for (const m of filteredMeters.value) {
    if (!METER_KINDS_IN_SETTINGS70.has(String(m.meter_kind || ''))) continue
    const g = groups.find(g => g.key === m.meter_kind)
    if (g) g.items.push(m)
  }
  return groups
})
const activeMeterGroup = ref<string>('production')
const activeGroupItems = computed(() => meterGroups.value.find(g => g.key === activeMeterGroup.value)?.items || [])
watch(meterGroups, (gs) => {
  if (!gs.find(g => g.key === activeMeterGroup.value)) {
    activeMeterGroup.value = (gs[0]?.key as any) || 'dorm'
  }
})

const sortMetersLocal = () => {
  meters.value = [...meters.value].sort((a: any, b: any) =>
    (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0) ||
    (a.created_at || '').localeCompare(b.created_at || ''))
}
const loadMetersOnly = async () => {
  // 加载期间不允许渲染任何空态：切介质时先闪「暂无」再出数据正是抖动来源。
  metersLoading.value = true
  try {
    const res: any = await listUtilityMeters({ category: props.category })
    const mets = res?.data || res || []
    meters.value = Array.isArray(mets) ? mets : []
    sortMetersLocal()
  } catch { /* ignore */ } finally {
    metersLoading.value = false
  }
}

const loadKinds = async () => {
  try {
    const res: any = await listUtilityKinds(props.category)
    const list = res?.data || []
    if (Array.isArray(list) && list.length) {
      customKinds.value = list
        .filter((k: any) => !KIND_BUILTIN.some(b => b.value === k.value))
        .map((k: any) => ({ id: k.id, value: k.value, label: k.label }))
      for (const k of list) {
        const b = KIND_BUILTIN.find(x => x.value === k.value)
        if (b && k.label) b.label = k.label
      }
    }
  } catch (e) {
    // 用途列表是辅助数据：网络抖动时不要打 console.error（会污染真机门禁），
    // 表格与表计读写不依赖它。
    console.warn('load kinds failed', e)
  }
}

const _addCustomKind = async () => {
  const n = customKinds.value.length + 1
  const value = `custom${n}`
  const label = `自定义${n}`
  try {
    const res: any = await createUtilityKind(props.category, { value, label })
    const item = res?.data || res
    if (item?.id) customKinds.value.push({ id: item.id, value: item.value, label: item.label })
    else customKinds.value.push({ value, label })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '新增用途失败')
  }
}
const _removeCustomKind = async (value: string) => {
  // 引用检查:有表计正在使用该用途时禁止删除
  const used = meters.value.filter(m => String(m.meter_kind) === value)
  if (used.length) {
    MessagePlugin.warning(`该用途已被 ${used.length} 个表计引用,请先删除或修改这些表计的用途后再删除`)
    return
  }
  const item = customKinds.value.find(k => k.value === value)
  if (!item) return
  if (item.id) {
    try {
      await deleteUtilityKind(item.id)
    } catch (e: any) {
      MessagePlugin.error(e?.message || '删除失败')
      return
    }
  }
  customKinds.value = customKinds.value.filter(k => k.value !== value)
}
const savingKinds = ref(false)
// 保存用途配置：内置用途 upsert 到后端（改名持久化），自定义用途按 id 更新
const _saveKinds = async () => {
  savingKinds.value = true
  try {
    for (const k of meterKinds.value) {
      const label = (k.label || '').trim()
      if (!label) {
        MessagePlugin.warning('用途名称不能为空')
        return
      }
      if ((k as any).builtin) {
        await createUtilityKind(props.category, { value: k.value, label })
      } else if ((k as any).id) {
        if (label !== k.label) k.label = label
        await updateUtilityKind((k as any).id, { label })
      }
    }
    MessagePlugin.success('用途配置已保存')
    kindManageVisible.value = false
    await loadKinds()
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingKinds.value = false
  }
}

const meterNoPrefix = () => (props.category === 'water' ? 'W' : props.category === 'electricity' ? 'E' : 'G')
const nextMeterNo = () => {
  let max = 0
  for (const m of meters.value) {
    const mm = String(m.meter_no || '').match(/^[A-Za-z]*(\d+)$/)
    if (mm) max = Math.max(max, parseInt(mm[1], 10))
  }
  return meterNoPrefix() + String(max + 1).padStart(3, '0')
}

const openMeterForm = (m: any) => {
  // 新增默认用途:水表(总表/分表/消防均属工商业)默认工商业;电表/气表默认居民
  const defaultKind = '工商业'
  // 新增默认类型:电表/气表默认普通;水表类型(总表/分表/消防)由用户按需选择
  const defaultType = props.category === 'water' ? '' : 'normal'
  meterForm.value = m ? {
    id: m.id,
    alias: m.alias || '',
    meter_no: m.meter_no || '',
    // 水表历史脏数据 normal 不在合法类型内,编辑时置空引导重选
    meter_type: props.category === 'water' && !['total', 'sub', 'fire'].includes(m.meter_type)
      ? '' : (m.meter_type || ''),
    meter_kind: m.meter_kind || defaultKind,
    // 存量简称（重庆星达 / 持睿汽车）回显为全称，保存后彻底对齐
    owner_unit: normalizeCompanyName(m.owner_unit, companyNames.value),
    rate: Number(m.rate) > 0 ? m.rate : 1,
    default_unit_price: Number(m.default_unit_price) || 0,
    use_unit: normalizeCompanyName(m.use_unit, companyNames.value),
    manager: m.manager || '',
    contact: m.contact || '',
    meter_mode: m.meter_mode || 'manual',
    install_date: m.install_date || '',
    remark: m.remark || '',
    enabled: m.enabled !== false,
  } : {
    id: '', alias: '', meter_no: nextMeterNo(), meter_type: defaultType,
    meter_kind: defaultKind || '工商业', owner_unit: '', rate: 1, default_unit_price: 0,
    use_unit: '', manager: '', contact: '', meter_mode: 'manual',
    install_date: '', remark: '', enabled: true,
  }
  meterFormVisible.value = true
  nextTick(() => { if (!m) (meterAliasInput.value as any)?.focus?.() })
}
const toggleMeterEdit = (m: any) => {
  if (meterFormVisible.value && meterForm.value.id === m.id) {
    meterFormVisible.value = false
    return
  }
  openMeterForm(m)
}

const saveMeter = async () => {
  if (!meterForm.value.alias?.trim()) {
    MessagePlugin.warning('请填写别名')
    return
  }
  savingMeter.value = true
  try {
    const payload = {
      category: props.category,
      alias: meterForm.value.alias.trim(),
      meter_no: meterForm.value.meter_no || '',
      meter_type: meterForm.value.meter_type || '',
      meter_kind: String(meterForm.value.meter_kind || '').trim() || '工商业',
      owner_unit: normalizeCompanyName(meterForm.value.owner_unit, companyNames.value),
      rate: Number(meterForm.value.rate) > 0 ? Number(meterForm.value.rate) : 1,
      default_unit_price: Number(meterForm.value.default_unit_price) || 0,
      use_unit: normalizeCompanyName(meterForm.value.use_unit, companyNames.value),
      manager: meterForm.value.manager || '',
      contact: meterForm.value.contact || '',
      meter_mode: meterForm.value.meter_mode || 'manual',
      install_date: meterForm.value.install_date || '',
      remark: meterForm.value.remark || '',
      enabled: meterForm.value.enabled !== false,
    }
    const isEdit = !!meterForm.value.id
    if (isEdit) {
      await updateUtilityMeter(meterForm.value.id, payload)
      MessagePlugin.success('已保存')
    } else {
      await createUtilityMeter(payload)
      MessagePlugin.success('已新增')
    }
    // 保存时一并提交此前拖拽产生的顺序（第 19 轮：拖拽不再自动落库）
    if (pendingSortIds.value.length) {
      try {
        await sortUtilityMeters(props.category, pendingSortIds.value)
        pendingSortIds.value = []
      } catch { /* 顺序提交失败不阻断表计保存本身 */ }
    }
    await loadMetersOnly()
    emit('changed')
    if (isEdit) {
      meterFormVisible.value = false
    } else {
      // 连续新增：保持表单展开，表号自动递增，清空其余字段并聚焦别名
      meterForm.value = {
        id: '',
        alias: '',
        meter_no: nextMeterNo(),
        meter_type: props.category === 'water' ? '' : 'normal',
        meter_kind: '工商业',
        owner_unit: '',
        rate: '',
        default_unit_price: '',
        use_unit: '',
        manager: '',
        contact: '',
        meter_mode: 'manual',
        install_date: '',
        remark: '',
        enabled: true,
      }
      meterFormVisible.value = true
      nextTick(() => { (meterAliasInput.value as any)?.focus?.() })
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}

/**
 * 表计启用开关：只改本地（第 19 轮）。
 *
 * 旧实现每次拨动就 PUT 一次，用户在配置表计档案时顺手关几个就会连发请求。
 * 现在开关状态随该表计的「保存」按钮一起提交（见 saveMeter）。
 */
const dirtyEnabled = ref<Set<string>>(new Set())
const toggleEnabled = (m: any, v: any) => {
  m.enabled = !!v
  // 第 70 轮：开关只记脏，随底部「保存」统一提交（不再静默自动落库）
  const next = new Set(dirtyEnabled.value)
  next.add(String(m.id))
  dirtyEnabled.value = next
}

/** 行数据 → 更新接口的完整载荷（后端要求 alias 必填、rate>0）。 */
function meterPayloadOf(m: any) {
  return {
    category: props.category,
    alias: String(m?.alias || '').trim(),
    meter_no: m?.meter_no || '',
    meter_type: m?.meter_type || '',
    meter_kind: m?.meter_kind || 'production',
    owner_unit: normalizeCompanyName(m?.owner_unit, companyNames.value),
    rate: Number(m?.rate) > 0 ? Number(m.rate) : 1,
    default_unit_price: Number(m?.default_unit_price) || 0,
    use_unit: normalizeCompanyName(m?.use_unit, companyNames.value),
    manager: m?.manager || '',
    contact: m?.contact || '',
    meter_mode: m?.meter_mode || 'manual',
    install_date: m?.install_date || '',
    remark: m?.remark || '',
    enabled: m?.enabled !== false,
  }
}

const deleteMeter = async (m: any) => {
  try {
    await deleteUtilityMeter(m.id)
    MessagePlugin.success('已删除')
    meters.value = meters.value.filter(x => x.id !== m.id)
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

const savingAll = ref(false)
/**
 * 第 70 轮：抽屉底部「保存」= 一次性提交本面板的全部待存变更。
 *
 * 覆盖三类：展开中的编辑表单（含新增）、启用开关、拖拽排序。
 * 提交后刷新列表、通知父级重算，并关闭抽屉；「取消」直接关闭、丢弃草稿。
 */
/**
 * 第 77 轮：内嵌模式下「收起」= 把编辑表单并入内存草稿列表，不落库。
 * 抽屉底栏「保存」时统一创建/更新，切标签不会静默写库。
 */
function foldMeterForm(): boolean {
  const alias = String(meterForm.value?.alias || '').trim()
  if (!alias) { MessagePlugin.warning('请输入别名'); return false }
  const copy = { ...meterForm.value, alias }
  if (copy.id) {
    const i = meters.value.findIndex((x: any) => String(x.id) === String(copy.id))
    if (i >= 0) meters.value[i] = { ...meters.value[i], ...copy }
  } else {
    // 草稿行：id 为空，保存时走 create；插到列表首位便于用户核对
    meters.value = [{ ...copy, id: '', category: props.category, _draft: true }, ...meters.value]
  }
  sortMetersLocal()
  meterFormVisible.value = false
  return true
}

async function saveAllMeters(closeAfter = true) {
  savingAll.value = true
  try {
    if (meterFormVisible.value && !foldMeterForm()) return
    // 草稿行（本次新增、尚未落库）先创建
    for (const draft of meters.value.filter((m: any) => !m.id)) {
      await createUtilityMeter(meterPayloadOf(draft))
    }
    meters.value = meters.value.filter((m: any) => !!m.id)
    for (const id of Array.from(dirtyEnabled.value)) {
      const m = meters.value.find((x: any) => String(x.id) === id)
      // 第 75 轮：只提交**本介质**的表计（meters 已按 category 加载；
      // 再比一次 category 作为双保险，绝不允许电表草稿写进水/气表）。
      if (!m || String(m.category || props.category) !== String(props.category)) continue
      await updateUtilityMeter(id, meterPayloadOf(m))
    }
    dirtyEnabled.value = new Set()
    if (pendingSortIds.value.length) {
      try {
        await sortUtilityMeters(props.category, pendingSortIds.value)
      } catch { /* 顺序失败不阻断保存 */ }
      pendingSortIds.value = []
    }
    await loadMetersOnly()
    emit('changed')
    MessagePlugin.success('已保存')
    if (closeAfter) emit('close')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingAll.value = false
  }
}

/** 取消：丢弃草稿（未保存的开关/排序/表单），直接关闭抽屉。 */
function cancelAllMeters(closeAfter = true) {
  dirtyEnabled.value = new Set()
  pendingSortIds.value = []
  meterFormVisible.value = false
  if (closeAfter) emit('close')
}

// 第 74 轮：统一设置抽屉底栏「保存 / 取消」驱动本面板（不再自带保存栏）。
defineExpose({
  commitAll: () => saveAllMeters(false),
  discardAll: () => cancelAllMeters(false),
})


// ---- 表计拖动排序（sort_order 持久化） ----
const meterSortableEl = ref<HTMLElement | null>(null)
const setMeterSortableRef = (el: any) => {
  meterSortableEl.value = Array.isArray(el) ? (el[0] || null) : (el || null)
}
let sortableInst: any = null
const initMeterSortable = () => {
  const el = meterSortableEl.value
  if (!el) return
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
  try {
    sortableInst = Sortable.create(el, {
      draggable: '.meter-table-row',
      handle: '.meter-drag-handle',
      animation: 150,
      ghostClass: 'meter-row-ghost',
      onEnd: persistMeterSort,
    })
  } catch (e) {
    console.error('[Sortable] init failed:', e)
  }
}
/**
 * 拖拽结束：**只本地生效**（第 19 轮）。
 *
 * 旧实现在 onEnd 里直接调 sortUtilityMeters，一放下鼠标就落库。
 * 现在顺序改动记在 pendingSortIds 里，等用户点「保存」（saveMeter）时一并提交，
 * 避免「还没排完就已经写库」以及连续拖拽产生的并发请求。
 */
const pendingSortIds = ref<string[]>([])
const persistMeterSort = () => {
  const el = meterSortableEl.value
  if (!el) return
  const ids = [...el.querySelectorAll('.meter-table-row')]
    .map(r => r.getAttribute('data-id')).filter(Boolean) as string[]
  pendingSortIds.value = ids
  // 不再自动提交：sortUtilityMeters 由 saveMeter 在保存时调用
  void sortUtilityMeters
}
watch([activeMeterGroup, () => activeGroupItems.value], async () => {
  await nextTick()
  setTimeout(initMeterSortable, 120)
})

/**
 * 第 75 轮：介质切换必须**整块重置**。
 *
 * 旧实现没有这个 watch：在「工业表计」里点水表/气表时，面板仍持有电表的
 * meters 草稿；用户一保存，就把电表 id 连同 water 载荷写回去（三表串写），
 * 水表/气表列表也一直显示电表的 6 张表。
 */
watch(() => props.category, async () => {
  meterFormVisible.value = false
  dirtyEnabled.value = new Set()
  pendingSortIds.value = []
  meters.value = []
  await Promise.all([loadMetersOnly(), loadKinds()])
  await nextTick()
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
  setTimeout(initMeterSortable, 120)
})

onMounted(async () => {
  await Promise.all([loadMetersOnly(), loadKinds(), loadCompanyOptions()])
  await nextTick()
  setTimeout(initMeterSortable, 200)
})
onBeforeUnmount(() => {
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
})
</script>

<style lang="less" scoped>
.meter-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 28px 0;
  color: var(--td-text-color-placeholder);
  font-size: 13px;

  .meter-empty-icon { color: var(--td-text-color-placeholder); }
  .meter-empty-text { color: var(--td-text-color-secondary); }
}

.meter-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 28px 0;
}

.kind-manage-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.kind-manage-tip {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
  margin: 0 0 4px;
}
.kind-manage-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
.kind-manage-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.kind-manage-row .t-input { flex: 1; }
.kind-value-tag {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  margin-right: 4px;
}
.kind-builtin-tag {
  width: 32px;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.meter-search { margin-bottom: 12px; }
.meter-settings-body { padding: 4px 0 24px; }

.settings-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.meter-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.meter-settings-tabs {
  margin-bottom: 10px;

  :deep(.t-tabs__header) { margin-bottom: 8px; }
  :deep(.t-tabs__nav-item) { font-size: 13px; }
  :deep(.t-tabs__nav-item-wrapper) { padding: 0; margin: 0; }
  :deep(.t-tabs__nav-item:not(:first-child) .t-tabs__nav-item-wrapper) { margin-left: 8px; }
  :deep(.t-tabs__content) { overflow: visible; }
}

.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);

  .meter-table-head,
  .meter-table-row {
    display: grid;
    grid-template-columns: 0.4fr 1.4fr 1fr 1.2fr 0.7fr 1.2fr 1fr 0.7fr 1.2fr;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    font-size: 12px;
  }

  .meter-drag-th {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
  }

  .meter-table-head {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .meter-table-row {
    border-bottom: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    cursor: pointer;
    transition: background 0.15s;

    .meter-drag-handle {
      display: flex;
      align-items: center;
      justify-content: center;
      color: var(--td-text-color-placeholder);
      cursor: grab;

      &:active { cursor: grabbing; }
    }

    &:hover { background: var(--td-bg-color-secondarycontainer); }
    &.editing { background: var(--td-brand-color-light); }
    &:last-child { border-bottom: none; }

    .mtr-alias {
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mtr-mono {
      font-variant-numeric: tabular-nums;
      color: var(--td-text-color-primary);
    }
    .mtr-type {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--td-text-color-secondary);
    }
    .mtr-owner {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mtr-switch {
      display: flex;
      align-items: center;
      justify-self: start;

      .t-switch { width: auto; }
    }

    .meter-row-actions {
      display: flex;
      align-items: center;
      gap: 2px;
      justify-content: flex-start;
    }
  }

  .meter-row-ghost {
    opacity: 0.45;
    background: var(--td-brand-color-light);
  }
}

.meter-form {
  margin-top: 0;
  grid-column: 1 / -1;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 14px;
  background: var(--td-bg-color-secondarycontainer);

  &.meter-form--inline {
    margin: 0;
    border-left: none;
    border-right: none;
    border-bottom: none;
    border-radius: 0;
    grid-column: auto;
  }

  .meter-form-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    color: var(--td-text-color-primary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
  }

  .form-item {
    label {
      display: block;
      font-size: 12px;
      color: var(--td-text-color-secondary);
      margin-bottom: 4px;

      .required { color: var(--td-error-color); }
    }

    &--full { margin-top: 12px; }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }
}
</style>

<style>
/* 删除图标统一口径（16px 灰→红悬停 + 浅红底 + .15s），全系统共用一份 */
@import './styles/business-delete-btn.css';

/* 第 70 轮：表计设置底部固定操作栏 */
.meter-save-bar {
  position: sticky;
  bottom: 0;
  z-index: 6;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
