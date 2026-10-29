<template>
  <div class="utility-meter-tab">
    <!-- 筛选工具栏：与发票/合同/制度/通知、市电账单完全同一个 BusinessFilterBar
         （搜索全部字段 220px/32px、月份与下拉 32px、gap 8px、右侧 字段→设置→主按钮）。
         旧自写 BusinessListToolbar + 手写下拉已淘汰——那是搜索框占位文案、控件高度
         与发票不一致的根源。 -->
    <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
      :columns="activeColDefs" :visible-keys="visibleKeys" :selected-count="selectedRowKeysArr.length"
      settings-text="设置" hide-batch-bar :primary-action="{ text: '新增记录', icon: 'add' }"
      @columns-change="(v: string[]) => { visibleKeys = v; persistColumns() }"
      @reset-columns="resetColumns" @select-all-columns="selectAllColumns"
      @filter-change="load" @settings="openFieldSettings" @primary="openCreate"
      @clear-selection="clearSelection" />

    <!-- 列表（自绘 grid，数据少时只包裹记录行，超出滚动） -->
    <div class="doc-list-scroll meter-list-scroll" ref="listScrollRef">
      <div class="doc-list-view" :style="headerFixStyle">
        <t-table
        :data="displayRows"
        :columns="tableColumns"
        row-key="rowKey"
        size="small"
        :hover="true"
        :loading="loading"
        max-height="100%"
        sticky-header
        :expand-icon="false"
        class="doc-table"
        :selected-row-keys="selectedRowKeysArr"
        select-on-change
        :row-class-name="({ row }: any) => selectedKeys.has(row.key) ? 'row-selected' : ''"
        :expanded-row-keys="expandedRowKeysArr"
        @row-click="({ row }: any) => onRowClick(row)"
        @select-change="(val: any) => onTableSelectChange(val)"
        @expand-change="(ctx: any) => onExpandChange(ctx)"
      >
        <template #month="{ row }: any">
          <span class="row-mono">{{ row.month }}</span>
        </template>
        <template #meter="{ row }: any">
          <!-- 电表名称必须与普通电表严格左对齐：展开符放在名称**右侧**，
               不能像旧版那样用小三角占位（真机：持睿工业分表1/2/3 比宿舍312 右移 20px）。 -->
          <span class="row-text row-meter" :title="row.meter_alias">
            <span class="row-label">{{ row.meter_alias }}</span>
            <button v-if="isTimeRow(row)" type="button" class="row-expand-toggle"
              :class="{ 'is-open': expandedKey === row.key }" :title="expandedKey === row.key ? '收起分时明细' : '展开分时明细'"
              @click.stop="toggleExpand(row)">{{ expandedKey === row.key ? '−' : '+' }}</button>
          </span>
        </template>
        <template #start_reading="{ row }: any">
          <span class="row-mono">{{ fmtNum(row.start_reading) }}</span>
        </template>
        <template #end_reading="{ row }: any">
          <span class="row-mono">{{ fmtNum(row.end_reading) }}</span>
        </template>
        <template #rate="{ row }: any">
          <span class="row-mono">{{ fmtNum(row.rate) }}</span>
        </template>
        <template #usage="{ row }: any">
          <span class="row-mono">{{ fmtNum(row.usage) }}</span>
        </template>
        <template #unit_price="{ row }: any">
          <span class="row-mono" :class="{ 'row-dash': isTimeRow(row) }">{{ isTimeRow(row) ? '—' : fmtUnitPrice(row.unit_price) }}</span>
        </template>
        <template #garbage_fee="{ row }: any">
          <span class="row-mono">{{ fmtMoney(row.garbage_fee) }}</span>
        </template>
        <template #secondary_water_fee="{ row }: any">
          <span class="row-mono">{{ fmtMoney(row.secondary_water_fee) }}</span>
        </template>
        <template #sewage_fee="{ row }: any">
          <span class="row-mono">{{ fmtMoney(row.sewage_fee) }}</span>
        </template>
        <template #subsidy="{ row }: any">
          <span class="row-mono" :class="{ 'os-neg': Number(row.subsidy) < 0 }">{{ isTimeRow(row) ? '—' : fmtMoney(row.subsidy) }}</span>
        </template>
        <template #amount="{ row }: any">
          <span class="row-mono" :class="{ 'row-dash': isTimeRow(row) }">{{ isTimeRow(row) ? '—' : fmtMoney(row.amount) }}</span>
        </template>
        <template #reading_date="{ row }: any">
          <span class="row-mono">{{ row.reading_date || '' }}</span>
        </template>
        <template #reader="{ row }: any">
          <span class="row-text" :title="String(row.reader ?? '')">{{ row.reader || '' }}</span>
        </template>
        <template #meter_no="{ row }: any">
          <span class="row-mono" :title="String(row.meter_no ?? '')">{{ row.meter_no || '' }}</span>
        </template>
        <template #use_unit="{ row }: any">
          <span class="row-text" :title="String(row.use_unit ?? '')">{{ row.use_unit || '' }}</span>
        </template>
        <template #default_unit_price="{ row }: any">
          <span class="row-mono">{{ fmtUnitPrice(row.default_unit_price) }}</span>
        </template>
        <template #meter_mode="{ row }: any">
          <span class="row-text">{{ row.meter_mode === 'auto' ? '自动抄表' : row.meter_mode === 'manual' ? '手动抄表' : '' }}</span>
        </template>
        <template #install_date="{ row }: any">
          <span class="row-mono">{{ row.install_date || '' }}</span>
        </template>
        <template #remark="{ row }: any">
          <span class="row-text" :title="String(row.remark ?? '')">{{ row.remark }}</span>
        </template>
        <template #expandedRow="{ row }: any">
          <!-- 分时明细：与主表**同一套列宽**（colgroup 用实测表头宽度），
               时段⟷电表名称、起度⟷起度、止度⟷止度、倍率⟷倍率、时段电量⟷用电量。
               旧版是独立 grid 五等分，整块与主表错位（真机截图为证）。 -->
          <div v-if="isTimeRow(row) && expandCols.length" class="expand-inner expand-inner--aligned"
            :style="{ height: expandHeight + 'px', marginLeft: expandShift + 'px' }">
            <table class="expand-table" :style="{ width: expandWidth + 'px' }">
              <colgroup>
                <col v-for="(c, i) in expandCols" :key="i" :style="{ width: c.width + 'px' }" />
              </colgroup>
              <tbody>
                <tr class="expand-head-row">
                  <td v-for="(c, i) in expandCols" :key="i" class="expand-cell">{{ detailHeadOf(c.key) }}</td>
                </tr>
                <tr v-for="p in periodsOf(row)" :key="p.name">
                  <td v-for="(c, i) in expandCols" :key="i" class="expand-cell"
                    :class="{ 'expand-cell--num': detailNumericOf(c.key) }">{{ detailCellOf(c.key, p, row) }}</td>
                </tr>
                <tr class="expand-total-row">
                  <td v-for="(c, i) in expandCols" :key="i" class="expand-cell"
                    :class="{ 'expand-cell--num': detailNumericOf(c.key) }">{{ detailTotalOf(c.key, row) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
        <template #empty>
          <div v-if="!loading" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </template>
      </t-table>
    </div>
      <!-- 底部汇总：与发票管理一致，位于白色列表卡片（.doc-list-scroll）内部最底端；
           旧版挂在外层灰底上，视觉上「汇总条游离在表格外」（真机截图）。 -->
      <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
        <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
        <span class="doc-summary-item">总用量 <span class="doc-summary-val">{{ fmtNum(summaryUsage) }}</span> {{ unitLabel }}</span>
        <span class="doc-summary-item">总金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
      </div>
    </div>
            <!-- 分时表：行内展开四时段明细（参考电量明细布局） -->


    <!-- 底部浮动工具栏（选中行时显示；打印弹窗打开时隐藏） -->
    <transition name="batch-bar-fade">
      <div v-if="selectedKeys.size && !printVisible" class="doc-batch-bar-fixed" role="region">
        <div class="batch-bar-inner">
          <div class="batch-bar-left">
            <span class="batch-bar-count">已选 {{ selectedKeys.size }} 项</span>
            <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
              清除
            </t-button>
          </div>
          <div class="batch-bar-actions">
            <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="openEditSelected">
              <template #icon><t-icon name="edit" size="14px" /></template>
              编辑
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
            <t-popconfirm theme="warning"
              :content="`确定删除所选 ${selectedKeys.size} 条记录吗？`"
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

    <!-- 编辑/新增抽屉：与发票/合同同款右侧抽屉底座（SettingDrawer + t-form 双列），
         旧自写 <t-drawer> + 手写 .rec-field 布局已淘汰 → MeterRecordDrawer 组件。 -->
    <!-- 字段全部由当前分类的「字段定义（subs）」驱动：编辑与新增共用，天然 1:1。
         底部只留 取消 / 保存；删除入口在浮动批量栏（单选后「删除」），
         抽屉里再放一个「删除本行」既重复又与中台抽屉规范不符。 -->
    <MeterRecordDrawer :visible="drawerVisible" :category="props.category" :title="drawerTitle" :form="form"
      :subs="categorySubs" :meter="currentMeter" :editing-item-id="editingItemId" :meter-options="meterEditOptions"
      :use-unit-options="useUnits.map((u) => ({ label: u, value: u }))"
      :default-unit-price="currentMeter?.default_unit_price" :is-time-meter="isTimeMeter"
      :reading-invalid="readingInvalid" :unit-price-diff="unitPriceDiff" :duplicate-warning="duplicateWarning"
      :form-rate="formRate" :form-usage="formUsage" :form-amount="formAmount" :saving="saving" :today="today"
      @update:visible="(v: boolean) => (drawerVisible = v)" @close="onDrawerClose" @save="save"
      @meter-change="onMeterChange" @use-unit-change="onUseUnitChange" />

    <!-- 打印预览：全中台统一组件 -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="`${categoryLabel}目录（${printCount} 条）打印预览`"
      :url="printUrl" :data="printBytes" :download-name="`${categoryLabel}目录.pdf`" loading-text="正在生成目录…"
      @close="closePrint" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeterRecords,
  createUtilityMeterRecord,
  updateUtilityMeterRecord,
  deleteUtilityMeterRecord,
  listUtilityMeters,
  listUtilityKinds,
  createUtilityKind,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { useTableHeaderFix } from './useTableHeaderFix'
import { loadEnergyCategorySubs, applySubsToColumns, type EnergySub } from './energyViewColumns'
import BusinessFilterBar from './BusinessFilterBar.vue'
import MeterRecordDrawer from './MeterRecordDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import { colWidthOf } from './columnWidth'
import { rowFlattenText, matchAllTokens, notifyEnergyDataChanged } from './energyCounters'

const props = defineProps<{
  category: 'water' | 'gas' | 'electricity'
}>()

// 表头右侧滚动条轨道遮罩的实测值（与发票管理同一套机制）
const { headerFixStyle } = useTableHeaderFix()

// 「设置」→ 父级统一设置抽屉（父级常驻挂载，不随 6 个卡片视图的 v-if 卸载）
const emit = defineEmits<{ (e: 'open-settings'): void }>()

const categoryLabel = computed(() => (props.category === 'water' ? '水费' : props.category === 'electricity' ? '电费' : '气费'))
const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))
const usageLabel = computed(() => (props.category === 'water' ? '用水量' : props.category === 'electricity' ? '用电量' : '用气量'))

