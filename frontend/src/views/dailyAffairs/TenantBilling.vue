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
            :title="isOwnerView ? '重新加载账单与表计数据并重新核算列表（数据有误时可随时点击）' : '按所选记录生成账单；未选择则全部重新生成（缺项按 0 参与核算，不再拦截）'">
            <template #icon><t-icon name="refresh" size="14px" /></template>
            {{ isOwnerView ? '重新核算' : '生成账单' }}
          </t-button>
        </t-popconfirm>
      </template>
    </BusinessFilterBar>

    <!-- 列表 -->
    <div class="doc-list-scroll tenant-list-scroll">
      <div :class="listViewClass" :style="headerFixStyle">
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
        :expanded-row-keys="expandedRowKeys"
        :expand-icon="false"
        :row-class-name="({ row }: any) => selectedKeys.has(row.id) ? 'row-selected' : ''"
        :foot-data="footData"
        :rowspan-and-colspan-in-footer="footSpan"
        @select-change="(val: any) => onTableSelectChange(val)"
        @expand-change="onExpandChange"
      >
        <!-- 大类明细（手风琴）：点单元格的 + 在**当前行下方**展开，
             同时只允许展开一个大类（toggleFeeGroup 里保证互斥）。
             视觉 100% 对齐「电费核算」的分时明细：浅灰表头行 + 明细行 + 底部加粗合计行，
             没有标题横幅、没有卡片边框/阴影（旧版那套是另起的样式，已废弃）。 -->
        <template #expanded-row="{ row }">
          <div v-if="expandedGroupOfRow(row)" class="fee-group-panel"
            :style="groupPanelWidth ? { width: groupPanelWidth + 'px' } : undefined">
            <div v-if="expandedGroupOfRow(row).breakdownHint" class="fg-breakdown-hint">
              {{ expandedGroupOfRow(row).breakdownHint }}
            </div>
            <table class="fee-group-table">
              <colgroup>
                <col style="width: 18%" />
                <col style="width: 6%" />
                <col style="width: 9%" />
                <col style="width: 9%" />
                <col style="width: 9%" />
                <col style="width: 9%" />
                <col style="width: 11%" />
                <col style="width: 9%" />
                <col style="width: 11%" />
                <col style="width: 9%" />
              </colgroup>
              <tbody>
                <tr class="fg-head-row">
                  <td class="fg-cell">细项名称</td>
                  <td class="fg-cell">时段</td>
                  <td class="fg-cell fg-num">电表电量</td>
                  <td class="fg-cell fg-num">差额电量</td>
                  <td class="fg-cell fg-num">线损</td>
                  <td class="fg-cell fg-num">加减电量</td>
                  <td class="fg-cell fg-num">账单原值/计费数量</td>
                  <td class="fg-cell fg-num">单价/分摊比例</td>
                  <td class="fg-cell">计算方式</td>
                  <td class="fg-cell fg-num">分摊后金额(元)</td>
                </tr>
                <tr v-for="d in expandedGroupOfRow(row).rows" :key="d.key">
                  <td class="fg-cell">{{ d.label }}</td>
                  <td class="fg-cell">{{ d.period }}</td>
                  <td class="fg-cell fg-num">{{ d.breakdown ? fmtKwh(d.breakdown.meterQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ d.breakdown ? fmtKwh(d.breakdown.diffQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ d.breakdown ? fmtKwh(d.breakdown.lineLossQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ d.breakdown ? fmtKwh(d.breakdown.adjustQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ d.qty != null ? fmtKwh(d.qty) : fmtMoney(d.raw) }}</td>
                  <td class="fg-cell fg-num">{{ d.price != null ? d.price : (d.ratio != null ? fmtRatio(d.ratio) : '—') }}</td>
                  <td class="fg-cell">{{ d.method }}</td>
                  <td class="fg-cell fg-num">{{ fmtMoney(d.value) }}</td>
                </tr>
                <tr class="fg-total-row">
                  <td class="fg-cell" colspan="2">合计</td>
                  <td class="fg-cell fg-num">{{ expandedGroupOfRow(row).breakdownTotals ? fmtKwh(expandedGroupOfRow(row).breakdownTotals.meterQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ expandedGroupOfRow(row).breakdownTotals ? fmtKwh(expandedGroupOfRow(row).breakdownTotals.diffQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ expandedGroupOfRow(row).breakdownTotals ? fmtKwh(expandedGroupOfRow(row).breakdownTotals.lineLossQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ expandedGroupOfRow(row).breakdownTotals ? fmtKwh(expandedGroupOfRow(row).breakdownTotals.adjustQty) : '—' }}</td>
                  <td class="fg-cell fg-num">{{ expandedGroupOfRow(row).breakdownTotals ? fmtKwh(expandedGroupOfRow(row).breakdownTotals.billingQty) : '—' }}</td>
                  <td class="fg-cell fg-num" colspan="2"></td>
                  <td class="fg-cell fg-num">{{ fmtMoney(expandedGroupOfRow(row).total) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
        <template #empty>
          <EmptyState v-if="!loading" title="暂无数据" icon="search-error" compact />
        </template>
      </t-table>
    </div>
    <!-- 底部**不再**挂摘要栏（第 21 轮）：表尾合计行的首列已经是
         「合计（共 N 条）/ 合计（已选 K 条）」，条数与选中数都在里面；
         下方再放一条「共 N 条」既重复又占纵向空间（用户截图红框）。 -->
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

    <!-- 详情抽屉已下线（第 18 轮）：费用清单是纯核算结果表，点行不再弹右侧抽屉。
         查看/输出统一走底部浮动栏的「打印清单 / 打印目录」，
         行内交互只保留 复选框 / 大类 +− 展开 / 单元格 hover 口径浮窗。 -->

    <!-- 打印预览：全中台统一组件（壳子只有一份，内容由本页生成） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="`${printTitle} 打印预览`" :url="printUrl"
      :data="printBytes" :download-name="printFileName" loading-text="正在生成费用清单…" @close="closePrint" />

    <!-- 单元格口径浮层：独立组件（不参与本页渲染，避免 hover 触发整表重绘） -->
    <BillingCellTip />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, getBillingTenant,
  listBillingRecords, generateBillingRecord, regenerateBillingRecords, regenerateBillingByPeriod, deleteBillingRecord,
  listKnowledgeBases,
  listUtilityBillRecords, listSolarBillRecords, listUtilityMeterRecords, listUtilityMeters,
} from '@/api/knowledge-base'
import { cellNum, money2, round2 } from './formatNumber'
import { gasApplicableOf, buildTenantBillingStatementDoc, type TenantStatementCtx } from './energyBillStatements'
import { generateCatalogPdf, generateStatementPdf, loadCatalogFont, type CatalogColumn, type StatementDoc } from './useCatalogPdf'
import { useTableFlatState, flatClassOf } from './useTableFlatState'

import { rowFlattenText, matchAllTokens, notifyEnergyDataChanged, ENERGY_DATA_CHANGED_EVENT } from './energyCounters'
import { parseFieldSource, matchBillFeeItems, billRawAmountOf, resolveSourceValue, resolveTenantItemValue, resolveTimedItemFee, timedBreakdownOf, timedBreakdownTotalsOf, type FieldSource, type SourceResolveContext, type TimedAllocation } from './tenantBillingSources'
import {
  DEFAULT_GROUP_KEY,
  TENANT_ELEC_TOTAL_FIELD,
  TENANT_WATER_TOTAL_FIELD,
  TENANT_TOTAL_FIELDS,
  LEGACY_TOTAL_FIELDS,
  collapseFeeGroups,
  subsOfGroup,
  uniqueTenantItems,
  tenantGroupKey,
} from './billingFieldGroups'
import { buildRowTips, sourceFieldTip, tenantItemTip, tenantTimedTip } from './tenantBillingTips'
import { buildFootRow, footSpanOf, summaryScopeRows, type SummaryColumn } from './listSummary'
import EmptyState from '@/components/EmptyState.vue'

// ---- 使用单位(房东/租户) ----
import { useTableHeaderFix } from './useTableHeaderFix'
import { loadEnergyCategorySubs, type EnergySub } from './energyViewColumns'
import BusinessFilterBar from './BusinessFilterBar.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import BillingCellTip from './BillingCellTip.vue'
// 行内展开徽标的唯一样式源（与「电费核算」共用；h() 生成的节点拿不到 scoped 样式）
import './styles/row-expand-toggle.css'
// 逐列表尾汇总行样式（TDesign 表尾 td 拿不到 scoped 作用域）
import './styles/list-summary-footer.css'
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
  /**
   * 大类汇总列（市场化购电费 / 系统运行费用 / 政府性基金及附加）。
   *
   * 值 = 成员分摊后金额之和；单元格旁边有 +/−，点开在行下方展开成员明细。
   * 成员仍是普通分摊子项（各自有列 key），只是不出现在表头里。
   */
  feeGroup?: {
    name: string
    label: string
    category: string
    members: Array<{ key: string; label: string; itemKey: string }>
  }
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
  // 气费：租户分组的派生字段里有「用气量 / 气费」，这里必须有同名渲染器，
  // 否则 subs 里那两条会被 activeColDefs 当成无渲染器的键丢掉（表头缺列）。
  { key: 'gas_usage', label: '用气量', default: true, w: '0.9fr', tip: '气表记录汇总(公租房除外)' },
  { key: 'gas_fee', label: '气费', default: true, w: '0.9fr', tip: '气表记录费用汇总(用量×单价)' },
  // 金额收口列（第 18 轮由「合计应付」拆出）：
  // 电费合计 = 工业电费 + 宿舍电费；水费合计 = 工业水费 + 宿舍水费。
  { key: TENANT_ELEC_TOTAL_FIELD, label: '电费合计', default: true, w: '0.9fr', tip: '工业电费 + 宿舍电费' },
  { key: TENANT_WATER_TOTAL_FIELD, label: '水费合计', default: true, w: '0.9fr', tip: '工业水费 + 宿舍水费' },
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
 * **只过滤「启用字段」，不过滤「参与分摊」**，两者语义正交：
 *   · 参与分摊 = 金额按本月用电量比例折算并计入应付（算钱）；
 *   · 启用字段 = 这一列是否显示在列表 / 字段筛选器 / 详情抽屉（显示）。
 * 旧实现用同一个 `enabled` 兼两职，于是「只想隐藏一列」会连带把分摊关掉，
 * 而「关闭分摊」又会让列整个消失 —— 用户无法表达「参与分摊但不显示」。
 *
 * 关掉「参与分摊」的子项仍出现在列表里（值按 0 显示），这样用户能看出
 * 「这一项本月没计入」，而不是让它凭空消失、事后无法核对。
 */
const tenantEnabledItems = computed(() =>
  tenantItems.value.filter((it: any) => it && it.field_enabled !== false),
)

/**
 * 去重后的分摊子项。
 *
 * 库里同一个费用项可能存在多条记录（功率因数调整电费 / 功率因素调整电费 /
 * 功率因数调整），item_key 各不相同。列表列早已按签名去重，但**取值循环**
 * 没有 —— 力调被算两次（-598.43 × 2），「合计应付」因此比「工业电费」少 598。
 * 去重规则收在 billingFieldGroups.tenantItemSignature（与字段配置同一份），
 * 优先保留「字段定义里引用到的」那条，避免列表丢列。
 */
const tenantItemsUnique = computed(() => {
  const inSubs = new Set((categorySubs.value || []).map((s: any) => String(s?.name || '').trim()))
  return uniqueTenantItems(tenantEnabledItems.value, inSubs)
})

/** 参与分摊的子项（金额折算口径）。 */
const tenantApportionItems = computed(() =>
  tenantItemsUnique.value.filter((it: any) => it.enabled !== false),
)

/** 分摊子项列 key（item_key 内已含「大类|子项名」，加前缀避免与内置列撞名）。 */
const tenantItemColKey = (it: any) => `item_${String(it?.item_key || it?.itemKey || '')}`

/**
 * 生效列定义 —— **严格以「字段定义（subs）」的下标顺序为唯一主序**。
 *
 * 三端（设置抽屉 / `[☰ 字段]` 筛选器 / 列表表头）必须同名、同量、同序。
 * 旧实现是「先铺内置列（按 subs 里的相对顺序），再把带 source 的跨分类引用列
 * 追加到末尾」——于是用户把「账单周期 / 使用单位 / 本期电量…」拖到定义列表最前面
 * 保存后，列表里它们仍被压在十几列之后（真机问题 1）。
 *
 * 遍历口径（subs[0] → subs[n-1]，一步不跳）：
 *   · `enabled === false` → 跳过（关了「启用字段」的字段不得出现在任何一端）；
 *   · 带 `source` → 跨分类引用列；
 *   · 否则按 key 命中**当前视角**的内置渲染器 → 内置列（名称/宽度/默认显隐取 subs）。
 * 当前视角没有渲染器的键（如业主视角里的「分摊比例」）不产出列 —— 生成了也是空列。
 */
const activeColDefs = computed<ColDef[]>(() => {
  const allSubs = categorySubs.value || []
  const base = baseColDefs.value
  const byKey = new Map(base.map((c) => [String(c.key), c]))
  // 当前生效的分组：业主单位 → 默认分组；具体租户 → tenant:<id> 分组。
  // 「字段定义」把每个分组各存一份（subs.group），这里只取当前单位那一份，
  // 否则业主表头会混进租户的分摊子项、租户表头会混进默认分组的引用字段。
  const groupKey = isOwnerView.value ? DEFAULT_GROUP_KEY : tenantGroupKey(activeTenantId.value)
  const subs = subsOfGroup(allSubs, groupKey)
  // 业主（默认分组）视角**不存在**分摊子项：`tenantItems` 是「上一家租户」的残留
  // （activeTenantId 不会因为切回业主而被清空），拿它去补列会在重庆星达表头右侧
  // 长出 7 列空值 + 孤立的 +/−（真机截图）。这里直接按视角切断。
  const ownerView = isOwnerView.value
  const itemByKey = ownerView
    ? new Map<string, any>()
    : new Map(tenantItemsUnique.value.map((it: any) => [tenantItemColKey(it), it]))

  // 清单设置里新开启、字段定义还没保存过的子项：补在「合计应付」之前。
  // 必须走同一套 collapseFeeGroups —— 否则它们会单独出一列，而字段配置把
  // 它们收进了大类，两端列数就对不上（六同步违规）。
  const known = new Set(subs.map((s: any) => String(s?.name || '').trim()))
  const extra: any[] = []
  for (const it of ownerView ? [] : tenantItemsUnique.value) {
    const key = tenantItemColKey(it)
    if (!key || known.has(key)) continue
    extra.push({
      name: key,
      label: String(it.item_name || it.itemName || it.source_key || '分摊项'),
      enabled: true,
      is_default: true,
      data_type: 'number',
      width: Number(it.width) || 0,
      group: groupKey,
    })
  }
  let effective = subs as any[]
  if (extra.length) {
    // 新开的子项插在两条收口列之前，保证「电费合计 / 水费合计」始终在最后
    const at = subs.findIndex((s: any) => TENANT_TOTAL_FIELDS.includes(String(s?.name || '').trim()))
    effective = at < 0 ? [...subs, ...extra] : [...subs.slice(0, at), ...extra, ...subs.slice(at)]
  }

  const out: ColDef[] = []
  const seen = new Set<string>()

  for (const row of collapseFeeGroups(effective)) {
    // ---- 大类汇总列：值 = 成员之和，单元格带 +/− 展开明细 ----
    if (row.kind === 'group') {
      const sub: any = row.sub
      if (sub && sub.enabled === false) continue
      if (seen.has(row.name)) continue
      const members = row.members
        .map((m) => {
          const it = itemByKey.get(m.name)
          if (!it) return null
          const ms: any = m.sub
          const label = String(ms?.label || it.item_name || it.itemName || m.name).trim() || m.name
          return { key: m.name, label, itemKey: String(it.item_key || it.itemKey || '') }
        })
        .filter(Boolean) as Array<{ key: string; label: string; itemKey: string }>
      // 成员全被关掉 → 大类列也没有意义（不能留一列恒为 0 的空壳）
      if (!members.length) continue
      seen.add(row.name)
      const label = String(sub?.label || '').trim() || row.label
      out.push({
        key: row.name,
        label,
        default: sub ? sub.is_default === true : true,
        w: '1fr',
        width: Number(sub?.width) > 0 ? Math.round(Number(sub.width)) : undefined,
        tip: row.spec.tip,
        feeGroup: { name: row.name, label, category: row.spec.category, members },
      })
      continue
    }

    const sub: any = row.sub
    const key = row.name
    if (!sub || sub.enabled === false) continue
    if (seen.has(key)) continue
    const label = String(sub.label || '').trim()
    const width = Number(sub.width) > 0 ? Math.round(Number(sub.width)) : 0
    // 旧的合并列「合计应付」已废弃：即使 subs 里还残留也不出列（六同步铁律）。
    if (LEGACY_TOTAL_FIELDS.includes(key)) continue
    // 分摊子项列：由「清单设置 → 分摊子项」驱动，subs 只负责顺序/文案/宽度/显隐
    if (key.startsWith('item_')) {
      const it = itemByKey.get(key)
      if (!it) continue
      seen.add(key)
      out.push({
        key,
        label: label || String(it.item_name || it.itemName || it.source_key || '分摊项'),
        default: sub.is_default === true,
        w: '1fr',
        width: width || Number(it.width) || undefined,
        tip: it.source_category ? `来源：${it.source_category}` : '市电账单费用分项',
        item: it,
      })
      continue
    }
    const src = parseFieldSource(sub)
    const hit = src ? null : byKey.get(key)
    if (!src && !hit) continue
    seen.add(key)
    if (src) {
      out.push({
        key,
        label: label || key,
        // `is_default !== false`（缺省视为显示）：新增引用字段本就是用户主动
        // 检索添加的，让它默认出列；只有显式关掉「默认表头」才隐藏。
        default: sub.is_default !== false,
        w: '1fr',
        width: width || undefined,
        tip: `引用自 ${src.category}${src.groupLabel || src.group ? ` · ${src.groupLabel || src.group}` : ''}`,
        source: src,
      })
    } else {
      out.push({
        ...hit!,
        label: label || hit!.label,
        default: sub.is_default === true,
        width: width || hit!.width,
      })
    }
  }

  // 完全没有该分组的字段定义（首次进入 / 分类未配置）→ 回落静态列，避免空白表格。
  // 有定义但全被关掉时保持空列，这才是「用户的选择」。
  if (!subs.length) {
    if (isOwnerView.value) return [...base]
    // 租户视角回落：静态列（已含两条收口列）+ 分摊子项
    const items = tenantItemsUnique.value.map((it: any) => ({
      key: tenantItemColKey(it),
      label: String(it.item_name || it.itemName || it.source_key || '分摊项'),
      default: true,
      w: '1fr',
      width: Number(it.width) || undefined,
      tip: it.source_category ? `来源：${it.source_category}` : '市电账单费用分项',
      item: it,
    })) as ColDef[]
    if (!items.length) return [...base]
    return [...base, ...items]
  }
  if (isOwnerView.value) return out

  // 收口列缺失时补在最后（老数据没有这两条 subs）：
  // 必须补，否则「合计」信息会整片消失，用户只能自己拿计算器加。
  for (const key of TENANT_TOTAL_FIELDS) {
    if (seen.has(key)) continue
    const hit = baseColDefs.value.find((c) => c.key === key)
    if (!hit) continue
    out.push({ ...hit, default: true })
  }
  return out
})

/**
 * 拉取当前租户的「分摊子项」。
 *
 * 租户视角的列 = 锚点列 + 这里启用的项目，所以必须在切换使用单位后立即拉一次；
 * 否则列表会停在上一家的列（用户看到的字段与所选单位不匹配）。
 */
async function loadTenantItems() {
  const id = activeTenantId.value
  // 业主视角不需要分摊子项：切回重庆星达必须清空，否则上一家租户的
  // 子项会留在内存里被 activeColDefs 当成「新开字段」补到业主表头。
  if (!id || isOwnerView.value) { tenantItems.value = []; syncColumnKeys(); return }
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
  // 列集变了必须**重算行**：引用字段的值是 buildRows 写进 row 对象的
  // （不是渲染期现算），不重建就会出现「表头有了、单元格整列空白」，
  // 详情抽屉也会因为 `key in row` 判定失败而缺字段。
  if (dataReady.value) buildRows()
}
// 列可见性存档按**单位**隔离：每个租户有自己的分组字段集，
// 共用一份存档会让「切到 B 租户时 A 的列全部隐藏、B 的列全部复活」。
// activeTenantId 提前到这里声明：syncColumnKeys() 在 setup 阶段就会读存档。
const activeTenantId = ref('')
const colStorageKey = computed(() => `weknora-tenant-billing-cols-${
  isOwnerView.value ? 'owner' : `tenant-${activeTenantId.value || 'none'}`
}-v3`)
const visibleKeys = ref<string[]>([])
/**
 * 列可见性存档。
 *
 * v1（旧）= 纯数组，只记「当时可见的列」。这**信息量不够**：
 * 无法区分「用户主动取消了勾选」与「这一列是这一版新增的」，两者都会表现为
 * 「不在数组里」。于是必然有一头出错：
 *   · 只做补新 → 用户取消勾选的默认列下次加载自己复活；
 *   · 只做过滤 → 新增列永远不出现（上一轮真机问题）。
 *
 * v2 = `{ v: 2, known, visible }`
 *   · known   = 存档时**存在过**的列 → 用「不在 known 里」判定「这是新列」；
 *   · visible = 存档时**勾选**的列 → 不在其中且属于 known 的，就是用户关掉的。
 * 读 v1 数组时把 known 近似成那份可见数组：宁可让隐藏的默认列复活一次，
 * 也绝不吞掉用户新加的字段（后者无法自行恢复，前者点一下就好）。
 */
interface ColStore { v: 2; known: string[]; visible: string[] }
function readColStore(): ColStore | null {
  try {
    const raw = localStorage.getItem(colStorageKey.value)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      const arr = parsed.map(String)
      return { v: 2, known: arr, visible: arr }
    }
    if (parsed && typeof parsed === 'object') {
      const known = Array.isArray((parsed as any).known) ? (parsed as any).known.map(String) : []
      const visible = Array.isArray((parsed as any).visible) ? (parsed as any).visible.map(String) : []
      return { v: 2, known, visible }
    }
  } catch { /* ignore */ }
  return null
}
/**
 * 读列可见性。
 *
 * 口径：
 *   ① 存档里勾选过的列 → 保留（但只保留**当前列集仍存在**的键，删掉的列不许复活）；
 *   ② 存档里从没出现过的列（新增字段）→ 按 `default` 自动勾上；
 *   ③ 存档里出现过、但没勾的列 → 保持隐藏（用户明确关掉的）。
 */
function loadStoredKeys(): string[] {
  const defs = activeColDefs.value
  const defaults = defs.filter(c => c.default).map(c => c.key)
  const store = readColStore()
  if (!store) return defaults
  const known = new Set(store.known)
  const picked = new Set(store.visible)
  const valid = defs.filter(c => picked.has(c.key)).map(c => c.key)
  const fresh = defs.filter(c => !known.has(c.key) && c.default).map(c => c.key)
  const merged = [...valid, ...fresh.filter(k => !valid.includes(k))]
  // 空 = 用户把所有列都关了，或存档全是已删除的键：回落到默认列，
  // 否则表头会变成空白表格，用户只能清缓存自救。
  return merged.length ? merged : defaults
}
/** 写存档：同时记下「当前存在过的列」与「当前勾选的列」。 */
const persistColumns = () => {
  const defs = activeColDefs.value
  const store: ColStore = {
    v: 2,
    known: defs.map(c => c.key),
    visible: visibleKeys.value.filter(k => defs.some(c => c.key === k)),
  }
  try { localStorage.setItem(colStorageKey.value, JSON.stringify(store)) } catch { /* ignore */ }
}
const syncColumnKeys = () => {
  visibleKeys.value = loadStoredKeys()
  // 顺手把 known 记下来：否则「用户取消勾选的默认列」每次加载都会被当成新列复活。
  persistColumns()
}
syncColumnKeys()
watch(useUnit, syncColumnKeys)
const visibleColDefs = computed(() => activeColDefs.value.filter(c => visibleKeys.value.includes(c.key)))

/**
 * 列的数据类型（费用清单的列定义没有 fieldType，只能按键名推断）。
 *
 * 只影响「表尾汇总要不要相加」与金额格式，不影响单元格渲染：
 *   · 金额列：`*_fee / *_amount / total_*`（分摊子项、大类汇总、两条收口列都在内）；
 *   · 数量列：`*_kwh / *_usage`；
 *   · 其余（单价/比例/备注…）交给 listSummary 的不可累加判定兜底。
 */
/** 点位值/比率：绝不能相加（键名兜底之外，中文表头也兜一层）。 */
const NON_SUM_LABEL = /(单价|均价|平均电价|比例|倍率|功率因|起度|止度|示数|环比|同比|税率|编号|户号|表号)/
/** 电量类键：`solar_gen` / `solar_grid` 不带 `_kwh` 后缀，只按后缀判会漏（真机表尾显示 —）。 */
const QTY_KEYS = new Set([
  'solar_gen', 'solar_grid', 'total_kwh', 'ind_kwh', 'dorm_kwh', 'res_kwh',
  'gas_usage', 'water_ind_usage', 'water_dorm_usage', 'water_fire_usage',
])
const AMOUNT_KEYS = new Set([
  'total_fee', 'solar_amount', 'ind_fee', 'dorm_fee', 'res_fee', 'gas_fee',
  'water_ind_fee', 'water_dorm_fee', 'water_fire_fee',
  TENANT_ELEC_TOTAL_FIELD, TENANT_WATER_TOTAL_FIELD,
])

function colTypeOf(c: ColDef): string {
  const key = String(c.key || '')
  const label = String(c.label || '')
  // ① 点位值优先判死：哪怕键名像金额（如 ind_price 含 _price）也不加
  if (NON_SUM_LABEL.test(label) || /_price$|^ratio$|^avg_price$/.test(key)) return 'text'
  // ② 大类汇总列与分摊子项列都是**金额**：子项 key 形如
  // `item_(2)上网环节线损费用|上网环节线损费用`，不含 `_fee` 后缀，
  // 只靠后缀判定会让「上网环节线损费用 / 零售输配电费 / 系统运行费用」
  // 这些真正要合计的列在表尾显示 `—`（真机 2026-09 持睿列表）。
  if (c.feeGroup || key.startsWith('item_')) return 'amount'
  if (AMOUNT_KEYS.has(key) || /_fee$|_amount$|_total$|合计|电费|水费|气费|金额|税额|价税/.test(key + label)) return 'amount'
  // ③ 电量与用量（键白名单 + 后缀 + 中文兜底）
  if (QTY_KEYS.has(key) || /_kwh$|_usage$|_count$/.test(key) || /电量|用量|水量|气量|度数/.test(label)) return 'number'
  return 'text'
}
/** 表尾汇总的列口径（与当前可见列严格同序，保证逐列对齐）。 */
const summaryColumns = computed<SummaryColumn[]>(() =>
  visibleColDefs.value.map((c) => ({ key: c.key, label: c.label, fieldType: colTypeOf(c) })),
)
/** 表尾汇总行：数据随筛选/视角/勾选变化实时重算。 */
/**
 * 表尾统计范围：**勾选优先，否则全部可见行**（第 20 轮）。
 * 勾了 2 行还显示全量合计会让人误判总额；首列文案同步切成「已选 K 条」。
 */
const summaryScope = computed(() =>
  summaryScopeRows(displayRows.value, selectedKeys.value, (r: any) => String(r?.id ?? '')),
)
const footData = computed(() => {
  const { rows, selectedCount } = summaryScope.value
  return rows.length ? [buildFootRow(summaryColumns.value, rows, { selectedCount })] : []
})
const footSpan = computed(() => footSpanOf(summaryColumns.value))
/**
 * 列的最小宽度（px）。
 *
 * 两个约束取大：
 *   ① 表头文字必须完整显示 —— 14px 中文字按 15px/字 估，加左右 padding；
 *   ② 单元格内容不能被截断 —— 数字按 8px/字符 估（等宽数字），加 padding。
 * 大类汇总列再补 26px：单元格里除了金额还有 18px 的 +/− 徽标。
 *
 * 为什么要算而不是写死：字段名由用户在「字段定义」里随便改，
 * 写死宽度只会在下一次改名后再截断一次。
 */
function minColumnWidth(c: ColDef): number {
  const label = String(c.label || '')
  // 表头：t-size-s 约 14px，中文按 15px/字（含字距）估，左右 padding 各 16
  let need = label.length * 15 + 34
  const extra = c.feeGroup ? 26 : 0
  for (const r of displayRows.value) {
    const t = cellText(c.key, r)
    if (!t) continue
    need = Math.max(need, String(t).length * 8 + 26 + extra)
  }
  return Math.min(400, Math.max(96, Math.round(need)))
}

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
    const minW = minColumnWidth(c)
    cols.push({
      colKey: c.key,
      title: c.label,
      ellipsis: true,
      // 用户配的宽度可以更大，但**不能小于表头文字所需宽度**：
      // 「上网环节线损费用 / 政府性基金及附加 / 功率因素调整电费」在 100px 里
      // 会被 TDesign 截成「上网环节线...」，用户分不清列（真机截图）。
      width: Math.min(400, Math.max(cfg > 0 ? cfg : 0, minW)),
      // 数值单元格悬停显示「计算方式 / 数据来源」。
      // 只对算得出来的列挂提示：文本列（备注等）没有口径可讲，挂空提示反而碍事。
      cell: (h: any, { row }: any) => renderCell(h, c.key, row),
    })
  }
  return cols
})

/**
 * 单元格渲染：把口径说明挂到 `data-tip` 上，由**一个共享浮层**统一展示。
 *
 * 为什么不用「每个单元格一个 t-tooltip」：
 *   这张表有 20+ 个数值列 × 十几行，就是几百个 Tooltip 实例；实测 hover 事件
 *   互相抢占，同一列有时弹有时不弹（真机验证反复失败）。改成一个浮层 + 事件委托：
 *     · 列表里只多一个浮层节点，行数再多也不会掉帧；
 *     · 触发路径只有一条（容器的 mouseover），行为确定。
 * 说明文本来自 buildRows 当行算出的 `row.__tips`，与单元格数值同源。
 */
function renderCell(h: any, key: string, row: any) {
  const col = feeGroupColMap.value.get(key)
  const tip = row?.__tips?.[key]
  if (col) {
    // 大类汇总列：金额 + `+ / −` 展开徽标。
    // 第 18 轮起整行不再有 @row-click（不弹详情抽屉），但仍显式 stop：
    // 徽标是行内唯一可点区域，阻止冒泡可避免未来再加行级监听时「点展开连带触发」。
    const open = expandedGroupOf.value[String(row?.id)] === key
    return h('div', { class: 'fee-group-cell' }, [
      h('span', {
        class: ['row-cell-text', 'row-cell-text--has-tip', open ? 'is-open' : ''],
        'data-tip': tip || '',
      }, fmtMoney(row?.[key])),
      h('button', {
        // 与「电费核算」共用 .row-expand-toggle（styles/row-expand-toggle.css），
        // 圆形徽标：浅灰底 + 深色 +，展开后实心品牌绿底 + 白色 −。
        class: ['row-expand-toggle', open ? 'is-open' : ''],
        type: 'button',
        title: open ? `收起「${col.feeGroup!.label}」细项` : `展开「${col.feeGroup!.label}」细项`,
        onClick: (e: any) => {
          e?.stopPropagation?.()
          toggleFeeGroup(row, key)
        },
      }, open ? '−' : '+'),
    ])
  }
  const text = cellText(key, row)
  if (!tip) return h('span', { class: 'row-cell-text' }, text)
  return h('span', { class: 'row-cell-text row-cell-text--has-tip', 'data-tip': tip }, text)
}

/**
 * 大类明细的展开态（手风琴互斥）。
 *
 * TDesign 的 expandedRowKeys 只认 rowKey，所以「展开哪一行」与「展开哪一列」
 * 要分开记：rowKeys 决定行、expandedGroupOf[rowId] 决定列。
 * 互斥是硬要求（同一时间全表最多 1 个明细子表），否则一屏展开十几个子表。
 */
const expandedRowKeys = ref<Array<string | number>>([])
const expandedGroupOf = ref<Record<string, string>>({})
const feeGroupColMap = computed(() => {
  const m = new Map<string, ColDef>()
  for (const c of activeColDefs.value) if (c.feeGroup) m.set(c.key, c)
  return m
})

/**
 * 明细面板的宽度。
 *
 * 为什么必须显式给宽度：展开行是一个 colspan 全表的 <td>，宽度等于**整张表**
 * （20 列约 2000px，比视口宽）。用户是横向滚到右边才点开大类的，此时面板左半边
 * （细项名称列）就在屏幕外 —— 真机截图里「只有金额、没有名称」就是这么来的。
 * 面板用 position: sticky + left: 0 钉在可视区左侧，宽度取滚动容器的 clientWidth。
 */
const groupPanelWidth = ref(0)
function measureGroupPanel() {
  // 真正的横向滚动容器是 TDesign 的 `.t-table__content`（overflow-x:auto），
  // 不是外层的 .tenant-list-scroll（那一层 overflow 被共享样式收掉了，
  // 拿它量出来的宽度和可视区并不一致）。sticky 的参照系也是它。
  const el = document.querySelector('.t-table__content') as HTMLElement | null
  if (!el || el.clientWidth <= 0) return
  // 留 24px 余量：sticky 元素不能溢出其包含块（展开行 td = 整表宽），
  // 面板与容器等宽时，滚到最右会被 td 右缘顶回来几像素（真机 22px），
  // 细项名称列因此被裁掉一截。收窄 24px 后左缘恒定贴住可视区。
  groupPanelWidth.value = Math.max(360, el.clientWidth - 24)
}

function toggleFeeGroup(row: any, key: string) {
  const rid = String(row?.id ?? '')
  if (!rid) return
  measureGroupPanel()
  const same = expandedRowKeys.value.length === 1
    && String(expandedRowKeys.value[0]) === rid
    && expandedGroupOf.value[rid] === key
  if (same) {
    expandedRowKeys.value = []
    expandedGroupOf.value = {}
    return
  }
  expandedRowKeys.value = [rid]
  expandedGroupOf.value = { [rid]: key }
}

/** TDesign 的展开回调（默认图标已关闭，仅作兜底：保持单行单列互斥）。 */
function onExpandChange(keys: Array<string | number>) {
  const next = Array.isArray(keys) ? keys.slice(0, 1) : []
  expandedRowKeys.value = next
  const rid = next.length ? String(next[0]) : ''
  const prev = expandedGroupOf.value[rid]
  expandedGroupOf.value = rid && prev ? { [rid]: prev } : {}
}

/** 当前行正在展开的大类明细（模板用）。 */
function expandedGroupOfRow(row: any) {
  const rid = String(row?.id ?? '')
  const key = expandedGroupOf.value[rid]
  if (!key) return null
  return row?.__groups?.[key] || null
}
const selectedRowKeysArr = computed(() => Array.from(selectedKeys.value))
function onTableSelectChange(val: any) { selectedKeys.value = new Set(Array.isArray(val) ? val : []) }
const selectAllColumns = () => { visibleKeys.value = activeColDefs.value.map(c => c.key); persistColumns() }
const resetColumns = () => { visibleKeys.value = activeColDefs.value.filter(c => c.default).map(c => c.key); persistColumns() }

// ---- 租户与账单 ----
const loading = ref(false)
const tenants = ref<any[]>([])
// activeTenantId 在列存档（colStorageKey）之前已声明，此处不再重复定义
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
/**
 * 首批数据是否已装载完成。
 *
 * 用途：设置抽屉保存后 `fleet-categories-changed` 会触发 `loadCategorySubs`，
 * 此时若数据还没到，直接 buildRows 会把空行集写进列表（闪一下空表）。
 */
const dataReady = ref(false)

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
/**
 * 装载全部数据源。
 *
 * 顺序说明（性能关键）：
 *   1. `loadKb()` 必须先完成 —— 市电/光伏两份账单列表都依赖它解析出的 kbId，
 *      这是**真实的数据依赖**，不能并行；
 *   2. 其余 6 个数据源（已生成账单 / 电 / 气 / 水表档案 / 水读数）与它无关，
 *      全部塞进同一个 Promise.all 并发发出去。
 * 旧实现把 7 个请求依次 `await`，再叠加 `autoGenerateReady` 的逐月串行生成，
 * 切到「费用清单」要等数秒。
 */
const loadAllData = async () => {
  // 数据一重载，已缓存的打印清单就可能过期（分摊比例/子项金额都会变），
  // 直接整片清掉比逐条失效更安全。
  statementCache.clear()
  const tenantId = activeTenantId.value
  if (!tenantId) {
    await loadKb()
    records.value = []
    displayRows.value = []
    return
  }
  try {
    await loadKb()
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
    const results: any[] = await Promise.all(tasks)
    let cursor = 0
    const billRes = kbId.value ? results[cursor++] : null
    const solarRes = kbId.value ? results[cursor++] : null
    const recRes = results[cursor++]
    const elecRes = results[cursor++]
    const gasRes = results[cursor++]
    const wmRes = results[cursor++]
    const wrRes = results[cursor++]
    utilityBills.value = Array.isArray(billRes?.data || billRes?.list) ? (billRes.data || billRes.list) : []
    solarBills.value = Array.isArray(solarRes?.data || solarRes?.list) ? (solarRes.data || solarRes.list) : []
    records.value = recRes?.data || []
    elecRows.value = flattenMeterRecords(elecRes)
    gasRows.value = flattenMeterRecords(gasRes)
    waterMeters.value = Array.isArray(wmRes?.data) ? wmRes.data : (Array.isArray(wmRes) ? wmRes : [])
    waterReadings.value = flattenWaterRecords(wrRes)
    buildRows()
    dataReady.value = true
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
        // 分时表逐段抄见电量：分时电费要按「逐表逐时段基准电量 × 时段单价」算，
        // 只有整月 usage 是算不出来的（见 tenantBillingSources.allocateTimedKwh）
        period_usage: {
          deep: round2Num(((Number(it.deep_curr) || 0) - (Number(it.deep_prev) || 0)) * (Number(meter?.rate) > 0 ? Number(meter.rate) : 1)),
          peak: round2Num(((Number(it.peak_curr) || 0) - (Number(it.peak_prev) || 0)) * (Number(meter?.rate) > 0 ? Number(meter.rate) : 1)),
          flat: round2Num(((Number(it.flat_curr) || 0) - (Number(it.flat_prev) || 0)) * (Number(meter?.rate) > 0 ? Number(meter.rate) : 1)),
          valley: round2Num(((Number(it.valley_curr) || 0) - (Number(it.valley_prev) || 0)) * (Number(meter?.rate) > 0 ? Number(meter.rate) : 1)),
        },
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
/**
 * 居民（定比）电量与电费。
 *
 * `residential_readings` 的语义陷阱：**每一行的 `kwh` 都是同一个「本期电量」基数**，
 * `ratio` 是该行的比例（定比行 0.015、基础电费 0.5003、各类基金 0.001~0.02），
 * `bill_kwh` 才是这行的计费电量。旧实现把所有行的 `kwh` 相加 → 7 行 × 596168
 * 得到天文数字，再被 `indKwh = 总电量 − 居民电量` 一减，定比电量/工业电量全废
 * （真机表现：定比电量恒为 0、工业电量对不上）。
 *
 * 正确口径：**只取主行**（定比 / 第一行），用它的 `bill_kwh`（= 定比电量，如 8938）。
 */
const residentInfoOf = (bill: any) => {
  const item = bill?.item || bill || {}
  const rows: any[] = Array.isArray(item.residential_readings) ? item.residential_readings : []
  // 主行：优先 ratio≈0.015 的定比行；否则取第一行（没有 ratio 时视为唯一的定比行）
  const main = rows.find((r) => {
    const ratio = Number(r?.ratio) || 0
    return ratio > 0 && ratio < 0.05 && String(r?.project || '').includes('定比')
  }) || rows[0]
  /**
   * 第 43 轮：电量（定比）的唯一权威来源是**票面分项** `(5)目录电费` 的计费数量。
   *
   * `residential_readings[].bill_kwh` 只是派生缓存，历史管线在 17 个月里写坏了
   * 11 个（2026-07 = 147.32 = 9821×0.015 乘了两次、2026-06/2026-04/2025-10 = 0、
   * 2026-05 = 149.79 …），而票面 `(5)目录电费` 的 qty 全部正确（2026-10=9354 …
   * 2025-06=14363）。旧实现优先读 bill_kwh，于是把脏值显示到费用清单。
   *
   * 优先级：票面目录电费 qty 之和 → 主行 bill_kwh → kwh×ratio+adjust 反算。
   * 严禁把已算好的居民计费电量再乘一次 0.015。
   */
  const catalogKwh = (Array.isArray(item.fee_items) ? item.fee_items : [])
    .filter((f: any) => {
      const c = String(f?.category || '').trim()
      return c.includes('目录电费') && !c.includes('工商业')
    })
    .reduce((s: number, f: any) => s + Math.abs(Number(f?.qty) || 0), 0)
  let kwh = Math.round(catalogKwh * 100) / 100
  if (!kwh) kwh = Math.abs(Number(main?.bill_kwh) || 0)
  if (!kwh) {
    const base = Number(main?.kwh) || 0
    const ratio = Number(main?.ratio) || 0
    const adjust = Number(main?.adjust) || 0
    kwh = base > 0 && ratio > 0 ? Math.round((base * ratio + adjust) * 100) / 100 : 0
  }
  const gov = (Array.isArray(item.fee_items) ? item.fee_items : [])
    .filter((f: any) => String(f.category || '').includes('政府性基金') && Number(f.qty || 0) < 100000 && !String(f.name || '').includes('功率因数'))
    .reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0)
  const fee = Math.round((Number(item.residential_amount) || (Number(item.catalog_amount) || 0) + (Number(gov) || 0)) * 100) / 100
  return { kwh: Math.round(kwh * 100) / 100, fee }
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
const sourceCtxOf = (month: string, bill: any, solarArr: any[], rec?: any): SourceResolveContext => {
  // 分时电费的两项输入：该租户当月分时表的逐段抄见电量、当月差额电量 ΔQ
  const timedMeters = elecRows.value
    .filter((r: any) => r.month === month && r.meter_type === 'time' && String(r.use_unit || '').trim() === useUnit.value)
    .map((r: any) => ({ name: String(r.meter_alias || ''), usage: r.period_usage || {} }))
  // ΔQ 优先取已生成账单的固话值；未生成时用「星达工业分表合计 − 租户分时表合计」现算，
  // 否则那些月份的差额分摊会凭空少掉（分时电费整体偏低）。
  let deltaQ = Number(rec?.line_loss) || 0
  // 星达工业分表总电量：已生成账单取固化值，否则现算（与 ratioOfMonth 同口径）
  let starTotal = Number(rec?.total_kwh) || 0
  const starUsage = starTotal > 0 ? starTotal : elecRows.value
    .filter((r: any) => r.month === month && r.use_unit === OWNER_UNIT && r.meter_type !== 'time' && r.meter_kind !== 'dorm')
    .reduce((s: number, r: any) => s + (Number(r.usage) || 0), 0)
  if (starTotal <= 0) starTotal = starUsage
  if (!rec) {
    const subUsage = timedMeters.reduce(
      (s: number, m: any) => s + Object.values(m.usage || {}).reduce((a: number, b: any) => a + (Number(b) || 0), 0),
      0,
    )
    if (starUsage > 0 && subUsage > 0) deltaQ = Math.round((starUsage - subUsage) * 100) / 100
  }
  // 分摊比例：已生成账单取固化值（6 位），否则用「工业总电量 ÷ 账单总电量」现算。
  // 账单电量明细的损耗/加减电量是全厂口径，分时折算前要乘它。
  const billKwh = Number(bill?.item?.total_kwh) || 0
  let monthRatio = Number(rec?.ratio) || 0
  if (monthRatio <= 0 && billKwh > 0 && starTotal > 0) {
    monthRatio = Math.round((starTotal / billKwh) * 1e8) / 1e8
  }
  return {
    month,
    tenantName: useUnit.value,
    bill,
    solarItems: solarArr,
    elecRows: elecRows.value,
    waterMeters: waterMeters.value,
    waterReadingMap: waterReadingMap.value,
    gasRows: gasRows.value,
    timedMeters,
    deltaQ,
    // 平段按度计费项的计费数量基准（工业总电量）
    starTotal: Math.round(starTotal * 100) / 100,
    ratio: monthRatio,
  }
}

/**
 * 当月分摊比例 = 工业电量 ÷ 市电账单电量。
 *
 * **优先现算，不用 rec.ratio**：`billing_records.ratio` 是 numeric(18,6)，
 * 8 位精度落库即被截断（0.21235625 → 0.212356）。这个比例要乘 44,000
 * 这样的基数，6 位会让基本电费差 1 分（9,343.66 vs 标准 9,343.67）。
 * 只有拿不到两端电量时才退回固化值。
 */
const ratioOfMonth = (month: string, bill: any, rec: any): number => {
  const billKwh = Number(bill?.item?.total_kwh) || 0
  const star = Number(rec?.total_kwh) || elecRows.value
    .filter((r) => r.month === month && r.meter_type === 'time' && r.use_unit === OWNER_UNIT)
    .reduce((sum, r) => sum + (Number(r.usage) || 0), 0)
  if (billKwh > 0 && star > 0) return Math.round((star / billKwh) * 1e8) / 1e8
  return Number(rec?.ratio) || 0
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
    /**
     * 当月气表记录 —— **必须按使用单位过滤**。
     *
     * 旧实现只按月 + `meter_kind !== 'public'` 过滤，等于把全厂所有单位的气费
     * 加在一起：2026-09 持睿汽车的费用清单里凭空多出一行「气费 1,188.20」，
     * 而那是重庆星达的气表读数，PDF 应收合计因此比列表「合计应付」多 1,188.20。
     * 气费与电/水不同，不是按比例分摊的，只能「谁的表算谁的钱」。
     */
    const gasOf = (unit: string) => gasRows.value.filter((r: any) =>
      r.month === month && r.meter_kind !== 'public' && String(r.use_unit || '').trim() === unit)
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
      // 业主视角 = 星达自己的气表（气费不参与分摊，别的单位的表不能算进来）
      const gas = gasOf(OWNER_UNIT)
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
      // 跨分类引用字段（费用清单「字段定义」里检索添加的列）+ 悬浮说明
      const ctx = sourceCtxOf(month, bill, solarArr)
      const extraTips: Record<string, string> = {}
      for (const c of activeColDefs.value) {
        // 大类汇总列带 source（mode=fee_group），但它不是「引用字段」——
        // 交给上面的汇总逻辑，绝不能再用 resolveSourceValue 覆盖一遍。
        if (!c.source || c.feeGroup) continue
        const v = resolveSourceValue(c.source, ctx)
        base[c.key] = v
        extraTips[c.key] = sourceFieldTip({
          category: c.source.category,
          groupLabel: c.source.groupLabel || c.source.group,
          // 原名优先，缺失时回退到**当前列名**（存量数据没有 source.label，
          // 直接显示 `total_kwh` 这种英文键对用户毫无意义）
          fieldLabel: c.source.label || c.label || c.source.field,
          month,
          value: v,
          // 核算类是多表汇总，条数写进提示才说得清「这个数是怎么来的」
          rowCount: c.source.category === '电费核算'
            ? elecRows.value.filter((r: any) => r.month === month && r.use_unit === useUnit.value).length
            : c.source.category === '气费核算'
              ? gasRows.value.filter((r: any) => r.month === month && String(r.use_unit || '').trim() === useUnit.value).length
              : undefined,
        })
      }
      base.__tips = buildRowTips({
        ownerView: true, useUnit: useUnit.value, tenantName, month,
        totalKwh, totalFee,
        resKwh: res.kwh, resFee: res.fee,
        dormKwh: dorm.kwh, dormFee: dorm.fee,
        recKwh: Number(rec?.total_kwh) || 0, recFee: Number(rec?.industrial_fee) || 0,
        ratio: ratioOfMonth(month, bill, rec),
        indKwh, indFee, indPrice,
        tenantDormKwh: tenantDorm.kwh, tenantDormFee: tenantDorm.fee,
        waterTotalUsage: totalMain.usage, waterTotalPrice: totalMain.price || 0,
        waterIndUsage: wIndUsage, waterIndFee: wIndFee,
        waterDormUsage: wDorm.usage, waterDormFee: wDorm.fee,
        waterFireUsage: wFire.usage, waterFireFee: wFire.fee,
        waterTenantDormUsage: tDorm.usage, waterTenantIndUsage: tInd.usage,
        gasUsage: sumBy(gas, 'usage'), gasFee: sumBy(gas, 'amount'), gasCount: gas.length,
      }, extraTips)
    } else {
      // ===== 持睿(租户)视图 =====
      const totalKwh = Number(bill?.item?.total_kwh) || 0
      const totalFee = Number(bill?.item?.grand_total ?? bill?.item?.total_amount) || 0
      const dorm = dormOf(tenantName)
      const wInd = tenantWater(tenantName, month, false)
      const wDorm = tenantWater(tenantName, month, true)
      const indKwh = Number(rec?.total_kwh) || 0
      const indFee = Number(rec?.industrial_fee) || 0
      // 租户视角 = 该租户自己的气表。没有气表的租户（持睿汽车）这里是空数组，
      // 「用气量 / 气费」两列恒为 0，打印清单也不会出现气费区块。
      const gas = gasOf(tenantName)
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
        // 气费：租户分组的派生字段里有「用气量 / 气费」，没有值就是一列空白
        gas_usage: sumBy(gas, 'usage'),
        gas_fee: sumBy(gas, 'amount'),
      })
      // 该租户的分摊子项：列与值同源（tenantEnabledItems），不可能出现「有列无数」。
      // 未开启「参与分摊」的子项照常出列，但金额记 0 —— 让它显式显示为「本项未计入」，
      // 而不是把列隐藏掉让人无从核对。
      const ctx = sourceCtxOf(month, bill, solarArr, rec)
      const ratio = ratioOfMonth(month, bill, rec)
      const itemTips: Record<string, string> = {}
      // 每个子项的分摊后金额 + 原账单金额（大类展开明细要按成员逐条列示）
      const itemValue = new Map<string, number>()
      const itemRaw = new Map<string, number>()
      // 分时项的四段明细（尖峰/峰/平/谷）：展开大类时要逐段列出来，
      // 只给一个合计用户没法核对「54,422.79 是怎么来的」。
      const itemPeriods = new Map<string, Array<{ period: string; qty: number; rate: number; fee: number }>>()
      // 第 40 轮：分时项的「逐表逐时段」分摊结果，展开明细的四段拆解列要靠它。
      const itemAlloc = new Map<string, TimedAllocation>()
      for (const it of tenantItemsUnique.value) {
        const participating = it.enabled !== false
        const key = String(it?.source_key || it?.item_name || '').trim()
        // 分时项（零售交易电费 / 零售输配电费…）：按「逐表逐时段基准电量 × 时段单价」算。
        // 旧实现是「账单整月金额 × 分摊比例」，把尖峰高价电与谷段低价电按同一比例摊，
        // 2026-09 持睿汽车因此算成 54,369.99（标准 54,422.79）。
        // 分组（item.category）参与匹配：工商业/居民政府基金同名，靠它隔离
        const grp = String(it?.category || '')
        const timed = participating && key ? resolveTimedItemFee(ctx, key, grp) : null
        const v = !participating ? 0 : timed ? timed.value : resolveTenantItemValue(it, ctx, ratio)
        const ckey = tenantItemColKey(it)
        base[ckey] = v
        itemValue.set(ckey, v)
        if (timed) {
          itemPeriods.set(ckey, timed.periods)
          itemAlloc.set(ckey, timed.allocation)
          itemTips[tenantItemColKey(it)] = tenantTimedTip({
            itemName: String(it.item_name || key),
            sourceCategory: it.source_category || '市电账单',
            deltaQ: timed.allocation.deltaQ,
            totalUsage: timed.allocation.totalUsage,
            totalBill: timed.allocation.totalBill,
            periods: timed.periods,
            value: v,
          })
          continue
        }
        // 浮窗里的「账单原始金额」必须真的去账单里取。
        //
        // 旧实现只把「固定金额分项」（基本电费/力调电费）的概况值传进来，
        // 其余分项一律 undefined → `Number(undefined) || 0` → 文案变成
        // 「= 0 × 21.24% = 4,434.02 元」（真机截图）：乘数是 0 却得出非零金额。
        // 现在统一走 billRawAmountOf（与列表取值同一个 resolveFeeRow）。
        const billAmount = participating && key ? billRawAmountOf(ctx, key, grp) : 0
        itemRaw.set(ckey, billAmount)
        // 平段按度项（非分时）的浮窗要写「工业总电量 × 单价」，
        // 不能写「账单金额 × 比例」—— 比例法对按度项是错的口径。
        const plainQty = participating && key && !(itemPeriods.get(ckey) || []).length
          ? Number(ctx.starTotal) || 0
          : 0
        const plainRate = plainQty ? Number(matchBillFeeItems(ctx, key, grp)[0]?.rate) || 0 : 0
        itemTips[tenantItemColKey(it)] = tenantItemTip({
          itemName: String(it.item_name || key),
          sourceCategory: it.source_category || '市电账单',
          billAmount,
          ratio,
          value: v,
          participating,
          directSum: String(it.value_mode || '').trim() === 'direct_sum',
          qty: plainQty,
          unitRate: plainRate,
        })
      }
      // 大类汇总列：值 = 成员分摊后金额之和；同时备好展开明细（与单元格同源）。
      const groupDetails: Record<string, any> = {}
      for (const c of activeColDefs.value) {
        if (!c.feeGroup) continue
        const rows: any[] = []
        let sum = 0
        // 第 40 轮：四段拆解的合计（电表/差额/线损/加减/基准电量）。
        // 同一个大类下可能有多个分时子项（零售交易 + 零售输配），它们共用
        // **同一套分时电量**，所以合计只取第一份 allocation，绝不逐项累加。
        let firstAlloc: TimedAllocation | null = null
        for (const m of c.feeGroup.members) {
          const v = Number(itemValue.get(m.key)) || 0
          sum += v
          const periods = itemPeriods.get(m.key) || []
          const alloc = itemAlloc.get(m.key) || null
          if (periods.length) {
            if (!firstAlloc && alloc) firstAlloc = alloc
            // 分时项：尖峰 / 峰 / 平 / 谷 各一行（电量 × 单价 = 金额）。
            // 旧实现只给一行「分时：四段基准电量 × 时段单价」+ 合计，
            // 用户看不到每一段的电量与单价，等于没明细。
            for (const pd of periods) {
              const bd = timedBreakdownOf(alloc, pd.period)
              rows.push({
                key: `${m.key}#${pd.period}`,
                label: m.label,
                period: pd.period,
                qty: Math.round(pd.qty * 100) / 100,
                price: pd.rate,
                ratio: null,
                raw: null,
                // 四分量：电表电量 / 差额电量 / 线损 / 加减电量
                breakdown: bd,
                method: '基准电量 × 时段单价',
                value: Math.round(pd.fee * 100) / 100,
              })
            }
            continue
          }
          // 平段按度项：计费数量 = 工业总电量、单价 = 账单该项单价，
          // 与列表单元格、悬浮窗同一口径（不再写「账单原值 × 比例」）。
          // members 的 key 形如 `item_(4)系统运行费|煤电容量电费`：解析出大类与子项名
          const rawKey = String(m.key || '').replace(/^item_/, '')
          const barAt = rawKey.indexOf('|')
          const plainGroup = barAt > 0 ? rawKey.slice(0, barAt) : ''
          const plainKey = barAt > 0 ? rawKey.slice(barAt + 1) : rawKey
          // 「零售损益分摊电费」按 **账单原值 × 分摊比例**（第 39 轮，与后端同源）：
          // 计费数量列显示账单原值（不是电量）、单价列显示分摊比例百分比，
          // 否则用户会看到「27.63%」被当成单价、金额却是 0（真机图 4）。
          const isRetailLoss = plainKey.includes('零售损益分摊')
          const lossRaw = isRetailLoss
            ? round2(matchBillFeeItems(ctx, plainKey, plainGroup)
              .reduce((acc: number, r: any) => acc + (Number(r?.fee) || Number(r?.fee_amount) || 0), 0))
            : 0
          // 系统运行费大类（第 40 轮）：**100% 强制平段按度**，含单价为 0 的
          // 「辅助服务费用」。旧实现 plainUnit=0 时 qty=null、method 掉进
          // 「账单原值 × 分摊比例」，展示成 27.63% / 0（真机图）。
          const isSystemRunning = plainGroup.includes('系统运行费')
          const plainUnit = isRetailLoss ? 0 : Number(matchBillFeeItems(ctx, plainKey, plainGroup)[0]?.rate) || 0
          const plainQty = isSystemRunning ? Number(ctx.starTotal) || 0 : (plainUnit ? Number(ctx.starTotal) || 0 : 0)
          rows.push({
            key: m.key,
            label: m.label,
            period: '—',
            qty: isRetailLoss ? null : (plainQty || null),
            price: isRetailLoss ? null : (isSystemRunning ? plainUnit : (plainUnit || null)),
            raw: isRetailLoss ? lossRaw : Math.round((Number(itemRaw.get(m.key)) || 0) * 100) / 100,
            ratio: isSystemRunning ? null : ratio,
            // 非分时行四列拆解显示「—」（没有时段口径）
            breakdown: null,
            // 计算方式严格区分两种口径，与悬浮窗一致
            method: isSystemRunning ? '工业总电量 × 单价'
              : (isRetailLoss ? '账单原值 × 分摊比例' : (plainUnit ? '工业总电量 × 单价' : '账单原值 × 分摊比例')),
            value: Math.round(v * 100) / 100,
          })
        }
        base[c.key] = Math.round(sum * 100) / 100
        itemTips[c.key] = [
          `「${c.feeGroup.label}」= 下属 ${c.feeGroup.members.length} 个细项之和（${c.feeGroup.category}）`,
          ...rows.map((r) => `  ${r.label}：${fmtMoney(r.value)}`),
          `合计 = ${fmtMoney(base[c.key])} 元`,
          '点单元格右侧 + 可展开明细（表头只留大类，细项配置在「设置 → 字段定义」）',
        ].join('\n')
        const bdTotals = firstAlloc ? timedBreakdownTotalsOf(firstAlloc) : null
        const starKwh = Number(ctx.starTotal) || 0
        const subKwh = bdTotals?.meterQty || 0
        // ΔQ 是名义差额（172,400 − 166,550 = 5,850）；四段分摊时逐表逐段
        // round2 会留下 ≤0.01 尾差（2026-10 合计 5,849.99）。这里把两个数字
        // 都写出来，避免用户看到「合计 5849.99」却以为算错；绝不反过去改
        // 计费电量（那会动到已锁定的分时电费金额）。
        const tail = bdTotals && Math.abs((starKwh - subKwh) - bdTotals.diffQty) >= 0.01
          ? `；四段分摊逐段取整，合计 ${fmtKwh(bdTotals.diffQty)}` : ''
        const deltaHint = bdTotals && starKwh > 0 && subKwh > 0
          ? `差额电量（星达分表 ${fmtKwh(starKwh)} - 持睿分表 ${fmtKwh(subKwh)} = ${fmtKwh(starKwh - subKwh)}${tail}）`
          : ''
        groupDetails[c.key] = {
          name: c.feeGroup.name,
          label: c.feeGroup.label,
          category: c.feeGroup.category,
          total: base[c.key],
          rows,
          breakdownTotals: bdTotals,
          breakdownHint: deltaHint,
        }
      }
      // 两条金额收口列（第 18 轮由「合计应付」拆出）：
      //   电费合计 = 分摊后工业电费 + 宿舍电费（宿舍电费已含定比居民电费）
      //   水费合计 = 工业水费 + 宿舍水费
      // 口径与「打印清单」的应收汇总、底部表尾汇总完全一致（三处同源）。
      const elecTotal = Math.round((indFee + (dorm.fee || 0)) * 100) / 100
      const waterTotal = Math.round(((wInd.fee || 0) + (wDorm.fee || 0)) * 100) / 100
      base[TENANT_ELEC_TOTAL_FIELD] = elecTotal
      base[TENANT_WATER_TOTAL_FIELD] = waterTotal
      itemTips[TENANT_ELEC_TOTAL_FIELD] = [
        // 浮窗算式按产品口径固定两位小数（930 要写成 930.00，
        // 与「电费合计 102,079.06 = 101,149.06 + 930.00」的读法一致）
        `计算方式：工业电费 (${money2(indFee)}) + 宿舍电费 (${money2(dorm.fee)}) = ${money2(elecTotal)} 元`,
        '工业电费为市电分项按当月分摊比例折算后的合计；宿舍电费按宿舍电表读数 × 单价。',
      ].join('\n')
      itemTips[TENANT_WATER_TOTAL_FIELD] = [
        `计算方式：工业水费 (${money2(wInd.fee)}) + 宿舍水费 (${money2(wDorm.fee)}) = ${money2(waterTotal)} 元`,
        '水费按水表读数 × 单价直接归属该租户，不参与市电分摊。',
      ].join('\n')
      // 跨分类引用字段：与业主视角同一口径（当月账单概况 / 核算汇总）。
      //
      // 这一步过去只写在业主分支里，于是即便列被拼出来，租户视角的单元格也是空的
      // —— 用户看到「有表头、没数据」，会误判成取值链路坏了。
      for (const c of activeColDefs.value) {
        if (!c.source || c.feeGroup) continue
        const v = resolveSourceValue(c.source, ctx)
        base[c.key] = v
        itemTips[c.key] = sourceFieldTip({
          category: c.source.category,
          groupLabel: c.source.groupLabel || c.source.group,
          fieldLabel: c.source.label || c.label || c.source.field,
          month,
          value: v,
          rowCount: c.source.category === '电费核算'
            ? elecRows.value.filter((r: any) => r.month === month && r.use_unit === useUnit.value).length
            : c.source.category === '气费核算'
              ? gasRows.value.filter((r: any) => r.month === month && String(r.use_unit || '').trim() === useUnit.value).length
              : undefined,
        })
      }
      base.__groups = groupDetails
      base.__tips = buildRowTips({
        ownerView: false, useUnit: useUnit.value, tenantName, month,
        totalKwh, totalFee,
        resKwh: 0, resFee: 0,
        dormKwh: dorm.kwh, dormFee: dorm.fee,
        recKwh: indKwh, recFee: indFee,
        ratio,
        indKwh, indFee, indPrice: indKwh > 0 ? Math.round(indFee / indKwh * 10000) / 10000 : 0,
        tenantDormKwh: dorm.kwh, tenantDormFee: dorm.fee,
        waterTotalUsage: 0, waterTotalPrice: 0,
        waterIndUsage: wInd.usage, waterIndFee: wInd.fee,
        waterDormUsage: wDorm.usage, waterDormFee: wDorm.fee,
        waterFireUsage: 0, waterFireFee: 0,
        waterTenantDormUsage: 0, waterTenantIndUsage: 0,
        gasUsage: 0, gasFee: 0, gasCount: 0,
      }, itemTips)
    }
    base.remark = rec?.remark || ''
    return base
  })
  let out = filters.value.month ? list.filter(r => r.month === filters.value.month) : list
  // 「搜索全部字段」：行内所有可读值拼串匹配（多关键词空格分词 AND）。
  const kw = String(filters.value.keyword || '').trim()
  if (kw) out = out.filter((r) => matchAllTokens(rowFlattenText(r), kw))
  displayRows.value = out
  // 切换使用单位 / 重算后行 id 会变：把已经不存在的展开态清掉，
  // 否则 TDesign 会记着一个不存在的 rowKey（表格显示为「没展开」但状态还在）。
  if (expandedRowKeys.value.length) {
    const aliveIds = new Set(out.map((r: any) => String(r.id)))
    if (!expandedRowKeys.value.every((k) => aliveIds.has(String(k)))) {
      expandedRowKeys.value = []
      expandedGroupOf.value = {}
    }
  }

  // 选中态校正：只剔除「已不在列表里」的 id，不整批清空。
  //
  // 旧实现无条件 clearSelection()。而 buildRows 会被 loadAllData / 视野切换 /
  // 保存后刷新反复调用 —— 用户勾选一条记录再点「生成账单」，只要中间有过一次
  // 数据重载，选中态就被清空，generateTargets 退化成「全部记录」，
  // 于是校验了所有月份并报「其它月份数据不全」（真机问题 8）。
  const alive = new Set(out.map((r: any) => r.id))
  const nextSelected = new Set([...selectedKeys.value].filter((id) => alive.has(id)))
  if (nextSelected.size !== selectedKeys.value.size) selectedKeys.value = nextSelected
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
// 底部摘要栏已下线（第 21 轮），条数由表尾合计行首列承载，不再单独维护 summary。
// 行点击 = **不再打开详情抽屉**（第 18 轮）。
//
// 费用清单的每一列都是核算结果，只读抽屉既不能改数、又容易被误当成「点行能编辑」；
// 真正的查看入口是「打印清单 / 打印目录」。行内仍保留：复选框勾选、
// 大类汇总列的 +/− 展开、单元格 hover 口径浮窗。
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 生成月度账单 ----
const generateBusy = ref(false)
/** 生成目标：勾选了就只处理所选，未勾选 = 全部（与确认文案一致）。 */
const generateTargets = computed(() => (selectedKeys.value.size ? selectedRows.value : displayRows.value))
const generateConfirmText = computed(() => (selectedKeys.value.size
  ? `确认重新生成所选 ${selectedKeys.value.size} 条记录的账单？`
  : '当前未选择记录，确认全部重新生成账单？'))

/**
 * 生成前完整性校验 —— **已弃用**（产品口径变更）。
 *
 * 旧实现逐月检查市电账单 / 星达分时读数 / 该租户电表读数 / 水气读数，任一缺失
 * 即整批禁止生成。真机反馈：2026-05 只缺「星达分时读数」就整批拒绝，
 * 而其余月份本来能算、用户也没有别的补数路径 —— 门禁比缺数本身更阻碍业务。
 *
 * 现在一律允许生成，缺项按 0 参与计算（后端 GenerateBillingRecord 同步放开），
 * 结果里缺失的部分显示 0，用户补完数据再点一次「重新核算」即可。
 */

/**
 * 「重新核算」主流程（第 43 轮重构）。
 *
 * 三条产品要求：
 *   1. 支持勾选单条 / 多条 / 不选则全量；
 *   2. **真正从上游源头重取**：走 /records/regenerate，后端每月重新读
 *      市电账单（目录电费计费数量 + 各分项电费）、光伏账单、电/水/气核算，
 *      再覆写记录 —— 而不是只读费用清单旧缓存自循环（旧实现修不回 147.32）；
 *   3. 业主视角（重庆星达）没有 billing_records 行，重算 = 重新拉取上游后
 *      由前端 buildRows 现算，因此只需 loadAllData。
 */
const handleGenerateAll = async () => {
  const targets = generateTargets.value
  if (!targets.length) {
    MessagePlugin.warning('当前没有可生成的记录')
    return
  }
  const picked = selectedKeys.value.size
  const months = Array.from(new Set(targets.map((r: any) => String(r.month || '')).filter((m) => m.length === 7)))

  if (isOwnerView.value || !activeTenantId.value) {
    // 业主视角：先真的从上游重取一次（跨租户按周期重算），再由前端 buildRows 现算
    generateBusy.value = true
    try {
      await regenerateBillingByPeriod(picked ? months : [])
      await loadAllData()
      MessagePlugin.success(picked
        ? `已重新核算选中的 ${picked} 条记录`
        : `已完成全部 ${displayRows.value.length} 条记录重新核算`)
    } catch (e: any) {
      MessagePlugin.error(e?.message || '重新核算失败')
    } finally {
      generateBusy.value = false
    }
    return
  }

  generateBusy.value = true
  try {
    // 勾选 → 只重算所选月份；未勾选 → 传空数组由后端取该租户全部月份
    const res: any = await regenerateBillingRecords(activeTenantId.value, picked ? months : [])
    const okMonths: string[] = res?.data?.regenerated || []
    const failed: any[] = res?.data?.failed || []
    if (okMonths.length) {
      MessagePlugin.success(picked
        ? `已重新核算选中的 ${picked} 条记录：${okMonths.join('、')}`
        : `已完成全部 ${okMonths.length} 条记录重新核算`)
    }
    if (failed.length) {
      MessagePlugin.warning(`以下月份重算失败：${failed.map((f: any) => `${f.month}（${f.error || '未知错误'}）`).join('；')}`)
    }
    await loadAllData()
    notifyEnergyDataChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新核算失败')
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
  // 并发度 4：逐月串行会让 16 个月排成一条长链（N 次 RTT 叠加），
  // 而全部并发又可能打满后端。分批并发既快又不冲击写接口。
  const CONCURRENCY = 4
  for (let i = 0; i < pending.length; i += CONCURRENCY) {
    await Promise.all(pending.slice(i, i + CONCURRENCY).map(async (row: any) => {
      try {
        await generateBillingRecord(activeTenantId.value, { month: row.month })
      } catch { /* 缺项由手动生成提示 */ }
    }))
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
// 金额 / 数量 / 单价 / 比例统一走 formatNumber（唯一出口，含 1e-6 浮点补偿）。
// 局部保留 `round2Num` 别名，避免改动 7 处调用点。
const round2Num = round2

/** 该行的分摊比例：已生成账单用固化值，否则按月内星达分时电量 ÷ 账单电量估算 */
function ratioForRow(row: any): number {
  const rec = records.value.find((r: any) => r.month === row.month)
  return ratioOfMonth(String(row.month || ''), billIndex.value.get(String(row.month || '')), rec)
}

/**
 * 打印清单的字节缓存。
 *
 * 为什么需要：`generateStatementPdf` 要把内嵌中文字体嵌进 PDF 并做子集化，
 * 单次 700~900ms（真机实测）。而用户的操作节奏是「勾一行 → 看两眼 → 点打印」，
 * 这份 PDF 与「哪些行 + 这些行的核算结果」一一对应，同一次会话里几乎不会变。
 * 缓存后第二次打开是**解析级别**的快（≈ viewer 自身耗时），也能让
 * 关闭弹窗后重新打开不至于又等小一秒。
 *
 * 失效策略：`loadAllData()` 里整片清空（任何数据刷新都会走到那里）；
 * 键里再带上行值与月份，避免「行对象换了但 id 没变」时分到旧字节。
 */
/**
 * 某月某单位的**工业分表抄见合计**（不含宿舍表 / 公共表）。
 *
 * 用途：「本期总电量」卡片要展示「工业 X + 差额 Y + 宿舍 Z」三段。
 * 其中 X 是真正抄到的分表度数，Y 是工业总电量与分表合计之间的差
 * （线路损耗 / 公摊等无法分表计量的部分）。
 *
 * 持睿汽车 2026-10：三个工业分表 89,400 + 57,150 + 20,000 = 166,550，
 * 工业总电量 172,400 → 差额 5,850；三段之和 166,550 + 5,850 + 686 = 173,086
 * 正好等于卡片大字值。
 */
const subMeterKwhOf = (month: string, unit: string): number => {
  let usage = 0
  for (const r of elecRows.value) {
    if (r.month !== month) continue
    if (String(r.use_unit || '').trim() !== unit) continue
    // 只算工业表：宿舍(dorm)与公共(public)不是工业分表
    if (['dorm', 'public'].includes(String(r.meter_kind || ''))) continue
    usage += Number(r.usage) || 0
  }
  return Math.round(usage * 100) / 100
}

/**
 * 打印清单的取数上下文。
 *
 * 构造器已外移到 `energyBillStatements.buildTenantBillingStatementDoc`（与市电/光伏
 * 清单同层），这里只把组件状态包成它需要的读写口 —— 版式与取数口径在那边统一，
 * 页面不再持有一份 400 行的打印实现。
 */
function statementCtx(unit: string, months: string[]): TenantStatementCtx {
  return {
    ownerView: isOwnerView.value,
    billOf: (month: string) => billIndex.value.get(month),
    recordOf: (month: string) => records.value.find((r: any) => r.month === month),
    items: tenantApportionItems.value,
    elecRows: elecRows.value,
    waterMeters: waterMeters.value,
    waterReadingMap: waterReadingMap.value,
    gasRows: gasRows.value,
    ratioOf: (row: any) => ratioForRow(row),
    // 工业分表抄见合计（不含宿舍/公共表）：用于「本期总电量」卡片的
    // 「工业 + 差额 + 宿舍」三段式。差额 = 工业总电量 − 该合计。
    subMeterKwhOf: (month: string, unitName: string) => subMeterKwhOf(month, unitName),
    // 气费是否适用：整份清单统一判定（同一单位、同一批月份），逐行判会让
    // 「多月份打印」里某几页有气费行、某几页没有。
    includeGas: gasApplicableOf(gasRows.value, activeColDefs.value.map((c) => c.key), unit, months),
    sourceCtxOf: (month: string, bill: any, solarItems: any[], rec: any) =>
      sourceCtxOf(month, bill, solarItems, rec),
  }
}

const buildStatementDoc = (rows: any[], unit: string) => {
  const months = rows.map((r: any) => r.month).filter(Boolean)
  return buildTenantBillingStatementDoc(statementCtx(unit, months), rows, unit)
}

const statementCache = new Map<string, Uint8Array>()
const statementKey = (rows: any[], unit: string) =>
  `${unit}|${rows.map((r) => [r.id, r.month, r.total_kwh, r.ind_fee, r[TENANT_ELEC_TOTAL_FIELD], r[TENANT_WATER_TOTAL_FIELD], r.dorm_fee, r.water_ind_fee, r.water_dorm_fee, r.gas_fee].join(':')).join(',')}`

/** 同一份内容只算一次；**在途**的那次也要复用，否则预生成与点击会各算一遍。 */
let statementInflight: { key: string; p: Promise<Uint8Array> } | null = null
async function statementBytesOf(rows: any[], unit: string, build: () => StatementDoc): Promise<Uint8Array> {
  const key = statementKey(rows, unit)
  const hit = statementCache.get(key)
  if (hit) return hit
  if (statementInflight?.key === key) return statementInflight.p
  const p = generateStatementPdf({ doc: build() })
    .then((bytes) => {
      statementCache.set(key, bytes)
      return bytes
    })
    .finally(() => {
      if (statementInflight?.key === key) statementInflight = null
    })
  statementInflight = { key, p }
  return p
}

/**
 * 打印清单（结算单）。
 *
 * 与「打印目录」的区别：目录 = 列表表头 + 行值（横向表格，内部对账）；
 * 清单 = 市电账单同款「概览 + 分项细项 + 水电气分区」纵向文档，可直接发租户。
 */
/**
 * 组装结算单文档模型。
 *
 * 抽出来是为了让「浮动栏打印清单」与「抽屉底栏打印」共用同一份内容 ——
 * 两条入口曾经各写一遍，字段顺序/概览口径改一处漏一处。
 */

/**
 * 勾选后**预生成**打印清单。
 *
 * 生成要嵌中文字体+子集化（r 真机 ~750ms），而用户「勾一行 → 点打印」之间
 * 通常有半秒以上的间隔。这里在勾选后立即后台生成，点击时直接命中缓存，
 * 体感从「转圈一秒多」变成「点开就有」。失败静默（真正点击时还会重试一次）。
 */
let statementPrewarm: ReturnType<typeof setTimeout> | null = null
watch(selectedKeys, () => {
  if (statementPrewarm) clearTimeout(statementPrewarm)
  if (!selectedKeys.value.size) return
  statementPrewarm = setTimeout(() => {
    statementPrewarm = null
    const arr = printableRows.value
    if (!arr.length) return
    const unit = (arr[0]?.unit as string) || useUnit.value
    void statementBytesOf(arr, unit, () => buildStatementDoc(arr, unit)).catch(() => {})
  }, 0)
})
onBeforeUnmount(() => { if (statementPrewarm) clearTimeout(statementPrewarm) })

const handlePrintStatement = async () => {
  const arr = printableRows.value
  if (!arr.length) return
  statementBusy.value = true
  try {
    const unit = (arr[0]?.unit as string) || useUnit.value
    const bytes = await statementBytesOf(arr, unit, () => buildStatementDoc(arr, unit))
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
  }
  // 子项必须重拉：在「清单设置 -> 分摊子项」里改的「启用字段 / 宽度(px) /
  // 参与分摊」都落在这份数据上，不重拉则保存后列表列集与列宽原地不动 ——
  // 用户看到的是「点了保存没反应」，而库里其实已经改了。
  await loadTenantItems()
  await loadAllData()
}
/**
 * 字段定义（`fleet_categories.subs`）变更后重载列集。
 *
 * 主链路是 `fleet-categories-changed` 事件（设置抽屉保存时派发，本页在
 * onMounted 里监听）；这里额外暴露一个方法，供父级在「抽屉关闭 / 保存成功」
 * 这类没有事件可依赖的时机显式调用，避免六同步任何一环靠猜测。
 */
const reloadSubs = async () => {
  await loadCategorySubs()
  buildRows()
}
defineExpose({ reloadTenants, reloadSubs })

// ---- 分时电表 CRUD(按归属单位分组 + 内联表单) ----
// ---- 打印 ----
// 详情抽屉已下线（第 18 轮）：行内不再有任何「打开详情」入口，
// 打印统一走浮动栏的「打印清单 / 打印目录」两个入口。
const printVisible = ref(false)
const printUrl = ref('')
const printBytes = ref<Uint8Array | ArrayBuffer | null>(null)
const printTitle = ref('')
const printFileName = ref('preview.pdf')

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

// ---- 格式化 ----
// 列表单元格口径（空值不占位）。金额与电量在这里显示规则一致：
// 整数原样、非整数两位，只是 fmtMoney 语义上更强调金额。
const fmtMoney = cellNum
const fmtKwh = cellNum
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
  const v = row[key]
  // 0 是有效值（如「参与分摊关闭」的分项、当月用量为 0），不能当空值吞掉
  if (v === 0) return '0'
  return v === null || v === undefined ? '' : String(v)
}
const fmtPrice = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(4)))
}

onMounted(() => {
  // 中文字体是打印链路的固定前置（清单与目录都要内嵌）：进页面就拉，
  // 别等用户点「打印」才开始下载（真机首次点击多花 200~400ms）。
  void loadCatalogFont().catch(() => {})
  loadTenants()
  void loadCategorySubs()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
  window.addEventListener('resize', measureGroupPanel)
  // 第 43 轮：上游数据源（市电/光伏账单、电/水/气核算）变更后自动重算并刷新，
  // 用户切回「费用清单」页签即可看到最新结果，不必再手点「重新核算」。
  window.addEventListener(ENERGY_DATA_CHANGED_EVENT, onUpstreamDataChanged)
})
/**
 * 字段定义保存后的联动。
 *
 * 事件 detail 里带着**刚保存的 subs**：保存接口已经把权威数据返回过了，
 * 再发一次 GET 分类只是白等一轮网络（真机：改名后表头要滞后半秒以上）。
 * 这里直接落内存、重建列与行 —— 纯前端计算，不碰任何账单/抄表接口。
 * 只有 detail 缺失（增删分类、从别的入口广播）时才回退到重新拉取。
 */
function onCategoriesChanged(ev?: Event) {
  const detail = (ev as CustomEvent | undefined)?.detail as { name?: string; subs?: any[] } | undefined
  const name = String(detail?.name || '').trim()
  if (Array.isArray(detail?.subs) && (!name || name === CATEGORY_NAME)) {
    categorySubs.value = detail!.subs as EnergySub[]
    syncColumnKeys()
    if (dataReady.value) buildRows()
    return
  }
  void loadCategorySubs()
}
/**
 * 上游数据源变更后的自动重算（第 43 轮）。
 *
 * 触发源：市电账单 / 光伏账单 的保存与删除、电/水/气核算记录的增删改、
 * 提取完成落库（它们各自 notifyEnergyDataChanged）。
 *
 * 只做「重新拉取 + 重建行」：费用清单的行是从上游数据**现算**出来的，
 * 拉一次最新上游就等于重算，不需要再逐月调 generateBillingRecord。
 * 正在重算/打印时跳过，避免与用户操作打架。
 */
let upstreamReloadTimer: ReturnType<typeof setTimeout> | null = null
function onUpstreamDataChanged() {
  if (generateBusy.value) return
  // 合并抖动：一次保存可能连发多个事件（账单 + 核算 + 分类），200ms 内只重载一次
  if (upstreamReloadTimer) clearTimeout(upstreamReloadTimer)
  upstreamReloadTimer = setTimeout(() => {
    upstreamReloadTimer = null
    void loadAllData()
  }, 200)
}

onBeforeUnmount(() => {
  if (upstreamReloadTimer) { clearTimeout(upstreamReloadTimer); upstreamReloadTimer = null }
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
  window.removeEventListener('resize', measureGroupPanel)
  window.removeEventListener(ENERGY_DATA_CHANGED_EVENT, onUpstreamDataChanged)
})

// 列表空态/加载态禁止横向滚动条（第 37 轮）：loading 或 0 行时挂 is-table-flat，
// 样式在 styles/business-list-shared.css 统一收口。
const { isTableFlat } = useTableFlatState({ loading, rows: displayRows })
const listViewClass = computed(() => flatClassOf(isTableFlat.value))
</script>

<style lang="less" scoped>
/* 中台列表共享样式：.doc-list-scroll / .doc-list-view / 表头遮罩 / 表格令牌。
   子组件是挂在 .biz-page 下的（父页面提供），但共享样式是按组件独立 import 的，
   不引入就会退化成 display:block + overflow:visible —— 表格不被限高、
   滚动条落在外层容器上，表现就是「滚动条侵入表头 + 汇总栏被顶到几千像素」。 */

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

/*
  行不再可点开抽屉（第 18 轮）→ 光标必须是默认箭头。
  仅保留两处可点击语义：复选框、大类汇总列的 +/− 展开徽标。
  不写 `:deep(.doc-table tbody tr)` 全表规则的话，浏览器会沿用
  TDesign 默认的默认光标，但历史样式里曾经加过 pointer，这里显式钉死防回归。
*/
.doc-table :deep(tbody tr) { cursor: default; }
.doc-table :deep(tbody tr .t-checkbox) { cursor: pointer; }
.doc-table :deep(tbody tr .row-expand-toggle) { cursor: pointer; }

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

<style lang="less">
/* 共享列表样式：必须在**非 scoped** 块里导入。
 * 放在 <style scoped> 里时 Vue 会给每条规则追加 [data-v-xxx]，
 * 而 .t-table__content / .t-table__body 是 TDesign 子组件内部元素、
 * 不带当前组件的 data-v 属性 → 选择器永不匹配（横向滚动条收不住）。
 * 发票页一直是对的（它就在非 scoped 块里），其余三页对齐它。 */
@import './styles/business-list-shared.css';
/* ---- 大类汇总列：单元格 +/− 与展开后的明细子表 ----
   这一块**必须非 scoped**：
   · 单元格内容由 renderCell 用 h() 生成，vnode 带的是 t-table 的 scopeId，
     父组件的 scoped 规则匹配不到（.row-cell-text 一直没样式就是这个原因）；
   · expanded-row 插槽内容由子组件渲染，同样不受父 scoped 约束。
   前缀 fee-group-/fgp- 全局唯一，不会污染其它页面。 */
.fee-group-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-width: 0;

  .row-cell-text {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: right;
  }
  .row-cell-text.is-open {
    color: var(--td-brand-color-7);
    font-weight: 600;
  }
}

