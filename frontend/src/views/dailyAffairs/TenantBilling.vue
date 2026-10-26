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
        <t-button theme="primary" size="small" :loading="generateBusy"
          :title="isOwnerView ? '重新加载账单与表计数据并重新核算列表（数据有误时可随时点击）' : '为当前筛选范围内未生成账单的月份生成月度账单（数据齐全时自动生成）'"
          @click="handleGenerateAll">
          <template #icon><t-icon name="refresh" size="14px" /></template>
          {{ isOwnerView ? '重新核算' : '生成账单' }}
        </t-button>
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
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" :disabled="!printableRows.length" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印清单
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
    <BusinessSplitDetailDrawer v-model:visible="recordVisible" :title="recordTitle" :row="null"
      :form="recordFormWrapper" :fields="billingFields" :show-preview="false"
      block-title="费用清单" :saving="recordSaving"
      :storage-key="'weknora-tenant-record-drawer-width'" @save="saveRecordDetail">
      <template #footer-left>
        <t-button variant="outline" size="small" @click="printRecord">打印</t-button>
      </template>
      <template #extra>
        <!-- 分摊明细（电量表 / 费用大项 / 水表）：只读表格，与发票「项目明细」同形 -->
        <section v-if="recordDetail" class="bsd-block">
          <div class="bsd-block-title">电量明细</div>
          <div v-if="recordDetail.meters?.length" class="tb-items">
            <table class="tb-items-table">
              <thead>
                <tr><th>分时电表</th><th>起度</th><th>止度</th><th>倍率</th><th>使用电量</th><th>损耗</th><th>加减电量</th><th>计费电量</th><th>差额分摊</th></tr>
              </thead>
              <tbody>
                <tr v-for="(mr, i) in recordDetail.meters" :key="i">
                  <td>{{ mr.meter_name }}<span class="tb-period">{{ mr.period }}</span></td>
                  <td class="row-mono">{{ fmtNum(mr.prev) }}</td>
                  <td class="row-mono">{{ fmtNum(mr.curr) }}</td>
                  <td class="row-mono">{{ fmtNum(mr.rate) }}</td>
                  <td class="row-mono">{{ fmtKwh(mr.usage) }}</td>
                  <td class="row-mono">{{ fmtKwh(mr.line_loss) }}</td>
                  <td class="row-mono">{{ fmtKwh(mr.adjust) }}</td>
                  <td class="row-mono">{{ fmtKwh(mr.bill_kwh) }}</td>
                  <td class="row-mono">{{ fmtKwh(mr.diff_kwh) }}</td>
                </tr>
                <tr class="tb-total">
                  <td>汇总</td><td></td><td></td><td></td>
                  <td class="row-mono">{{ fmtKwh(recordMeterSum('usage')) }}</td>
                  <td class="row-mono">{{ fmtKwh(recordMeterSum('line_loss')) }}</td>
                  <td class="row-mono">{{ fmtKwh(recordMeterSum('adjust')) }}</td>
                  <td class="row-mono">{{ fmtKwh(recordMeterSum('bill_kwh')) }}</td>
                  <td class="row-mono">{{ fmtKwh(recordMeterSum('diff_kwh')) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="tb-items-empty">该月未固化分时电量明细</div>
        </section>

        <section v-if="recordDetail && feeGroups.length" class="bsd-block">
          <div class="bsd-block-title">费用明细</div>
          <div v-for="g in feeGroups" :key="g.category" class="tb-fee-group">
            <div class="tb-fee-head">
              <span class="tb-fee-name">{{ g.category }}</span>
              <span class="tb-fee-sum">小计 {{ fmtMoney(g.sum) }}</span>
            </div>
            <table class="tb-items-table">
              <thead>
                <tr><th>项目</th><th>时段</th><th>电量</th><th>单价</th><th>费用</th></tr>
              </thead>
              <tbody>
                <tr v-for="(it, i) in g.rows" :key="i" :class="{ 'os-neg': Number(it.fee) < 0 }">
                  <td>{{ it.name }}</td>
                  <td>{{ it.period || '—' }}</td>
                  <td class="row-mono">{{ fmtKwh(it.qty) }}</td>
                  <td class="row-mono">{{ fmtRate(it.rate) }}</td>
                  <td class="row-mono">{{ fmtMoney(it.fee) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="tb-fee-total">合计 {{ fmtMoney(totalElecFee) }} 元</div>
        </section>

        <section v-if="recordDetail?.waters?.length" class="bsd-block">
          <div class="bsd-block-title">水费明细（小计 {{ fmtMoney(recordDetail.record.water_fee) }} 元）</div>
          <table class="tb-items-table">
            <thead>
              <tr><th>水表</th><th>起度</th><th>止度</th><th>用量</th><th>单价</th><th>水费</th></tr>
            </thead>
            <tbody>
              <tr v-for="(wr, i) in recordDetail.waters" :key="i">
                <td>{{ wr.meter_name }}</td>
                <td class="row-mono">{{ fmtNum(wr.prev) }}</td>
                <td class="row-mono">{{ fmtNum(wr.curr) }}</td>
                <td class="row-mono">{{ fmtKwh(wr.usage) }}</td>
                <td class="row-mono">{{ fmtRate(wr.price) }}</td>
                <td class="row-mono">{{ fmtMoney(wr.fee) }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 打印预览 -->
    <div v-if="printVisible" class="tenant-print-mask">
      <div class="tenant-print-dialog">
        <div class="tenant-print-header">
          <span class="tenant-print-title">{{ printTitle }} 打印预览</span>
        </div>
        <div class="tenant-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading"><t-loading size="small" /></div>
        </div>
        <div class="tenant-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants,
  listBillingRecords, generateBillingRecord, getBillingRecord, updateBillingRecord, deleteBillingRecord,
  listKnowledgeBases,
  listUtilityBillRecords, listSolarBillRecords, listUtilityMeterRecords, listUtilityMeters,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { rowFlattenText, matchAllTokens, notifyEnergyDataChanged } from './energyCounters'
import EmptyState from '@/components/EmptyState.vue'

// ---- 使用单位(房东/租户) ----
import { useTableHeaderFix } from './useTableHeaderFix'
import { loadEnergyCategorySubs, applySubsToColumns, type EnergySub } from './energyViewColumns'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
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
interface ColDef { key: string; label: string; default: boolean; w: string; tip?: string }
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
/**
 * 生效列定义 = 费用清单分类的字段定义（subs）。
 *
 * 两套视角（业主 / 租户）列集不同，而 subs 是并集，所以这里 restrictToBase：
 * 只保留当前视角本来就有的列，再用 subs 的名字/顺序/宽度/默认显示覆盖。
 */
const baseColDefs = computed(() => (isOwnerView.value ? COL_DEFS_OWNER : COL_DEFS_TENANT))
const activeColDefs = computed(() =>
  applySubsToColumns(
    baseColDefs.value,
    categorySubs.value,
    undefined,
    { restrictToBase: true },
  ),
)

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
        meter_kind: meter?.meter_kind || 'dorm',
        meter_type: meter?.meter_type || '',
        use_unit: meter?.use_unit || (it as any).use_unit || '',
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

// ---- 生成月度账单（仅租户视图） ----
const generateBusy = ref(false)
// 星达视图：重新加载账单与表计数据并重新核算；租户视图：对未生成月份生成账单
const handleGenerateAll = async () => {
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
  const pending = displayRows.value.filter((r: any) => !r.hasRec)
  if (!pending.length) {
    MessagePlugin.warning('当前月份均已生成账单，如需重算请选中记录使用「重新生成」')
    return
  }
  const missingBill = pending.filter((r: any) => !r.billReady)
  const missingReading = pending.filter((r: any) => r.billReady && !r.readingReady)
  if (missingBill.length) {
    MessagePlugin.error(`以下月份缺少市电账单，无法生成：${missingBill.map((r: any) => r.month).join('、')}`)
    return
  }
  if (missingReading.length) {
    MessagePlugin.error(`以下月份缺少星达分表读数，无法生成：${missingReading.map((r: any) => r.month).join('、')}`)
    return
  }
  generateBusy.value = true
  try {
    const okMonths: string[] = []
    const fail: string[] = []
    for (const row of pending) {
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

// ---- 浮动工具栏：打印清单 ----
const catalogBusy = ref(false)
const printableRows = computed(() => (selectedKeys.value.size ? selectedRows.value : displayRows.value))
const handlePrint = async () => {
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
    showPrint(`${arr[0].month} 账单目录（${arr.length} 条）`, bytes)
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
const recordDetail = ref<any>(null)
const recordRow = ref<any>(null)
const recordSaving = ref(false)
const recordForm = ref<Record<string, any>>({})
// 说明：不向 BSD 传 :dirty —— 费用清单行是即时可编辑的核算值，
// 保存按钮恒可点（BSD 的 dirty===false 会禁用保存，这里不需要该门禁）。
const printVisible = ref(false)
const printUrl = ref('')
const printTitle = ref('')

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
  const subs = (categorySubs.value || []).filter((x: any) => x && x.enabled !== false && has(String(x.name)))
  if (subs.length) {
    return subs.map((x: any) => ({
      name: String(x.name),
      label: String(x.label || x.name),
      dataType: x.data_type === 'number' || x.data_type === 'amount' ? 'number'
        : x.data_type === 'date' ? 'date' : 'text',
    }))
  }
  return activeColDefs.value
    .filter((c) => has(c.key))
    .map((c) => ({ name: c.key, label: c.label, dataType: 'text' }))
})

// BSD 表单契约 { data, remarkParts, items }：data 指向行副本，编辑不回写列表
const recordFormWrapper = computed(() => ({ data: recordForm.value, remarkParts: {}, items: [] }))

const openRecord = async (row: any) => {
  recordRow.value = row
  const editable: Record<string, any> = {}
  for (const f of billingFields.value) editable[f.name] = row[f.name] ?? ''
  recordForm.value = editable
  recordDetail.value = null
  recordVisible.value = true
  if (!row?.hasRec) return
  const rec = records.value.find((r: any) => r.month === row.month)
  if (!rec) return
  try {
    const res: any = await getBillingRecord(rec.id)
    recordDetail.value = res?.data || null
  } catch { /* 明细拉取失败不影响主表单：质量面板与明细区静默 */ }
}

/**
 * 保存：把可人工调整的项写回 billing_records（备注 / 宿舍电费 / 水费 / 总应付）。
 *
 * 其余列是核算派生值（总电量、均价、工业电费…），改它们等于改口径，
 * 必须走「生成账单/重新核算」，不能在这里静默覆盖。
 */
const saveRecordDetail = async () => {
  const row = recordRow.value
  if (!row?.hasRec) {
    MessagePlugin.warning('该月尚未生成账单，请先生成账单后再编辑')
    return
  }
  const rec = records.value.find((r: any) => r.month === row.month)
  if (!rec) return
  recordSaving.value = true
  try {
    const payload: Record<string, unknown> = {}
    if ('remark' in recordForm.value) payload.remark = String(recordForm.value.remark ?? '')
    for (const k of ['dorm_fee', 'water_fee', 'total_fee']) {
      if (k in recordForm.value && recordForm.value[k] !== '' && recordForm.value[k] !== undefined) {
        payload[k] = Number(recordForm.value[k]) || 0
      }
    }
    if (!Object.keys(payload).length) {
      MessagePlugin.info('该行没有可保存的人工字段')
      return
    }
    await updateBillingRecord(rec.id, payload)
    MessagePlugin.success('已保存')
    recordVisible.value = false
    await loadAllData()
    notifyEnergyDataChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    recordSaving.value = false
  }
}

// 直接打印明细页面（浏览器打印，默认纵向 A4）
// 长内容可多页分页：克隆明细到 body 顶层(脱离抽屉 fixed 容器)后打印,每个大项分组尽量不跨页
const printRecord = () => {
  if (!recordDetail.value) return
  const src = document.querySelector('.bsd-drawer .bsd-body') as HTMLElement | null
  if (!src) return
  const clone = src.cloneNode(true) as HTMLElement
  clone.id = 'rd-print-clone'
  clone.style.cssText = 'position:absolute;left:0;top:0;width:100%;background:#fff;padding:16px;box-sizing:border-box;'
  document.body.appendChild(clone)
  window.print()
  setTimeout(() => { clone.remove() }, 200)
}

const showPrint = (title: string, bytes: ArrayBuffer | Uint8Array) => {
  printTitle.value = title
  if (printUrl.value) URL.revokeObjectURL(printUrl.value)
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
  printVisible.value = true
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 抽屉拖动调宽(宽度持久化) ----

// ---- 账单明细分组（按大项排序 + 汇总） ----
const FEE_CATEGORY_ORDER = ['市场化购电费', '上网环节线损', '输配电', '系统运行费', '政府性基金及附加', '居民', '基本电费', '功率因素调整电费']
const feeGroups = computed(() => {
  const items = (recordDetail.value?.items || []).filter((it: any) => it.category !== '水费')
  const map = new Map<string, any[]>()
  const order: string[] = []
  for (const it of items) {
    const cat = it.category || '其他费用'
    if (!map.has(cat)) { map.set(cat, []); order.push(cat) }
    map.get(cat)!.push(it)
  }
  order.sort((a, b) => {
    const ia = FEE_CATEGORY_ORDER.findIndex(c => a.includes(c))
    const ib = FEE_CATEGORY_ORDER.findIndex(c => b.includes(c))
    return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
  })
  return order.map(cat => ({
    category: cat,
    rows: map.get(cat)!,
    sum: Math.round(map.get(cat)!.reduce((s, it) => s + (Number(it.fee) || 0), 0) * 100) / 100,
  }))
})
const recordMeterSum = (key: string): number =>
  Math.round((recordDetail.value?.meters || []).reduce((s: number, mr: any) => s + (Number(mr[key]) || 0), 0) * 100) / 100
// 总电费 = 工业(分摊) + 宿舍,不含水费(水费归水费清单)
const totalElecFee = computed(() =>
  Math.round(((Number(recordDetail.value?.record?.total_fee) || 0) - (Number(recordDetail.value?.record?.water_fee) || 0)) * 100) / 100)

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
const fmtNum = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return String(parseFloat(n.toFixed(4)))
}
const fmtRate = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}
const fmtRatio = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return (n * 100).toFixed(2) + '%'
}
const MONEY_KEYS = ['total_fee', 'ind_fee', 'res_fee', 'dorm_fee', 'water_ind_fee', 'water_dorm_fee', 'water_fire_fee', 'gas_fee', 'solar_amount']
const KWH_KEYS = ['total_kwh', 'ind_kwh', 'res_kwh', 'dorm_kwh', 'water_ind_usage', 'water_dorm_usage', 'water_fire_usage', 'gas_usage', 'solar_gen', 'solar_grid']
const printLoaded = ref(false)
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
.tenant-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  .tenant-print-dialog {
    width: min(960px, 92vw);
    height: min(720px, 88vh);
    background: var(--td-bg-color-container);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    .tenant-print-header {
      display: flex;
      align-items: center;
      padding: 10px 16px;
      border-bottom: 1px solid var(--td-component-stroke);
      .tenant-print-title { font-size: 14px; font-weight: 600; }
    }
    .tenant-print-body {
      flex: 1;
      min-height: 0;
      .print-preview-frame { width: 100%; height: 100%; border: none; }
      .print-preview-loading { height: 100%; display: flex; align-items: center; justify-content: center; }
    }
    .tenant-print-footer {
      display: flex;
      justify-content: flex-end;
      padding: 10px 16px;
      border-top: 1px solid var(--td-component-stroke);
    }
  }
}
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