// ---- 列定义 ----
interface ColDef { key: string; label: string; default: boolean; w: string; tip?: string; only?: 'water'; width?: number }
const categoryCols = (defs: ColDef[]) => defs.filter(c => !c.only || c.only === props.category)
const COLUMN_DEFS: ColDef[] = [
  { key: 'month', label: '月份', default: true, w: '1fr', tip: '抄表所属月份' },
  { key: 'meter', label: meterLabel.value, default: true, w: '1.2fr', tip: '表计别名' },
  { key: 'start_reading', label: '起度', default: true, w: '0.9fr', tip: '上期止度自动带入' },
  { key: 'end_reading', label: '止度', default: true, w: '0.9fr', tip: '本期抄表读数(须 ≥ 起度)' },
  { key: 'rate', label: '倍率', default: props.category !== 'gas', w: '0.7fr', tip: '表计配置倍率(不可修改)' },
  { key: 'usage', label: usageLabel.value, default: true, w: '1fr', tip: `(止度 − 起度) × 倍率` },
  { key: 'unit_price', label: '单价', default: true, w: '1.2fr', tip: '默认取表计配置单价(最多三位小数)' },
  // 水费附加费用（仅水费；电费/气费无此列）
  { key: 'garbage_fee', label: '垃圾处置费', default: false, w: '0.9fr', tip: '水费附加,新增默认 13 元/套', only: 'water' },
  { key: 'secondary_water_fee', label: '二次供水费', default: false, w: '0.9fr', tip: '水费附加,手工填写', only: 'water' },
  { key: 'sewage_fee', label: '污水处理费', default: false, w: '0.9fr', tip: '水费附加,手工填写', only: 'water' },
  { key: 'subsidy', label: '补差', default: false, w: '0.8fr', tip: '手工填写,可为正负数' },
  { key: 'amount', label: categoryLabel.value, default: true, w: '1fr', tip: `${usageLabel.value} × 单价 + 附加费 + 补差` },
  { key: 'remark', label: '备注', default: false, w: '2fr', tip: '手工填写' },
  // 默认显示：抄表信息
  { key: 'reading_date', label: '抄表日期', default: true, w: '1fr', tip: '实际抄表日期' },
  { key: 'reader', label: '抄表人', default: true, w: '1fr', tip: '默认取表计配置管理人员' },
  { key: 'use_unit', label: '使用单位', default: true, w: '1.2fr', tip: '表计使用单位' },
  { key: 'meter_mode', label: '抄表方式', default: true, w: '0.9fr', tip: '自动抄表/手动抄表' },
  // 详细字段：表计档案参数
  { key: 'meter_no', label: '表号', default: false, w: '1fr', tip: '表计编号' },
  { key: 'default_unit_price', label: '默认单价', default: false, w: '0.9fr', tip: '表计配置单价' },
  { key: 'install_date', label: '安装日期', default: false, w: '1.1fr', tip: '表计安装日期' },
]
// 分类名与设置抽屉里的 fleet_categories.name 必须逐字一致
const CATEGORY_NAME: Record<string, string> = {
  electricity: '电费核算',
  water: '水费核算',
  gas: '气费核算',
}
const categorySubs = ref<EnergySub[]>([])
/**
 * 生效列定义 = 字段定义（subs）驱动的结果。
 *
 * subs 是唯一权威源：改名 / 改宽度 / 拖顺序 / 显隐都在设置抽屉里改，列表立刻跟着变。
 * 静态 COLUMN_DEFS 只负责提供渲染器与缺省宽度（subs 缺失或还没加载时兜底）。
 */
const activeColDefs = computed<ColDef[]>(() =>
  applySubsToColumns(
    categoryCols(COLUMN_DEFS),
    categorySubs.value,
    (key, sub) => ({
      key,
      label: String(sub.label || key),
      default: sub.is_default === true,
      w: '1fr',
      tip: `自定义字段：${key}`,
    }),
  ),
)
// 列显隐存档键：带上分类，避免电/水/气三张表互相覆盖
const STORAGE_KEY = computed(() => `weknora-utility-meter-${props.category}-columns-v4`)
const visibleKeys = ref<string[]>(loadStoredKeys())
const visibleColDefs = computed(() => activeColDefs.value.filter(c => visibleKeys.value.includes(c.key)))
const tableColumns = computed(() => {
  // 与发票管理一致：[复选框] → [序号] → 业务列。
  // colKey 必须写 'row-select'：只写 type:'multiple' 时 TDesign 照样渲染表头，
  // 但 tbody 单元格是空的（列标识对不上），看着像「复选框没渲染」。
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 56, align: 'center' },
  ]
  for (const c of visibleColDefs.value) {
    // subs.width>0 精确固定（clamp 60~400 防误输入）；0/未配置自适应。
    cols.push({ colKey: c.key, title: c.label, ellipsis: true, width: colWidthOf(c.width, c.label, []) })
  }
  return cols
})

/** 打开中台设置抽屉（字段定义 / 提取规则），定位到本分类。 */
function openFieldSettings() { emit('open-settings') }
// 表计设置在统一抽屉里增删改后，记录表单的「表计」下拉必须重新拉一次
const reloadMeters = async () => { await load() }
defineExpose({ reloadMeters })

/** 落到本地 subs 并校正可见列（保存事件与首次加载共用同一段收尾）。 */
function applySubs(subs: EnergySub[]) {
  categorySubs.value = subs
  // 订阅结束后若存档里的键在新列集里一个都不剩（例如用户删光了旧列），
  // 回退到「默认表头」，避免出现"设置里明明有字段、列表一列都不显示"。
  if (visibleKeys.value.length && !visibleKeys.value.some(k => activeColDefs.value.some(c => c.key === k))) {
    resetColumns()
  }
}

/** 拉取本分类的字段定义（事件里没带 subs 时才走这条，含一次网络往返）。 */
async function loadCategorySubs() {
  try {
    applySubs(await loadEnergyCategorySubs(CATEGORY_NAME[props.category] || ''))
  } catch {
    applySubs([])
  }
}
const selectedRowKeysArr = computed(() => Array.from(selectedKeys.value))
// 分时明细手风琴：只允许一行展开（旧版 Set 可多行同时展开，真机截图 51/52 同时打开）
const expandedKey = ref<string | null>(null)
const expandedRowKeysArr = computed(() => (expandedKey.value ? [expandedKey.value] : []))
function onTableSelectChange(val: any) { selectedKeys.value = new Set(Array.isArray(val) ? val : []) }
/** 名称右侧的 +/− 徽标：展开/收起该行的分时明细（替代 TDesign 自带展开列，
 *  那列会插在复选框左边，破坏「复选框 + 序号」的标准列序）。 */
