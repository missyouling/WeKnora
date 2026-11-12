<!--
  人事管理（第 45 轮）：花名册 / 社保管理 / 公积金管理 单页三卡片。

  骨架 100% 照抄 FleetRecordList.vue（日常事务中台）：
    ① 顶部 DashboardKpiGroup（恰好 3 张 type 卡，无任何汇总 / 历史卡）；
    ② 中部 BusinessFilterBar（#batch-actions 浮动条 + #right-extra 业务按钮）；
    ③ 下部 doc-list-scroll > doc-list-view > t-table（勾选感知合计行）+ 各类抽屉。

  表头首帧一次成型：切换 tab / 二级视图时先把 headerReady 置 false 撤掉表格，
  await Promise.all([字段配置, 列表数据]) 后一次性置 true —— 绝不先渲染
  「只有复选框」的半套列（第 39 轮门禁）。
-->
<template>
  <div class="biz-page hr-page" :data-hr-tab="activeTab">
    <!-- ① 顶部 KPI：3 张卡，包裹 div 带 data-hr-kpi-group 便于真机断言 -->
    <div class="overview-group" data-hr-kpi-group>
      <DashboardKpiGroup :loading="false" :skeleton="false" :cards="kpiCards" :active-type="activeTab"
        @card-click="onCardClick" />
    </div>

    <div class="biz-list-view">
      <!-- 公积金二级切换：缴存台账 / 月度账单（账单严格倒序） -->
      <div v-if="activeTab === 'hr-fund'" class="hr-subview-bar">
        <t-radio-group v-model="fundSubView" size="small" @change="onFundSubViewChange">
          <t-radio-button value="ledger">缴存台账</t-radio-button>
          <t-radio-button value="bills">月度账单</t-radio-button>
        </t-radio-group>
        <span v-if="fundSettings" class="hr-unit-config">
          单位：{{ fundSettings.unit_name || '—' }}
          · 账号 {{ fundSettings.unit_account || '—' }}
          · 默认比例 {{ fundSettings.default_ratio ?? 6 }}%
        </span>
      </div>

      <!-- ② 筛选工具栏 -->
      <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
        :columns="columnDefsForFilter" :visible-keys="visibleColKeys" :selected-count="selectedKeys.size"
        settings-text="设置" :primary-action="primaryAction" @columns-change="onColumnsChange"
        @reset-columns="resetColumns" @select-all-columns="selectAllColumns" @filter-change="applyFilter"
        @settings="settingsVisible = true" @primary="onPrimaryAction" @clear-selection="clearSelection">
        <!-- 第 52 轮：花名册工具栏只保留「搜索全部字段 + 部门 + 刷新」。
             列表固定只查询在职人员，人员状态 / 在职状态筛选框已彻底移除。 -->
        <template #type-extra>
          <t-select v-if="activeTab === 'hr-roster'" v-model="rosterDeptFilter"
            :options="rosterDeptSelectOptions" placeholder="部门" clearable
            class="hr-status-select" @change="onRosterDeptChange" />
          <!-- 第 47 轮：社保记录类型（原始 / 汇总）+ 账期 -->
          <template v-if="activeTab === 'hr-social'">
            <t-select v-model="siRecordView" :options="SI_RECORD_OPTIONS" placeholder="记录类型"
              class="hr-status-select" @change="onSiRecordViewChange" />
            <t-date-picker v-model="siMonthFilter" mode="month" format="YYYY-MM" value-type="YYYY-MM"
              placeholder="账期" clearable class="hr-month-picker" @change="onSiMonthChange" />
          </template>
          <!-- 第 49 轮：劳动合同先部门、后合同状态 -->
          <t-select v-if="activeTab === 'hr-contract'" v-model="rosterDeptFilter"
            :options="rosterDeptSelectOptions" placeholder="部门" clearable
            class="hr-status-select" @change="onContractFilterChange" />
          <t-select v-if="activeTab === 'hr-contract'" v-model="contractStatusView"
            :options="CONTRACT_STATUS_OPTIONS" placeholder="合同状态" class="hr-status-select"
            @change="onContractStatusChange" />
        </template>
        <template #batch-actions>
          <!-- display:contents 让按钮仍是 .batch-bar-actions 的直接 flex 项，不额外产生布局层 -->
          <div class="hr-batch-actions" data-hr-batch-bar>
            <t-button theme="default" variant="outline" size="small" @click="handleExportExcel">
              <template #icon><t-icon name="download" size="14px" /></template>
              导出 Excel
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="printBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印清单
            </t-button>
            <!-- 公积金：封存 / 启封 -->
            <template v-if="activeTab === 'hr-fund' && fundSubView === 'ledger'">
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleSeal(true)">
                <template #icon><t-icon name="lock-on" size="14px" /></template>
                封存
              </t-button>
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleSeal(false)">
                <template #icon><t-icon name="lock-off" size="14px" /></template>
                启封
              </t-button>
            </template>
            <!-- 花名册：办理离职 / 恢复在职；离职管理：恢复在职 -->
            <template v-if="activeTab === 'hr-roster' || activeTab === 'hr-resign'">
              <t-button v-if="activeTab === 'hr-roster'" theme="default" variant="outline" size="small"
                :disabled="!selectedRowKeys.length" @click="handleResign">
                <template #icon><t-icon name="logout" size="14px" /></template>
                办理离职
              </t-button>
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleRestore">
                <template #icon><t-icon name="rollback" size="14px" /></template>
                恢复在职
              </t-button>
            </template>
            <!-- 第 47 轮：社保原始记录 → 按账期生成月度汇总 -->
            <t-button v-if="activeTab === 'hr-social' && siRecordView === 'raw'"
              theme="primary" variant="outline" size="small" @click="handleGenerateSummary">
              <template #icon><t-icon name="calculation" size="14px" /></template>
              生成汇总
            </t-button>
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedRowKeys.length} 条记录吗？删除后不可恢复。`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleBatchDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                批量删除
              </t-button>
            </t-popconfirm>
          </div>
        </template>

        <!-- 第 47 轮：右侧固定 4 按钮 = 字段 | 设置 | 导入 | 新增。
             「导入」必须排在「设置」之后，故走 BusinessFilterBar 的 after-settings 插槽。 -->
        <template #after-settings>
          <t-button v-if="activeTab !== 'hr-fund'" theme="default" variant="outline" size="small"
            @click="onImportClick">
            <template #icon><t-icon name="upload" size="14px" /></template>
            导入
          </t-button>
        </template>

        <!-- 业务专属按钮：#right-extra（不进批量浮条，常驻工具栏右侧） -->
        <template #right-extra>
          <template v-if="activeTab === 'hr-social'">
            <t-button theme="default" variant="outline" size="small" @click="handleExportExcel">
              <template #icon><t-icon name="download" size="14px" /></template>
              导出
            </t-button>
          </template>
          <t-button v-if="activeTab === 'hr-fund'" theme="default" variant="outline" size="small"
            @click="handleGenerateBill">
            <template #icon><t-icon name="file-add" size="14px" /></template>
            生成月度账单
          </t-button>
        </template>
      </BusinessFilterBar>

      <!-- ③ 列表 -->
      <div class="doc-list-scroll" :data-hr-row-count="rows.length">
        <!-- 表头就绪前只渲染占位块：绝不先渲染半套列 -->
        <div v-if="!headerReady" class="doc-list-view">
          <div class="hr-empty-placeholder" />
        </div>
        <div v-else :class="listViewClass" :style="headerFixStyle">
          <t-table :data="rows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="tableLoading" :max-height="'100%'"
            :foot-data="tableFootData" :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header
            class="doc-table hr-table"
            :row-class-name="({ row }: any) => selectedKeys.has(hrRowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <!-- 4 个系统状态列：**只读彩色 Tag**（具名插槽 #<colKey>）。
                 TDesign 1.19 的 cell 回调不注入 h，用 cell+h(Tag) 会渲染成空白；
                 同一个插槽还会被复用于表头，此时必须输出纯文本列标题，
                 否则表头会变成「—」或一枚彩色 Tag。 -->
            <template #在职状态="{ row }: any">
              <span v-if="isHeaderContext(row)">在职状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '在职状态'))" variant="light" size="small">
                {{ colValue(row, '在职状态') }}
              </t-tag>
            </template>
            <template #账期状态="{ row }: any">
              <span v-if="isHeaderContext(row)">账期状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '账期状态'))" variant="light" size="small">
                {{ colValue(row, '账期状态') }}
              </t-tag>
            </template>
            <template #参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">参保状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '参保状态'))" variant="light" size="small">
                {{ colValue(row, '参保状态') }}
              </t-tag>
            </template>
            <template #缴存状态="{ row }: any">
              <span v-if="isHeaderContext(row)">缴存状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '缴存状态'))" variant="light" size="small">
                {{ colValue(row, '缴存状态') }}
              </t-tag>
            </template>
            <!-- 账期 / 账单月份：合法 YYYY-MM 的单元格挂 data-hr-month-cell 便于真机断言倒序 -->
            <template #账期="{ row }: any">
              <span v-if="isHeaderContext(row)">账期</span>
              <span v-else-if="isHrMonth(colValue(row, '账期'))" :data-hr-month-cell="colValue(row, '账期')">
                {{ colValue(row, '账期') }}
              </span>
              <span v-else>{{ colValue(row, '账期') }}</span>
            </template>
            <template #月份="{ row }: any">
              <span v-if="isHeaderContext(row)">月份</span>
              <span v-else-if="isHrMonth(colValue(row, '月份'))" :data-hr-month-cell="colValue(row, '月份')">
                {{ colValue(row, '月份') }}
              </span>
              <span v-else>{{ colValue(row, '月份') }}</span>
            </template>
            <!-- 离职证明：有附件显示可点击查看链接，无则 —。
                 表头上下文必须输出纯文本列标题（TDesign 会复用该插槽渲染 th）。 -->
            <template #离职证明="{ row }: any">
              <span v-if="isHeaderContext(row)">离职证明</span>
              <t-link v-else-if="proofPathOf(row)" theme="primary" @click.stop="previewProof(row)">查看</t-link>
              <span v-else>—</span>
            </template>
            <!-- 第 47 轮：合同状态三色 Tag（正常绿 / 即将到期橙 / 到期红） -->
            <template #合同状态="{ row }: any">
              <span v-if="isHeaderContext(row)">合同状态</span>
              <t-tag v-else-if="contractStatusOfRow(row)" :theme="contractStatusTheme(contractStatusOfRow(row))"
                variant="light" size="small">
                {{ contractStatusOfRow(row) }}
              </t-tag>
              <span v-else>—</span>
            </template>
            <!-- 社保原始记录：记录性质 Tag（正常 / 异常补退） -->
            <template #记录性质="{ row }: any">
              <span v-if="isHeaderContext(row)">记录性质</span>
              <t-tag v-else :theme="String(colValue(row, '记录性质')).includes('异常') ? 'warning' : 'success'"
                variant="light" size="small">
                {{ colValue(row, '记录性质') }}
              </t-tag>
            </template>
            <template #empty>
              <BusinessEmptyState v-if="!tableLoading" compact :icon="emptyIcon"
                :description="emptyText" :hint="emptyHint" />
              <div v-else class="hr-empty-placeholder" />
            </template>
          </t-table>
        </div>
      </div>
    </div>

    <!-- 详情抽屉：深拷贝草稿 + 脏检查只标脏不自动保存，必须点「保存」才调 API -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="detailRow"
      :page-no="0" :form="editForm" :fields="drawerFields"
      :groups="drawerGroups" :sub-fields="[]"
      :supports-sub-fields="false" :dirty="detailDirty" :saving="saving"
      storage-key="weknora-hr-detail-width" :show-preview="false"
      @save="saveDetail" @delete="onDetailDelete">
      <!-- 第 48 轮：部门 / 职务变更后重算工号建议值（用户手改过则不覆盖） -->
      <template #field-部门>
        <t-select v-model="editForm.data['部门']" clearable filterable placeholder="选择部门"
          @change="onDeptOrTitleChange">
          <t-option v-for="d in rosterDeptOptions" :key="d" :label="d" :value="d" />
        </t-select>
      </template>
      <template #field-职务>
        <t-input v-model="editForm.data['职务']" placeholder="职务" @change="onDeptOrTitleChange" />
      </template>
      <!-- 第 52 轮：离职证明附件上传 / 预览（只写草稿路径，保存才落库） -->
      <template #field-离职证明>
        <div class="hr-proof-field">
          <t-input :value="String(editForm.data['离职证明'] || '')" readonly placeholder="上传后自动填入" />
          <t-button variant="outline" size="small" :loading="proofUploading" @click="pickProof">上传</t-button>
          <t-link v-if="editForm.data['离职证明']" theme="primary"
            @click.stop="previewProof({ 离职证明: editForm.data['离职证明'] })">查看</t-link>
          <input ref="proofInputRef" type="file" hidden accept=".pdf,.jpg,.jpeg,.png"
            @change="onProofFileChange" />
        </div>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 第 52 轮：100% 复用中台 StandardSettingDrawer + FleetCertPanel。
         花名册取消胶囊分组（字段仍按 6 组顺序连续展示，group 供编辑抽屉分块）；
         离职管理 / 劳动合同复用 FleetCertPanel 既有的跨表字段搜索引用机制。 -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="''" :scope="hrScopeOf(activeTab)"
      :initial-category="initialCategory" :title="settingsTitle"
      fields-tab-label="字段设置"
      :hide-group-tabs="activeTab === 'hr-roster'"
      :ref-sources="refSources"
      storage-key="weknora-hr-settings-drawer-width" :show-rules="false"
      @saved="onSettingsSaved" />

    <!-- 社保账期：个人 / 单位扣款明细（点行打开） -->
    <t-drawer v-if="periodDetailVisible" :visible="true"
      :header="`账期 ${periodDetail?.year_month || ''} 扣款明细`" size="720px" :footer="false"
      @close="periodDetailVisible = false">
      <div class="hr-charge-group">
        <div class="hr-charge-title">个人扣款明细</div>
        <t-table :data="personalCharges" :columns="chargeColumns" row-key="id" size="small" bordered
          :empty="'暂无个人扣款明细'" />
      </div>
      <div class="hr-charge-group">
        <div class="hr-charge-title">单位扣款明细</div>
        <t-table :data="unitCharges" :columns="chargeColumns" row-key="id" size="small" bordered
          :empty="'暂无单位扣款明细'" />
      </div>
    </t-drawer>

    <!-- 打印预览（统一中台组件） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="printTitle" :url="printUrl" :data="printData"
      :download-name="`${printTitle}.pdf`" @close="closePrint" />

    <!-- 第 47 轮：社保多文件自适应导入（正常 + 异常一起选） -->
    <input ref="siFileInputRef" type="file" multiple accept=".xlsx,.xls,.csv" class="hr-file-input"
      @change="onSiFilesChange" />

    <!-- 第 47 轮：社保月度费用清单抽屉 -->
    <t-drawer v-model:visible="siDetailVisible" :header="siDetailTitle" size="90%" placement="right"
      :footer="false" class="hr-si-detail-drawer">
      <div class="hr-si-detail">
        <div class="hr-si-filters">
          <t-input v-model="siDetailKeyword" placeholder="搜索姓名 / 身份证 / 工号" clearable
            class="hr-si-search">
            <template #prefixIcon><t-icon name="search" size="16px" /></template>
          </t-input>
          <t-select v-model="siDetailDept" placeholder="全部部门" clearable class="hr-si-dept">
            <t-option v-for="d in siDetailDepts" :key="d" :label="d" :value="d" />
          </t-select>
          <t-select v-model="siDetailScheme" :options="SI_SCHEME_OPTIONS" placeholder="全部险种"
            class="hr-si-scheme" />
          <span class="hr-si-count">共 {{ siDetailFiltered.length }} 人</span>
        </div>
        <t-table :data="siDetailFiltered" :columns="siDetailColumns" row-key="id" size="small"
          :loading="siDetailLoading" :max-height="'100%'" :foot-data="siDetailFoot"
          :rowspan-and-colspan-in-footer="siDetailFootSpan" class="hr-si-table" />
      </div>
    </t-drawer>

    <!-- 第 52 轮：三套独立 Excel 导入模板（花名册 / 离职管理 / 劳动合同）共用弹窗 -->
    <RosterImportDialog v-model:visible="rosterImportVisible" :module="importModule" @imported="onRosterImported" />
    <ResignEmployeeDialog v-model:visible="resignVisible" :ids="selectedRowKeys" @done="onResignDone" />

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessEmptyState from './BusinessEmptyState.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import { listFleetCategories } from '@/api/fleet'
import { useTableFlatState, flatClassOf } from './useTableFlatState'
import { useTableHeaderFix } from './useTableHeaderFix'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import { buildFootRow, footSpanOf, summaryScopeRows, type SummaryColumn } from './listSummary'
import './styles/list-summary-footer.css'
import { generateCatalogPdf } from './useCatalogPdf'
import { grouped } from './formatNumber'
import {
  HR_TABS, buildHrKpiCards, hrScopeOf, hrColumnDefs, hrBillColumnDefs,
  sortMonthsDesc, isHrMonth, hrFieldSeeds, hrRowKey,
  hrRosterGroupColumns, hrResignColumns, hrContractColumns, hrColumnsFromSubs,
  hrSocialColumns,
  contractStatusOf,
  contractStatusTheme,
  CONTRACT_STATUS_OPTIONS,
} from './hrCore'
import type { HrTab, HrColumn } from './hrCore'
import { drawerGroupsFor, type RosterStatusFilter } from './hrRosterGroups'
import {
  hrModuleGroups, hrModuleFields, hrEnumsOf, hrDataTypeOf, hrParseIdCard,
  hrSeniorityOf, hrRegularDateOf, hrCleanForm, hrValidateForm, hrCleanValue,
  HR_DERIVED_FIELDS,
  type HrModule,
} from './hrFieldSchema'
import RosterImportDialog from './RosterImportDialog.vue'
import ResignEmployeeDialog from './ResignEmployeeDialog.vue'
import * as hrApi from './hrApi'
import type { FilterFieldSpec } from './businessFilter'

// ---------------------------------------------------------------------------
// 基础状态
// ---------------------------------------------------------------------------
const activeTab = ref<HrTab>('hr-roster')
const fundSubView = ref<'ledger' | 'bills'>('ledger')
/** 表头门禁：false 时不渲染 t-table，保证第 1 帧列数一次成型。 */
const headerReady = ref(false)
const loading = ref(false)
const rows = ref<any[]>([])
const selectedKeys = ref<Set<string>>(new Set())
const selectedRowKeys = computed(() => Array.from(selectedKeys.value))
const filterState = ref<Record<string, any>>({ keyword: '' })
const kpiCounts = ref<Record<string, number>>({})
const fundSettings = ref<any>(null)
const fieldConfig = ref<Record<string, any[]>>({})
/**
 * 第 52 轮：花名册列表固定只查询在职人员，状态筛选框已从工具栏移除。
 * 保留该状态仅为兼容既有列显隐/抽屉派生函数，恒为「在职」且不再对外暴露修改入口。
 */
const rosterStatusView = ref<RosterStatusFilter>('在职')

/**
 * 人事 5 个列表的分类名 + scope（第 53 轮：互相引用时的候选目录）。
 * 与后端 hrCategorySeeds 的分类名严格一致，否则 SourceFieldPicker 取不到 subs。
 */
const HR_REF_CATALOG: readonly { scope: string; category: string }[] = [
  { scope: 'hr-roster', category: '花名册' },
  { scope: 'hr-resign', category: '离职管理' },
  { scope: 'hr-contract', category: '劳动合同' },
  { scope: 'hr-social', category: '社保管理' },
  { scope: 'hr-fund', category: '公积金管理' },
]
/** 当前 Tab 的候选引用源：除自己之外的人事分类（避免把本表明字段再引一遍）。 */
const refSources = computed(() =>
  HR_REF_CATALOG.filter((s) => s.scope !== hrScopeOf(activeTab.value)),
)

/** 当前导入弹窗对应的模板模块（社保走多文件自适应，不走本弹窗）。 */
const importModule = computed<HrModule>(() =>
  activeTab.value === 'hr-resign' ? 'resign'
    : activeTab.value === 'hr-contract' ? 'contract' : 'roster')

/** 组织机构单位名（合同公司 / 签约公司下拉数据源）。 */
const rosterUnitOptions = ref<string[]>([])
/** 第 47 轮：社保记录类型（原始记录 / 汇总记录）与劳动合同状态筛选。 */
const siRecordView = ref<'raw' | 'summary'>('raw')
const contractStatusView = ref<string>('全部')
/** 第 48 轮：花名册部门筛选（空 = 全部部门）。 */
const rosterDeptFilter = ref<string>('')
const rosterDeptOptions = ref<string[]>([])
const siMonthFilter = ref<string>('')
/** 社保月度费用清单抽屉（点汇总行打开）。 */
const siDetailVisible = ref(false)
const siDetailLoading = ref(false)
const siDetailItems = ref<any[]>([])
const siDetailDepts = ref<string[]>([])
const siDetailTitle = ref('')
const siDetailKeyword = ref('')
const siDetailDept = ref('')
const siDetailScheme = ref('全部')

const SI_RECORD_OPTIONS = [
  { label: '原始记录', value: 'raw' },
  { label: '汇总记录', value: 'summary' },
]

/** 险种筛选项（抽屉内）。 */
const SI_SCHEME_OPTIONS = [
  { label: '全部险种', value: '全部' },
  { label: '养老', value: '养老' },
  { label: '医疗', value: '医疗' },
  { label: '大病', value: '大病' },
  { label: '失业', value: '失业' },
  { label: '工伤', value: '工伤' },
]

/** 合同状态：优先取后端派生的 custom_data.合同状态，否则前端现算。 */
function contractStatusOfRow(row: any): string {
  const v = row?.合同状态 ?? row?.custom_data?.['合同状态'] ?? row?.contract_status
  if (v) return String(v)
  const pickField = (...keys: string[]) => {
    for (const k of keys) {
      const val = row?.[k] ?? row?.custom_data?.[k] ?? row?.data?.[k]
      if (val !== undefined && val !== null && val !== '') return val
    }
    return ''
  }
  // 新字段名优先，兼容存量「现合同…」键
  return contractStatusOf(
    pickField('合同结束日期', '现合同结束日期'),
    pickField('合同类型', '现合同类型'),
  )
}

/** 抽屉明细按搜索 / 部门 / 险种过滤。 */
const siDetailFiltered = computed(() => {
  const kw = siDetailKeyword.value.trim().toLowerCase()
  return siDetailItems.value.filter((r) => {
    if (siDetailDept.value && String(r['部门'] || '') !== siDetailDept.value) return false
    if (siDetailScheme.value !== '全部') {
      const scheme = String(siDetailScheme.value)
      // 险种筛选：该险种单位/个人任一非 0 即保留
      const key = { 养老: ['养老单位', '养老个人'], 医疗: ['医疗单位', '医疗个人'], 大病: ['大病'],
        失业: ['失业单位', '失业个人'], 工伤: ['工伤单位'] }[scheme] || []
      if (!key.some((k) => Number(r[k] || 0) !== 0)) return false
    }
    if (!kw) return true
    return [r['姓名'], r['证件号码'], r['工号'], r['个人社保编号']]
      .some((v) => String(v || '').toLowerCase().includes(kw))
  })
})
const rosterImportVisible = ref(false)
const resignVisible = ref(false)

const tabLabel = computed(() => HR_TABS.find((t) => t.key === activeTab.value)?.label || '人事管理')
const settingsTitle = computed(() => `${tabLabel.value}设置`)
const initialCategory = computed(() => tabLabel.value)
// 第 47 轮：抽屉标题用「员工详情 / 合同详情」，不再拼「花名册详情」，
// 且 BusinessSplitDetailDrawer 已不再渲染 [P1] 页码徽标（人事模块无 PDF 分页）。
const detailTitle = computed(() => {
  const name = String(detailRow.value?.姓名 || detailRow.value?.name || '').trim()
  const base = !detailRow.value ? '新增员工'
    : activeTab.value === 'hr-contract' ? '合同详情'
      : activeTab.value === 'hr-resign' ? '离职详情' : '员工详情'
  return name ? `${base} · ${name}` : base
})
const printTitle = computed(() => `${tabLabel.value}清单`)

const primaryAction = computed(() => {
  // 文案统一精简为「新增」；劳动合同视图的「新增」跳回花名册新增员工（合同随人走）
  if (activeTab.value === 'hr-social') return { text: '新增', icon: 'add' }
  if (activeTab.value === 'hr-fund') return { text: '新增', icon: 'add' }
  return { text: '新增', icon: 'add' }
})

const kpiCards = computed(() => buildHrKpiCards(kpiCounts.value))

// ---------------------------------------------------------------------------
// 列定义 / 列显隐
// ---------------------------------------------------------------------------
/** 当前 Tab 的字段配置 subs（六同步唯一权威源）。 */
function subsOfTab(tab: HrTab): any[] {
  const cats = fieldConfig.value[tab]
  const cat = Array.isArray(cats)
    ? (cats.find((c: any) => String(c?.builtin_key ?? '') === hrScopeOf(tab)) || cats[0])
    : null
  return Array.isArray(cat?.subs) ? cat.subs : []
}

/**
 * 第 52 轮：花名册 / 离职管理 / 劳动合同三个模块的列全集全部来自
 * 「设置 → 字段设置」的 subs；用户在设置里新增的跨模块引用字段会立刻
 * 出现在列表表头，实现六同步。subs 未就绪时回退字段契约派生列。
 */
const currentColumns = computed<HrColumn[]>(() => {
  if (activeTab.value === 'hr-roster') {
    return hrColumnsFromSubs(subsOfTab('hr-roster'), hrRosterGroupColumns())
  }
  if (activeTab.value === 'hr-resign') {
    return hrColumnsFromSubs(subsOfTab('hr-resign'), hrResignColumns())
  }
  if (activeTab.value === 'hr-contract') {
    return hrColumnsFromSubs(subsOfTab('hr-contract'), hrContractColumns())
  }
  // 社保按「记录类型」切换原始 / 汇总列集合（第 47 轮）。
  // 第 53 轮：原始记录 / 公积金台账是「人级行」，在固定列后追加用户在字段
  // 设置里新增或跨模块引用的字段；汇总/账单是账期聚合，追加人员字段只会空白。
  if (activeTab.value === 'hr-social') {
    const cols = hrSocialColumns(siRecordView.value)
    return siRecordView.value === 'raw' ? withExtraSubs(cols, subsOfTab('hr-social')) : cols
  }
  if (activeTab.value === 'hr-fund' && fundSubView.value === 'bills') return hrBillColumnDefs()
  if (activeTab.value === 'hr-fund') return withExtraSubs(hrColumnDefs('hr-fund'), subsOfTab('hr-fund'))
  return hrColumnDefs(activeTab.value)
})

/**
 * 固定列 + subs 里「固定列之外」的新增/引用字段。
 *
 * 社保与公积金的列表骨架是代码固定的（账期/险种/缴存基数…），不能整体改成
 * subs 派生（会把业务必需列冲掉）；但用户在字段设置里主动添加的字段必须
 * 能出列（六同步）。这里只追加差集，顺序排在固定列之后。
 */
function withExtraSubs(fixed: HrColumn[], subs: readonly any[]): HrColumn[] {
  const fixedNames = new Set(fixed.map((c) => c.colKey))
  const extra = hrColumnsFromSubs(subs, []).filter(
    (c) => c.colKey !== 'row-select' && !fixedNames.has(c.colKey),
  )
  return extra.length ? [...fixed, ...extra] : fixed
}
const businessColumns = computed(() => currentColumns.value.filter((c) => c.colKey !== 'row-select'))
/**
 * `[☰ 字段]` 可选全集：三个 HR 模块只能勾「启用字段」（后端 `enabled !== false`）。
 * 引用字段的 subs.name 是 `src_…`，列 colKey 是 label，故键集合以 label 优先。
 */
const configKeys = computed(() => {
  const subs = subsOfTab(activeTab.value)
  const nameOf = (s: any) => String(s?.label || s?.name || '').trim()
  return {
    defaults: subs.filter((s) => s && s.enabled !== false && s.is_default === true).map(nameOf).filter(Boolean),
    enabled: subs.filter((s) => s && s.enabled !== false).map(nameOf).filter(Boolean),
  }
})
const columnDefsForFilter = computed(() => {
  // 第 53 轮：5 个 HR 列表都能在「字段」筛选器里勾选新增/引用字段
  const isHrCfg = ['hr-roster', 'hr-resign', 'hr-contract', 'hr-social', 'hr-fund'].includes(activeTab.value)
  const enabled = isHrCfg && configKeys.value.enabled.length
    ? new Set(configKeys.value.enabled) : null
  return businessColumns.value
    .filter((c) => !enabled || enabled.has(c.colKey))
    .map((c) => ({ key: c.colKey, label: c.title }))
})

// 四态视图各自保存列显隐，切换状态时互不污染。
const STORAGE_KEY = computed(() => {
  const view = activeTab.value === 'hr-roster' ? rosterStatusView.value
    : activeTab.value === 'hr-social' ? siRecordView.value
      : activeTab.value === 'hr-contract' ? contractStatusView.value
        : 'all'
  // 第 52 轮字段大改（11 组 → 6 组、离职/合同剥离）：把存档版本升到 v2，
  // 让旧 localStorage 里含已删除字段的勾选自动失效，避免首屏仍显示旧表头。
  return `weknora-hr-${activeTab.value}-${fundSubView.value}-${view}-cols-v2`
})
const visibleColKeys = ref<string[]>([])
const visibleCols = computed(() => businessColumns.value.filter((c) => visibleColKeys.value.includes(c.colKey)))

function persistColumns() {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
/**
 * `[☰ 字段]` 的「重置」= 精确回到**后端** `defaultHeader` 字段集合。
 *
 * 严禁回全选或前端静态数组：用户可能刚在设置里关掉了几个默认表头，
 * 重置却把它们全勾回来，等于把上一次保存冲掉。
 */
function resetColumns() {
  const isHrCfg = ['hr-roster', 'hr-resign', 'hr-contract'].includes(activeTab.value)
  if (isHrCfg) {
    // 后端 subs 尚未加载时也不能回退成「全列」：字段契约里带 defaultHeader
    // 的字段就是静态默认表头，与后端 seed 同源（六同步首帧不闪全表）。
    const defaults = configKeys.value.defaults.length
      ? configKeys.value.defaults
      : hrModuleFields(activeModule.value || 'roster')
        .filter((d) => d.defaultHeader)
        .map((d) => d.name)
    visibleColKeys.value = defaults.filter((k) => businessColumns.value.some((c) => c.colKey === k))
  } else {
    visibleColKeys.value = businessColumns.value.map((c) => c.colKey)
  }
  persistColumns()
}
function selectAllColumns() { resetColumns() }
function onColumnsChange(keys: string[]) { visibleColKeys.value = keys; persistColumns() }
function initColumns() {
  const allKeys = businessColumns.value.map((c) => c.colKey)
  const isHrCfg = ['hr-roster', 'hr-resign', 'hr-contract'].includes(activeTab.value)
  const saved = localStorage.getItem(STORAGE_KEY.value)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      const matched = allKeys.filter((k) => Array.isArray(arr) && arr.includes(k))
      // 第 52 轮：旧存档可能含已删除分组/字段，只保留当前列全集里仍存在的键，
      // 且存档若已空（字段精简后全被过滤）就回落到后端默认表头。
      if (matched.length) { visibleColKeys.value = matched; return }
    } catch { /* ignore */ }
  }
  if (isHrCfg && configKeys.value.defaults.length) {
    visibleColKeys.value = configKeys.value.defaults.filter((k) => allKeys.includes(k))
    return
  }
  resetColumns()
}
watch(STORAGE_KEY, () => { initColumns() })

const tableColumns = computed(() => {
  const cols: any[] = []
  const vis = currentColumns.value.filter(
    (c) => c.colKey === 'row-select' || visibleColKeys.value.includes(c.colKey),
  )
  vis.forEach((c, i) => {
    // 勾选列：不带 fixed（第 38 轮起全站取消冻结列）
    if (c.colKey === 'row-select') {
      cols.push({ colKey: 'row-select', type: 'multiple', width: 44 })
      return
    }
      const values = rows.value.slice(0, 30).map((r) => c.value?.(r))
      const cfgWidth = Number(c.width) || 0
      const w = Math.max(colWidthOf(cfgWidth, c.title, values, c.dataType), Number(c.minWidth) || 0)
      const base: any = {
        colKey: c.colKey,
        title: c.title,
        // 用户显式配了列宽（>0）时关闭省略号：18 位身份证/19 位卡号要在 250px 里
        // 完整显示，不能再被 TEllipsis 截成 `5101111969122542...`。
        ellipsis: cfgWidth > 0 ? false : ellipsisOf(c),
        // 状态 / 月份列走具名插槽；其余用 cell 返回字符串（TDesign 1.19 不注入 h）
        cell: c.slot ? undefined : (_h: any, params: any) => c.value?.(params?.row) ?? '',
        dataType: c.dataType,
      }
      if (cfgWidth > 0) {
        // 第 53 轮硬性要求：配置了列宽就 width 与 minWidth 同时钉死，
        // 既不被自适应覆盖，也不被「最后一列拉伸」吃掉。
        base.width = cfgWidth
        base.minWidth = cfgWidth
      } else if (i === vis.length - 1) base.minWidth = w
      else base.width = w
    const cls = cellClassOf(c)
    if (cls) base.className = cls
    cols.push(base)
  })
  return cols
})
const tableScroll = computed(() => tableScrollOf(visibleCols.value))

// ---------------------------------------------------------------------------
// 合计行（勾选感知；loading 时不渲染）
// ---------------------------------------------------------------------------
// 从 currentColumns 派生，保证四态动态列 / 账单子视图与合计列严格同源，
// 不会出现「表头换了但合计列还是旧集合」的错位。
const summaryCols = computed<SummaryColumn[]>(() =>
  businessColumns.value
    .filter((c) => visibleColKeys.value.includes(c.colKey))
    .map((c) => ({
      key: c.colKey,
      label: c.title,
      fieldType: c.dataType === 'number' ? 'number' : 'text',
      value: (row: any) => c.value?.(row) ?? '',
    })),
)
const footSpan = computed(() => footSpanOf(summaryCols.value))
const summaryScope = computed(() => summaryScopeRows(rows.value, selectedKeys.value, hrRowKey))
const footData = computed(() => [buildFootRow(summaryCols.value, summaryScope.value.rows, {
  selectKey: 'row-select',
  selectedCount: summaryScope.value.selectedCount,
})])
const tableLoading = computed(() => loading.value)
const tableFootData = computed(() => (tableLoading.value ? [] : footData.value))

const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
])

// ---------------------------------------------------------------------------
// 展示辅助
// ---------------------------------------------------------------------------
/**
 * TDesign 1.19 会把 `#<colKey>` 具名插槽**复用于表头渲染**，此时 row 为空，
 * 若照常走取值函数，表头就会显示成占位符「—」（真机截图实测）。
 * 表头上下文一律返回列标题，数据行才取真实值。
 */