/* 展开区：与「电费核算」的分时明细完全同一套视觉 ——
   浅灰表头行 + 明细行（行间 1px 分隔）+ 底部加粗合计行；
   没有标题横幅、没有卡片边框/阴影。
   宽度与 left:0 由 groupPanelWidth / sticky 保证：展开行是整表宽的 td，
   宽表横向滚动时若不钉左，细项名称会跑到屏幕外（真机截图「只有金额没有名称」）。 */
.t-table__row--expanded > td { padding: 0 !important; }

.fee-group-panel {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  background: var(--td-bg-color-container-hover);
  border-bottom: 1px solid var(--td-component-stroke);
  /* 10 列拆解表在窄视口下可横向滚动，绝不能把数字列裁掉 */
  overflow-x: auto;
  overflow-y: hidden;
}

/* 差额算式说明（第 40 轮）：贴在拆解表上方，让用户一眼看到
   「星达分表 − 持睿分表 = 差额」而不是自己去减。 */
.fg-breakdown-hint {
  padding: 6px var(--td-comp-paddingLR-s);
  font-size: 12px;
  line-height: 18px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-stroke);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fee-group-table {
  min-width: 980px;
  table-layout: fixed;
  border-collapse: collapse;

  .fg-cell {
    /* 与 TDesign t-size-s 单元格同 padding：文字左缘与主表同列严格对齐 */
    padding: 6px var(--td-comp-paddingLR-s);
    height: 18px;
    font-size: 12px;
    line-height: 18px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .fg-head-row .fg-cell {
    color: var(--td-text-color-placeholder);
    background: var(--td-bg-color-secondarycontainer);
    font-weight: 600;
  }
  .fg-num {
    font-family: var(--app-font-family);
    font-variant-numeric: tabular-nums;
    text-align: right;
  }
  tr + tr .fg-cell { border-top: 1px solid var(--td-component-stroke); }
  .fg-total-row .fg-cell {
    font-weight: 600;
    background: var(--td-bg-color-container);
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