function toggleExpand(row: any) {
  expandedKey.value = expandedKey.value === row.key ? null : row.key
  scheduleMeasure()
}

function onExpandChange(ctx: any) {
  const keys = Array.isArray(ctx?.expandedRowKeys) ? ctx.expandedRowKeys : []
  expandedKey.value = keys.length ? String(keys[0]) : null
  scheduleMeasure()
}

/**
 * 延迟一帧再测列宽/偏移。
 *
 * TDesign 的展开行不是同步插入 DOM 的（expandedRowKeys 走到内部控制态才渲染），
 * 单个 nextTick 里 querySelector('.expand-table') 还是 null，测出来的 shift 恒为 0
 * ——真机表现就是「子表整体右移 16px」。这里用 nextTick + rAF + 80ms 兜底三连测。
 */
function scheduleMeasure() {
  void nextTick(measureTableColumns)
  // 展开行由 TDesign 异步插入，补两次定时测量（不要挂在 rAF 上：
  // headless / 后台标签会节流 rAF，兜底链直接断掉，shift 永远停在 0）
  window.setTimeout(measureTableColumns, 60)
  window.setTimeout(measureTableColumns, 220)
}

// ---- 分时明细列对齐：实测主表表头宽度，子表用同一套 colgroup ----
//
// 为什么不能按 colWidthOf 声明的宽度直接算：TDesign 的 table-layout:fixed 在
// 「声明宽度之和 < 容器宽度」时会把富余宽度按比例摊到各列，真实左边界与声明值
// 并不相等。只有实测 <th> 的 boundingClientRect 才能做到像素级对齐。
const expandCols = ref<Array<{ key: string; width: number }>>([])
/**
 * 展开子表的显式宽度。
 *
 * 展开行的 `<td>` 内容盒比主表窄一道单元格 padding（16px），由 CSS
 * `margin-left: calc(-1 * var(--td-comp-paddingLR-s))` 把容器拉回主表左边界；
 * 宽度取实测列宽之和，列左缘由 colgroup 保证与主表逐列相等。
 */
const expandWidth = computed(() => Math.round(expandCols.value.reduce((s, c) => s + c.width, 0) * 100) / 100)
/** 展开行的容器相对主表左缘的内缩量（负值 = 需要左移多少才能对齐） */
const expandShift = ref(0)
/** 展开块高度 = (表头 + 四时段 + 合计) × 单行 30px；子表绝对定位，必须显式给高 */
const EXPAND_ROW_H = 30
// 行间的 1px 分隔线也算进高度（6 行 = 5 条线），否则最后一行的合计会被裁掉半行
const expandHeight = EXPAND_ROW_H * 6 + 5
function measureTableColumns() {
  const wrap = listScrollRef.value
  const cols = tableColumns.value
  if (!wrap || !cols.length) return
  const ths = Array.from(wrap.querySelectorAll('thead th')) as HTMLElement[]
  if (!ths.length) return
  const next = ths.slice(0, cols.length).map((th, i) => ({
    key: String((cols[i] as any)?.colKey || ''),
    width: Math.round(th.getBoundingClientRect().width * 100) / 100,
  }))
  if (!next.length) return
  const same = next.length === expandCols.value.length &&
    next.every((c, i) => c.key === expandCols.value[i].key && Math.abs(c.width - expandCols.value[i].width) < 0.5)
  if (!same) expandCols.value = next

  // 残余内缩：主表左缘 − 展开容器左缘。容器当前已带 expandShift，
  // 因此 dx 就是"还差多少"，累加即一步收敛（不会来回抖）。
  const hostBox = wrap.querySelector('.expand-inner--aligned') as HTMLElement | null
  const mainTable = wrap.querySelector('table') as HTMLElement | null
  if (hostBox && mainTable) {
    const dx = mainTable.getBoundingClientRect().left - hostBox.getBoundingClientRect().left
    if (Math.abs(dx) > 0.5) expandShift.value = Math.round((expandShift.value + dx) * 100) / 100
  }
}
/** 时段列对齐「电表名称」列；其余四列按 key 落到主表同名列 */
const DETAIL_HEAD: Record<string, string> = {
  meter: '时段', start_reading: '起度', end_reading: '止度', rate: '倍率', usage: '时段电量',
}
const detailHeadOf = (key: string) => DETAIL_HEAD[key] || ''
const detailNumericOf = (key: string) => key === 'start_reading' || key === 'end_reading' || key === 'rate' || key === 'usage'
/** 单元格取值：时段名 / 起度 / 止度 / 倍率 / 时段电量（纯数值，不带单位） */
function detailCellOf(key: string, p: any, row: any): string {
  switch (key) {
    case 'meter': return p.name
    case 'start_reading': return fmtNum(p.prev)
    case 'end_reading': return fmtNum(p.curr)
    case 'rate': return fmtNum(row.rate)
    // 与主表「用电量」同口径：只显示数值（旧版带「千瓦时」，与主表不一致）
    case 'usage': return fmtNum(p.usage)
    default: return ''
  }
}
function detailTotalOf(key: string, row: any): string {
  if (key === 'meter') return '合计'
  if (key === 'usage') return fmtNum(row.usage)
  return ''
}

/**
 * 读取列显隐存档。
 *
 * 存档里的 key 必须仍存在于 `activeColDefs`（= subs 派生结果）：
 * 字段在「字段配置」里被删掉后，旧存档 key 属于幽灵，原样返回会让
 * 「重置」基于过期集合 —— 这里直接剔除并回写，保证重置只落到现有字段。
 */
function loadStoredKeys(): string[] {
  const defaults = activeColDefs.value.filter(c => c.default).map(c => c.key)
  try {
    const raw = localStorage.getItem(STORAGE_KEY.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        // 仅保留本组件认识的字段，避免跨页面污染
        const valid = arr.filter(k => activeColDefs.value.some(c => c.key === k))
        if (valid.length) {
          if (valid.length !== arr.length) {
            try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(valid)) } catch { /* ignore */ }
          }
          return valid
        }
        return defaults
      }
    }
  } catch { /* ignore */ }
  return defaults
}

const selectAllColumns = () => {
  visibleKeys.value = activeColDefs.value.map(c => c.key)
  persistColumns()
}
const resetColumns = () => {
  visibleKeys.value = activeColDefs.value.filter(c => c.default).map(c => c.key)
  persistColumns()
}
const persistColumns = () => {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ }
}

// ---- 数据 ----
const meters = ref<any[]>([])
const rows = ref<any[]>([]) // 展开后的扁平行
const displayRows = ref<any[]>([])
const loading = ref(true)
// 列表滚动容器（表头列宽实测的观察对象）
const listScrollRef = ref<HTMLElement>()
const filters = ref<{ keyword?: string; month?: string; kind?: string; useUnit?: string }>({ keyword: '', month: undefined, kind: undefined, useUnit: undefined })

const kindFilterOptions = computed(() => [
  ...meterKinds.value.map(k => ({ label: k.label, value: k.value })),
])

const useUnitOptions = computed(() => {
  const set = new Set<string>()
  meters.value.forEach((m: any) => { if (m.use_unit) set.add(m.use_unit) })
  return Array.from(set).map(v => ({ label: v, value: v }))
})
// BusinessFilterBar 的扁平筛选态 ↔ 既有 filters ref 双向桥接
const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'month', type: 'month', placeholder: '月份' },
  { key: 'kind', type: 'select', placeholder: '表计类型', icon: 'file', options: kindFilterOptions.value },
  { key: 'useUnit', type: 'select', placeholder: '使用单位', icon: 'usergroup', options: useUnitOptions.value },
])
const filterState = computed<FilterState>({
  get: () => ({ keyword: filters.value.keyword || '', month: filters.value.month, kind: filters.value.kind, useUnit: filters.value.useUnit }),
  set: (v) => {
    filters.value = {
      keyword: String(v?.keyword ?? ''),
      month: v?.month || undefined,
      kind: v?.kind || undefined,
      useUnit: v?.useUnit || undefined,
    }
  },
})
// 用途体系:内置 宿舍/工商业(锁定 value 驱动租户核算计算,label 可改名) + 用户自定义
// 公租房降为自定义用途(可删除/修改/新增),仅分组展示、不参与租户核算计算
const KIND_BUILTIN = reactive([
  { value: 'dorm', label: '宿舍', builtin: true },
  { value: 'production', label: '工商业', builtin: true },
])
const kindsStoreKey = 'weknora-utility-kinds'
type KindItem = { id?: string; value: string; label: string }
const customKinds = ref<KindItem[]>([])
const meterKinds = computed(() => [...KIND_BUILTIN, ...customKinds.value])