function isHeaderContext(row: any): boolean {
  return !row || (typeof row === 'object' && Object.keys(row).length === 0)
}
function colValue(row: any, colKey: string): string {
  const c = currentColumns.value.find((x) => x.colKey === colKey)
  if (!c) return '—'
  if (isHeaderContext(row)) return c.title
  return c.value?.(row) ?? '—'
}
function statusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '')
  if (/在职|正常|已缴|生效|已汇总|已完成|正常缴存|active/.test(s)) return 'success'
  if (/离职|封存|停缴|暂停|过期|异常|作废/.test(s)) return 'danger'
  if (/待|进行|处理中|未|pending/.test(s)) return 'warning'
  return 'default'
}

const emptyIcon = 'search-error'
const emptyText = computed(() => '暂无数据')
const emptyHint = computed(() =>
  activeTab.value === 'hr-social'
    ? '点击「新增账期」创建账期后，可导入五险数据并执行汇总'
    : '点击右上角「设置」可调整字段，或先导入数据',
)

// ---------------------------------------------------------------------------
// 数据加载 + headerReady 门禁
// ---------------------------------------------------------------------------
function extractItems(res: any): any[] {
  const d = res?.data
  if (Array.isArray(d)) return d
  if (Array.isArray(d?.items)) return d.items
  if (Array.isArray(d?.list)) return d.list
  if (Array.isArray(res)) return res
  return []
}

