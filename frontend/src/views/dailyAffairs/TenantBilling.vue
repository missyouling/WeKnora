<template>
  <div class="tenant-billing-container">
    <!-- 筛选工具栏：与发票/合同/制度/通知、市电账单同一个 BusinessFilterBar
         （搜索全部字段 220px/32px、控件 32px、gap 8px）。
         右侧固定 字段 → 设置 → 主操作 三按钮；主操作带 loading/title，
         中台 primaryAction 插槽不带这两个属性，因此放 right-extra
         （BusinessListToolbar 渲染顺序：columns → right-extra → primaryAction）。 -->
    <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
      :columns="activeColDefs" :visible-keys="visibleKeys" :selected-count="selectedKeys.size"
      hide-batch-bar @columns-change="(v: string[]) => { visibleKeys = v; persistColumns() }"
      @reset-columns="resetColumns" @select-all-columns="selectAllColumns"
      @filter-change="onBarFilterChange" @clear-selection="clearSelection">
      <template #right-extra>
        <t-button variant="outline" size="small" @click="openFieldSettings">
          <template #icon><t-icon name="setting" size="14px" /></template>
          设置
        </t-button>
        <!-- 二次确认是硬要求：生成会覆盖该月账单，误点代价高。
             文案区分「所选 N 条」与「未选择 → 全部」，让用户清楚影响范围。 -->
        <t-popconfirm theme="warning" :content="generateConfirmText" placement="bottom-right"
          :confirm-btn="{ content: '确认生成', theme: 'primary' }" :cancel-btn="{ content: '取消' }"
          @confirm="handleGenerateAll">
          <t-button theme="primary" size="small" :loading="generateBusy"
            :title="isOwnerView ? '重新加载账单与表计数据并重新核算列表（数据有误时可随时点击）' : '按所选记录生成账单；未选择则全部重新生成（数据不全时禁止生成）'">
            <template #icon><t-icon name="refresh" size="14px" /></template>
            {{ isOwnerView ? '重新核算' : '生成账单' }}
          </t-button>
        </t-popconfirm>
      </template>
    </BusinessFilterBar>

    <!-- 列表 -->
    <div class="doc-list-scroll tenant-list-scroll">
      <div class="doc-list-view" :style="headerFixStyle">
        <t-table
        :data="displayRows"
        :columns="tableColumns"
        row-key="id"
        size="small"
        :hover="true"
        :loading="loading"
        max-height="100%"
        sticky-header
        class="doc-table"
        :selected-row-keys="selectedRowKeysArr"
        select-on-change
        :row-class-name="({ row }: any) => selectedKeys.has(row.id) ? 'row-selected' : ''"
        @row-click="({ row }: any) => onRowClick(row)"
        @select-change="(val: any) => onTableSelectChange(val)"
      >
        <template #month="{ row }: any">
          <span class="row-mono">{{ row.month }}</span>
        </template>
        <template #ratio="{ row }: any">
          <span class="row-mono">{{ fmtRatio(row.ratio) }}</span>
        </template>
        <template #ind_price="{ row }: any">
          <span class="row-mono">{{ fmtPrice(row.ind_price) }}</span>
        </template>
        <template #empty>
          <EmptyState v-if="!loading" title="暂无数据" icon="search-error" compact />
        </template>
      </t-table>
    </div>
      <!-- 底部汇总：与发票管理一致，位于白色列表卡片（.doc-list-scroll）内部最底端 -->
      <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <template v-if="isOwnerView">
        <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
        <span class="doc-summary-item">总电量 <span class="doc-summary-val">{{ fmtKwh(summaryKwh) }}</span> 千瓦时</span>
        <span class="doc-summary-item">总电费 <span class="doc-summary-val">{{ fmtMoney(summaryFee) }}</span> 元</span>
      </template>
      <template v-else>
        <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条记录</span>
        <span class="doc-summary-item">总应付电费 <span class="doc-summary-val">{{ fmtMoney(summaryPayableFee) }}</span> 元</span>
        <span class="doc-summary-item">总应付水费 <span class="doc-summary-val">{{ fmtMoney(summaryWaterFee) }}</span> 元</span>
      </template>
    </div>
    </div>

    <!-- 底部浮动工具栏 -->
    <transition name="batch-bar-fade">
      <div v-if="selectedKeys.size && !printVisible" class="doc-batch-bar-fixed" role="region">
        <div class="batch-bar-inner">
          <div class="batch-bar-left">
            <span class="batch-bar-count">已选 {{ selectedKeys.size }} 项</span>
            <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">清除</t-button>
          </div>
          <div class="batch-bar-actions">
            <!-- 两种打印版式，语义不同：
                 清单 = 每个所选记录一页纵向结算单（逐项分摊明细 + 合计），发给租户；
                 目录 = 当前列表表头 + 所选行数据（横向汇总表），内部对账用。 -->
            <t-button theme="default" variant="outline" size="small" :loading="statementBusy"
              :disabled="!printableRows.length" @click="handlePrintStatement">
              <template #icon><t-icon name="file-pdf" size="14px" /></template>
              打印清单
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy"
              :disabled="!printableRows.length" @click="handlePrintCatalog">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印目录
            </t-button>
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedKeys.size} 条账单吗？`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                删除
              </t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </transition>

    <!-- 账单详情抽屉：与发票/合同同款中台 BusinessSplitDetailDrawer
         （右侧滑出 + 标题栏 + 双列 t-form 网格 + 统一底栏）。
         旧自写 <t-drawer> + .record-detail 表格已淘汰。
         字段顺序严格跟随当前分类「字段定义（subs）」，见 billingFields。 -->
    <!-- 只读抽屉：费用清单的每一列都是核算结果，查看可以、编辑必须走「生成/重算」，
         所以这里 readonly（无输入框、无保存按钮），字段与列表启用列 1:1 对应。 -->
    <BusinessSplitDetailDrawer v-model:visible="recordVisible" :title="recordTitle" :row="null"
      :form="recordFormWrapper" :fields="billingFields" :show-preview="false" readonly
      block-title="费用清单" :storage-key="'weknora-tenant-record-drawer-width'">
      <template #footer-left>
        <t-button variant="outline" size="small" @click="printRecord">打印</t-button>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 打印预览：全中台统一组件（壳子只有一份，内容由本页生成） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="`${printTitle} 打印预览`" :url="printUrl"
      :data="printBytes" :download-name="printFileName" loading-text="正在生成费用清单…" @close="closePrint" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, getBillingTenant,
  listBillingRecords, generateBillingRecord, deleteBillingRecord,
  listKnowledgeBases,
  listUtilityBillRecords, listSolarBillRecords, listUtilityMeterRecords, listUtilityMeters,
} from '@/api/knowledge-base'
import { generateCatalogPdf, generateStatementPdf, type CatalogColumn } from './useCatalogPdf'
import { rowFlattenText, matchAllTokens, notifyEnergyDataChanged } from './energyCounters'
import { parseFieldSource, resolveSourceValue, resolveTenantItemValue, type FieldSource, type SourceResolveContext } from './tenantBillingSources'
import EmptyState from '@/components/EmptyState.vue'

// ---- 使用单位(房东/租户) ----
import { useTableHeaderFix } from './useTableHeaderFix'
import { loadEnergyCategorySubs, applySubsToColumns, type EnergySub } from './energyViewColumns'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'

// 表头右侧滚动条轨道遮罩的实测值（与发票管理同一套机制）
const { headerFixStyle } = useTableHeaderFix()

// 「设置」→ 父级统一设置抽屉（父级常驻挂载，不随 6 个卡片视图的 v-if 卸载）
const emit = defineEmits<{ (e: 'open-settings'): void }>()

const OWNER_UNIT = '重庆星达'
const BILL_KB_NAME = '日常事务-电费'
const useUnit = ref(OWNER_UNIT)
const kbId = ref('')

// ---- 列定义(按使用单位双视图) ----
interface ColDef {
  key: string
  label: string
  default: boolean
  w: string
  tip?: string
  /** 精确列宽（px）；0/未配置走表格自适应 */
  width?: number
  /** 跨分类引用字段的来源（仅「字段定义」里添加的字段有值） */
  source?: FieldSource
  /** 该列来自租户分摊子项（租户视角） */
  item?: any
}
const COL_DEFS_OWNER: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr', tip: '账单月份' },
  { key: 'unit', label: '使用单位', default: true, w: '0.9fr', tip: '当前核算单位' },
  { key: 'total_kwh', label: '总电量', default: false, w: '0.9fr', tip: '市电账单本期电量(提取)' },
  { key: 'total_fee', label: '总电费', default: false, w: '1fr', tip: '市电账单本期电费(提取)' },
  { key: 'solar_gen', label: '光伏发电量', default: true, w: '0.9fr', tip: '光伏账单发电量(提取)' },
  { key: 'solar_grid', label: '上网电量', default: true, w: '0.9fr', tip: '光伏账单上网电量(提取)' },
  { key: 'solar_amount', label: '结算金额', default: true, w: '0.9fr', tip: '光伏账单结算金额(提取)' },
  { key: 'ind_kwh', label: '用电量（工业）', default: true, w: '1fr', tip: '总电量 − 星达居民(定比) − 持睿工业(星达分表总电量)' },
  { key: 'ind_price', label: '均价（工业）', default: true, w: '0.9fr', tip: '(总电费 − 持睿居民电费 − 持睿工业电费) ÷ (总电量 − 持睿居民电量 − 持睿工业电量)' },
  { key: 'ind_fee', label: '电费（工业）', default: true, w: '1fr', tip: '用电量(工业) × 均价(工业)' },
  { key: 'res_kwh', label: '电量（定比）', default: false, w: '0.9fr', tip: '账单提取 定比0.015×本期电量' },
  { key: 'res_fee', label: '电费（定比）', default: false, w: '0.9fr', tip: '账单提取 居民电费(目录电费+政府性基金及附加)' },
  { key: 'dorm_kwh', label: '电量（宿舍）', default: true, w: '0.9fr', tip: '用途=宿舍的电表 本期用电量汇总' },
  { key: 'dorm_fee', label: '电费（宿舍）', default: true, w: '0.9fr', tip: '用途=宿舍的电表 本期电费汇总(电量×单价)' },
  { key: 'water_ind_usage', label: '用水量（工业）', default: true, w: '1fr', tip: '自来水总表 − 星达宿舍 − 持睿宿舍 − 持睿工业' },
  { key: 'water_ind_fee', label: '水费（工业）', default: true, w: '0.9fr', tip: '用水量(工业) × 自来水总表单价' },
  { key: 'water_dorm_usage', label: '用水量（宿舍）', default: true, w: '1fr', tip: '用途=宿舍的水表 本期用水量汇总' },
  { key: 'water_dorm_fee', label: '水费（宿舍）', default: true, w: '0.9fr', tip: '用途=宿舍的水表 本期水费汇总(用量×单价)' },
  { key: 'water_fire_usage', label: '用水量（消防）', default: true, w: '1fr', tip: '水表类型=消防 本期用水量汇总' },
  { key: 'water_fire_fee', label: '水费（消防）', default: true, w: '0.9fr', tip: '水表类型=消防 本期水费汇总(用量×单价)' },
  { key: 'gas_usage', label: '用气量', default: true, w: '0.9fr', tip: '气表记录汇总(公租房除外)' },
  { key: 'gas_fee', label: '气费', default: true, w: '0.9fr', tip: '气表记录费用汇总(用量×单价)' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr', tip: '手工填写' },
]
const COL_DEFS_TENANT: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr', tip: '账单月份' },
  { key: 'unit', label: '使用单位', default: true, w: '0.9fr', tip: '当前核算单位' },
  { key: 'total_kwh', label: '总电量', default: false, w: '0.9fr', tip: '市电账单本期电量(提取)' },
  { key: 'total_fee', label: '总电费', default: false, w: '1fr', tip: '市电账单本期电费(提取)' },
  { key: 'ratio', label: '分摊比例', default: true, w: '0.9fr', tip: '星达分表总电量 ÷ 市电账单本期电量' },
  { key: 'ind_kwh', label: '用电量（工业）', default: true, w: '0.9fr', tip: '持睿工业分表总电量(星达分表)' },
  { key: 'ind_fee', label: '电费（工业）', default: true, w: '1fr', tip: '市电子项按分摊比例折算汇总' },
  { key: 'ind_price', label: '均价（工业）', default: true, w: '0.9fr', tip: '电费(工业) ÷ 用电量(工业)' },
  { key: 'dorm_kwh', label: '电量（居民）', default: true, w: '0.9fr', tip: '持睿宿舍电表 用电量汇总' },
  { key: 'dorm_fee', label: '电费（居民）', default: true, w: '0.9fr', tip: '持睿宿舍电表 电费汇总(电量×单价)' },
  { key: 'water_ind_usage', label: '用水量（工业）', default: true, w: '1fr', tip: '持睿工业水表 用水量汇总' },
  { key: 'water_ind_fee', label: '水费（工业）', default: true, w: '0.9fr', tip: '持睿工业水表 水费汇总(用量×单价)' },
  { key: 'water_dorm_usage', label: '用水量（居民）', default: true, w: '1fr', tip: '持睿宿舍水表 用水量汇总' },
  { key: 'water_dorm_fee', label: '水费（居民）', default: true, w: '0.9fr', tip: '持睿宿舍水表 水费汇总(用量×单价)' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr', tip: '手工填写' },
]
const isOwnerView = computed(() => useUnit.value === OWNER_UNIT)
// 分类名与设置抽屉里的 fleet_categories.name 逐字一致
const CATEGORY_NAME = '费用清单'
const categorySubs = ref<EnergySub[]>([])
/** 当前租户的「分摊子项」配置（切换使用单位时按需加载） */
const tenantItems = ref<any[]>([])
/**
 * 生效列定义 = 费用清单分类的字段定义（subs）。
 *
 * 两套视角（业主 / 租户）列集不同，而 subs 是并集，所以这里 restrictToBase：
 * 只保留当前视角本来就有的列，再用 subs 的名字/顺序/宽度/默认显示覆盖。
 */
const baseColDefs = computed(() => (isOwnerView.value ? COL_DEFS_OWNER : COL_DEFS_TENANT))

/**
 * 租户视角：当前租户在「清单设置 -> 分摊子项」里启用的项目（按 sort）。
 *
 * 这些项目既是列表列，也是抽屉字段的唯一来源 —— 所以列与数据天然一一对应，
 * 不存在「配置里开了、列表里看不到」。
 */
const tenantEnabledItems = computed(() =>
  tenantItems.value.filter((it: any) => it && it.enabled !== false),
)

/** 分摊子项列 key（item_key 内已含「大类|子项名」，加前缀避免与内置列撞名）。 */
const tenantItemColKey = (it: any) => `item_${String(it?.item_key || it?.itemKey || '')}`

/**
 * 生效列定义。
 *
 * 业主视角：内置派生列 + 「字段定义」里跨分类引用的字段（追加列）。
 * 租户视角：固定锚点列（月份/使用单位/总电量/总电费/分摊比例）+ 该租户启用的
 * 分摊子项 + 合计应付。未配置任何子项时回落到内置租户列，避免空表。
 */
const activeColDefs = computed<ColDef[]>(() => {
  const subs = categorySubs.value || []
  // 内置派生列：仍受 subs 的改名/顺序/宽度/默认显隐控制（restrictToBase =
  // 只有本来就存在的列会被应用，避免把引用字段误当成内置列）。
  const builtinSubs = subs.filter((s) => !parseFieldSource(s))
  const base = applySubsToColumns(baseColDefs.value, builtinSubs, undefined, { restrictToBase: true })

  if (!isOwnerView.value) {
    const anchors = base.filter((c) => ['month', 'unit', 'total_kwh', 'total_fee', 'ratio'].includes(c.key))
    const items = tenantEnabledItems.value.map((it: any) => ({
      key: tenantItemColKey(it),
      label: String(it.item_name || it.itemName || it.source_key || '分摊项'),
      default: true,
      w: '1fr',
      width: Number(it.width) || undefined,
      tip: it.source_category ? `来源：${it.source_category}` : '市电账单费用分项',
      item: it,
    })) as ColDef[]
    if (!items.length) return base
    const totalCol: ColDef = { key: 'item_total', label: '合计应付', default: true, w: '1fr', tip: '该租户当月应付合计' }
    return [...anchors, ...items, totalCol]
  }

  // 引用字段一律追加在内置列之后：它们是「从别的分类借来的列」，
  // 不能插进内置口径列中间（否则读表顺序会变成混排，用户很难对着看）。
  const extras: ColDef[] = []
  for (const sub of subs) {
    const src = parseFieldSource(sub)
    if (!src || sub.enabled === false) continue
    extras.push({
      key: String(sub.name),
      label: String(sub.label || sub.name),
      default: sub.is_default === true,
      w: '1fr',
      width: Number(sub.width) || undefined,
      tip: `引用自 ${src.category}`,
      source: src,
    })
  }
  return extras.length ? [...base, ...extras] : base
})

/**
 * 拉取当前租户的「分摊子项」。
 *
 * 租户视角的列 = 锚点列 + 这里启用的项目，所以必须在切换使用单位后立即拉一次；
 * 否则列表会停在上一家的列（用户看到的字段与所选单位不匹配）。
 */
async function loadTenantItems() {
  const id = activeTenantId.value
  if (!id) { tenantItems.value = []; return }
  try {
    const res: any = await getBillingTenant(id)
    const items: any[] = res?.data?.items || []
    tenantItems.value = [...items].sort((a, b) => (Number(a.sort) || 0) - (Number(b.sort) || 0))
  } catch {
    tenantItems.value = []
  }
  syncColumnKeys()
}

/** 拉取字段定义（设置抽屉保存后由 window 事件触发重载）。 */
async function loadCategorySubs() {
  try {
    categorySubs.value = await loadEnergyCategorySubs(CATEGORY_NAME)
  } catch {
    categorySubs.value = []
  }
  syncColumnKeys()
}
const colStorageKey = computed(() => `weknora-tenant-billing-cols-${isOwnerView.value ? 'owner' : 'tenant'}-v3`)
const visibleKeys = ref<string[]>([])
function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(colStorageKey.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        const valid = arr.filter(k => activeColDefs.value.some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return activeColDefs.value.filter(c => c.default).map(c => c.key)
}
const syncColumnKeys = () => { visibleKeys.value = loadStoredKeys() }
syncColumnKeys()
watch(useUnit, syncColumnKeys)
const visibleColDefs = computed(() => activeColDefs.value.filter(c => visibleKeys.value.includes(c.key)))
const tableColumns = computed(() => {
  // 与发票管理一致：[复选框] → [序号] → 业务列
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 56, align: 'center' },
  ]
  // 宽度严格跟随「字段定义」：subs.width>0 → clamp(60~400) 精确生效；0 → 自适应。
  // （旧「总和 ≤1500 预算守卫」已删除：24 列宽表一超过阈值就把用户配置的全部
  // 宽度作废，真机表现为「填 260 保存后列纹丝不动」。列总宽超出容器时，
  // 和其它宽表一样启用横向滚动即可，没有任何理由静默吞掉用户配置。）
  for (const c of visibleColDefs.value) {
    const cfg = Number((c as any).width) || 0
    cols.push({ colKey: c.key, title: c.label, ellipsis: true, width: cfg > 0 ? Math.min(Math.max(cfg, 60), 400) : undefined })
  }
  return cols
})
const selectedRowKeysArr = computed(() => Array.from(selectedKeys.value))
function onTableSelectChange(val: any) { selectedKeys.value = new Set(Array.isArray(val) ? val : []) }
const selectAllColumns = () => { visibleKeys.value = activeColDefs.value.map(c => c.key); persistColumns() }
const resetColumns = () => { visibleKeys.value = activeColDefs.value.filter(c => c.default).map(c => c.key); persistColumns() }
const persistColumns = () => { try { localStorage.setItem(colStorageKey.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ } }

// ---- 租户与账单 ----
const loading = ref(false)
const tenants = ref<any[]>([])
const activeTenantId = ref('')
const useUnitOptions = computed(() => {
  const units = new Set<string>([OWNER_UNIT])
  tenants.value.forEach((t: any) => { if (t.name) units.add(t.name) })
  return Array.from(units).map(v => ({ label: v, value: v }))
})
const filters = ref<{ keyword?: string; month?: string; useUnit?: string }>({ keyword: '', month: undefined, useUnit: undefined })

// BusinessFilterBar 扁平筛选态 ↔ 既有 ref 双向桥接。
// 「使用单位」在费用清单里不只是筛选器：它切换整个视角（业主/租户列集、
// 存档列偏好），所以 set 里检测变化后直接走既有 onUseUnitChange 副作用链。
const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'month', type: 'month', placeholder: '账单月份' },
  { key: 'useUnit', type: 'select', placeholder: '使用单位', icon: 'usergroup', options: useUnitOptions.value, immediate: true },
])
const filterState = computed<FilterState>({
  get: () => ({ keyword: filters.value.keyword || '', month: filters.value.month, useUnit: useUnit.value }),
  set: (v) => {
    filters.value.keyword = String(v?.keyword ?? '')
    filters.value.month = v?.month || undefined
    const unit = String(v?.useUnit ?? '')
    if (unit && unit !== useUnit.value) {
      useUnit.value = unit
      onUseUnitChange()
    } else {
      buildRows()
    }
  },
})
/** 刷新按钮 / immediate 控件变更：重建行集（全本地过滤，无需请求）。 */
function onBarFilterChange() { buildRows() }
const records = ref<any[]>([])
const displayRows = ref<any[]>([])

// ---- 数据源缓存(市电/光伏/水电气记录/水表) ----
const utilityBills = ref<any[]>([])      // 市电账单 record(含 item)
const solarBills = ref<any[]>([])        // 光伏 record
const elecRows = ref<any[]>([])          // 电费页电表记录(宿舍表)
const gasRows = ref<any[]>([])           // 气费记录
const waterMeters = ref<any[]>([])       // 租户核算水表
const waterReadings = ref<any[]>([])     // 租户核算水表读数

const monthOfPeriod = (v: any): string => {
  const s = String(v || '')
  return s.length >= 7 ? s.slice(0, 7) : s
}

const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === BILL_KB_NAME) : null
    kbId.value = found?.id || ''
  } catch { /* ignore */ }
}

const loadTenants = async () => {
  loading.value = true
  try {
    const res: any = await listBillingTenants()
    tenants.value = res.data || []
    if (tenants.value.length && !activeTenantId.value) {
      activeTenantId.value = tenants.value[0].id
    }
    await loadTenantItems()
    await loadAllData()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户失败')
  } finally {
    loading.value = false
  }
}

// 并行加载所有数据源并计算行
const loadAllData = async () => {
  await loadKb()
  const tenantId = activeTenantId.value
  if (!tenantId) { records.value = []; displayRows.value = []; return }
  try {
    const tasks: Promise<any>[] = []
    if (kbId.value) {
      tasks.push(listUtilityBillRecords(kbId.value, { page: 1, page_size: 500 }))
      tasks.push(listSolarBillRecords(kbId.value, { page: 1, page_size: 500 }))
    }
    tasks.push(
      listBillingRecords(tenantId),
      listUtilityMeterRecords({ category: 'electricity' }),
      listUtilityMeterRecords({ category: 'gas' }),
      listUtilityMeters({ category: 'water' }),
      listUtilityMeterRecords({ category: 'water' }),
    )
    const [billRes, solarRes, recRes, elecRes, gasRes, wmRes, wrRes]: any[] = await Promise.all(tasks)
    utilityBills.value = Array.isArray(billRes?.data || billRes?.list) ? (billRes.data || billRes.list) : []
    solarBills.value = Array.isArray(solarRes?.data || solarRes?.list) ? (solarRes.data || solarRes.list) : []
    records.value = recRes?.data || []
    elecRows.value = flattenMeterRecords(elecRes)
    gasRows.value = flattenMeterRecords(gasRes)
    waterMeters.value = Array.isArray(wmRes?.data) ? wmRes.data : (Array.isArray(wmRes) ? wmRes : [])
    waterReadings.value = flattenWaterRecords(wrRes)
    buildRows()
    autoGenerateReady()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

// 电/气记录展平(与 UtilityMeterTab 一致)
const flattenMeterRecords = (res: any): any[] => {
  const recData = res?.data || {}
  const records = recData?.records || res?.records || []
  const mets = Array.isArray(recData?.meters) ? recData.meters : []
  const meterMap = new Map(mets.map((m: any) => [m.id, m]))
  const flat: any[] = []
  for (const rec of records) {
    for (const it of (rec.items || [])) {
      const meter = meterMap.get(it.meter_id) as any
      flat.push({
        month: rec.month,
        meter_id: it.meter_id,
        meter_alias: meter?.alias || (it as any).meter_name || '',
        meter_kind: meter?.meter_kind || 'dorm',
        meter_type: meter?.meter_type || '',
        use_unit: meter?.use_unit || (it as any).use_unit || '',
        rate: Number(meter?.rate) > 0 ? Number(meter?.rate) : 1,
        start_reading: Number(it.start_reading) || 0,
        end_reading: Number(it.end_reading) || 0,
        usage: Number(it.usage) || 0,
        unit_price: Number(it.unit_price ?? meter?.default_unit_price) || 0,
        amount: Number(it.amount) || 0,
      })
    }
  }
  return flat
}

// 水表记录展平(utility 数据源)：读取每个子行(表计/起止/用量/金额)
const flattenWaterRecords = (res: any): any[] => {
  const recData = res?.data || {}
  const records = recData?.records || res?.records || []
  const mets = Array.isArray(recData?.meters) ? recData.meters : []
  const meterMap = new Map(mets.map((m: any) => [m.id, m]))
  const flat: any[] = []
  for (const rec of records) {
    for (const it of (rec.items || [])) {
      const meter = meterMap.get(it.meter_id) as any
      flat.push({
        month: rec.month,
        meter_id: it.meter_id,
        meter_alias: meter?.alias || (it as any).meter_name || '',
        meter_kind: meter?.meter_kind || 'dorm',
        meter_type: meter?.meter_type || 'sub',
        owner_unit: meter?.owner_unit || '',
        use_unit: meter?.use_unit || '',
        rate: Number(meter?.rate) > 0 ? Number(meter?.rate) : 1,
        start_reading: Number(it.start_reading) || 0,
        end_reading: Number(it.end_reading) || 0,
        usage: Number(it.usage) || 0,
        unit_price: Number(it.unit_price) || 0,
        amount: Number(it.amount) || 0,
      })
    }
  }
  return flat
}

const sumBy = (arr: any[], key: string): number => Math.round(arr.reduce((s, r) => s + (Number(r[key]) || 0), 0) * 100) / 100

// 市电账单居民电量/电费
// 电量(定比)= 账单提取 residential_readings 的定比电量原值 kwh(不取 bill_kwh:部分月份 bill_kwh 被算成 kwh×0.015)
// 电费(定比)= 账单提取 residential_amount(目录电费+政府性基金及附加)；旧账单无该字段时兜底 = catalog_amount + 政府性基金及附加(居民)
const residentInfoOf = (bill: any) => {
  const item = bill?.item || bill || {}
  const rows = Array.isArray(item.residential_readings) ? item.residential_readings : []
  const kwh = Math.round(rows.reduce((s: number, r: any) => s + (Number(r.kwh) || 0), 0) * 100) / 100
  const gov = (Array.isArray(item.fee_items) ? item.fee_items : [])
    .filter((f: any) => String(f.category || '').includes('政府性基金') && Number(f.qty || 0) < 100000 && !String(f.name || '').includes('功率因数'))
    .reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0)
  const fee = Math.round((Number(item.residential_amount) || (Number(item.catalog_amount) || 0) + (Number(gov) || 0)) * 100) / 100
  return { kwh, fee }
}

// 水表读数按 (meterId, month) 索引(utility 子行)
const waterReadingMap = computed(() => {
  const m = new Map<string, any>()
  waterReadings.value.forEach((r: any) => m.set(`${r.meter_id}__${r.month}`, r))
  return m
})

// 汇总指定层级/归属水表某月用量/费用：
//   kind: total 总表 | sub 分表 | fire 消防(即 meter_type)
//   owner: '' 全部 | 使用单位/归属单位匹配
//   dormOnly: true 仅用途=宿舍；false 仅用途=生产(默认不含宿舍)
const waterAgg = (kind: string, owner: string, month: string, dormOnly = false) => {
  const meters = waterMeters.value.filter((m: any) =>
    m.enabled !== false && (m.meter_type === 'normal' ? 'sub' : (m.meter_type || 'sub')) === kind &&
    (owner === '' || m.owner_unit === owner || m.use_unit === owner) &&
    // 公租房数据独立存在,不参与任何核算(宿舍侧仅统计 dorm,工业侧剔除 dorm/public)
    (dormOnly ? m.meter_kind === 'dorm' : !['dorm', 'public'].includes(m.meter_kind)),
  )
  let usage = 0
  let fee = 0
  let price = 0
  for (const m of meters) {
    const r = waterReadingMap.value.get(`${m.id}__${month}`)
    if (!r) continue
    usage += Number(r.usage) || 0
    fee += Number(r.amount) || 0
    if (!price && Number(m.default_unit_price) > 0) price = Number(m.default_unit_price)
  }
  if (!price && usage > 0) price = fee / usage
  return { usage: Math.round(usage * 100) / 100, fee: Math.round(fee * 100) / 100, price }
}

// 租户视图用水：按使用单位/归属单位 + 用途(宿舍/生产)匹配，不限定计量层级，
// 以覆盖 normal 宿舍水表(持睿403~418)与 sub 分表(CRM01/02)两类数据源。
const tenantWater = (owner: string, month: string, dormOnly = false) => {
  const meters = waterMeters.value.filter((m: any) =>
    m.enabled !== false &&
    (m.owner_unit === owner || m.use_unit === owner) &&
    (dormOnly ? m.meter_kind === 'dorm' : !['dorm', 'public'].includes(m.meter_kind)),
  )
  let usage = 0
  let fee = 0
  for (const m of meters) {
    const r = waterReadingMap.value.get(`${m.id}__${month}`)
    if (!r) continue
    usage += Number(r.usage) || 0
    fee += Number(r.amount) || 0
  }
  return { usage: Math.round(usage * 100) / 100, fee: Math.round(fee * 100) / 100 }
}

// 组装列表行
/**
 * 引用字段取值上下文。
 *
 * 所有数据集都是列表已经加载过的（市电账单 / 光伏账单 / 电/气抄表 / 水表读数），
 * 直接用同一份内存数据算，避免后端再聚合一版造成口径漂移。
 */
const sourceCtxOf = (month: string, bill: any, solarArr: any[]): SourceResolveContext => ({
  month,
  tenantName: useUnit.value,
  bill,
  solarItems: solarArr,
  elecRows: elecRows.value,
  waterMeters: waterMeters.value,
  waterReadingMap: waterReadingMap.value,
  gasRows: gasRows.value,
})

/** 当月分摊比例：已生成账单用固化值；未生成按月内星达分时总电量 ÷ 账单电量估算。 */
const ratioOfMonth = (month: string, bill: any, rec: any): number => {
  const fixed = Number(rec?.ratio) || 0
  if (fixed > 0) return fixed
  const billKwh = Number(bill?.item?.total_kwh) || 0
  if (billKwh <= 0) return 0
  const star = elecRows.value
    .filter((r) => r.month === month && r.meter_type === 'time' && r.use_unit === OWNER_UNIT)
    .reduce((sum, r) => sum + (Number(r.usage) || 0), 0)
  return star > 0 ? Math.round((star / billKwh) * 1e6) / 1e6 : 0
}

const buildRows = () => {
  const tenant = tenants.value.find((t: any) => t.id === activeTenantId.value)
  const tenantName = tenant?.name || ''
  const billByMonth = new Map<string, any>()
  utilityBills.value.forEach((r: any) => { const m = monthOfPeriod(r.item?.bill_period_start || r.item?.bill_period_end || r.item?.bill_month); if (m) billByMonth.set(m, r) })
  const solarByMonth = new Map<string, any>()
  solarBills.value.forEach((r: any) => { const m = monthOfPeriod(r.item?.bill_period_start || r.item?.bill_period_end); if (m) { const arr = solarByMonth.get(m) || []; arr.push(r); solarByMonth.set(m, arr) } })
  const recByMonth = new Map<string, any>()
  records.value.forEach((r: any) => recByMonth.set(r.month, r))

  const months = new Set<string>()
  billByMonth.forEach((_, m) => months.add(m))
  solarByMonth.forEach((_, m) => months.add(m))
  recByMonth.forEach((_, m) => months.add(m))
  elecRows.value.forEach(r => { if (r.month) months.add(r.month) })
  gasRows.value.forEach(r => { if (r.month) months.add(r.month) })
  waterReadings.value.forEach(r => { if (r.month) months.add(r.month) })

  const list = Array.from(months).sort().reverse().map((month: string) => {
    const bill = billByMonth.get(month)
    const rec = recByMonth.get(month)
    const solarArr = solarByMonth.get(month) || []
    // 宿舍电表(utility 电费页表,按使用单位+表类型宿舍)
    const elecOf = (unit: string) => elecRows.value.filter(r => r.month === month && r.use_unit === unit && r.meter_kind === 'dorm')
    const dormOf = (unit: string) => {
      const arr = elecOf(unit)
      return { kwh: sumBy(arr, 'usage'), fee: Math.round(arr.reduce((s, r) => s + (Number(r.usage) || 0) * (Number(r.unit_price) || 0), 0) * 100) / 100 }
    }
    const gasOf = () => gasRows.value.filter(r => r.month === month && r.meter_kind !== 'public')
    const base: Record<string, any> = {
      // 主键必须带「使用单位」：同一月份在不同单位下各有一行，
      // 只用 row-${month} 会撞键，勾选一行的勾选态会同时命中同月的所有行。
      id: `row-${useUnit.value || 'unit'}__${month}`, month, unit: useUnit.value, hasRec: !!rec,
      billReady: !!bill,
      readingReady: elecRows.value.some(r => r.month === month && r.meter_type === 'time'),
    }

    if (isOwnerView.value) {
      // ===== 星达(房东)视图 =====
      const totalKwh = Number(bill?.item?.total_kwh) || 0
      const totalFee = Number(bill?.item?.grand_total ?? bill?.item?.total_amount) || 0
      const res = residentInfoOf(bill?.item)
      const dorm = dormOf(OWNER_UNIT)
      const tenantDorm = dormOf(tenantName)
      // 总表(星达自来水总表)、消防总表、持睿工业/居民用水
      const totalMain = waterAgg('total', '', month)
      const wFire = waterAgg('fire', '', month)
      const tInd = tenantWater(tenantName, month, false)
      const tDorm = tenantWater(tenantName, month, true)
      const wDorm = waterAgg('sub', OWNER_UNIT, month, true)
      const gas = gasOf()
      // 星达工业用电量 = 总电量 - 星达居民电量 - 持睿工业电量(星达分表总电量)
      const indKwh = Math.round((totalKwh - res.kwh - (Number(rec?.total_kwh) || 0)) * 100) / 100
      // 星达工业均价 = (总电费 - 持睿居民电费 - 持睿工业电费) ÷ (总电量 - 持睿居民电量 - 持睿工业电量)
      const priceDenom = totalKwh - tenantDorm.kwh - (Number(rec?.total_kwh) || 0)
      const priceExact = priceDenom > 0 ? (totalFee - tenantDorm.fee - (Number(rec?.industrial_fee) || 0)) / priceDenom : 0
      const indPrice = Math.round(priceExact * 10000) / 10000
      // 星达工业电费 = 用电量(工业) × 均价(工业)（用未取整均价计算,列表自洽）
      const indFee = Math.round(indKwh * priceExact * 100) / 100
      // 星达工业用水量 = 自来水总表 − 星达宿舍 − 持睿宿舍 − 持睿工业
      const wIndUsage = Math.round((totalMain.usage - wDorm.usage - tDorm.usage - tInd.usage) * 100) / 100
      // 星达工业水费 = 工业用水量 × 星达自来水总表单价
      const wIndFee = Math.round((wIndUsage * (totalMain.price || 0)) * 100) / 100
      Object.assign(base, {
        total_kwh: totalKwh,
        total_fee: totalFee,
        solar_gen: sumBy(solarArr.map((s: any) => ({ v: s.item?.generation_kwh })), 'v'),
        solar_grid: sumBy(solarArr.map((s: any) => ({ v: s.item?.grid_kwh })), 'v'),
        solar_amount: sumBy(solarArr.map((s: any) => ({ v: s.item?.settlement_amount })), 'v'),
        ind_kwh: indKwh,
        ind_fee: indFee,
        ind_price: indPrice,
        res_kwh: res.kwh,
        res_fee: res.fee,
        dorm_kwh: dorm.kwh,
        dorm_fee: dorm.fee,
        water_ind_usage: wIndUsage, water_ind_fee: wIndFee,
        water_dorm_usage: wDorm.usage, water_dorm_fee: wDorm.fee,
        water_fire_usage: wFire.usage, water_fire_fee: wFire.fee,
        gas_usage: sumBy(gas, 'usage'),
        gas_fee: sumBy(gas, 'amount'),
      })
      // 跨分类引用字段（费用清单「字段定义」里检索添加的列）
      const ctx = sourceCtxOf(month, bill, solarArr)
      for (const c of activeColDefs.value) {
        if (!c.source) continue
        base[c.key] = resolveSourceValue(c.source, ctx)
      }
    } else {
      // ===== 持睿(租户)视图 =====
      const totalKwh = Number(bill?.item?.total_kwh) || 0
      const totalFee = Number(bill?.item?.grand_total ?? bill?.item?.total_amount) || 0
      const dorm = dormOf(tenantName)
      const wInd = tenantWater(tenantName, month, false)
      const wDorm = tenantWater(tenantName, month, true)
      const indKwh = Number(rec?.total_kwh) || 0
      const indFee = Number(rec?.industrial_fee) || 0
      Object.assign(base, {
        total_kwh: totalKwh,
        total_fee: totalFee,
        ratio: Number(rec?.ratio) || 0,
        ind_kwh: indKwh,
        ind_fee: indFee,
        ind_price: indKwh > 0 ? Math.round(indFee / indKwh * 10000) / 10000 : 0,
        dorm_kwh: dorm.kwh,
        dorm_fee: dorm.fee,
        water_ind_usage: wInd.usage, water_ind_fee: wInd.fee,
        water_dorm_usage: wDorm.usage, water_dorm_fee: wDorm.fee,
      })
      // 该租户的分摊子项：列与值同源（tenantEnabledItems），不可能出现「有列无数」
      const ctx = sourceCtxOf(month, bill, solarArr)
      const ratio = ratioOfMonth(month, bill, rec)
      let total = 0
      for (const it of tenantEnabledItems.value) {
        const v = resolveTenantItemValue(it, ctx, ratio)
        base[tenantItemColKey(it)] = v
        total += v
      }
      if (tenantEnabledItems.value.length) base.item_total = Math.round(total * 100) / 100
    }
    base.remark = rec?.remark || ''
    return base
  })
  let out = filters.value.month ? list.filter(r => r.month === filters.value.month) : list
  // 「搜索全部字段」：行内所有可读值拼串匹配（多关键词空格分词 AND）。
  const kw = String(filters.value.keyword || '').trim()
  if (kw) out = out.filter((r) => matchAllTokens(rowFlattenText(r), kw))
  displayRows.value = out
  clearSelection()
}

const loadRecords = async () => {
  await loadAllData()
}


const onUseUnitChange = () => {
  clearSelection()
  syncColumnKeys()
  // 切到租户视角要按该租户的分摊子项重建列（异步，buildRows 在拿到后重跑）
  void loadTenantItems().then(() => buildRows())
  buildRows()
}

// ---- 汇总 ----
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => displayRows.value.filter(r => selectedKeys.value.has(r.id)))
const summary = computed(() => ({ total: (selectedKeys.value.size ? selectedRows.value : displayRows.value).length }))
const summaryKwh = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_kwh) || 0), 0) * 100) / 100
})
const summaryFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_fee) || 0), 0) * 100) / 100
})
const summaryPayableFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.ind_fee) || 0) + (Number(r.dorm_fee) || 0), 0) * 100) / 100
})
const summaryWaterFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.water_ind_fee) || 0) + (Number(r.water_dorm_fee) || 0), 0) * 100) / 100
})
// 行点击 = 打开详情抽屉（业主/租户两个视角一致）。
//
// 旧实现只在租户视角、且该月已固化 billing_records 时才开抽屉 ——
// 业主视角点行永远没有任何反应（这就是「费用清单点击记录无弹窗」）。
// 现在传的是**行本身**，抽屉直接用行上的核算值渲染，不再依赖固化记录。
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.id])
  void openRecord(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 生成月度账单 ----
const generateBusy = ref(false)
/** 生成目标：勾选了就只处理所选，未勾选 = 全部（与确认文案一致）。 */
const generateTargets = computed(() => (selectedKeys.value.size ? selectedRows.value : displayRows.value))
const generateConfirmText = computed(() => (selectedKeys.value.size
  ? `确认重新生成所选 ${selectedKeys.value.size} 条记录的账单？`
  : '当前未选择记录，确认全部重新生成账单？'))

/**
 * 生成前完整性校验（产品口径：严格拦截）。
 *
 * 逐月检查：市电账单、星达分时读数、该租户电表读数，以及已开启分摊子项
 * 依赖的水/气读数。任一缺失即整批禁止生成 —— 带病生成出来的账单会静默
 * 少算/错算，比生成失败更难发现。
 */
function readinessOf(rows: any[]): Array<{ month: string; missing: string[] }> {
  const issues: Array<{ month: string; missing: string[] }> = []
  const itemCats = new Set(tenantEnabledItems.value.map((it: any) => String(it.source_category || '市电账单')))
  for (const r of rows) {
    const missing: string[] = []
    if (!r.billReady) missing.push('市电账单')
    if (!r.readingReady) missing.push('星达分时读数')
    if (!isOwnerView.value) {
      const dormRows = elecRows.value.filter((x) =>
        x.month === r.month && x.use_unit === useUnit.value && x.meter_kind === 'dorm')
      if (!dormRows.length) missing.push(`「${useUnit.value}」电表读数`)
      if (itemCats.has('水费核算')) {
        const meters = waterMeters.value.filter((m: any) =>
          m.enabled !== false && (m.owner_unit === useUnit.value || m.use_unit === useUnit.value))
        if (!meters.some((m: any) => waterReadingMap.value.get(`${m.id}__${r.month}`))) missing.push('水表读数')
      }
      if (itemCats.has('气费核算')) {
        const hasGas = gasRows.value.some((g) =>
          g.month === r.month && g.use_unit === useUnit.value && g.meter_kind !== 'public')
        if (!hasGas) missing.push('气表读数')
      }
    }
    if (missing.length) issues.push({ month: r.month, missing })
  }
  return issues
}

/** 数据不全提示：逐月列出缺失项，明确告诉用户去补什么。 */
function reportReadiness(issues: Array<{ month: string; missing: string[] }>): boolean {
  const detail = `以下月份数据不全，已禁止生成：\n${issues
    .map((i) => `· ${i.month}：缺少 ${i.missing.join('、')}`)
    .join('\n')}`
  MessagePlugin.error({ content: detail, duration: 8000 })
  return false
}

const handleGenerateAll = async () => {
  const targets = generateTargets.value
  if (!targets.length) {
    MessagePlugin.warning('当前没有可生成的记录')
    return
  }
  const issues = readinessOf(targets)
  if (issues.length) { reportReadiness(issues); return }

  if (isOwnerView.value) {
    generateBusy.value = true
    try {
      await loadAllData()
      MessagePlugin.success('已重新核算')
    } finally {
      generateBusy.value = false
    }
    return
  }
  if (!activeTenantId.value) return
  generateBusy.value = true
  try {
    const okMonths: string[] = []
    const fail: string[] = []
    for (const row of targets) {
      try {
        await generateBillingRecord(activeTenantId.value, { month: row.month })
        okMonths.push(row.month)
      } catch (e: any) {
        fail.push(`${row.month}（${e?.message || '未知错误'}）`)
      }
    }
    if (okMonths.length) MessagePlugin.success(`已生成 ${okMonths.length} 个月度账单：${okMonths.join('、')}`)
    if (fail.length) MessagePlugin.warning(`以下月份生成失败：${fail.join('；')}`)
    await loadAllData()
    notifyEnergyDataChanged()
  } finally {
    generateBusy.value = false
  }
}

// 数据齐全时默认自动生成（租户视图；缺市电账单或分时读数的月份跳过，交给手动生成）
const autoGenAttempted = ref<Set<string>>(new Set())
const autoGenerateReady = async () => {
  if (isOwnerView.value || !activeTenantId.value) return
  const pending = displayRows.value.filter((r: any) =>
    !r.hasRec && r.billReady && r.readingReady && !autoGenAttempted.value.has(r.month))
  if (!pending.length) return
  autoGenAttempted.value = new Set([...autoGenAttempted.value, ...pending.map((r: any) => r.month)])
  for (const row of pending) {
    try {
      await generateBillingRecord(activeTenantId.value, { month: row.month })
    } catch { /* 缺项由手动生成提示 */ }
  }
  await loadAllData()
}

// ---- 浮动工具栏：打印 ----
const catalogBusy = ref(false)
const statementBusy = ref(false)
const printableRows = computed(() => (selectedKeys.value.size ? selectedRows.value : displayRows.value))

// ---- 打印清单：按市电账单「概览 + 细项 + 水电气分区」组装 ----
//
// 版式来源：市电账单详情页（概览 → 各费用组成细项）。租户视角下所有市电分项
// 按「用电量分摊比例」折算，水/电/气核算明细按「当月 + 使用单位」直接归属，
// 且**水、电、气各自独立成表**（旧版混在一张 3 列表里，电量还落在「金额(元)」列）。
const billIndex = computed(() => {
  const m = new Map<string, any>()
  for (const r of utilityBills.value as any[]) {
    const mo = monthOfPeriod(r.item?.bill_period_start || r.item?.bill_period_end || r.item?.bill_month)
    if (mo) m.set(mo, r)
  }
  return m
})
const money2 = (v: any): string => {
  const n = Number(v)
  return Number.isFinite(n) ? (Math.round(n * 100) / 100).toFixed(2) : '—'
}
const num2 = (v: any): string => {
  const n = Number(v)
  if (!Number.isFinite(n)) return '—'
  const r = Math.round(n * 100) / 100
  return Number.isInteger(r) ? String(r) : r.toFixed(2)
}
const rateText = (v: any): string => {
  const n = Number(v)
  if (!Number.isFinite(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}

/** 该行的分摊比例：已生成账单用固化值，否则按月内星达分时电量 ÷ 账单电量估算 */
function ratioForRow(row: any): number {
  const rec = records.value.find((r: any) => r.month === row.month)
  return ratioOfMonth(String(row.month || ''), billIndex.value.get(String(row.month || '')), rec)
}

interface PrintTable { title: string; columns: any[]; rows: string[][]; total?: string[]; empty?: string }

/** 市电账单费用分项 → 细项表（租户视角按比例折算） */
function feeTableOf(month: string, unitRatio: number, prefix: string): PrintTable {
  const bill = billIndex.value.get(month)
  const items = (bill?.item?.fee_items || []) as any[]
  const map = new Map<string, any[]>()
  for (const it of items) {
    const cat = String(it.category || '其他费用')
    if (!map.has(cat)) map.set(cat, [])
    map.get(cat)!.push(it)
  }
  const order = Array.from(map.keys()).sort((a, b) => {
    const ia = FEE_CATEGORY_ORDER.findIndex((c) => a.includes(c))
    const ib = FEE_CATEGORY_ORDER.findIndex((c) => b.includes(c))
    return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
  })
  const rows: string[][] = []
  let sum = 0
  for (const cat of order) {
    for (const it of map.get(cat)!) {
      const fee = (Number(it.fee) || 0) * unitRatio
      const qty = (Number(it.qty) || 0) * unitRatio
      sum += fee
      // 提取出来的 name 通常已带「（1）市场化购电费.偏差电费」这类前缀，
      // 再拼一次分类会变成「（1）市场化购电费｜（1）市场化购电费.偏差电费」。
      const rawName = String(it.name || '').trim()
      const name = rawName && cat && !rawName.includes(cat) ? `${cat}｜${rawName}` : (rawName || cat)
      rows.push([name, it.period || '—', qty ? num2(qty) : '—', rateText(it.rate), money2(fee)])
    }
  }
  return {
    title: `${prefix}电费分项细项`,
    columns: [
      { label: '费用组成', weight: 3.4 },
      { label: '时段', weight: 1, align: 'center' },
      { label: '计费数量', weight: 1.3, align: 'right' },
      { label: '单价/标准', weight: 1.2, align: 'right' },
      { label: '金额(元)', weight: 1.4, align: 'right' },
    ],
    rows,
    total: ['合计', '', '', '', money2(sum)],
    empty: '当月无市电账单分项数据',
  }
}

/** 电费核算抄表明细（分表 / 宿舍表），与主表同口径：用量 × 单价 = 金额 */
function elecTableOf(month: string, unit: string, prefix: string): PrintTable {
  const rows = elecRows.value.filter((r: any) => r.month === month && String(r.use_unit || '').trim() === unit)
  const body = rows.map((r: any) => [
    r.meter_alias || '—',
    r.meter_kind === 'dorm' ? '宿舍' : '工商业',
    num2(r.start_reading), num2(r.end_reading), num2(r.rate),
    num2(r.usage), rateText(r.unit_price), money2(r.amount),
  ])
  return {
    title: `${prefix}电费核算明细`,
    columns: [
      { label: '电表', weight: 2.2 },
      { label: '用途', weight: 1, align: 'center' },
      { label: '起度', weight: 1.1, align: 'right' },
      { label: '止度', weight: 1.1, align: 'right' },
      { label: '倍率', weight: 0.8, align: 'right' },
      { label: '用电量', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '电费(元)', weight: 1.3, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', '', '', num2(sumBy(rows, 'usage')), '', money2(sumBy(rows, 'amount'))],
    empty: '当月无电费核算记录',
  }
}

/** 水费核算明细（独立区块，绝不与电费合表） */
function waterTableOf(month: string, unit: string, prefix: string): PrintTable {
  const meters = (waterMeters.value as any[]).filter((m: any) =>
    m.enabled !== false && (m.owner_unit === unit || m.use_unit === unit))
  const rows: any[] = []
  for (const m of meters) {
    const r = waterReadingMap.value.get(`${m.id}__${month}`)
    if (r) rows.push({ ...r, alias: m.alias })
  }
  const body = rows.map((r: any) => [
    r.alias || '—',
    num2(r.start_reading), num2(r.end_reading), num2(r.usage),
    rateText(r.unit_price ?? r.price), money2(r.amount),
  ])
  return {
    title: `${prefix}水费核算明细`,
    columns: [
      { label: '水表', weight: 2.6 },
      { label: '起度', weight: 1.2, align: 'right' },
      { label: '止度', weight: 1.2, align: 'right' },
      { label: '用量(吨)', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '水费(元)', weight: 1.4, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', num2(rows.reduce((s, r) => s + (Number(r.usage) || 0), 0)), '', money2(rows.reduce((s, r) => s + (Number(r.amount) || 0), 0))],
    empty: '当月无用水记录',
  }
}

/** 气费核算明细（独立区块；公租房不参与核算） */
function gasTableOf(month: string, unit: string, prefix: string): PrintTable {
  const rows = gasRows.value.filter((r: any) =>
    r.month === month && String(r.use_unit || '').trim() === unit && r.meter_kind !== 'public')
  const body = rows.map((r: any) => [
    r.meter_alias || '—',
    num2(r.start_reading), num2(r.end_reading), num2(r.usage),
    rateText(r.unit_price), money2(r.amount),
  ])
  return {
    title: `${prefix}气费核算明细`,
    columns: [
      { label: '气表', weight: 2.6 },
      { label: '起度', weight: 1.2, align: 'right' },
      { label: '止度', weight: 1.2, align: 'right' },
      { label: '用量(m³)', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '气费(元)', weight: 1.4, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', num2(rows.reduce((s, r) => s + (Number(r.usage) || 0), 0)), '', money2(rows.reduce((s, r) => s + (Number(r.amount) || 0), 0))],
    empty: '当月无燃气记录',
  }
}

/** 应收汇总：工商业电费 / 居民宿舍电费 / 水费 / 气费 → 合计 */
function summaryTableOf(rows: any[], unit: string, prefix: string): PrintTable {
  let ind = 0; let dorm = 0; let water = 0; let gas = 0
  for (const r of rows) {
    ind += Number(r.ind_fee) || 0
    dorm += (Number(r.dorm_fee) || 0) + (Number(r.res_fee) || 0)
    water += (Number(r.water_ind_fee) || 0) + (Number(r.water_dorm_fee) || 0) + (Number(r.water_fire_fee) || 0)
    gas += Number(r.gas_fee) || 0
  }
  const total = Math.round((ind + dorm + water + gas) * 100) / 100
  return {
    title: `${prefix}应收费用汇总`,
    columns: [
      { label: '费用项目', weight: 3 },
      { label: '使用单位', weight: 2 },
      { label: '金额(元)', weight: 1.6, align: 'right' },
    ],
    rows: [
      ['工商业电费（分摊后）', unit, money2(ind)],
      ['居民 / 宿舍电费', unit, money2(dorm)],
      ['水费（分表 + 宿舍）', unit, money2(water)],
      ['气费', unit, money2(gas)],
    ],
    total: ['应收合计', unit, money2(total)],
  }
}

/**
 * 打印清单（结算单）。
 *
 * 与「打印目录」的区别：目录 = 列表表头 + 行值（横向表格，内部对账）；
 * 清单 = 市电账单同款「概览 + 分项细项 + 水电气分区」纵向文档，可直接发租户。
 */
const handlePrintStatement = async () => {
  const arr = printableRows.value
  if (!arr.length) return
  statementBusy.value = true
  try {
    const unit = (arr[0]?.unit as string) || useUnit.value
    const months = arr.map((r: any) => r.month).filter(Boolean)
    const single = arr.length === 1
    const ratioList = arr.map((r: any) => ratioForRow(r)).filter((v) => v > 0)
    const ratio = ratioList.length ? ratioList[0] : 0
    const tables: PrintTable[] = []
    arr.forEach((row: any) => {
      const month = String(row.month || '')
      const prefix = single ? '' : `${month} · `
      // 租户视角必须按真实分摊比例；ratio=0（当月未生成/未抄表）时按 0 列示，
      // 绝不能兜底成 1 —— 那会把「整张市电账单」当成该租户的份额打印出去。
      const r = isOwnerView.value ? 1 : ratioForRow(row)
      tables.push(feeTableOf(month, r, prefix))
      tables.push(elecTableOf(month, unit, prefix))
      tables.push(waterTableOf(month, unit, prefix))
      const gas = gasTableOf(month, unit, prefix)
      if (gas.rows.length) tables.push(gas)
      tables.push(summaryTableOf([row], unit, prefix))
    })
    const totalKwh = arr.reduce((s, r: any) => s + (Number(r.total_kwh) || 0), 0)
    const totalFee = arr.reduce((s, r: any) => s + (Number(r.total_fee) || 0), 0)
    const payable = arr.reduce((s, r: any) => s
      + (Number(r.ind_fee) || 0) + (Number(r.dorm_fee) || 0) + (Number(r.res_fee) || 0)
      + (Number(r.water_ind_fee) || 0) + (Number(r.water_dorm_fee) || 0) + (Number(r.water_fire_fee) || 0)
      + (Number(r.gas_fee) || 0), 0)
    const doc = {
      title: `${unit} 费用清单`,
      subtitle: `${months.join('、')} · 使用单位 ${unit} · ${isOwnerView.value ? '业主全厂口径' : '租户分摊口径'}`,
      overviewTitle: '一、账单概览',
      meta: [
        { label: '账单周期', value: months.join('、') },
        { label: '使用单位', value: unit },
        { label: '计费方式', value: isOwnerView.value ? '业主全厂（不分摊）' : '按用电量分摊比例折算' },
        { label: '分摊比例', value: isOwnerView.value ? '—' : (ratio > 0 ? `${(ratio * 100).toFixed(2)}%` : '—') },
        { label: '账单户号', value: String(billIndex.value.get(months[0])?.item?.account_no || '—') },
        { label: '生成时间', value: new Date().toLocaleString('zh-CN', { hour12: false }) },
      ],
      kpis: [
        { label: '本期总电量', value: `${num2(totalKwh)} kWh` },
        { label: '本期电费合计', value: `${money2(totalFee)} 元` },
        { label: isOwnerView.value ? '工商业电费' : '分摊电费合计', value: `${money2(arr.reduce((s, r: any) => s + (Number(r.ind_fee) || 0), 0))} 元` },
        { label: '应收合计', value: `${money2(payable)} 元` },
      ],
      tables,
      footerNote: isOwnerView.value || ratio > 0
        ? '说明：市电分项按账单费用组成逐项列示；租户视角下分项电量与金额均已按当月用电量分摊比例折算；水、电、气核算明细按使用单位直接归属，单独成表。'
        : '说明：当月尚未生成该租户的分摊结果（无市电账单或缺少分时抄表），电费分项暂按 0 列示；水、电、气核算明细按使用单位直接归属。',
    }
    const bytes = await generateStatementPdf({ doc })
    showPrint(`${unit} 费用清单（${arr.length} 条）`, bytes, `${unit}-费用清单.pdf`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '生成清单失败')
  } finally {
    statementBusy.value = false
  }
}

const handlePrintCatalog = async () => {
  const arr = printableRows.value
  if (!arr.length) return
  catalogBusy.value = true
  try {
    const columns: CatalogColumn[] = visibleColDefs.value.map(col => ({
      key: col.key,
      label: col.label,
      value: (r: any) => cellText(col.key, r),
    }))
    const bytes = await generateCatalogPdf({
      title: `${useUnit.value}月度账单`,
      columns,
      rows: arr,
    })
    showPrint(`${arr[0].month} 账单目录（${arr.length} 条）`, bytes, `${useUnit.value}-账单目录.pdf`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const handleDelete = async () => {
  const arr = selectedRows.value
  if (!arr.length) return
  try {
    for (const r of arr) await deleteBillingRecord(r.id)
    MessagePlugin.success('已删除')
    notifyEnergyDataChanged()
    clearSelection()
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 抽屉拖动调宽(宽度持久化) ----
// ---- 专属设置（租户档案 / 分摊子项） ----
// 原「租户设置」抽屉已融合进统一设置抽屉的「清单设置」Tab（TenantSettingsPanel）。
// 页面只保留列表需要的租户下拉，变更由面板 emit('changed') 后 reloadTenants 刷新。
function openFieldSettings() { emit('open-settings') }
/**
 * 清单设置面板改完租户后的回灌。
 *
 * 面板里可能删掉当前正在看的租户：不校验会让 activeTenantId 指向一个已删除的
 * id，loadAllData 直接 404，列表变空且没有任何提示。这里先纠回第一个可用租户。
 */
const reloadTenants = async () => {
  await loadTenants()
  if (activeTenantId.value && !tenants.value.some((t: any) => t.id === activeTenantId.value)) {
    activeTenantId.value = tenants.value[0]?.id || ''
    await loadAllData()
  }
}
defineExpose({ reloadTenants })

// ---- 分时电表 CRUD(按归属单位分组 + 内联表单) ----
// ---- 账单详情与打印 ----
// 行点击详情：与发票同款 BusinessSplitDetailDrawer。
//
// 数据来源是**行本身**（buildRows 的核算结果）而不是 billing_records：
// 业主视角多数月份没有固化账单，早期实现「找不到 rec 就 return」正是
// 「费用清单点行没反应」的直接原因。字段顺序跟随「字段定义（subs）」。
const recordVisible = ref(false)
const recordRow = ref<any>(null)
const recordForm = ref<Record<string, any>>({})
// 说明：不向 BSD 传 :dirty —— 费用清单行是即时可编辑的核算值，
// 保存按钮恒可点（BSD 的 dirty===false 会禁用保存，这里不需要该门禁）。
const printVisible = ref(false)
const printUrl = ref('')
const printBytes = ref<Uint8Array | ArrayBuffer | null>(null)
const printTitle = ref('')
const printFileName = ref('preview.pdf')

const recordTitle = computed(() => {
  const r = recordRow.value
  if (!r) return '费用清单'
  return `${r.month || ''} 费用清单 · ${r.unit || useUnit.value || ''}`
})

/**
 * 详情字段 = 费用清单 subs（启用项，按序）∩ 该行真实存在的键。
 *
 * 交集是必须的：subs 是业主视角 ∪ 租户视角的并集，直接铺开会给
 * 「业主视角」多出一堆只在租户视角有意义的空字段（分摊比例等）。
 * subs 不可用时回退当前可见列定义，保证任何情况下都有字段可看。
 */
const billingFields = computed<Array<{ name: string; label: string; dataType?: string }>>(() => {
  const row = recordRow.value || {}
  const has = (k: string) => k in row && row[k] !== undefined
  // 与列表当前生效的列严格同源：列上有的、值也有的才进抽屉。
  // （旧实现从 subs 出发，租户视角的分摊子项列不在 subs 里 → 抽屉缺字段。）
  const cols = activeColDefs.value.filter((c) => has(c.key))
  if (cols.length) {
    return cols.map((c) => ({
      name: c.key,
      label: c.label,
      dataType: /fee|amount|price|kwh|usage|total|ratio|gen|grid|_fee$/.test(c.key) ? 'number' : 'text',
    }))
  }
  return visibleColDefs.value.filter((c) => has(c.key)).map((c) => ({ name: c.key, label: c.label, dataType: 'text' }))
})

// BSD 表单契约 { data, remarkParts, items }：data 指向行副本，编辑不回写列表
const recordFormWrapper = computed(() => ({ data: recordForm.value, remarkParts: {}, items: [] }))

/**
 * 打开只读详情抽屉。
 *
 * 抽屉只渲染字段（到「备注」为止），不再追加电量/费用/水费明细块 ——
 * 那些数据在「打印清单」的结构化 PDF 里按水/电/气分区展示。
 */
const openRecord = (row: any) => {
  recordRow.value = row
  const editable: Record<string, any> = {}
  for (const f of billingFields.value) editable[f.name] = row[f.name] ?? ''
  recordForm.value = editable
  recordVisible.value = true
}

// 说明：费用清单抽屉是**只读**的（所有值都是核算结果）。
// 需要改数 → 走工具栏「生成账单 / 重新核算」；需要写备注 → 备注本身也属核算记录，
// 由生成管线维护。这里不再保留逐字段保存通道，避免绕过口径直接改金额。

/**
 * 抽屉底栏「打印」：走与浮动栏「打印清单」同一套 PDF（`概览 + 细项 + 水电气分区`）。
 * 旧实现是克隆抽屉 DOM 交给浏览器打印 —— 抽屉里已经没有明细块（按需求移除），
 * 而且 DOM 打印的分页/表头完全不可控。
 */
const printRecord = () => {
  const row = recordRow.value
  if (!row) return
  statementBusy.value = true
  void (async () => {
    try {
      const unit = String(row.unit || useUnit.value || '')
      const month = String(row.month || '')
      const ratio = isOwnerView.value ? 1 : ratioForRow(row)
      const bytes = await generateStatementPdf({
        doc: {
          title: `${unit} 费用清单`,
          subtitle: `${month} · 使用单位 ${unit} · ${isOwnerView.value ? '业主全厂口径' : '租户分摊口径'}`,
          overviewTitle: '一、账单概览',
          meta: [
            { label: '账单周期', value: month },
            { label: '使用单位', value: unit },
            { label: '计费方式', value: isOwnerView.value ? '业主全厂（不分摊）' : '按用电量分摊比例折算' },
            { label: '分摊比例', value: isOwnerView.value ? '—' : `${(ratio * 100).toFixed(2)}%` },
          ],
          kpis: [
            { label: '本期总电量', value: `${num2(row.total_kwh)} kWh` },
            { label: '本期电费合计', value: `${money2(row.total_fee)} 元` },
            { label: isOwnerView.value ? '工商业电费' : '分摊电费合计', value: `${money2(row.ind_fee)} 元` },
            { label: '应收合计', value: `${money2((Number(row.ind_fee) || 0) + (Number(row.dorm_fee) || 0) + (Number(row.res_fee) || 0) + (Number(row.water_ind_fee) || 0) + (Number(row.water_dorm_fee) || 0) + (Number(row.gas_fee) || 0))} 元` },
          ],
          tables: [
            feeTableOf(month, ratio, ''),
            elecTableOf(month, unit, ''),
            waterTableOf(month, unit, ''),
            ...(gasTableOf(month, unit, '').rows.length ? [gasTableOf(month, unit, '')] : []),
            summaryTableOf([row], unit, ''),
          ],
          footerNote: isOwnerView.value || ratio > 0
            ? '说明：市电分项按账单费用组成逐项列示；租户视角下分项电量与金额均已按当月用电量分摊比例折算；水、电、气核算明细按使用单位直接归属，单独成表。'
            : '说明：当月尚未生成该租户的分摊结果（无市电账单或缺少分时抄表），电费分项暂按 0 列示；水、电、气核算明细按使用单位直接归属。',
        },
      })
      showPrint(`${unit} 费用清单`, bytes, `${unit}-${month}-费用清单.pdf`)
    } catch (e: any) {
      MessagePlugin.error(e?.message || '生成清单失败')
    } finally {
      statementBusy.value = false
    }
  })()
}

const showPrint = (title: string, bytes: ArrayBuffer | Uint8Array, fileName = 'preview.pdf') => {
  printTitle.value = title
  printFileName.value = fileName
  if (printUrl.value) URL.revokeObjectURL(printUrl.value)
  printBytes.value = bytes
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
  printVisible.value = true
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
  printBytes.value = null
}

// ---- 抽屉拖动调宽(宽度持久化) ----

// ---- 账单费用分组的排序基准（打印清单「电费分项细项」按此顺序） ----
const FEE_CATEGORY_ORDER = ['市场化购电费', '上网环节线损', '输配电', '系统运行费', '政府性基金及附加', '居民', '基本电费', '功率因素调整电费']

// ---- 格式化 ----
const fmtMoney = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  const r = Math.round(n * 100) / 100
  return Number.isInteger(r) ? String(r) : r.toFixed(2)
}
const fmtKwh = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  const r = Math.round(n * 100) / 100
  return Number.isInteger(r) ? String(r) : String(r)
}
const fmtRatio = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return (n * 100).toFixed(2) + '%'
}
const MONEY_KEYS = ['total_fee', 'ind_fee', 'res_fee', 'dorm_fee', 'water_ind_fee', 'water_dorm_fee', 'water_fire_fee', 'gas_fee', 'solar_amount']
const KWH_KEYS = ['total_kwh', 'ind_kwh', 'res_kwh', 'dorm_kwh', 'water_ind_usage', 'water_dorm_usage', 'water_fire_usage', 'gas_usage', 'solar_gen', 'solar_grid']
const cellText = (key: string, row: any): string => {
  if (key === 'ratio') return fmtRatio(row.ratio)
  if (key === 'ind_price') return fmtPrice(row.ind_price)
  if (MONEY_KEYS.includes(key)) return fmtMoney(row[key])
  if (KWH_KEYS.includes(key)) return fmtKwh(row[key])
  return row[key] || ''
}
const fmtPrice = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(4)))
}

onMounted(() => {
  loadTenants()
  void loadCategorySubs()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
function onCategoriesChanged() { void loadCategorySubs() }
onBeforeUnmount(() => window.removeEventListener('fleet-categories-changed', onCategoriesChanged))
</script>

<style lang="less" scoped>
/* 中台列表共享样式：.doc-list-scroll / .doc-list-view / 表头遮罩 / 表格令牌。
   子组件是挂在 .biz-page 下的（父页面提供），但共享样式是按组件独立 import 的，
   不引入就会退化成 display:block + overflow:visible —— 表格不被限高、
   滚动条落在外层容器上，表现就是「滚动条侵入表头 + 汇总栏被顶到几千像素」。 */
@import './styles/business-list-shared.css';

.tenant-billing-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  padding-top: 4px;
}

/* 筛选工具栏（与电费/合同管理一致） */
.doc-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;

  &__leading {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1;
  }
  &__trailing {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .doc-filter-field {
    display: flex;
    align-items: center;
    .doc-date-picker { width: 160px; }
    .doc-filter-select { width: 160px; }
  }
}

.field-popup-content {
  width: 240px;
  padding: 12px;
  box-sizing: border-box;
  .field-popup-head {
    display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;
    .field-popup-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
    .field-popup-actions { display: flex; gap: 0; }
  }
  .field-popup-list {
    display: flex; flex-direction: column; gap: 6px; max-height: 320px; overflow-y: auto;
    .field-popup-item { display: flex; align-items: center; }
  }
}

/* 列表（与电费/合同管理一致） */
.doc-list-view { width: 100%; min-width: 100%; box-sizing: border-box; }
.doc-list-header,
.doc-list-row {
  display: grid;
  align-items: center;
  padding: 0 16px;
  min-width: 100%;
  box-sizing: border-box;
}
.doc-list-header {
  position: sticky;
  top: 0;
  z-index: 5;
  height: 40px;
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-stroke);
  .cell { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .col-tip {
    cursor: help;
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: middle;
  }
}
.doc-list-body { display: flex; flex-direction: column; }
.doc-list-row {
  position: relative;
  min-height: 52px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background-color 0.2s ease;
  &:last-child { border-bottom: 0; }
  &:hover { background: var(--td-bg-color-secondarycontainer); }
  &.row-selected { background: var(--td-brand-color-light); }
}
.cell {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  padding: 0 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  &:first-child { padding-left: 0; }
  &:last-child { padding-right: 0; }
}
.cell-check {
  justify-content: center;
  position: sticky;
  left: 0;
  z-index: 2;
  background: transparent;
  padding: 0;
}
.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }
.row-mono,
.row-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-mono { font-family: var(--app-font-family); }
.os-neg { color: var(--td-error-color); }
.strong { font-weight: 600; }

.tenant-list-scroll {
  /* 只保留尺寸：滚动由 t-table 内部承担，外层不滚（否则滚动条侵表头） */
  flex: 1;
  min-height: 0;
}

/* 底部汇总 */
.doc-summary-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 2px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .doc-summary-count { font-weight: 600; color: var(--td-text-color-primary); }
  .doc-summary-item {
    display: inline-flex; align-items: baseline; gap: 6px;
    .doc-summary-val { font-variant-numeric: tabular-nums; color: var(--td-text-color-primary); font-weight: 600; }
  }
  /* 汇总栏已进卡片内部：不再用 margin-bottom 上移（旧版挂在卡片外才需要），
     浮动栏与它重叠的表现与发票管理一致。 */
}

/* 抽屉 resize 手柄（发票管理同款） */
.doc-drawer-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2200;
  cursor: col-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  .doc-drawer-resize-line {
    width: 2px;
    height: 40px;
    border-radius: 1px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity 0.15s ease, height 0.15s ease;
  }
  &:hover .doc-drawer-resize-line { opacity: 1; height: 80px; }
}

/* 浮动工具栏 */
.doc-batch-bar-fixed {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: 24px;
  z-index: 3000;
  .batch-bar-inner {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    border-radius: 10px;
    background: var(--td-bg-color-container);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
    border: 1px solid var(--td-component-stroke);
  }
  .batch-bar-left {
    display: flex;
    align-items: center;
    gap: 8px;
    .batch-bar-count { font-size: 13px; color: var(--td-text-color-primary); }
    .batch-bar-clear { color: var(--td-brand-color); }
  }
  .batch-bar-actions { display: flex; align-items: center; gap: 8px; }
}
.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active { transition: opacity 0.18s ease, transform 0.18s ease; }
.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to { opacity: 0; transform: translateX(-50%) translateY(6px); }

/* 读数抽屉（单表连续录入：尖峰平谷 4 时段起止度，复刻电费新增记录样式） */
.reading-body { padding: 4px 0 24px; }
.reading-type-switch { margin-bottom: 14px; }
.period-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 16px;
  border: 1px solid var(--td-component-border);
  border-radius: 8px;
  padding: 12px;
  .period-row {
    display: grid;
    grid-template-columns: 34px minmax(0, 1fr) 18px minmax(0, 1fr) auto;
    align-items: center;
    gap: 8px;
    &.row-invalid {
      .period-label { color: var(--td-error-color); }
      :deep(.t-input) { border-color: var(--td-error-color); }
    }
    .period-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      text-align: center;
    }
    .period-input {
      :deep(.t-input__inner) { text-align: center; }
    }
    .rr-sep { color: var(--td-text-color-placeholder); font-size: 12px; text-align: center; }
    .period-kwh { font-size: 12px; color: var(--td-text-color-secondary); text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  }
}
.calc-val {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 14px;
  padding: 10px 12px;
  background: var(--td-brand-color-light);
  border-radius: 8px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .row-mono { color: var(--td-text-color-primary); font-weight: 600; font-size: 15px; }
}
.readonly-val {
  height: 32px;
  line-height: 32px;
  padding: 0 12px;
  border-radius: var(--td-radius-default, 6px);
  background: var(--td-bg-color-component);
  color: var(--td-text-color-primary);
  font-size: 14px;
}
.field-hint { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 4px; line-height: 1.4; }
.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 设置抽屉 */
.settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  .settings-header-title { font-size: 16px; font-weight: 600; }
  .settings-header-actions { display: flex; align-items: center; gap: 8px; }
}
.settings-tabs { height: 100%; }
.tenant-settings-drawer :deep(.t-drawer__body) { overflow-y: auto; }
.settings-tabs :deep(.t-tabs__content) { height: calc(100% - 48px); overflow-y: auto; }
.settings-panel { padding: 4px 0 40px; }
.rec-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 20px;
}
.rec-field {
  min-width: 0;
  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;
  }
  .required { color: var(--td-error-color); }
}
.rec-field--wide { grid-column: 1 / -1; }

/* 租户卡片(多租户) */
.tenant-cards {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  align-items: start;
  .tenant-card {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    padding: 12px;
    transition: all 0.2s;
    &:hover { border-color: var(--td-brand-color); box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06); }
    .tenant-card-head {
      display: flex;
      align-items: center;
      gap: 8px;
      .tenant-card-name { font-size: 13px; font-weight: 600; }
      .tenant-card-no { font-size: 12px; color: var(--td-text-color-secondary); }
      .tenant-card-actions { margin-left: auto; display: flex; align-items: center; }
    }
    .tenant-card-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 6px 12px;
      margin-top: 10px;
      .tenant-card-item {
        font-size: 12px;
        display: flex;
        gap: 6px;
        min-width: 0;
        .k { color: var(--td-text-color-secondary); flex-shrink: 0; }
        .v { color: var(--td-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      }
    }
  }
  .meter-empty { grid-column: span 2; }
}

/* 内联表单(租户/电表) */
.tenant-form, .meter-form {
  grid-column: span 2;
  border: 1px solid var(--td-component-border);
  border-radius: 8px;
  padding: 14px;
  background: var(--td-bg-color-container);
  .tenant-form-title, .meter-form-title { font-size: 13px; font-weight: 600; margin-bottom: 12px; }
  .form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
}

.meter-section {
  margin-bottom: 20px;
  .meter-section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
    .meter-section-title { font-size: 13px; font-weight: 600; }
  }
}
.meter-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  align-items: start;
  .meter-card {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    padding: 12px;
    transition: all 0.2s;
    &:hover { border-color: var(--td-brand-color); box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06); }
    &.disabled { opacity: 0.55; }
    .meter-card-head {
      display: flex;
      align-items: center;
      gap: 8px;
      .meter-card-name { font-size: 13px; font-weight: 500; }
      .meter-card-actions { margin-left: auto; display: flex; align-items: center; gap: 2px; }
    }
    .meter-card-grid {
      display: grid;
      grid-template-columns: 1fr 1fr 1fr;
      gap: 6px 10px;
      margin-top: 10px;
      .meter-card-item {
        font-size: 12px;
        display: flex;
        gap: 4px;
        min-width: 0;
        .k { color: var(--td-text-color-secondary); flex-shrink: 0; }
        .v { color: var(--td-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      }
    }
  }
  .meter-empty {
    grid-column: span 2;
    padding: 24px;
    text-align: center;
    color: var(--td-text-color-placeholder);
    font-size: 12px;
    border: 1px dashed var(--td-component-border);
    border-radius: 8px;
  }
}

/* 分摊子项大类分组(内联展开) */
.item-groups {
  display: flex;
  flex-direction: column;
  gap: 8px;
  .item-group {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    overflow: hidden;
    .item-group-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      padding: 10px 12px;
      cursor: pointer;
      transition: border-color 0.2s, box-shadow 0.2s;
      &:hover { background: var(--td-bg-color-secondarycontainer); }
      .item-group-name { font-size: 13px; font-weight: 500; color: var(--td-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .item-group-arrow { color: var(--td-text-color-placeholder); }
    }
    .item-group-body {
      border-top: 1px solid var(--td-component-stroke);
      padding: 10px 14px;
    }
  }
  .item-empty { padding: 24px; text-align: center; color: var(--td-text-color-placeholder); font-size: 12px; }
}
.item-row {
  display: grid;
  grid-template-columns: 1fr 120px;
  align-items: center;
  gap: 12px;
  padding: 9px 2px;
  border-bottom: 1px solid var(--td-component-stroke);
  &:last-child { border-bottom: none; }
  &.item-row--head {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    border-bottom: 1px solid var(--td-component-border);
    padding: 4px 2px 7px;
  }
  .item-name { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .item-op { display: flex; justify-content: center; }
}
.item-empty { padding: 24px; text-align: center; color: var(--td-text-color-placeholder); font-size: 12px; }

.op { cursor: pointer; color: var(--td-text-color-secondary); transition: color 0.2s; &:hover { color: var(--td-brand-color); } &.danger:hover { color: var(--td-error-color); } }
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
  .form-item {
    display: flex;
    flex-direction: column;
    gap: 6px;
    label { font-size: 12px; color: var(--td-text-color-secondary); .required { color: var(--td-error-color); } }
    &.form-item--full { grid-column: span 2; }
  }
}
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }

/* 详情抽屉 */
.tb-items { overflow-x: auto; }
.tb-items-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;

  th, td {
    padding: 6px 8px;
    text-align: left;
    border-bottom: 1px solid var(--td-component-stroke);
    white-space: nowrap;
  }
  th { color: var(--td-text-color-secondary); font-weight: 500; background: var(--td-bg-color-secondarycontainer); }
  td { color: var(--td-text-color-primary); }
  tr:last-child td { border-bottom: none; }
  .tb-total td { font-weight: 600; background: var(--td-bg-color-secondarycontainer); }
}
.tb-period { margin-left: 4px; color: var(--td-text-color-placeholder); font-size: 11px; }
.tb-items-empty { padding: 14px 0; color: var(--td-text-color-placeholder); font-size: 12px; }
.tb-fee-group { margin-bottom: 10px; }
.tb-fee-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
.tb-fee-name { font-size: 12px; font-weight: 600; color: var(--td-text-color-primary); }
.tb-fee-sum { font-size: 12px; color: var(--td-text-color-secondary); font-variant-numeric: tabular-nums; }
.tb-fee-total { margin-top: 8px; text-align: right; font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }

.record-detail {
  .record-overview {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    margin-bottom: 16px;
    .ro-item {
      border: 1px solid var(--td-component-border);
      border-radius: 8px;
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 6px;
      .ro-label { font-size: 12px; color: var(--td-text-color-secondary); }
      .ro-value { font-size: 15px; font-weight: 600; color: var(--td-text-color-primary); }
    }
  }
  .record-items {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    overflow: hidden;
    .record-items-head, .record-items-row {
      display: grid;
      grid-template-columns: 1.6fr 0.7fr 1fr 1fr 1.1fr;
      gap: 8px;
      padding: 9px 12px;
      font-size: 13px;
    }
    .record-items-head {
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-secondary);
      font-size: 12px;
    }
    .record-items-row {
      border-bottom: 1px solid var(--td-component-stroke);
      &:last-child { border-bottom: none; }
      &.record-items-total { background: var(--td-brand-color-light); font-weight: 600; }
    }
  }

  /* 明细分组卡片 */
  .rd-card {
    margin-bottom: 16px;
    .rd-card-title {
      font-size: 14px;
      font-weight: 600;
      margin-bottom: 10px;
      color: var(--td-text-color-primary);
    }
  }
  .rd-fee-group {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    margin-bottom: 12px;
    overflow: hidden;
    .rd-fee-group-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 9px 12px;
      background: var(--td-bg-color-secondarycontainer);
      .rd-fee-group-name { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
      .rd-fee-group-sum { font-size: 12px; color: var(--td-text-color-secondary); font-variant-numeric: tabular-nums; }
    }
    .record-items {
      border: none;
      border-radius: 0;
      border-top: 1px solid var(--td-component-stroke);
    }
  }
  .rd-meter-grid {
    .rd-meter-head, .rd-meter-row {
      grid-template-columns: 1.3fr 0.8fr 0.8fr 0.6fr 0.9fr 0.8fr 0.8fr 0.9fr 0.9fr;
    }
    .rd-period { color: var(--td-text-color-secondary); font-size: 12px; margin-left: 4px; }
  }
  .rd-water-grid {
    .rd-water-head, .rd-water-row {
      grid-template-columns: 1.3fr 0.9fr 0.9fr 1fr 1fr 1fr;
    }
  }
}

/* 打印预览 */
</style>

<style>
/* 账单明细直接打印（纵向 A4）：克隆节点脱离抽屉 fixed 容器，可多页分页 */
@media print {
  @page { size: A4 portrait; margin: 12mm; }
  body * { visibility: hidden !important; }
  #rd-print-clone,
  #rd-print-clone * { visibility: visible !important; }
  #rd-print-clone {
    position: absolute !important;
    left: 0 !important;
    top: 0 !important;
    width: 100% !important;
    max-height: none !important;
    overflow: visible !important;
    background: var(--td-bg-color-container) !important;
    padding: 0 !important;
  }
  #rd-print-clone .record-overview { grid-template-columns: repeat(4, 1fr) !important; page-break-inside: avoid; }
  #rd-print-clone .rd-fee-group { page-break-inside: avoid; break-inside: avoid; }
  #rd-print-clone .rd-print-title {
    display: block !important;
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 12px;
  }
}
.rd-print-title { display: none; }
</style>