// 从后端加载自定义用途；首次使用且后端为空时迁移旧 localStorage 数据（保留 value/label）
const loadKinds = async () => {
  try {
    const res: any = await listUtilityKinds(props.category)
    const list = res?.data || []
    if (Array.isArray(list) && list.length) {
      // 后端已持久化内置用途：内置 value 过滤出 customKinds，内置 label 同步到 KIND_BUILTIN
      customKinds.value = list
        .filter((k: any) => !KIND_BUILTIN.some(b => b.value === k.value))
        .map((k: any) => ({ id: k.id, value: k.value, label: k.label }))
      for (const k of list) {
        const b = KIND_BUILTIN.find(x => x.value === k.value)
        if (b && k.label) b.label = k.label
      }
      return
    }
    // 迁移旧本地数据
    let old: any[] = []
    try { old = JSON.parse(localStorage.getItem(kindsStoreKey) || '[]') } catch { /* ignore */ }
    const olds = Array.isArray(old)
      ? old.filter((k: any) => k && k.value && k.label && !KIND_BUILTIN.some(b => b.value === k.value))
      : []
    for (const k of olds) {
      try {
        const created: any = await createUtilityKind(props.category, { value: k.value, label: k.label })
        const item = created?.data || created
        if (item?.value) customKinds.value.push({ id: item.id, value: item.value, label: item.label })
      } catch { /* ignore */ }
    }
    localStorage.removeItem(kindsStoreKey)
  } catch (e) {
    console.error('load kinds failed', e)
  }
}

const load = async () => {
  loading.value = true
  try {
    const [recRes, meterRes]: any[] = await Promise.all([
      listUtilityMeterRecords({ category: props.category }),
      listUtilityMeters({ category: props.category }),
    ])
    const recData = recRes?.data || {}
    const records = recData?.records || recRes?.records || []
    const mets = meterRes?.data || recData?.meters || meterRes || []
    meters.value = Array.isArray(mets) ? mets : []
    await loadKinds()
    const meterMap = new Map(meters.value.map((m: any) => [m.id, m]))
    const flat: any[] = []
    for (const rec of records) {
      for (const it of (rec.items || [])) {
        const meter = meterMap.get(it.meter_id)
        flat.push({
          record_id: rec.id,
          item_id: it.id,
          key: `${rec.id}__${it.id}`,
          // TDesign 的 row-key 只认字符串字段名；直接把 rowKey 落成字段，
          // 三处兜底保证任何一行都不会是 undefined/空串（否则勾选一行=全选）。
          rowKey: `${rec.id || rec.month || 'rec'}__${it.id || it.meter_id || flat.length}`,
          month: rec.month,
          record_date: rec.record_date || '',
          reading_date: it.reading_date || '',
          reader: it.reader || '',
          meter_id: it.meter_id,
          meter_alias: meter?.alias || it.meter_name || '未配置',
          meter_no: meter?.meter_no || '',
          meter_type: meter?.meter_type || '',
          meter_kind: meter?.meter_kind || 'other',
          use_unit: meter?.use_unit || '',
          default_unit_price: meter?.default_unit_price ?? '',
          meter_mode: meter?.meter_mode || '',
          install_date: meter?.install_date || '',
          start_reading: it.start_reading,
          end_reading: it.end_reading,
          deep_prev: it.deep_prev,
          deep_curr: it.deep_curr,
          peak_prev: it.peak_prev,
          peak_curr: it.peak_curr,
          flat_prev: it.flat_prev,
          flat_curr: it.flat_curr,
          valley_prev: it.valley_prev,
          valley_curr: it.valley_curr,
          rate: meter?.rate ?? it.rate ?? '',
          usage: it.usage,
          unit_price: it.unit_price,
          garbage_fee: it.garbage_fee,
          secondary_water_fee: it.secondary_water_fee,
          sewage_fee: it.sewage_fee,
          subsidy: it.subsidy,
          amount: it.amount,
          remark: it.remark || '',
        })
      }
    }
    rows.value = flat
    // 构建 表计→月份→读数快照 索引（新增记录时上期止度自动填充本期起度）
    //
    // 快照必须带四时段止度：分时电表不使用 start_reading/end_reading，
    // 只索引 end_reading 会让分时表的起度永远填 0（真机：2026-09 持睿工业分表1
    // 尖峰平谷起度全是 0，而 2026-08 的止度是 1833/5449/7304/7284）。
    const idx = new Map<string, Map<string, ReadingSnapshot>>()
    for (const r of flat) {
      if (!r.meter_id) continue
      let m = idx.get(r.meter_id)
      if (!m) { m = new Map(); idx.set(r.meter_id, m) }
      m.set(r.month, {
        end_reading: Number(r.end_reading) || 0,
        deep_curr: Number(r.deep_curr) || 0,
        peak_curr: Number(r.peak_curr) || 0,
        flat_curr: Number(r.flat_curr) || 0,
        valley_curr: Number(r.valley_curr) || 0,
      })
    }
    meterMonthEnds.value = idx
    // 抄表人兜底：本地无记录时，取最近一次已保存记录的抄表人作为默认
    if (!lastReader.value) {
      const last = [...flat].reverse().find((r: any) => (r.reader || '').trim())
      if (last?.reader) lastReader.value = last.reader
    }
    applyFilters()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
    // 数据渲染完成后再量一次列宽（首屏没有展开行，但表头宽度决定后续对齐）
    void nextTick(() => nextTick(measureTableColumns))
  }
}

const applyFilters = () => {
  let list = rows.value
  // 「搜索全部字段」：行内所有可读值（月份/表计/表号/起止度/用量/单价/费用/
  // 抄表人/使用单位/备注…）拼串匹配，多关键词空格分词取 AND。
  // 旧版把搜索框接在别处、列表根本没按 keyword 过滤（真机：搜"宿舍301"纹丝不动）。
  const kw = String(filters.value.keyword || '').trim()
  if (kw) list = list.filter(r => matchAllTokens(rowFlattenText(r), kw))
  if (filters.value.month) list = list.filter(r => r.month === filters.value.month)
  if (filters.value.kind) list = list.filter(r => r.meter_kind === filters.value.kind)
  // 使用单位是表计档案属性：记录本身不带该字段，必须用扁平行上的 use_unit 比对
  // （旧实现读的是未联动的旧 ref，选「持睿汽车」列表仍是「重庆星达」）。
  if (filters.value.useUnit) list = list.filter(r => String(r.use_unit || '').trim() === filters.value.useUnit)
  // 同一周期内默认排序:按用途配置顺序(宿舍→公租房→工商业...);跨月保持数据原序
  const order = new Map(meterKinds.value.map((k, i) => [k.value, i]))
  list = [...list].sort((a, b) => {
    if (a.month !== b.month) return 0
    return (order.get(a.meter_kind) ?? 99) - (order.get(b.meter_kind) ?? 99)
  })
  displayRows.value = list
  // 表头列宽随数据（自适应列）变化 → 展开子表 colgroup 需要重测
  void nextTick(measureTableColumns)
}

/**
 * 筛选态 → 列表重算。
 *
 * BusinessFilterBar 只发不可变更新（`update:modelValue`），非 `immediate` 的控件
 * 不会派发 `filter-change`。旧版把重算挂在 `load()`（只有刷新/筛选事件才走），
 * 于是「选了下拉框但列表不动」——这是本次 6 个视图筛选失效的共同根因。
 * 这里直接监听筛选态本身：任何键变化都立即重算，且纯本地、零请求。
 */
watch(filters, () => applyFilters(), { deep: true })

// ---- 汇总 ----
const summary = computed(() => {
  const base = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return { total: base.length }
})
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => rows.value.filter(r => selectedKeys.value.has(r.key)))
const summaryUsage = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.usage) || 0), 0) * 100) / 100
})
const summaryAmount = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.amount) || 0), 0) * 100) / 100
})

// ---- 分时表：行内四时段展开 ----
const isTimeRow = (row: any) => row.meter_type === 'time'
const periodsOf = (row: any) => {
  const rate = Number(row.rate) > 0 ? Number(row.rate) : 1
  const mk = (name: string, prev: number, curr: number) => ({
    name,
    prev: Number(prev) || 0,
    curr: Number(curr) || 0,
    usage: Math.round((Number(curr) - Number(prev)) * rate * 100) / 100,
  })
  return [
    mk('尖', row.deep_prev, row.deep_curr),
    mk('峰', row.peak_prev, row.peak_curr),
    mk('平', row.flat_prev, row.flat_curr),
    mk('谷', row.valley_prev, row.valley_curr),
  ]
}