async function loadFieldConfig(tab: HrTab) {
  try {
    const res: any = await listFleetCategories({ scope: hrScopeOf(tab) })
    fieldConfig.value = { ...fieldConfig.value, [tab]: Array.isArray(res?.data) ? res.data : [] }
  } catch {
    // 分类接口未就绪时用同步种子兜底，绝不让门禁卡住表头
    fieldConfig.value = { ...fieldConfig.value, [tab]: hrFieldSeeds(tab) as any }
  }
}

async function loadList(tab: HrTab) {
  loading.value = true
  try {
    let items: any[] = []
    const kw = String(filterState.value?.keyword || '').trim() || undefined
    if (tab === 'hr-roster') {
      // 第 52 轮：花名册固定只查询在职人员（工具栏状态筛选框已移除）。
      const res: any = await hrApi.listRosterEmployees({
        q: kw,
        department: rosterDeptFilter.value || undefined,
        status: '在职',
      })
      items = extractItems(res)
    } else if (tab === 'hr-resign') {
      // 离职管理：离职人员 + 9 个离职字段（与花名册同一张 hr_employees 表）
      const res: any = await hrApi.listRosterEmployees({ q: kw, status: '离职' })
      items = extractItems(res)
    } else if (tab === 'hr-contract') {
      const res: any = await hrApi.listContracts({
        q: kw,
        department: rosterDeptFilter.value || undefined,
        contract_status: contractStatusView.value === '全部' ? undefined : contractStatusView.value,
      })
      items = extractItems(res)
    } else if (tab === 'hr-social') {
      if (siRecordView.value === 'raw') {
        // 原始记录：一行 = 某人某险种明细
        items = extractItems(await hrApi.listSocialRaw({ q: kw }))
      } else {
        // 汇总记录：一行 = 一个账期（后端 year_month DESC）
        items = sortMonthsDesc(extractItems(await hrApi.listSocialPeriods()), 'year_month')
      }
    } else if (fundSubView.value === 'bills') {
      items = sortMonthsDesc(extractItems(await hrApi.listFundBills()), 'month_label')
    } else {
      items = extractItems(await hrApi.listFundRecords())
    }
    // TDesign 的 row-key="id" 是列名，要求每行都有 id 字段。后端三个模块主键都是
    // string id；万一某视图缺 id，用 hrRowKey(row) 补一个，保证 TDesign 的选中键、
    // selectedKeys 与合计行的 hrRowKey 三者口径统一（都等于该行 id）。
    rows.value = items.map((r: any) => ({ ...(r || {}), id: r?.id ?? hrRowKey(r) }))
    // 第 48 轮：花名册卡片恒为模块总人数，不随搜索/部门/状态筛选变化。
    if (tab !== 'hr-roster') {
      kpiCounts.value = { ...kpiCounts.value, [tab]: items.length }
    }
  } catch (e: any) {
    rows.value = []
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadFundSettings() {
  try {
    const res: any = await hrApi.getFundSettings()
    fundSettings.value = res?.data || res || null
  } catch { fundSettings.value = null }
}

async function reloadStats() {
  try {
    const res: any = await hrApi.getRosterStats()
    const d = res?.data || {}
    kpiCounts.value = {
      'hr-roster': Number(d.total ?? d.roster ?? d.employees ?? kpiCounts.value['hr-roster'] ?? 0) || 0,
      'hr-resign': Number(d.resigned ?? d.resign ?? kpiCounts.value['hr-resign'] ?? 0) || 0,
      'hr-contract': Number(d.contract ?? kpiCounts.value['hr-contract'] ?? 0) || 0,
      'hr-social': Number(d.social ?? d.periods ?? kpiCounts.value['hr-social'] ?? 0) || 0,
      'hr-fund': Number(d.fund ?? d.records ?? kpiCounts.value['hr-fund'] ?? 0) || 0,
      // 第 49 轮：合同异常计数（供卡副文案）
      contract_expiring: Number(d.contract_expiring ?? 0) || 0,
      contract_expired: Number(d.contract_expired ?? 0) || 0,
    }
  } catch { /* 统计失败不阻塞列表 */ }
}

/**
 * 切换一级 Tab：**先撤表头 → 并行等字段配置与列表 → 一次性置 true**。
 * 这是「表头列数第 1 帧一次成型」的硬保证。
 */
async function switchTab(tab: HrTab) {
  headerReady.value = false
  activeTab.value = tab
  fundSubView.value = 'ledger'
  selectedKeys.value = new Set()
  rosterDeptFilter.value = ''
  filterState.value = { keyword: '' }
  rows.value = []
  initColumns()
  if (tab === 'hr-fund') void loadFundSettings()
  await Promise.all([loadFieldConfig(tab), loadList(tab)])
  headerReady.value = true
}

function onCardClick(card: any) {
  if (card.action !== 'type' || !card.typeValue) return
  if (activeTab.value === card.typeValue) return
  void switchTab(card.typeValue as HrTab)
}

/**
 * 第 46 轮：切换「在职状态」→ 先撤表头，再一次性换成该状态的专属列集合，
 * 并按状态重新拉列表（后端 status 过滤）。保证表头第 1 帧一次成型。
 */
/** 第 47 轮：统一「导入」入口（花名册 / 劳动合同走花名册模板，社保走多文件自适应）。 */
function onImportClick() {
  if (activeTab.value === 'hr-social') {
    siFileInputRef.value?.click()
    return
  }
  rosterImportVisible.value = true
}

/** 社保记录类型切换：先撤表头再换列集合，保证首帧一次成型。 */
async function onSiRecordViewChange(v: any) {
  siRecordView.value = (v as 'raw' | 'summary') || 'raw'
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await loadList('hr-social')
  headerReady.value = true
}

async function onSiMonthChange() {
  await loadList('hr-social')
}

async function onContractFilterChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  await loadList('hr-contract')
  headerReady.value = true
}

async function onContractStatusChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await loadList('hr-contract')
  headerReady.value = true
}