// 单击行：单选该行并打开编辑抽屉（与发票管理一致）
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.key])
  openEdit(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 编辑抽屉 ----
const drawerVisible = ref(false)
const saving = ref(false)
const editingRecordId = ref('')
const editingItemId = ref('')
const editingOriginalMonth = ref('') // 编辑行原月份（改月时仅迁移该行）
const editingOriginalMeterId = ref('') // 编辑行原表计（改表时删除原行避免产生空记录）
const lastRecordId = ref('')
const recordItems = ref<any[]>([]) // 当前编辑 record 的原始 items（编辑时保留其它行）
interface MeterForm {
  month: string; useUnit: string; meterId: string; readingDate: string; reader: string; recordDate: string
  startReading: number; endReading: number
  deepPrev: number; deepCurr: number; peakPrev: number; peakCurr: number
  flatPrev: number; flatCurr: number; valleyPrev: number; valleyCurr: number
  unitPrice: number; subsidy: string | number; remark: string
  garbageFee: number; secondaryWaterFee: number; sewageFee: number
}
const emptyForm = (): MeterForm => ({
  month: '', useUnit: '', meterId: '', readingDate: '', reader: '', recordDate: '',
  startReading: 0, endReading: 0,
  deepPrev: 0, deepCurr: 0, peakPrev: 0, peakCurr: 0,
  flatPrev: 0, flatCurr: 0, valleyPrev: 0, valleyCurr: 0,
  unitPrice: 0, subsidy: '', remark: '',
  // 水费三项附加费默认全 0：旧实现把「垃圾处置费」预填 13 元/套，
  // 新增记录时用户没注意就被算进金额（金额 = 用量×单价 + 三项附加）。
  // 需要时手工填，编辑已有记录仍按 saved 值回显。
  garbageFee: 0, secondaryWaterFee: 0, sewageFee: 0,
})
const form = ref<MeterForm>(emptyForm())
// 补差输入过滤:仅保留数字、负号(仅开头)、小数点(仅一个),直接输入 -0.16 不被吞
watch(() => form.value.subsidy, (v) => {
  if (typeof v === 'string' && /[^0-9.\-]/.test(v)) {
    form.value.subsidy = v.replace(/[^0-9.\-]/g, '')
  }
  if (typeof v === 'string' && /(?!^)-|\.\d*\./.test(v)) {
    form.value.subsidy = v.replace(/(?!^)-/g, '').replace(/(\..*)\./g, '$1')
  }
})
const today = (() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
})()
// 最近一次录入的抄表人：本地持久化，新增记录自动带出避免手动输入
const lastReaderKey = 'weknora-utility-last-reader'
const lastReader = ref<string>('')
try { lastReader.value = localStorage.getItem(lastReaderKey) || '' } catch { /* ignore */ }
const rememberReader = (r: string) => {
  const v = (r || '').trim()
  if (!v) return
  lastReader.value = v
  try { localStorage.setItem(lastReaderKey, v) } catch { /* ignore */ }
}
const drawerTitle = computed(() => (editingItemId.value ? '编辑' + categoryLabel.value + '记录' : '新增' + categoryLabel.value + '记录'))

// 系统内使用单位（去重，保持稳定顺序，默认取第一项）
const useUnits = computed(() => {
  const set = new Map<string, string>()
  for (const m of meters.value as any[]) {
    const u = (m.use_unit || '').trim()
    if (u) set.set(u, u)
  }
  return Array.from(set.keys())
})

// 新增/编辑模式选项：按使用单位过滤 + 新增时隐藏当月已录入的表计
const meterEditOptions = computed(() => {
  const u = (form.value.useUnit || '').trim()
  const opts = meters.value
    .filter((m: any) => {
      if (!m.enabled) return false
      if (u && (m.use_unit || '').trim() !== u) return false
      return true
    })
    .map((m: any) => ({ label: m.alias, value: m.id }))
  if (!editingItemId.value) {
    const recorded = new Set<string>()
    if (form.value.month) {
      for (const m of meters.value as any[]) {
        if (meterMonthEnds.value.get(m.id)?.has(form.value.month)) recorded.add(m.id)
      }
    }
    return opts.filter(o => !recorded.has(o.value))
  }
  // 编辑时若当前表计已停用或不在使用单位内，保留在选项中
  if (form.value.meterId && !opts.some(o => o.value === form.value.meterId)) {
    const cur = meters.value.find((m: any) => m.id === form.value.meterId)
    if (cur) opts.push({ label: cur.alias, value: cur.id })
  }
  return opts
})

// 切换使用单位：清空表计选择（读数由表计 watch 重置）
const onUseUnitChange = () => {
  if (editingItemId.value) return
  if (form.value.meterId) {
    const cur = meters.value.find((m: any) => m.id === form.value.meterId) as any
    if (cur && (cur.use_unit || '').trim() !== (form.value.useUnit || '').trim()) {
      form.value.meterId = ''
    }
  }
}

const currentMeter = computed(() => meters.value.find((m: any) => m.id === form.value.meterId))
const isTimeMeter = computed(() => currentMeter.value?.meter_type === 'time')
const formRate = computed(() => Number(currentMeter.value?.rate) > 0 ? Number(currentMeter.value?.rate) : 1)
const formUsage = computed(() => {
  const rate = formRate.value
  if (isTimeMeter.value) {
    const d = Number(form.value.deepCurr) - Number(form.value.deepPrev)
    const p = Number(form.value.peakCurr) - Number(form.value.peakPrev)
    const f = Number(form.value.flatCurr) - Number(form.value.flatPrev)
    const v = Number(form.value.valleyCurr) - Number(form.value.valleyPrev)
    return Math.round((d + p + f + v) * rate * 100) / 100
  }
  return Math.round((Number(form.value.endReading) - Number(form.value.startReading)) * rate * 100) / 100
})
const formAmount = computed(() => {
  // 分时电表不按「用电量 × 单一标准单价」计费（费用走租户分摊核算），金额不参与计算
  if (isTimeMeter.value) return 0
  const base = formUsage.value * Number(form.value.unitPrice)
  const fees = props.category === 'water'
    ? (Number(form.value.garbageFee) || 0) + (Number(form.value.secondaryWaterFee) || 0) + (Number(form.value.sewageFee) || 0)
    : 0
  return Math.round((base + fees + (Number(form.value.subsidy) || 0)) * 100) / 100
})

// ---- 录入校验：差异标红提醒不拦截，止度<起度（起度>止度）标红且保存拦截；起度=止度视为当月无用量，合法 ----
const readingInvalid = computed(() => {
  if (isTimeMeter.value) {
    return Number(form.value.deepCurr) < Number(form.value.deepPrev) ||
      Number(form.value.peakCurr) < Number(form.value.peakPrev) ||
      Number(form.value.flatCurr) < Number(form.value.flatPrev) ||
      Number(form.value.valleyCurr) < Number(form.value.valleyPrev)
  }
  const s = Number(form.value.startReading) || 0
  const e = Number(form.value.endReading) || 0
  return s > e
})
const duplicateWarning = computed(() => {
  if (editingItemId.value) return false
  if (!form.value.month || !form.value.meterId) return false
  return meterMonthEnds.value.get(form.value.meterId)?.has(form.value.month) || false
})
const unitPriceDiff = computed(() => {
  if (isTimeMeter.value) return false // 分时表无标准单价可比
  const up = Number(form.value.unitPrice)
  const def = Number(currentMeter.value?.default_unit_price)
  return !!form.value.meterId && up > 0 && def > 0 && up !== def
})
// 设置抽屉表计按居民/工商业分组表格展示(原卡片网格列数逻辑移除)

const onMeterChange = () => {
  // 切换表计：新增/连续录入模式清空**本期止度**便于录入；起度由
  // applyPrevReadings() 按「该表上期止度」重填（普通表 1 个、分时表 4 段）。
  // 编辑模式保留原读数（修正表号场景数据应跟随记录，而不是产生空记录）
  if (!editingItemId.value) {
    form.value.endReading = 0
    form.value.deepCurr = 0
    form.value.peakCurr = 0
    form.value.flatCurr = 0
    form.value.valleyCurr = 0
    applyPrevReadings()
  }
  if (currentMeter.value) {
    form.value.unitPrice = Number(currentMeter.value.default_unit_price) || 0
    // 抄表人：表计管理人员优先，未配置则沿用最近一次录入的抄表人
    form.value.reader = currentMeter.value.manager || lastReader.value || form.value.reader || ''
  }
}

const openCreate = () => {
  editingRecordId.value = ''
  editingItemId.value = ''
  editingOriginalMonth.value = ''
  editingOriginalMeterId.value = ''
  lastRecordId.value = ''
  recordItems.value = []
  const f = emptyForm()
  f.readingDate = today
  f.recordDate = today
  // 默认月份=上月(如抄表日期 9 月 → 默认 2026-08)
  f.month = prevMonthOf(today.slice(0, 7))
  f.useUnit = useUnits.value[0] || ''
  // 抄表人默认沿用最近一次录入的抄表人，避免每次手动输入
  f.reader = lastReader.value || ''
  form.value = f
  drawerVisible.value = true
}

// ---- 新增记录：上期止度自动填充本期起度 ----
/** 某表计某月的读数快照（普通表用 end_reading，分时表用四段 curr） */
interface ReadingSnapshot {
  end_reading: number
  deep_curr: number
  peak_curr: number
  flat_curr: number
  valley_curr: number
}
const meterMonthEnds = ref<Map<string, Map<string, ReadingSnapshot>>>(new Map())
const prevMonthOf = (month: string) => {
  const [y, m] = month.split('-').map(Number)
  if (!y || !m) return ''
  if (m === 1) return `${y - 1}-12`
  return `${y}-${String(m - 1).padStart(2, '0')}`
}
/**
 * 取该表计「账单周期之前最近一期」的读数快照。
 *
 * 先找上一自然月；该月没有记录时回退到更早的最近一期 —— 真机上表计常隔月抄
 * （如 2026-09 新增时只有 2026-06 的记录），只查 M-1 会让起度直接落 0。
 */