const siFileInputRef = ref<HTMLInputElement | null>(null)

/** 社保多文件自适应导入：正常 + 异常一起选，后端按文件名推断险种/所属期。 */
async function onSiFilesChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  if (!files.length) return
  const fd = new FormData()
  for (const f of files) fd.append('files', f)
  if (siMonthFilter.value) fd.append('year_month', siMonthFilter.value)
  loading.value = true
  try {
    const res: any = await hrApi.importSocialBatch(fd)
    const d = res?.data || res || {}
    const rows = Number(d.rows || 0)
    const okFiles = (Array.isArray(d.files) ? d.files : []).filter((f: any) => f.status === 'ok').length
    MessagePlugin.success(`已导入 ${okFiles} 个文件 / ${rows} 条原始记录`)
    const skipped = (Array.isArray(d.files) ? d.files : []).filter((f: any) => f.status !== 'ok')
    if (skipped.length) {
      MessagePlugin.warning(`${skipped.length} 个文件未导入：${skipped[0]?.message || ''}`)
    }
    siRecordView.value = 'raw'
    await onSiRecordViewChange('raw')
    await reloadStats()
  } catch (err: any) {
    MessagePlugin.error(err?.message || '导入失败')
  } finally {
    loading.value = false
  }
}

/** 月度费用清单抽屉列（一行 = 一名员工的各险种合计）。 */
const siDetailColumns = [
  { colKey: '工号', title: '工号', width: 120, ellipsis: true },
  { colKey: '姓名', title: '姓名', width: 90 },
  { colKey: '部门', title: '部门', width: 110, ellipsis: true },
  { colKey: '缴费基数', title: '缴费基数', width: 100, align: 'right' as const },
  { colKey: '养老单位', title: '养老单位', width: 100, align: 'right' as const },
  { colKey: '养老个人', title: '养老个人', width: 100, align: 'right' as const },
  { colKey: '医疗单位', title: '医疗单位', width: 100, align: 'right' as const },
  { colKey: '医疗个人', title: '医疗个人', width: 100, align: 'right' as const },
  { colKey: '大病', title: '大病', width: 80, align: 'right' as const },
  { colKey: '失业单位', title: '失业单位', width: 100, align: 'right' as const },
  { colKey: '失业个人', title: '失业个人', width: 100, align: 'right' as const },
  { colKey: '工伤单位', title: '工伤单位', width: 100, align: 'right' as const },
  { colKey: '补退金额', title: '补退', width: 90, align: 'right' as const },
  { colKey: '个人小计', title: '个人小计', width: 100, align: 'right' as const },
  { colKey: '单位小计', title: '单位小计', width: 100, align: 'right' as const },
  { colKey: '总计', title: '总计', width: 100, align: 'right' as const },
]