function prevSnapshotOf(meterId: string, month: string): ReadingSnapshot | null {
  const byMonth = meterMonthEnds.value.get(meterId)
  if (!byMonth || !month) return null
  const exact = byMonth.get(prevMonthOf(month))
  if (exact) return exact
  // 月份是 YYYY-MM，字典序即时间序：取 < 目标月里最大的那个
  let best = ''
  for (const k of byMonth.keys()) {
    if (k < month && k > best) best = k
  }
  return best ? byMonth.get(best) || null : null
}
/** 把快照写进表单的起度位置：普通表写 startReading，分时表写四段 prev */
function applyPrevReadings() {
  const meter = currentMeter.value
  const prev = prevSnapshotOf(form.value.meterId, form.value.month)
  if (meter?.meter_type === 'time') {
    form.value.deepPrev = prev?.deep_curr ?? 0
    form.value.peakPrev = prev?.peak_curr ?? 0
    form.value.flatPrev = prev?.flat_curr ?? 0
    form.value.valleyPrev = prev?.valley_curr ?? 0
    // 止度同步预填为起度：避免一打开就显示「止度 0 < 起度 1833」的红色报错
    // 和 -4374000 千瓦时的负用量（抄表场景本来就是"从上次读数起抄"）。
    form.value.deepCurr = form.value.deepPrev
    form.value.peakCurr = form.value.peakPrev
    form.value.flatCurr = form.value.flatPrev
    form.value.valleyCurr = form.value.valleyPrev
    return
  }
  form.value.startReading = prev?.end_reading ?? 0
  form.value.endReading = form.value.startReading
}
// 连续录入时跳过自动填充（起度/单价由保存逻辑显式写入）
const skipAutoFill = ref(false)
watch([() => form.value.month, () => form.value.meterId], () => {
  if (editingItemId.value) return // 编辑模式不填充
  if (skipAutoFill.value) { skipAutoFill.value = false; return } // 连续录入：已显式设置
  if (!form.value.month || !form.value.meterId) return
  applyPrevReadings()
})

const openEditSelected = () => {
  const row = selectedRows.value[0]
  if (!row) return
  openEdit(row)
}

const openEdit = (row: any) => {
  editingRecordId.value = row.record_id
  editingItemId.value = row.item_id
  editingOriginalMonth.value = row.month
  editingOriginalMeterId.value = row.meter_id || ''
  const rec = rowsOfRecord(row.record_id)
  recordItems.value = rec.map((r: any) => ({
    item_id: r.item_id,
    meter_id: r.meter_id,
    reading_date: r.reading_date || '',
    reader: r.reader || '',
    start_reading: Number(r.start_reading) || 0,
    end_reading: Number(r.end_reading) || 0,
    deep_prev: Number(r.deep_prev) || 0,
    deep_curr: Number(r.deep_curr) || 0,
    peak_prev: Number(r.peak_prev) || 0,
    peak_curr: Number(r.peak_curr) || 0,
    flat_prev: Number(r.flat_prev) || 0,
    flat_curr: Number(r.flat_curr) || 0,
    valley_prev: Number(r.valley_prev) || 0,
    valley_curr: Number(r.valley_curr) || 0,
    unit_price: Number(r.unit_price) || 0,
    garbage_fee: Number(r.garbage_fee) || 0,
    secondary_water_fee: Number(r.secondary_water_fee) || 0,
    sewage_fee: Number(r.sewage_fee) || 0,
    subsidy: Number(r.subsidy) || 0,
    remark: r.remark || '',
  }))
  form.value = {
    month: row.month,
    useUnit: (meters.value as any[]).find((m: any) => m.id === row.meter_id)?.use_unit || row.use_unit || '',
    meterId: row.meter_id || '',
    readingDate: row.reading_date || today,
    reader: row.reader || '',
    recordDate: row.record_date || today,
    startReading: Number(row.start_reading) || 0,
    endReading: Number(row.end_reading) || 0,
    deepPrev: Number(row.deep_prev) || 0,
    deepCurr: Number(row.deep_curr) || 0,
    peakPrev: Number(row.peak_prev) || 0,
    peakCurr: Number(row.peak_curr) || 0,
    flatPrev: Number(row.flat_prev) || 0,
    flatCurr: Number(row.flat_curr) || 0,
    valleyPrev: Number(row.valley_prev) || 0,
    valleyCurr: Number(row.valley_curr) || 0,
    unitPrice: Number(row.unit_price) || 0,
    garbageFee: Number(row.garbage_fee) || 0,
    secondaryWaterFee: Number(row.secondary_water_fee) || 0,
    sewageFee: Number(row.sewage_fee) || 0,
    subsidy: Number(row.subsidy) || 0,
    remark: row.remark || '',
  }
  drawerVisible.value = true
}

function rowsOfRecord(recordId: string) {
  return rows.value.filter(r => r.record_id === recordId)
}