/** 抽屉合计行跨列：首列跨满，其余返回空（与中台 footSpanOf 同口径）。 */
const siDetailFootSpan = (data: any) => {
  if (!data) return {}
  return { 工号: [1, siDetailColumns.length] }
}

/** 抽屉底部合计行（金额按分累加，避免浮点残留）。 */
const siDetailFoot = computed(() => {
  const rows = siDetailFiltered.value
  if (!rows.length) return []
  const money = (k: string) => {
    let cents = 0
    for (const r of rows) cents += Math.round(Number(r[k] || 0) * 100)
    return grouped(cents / 100, { fixed: true, fallback: '0.00' })
  }
  const foot: Record<string, string> = { 工号: `合计（${rows.length} 人）`, 姓名: '', 部门: '', 缴费基数: '' }
  for (const c of siDetailColumns) {
    if (['工号', '姓名', '部门', '缴费基数'].includes(c.colKey)) continue
    foot[c.colKey] = money(c.colKey)
  }
  return [foot]
})

/** 勾选原始记录 → 按账期生成汇总（每月 1 条）。 */
async function handleGenerateSummary() {
  if (siRecordView.value !== 'raw') { MessagePlugin.warning('请切换到原始记录后再生成汇总'); return }
  const ids = selectedRowKeys.value
  try {
    const res: any = await hrApi.generateSocialSummary(
      ids.length ? { ids } : { year_month: siMonthFilter.value || undefined },
    )
    const n = Number(res?.data?.count ?? res?.count ?? 0) || 0
    MessagePlugin.success(`已生成 ${n} 条月度汇总`)
    clearSelection()
    siRecordView.value = 'summary'
    await onSiRecordViewChange('summary')
    await reloadStats()
  } catch (e: any) { MessagePlugin.error(e?.message || '生成汇总失败') }
}

/** 点汇总行 → 打开社保月度费用清单抽屉。 */
async function openSiDetail(row: any) {
  const id = row?.id
  if (!id) return
  siDetailTitle.value = `社保月度费用清单 · ${row?.账期 || row?.year_month || ''}`
  siDetailVisible.value = true
  siDetailLoading.value = true
  siDetailItems.value = []
  siDetailDepts.value = []
  siDetailKeyword.value = ''
  siDetailDept.value = ''
  siDetailScheme.value = '全部'
  try {
    const res: any = await hrApi.getSocialPeriodDetail(id)
    const d = res?.data || res || {}
    siDetailItems.value = Array.isArray(d.items) ? d.items : []
    siDetailDepts.value = Array.isArray(d.departments) ? d.departments.filter(Boolean) : []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载明细失败')
  } finally {
    siDetailLoading.value = false
  }
}

/** 公积金二级切换同样走门禁，避免列集换了一半先渲染一帧。 */
async function onFundSubViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await Promise.all([loadFieldConfig(activeTab.value), loadList(activeTab.value)])
  headerReady.value = true
}

let filterTimer: ReturnType<typeof setTimeout> | null = null
let filterWatchReady = false
watch(filterState, () => {
  if (!filterWatchReady) return
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => { filterTimer = null; void loadList(activeTab.value) }, 300)
}, { deep: true })
function applyFilter() { void loadList(activeTab.value) }

function onTableSelectChange(val: any) {
  const arr = Array.isArray(val) ? val : []
  selectedKeys.value = new Set(arr.map(String))
}
function clearSelection() { selectedKeys.value = new Set() }
const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(hrRowKey(r))))

const { isTableFlat } = useTableFlatState({ loading, rows })
const listViewClass = computed(() => flatClassOf(isTableFlat.value))
const { headerFixStyle } = useTableHeaderFix()

// ---------------------------------------------------------------------------
// 详情抽屉（深拷贝草稿 + 手存）
// ---------------------------------------------------------------------------
const detailVisible = ref(false)
const detailRow = ref<any>(null)
const detailDirty = ref(false)
const saving = ref(false)
const editForm = ref<any>({ data: {}, remarkParts: {}, items: [] })
/**
 * 最近一次由系统自动生成的工号。
 *
 * 用于区分「用户手改」与「系统建议值」：两者相等才允许随部门重算。
 * 每次打开抽屉必须重置（否则上一个员工的建议值会残留在下一个新增会话里）。
 */
let lastAutoEmployeeId = ''
const editBaseline = ref('')
/** 部门下拉选项（全部部门 + 组织机构部门名）。 */
const rosterDeptSelectOptions = computed(() => [
  { label: '全部部门', value: '' },
  ...rosterDeptOptions.value.map((d) => ({ label: d, value: d })),
])

/**
 * 部门下拉选项 = 组织机构里单位的部门拍平（第 50 轮）。
 *
 * 必须走 `/org/units` 而不是旧的 `/org`：旧接口是单公司视角，
 * 多单位场景下会漏挂单位、顺序也与组织机构面板不一致。
 * 顺序严格按「单位顺序 → 单位内部门顺序」，与设置里的拖拽结果一致。
 */
async function loadRosterDepts() {
  try {
    const res: any = await hrApi.getOrgUnits()
    const d = res?.data || res || {}
    const units = Array.isArray(d.units) ? d.units : []
    const seen = new Set<string>()
    const out: string[] = []
    const unitNames: string[] = []
    for (const u of units) {
      const un = String(u?.name || '').trim()
      if (un && !unitNames.includes(un)) unitNames.push(un)
      for (const dept of Array.isArray(u?.departments) ? u.departments : []) {
        const name = String(dept?.name || '').trim()
        if (!name || seen.has(name)) continue
        seen.add(name)
        out.push(name)
      }
    }
    rosterDeptOptions.value = out
    rosterUnitOptions.value = unitNames
  } catch {
    rosterDeptOptions.value = []
    rosterUnitOptions.value = []
  }
}

async function onRosterDeptChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await loadList('hr-roster')
  headerReady.value = true
}

/** 派生/系统字段：详情抽屉里只读（自动跟随身份证 / 入职日期 / 合同到期计算）。 */
const READONLY_DETAIL_FIELDS = new Set([
  ...HR_DERIVED_FIELDS,
  '在职状态', '员工状态', '账期状态', '参保状态', '缴存状态', '状态',
])

/** 当前抽屉对应的字段契约模块（社保/公积金没有 HR 字段契约，走单块渲染）。 */
const activeModule = computed<HrModule | null>(() => {
  if (activeTab.value === 'hr-roster') return 'roster'
  if (activeTab.value === 'hr-resign') return 'resign'
  if (activeTab.value === 'hr-contract') return 'contract'
  return null
})

/**
 * 第 52 轮：花名册抽屉严格按 6 大分组（在职信息 → 基本信息 → 联系信息 →
 * 紧急联系人情况 → 教育情况 → 工资社保信息）分块展示；字段顺序、显隐严格与
 * 「设置 → 字段设置」里启用的字段六同步。
 *
 * 控件全部复用 BusinessSplitDetailDrawer 的 dataType / options 分支：
 * 日期 → t-date-picker，内置枚举 → t-select filterable，只读派生 → disabled。
 */
const drawerGroups = computed(() => {
  const mod = activeModule.value
  if (!mod) return undefined
  const enabled = new Set(configKeys.value.enabled)
  const groups = mod === 'roster'
    ? drawerGroupsFor(rosterStatusView.value, enabled)
    : hrModuleGroups(mod).map((g) => ({
      key: g.key,
      label: g.label,
      fields: g.fields
        .filter((d) => enabled.size === 0 || enabled.has(d.name))
        .map((d) => d.name),
    }))
  return groups.filter((g) => g.fields.length).map((g) => ({
    key: g.key,
    label: g.label,
    fields: g.fields.map((name: string) => {
      const c = businessColumns.value.find((x) => x.colKey === name)
      const def = mod ? hrModuleFields(mod).find((d) => d.name === name) : undefined
      const staticOptions = hrEnumsOf(name)
      const options = def?.dynamic === 'department' ? rosterDeptOptions.value
        : def?.dynamic === 'unit' ? rosterUnitOptions.value
          : (staticOptions.length ? staticOptions : (def?.options || staticOptions))
      return {
        name,
        label: c?.title || name,
        // 日期一律 date（后端 subs 与字段契约双向对齐）
        dataType: def ? hrDataTypeOf(def) : (c?.dataType || 'text'),
        readonly: READONLY_DETAIL_FIELDS.has(name) || def?.type === 'system',
        ...(options.length ? { options } : {}),
      }
    }),
  }))
})

const drawerFields = computed(() => {
  if (drawerGroups.value?.length) return drawerGroups.value.flatMap((g) => g.fields)
  return businessColumns.value.map((c) => ({
    name: c.colKey,
    label: c.title,
    dataType: c.dataType,
    readonly: READONLY_DETAIL_FIELDS.has(c.colKey),
  }))
})

// ---------------------------------------------------------------------------
// 身份证自动推导 + 入职日期派生（第 52 轮）
// ---------------------------------------------------------------------------
// 只写草稿对象，绝不 watch(form) 实时写回业务行、更不自动调保存接口。
/** 清洗证件号 + 推导出生日期 / 性别 / 年龄（幂等，打开抽屉时也调用一次）。 */
function applyIdCardDerivation(): void {
  const data = editForm.value?.data
  if (!data) return
  const cleaned = hrCleanValue('证件号码', data['证件号码'])
  if (cleaned !== data['证件号码']) data['证件号码'] = cleaned
  const info = hrParseIdCard(cleaned)
  if (!info) return
  data['出生日期'] = info.birthDate
  data['性别'] = info.gender
  data['年龄'] = String(info.age)
}

/** 按入职日期推导司龄；转正日期仅在为空时按试用期推算（不覆盖手填）。 */
function applyHireDerivation(): void {
  const data = editForm.value?.data
  if (!data) return
  const hire = data['入职日期']
  if (!String(hire || '').trim()) return
  data['司龄'] = hrSeniorityOf(hire)
  if (data['试用期'] && !String(data['转正日期'] || '').trim()) {
    data['转正日期'] = hrRegularDateOf(hire, data['试用期'])
  }
}