const save = async () => {
  if (!form.value.month) {
    MessagePlugin.warning('请选择月份')
    return
  }
  if (!form.value.meterId) {
    MessagePlugin.warning(`请选择${meterLabel.value}`)
    return
  }
  if (readingInvalid.value) {
    MessagePlugin.error('止度不得小于起度')
    return
  }
  // 新增时重复校验：同月同表已有记录则拦截，避免重复录入
  if (!editingItemId.value && !lastRecordId.value && duplicateWarning.value) {
    MessagePlugin.warning(`该月该${meterLabel.value}已有记录，请在列表中选择该记录进行编辑`)
    return
  }
  saving.value = true
  try {
    const editedItem = {
      meter_id: form.value.meterId,
      reading_date: form.value.readingDate || '',
      reader: form.value.reader || '',
      start_reading: Number(form.value.startReading) || 0,
      end_reading: Number(form.value.endReading) || 0,
      deep_prev: Number(form.value.deepPrev) || 0,
      deep_curr: Number(form.value.deepCurr) || 0,
      peak_prev: Number(form.value.peakPrev) || 0,
      peak_curr: Number(form.value.peakCurr) || 0,
      flat_prev: Number(form.value.flatPrev) || 0,
      flat_curr: Number(form.value.flatCurr) || 0,
      valley_prev: Number(form.value.valleyPrev) || 0,
      valley_curr: Number(form.value.valleyCurr) || 0,
      unit_price: isTimeMeter.value ? 0 : Number(form.value.unitPrice) || 0,
      garbage_fee: Number(form.value.garbageFee) || 0,
      secondary_water_fee: Number(form.value.secondaryWaterFee) || 0,
      sewage_fee: Number(form.value.sewageFee) || 0,
      subsidy: Number(form.value.subsidy) || 0,
      remark: form.value.remark || '',
    }
    const isNew = !editingItemId.value
    if (editingItemId.value) {
      // 单行改月份：仅迁移该行到目标月，其它行保留原月（不整月搬家）
      const monthChanged = form.value.month !== editingOriginalMonth.value
      if (monthChanged && recordItems.value.length > 1) {
        const others = recordItems.value.filter((it: any) => it.item_id !== editingItemId.value)
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: editingOriginalMonth.value,
          record_date: form.value.recordDate || today,
          remark: '',
          items: others.map((it: any) => ({
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: Number(it.start_reading) || 0,
            end_reading: Number(it.end_reading) || 0,
            deep_prev: Number(it.deep_prev) || 0,
            deep_curr: Number(it.deep_curr) || 0,
            peak_prev: Number(it.peak_prev) || 0,
            peak_curr: Number(it.peak_curr) || 0,
            flat_prev: Number(it.flat_prev) || 0,
            flat_curr: Number(it.flat_curr) || 0,
            valley_prev: Number(it.valley_prev) || 0,
            valley_curr: Number(it.valley_curr) || 0,
            unit_price: Number(it.unit_price) || 0,
            garbage_fee: Number(it.garbage_fee) || 0,
            secondary_water_fee: Number(it.secondary_water_fee) || 0,
            sewage_fee: Number(it.sewage_fee) || 0,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          })),
          // 该行迁往新月份：从原 record 删除
          delete_item_ids: [editingItemId.value],
        })
        await createUtilityMeterRecord({
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items: [editedItem],
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      } else {
        // 更新：整 record 全量提交，替换编辑行、保留其它行
        const items = recordItems.value.map((it: any) => {
          if (it.item_id === editingItemId.value) return editedItem
          return {
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: it.start_reading,
            end_reading: it.end_reading,
            deep_prev: it.deep_prev,
            deep_curr: it.deep_curr,
            peak_prev: it.peak_prev,
            peak_curr: it.peak_curr,
            flat_prev: it.flat_prev,
            flat_curr: it.flat_curr,
            valley_prev: it.valley_prev,
            valley_curr: it.valley_curr,
            unit_price: it.unit_price,
            garbage_fee: Number(it.garbage_fee) || 0,
            secondary_water_fee: Number(it.secondary_water_fee) || 0,
            sewage_fee: Number(it.sewage_fee) || 0,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          }
        })
        // 表计变更：删除原表计行，避免「新表新增 + 旧表残留」产生空记录
        const meterChanged = !!editingOriginalMeterId.value && editingOriginalMeterId.value !== form.value.meterId
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items,
          delete_item_ids: meterChanged ? [editingItemId.value] : [],
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      }
    } else if (lastRecordId.value) {
      // 连续录入后续条：同月 record 追加 item（同月唯一，后端要求 update）
      const existing = rowsOfRecord(lastRecordId.value)
      const items = [
        ...existing.map((r: any) => ({
          meter_id: r.meter_id,
          reading_date: r.reading_date || '',
          reader: r.reader || '',
          start_reading: Number(r.start_reading) || 0,
          end_reading: Number(r.end_reading) || 0,
          deep_prev: Number(r.deep_prev) || 0,
          deep_curr: Number(r.deep_curr) || 0,
          peak_prev: Number(r.peak_prev) || 0,
          peak_curr: Number(r.peak_curr) || 0,
          flat_prev: Number(r.flat_prev) || 0,
          flat_curr: Number(r.flat_curr) || 0,
          valley_prev: Number(r.valley_prev) || 0,
          valley_curr: Number(r.valley_curr) || 0,
          unit_price: Number(r.unit_price) || 0,
          garbage_fee: Number(r.garbage_fee) || 0,
          secondary_water_fee: Number(r.secondary_water_fee) || 0,
          sewage_fee: Number(r.sewage_fee) || 0,
          subsidy: Number(r.subsidy) || 0,
          remark: r.remark || '',
        })),
        editedItem,
      ]
      await updateUtilityMeterRecord(lastRecordId.value, {
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items,
      })
      MessagePlugin.success('已新增')
    } else {
      const res: any = await createUtilityMeterRecord({
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items: [editedItem],
      })
      const rec = res?.data?.data || res?.data || res
      lastRecordId.value = rec?.id || ''
      MessagePlugin.success('已新增')
    }
    const nextMeter = (() => {
      if (!isNew) return undefined
      // 连续录入：同使用单位、启用且当月未录入的表，按序找下一张（无则循环回第一张未录入的）
      const u = (form.value.useUnit || '').trim()
      const candidates = meters.value.filter((m: any) => {
        if (!m.enabled) return false
        if (u && (m.use_unit || '').trim() !== u) return false
        return !meterMonthEnds.value.get(m.id)?.has(form.value.month)
      })
      if (!candidates.length) return undefined
      const idx = candidates.findIndex((m: any) => m.id === form.value.meterId)
      if (idx >= 0) return candidates[idx + 1] || candidates[0]
      return candidates[0]
    })()
    if (nextMeter) {
      // 连续录入：抽屉不关闭，按启用表顺序切到下一张，月份复用、抄表日期=第一条、
      // 起度=下一张表自身上月止度（无则空）、单价=上一条、抄表人=下张表管理人员
      skipAutoFill.value = true
      const f = emptyForm()
      f.month = form.value.month
      f.useUnit = form.value.useUnit
      f.meterId = nextMeter.id
      f.readingDate = form.value.readingDate || today
      f.recordDate = form.value.recordDate || today
      f.reader = nextMeter.manager || lastReader.value || form.value.reader || ''
      const nextPrev = prevSnapshotOf(nextMeter.id, form.value.month)
      f.startReading = nextPrev?.end_reading ?? 0
      f.deepPrev = nextPrev?.deep_curr ?? 0
      f.peakPrev = nextPrev?.peak_curr ?? 0
      f.flatPrev = nextPrev?.flat_curr ?? 0
      f.valleyPrev = nextPrev?.valley_curr ?? 0
      // 分时电表不按单一标准单价核算（费用走租户分摊），单价留空
      f.unitPrice = nextMeter.meter_type === 'time' ? 0 : Number(form.value.unitPrice) || 0
      form.value = f
      editingRecordId.value = ''
      editingItemId.value = ''
      recordItems.value = []
    } else {
      drawerVisible.value = false
      clearSelection()
      lastRecordId.value = ''
    }
    await load()
    notifyEnergyDataChanged()
    rememberReader(form.value.reader)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const onDrawerClose = () => { drawerVisible.value = false }


// ---- 删除 ----
const handleDelete = async () => {
  const targets = selectedRows.value
  if (!targets.length) return
  // 按 record 分组：单 item record 直接删 record；多 item record 重提交去掉目标行
  const groups = new Map<string, any[]>()
  for (const r of targets) {
    const arr = groups.get(r.record_id) || []
    arr.push(r)
    groups.set(r.record_id, arr)
  }
  try {
    for (const [recordId, itemRows] of groups) {
      const all = rowsOfRecord(recordId)
      const targetItemIds = new Set(itemRows.map(r => r.item_id))
      const remaining = all.filter(r => !targetItemIds.has(r.item_id))
      if (!remaining.length) {
        await deleteUtilityMeterRecord(recordId)
      } else {
        await updateUtilityMeterRecord(recordId, {
          category: props.category,
          month: remaining[0].month,
          record_date: remaining[0].record_date || '',
          remark: '',
          delete_item_ids: Array.from(targetItemIds),
          items: remaining.map((r: any) => ({
            meter_id: r.meter_id,
            reading_date: r.reading_date || '',
            reader: r.reader || '',
            start_reading: Number(r.start_reading) || 0,
            end_reading: Number(r.end_reading) || 0,
            deep_prev: Number(r.deep_prev) || 0,
            deep_curr: Number(r.deep_curr) || 0,
            peak_prev: Number(r.peak_prev) || 0,
            peak_curr: Number(r.peak_curr) || 0,
            flat_prev: Number(r.flat_prev) || 0,
            flat_curr: Number(r.flat_curr) || 0,
            valley_prev: Number(r.valley_prev) || 0,
            valley_curr: Number(r.valley_curr) || 0,
            unit_price: Number(r.unit_price) || 0,
            garbage_fee: Number(r.garbage_fee) || 0,
            secondary_water_fee: Number(r.secondary_water_fee) || 0,
            sewage_fee: Number(r.sewage_fee) || 0,
            remark: r.remark || '',
          })),
        })
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await load()
    notifyEnergyDataChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 打印 ----
const printVisible = ref(false)
const catalogBusy = ref(false)
const printUrl = ref('')
const printBytes = ref<Uint8Array | ArrayBuffer | null>(null)
const printCount = ref(0)

const catalogValueOf = (r: any, key: string): string => {
  switch (key) {
    case 'month': return r.month
    case 'meter': return r.meter_alias
    case 'start_reading': return fmtNum(r.start_reading)
    case 'end_reading': return fmtNum(r.end_reading)
    case 'rate': return fmtNum(r.rate)
    case 'usage': return `${fmtNum(r.usage)} ${unitLabel.value}`
    case 'unit_price': return fmtUnitPrice(r.unit_price)
    case 'subsidy': return r.subsidy === '' || r.subsidy == null ? '' : fmtMoney(r.subsidy)
    case 'amount': return `${fmtMoney(r.amount)} 元`
    case 'reading_date': return r.reading_date || ''
    case 'reader': return r.reader || ''
    case 'meter_no': return r.meter_no || ''
    case 'use_unit': return r.use_unit || ''
    case 'default_unit_price': return r.default_unit_price === '' || r.default_unit_price == null ? '' : fmtNum(r.default_unit_price)
    case 'meter_mode': return r.meter_mode === 'auto' ? '自动抄表' : r.meter_mode === 'manual' ? '手动抄表' : (r.meter_mode || '')
    case 'install_date': return r.install_date || ''
    case 'remark': return r.remark || ''
    default: return ''
  }
}

const handlePrint = async () => {
  const rowsToPrint = selectedRows.value
  if (!rowsToPrint.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = visibleColDefs.value.map((c: any) => ({
      key: c.key,
      label: c.label,
      value: (r: any) => catalogValueOf(r, c.key),
    }))
    const bytes = await generateCatalogPdf({ title: `${categoryLabel.value}目录`, columns: cols, rows: rowsToPrint })
    printCount.value = rowsToPrint.length
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printBytes.value = bytes
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
  printBytes.value = null
}

// ---- 表计配置（已迁至 MeterSettingsPanel，作为统一设置抽屉的「表计设置」Tab） ----
// 计量层级按类别: 电表 普通/分时; 水表 总表/分表/消防; 气表 普通
const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
// 单价统一保留最多三位小数,末尾 0 不显示(如 3.4 / 5.22 / 2.196)
const fmtUnitPrice = (v: any) => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return String(Math.round(n * 1000) / 1000)
}

// 列宽实测的生命周期：挂载后量一次，容器尺寸变化（窗口缩放 / 侧栏收起 /
// 列显隐切换）重测，保证展开子表的 colgroup 始终等于主表真实列宽。
let columnResizeObserver: ResizeObserver | null = null
onMounted(() => {
  load()
  void loadCategorySubs()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
  void nextTick(() => {
    measureTableColumns()
    const wrap = listScrollRef.value
    if (wrap && typeof ResizeObserver !== 'undefined') {
      columnResizeObserver = new ResizeObserver(() => measureTableColumns())
      columnResizeObserver.observe(wrap)
    }
  })
})
watch(visibleKeys, () => void nextTick(measureTableColumns))
/**
 * 字段定义保存后的联动：事件 detail 带 subs 时直接落内存（省一次 GET），
 * 否则回退到重新拉取。
 */
function onCategoriesChanged(ev?: Event) {
  const detail = (ev as CustomEvent | undefined)?.detail as { name?: string; subs?: any[] } | undefined
  const name = String(detail?.name || '').trim()
  const mine = CATEGORY_NAME[props.category] || ''
  if (Array.isArray(detail?.subs) && (!name || name === mine)) {
    applySubs(detail!.subs as EnergySub[])
    return
  }
  void loadCategorySubs()
}
onBeforeUnmount(() => {
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
  columnResizeObserver?.disconnect()
  columnResizeObserver = null
})
</script>

<style lang="less" scoped>
/* 中台列表共享样式：.doc-list-scroll / .doc-list-view / 表头遮罩 / 表格令牌。
   子组件是挂在 .biz-page 下的（父页面提供），但共享样式是按组件独立 import 的，
   不引入就会退化成 display:block + overflow:visible —— 表格不被限高、
   滚动条落在外层容器上，表现就是「滚动条侵入表头 + 汇总栏被顶到几千像素」。 */
@import './styles/business-list-shared.css';

.utility-meter-tab {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding-top: 4px;
}

/* 筛选工具栏（与合同管理一致） */
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

    .doc-date-picker {
      width: 160px;
    }

    .doc-filter-select {
      width: 160px;
    }
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

/* 列表组件样式（与合同/发票/知识库列表保持一致，scoped 自包含） */
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
}

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

  .cell {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
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

.doc-list-body {
  display: flex;
  flex-direction: column;
}

.doc-list-row {
  position: relative;
  min-height: 52px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background-color 0.2s ease;

  &:last-child {
    border-bottom: 0;
  }

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.row-selected {
    background: var(--td-brand-color-light);
  }
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

  &:first-child {
    padding-left: 0;
  }

  &:last-child {
    padding-right: 0;
  }
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
.row-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-mono {
  font-family: var(--app-font-family);
}

.os-neg { color: var(--td-error-color); }
.row-dash { color: var(--td-text-color-placeholder); }
/* 电表名称：文字从单元格最左侧起排（与普通电表 100% 对齐），
   展开符是名称**右侧**的独立小徽标，不占首字缩进。 */
.row-meter {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-width: 0;
  vertical-align: bottom;
}
.row-meter .row-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-expand-toggle {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  margin-left: 6px;
  /* 圆形徽标：方形带边框的样式会和行首复选框混淆 */
  border: none;
  border-radius: 50%;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  user-select: none;

  &:hover { background: var(--td-brand-color-light); color: var(--td-brand-color); }
  &.is-open {
    background: var(--td-brand-color);
    color: var(--td-text-color-anti);
  }
}

/* 分时明细：与主表共用列宽的展开子表（colgroup 由表头实测宽度驱动） */
.expand-inner--aligned {
  position: relative;
  padding: 0;
  /* 左缘偏移由 expandShift（运行时实测）以 inline style 补偿 */
  /* 脱离文档流：子表比内容盒宽时不会反推主表列宽，横向溢出直接裁掉。 */
  overflow: hidden;
  background: var(--td-bg-color-container-hover);
  border-bottom: 1px solid var(--td-component-stroke);
}
.expand-table {
  position: absolute;
  top: 0;
  table-layout: fixed;
  border-collapse: collapse;

  .expand-cell {
    /* 与 TDesign t-size-s 单元格同 padding：文字左缘与主表同列严格对齐 */
    padding: 6px var(--td-comp-paddingLR-s);
    height: 18px;
    font-size: 12px;
    line-height: 18px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .expand-head-row .expand-cell {
    color: var(--td-text-color-placeholder);
    background: var(--td-bg-color-secondarycontainer);
    font-weight: 600;
  }
  .expand-cell--num {
    font-family: var(--app-font-family);
    font-variant-numeric: tabular-nums;
  }
  tr + tr .expand-cell { border-top: 1px solid var(--td-component-stroke); }
  .expand-total-row .expand-cell {
    font-weight: 600;
    background: var(--td-bg-color-container);
  }
}

.meter-list-scroll {
  /* 只保留尺寸：overflow / 边框 / 圆角 / 背景 / 阴影一律走共享的
     `.biz-page .doc-list-scroll`。这里写 overflow-y:auto 会让滚动落到外层容器，
     表头 sticky 失效、垂直滚动条从容器顶端开始 → 侵入表头标题栏（真机截图）。 */
  flex: 1;
  min-height: 0;
}

.meter-empty {
  padding: 40px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);

  .meter-empty-icon {
    color: var(--td-text-color-placeholder);
  }

  .meter-empty-text {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}

/* 底部汇总（与电费/光伏一致） */
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

/* 抽屉 resize 手柄（发票管理同款，teleport 到 body 避免被抽屉遮罩遮挡） */
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

/* 浮动工具栏（与电费/合同一致） */
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

    .batch-bar-count {
      font-size: 13px;
      color: var(--td-text-color-primary);
    }

    .batch-bar-clear {
      color: var(--td-brand-color);
    }
  }

  .batch-bar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(6px);
}

/* 编辑抽屉 */
.meter-drawer-body {
  padding: 4px 0 24px;
}

/* 新增/编辑记录：两列紧凑布局 */
.rec-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 20px;
}
.rec-field {
  min-width: 0;

  /* 编辑框宽度统一：输入/选择/日期组件占满列宽 */
  :deep(.t-input__wrap),
  :deep(.t-select__wrap),
  :deep(.t-date-picker) {
    width: 100%;
    min-width: 0;
  }

  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;
  }
  .reading-row { display: flex; align-items: center; gap: 8px; }
  .reading-input { flex: 1; }
  .reading-sep { color: var(--td-text-color-placeholder); flex: 0 0 auto; }
  .field-error-text {
    font-size: 12px;
    color: var(--td-error-color);
    margin-top: 4px;
    line-height: 1.4;
  }
}
.rec-field--wide {
  grid-column: 1 / -1;
}
/* 分时电表：时段起/止并排，任意宽度不截断 */
.rec-period-pair {
  display: grid;
  grid-template-columns: 1fr 24px 1fr;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-width: 0;
  .t-input__wrap { width: 100%; min-width: 0; }
}
.rec-period-sep {
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  user-select: none;
}
.readonly-val {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 13px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-component);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  padding: 0 10px;
}
.calc-val-lg {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-brand-color);
  font-variant-numeric: tabular-nums;
}
.field-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 4px;
  line-height: 1.4;
}
.calc-row {
  display: flex;
  align-items: center;
  gap: 16px;
  min-height: 30px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .calc-rate { color: var(--td-brand-color); }
  .calc-val { color: var(--td-text-color-primary); font-weight: 600; font-variant-numeric: tabular-nums; }
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 14px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 0 0 30%;
  max-width: 30%;
  padding-right: 16px;

  label {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;

    .required {
      color: var(--td-error-color);
    }
  }

  .desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex: 1;
  min-width: 0;
}

.reading-row {
  display: flex;
  align-items: center;
  gap: 10px;

  .reading-input {
    flex: 1;
  }

  .reading-sep {
    color: var(--td-text-color-placeholder);
  }
}

.calc-row {
  display: flex;
  gap: 24px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--td-bg-color-container-hover);
  font-size: 13px;
  color: var(--td-text-color-primary);

  .calc-val {
    font-size: 15px;
    color: var(--td-brand-color);
    font-family: var(--app-font-family);
  }
}

.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 用途管理 */
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
.kind-manage-row .t-input {
  flex: 1;
}
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

/* 水表配置抽屉 */
.meter-search {
  margin-bottom: 12px;
}
.meter-settings-body {
  padding: 4px 0 24px;
}

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

/* 分组标签:复用租户核算设置标签卡样式(选中绿色下划线) */
.meter-settings-tabs {
  margin-bottom: 10px;

  :deep(.t-tabs__header) {
    margin-bottom: 8px;
  }

  :deep(.t-tabs__nav-item) {
    font-size: 13px;
  }

  /* 下划线贴合文字:消除 wrapper 左右内边距/外边距,标签间保留间距 */
  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0;
    margin: 0;
  }

  :deep(.t-tabs__nav-item:not(:first-child) .t-tabs__nav-item-wrapper) {
    margin-left: 8px;
  }

  :deep(.t-tabs__content) {
    overflow: visible;
  }
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

      .required {
        color: var(--td-error-color);
      }
    }

    &--full {
      margin-top: 12px;
    }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }
}

</style>