watch(() => editForm.value?.data?.['证件号码'], () => {
  if (activeModule.value === 'roster') applyIdCardDerivation()
})
watch(
  () => [editForm.value?.data?.['入职日期'], editForm.value?.data?.['试用期']] as const,
  () => { if (activeModule.value === 'roster') applyHireDerivation() },
)

function openDetail(row: any) {
  // 社保汇总记录点行 → 打开月度费用清单抽屉（第 47 轮）
  if (activeTab.value === 'hr-social' && siRecordView.value === 'summary') { void openSiDetail(row); return }
  if (activeTab.value === 'hr-social') { void openPeriodDetail(row); return }
  detailRow.value = row
  // 第 53 轮：按**抽屉字段全集**灌值，而不是可见列（businessColumns）。
  // 用户把「部门」「职务」从表头隐藏后，旧实现草稿里没有这两个键，
  // 打开已有员工时部门显示为空、工号也无法随部门重算。
  const data: Record<string, any> = {}
  const fields = drawerFields.value.length ? drawerFields.value : businessColumns.value
  for (const f of fields) {
    const name = String((f as any).name || '')
    if (!name) continue
    const c = businessColumns.value.find((x) => x.colKey === name)
    const raw = c?.value?.(row)
      ?? row?.[name]
      ?? row?.custom_data?.[name]
      ?? row?.data?.[name]
      ?? ''
    const v = raw === '—' ? '' : raw
    data[name] = typeof v === 'string' ? v : String(v ?? '')
  }
  editForm.value = { data, remarkParts: {}, items: [] }
  // 新会话：清掉上一轮的系统建议值，否则「用户手改过」判定会把新员工的
  // 空工号当成手改值，选部门后不再自动生成。
  lastAutoEmployeeId = ''
  // 打开抽屉时先跑一次派生（幂等），再取基线 —— 避免 watch 在基线之后触发
  // 把刚打开的抽屉误标为「已修改」。
  if (activeModule.value === 'roster') {
    applyIdCardDerivation()
    applyHireDerivation()
  }
  editBaseline.value = JSON.stringify(editForm.value)
  detailDirty.value = false
  detailVisible.value = true
}

// 脏检查只标脏，**不自动保存**（第 19 轮）
watch(editForm, () => {
  detailDirty.value = JSON.stringify(editForm.value) !== editBaseline.value
}, { deep: true })

async function saveDetail() {
  const row = detailRow.value
  const form = hrCleanForm(editForm.value.data || {})
  // 第 52 轮：保存前统一校验，错误直接拦截并给出清晰表单提示。
  const invalid = hrValidateForm(form)
  if (invalid) { MessagePlugin.warning(invalid); return }
  saving.value = true
  try {
    const payload: Record<string, any> = { ...(row || {}), ...form }
    const id = row?.id
    if (activeModule.value) {
      // 花名册 / 离职管理 / 劳动合同共用 hr_employees：离职与合同字段都是
      // 同一行的 custom_data 投影，抽屉保存也走同一个更新接口。
      if (id) await hrApi.updateRosterEmployee(id, payload)
      else if (activeTab.value === 'hr-roster') await hrApi.createRosterEmployee(payload)
      else { MessagePlugin.warning('请从花名册新增员工'); return }
    } else if (fundSubView.value === 'bills') {
      MessagePlugin.info('月度账单由系统生成，不支持手工编辑')
    } else {
      if (id) await hrApi.updateFundRecord(id, payload)
      else await hrApi.createFundRecord(payload)
    }
    MessagePlugin.success('已保存')
    detailVisible.value = false
    await loadList(activeTab.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function onDetailDelete() {
  const row = detailRow.value
  if (!row?.id) { detailVisible.value = false; return }
  try {
    if (activeModule.value) await hrApi.removeRosterEmployee(row.id)
    else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
    else await hrApi.removeFundRecord(row.id)
    MessagePlugin.success('已删除')
    detailVisible.value = false
    await loadList(activeTab.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// 社保账期：个人 / 单位扣款明细
const periodDetailVisible = ref(false)
const periodDetail = ref<any>(null)
const personalCharges = ref<any[]>([])
const unitCharges = ref<any[]>([])
function chargeVal(row: any, keys: string[]): string {
  for (const k of keys) {
    const v = row?.[k]
    if (v !== undefined && v !== null && v !== '') return String(v)
  }
  return ''
}
// 列键对齐后端 HRSICharge：name / id_number / department / base / subtotal
const chargeColumns = [
  { colKey: 'name', title: '姓名', cell: (_h: any, { row }: any) => chargeVal(row, ['name', '姓名', 'employee_name']) },
  { colKey: 'id_number', title: '证件号', cell: (_h: any, { row }: any) => chargeVal(row, ['id_number', 'identity_number', '证件号', 'person_code']) },
  { colKey: 'department', title: '部门', cell: (_h: any, { row }: any) => chargeVal(row, ['department', '部门']) },
  {
    colKey: 'base', title: '缴费基数',
    cell: (_h: any, { row }: any) => grouped(chargeVal(row, ['base', '缴费基数', 'base_amount', 'personal_base']), { fixed: true, fallback: '', blankWhenEmpty: true }),
  },
  {
    colKey: 'subtotal', title: '小计',
    cell: (_h: any, { row }: any) => grouped(chargeVal(row, ['subtotal', 'amount', '小计', '金额']), { fixed: true, fallback: '', blankWhenEmpty: true }),
  },
]
async function openPeriodDetail(row: any) {
  periodDetail.value = row
  personalCharges.value = []
  unitCharges.value = []
  periodDetailVisible.value = true
  if (!row?.id) return
  try {
    const [p, u] = await Promise.all([
      hrApi.getSocialCharges(row.id, 'personal'),
      hrApi.getSocialCharges(row.id, 'unit'),
    ])
    personalCharges.value = extractItems(p)
    unitCharges.value = extractItems(u)
  } catch { /* 明细失败保留空表 */ }
}

// ---------------------------------------------------------------------------
// 设置 / 新增 / 打印
// ---------------------------------------------------------------------------
const settingsVisible = ref(false)
/**
 * 设置抽屉保存后的六同步入口（第 50 轮）。
 *
 * 顺序不能反：先 `loadFieldConfig` 把后端最新 subs 拉回来，
 * 再据 `is_default` 重算列显隐，最后覆盖 localStorage。
 * 之前的实现只做了第一步，所以「取消默认表头」保存后列表毫无变化。
 */
async function onSettingsSaved() {
  await loadFieldConfig(activeTab.value)
  const allKeys = businessColumns.value.map((c) => c.colKey)
  const isRosterLike = ['hr-roster', 'hr-resign', 'hr-contract'].includes(activeTab.value)
  if (isRosterLike) {
    // 后端已落库 → 立即以最新 defaultHeader 覆盖当前列表与缓存。
    visibleColKeys.value = configKeys.value.defaults.filter((k) => allKeys.includes(k))
  } else {
    // 社保 / 公积金：固定列不能按 subs 的 is_default 重算（会冲掉业务必需列），
    // 保存后直接把当前列全集（固定列 + 新增/引用字段）全部可见 ——
    // 用户刚在设置里添加字段，就是要立刻在表头看到它。
    visibleColKeys.value = allKeys
  }
  persistColumns()
  await loadList(activeTab.value)
}

function onPrimaryAction() {
  if (activeTab.value === 'hr-social') { void handleAddPeriod(); return }
  // 第 53 轮：新增时**不预生成工号**。部门还没选，任何编码都是猜的；
  // 工号由 #field-部门 的 change 事件在选定部门后生成（用户可再手改）。
  openDetail(null)
}

/**
 * 第 48 轮：新增员工时按「部门 + 职务」自动生成工号建议值。
 * 只填空白工号，用户手改过（非空）绝不覆盖；存量工号不受影响。
 */
/**
 * 按「部门 + 职务」生成工号建议值（第 53 轮重写）。
 *
 * 铁律：**部门为空时绝不生成** —— 工号里的 5 位部门码没有来源，
 * 生成出来必然是错的（旧实现落到 `C020000001` 这种兜底码）。
 * 生成后写 `lastAutoEmployeeId`，供「用户手改过就不覆盖」判定。
 */
async function autoFillEmployeeId() {
  if (activeTab.value !== 'hr-roster') return
  const dept = String(editForm.value?.data?.['部门'] || '').trim()
  if (!dept) {
    // 未选部门：工号保持空（新增场景）；已有员工的历史工号不动
    if (!detailRow.value) {
      editForm.value.data['工号'] = ''
      editForm.value.data['employee_id'] = ''
      lastAutoEmployeeId = ''
    }
    return
  }
  const title = String(editForm.value?.data?.['职务'] || editForm.value?.data?.['岗位'] || '').trim()
  try {
    const res: any = await hrApi.nextEmployeeId({ department: dept, title })
    const d = res?.data || res || {}
    const next = String(d.employee_id || '').trim()
    if (!next) return
    const cur = String(editForm.value?.data?.['工号'] || '').trim()
    // 已有工号且不是上次系统建议值 → 用户手改过，不覆盖
    if (cur && cur !== lastAutoEmployeeId) return
    editForm.value.data['工号'] = next
    editForm.value.data['employee_id'] = next
    lastAutoEmployeeId = next
  } catch { /* 编码建议失败不阻断新增 */ }
}

/** 部门 / 职务变化后重算工号（部门清空时把自动生成的工号一并清空）。 */
async function onDeptOrTitleChange() {
  if (activeTab.value !== 'hr-roster') return
  const dept = String(editForm.value?.data?.['部门'] || '').trim()
  if (!dept) {
    const cur = String(editForm.value?.data?.['工号'] || '').trim()
    if (!cur || cur === lastAutoEmployeeId) {
      editForm.value.data['工号'] = ''
      editForm.value.data['employee_id'] = ''
      lastAutoEmployeeId = ''
    }
    return
  }
  await autoFillEmployeeId()
}
async function handleAddPeriod() {
  const d = new Date()
  const year_month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    await hrApi.createSocialPeriod({ year_month })
    MessagePlugin.success('已新增账期')
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '新增失败') }
}

const printVisible = ref(false)
const printData = ref<Uint8Array | null>(null)
const printUrl = ref('')
const printBusy = ref(false)
async function handlePrint() {
  const list = selectedRows.value.length ? selectedRows.value : rows.value
  if (!list.length) { MessagePlugin.warning('当前无数据可打印'); return }
  printBusy.value = true
  try {
    const cols = visibleCols.value.map((c) => ({ key: c.colKey, label: c.title, value: (r: any) => c.value?.(r) ?? '' }))
    printData.value = await generateCatalogPdf({ title: printTitle.value, columns: cols, rows: list })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([printData.value as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '清单生成失败')
  } finally {
    printBusy.value = false
  }
}
function closePrint() {
  printVisible.value = false
  printData.value = null
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---------------------------------------------------------------------------
// 批量操作（导出 / 封存启封 / 离职恢复 / 删除）
// ---------------------------------------------------------------------------
function downloadBlob(blob: any, name: string) {
  const b = blob instanceof Blob
    ? blob
    : new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  const url = URL.createObjectURL(b)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}
async function handleExportExcel() {
  try {
    let blob: any
    if (activeModule.value) {
      blob = await hrApi.exportEmployees(selectedRowKeys.value)
    } else if (activeTab.value === 'hr-social') {
      const id = selectedRowKeys.value[0] || rows.value[0]?.id
      if (!id) { MessagePlugin.warning('请先选择账期'); return }
      blob = await hrApi.exportSocialCharges(id)
    } else {
      blob = await hrApi.exportFund(selectedRowKeys.value)
    }
    downloadBlob(blob, `${printTitle.value}.xlsx`)
    MessagePlugin.success('已导出')
  } catch (e: any) { MessagePlugin.error(e?.message || '导出失败') }
}

async function handleSeal(seal: boolean) {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    for (const id of ids) {
      if (seal) await hrApi.sealFundRecord(id)
      else await hrApi.unsealFundRecord(id)
    }
    MessagePlugin.success(seal ? '已封存' : '已启封')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

/** 第 46 轮：办理离职改为打开标准弹窗（离职时间/原因/证明/备注），不再静默提交。 */
function handleResign() {
  if (!selectedRowKeys.value.length) { MessagePlugin.warning('请先选择员工'); return }
  resignVisible.value = true
}

/**
 * 公司设置（左下角用户菜单弹窗）保存后：部门可能改名 → 存量员工已被后端级联更新，
 * 必须重拉部门/单位下拉、列表与卡片，否则界面还停在旧部门名上。
 *
 * 第 53 轮：组织机构从「设置抽屉」迁移到全局「公司设置」弹窗，
 * 用 window 事件跨组件通知（弹窗可能在任何页面打开）。
 */
async function onOrgChanged() {
  await loadRosterDepts()
  // 部门被删/改名后当前筛选可能指向不存在的部门 → 复位到全部
  if (rosterDeptFilter.value && !rosterDeptOptions.value.includes(rosterDeptFilter.value)) {
    rosterDeptFilter.value = ''
  }
  await loadFieldConfig(activeTab.value)
  await loadList(activeTab.value)
  await reloadStats()
}

async function onRosterImported() {
  await loadList(activeTab.value)
  await reloadStats()
}

async function onResignDone() {
  clearSelection()
  await loadList(activeTab.value)
  await reloadStats()
}

/** 第 52 轮：离职证明上传（只回填草稿路径，绝不自动保存员工记录）。 */
const proofInputRef = ref<HTMLInputElement | null>(null)
const proofUploading = ref(false)
function pickProof() { proofInputRef.value?.click() }
async function onProofFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (!/\.(pdf|jpe?g|png)$/i.test(f.name)) {
    MessagePlugin.warning('离职证明仅支持 PDF / JPG / PNG 格式')
    return
  }
  if (f.size > 10 * 1024 * 1024) {
    MessagePlugin.warning('附件不能超过 10MB')
    return
  }
  proofUploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', f)
    const res: any = await hrApi.uploadResignProof(fd)
    const data = res?.data || res || {}
    const path = String(data.path || '')
    if (!path) { MessagePlugin.error('附件上传失败'); return }
    editForm.value.data['离职证明'] = path
    editForm.value.data['resign_proof_path'] = path
    MessagePlugin.success('已上传，保存后生效')
  } catch (err: any) {
    MessagePlugin.error(err?.message || '附件上传失败')
  } finally {
    proofUploading.value = false
  }
}

/** 离职证明路径：主列优先，兼容 custom_data / data 包裹。 */
function proofPathOf(row: any): string {
  const v = row?.resign_proof_path ?? row?.['离职证明']
    ?? row?.custom_data?.['离职证明'] ?? row?.data?.['离职证明']
  return typeof v === 'string' ? v.trim() : ''
}

async function previewProof(row: any) {
  const id = proofPathOf(row).split('/').pop() || ''
  if (!id) { MessagePlugin.warning('附件路径无效'); return }
  try {
    const blob: any = await hrApi.getResignProof(id)
    const url = URL.createObjectURL(blob instanceof Blob ? blob : new Blob([blob]))
    window.open(url, '_blank')
    setTimeout(() => URL.revokeObjectURL(url), 60_000)
  } catch (e: any) { MessagePlugin.error(e?.message || '附件预览失败') }
}
async function handleRestore() {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择员工'); return }
  try {
    await hrApi.restoreRosterEmployees(ids)
    MessagePlugin.success('已恢复在职')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

async function handleBatchDelete() {
  const list = selectedRows.value
  if (!list.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    if (activeModule.value) {
      await hrApi.batchDeleteRosterEmployees(list.map((r) => r.id).filter(Boolean))
    } else {
      for (const row of list) {
        if (!row?.id) continue
        if (activeTab.value === 'hr-social') await hrApi.removeSocialPeriod(row.id)
        else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
        else await hrApi.removeFundRecord(row.id)
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---------------------------------------------------------------------------
// 社保业务按钮 / 公积金账单 / 文件导入
// ---------------------------------------------------------------------------
async function handleGenerateBill() {
  const d = new Date()
  const month_label = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    await hrApi.createFundBill({ month_label })
    MessagePlugin.success('已生成月度账单')
    fundSubView.value = 'bills'
    await onFundSubViewChange()
  } catch (e: any) { MessagePlugin.error(e?.message || '生成失败') }
}

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------
onMounted(async () => {
  // 第 53 轮：公司设置弹窗（左下角用户菜单）保存后广播事件，
  // 人事页据此重拉部门/单位下拉、列表与卡片。
  window.addEventListener('hr-org-changed', onOrgChanged)
  void loadRosterDepts()
  initColumns()
  await switchTab('hr-roster')
  await reloadStats()
  filterWatchReady = true
})
onUnmounted(() => window.removeEventListener('hr-org-changed', onOrgChanged))
</script>

<style lang="less" scoped>
/* 中台共享列表样式统一在文末的**非 scoped** 块 @import：
   scoped 会给每条规则加 [data-v-xxx]，而 TDesign 内部渲染的
   .t-table__content / tbody / tfoot 不带该属性，选择器永远命中不到。
   这里只放页面自用样式。 */
.hr-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  flex: 1;
}
.overview-group { flex: 0 0 auto; }
.biz-list-view { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 8px; }

.hr-subview-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 2px 0;
  flex: 0 0 auto;
}
.hr-unit-config {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
/* 批量按钮外层保留可见盒（真机 data-hr-batch-bar 断言可读性），
   内部按钮随浮条换行；不设固定宽度，避免撑破浮动条。 */
.hr-batch-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.hr-empty-placeholder { flex: 1; min-height: 0; }
.hr-file-input { display: none; }
/* 搜索框正后方的在职状态筛选（第 46 轮） */
.hr-status-select { width: 140px; flex-shrink: 0; }
.hr-month-picker { width: 130px; flex-shrink: 0; }
.hr-si-detail { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
.hr-si-filters { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.hr-si-search { width: 260px; }
.hr-si-dept { width: 160px; }
.hr-si-scheme { width: 140px; }
.hr-si-count { color: var(--td-text-color-secondary); }
.hr-si-table { flex: 1; min-height: 0; }
/* 离职证明附件：输入框 + 上传 + 查看 一行排列 */
.hr-proof-field {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.hr-charge-group { margin-bottom: 20px; }
.hr-charge-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 8px;
}
</style>

<style>
/* 共享列表样式必须在**非 scoped** 块再引入一次：TDesign 内部的
   .t-table__content / .t-table__body / tfoot 等元素拿不到当前组件的 data-v，
   scoped 版本的选择器命中不到它们（发票 / 车队页同款做法）。 */
@import './styles/business-list-shared.css';
</style>
