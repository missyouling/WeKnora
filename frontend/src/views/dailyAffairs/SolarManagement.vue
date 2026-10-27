<template>
  <div class="solar-management-container">

    <!-- 加载中 -->
    <div v-if="loading" class="loading-area">
      <!-- 模块初始化加载：与上游项目完全一致（small、无文字、父容器居中） -->
      <t-loading size="small" />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-电费」知识库（光伏与电费共用）">
        <template #image><t-icon name="dashboard" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建电费知识库</t-button>
    </div>

    <!-- 主界面 -->
    <div v-else class="solar-main">
      <!-- ================= 列表视图 ================= -->
      <template v-if="!detailMode">
        <!-- 筛选工具栏：与发票/合同/制度/通知、市电账单同一个 BusinessFilterBar
             （搜索全部字段 220px/32px、控件 32px、gap 8px、右侧 字段→设置→主按钮）。 -->
        <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
          :columns="columnDefs" :visible-keys="visibleColKeys" :selected-count="selectedRowKeys.length"
          settings-text="设置" hide-batch-bar :primary-action="{ text: '上传账单', icon: 'upload' }"
          @columns-change="(v: string[]) => (visibleColKeys = v)" @reset-columns="resetColumns"
          @select-all-columns="selectAllColumns" @filter-change="applyFilter" @settings="openFieldSettings"
          @primary="triggerUpload" @clear-selection="clearSelection" />

        <!-- 进行中文件状态横幅（解析/提取实时进度，终态自动收起）。
             与发票管理、市电账单同一个组件：上传/重新提取后必须立刻可见，
             否则用户只能盯着空列表等（真机：光伏页此前根本没挂这个条）。 -->
        <BusinessParseStatusBar :progress="[]" :pending-files="pendingFiles" />

        <!-- 列表 -->
        <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
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
        class="doc-table"
        :selected-row-keys="selectedRowKeys"
        select-on-change
        :row-class-name="({ row }: any) => selectedRowKeys.includes(row.rowKey) ? 'is-selected' : ''"
        @row-click="({ row }: any) => handleRowClick(row)"
        @select-change="(val: any) => onTableSelectChange(val)"
      >
        <template #extractStatus="{ row }: any">
          <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
            variant="light-outline" class="row-status-tag">
            <template v-if="statusOf(row).icon" #icon>
              <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
            </template>
            {{ statusOf(row).label }}
          </t-tag>
        </template>
        <template #tags="{ row }: any">
          <t-tooltip v-if="rowTags(row).length" :content="rowTags(row).map((t: any) => t.name).join('、')" placement="top">
            <div class="row-tag-chips is-clickable" @click.stop="openTagEdit(row)">
              <t-tag v-if="rowTags(row).length" size="small" variant="light-outline" class="row-tag">
                {{ rowTags(row)[0].name }}
              </t-tag>
              <span v-if="rowTags(row).length > 1" class="row-tag-more">+{{ rowTags(row).length - 1 }}</span>
            </div>
          </t-tooltip>
          <span v-else class="row-tag-chips is-clickable" @click.stop="row.kind !== 'pending' && openTagEdit(row)">
            <span class="row-tag-add">+ 标签</span>
          </span>
        </template>
      </t-table>
    </div>
          <!-- 底部汇总：与发票管理一致，位于白色列表卡片（.doc-list-scroll）内部最底端 -->
          <div v-if="summaryCount" class="doc-summary-bar">
            <span class="doc-summary-count">共 {{ summaryCount }} 条</span>
            <span class="doc-summary-item">发电量 <span class="doc-summary-val">{{ fmtKwh(summaryUsage) }}</span> 千瓦时</span>
            <span class="doc-summary-item">结算金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
            <span v-if="selectedRowKeys.length" class="doc-summary-item summary-selected">已选 {{ selectedRowKeys.length }} 条</span>
          </div>
        </div>

        <!-- 底部浮动工具栏 -->
        <transition name="batch-bar-fade">
          <div v-if="selectedRowKeys.length" class="doc-batch-bar-fixed" role="region">
            <div class="batch-bar-inner">
              <div class="batch-bar-left">
                <span class="batch-bar-count">已选 {{ selectedRowKeys.length }} 项</span>
                <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
                  清除
                </t-button>
              </div>
              <div class="batch-bar-actions">
                <t-popconfirm theme="warning"
                  :content="`确定重新解析并提取「${selectedSingle?.fileName || '该文件'}」吗？将覆盖已有提取结果。`"
                  :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                  @confirm="handleReExtract">
                  <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                    <template #icon><t-icon name="refresh" size="14px" /></template>
                    重新提取
                  </t-button>
                </t-popconfirm>
                <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1 || selectedSingle?.kind === 'pending'" @click="handleBatchEdit">
                  <template #icon><t-icon name="edit" size="14px" /></template>
                  编辑数据
                </t-button>
                <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.some(r => r.kind === 'pending')" @click="handleBatchPrint">
                  <template #icon><t-icon name="print" size="14px" /></template>
                  打印
                </t-button>
                <t-popconfirm theme="warning"
                  :content="`确定删除所选 ${selectedRowKeys.length} 条账单记录吗？源文件将移入删除历史。`"
                  :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                  @confirm="handleBatchDelete">
                  <t-button theme="danger" variant="outline" size="small" @click.stop>
                    <template #icon><t-icon name="delete" size="14px" /></template>
                    删除记录
                  </t-button>
                </t-popconfirm>
              </div>
            </div>
          </div>
        </transition>
      </template>

      <!-- ================= 详情视图（概览 + 明细） ================= -->
      <div v-else class="bill-detail-layout">
        <!-- 面包屑 -->
        <div class="bill-detail-head">
          <t-button variant="text" size="small" @click="exitDetail">
            <template #icon><t-icon name="chevron-left" size="15px" /></template>
            账单列表
          </t-button>
          <span class="bill-detail-title">{{ currentRow?.fileName || '账单详情' }}</span>
          <t-tag v-if="currentStatus.label !== '--'" size="small" :theme="currentStatus.theme"
            variant="light-outline" class="row-status-tag">
            <template v-if="currentStatus.icon" #icon>
              <t-icon :name="currentStatus.icon" :class="{ 'icon-spin': currentStatus.spin }" />
            </template>
            {{ currentStatus.label }}
          </t-tag>
        </div>
        <div class="bill-detail-body">
          <!-- 左侧分层菜单 -->
          <aside class="bill-nav">
            <div class="bill-nav-group">
              <div class="bill-nav-item" :class="{ active: activeMenu === 'overview' }" @click="activeMenu = 'overview'">
                账单概况
              </div>
              <div class="bill-nav-group-title">电费明细</div>
              <div class="bill-nav-item bill-nav-item--child" :class="{ active: activeMenu === 'grid-fee' }" @click="activeMenu = 'grid-fee'">
                上网电费
              </div>
              <div class="bill-nav-item bill-nav-item--child" :class="{ active: activeMenu === 'subsidy-fee' }" @click="activeMenu = 'subsidy-fee'">
                发电补助
              </div>
              <div class="bill-nav-group-title">电量明细</div>
              <div class="bill-nav-item bill-nav-item--child" :class="{ active: activeMenu === 'meter' }" @click="activeMenu = 'meter'">
                关口电量
              </div>
            </div>
          </aside>
          <!-- 右侧内容区 -->
          <div class="bill-content">
            <!-- 账单概况 -->
            <template v-if="activeMenu === 'overview'">
              <!-- 基础信息卡 -->
              <div class="bill-card">
                <div class="bill-card-head">
                  <span class="bill-card-title">基础信息</span>
                </div>
                <div class="solar-info-grid">
                  <div class="solar-info-item"><span class="solar-info-label">户号</span><span class="solar-info-value">{{ it.account_no || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">户名</span><span class="solar-info-value">{{ it.account_name || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">服务单位</span><span class="solar-info-value">{{ it.supply_unit || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">并网电压</span><span class="solar-info-value">{{ it.voltage_level || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">消纳方式</span><span class="solar-info-value">{{ it.consumption_mode || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">发电方式</span><span class="solar-info-value">{{ it.generation_mode || '' }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">账单周期</span><span class="solar-info-value">{{ metricPeriod }}</span></div>
                  <div class="solar-info-item"><span class="solar-info-label">纳税人类型</span><span class="solar-info-value">{{ it.taxpayer_type || '' }}</span></div>
                </div>
              </div>
              <!-- 指标卡 -->
              <div class="overview-metrics">
                <div class="metric-card">
                  <div class="metric-label">发电量</div>
                  <div class="metric-value">{{ fmtKwh(it.generation_kwh) }}<span class="metric-unit">千瓦时</span></div>
                </div>
                <div class="metric-card">
                  <div class="metric-label">上网电量</div>
                  <div class="metric-value">{{ fmtKwh(it.grid_kwh) }}<span class="metric-unit">千瓦时</span></div>
                </div>
                <div class="metric-card">
                  <div class="metric-label">结算金额</div>
                  <div class="metric-value">{{ fmtMoney(it.settlement_amount) }}<span class="metric-unit">元</span></div>
                </div>
                <div class="metric-card">
                  <div class="metric-label">年累计上网</div>
                  <div class="metric-value">{{ fmtKwh(it.cumulative_kwh) }}<span class="metric-unit">千瓦时</span></div>
                </div>
              </div>
              <!-- 电量分析 / 结算明细 -->
              <div class="overview-two-col">
                <div class="bill-card">
                  <div class="bill-card-head">
                    <span class="bill-card-title">电量分析</span>
                  </div>
                  <div class="solar-analysis">
                    <div class="solar-analysis-row">
                      <span class="solar-analysis-label">本期上网电量较上期</span>
                      <span class="solar-analysis-value">{{ it.mom_change || '—' }}</span>
                    </div>
                    <div class="solar-analysis-row">
                      <span class="solar-analysis-label">年累计上网电量</span>
                      <span class="solar-analysis-value">{{ fmtKwh(it.cumulative_kwh) }} 千瓦时</span>
                    </div>
                    <div class="solar-analysis-row">
                      <span class="solar-analysis-label">税率</span>
                      <span class="solar-analysis-value">{{ it.tax_rate || '—' }}</span>
                    </div>
                    <div class="solar-analysis-row">
                      <span class="solar-analysis-label">税额</span>
                      <span class="solar-analysis-value">{{ it.tax_amount || it.tax_amount === 0 ? fmtMoney(it.tax_amount) : '' }}</span>
                    </div>
                    <div class="solar-analysis-row">
                      <span class="solar-analysis-label">备注</span>
                      <span class="solar-analysis-value solar-analysis-note" :title="it.remark">{{ it.remark || '—' }}</span>
                    </div>
                  </div>
                </div>
                <div class="bill-card">
                  <div class="bill-card-head">
                    <span class="bill-card-title">结算明细</span>
                    <span class="bill-card-hint">点击行查看明细</span>
                  </div>
                  <div class="os-table">
                    <div class="os-row os-row--head">
                      <span>费用组成</span><span class="os-qty">计费数量</span><span class="os-amount">电费</span>
                    </div>
                    <div class="os-row os-row--link" v-for="r in overviewRows" :key="r.key" @click="gotoMenu(r.menuKey)">
                      <span class="os-name">{{ r.label }}</span>
                      <span class="os-qty">{{ r.qtyText }}</span>
                      <span class="os-amount" :class="{ 'os-neg': r.value < 0 }">{{ fmtRate6(r.value) }}</span>
                    </div>
                    <div class="os-row os-row--total">
                      <span>结算金额</span>
                      <span class="os-qty">—</span>
                      <span class="os-amount">{{ fmtRate6(overviewTotal) }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <!-- 上网电费 -->
            <template v-else-if="activeMenu === 'grid-fee'">
              <div class="bill-card">
                <div class="bill-card-head">
                  <span class="bill-card-title">上网电费明细</span>
                  <span class="bill-card-hint">电费 = 电量 × 电价，自动计算；点击行编辑</span>
                </div>
                <div class="fee-group-table">
                  <div class="fg-row fg-head">
                    <span v-for="c in gridFeeCols" :key="c.field_key">{{ c.label }}</span>
                  </div>
                  <div v-for="(it, i) in gridFeeRows" :key="i" class="fg-row" @click="openFeeEdit('grid', Number(i))">
                    <span v-for="c in gridFeeCols" :key="c.field_key"
                      :class="{ 'fg-name': c.field_key === 'category', 'row-mono': c.field_type !== 'text' }">
                      {{ feeCellText(it, c) }}
                    </span>
                  </div>
                  <div v-if="!gridFeeRows.length" class="fg-empty">暂无数据</div>
                  <div v-if="gridFeeRows.length" class="fg-row fg-total">
                    <span>小计</span>
                    <span v-for="c in gridFeeCols.slice(1)" :key="c.field_key" />
                    <span class="row-mono">{{ fmtMoney(gridFeeTotal) }}</span>
                  </div>
                </div>
              </div>
            </template>

            <!-- 发电补助 -->
            <template v-else-if="activeMenu === 'subsidy-fee'">
              <div class="bill-card">
                <div class="bill-card-head">
                  <span class="bill-card-title">发电补助明细</span>
                  <span class="bill-card-hint">点击行编辑</span>
                </div>
                <div class="fee-group-table">
                  <div class="fg-row fg-head">
                    <span v-for="c in subsidyFeeCols" :key="c.field_key">{{ c.label }}</span>
                  </div>
                  <div v-for="(it, i) in subsidyFeeRows" :key="i" class="fg-row" @click="openFeeEdit('subsidy', Number(i))">
                    <span v-for="c in subsidyFeeCols" :key="c.field_key"
                      :class="{ 'fg-name': c.field_key === 'category', 'row-mono': c.field_type !== 'text' }">
                      {{ feeCellText(it, c) }}
                    </span>
                  </div>
                  <div v-if="!subsidyFeeRows.length" class="fg-empty">暂无数据</div>
                  <div v-if="subsidyFeeRows.length" class="fg-row fg-total">
                    <span>小计</span>
                    <span v-for="c in subsidyFeeCols.slice(1)" :key="c.field_key" />
                    <span class="row-mono">{{ fmtMoney(subsidyFeeTotal) }}</span>
                  </div>
                </div>
              </div>
            </template>

            <!-- 关口电量 -->
            <template v-else>
              <div class="bill-card">
                <div class="bill-card-head">
                  <span class="bill-card-title">关口电量明细</span>
                  <span class="bill-card-hint">各关口本期示数 / 倍率 / 抄见电量</span>
                </div>
                <div class="fee-group-table">
                  <div class="fg-row fg-head">
                    <span>关口类型</span><span>电能表编号</span><span>示数类型</span><span>上期示数</span><span>本期示数</span><span>倍率</span><span>抄见电量</span><span>计费电量</span>
                  </div>
                  <div v-for="(g, gi) in gatewayRows" :key="gi" class="fg-row">
                    <span class="fg-name">{{ g.gateway_type }}</span>
                    <span class="row-mono" :title="g.meter_no">{{ g.meter_no }}</span>
                    <span class="fg-name">{{ g.readings[0]?.meter_type || '' }}</span>
                    <span class="row-mono">{{ g.readings[0]?.prev || g.readings[0]?.prev === 0 ? fmtKwh(g.readings[0].prev) : '' }}</span>
                    <span class="row-mono">{{ g.readings[0]?.curr || g.readings[0]?.curr === 0 ? fmtKwh(g.readings[0].curr) : '' }}</span>
                    <span class="row-mono">{{ displayMultiplier(g.readings[0]) }}</span>
                    <span class="row-mono">{{ g.readings[0]?.reading_kwh || g.readings[0]?.reading_kwh === 0 ? fmtKwh(g.readings[0].reading_kwh) : '' }}</span>
                    <span class="row-mono">{{ g.readings[0]?.bill_kwh || g.readings[0]?.bill_kwh === 0 ? fmtKwh(g.readings[0].bill_kwh) : '' }}</span>
                  </div>
                  <div v-if="!gatewayRows.length" class="fg-empty">暂无数据</div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- 上传账单：与发票管理完全同一个 FleetUploadDialog；分类固定「光伏账单」，
         提取走账单专用管线（extract-solar-bill）。上传即写 fleet_cert_type=光伏账单，
         市电页轮询据此让路，不再出现「光伏文件被当成电费提取」。 -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="solar"
      :type-options="uploadTypeOptions" default-type="光伏账单" title="上传账单" extract-mode="energy-bill"
      @done="onUploadDone" />

    <!-- 详情抽屉：与发票/合同同款中台 BusinessSplitDetailDrawer
         （质量面板 → 双列字段表单 → 底部折叠源文件预览 → 统一底栏）。
         旧自写 <t-drawer> + field-grid 已淘汰。表单契约是 { data, remarkParts, items }，
         data 直接指向 detailForm，v-model 写 form.data[key] 即落回 detailForm。 -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="currentRow"
      :page-no="Number(currentRow?.item?.page) || 0" :form="detailFormWrapper" :fields="bsdFields"
      block-title="光伏账单字段" :quality="detailQuality" :label-of="qualityLabel"
      :storage-key="'weknora-solar-detail-drawer-width'" @save="saveDetailFields" />

    <!-- 费用编辑抽屉 -->
    <t-drawer v-if="feeEditVisible" :visible="true" :header="'编辑费用 · ' + feeEditTitle" size="460px" :footer="false"
      :close-on-overlay-click="true" @close="feeEditVisible = false"
      @update:visible="(v: boolean) => (feeEditVisible = v)">
      <div class="edit-drawer-body">
        <div class="edit-field">
          <label class="edit-label">费用类别</label>
          <t-input :model-value="feeEditForm.category ?? ''" size="small" disabled />
        </div>
        <div class="edit-field">
          <label class="edit-label">计费数量（千瓦时）</label>
          <t-input :model-value="String(feeEditForm.qty ?? '')" type="number" size="small"
            @update:model-value="(v: string) => { feeEditForm.qty = Number(v) || 0; recalcFeeEdit() }" />
        </div>
        <div class="edit-field">
          <label class="edit-label">电价（元/千瓦时）</label>
          <t-input :model-value="String(feeEditForm.rate ?? '')" type="number" size="small"
            @update:model-value="(v: string) => { feeEditForm.rate = Number(v) || 0; recalcFeeEdit() }" />
        </div>
        <div class="edit-field">
          <label class="edit-label">电费（元）</label>
          <t-input :model-value="String(feeEditForm.fee ?? '')" type="number" size="small"
            @update:model-value="(v: string) => { feeEditForm.fee = Number(v) || 0 }" />
        </div>
        <div class="edit-note">电费 = 计费数量 × 电价，自动计算；补助类（0 元）保留原值</div>
        <div class="edit-drawer-footer">
          <t-button theme="primary" size="small" @click="saveFeeEdit">保存</t-button>
        </div>
      </div>
    </t-drawer>

    <!-- 标签编辑 -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理 -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 初始化向导 -->
    <UtilitiesKbWizard v-model:visible="wizardVisible" @created="onKbCreated" />

    <!-- 「光伏设置」不再单独开抽屉：已内嵌为统一设置抽屉（StandardSettingDrawer）的
         「光伏设置」Tab，由 UtilitiesManagement 注入 SolarSettingsPanel。 -->


    <!-- 打印预览：全中台统一组件 -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="`打印预览（${printCount} 份）`" :url="printUrl"
      :data="printBytes" :download-name="`${printCount}份光伏账单.pdf`" loading-text="正在加载账单 PDF…"
      @close="closePrint" />
  </div>
</template>

<script setup lang="ts">
import {
  ref,
  computed,
  watch,
  onMounted,
  onBeforeUnmount,
} from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  updateKnowledgeMetadata,
  updateBusinessMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractSolarBill,
  reparseKnowledge,
  listSolarBillRecords,
  previewKnowledgeFile,
} from '@/api/knowledge-base'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import UtilitiesKbWizard from './UtilitiesKbWizard.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import { colWidthOf } from './columnWidth'
import { notifyEnergyDataChanged, rowFlattenText, matchAllTokens, billMonthOfItem } from './energyCounters'
import { withTimeout } from '@/composables/useBusinessPolling'
import { parseExtractQuality, EMPTY_QUALITY, type ExtractQuality } from './extractQuality'

import { useTableHeaderFix } from './useTableHeaderFix'
import { loadEnergyCategorySubs, applySubsToColumns, type EnergySub } from './energyViewColumns'

// 表头右侧滚动条轨道遮罩的实测值（与发票管理同一套机制）
const { headerFixStyle } = useTableHeaderFix()

// 「设置」升级为向父级请求统一设置抽屉：抽屉只挂在 UtilitiesManagement 根部一份，
// 切换卡片时不会被 v-if 卸载（子组件自己开抽屉时，切卡后事件无人接住）。
const emit = defineEmits<{ (e: 'open-settings'): void }>()

const KB_NAME = '日常事务-电费'
const PAGE_SIZE = 30
const COLUMN_STORAGE_KEY = 'weknora-solar-columns-v1'
const COLUMN_STORAGE_VERSION = 1

interface ColDef { key: string; label: string; fieldType: string; default: boolean; w: string; width?: number }
const FALLBACK_COLUMNS: ColDef[] = [
  // 默认字段（按用户指定顺序）
  { key: 'bill_period', label: '账单周期', fieldType: 'text', default: true, w: '1.2fr' },
  { key: 'generation_kwh', label: '发电量', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'grid_kwh', label: '上网电量', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'settlement_amount', label: '结算金额', fieldType: 'amount', default: true, w: '1.2fr' },
  { key: 'cumulative_kwh', label: '年累计', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'meter_prev', label: '上期示数', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'meter_curr', label: '本期示数', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'meter_ratio', label: '倍率', fieldType: 'text', default: true, w: '0.9fr' },
  { key: 'meter_reading', label: '抄见电量', fieldType: 'number', default: true, w: '1.1fr' },
  { key: 'meter_bill', label: '计费电量', fieldType: 'number', default: true, w: '1.1fr' },
  // 详细字段
  { key: 'tax_rate', label: '税率', fieldType: 'text', default: false, w: '0.8fr' },
  { key: 'tax_amount', label: '税额', fieldType: 'amount', default: false, w: '1fr' },
  { key: 'account_no', label: '发电户号', fieldType: 'text', default: false, w: '1.2fr' },
  { key: 'account_name', label: '户名', fieldType: 'text', default: false, w: '1.4fr' },
  { key: 'voltage_level', label: '并网电压', fieldType: 'text', default: false, w: '1.1fr' },
  { key: 'consumption_mode', label: '消纳方式', fieldType: 'text', default: false, w: '1.2fr' },
  { key: 'generation_mode', label: '发电方式', fieldType: 'text', default: false, w: '1.1fr' },
  { key: 'supply_unit', label: '服务单位', fieldType: 'text', default: false, w: '1.3fr' },
]

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)

// 字段配置
// 分类名与设置抽屉里的 fleet_categories.name 逐字一致
const CATEGORY_NAME = '光伏账单'
const categorySubs = ref<EnergySub[]>([])
/**
 * 生效列定义 = 光伏账单分类的字段定义（subs）。
 *
 * 静态 FALLBACK_COLUMNS 只提供格式化类型（fieldType）与宽度比例；
 * 集合 / 顺序 / 中文名 / 宽度 / 默认显示全部以 subs 为准，改完设置列表立刻变。
 */
const columnDefs = computed<ColDef[]>(() =>
  applySubsToColumns(
    FALLBACK_COLUMNS,
    categorySubs.value,
    (key, sub) => ({
      key,
      label: String(sub.label || key),
      fieldType: 'text',
      default: sub.is_default === true,
      w: '1fr',
    }),
  ),
)
/** 打开中台设置抽屉（字段定义 / 提取规则），定位到光伏账单分类。 */
function openFieldSettings() { emit('open-settings') }

/** 拉取字段定义（设置抽屉保存后由 window 事件触发重载）。 */
async function loadCategorySubs() {
  try {
    categorySubs.value = await loadEnergyCategorySubs(CATEGORY_NAME)
  } catch {
    categorySubs.value = []
  }
  if (visibleColKeys.value.length && !visibleColKeys.value.some(k => columnDefs.value.some(c => c.key === k))) {
    resetColumns()
  }
}
const visibleColKeys = ref<string[]>(loadStoredColumns())
const visibleColDefs = computed(() => columnDefs.value.filter(c => visibleColKeys.value.includes(c.key)))
const tableColumns = computed(() => {
  // 与发票管理一致：[复选框] → [序号] → 业务列
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 56, align: 'center' },
  ]
  for (const c of visibleColDefs.value) {
    // subs.width>0 精确固定（clamp 60~400）；0/未配置走内容自适应（与发票同一套 colWidthOf）
    cols.push({ colKey: c.key, title: c.label, ellipsis: true, fieldType: c.fieldType, width: colWidthOf(c.width, c.label, displayRows.value.map((r: any) => r[c.key])) })
  }
  cols.push({ colKey: 'extractStatus', title: '状态', width: 120 })
  cols.push({ colKey: 'tags', title: '标签', width: 140 })
  return cols
})
function onTableSelectChange(val: any) { selectedRowKeys.value = Array.isArray(val) ? val : [] }

function loadStoredColumns(): string[] {
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY)
    if (raw) {
      const obj = JSON.parse(raw)
      if (obj && obj.v === COLUMN_STORAGE_VERSION && Array.isArray(obj.keys) && obj.keys.length) {
        return obj.keys.filter((k: string) => columnDefs.value.some(c => c.key === k))
      }
    }
  } catch { /* ignore */ }
  return columnDefs.value.filter(c => c.default).map(c => c.key)
}
function selectAllColumns() { visibleColKeys.value = columnDefs.value.map(c => c.key) }
function resetColumns() { visibleColKeys.value = columnDefs.value.filter(c => c.default).map(c => c.key) }
function persistColumns() {
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify({ v: COLUMN_STORAGE_VERSION, keys: visibleColKeys.value })) } catch { /* ignore */ }
}
watch(visibleColKeys, () => persistColumns(), { deep: true })

// ---- 列表 ----
interface Row {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  extractStatus: string
  extractError?: string
  kind: string
  item: Record<string, any>
  tags?: any[]
}
const rows = ref<Row[]>([])
const pendingFiles = ref<any[]>([])
const summary = ref<{ total: number; sumKwh: number; sumAmount: number }>({ total: 0, sumKwh: 0, sumAmount: 0 })
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const monthFilter = ref('')
const keyword = ref('')
// BusinessFilterBar 扁平筛选态 ↔ 既有 ref 桥接（keyword/monthFilter 仍是单一真源，
// applyFilter 直接读它们）
const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'month', type: 'month', placeholder: '账单月份' },
])
const filterState = computed<FilterState>({
  get: () => ({ keyword: keyword.value, month: monthFilter.value }),
  set: (v) => {
    keyword.value = String(v?.keyword ?? '')
    monthFilter.value = v?.month || ''
  },
})
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const taggingInFlight = ref<Set<string>>(new Set())
const listScrollRef = ref<HTMLElement>()

// 汇总口径 = 当前可见行（已按关键词 / 账期过滤）
const summaryUsage = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : displayRows.value
  return target.reduce((s, r) => s + (Number((r.item as any)?.generation_kwh) || 0), 0)
})
const summaryAmount = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : displayRows.value
  return target.reduce((s, r) => s + (Number((r.item as any)?.settlement_amount) || 0), 0)
})
const summaryCount = computed(() => (selectedRowKeys.value.length ? selectedRowKeys.value.length : displayRows.value.length))
const selectedRows = computed(() => displayRows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)))
const selectedSingle = computed(() => (selectedRows.value.length === 1 ? selectedRows.value[0] : null))
const clearSelection = () => { selectedRowKeys.value = [] }

function toggleRow(key: string, checked: boolean) {
  if (checked) { if (!selectedRowKeys.value.includes(key)) selectedRowKeys.value.push(key) }
  else selectedRowKeys.value = selectedRowKeys.value.filter(k => k !== key)
}
const handleRowClick = (row: Row) => {
  if (row.kind === 'pending') {
    if (['failed', 'parse_failed'].includes(row.extractStatus)) toggleRow(row.rowKey, !selectedRowKeys.value.includes(row.rowKey))
    return
  }
  openDetail(row)
}

/** 日期串 → YYYY-MM（取前 7 位；非法值返回空串，由调用方兜底）。 */
const monthOfDate = (v: any): string => {
  const m = /^(\d{4})-(\d{1,2})/.exec(String(v || '').trim())
  if (!m) return ''
  const mm = Number(m[2])
  if (mm < 1 || mm > 12) return ''
  return `${m[1]}-${String(mm).padStart(2, '0')}`
}

const mapRow = (r: any): Row => {
  const it = r.item || {}
  const gw = (it.gateways || []).find((g: any) => g.gateway_type === '上网关口')
  const rd = gw && gw.readings && gw.readings[0] ? gw.readings[0] : {}
  return {
    rowKey: r.row_key || `${r.knowledge_id}-0`,
    knowledgeId: r.knowledge_id || '',
    fileName: r.file_name || r.knowledge_title || '',
    fileType: r.file_type || '',
    extractStatus: r.extract_status || '',
    extractError: r.extract_error || '',
    kind: r.extract_status === 'manual' ? 'manual' : 'bill',
    item: it,
    ...it,
    meter_prev: rd.prev,
    meter_curr: rd.curr,
    meter_ratio: rd.multiplier,
    meter_reading: rd.reading_kwh,
    meter_bill: rd.bill_kwh,
    // 账单周期只显示 YYYY-MM：起止日范围在 1600 宽的列表里必被截成
    // 「2026-06-01 ~ 202...」，而日精度对账期聚合毫无用处（与市电账单同口径）。
    bill_period: it.bill_period || monthOfDate(it.bill_period_start) || monthOfDate(it.bill_period_end) || '',
    tags: r.tags || [],
  }
}

// 解析失败 / 提取失败的文件不进主列表。
//
// 它们没有任何业务数据，混在账单里只会在第一行渲染一条「⊗ 解析失败」的空壳
// （真机截图：光伏账单首行整行空白 + 红标）。异常文件统一在
// 「历史记录 → 异常文件」里展示与重提，这与发票/合同管理的口径一致。
const FAILED_ROW_STATUS = new Set(['failed', 'parse_failed'])
const pendingRows = computed(() => pendingFiles.value
  .filter((pf: any) => !FAILED_ROW_STATUS.has(pendingLabel(pf)))
  .map((pf: any) => ({
  rowKey: `pending-${pf.id}`,
  knowledgeId: pf.id,
  fileName: pf.file_name || pf.title || '',
  extractStatus: pendingLabel(pf),
  extractError: pf.custom_metadata?.extract_error || '',
  kind: 'pending',
  item: {},
  tags: [],
})))
// 列表过滤：月份 + 「搜索全部字段」全部在**本地**做。
//
// 旧实现把月份丢给接口的 date_from/date_to，而这对参数在后端是按通用日期
// 字段过滤的（不是账期），跨月账单会被整条丢掉；关键词又只比文件名，
// 搜「星达」「2026-08」「-5000」一律零命中。两张表都已整表加载（十几条），
// 本地过滤既准确又没有口径漂移。
const displayRows = computed(() => {
  const kw = keyword.value.trim()
  const month = monthFilter.value
  let all = [...pendingRows.value, ...rows.value] as any[]
  if (month) {
    // 解析中/待提取的文件还没有账期，按月筛选时不占位
    all = all.filter((r: any) => r.kind !== 'pending' && billMonthOfItem(r) === month)
  }
  if (kw) all = all.filter((r: any) => matchAllTokens(`${String(r.fileName || '')} ${rowFlattenText(r)}`.toLowerCase(), kw))
  return all
})

const metricPeriodOf = (it: any) => {
  if (!it) return ''
  const s = String(it.bill_period_start || '')
  const e = String(it.bill_period_end || '')
  if (s && e) {
    const sm = s.slice(0, 7)
    const em = e.slice(0, 7)
    if (sm === em) return sm
    return `${sm}~${em}`
  }
  return s || e
}

const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    rows.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) return
  else loadingMore.value = true
  try {
    // 月份不交给接口：账期是账单内的业务字段（bill_period），
    // 接口的 date_from/date_to 打的是通用日期，跨月账单会被误杀。见 displayRows。
    const res: any = await listSolarBillRecords(kbId.value, {
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    summary.value = { total, sumKwh: 0, sumAmount: 0 }
    const existing = new Set(rows.value.map(r => r.rowKey))
    const fresh = arr.map(mapRow).filter(r => !existing.has(r.rowKey))
    rows.value = [...rows.value, ...fresh]
    hasMore.value = rows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}
const onListScroll = () => {
  const el = listScrollRef.value
  if (!el || !hasMore.value || loadingMore.value) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 80) loadFiles()
}
const applyFilter = () => { loadFiles(true) }

// ---- 状态 ----
const STATUS_MAP: Record<string, { label: string; theme: any; icon?: string; spin?: boolean }> = {
  parsing: { label: '解析中', theme: 'primary', icon: 'loading', spin: true },
  extracting: { label: '提取中', theme: 'primary', icon: 'loading', spin: true },
  pending: { label: '待提取', theme: 'primary', icon: 'loading', spin: true },
  success: { label: '提取成功', theme: 'success' },
  manual: { label: '待补录', theme: 'warning', icon: 'edit-1' },
  parse_failed: { label: '解析失败', theme: 'danger', icon: 'close-circle' },
  failed: { label: '提取失败', theme: 'danger', icon: 'close-circle' },
}
const statusOf = (row: Row) => {
  const st = row.extractStatus || ''
  if (STATUS_MAP[st]) return STATUS_MAP[st]
  return { label: '--', theme: 'default' as const }
}
const pendingLabel = (pf: any) => {
  const ps = pf.parse_status || ''
  if (ps === 'parsing' || ps === 'pending' || ps === 'processing' || ps === 'finalizing') return 'parsing'
  if (ps === 'failed') return 'parse_failed'
  const meta = pf.custom_metadata || {}
  if (extractInFlight.value.has(pf.id)) return 'extracting'
  if (meta.extract_status === 'failed') return 'failed'
  if (meta.extract_status === 'extracting' || pf.extract_status === 'extracting') return 'extracting'
  return 'pending'
}

// ---- 轮询 ----
let pollTimer: ReturnType<typeof setInterval> | null = null
let polling = false

/**
 * 是否「本页该管」的光伏账单文件。
 *
 * 市电与光伏共用一个知识库，两边轮询都常驻，分类判定必须双向对称：
 * 本页只看光伏痕迹，任一带电费痕迹即让路；否则两边会互相抢文件
 * （真机事故：光伏文件被市电页提取成 kind=utility_bill，光伏列表里查不到）。
 */
function isSolarBillFile(item: any): boolean {
  const meta = item?.custom_metadata || {}
  const name = String(item?.file_name || item?.title || '')
  if (meta.kind === 'utility_bill' || meta.bill_kind === 'electricity' || meta.fleet_cert_type === '市电账单') return false
  if (meta.bill_kind === 'solar' || meta.kind === 'solar_bill' || meta.fleet_cert_type === '光伏账单') return true
  // 未打标：只认文件名明确的（上传弹窗会在上传时打标，这里只做兜底自愈）
  return !meta.kind && !meta.bill_kind && !meta.fleet_cert_type && /光伏/.test(name)
}

/** 上一轮在途数量：识别「由有变无」的整批完成时刻 */
let pendingCount = 0

/**
 * 立即执行一轮扫描。
 *
 * 上传 / 重新提取后必须马上刷一次：只靠 3s 定时器的话，状态条最多晚 3s 才出现，
 * 长任务（几分钟）里用户会以为没反应。
 */
const pollOnce = async () => {
  if (!kbId.value || polling) return
  polling = true
  try {
    // 同 useBusinessPolling：单次请求悬住会冻结整个轮询 → 状态条永不收起
    const res: any = await withTimeout(listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 }))
    const data = res?.data || res?.list || []
    const arr: any[] = Array.isArray(data) ? data : []
    // 在途文件（状态条数据源）：解析中、提取中都要算。
    // 必须从**知识库原始文件列表**里筛，不能从已渲染的表格行里筛 ——
    // 在途/失败行本来就不进主列表，从行里取永远是空数组（真机踩过）。
    const pend = arr.filter((it: any) => {
      if (!isSolarBillFile(it)) return false
      const ps = it.parse_status
      const meta = it.custom_metadata || {}
      const es = meta.extract_status || it.extract_status
      if (ps === 'parsing' || ps === 'pending' || ps === 'processing' || ps === 'finalizing') return true
      if (ps === 'failed') return true
      if (es === 'failed') return true
      // 后端在途态有两种写法：extracting（旧）与 processing（asynq 队列）
      if (es === 'extracting' || es === 'processing' || es === 'pending') return true
      return es == null && ps === 'completed'
    })
    const prevPending = pendingCount
    pendingCount = pend.length
    pendingFiles.value = pend
    // 整批完成（上一轮还有在途、这一轮清零）：立刻重载列表并同步卡片计数。
    // 旧实现只在「自己触发提取」的分支里刷新 —— 从上传弹窗提取完成的文件，
    // 列表永远看不到（真机：解析提取都完成，列表里没有新账单）。
    if (prevPending > 0 && pend.length === 0) {
      await loadFiles(true)
      notifyEnergyDataChanged()
    }
    // 已解析完成且已打标 solar 但尚未提取的文件自动触发提取；
    // 文件名含「光伏」但未打标未提取的自动补打标后提取（自愈，避免上传打标失败导致列表不可见）
    for (const it of arr) {
      if (!isSolarBillFile(it)) continue
      const ps = it.parse_status
      const meta = it.custom_metadata || {}
      const isSolarName = /光伏/.test(it.file_name || '')
      if (ps === 'completed' && isSolarName && !meta.kind && !meta.bill_kind && !meta.extract_status && !taggingInFlight.value.has(it.id)) {
        taggingInFlight.value.add(it.id)
        try {
          await updateKnowledgeMetadata(it.id, { custom_metadata: { bill_kind: 'solar', fleet_cert_type: '光伏账单' } })
        } catch { /* 打标失败下轮重试 */ } finally {
          taggingInFlight.value.delete(it.id)
        }
      }
      if (ps === 'completed' && meta.bill_kind === 'solar' && !meta.kind && !meta.extract_status && !extractInFlight.value.has(it.id)) {
        extractInFlight.value.add(it.id)
        try {
          await extractSolarBill(kbId.value, it.id)
          await loadFiles(true)
          notifyEnergyDataChanged()
        } catch { /* 失败下轮重试 */ } finally {
          extractInFlight.value.delete(it.id)
        }
      }
    }
  } catch { /* 轮询失败静默 */ } finally {
    polling = false
  }
}

/** 上传 / 重新提取 / 手动提取后主动刷一次在途状态（状态条立即出现）。 */
const refreshPending = () => { void pollOnce() }

const startPolling = () => {
  stopPolling()
  void pollOnce()
  pollTimer = setInterval(() => { void pollOnce() }, 3000)
}
const stopPolling = () => {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- KB ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      await loadTags()
      await loadFiles(true)
      startPolling()
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '知识库查询失败')
  } finally {
    loading.value = false
  }
}
const onKbCreated = async (kb: any) => {
  kbId.value = kb?.id || ''
  if (kbId.value) {
    await loadTags()
    await loadFiles(true)
    startPolling()
  }
}

// ---- 上传 ----
// 统一走中台上传弹窗：分类在上传时即写入元数据，轮询按分类认领，
// 不再依赖「上传完再补打标」的竞态窗口（那正是光伏文件被市电页抢提的成因）。
const uploadVisible = ref(false)
const uploadTypeOptions = [{ label: '光伏账单', value: '光伏账单' }]
const triggerUpload = () => { uploadVisible.value = true }
/** 弹窗内整批任务全部结束：立刻刷新列表与卡片，并同步父页计数。 */
const onUploadDone = async () => {
  await loadFiles(true)
  notifyEnergyDataChanged()
  refreshPending()
}

// ---- 详情 ----
const detailVisible = ref(false)
const detailMode = ref(false)
const currentRow = ref<Row | null>(null)
const activeMenu = ref<'overview' | 'grid-fee' | 'subsidy-fee' | 'meter'>('overview')
const detailForm = ref<Record<string, any>>({})

const it = computed(() => currentRow.value?.item || {})
const detailTitle = computed(() => currentRow.value?.fileName || '账单详情')
const currentStatus = computed(() => (currentRow.value ? statusOf(currentRow.value) : { label: '--', theme: 'default' as const }))
const metricPeriod = computed(() => metricPeriodOf(it.value))

const detailFields = computed(() => [
  { key: 'bill_period_start', label: '账单周期开始', type: 'text' },
  { key: 'bill_period_end', label: '账单周期结束', type: 'text' },
  { key: 'account_no', label: '发电户号', type: 'text' },
  { key: 'account_name', label: '户名', type: 'text' },
  { key: 'supply_unit', label: '服务单位', type: 'text' },
  { key: 'address', label: '发电地址', type: 'text' },
  { key: 'taxpayer_type', label: '纳税人类型', type: 'text' },
  { key: 'consumption_mode', label: '消纳方式', type: 'text' },
  { key: 'voltage_level', label: '并网电压', type: 'text' },
  { key: 'generation_mode', label: '发电方式', type: 'text' },
  { key: 'generation_kwh', label: '发电量（千瓦时）', type: 'number' },
  { key: 'grid_kwh', label: '上网电量（千瓦时）', type: 'number' },
  { key: 'settlement_amount', label: '结算金额（元）', type: 'number' },
  { key: 'mom_change', label: '本期上网环比', type: 'text' },
  { key: 'cumulative_kwh', label: '年累计上网（千瓦时）', type: 'number' },
  { key: 'meter_prev', label: '上期示数', type: 'number' },
  { key: 'meter_curr', label: '本期示数', type: 'number' },
  { key: 'meter_ratio', label: '倍率', type: 'number' },
  { key: 'meter_reading', label: '抄见电量（千瓦时）', type: 'number' },
  { key: 'meter_bill', label: '计费电量（千瓦时）', type: 'number' },
  { key: 'tax_rate', label: '税率', type: 'text' },
  { key: 'tax_amount', label: '税额（元）', type: 'number' },
  { key: 'remark', label: '备注', type: 'text' },
])

/**
 * 详情字段跟随「字段定义（subs）」：六同步——用户在设置里改名/停用字段后，
 * 详情抽屉必须同步（旧版写死 detailFields 数组，改了没反应）。
 * subs 不可用（后端未迁移）时回退内置 detailFields。
 */
const bsdFields = computed(() => {
  const subs = (categorySubs.value || []).filter((x: any) => x && x.enabled !== false)
  if (subs.length) {
    return subs.map((x: any) => ({
      name: String(x.name),
      label: String(x.label || x.name),
      dataType: x.data_type === 'number' || x.data_type === 'amount' ? 'number' : (x.data_type || 'text'),
    }))
  }
  return detailFields.value.map((f) => ({ name: f.key, label: f.label, dataType: f.type === 'number' ? 'number' : 'text' }))
})
// BSD 表单契约 { data, remarkParts, items }：data 直接指向 detailForm，
// v-model 写 form.data[key] 即落回 detailForm，零包装成本（合同页同款做法）。
const detailFormWrapper = computed(() => ({ data: detailForm.value, remarkParts: {}, items: [] }))

// 提取质量与溯源：详情接口的 custom_metadata（光伏账单一份文件通常一条记录）
const detailQuality = ref<ExtractQuality>(EMPTY_QUALITY)
const qualityLabel = (f: string) => f

/**
 * 详情表单数据 = 记录 + 电量明细上提。
 *
 * 「字段定义」里光伏账单有 meter_prev / meter_curr / meter_ratio / meter_reading /
 * meter_bill 五个字段，而提取结果把它们放在 gateways[].readings[0]。列表列做过
 * 这层拍平所以能显示，编辑抽屉若直接铺原始 item 就是一屏空输入框
 * （真机：列表 16.91 / 18.75 / 3000 / 5520，抽屉全是「请输入」）。
 * 这里统一补齐：顶层缺失时用上网关口首行回填，保证列表与抽屉同源。
 */
function detailFormOf(item: any): Record<string, any> {
  const out: Record<string, any> = { ...(item || {}) }
  const gateways: any[] = Array.isArray(out.gateways) ? out.gateways : []
  const gw = gateways.find((g: any) => g?.gateway_type === '上网关口' && g?.readings?.length)
    || gateways.find((g: any) => g?.readings?.length)
  const rd = gw?.readings?.[0]
  if (!rd) return out
  const lift = (key: string, val: any) => {
    if (out[key] === undefined || out[key] === null || out[key] === '') out[key] = val
  }
  lift('meter_prev', rd.prev)
  lift('meter_curr', rd.curr)
  lift('meter_ratio', rd.multiplier)
  lift('meter_reading', rd.reading_kwh)
  lift('meter_bill', rd.bill_kwh)
  return out
}

const openDetail = async (row: Row) => {
  currentRow.value = row
  detailForm.value = detailFormOf(row.item)
  detailQuality.value = EMPTY_QUALITY
  detailVisible.value = true
  try {
    const kd: any = await getKnowledgeDetails(row.knowledgeId)
    const meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
    const page = Number(row.item?.page)
    detailQuality.value = parseExtractQuality(meta, Number.isFinite(page) && page > 0 ? page - 1 : undefined)
  } catch {
    // 详情拉取失败不影响查看：质量面板保持静默（历史数据本来也可能没有这些键）
    detailQuality.value = EMPTY_QUALITY
  }
}
// 字段失焦自动保存（与合同/发票一致；数字字段清洗后仅更新该字段）
/** 数字字段收敛：BSD 的 number 控件产出字符串，落库前按 detailFields 类型清洗。 */
function typedDetailForm(): Record<string, any> {
  const out: Record<string, any> = { ...detailForm.value }
  for (const f of detailFields.value) {
    if (f.type === 'number' && out[f.key] !== undefined && out[f.key] !== '') out[f.key] = Number(out[f.key]) || 0
  }
  return out
}
/**
 * 保存时把顶层 5 项示数/电量同步回 gateways[].readings[0]。
 *
 * 顶层 meter_* 是列表与抽屉的读写面，gateways 是原始提取结构（明细表、导出、
 * 后续重提都按它走）。两边不同步会出现「抽屉改了、明细表还是旧值」，
 * 所以保存时双向写：顶层照用户输入，上网关口首行同步同一份值。
 */
function syncSolarMeterReading(rec: Record<string, any>): Record<string, any> {
  const num = (v: any) => (v === '' || v === undefined || v === null ? 0 : Number(v) || 0)
  const meter = {
    prev: num(rec.meter_prev),
    curr: num(rec.meter_curr),
    multiplier: num(rec.meter_ratio),
    reading_kwh: num(rec.meter_reading),
    bill_kwh: num(rec.meter_bill),
  }
  // 顶层补齐：详情抽屉只改 gateways 之外的字段时也要保证顶层可读
  rec.meter_prev = meter.prev
  rec.meter_curr = meter.curr
  rec.meter_ratio = meter.multiplier
  rec.meter_reading = meter.reading_kwh
  rec.meter_bill = meter.bill_kwh
  const gateways: any[] = Array.isArray(rec.gateways) ? rec.gateways : []
  let gw = gateways.find((g: any) => g?.gateway_type === '上网关口')
  if (!gw) {
    gw = { gateway_type: '上网关口', meter_no: '', project_name: '', readings: [], fees: [] }
    gateways.unshift(gw)
  }
  if (!Array.isArray(gw.readings)) gw.readings = []
  if (!gw.readings.length) {
    gw.readings.push({ meter_type: '反向有功（总）', ...meter })
  } else {
    gw.readings[0] = { ...gw.readings[0], ...meter }
  }
  rec.gateways = gateways
  return rec
}

const saveDetailFields = async () => {
  if (!currentRow.value || !kbId.value) return
  try {
    const kid = currentRow.value.knowledgeId
    const kd: any = await getKnowledgeDetails(kid)
    let meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
    // 兼容历史双层结构：{custom_metadata: {...}} 取内层再保存
    if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
    meta.kind = 'solar_bill'
    meta.records = meta.records || []
    const payload = typedDetailForm()
    if (meta.records.length) {
      meta.records[0] = syncSolarMeterReading({ ...(meta.records[0] || {}), ...payload })
    } else {
      meta.records = [syncSolarMeterReading({ ...payload, gateways: currentRow.value.item?.gateways || [] })]
    }
    // records 为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由
    await updateBusinessMetadata(kbId.value, kid, meta)
    // 本地行同步
    currentRow.value.item = { ...(currentRow.value.item || {}), ...typedDetailForm() }
    MessagePlugin.success('已保存')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}
const exitDetail = () => {
  detailMode.value = false
  activeMenu.value = 'overview'
}
const gotoMenu = (menu: string) => {
  if (menu === 'grid-fee') activeMenu.value = 'grid-fee'
  else if (menu === 'subsidy-fee') activeMenu.value = 'subsidy-fee'
  else activeMenu.value = 'meter'
}

// ---- 概览数据 ----
const gridGateways = computed(() => (it.value.gateways || []).filter((g: any) => g.gateway_type === '上网关口'))
const subsidyGateways = computed(() => (it.value.gateways || []).filter((g: any) => g.gateway_type === '发电关口'))
const gatewayRows = computed(() => it.value.gateways || [])

const DEFAULT_FEE_COLS = [
  { field_key: 'category', label: '费用组成', field_type: 'text' },
  { field_key: 'qty', label: '计费数量', field_type: 'number' },
  { field_key: 'rate', label: '电价', field_type: 'number' },
  { field_key: 'fee', label: '电费', field_type: 'amount' },
]
const gridFeeCols = ref<{ field_key: string; label: string; field_type: string }[]>(DEFAULT_FEE_COLS)
const subsidyFeeCols = ref<{ field_key: string; label: string; field_type: string }[]>(DEFAULT_FEE_COLS)

const gridFeeRows = computed(() => gridGateways.value.flatMap((g: any) => g.fees || []))
const subsidyFeeRows = computed(() => subsidyGateways.value.flatMap((g: any) => g.fees || []))
const gridFeeTotal = computed(() => gridFeeRows.value.reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0))
const subsidyFeeTotal = computed(() => subsidyFeeRows.value.reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0))

const overviewRows = computed(() => [
  { key: 'grid', label: '上网电费', qtyText: fmtKwh(gridFeeRows.value.reduce((s: number, f: any) => s + (Number(f.qty) || 0), 0)), value: gridFeeTotal.value, menuKey: 'grid-fee' },
  { key: 'subsidy', label: '发电补助', qtyText: fmtKwh(subsidyFeeRows.value.reduce((s: number, f: any) => s + (Number(f.qty) || 0), 0)), value: subsidyFeeTotal.value, menuKey: 'subsidy-fee' },
])
const overviewTotal = computed(() => gridFeeTotal.value + subsidyFeeTotal.value)

const feeCellText = (f: any, col: any) => {
  const v = f[col.field_key]
  if (v === undefined || v === null || v === '') return ''
  if (col.field_type === 'number') return fmtRate6(Number(v))
  if (col.field_type === 'amount') return fmtMoney(Number(v))
  return String(v)
}
const displayMultiplier = (r: any) => {
  if (r?.multiplier !== undefined && r.multiplier !== null && Number(r.multiplier) !== 0) return fmtRate6(r.multiplier)
  return ''
}

// ---- 费用编辑 ----
const feeEditVisible = ref(false)
const feeEditForm = ref<Record<string, any>>({})
const feeEditTitle = ref('')
let feeEditTarget: { group: 'grid' | 'subsidy'; index: number } | null = null
const openFeeEdit = (group: 'grid' | 'subsidy', index: number) => {
  const list = group === 'grid' ? gridFeeRows.value : subsidyFeeRows.value
  const f = list[index]
  if (!f) return
  feeEditTarget = { group, index }
  feeEditForm.value = { ...f }
  feeEditTitle.value = f.category || '费用项'
  feeEditVisible.value = true
}
const recalcFeeEdit = () => {
  const qty = Number(feeEditForm.value.qty) || 0
  const rate = Number(feeEditForm.value.rate) || 0
  if (rate !== 0 && qty !== 0) {
    feeEditForm.value.fee = Math.round(qty * rate * 100) / 100
  }
}
const saveFeeEdit = async () => {
  if (!currentRow.value || !kbId.value || !feeEditTarget) return
  const { group, index } = feeEditTarget
  const gws = (it.value.gateways || []).filter((g: any) =>
    group === 'grid' ? g.gateway_type === '上网关口' : g.gateway_type === '发电关口')
  let cursor = 0
  let gw: any = null
  let feeIdx = -1
  for (const g of gws) {
    const fees = Array.isArray(g.fees) ? g.fees : []
    if (index < cursor + fees.length) { gw = g; feeIdx = index - cursor; break }
    cursor += fees.length
  }
  if (!gw || feeIdx < 0) return
  if (!Array.isArray(gw.fees)) gw.fees = []
  gw.fees[feeIdx] = { ...(gw.fees[feeIdx] || {}), ...feeEditForm.value }
  try {
    const kid = currentRow.value.knowledgeId
    const kd: any = await getKnowledgeDetails(kid)
    let meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
    if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
    meta.kind = 'solar_bill'
    meta.records = meta.records || []
    if (meta.records.length) meta.records[0] = { ...(meta.records[0] || {}), gateways: it.value.gateways }
    else meta.records = [{ ...detailForm.value, gateways: it.value.gateways }]
    // records 为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由
    await updateBusinessMetadata(kbId.value, kid, meta)
    currentRow.value.item = { ...currentRow.value.item, gateways: [...(it.value.gateways || [])] }
    MessagePlugin.success('已保存')
    feeEditVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

// 上传 / 删除 / 提取落库后广播：父页 KPI 卡片计数跟着重算（口径铁律）。
// ---- 浮动工具栏 ----
const handleReExtract = async () => {
  const row = selectedSingle.value
  if (!row || !kbId.value) return
  try {
    // 与电费/合同一致：重新解析并提取（解析完成后由轮询自动触发字段提取）
    await reparseKnowledge(row.knowledgeId)
    MessagePlugin.success(`已触发「${row.fileName}」重新解析与提取`)
    refreshPending()
    setTimeout(() => { loadFiles(true); refreshPending() }, 1500)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新提取失败')
  }
}
const handleBatchEdit = async () => {
  const row = selectedSingle.value
  if (!row) return
  clearSelection()
  await openDetail(row)
}
const handleBatchDelete = async () => {
  const ids = selectedRows.value.map(r => r.knowledgeId)
  if (!ids.length) return
  try {
    await deleteSolarRecords(ids)
    MessagePlugin.success('已移至删除历史')
    clearSelection()
    loadFiles(true)
    notifyEnergyDataChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// 删除记录：调用知识库批量删除接口（软删到删除历史）
const deleteSolarRecords = async (ids: string[]) => {
  const { batchDeleteKnowledge } = await import('@/api/knowledge-base')
  await batchDeleteKnowledge(kbId.value, ids)
}

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagManageVisible = ref(false)
const tagTargetName = ref('')
const tagTargetTags = ref<any[]>([])
let tagTargetRow: Row | null = null
const loadTags = async () => {
  try {
    const res: any = await listKnowledgeTags(kbId.value)
    tagList.value = Array.isArray(res?.data || res) ? (res.data || res) : []
  } catch { /* ignore */ }
}
const rowTags = (row: Row) => row.tags || []
const openTagEdit = (row: Row) => {
  if (row.kind === 'pending') return
  tagTargetRow = row
  tagTargetName.value = row.fileName
  tagTargetTags.value = row.tags || []
  tagDialogVisible.value = true
}
const onTagEditConfirm = async (tags: any[]) => {
  if (!tagTargetRow || !kbId.value) return
  try {
    await updateKnowledgeTagBatch({ updates: { [tagTargetRow.knowledgeId]: tags.map((t: any) => t.id || t.tag_id || t) } })
    tagTargetRow.tags = tags
    MessagePlugin.success('标签已更新')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '标签更新失败')
  }
}
const onTagCreated = async () => { await loadTags() }
const onTagManageChanged = async () => { await loadTags() }
const openTagManage = () => { tagManageVisible.value = true }

// ---- 打印（合并多份账单为一个 PDF，iframe 预览，与电费一致） ----
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff']
const isImageRow = (r: any, blob: any) => {
  const ext = String((r as any).fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const printVisible = ref(false)
const printUrl = ref('')
const printBytes = ref<Uint8Array | ArrayBuffer | null>(null)
const printCount = ref(1)
const printBusy = ref(false)
const closePrint = () => {
  printVisible.value = false
  printUrl.value = ''
  printBytes.value = null
}
const handleBatchPrint = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  printBusy.value = true
  printUrl.value = ''
  printCount.value = rowsSel.length
  printVisible.value = true
  try {
    const out = await PDFDocument.create()
    const notes: string[] = []
    let mergedPages = 0
    for (const r of rowsSel) {
      const blob: any = await previewKnowledgeFile(r.knowledgeId)
      if (!blob) { notes.push(`${r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        if (isImageRow(r, blob)) {
          const ext = String((r as any).fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
      } catch {
        notes.push(`${r.fileName}（该文件无法合并，可单独打印）`)
      }
    }
    if (mergedPages === 0) {
      MessagePlugin.error('未获取到可打印的账单页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    printBytes.value = bytes
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    if (notes.length) setTimeout(() => MessagePlugin.warning(notes.join('；')), 300)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
    printVisible.value = false
  } finally {
    printBusy.value = false
  }
}

// ---- 格式化 ----
const fmtKwh = (v: any) => {
  if (v === undefined || v === null || v === '') return ''
  const n = Number(v)
  if (Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const fmtMoney = (v: any) => {
  if (v === undefined || v === null || v === '') return ''
  const n = Number(v)
  if (Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
const fmtRate6 = (v: any) => {
  if (v === undefined || v === null || v === '') return ''
  const n = Number(v)
  if (Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
}

onMounted(() => {
  loadKb()
  void loadCategorySubs()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
function onCategoriesChanged() { void loadCategorySubs() }
onBeforeUnmount(() => {
  stopPolling()
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})
</script>

<style lang="less" scoped>
/* 中台列表共享样式：.doc-list-scroll / .doc-list-view / 表头遮罩 / 表格令牌。
   子组件是挂在 .biz-page 下的（父页面提供），但共享样式是按组件独立 import 的，
   不引入就会退化成 display:block + overflow:visible —— 表格不被限高、
   滚动条落在外层容器上，表现就是「滚动条侵入表头 + 汇总栏被顶到几千像素」。 */
@import './styles/business-list-shared.css';

.solar-management-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px 24px;
  box-sizing: border-box;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 12px;

  .header-title {
    h2 {
      margin: 0 0 4px;
      font-size: 20px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .header-subtitle {
      margin: 0;
      font-size: 13px;
      color: var(--td-text-color-secondary);
    }
  }

  .header-actions {
    display: flex;
    gap: 8px;
  }
}

.loading-area, .empty-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  height: 100%;
  min-height: 320px;
}
/* 只有转圈时不需要 gap */
.loading-area { gap: 0; }

.solar-main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* 筛选工具栏（与合同/电费一致） */
.doc-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;

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
}
.doc-filter-field { display: flex; align-items: center; }
.doc-filter-field--search { flex: 1; min-width: 200px; max-width: 320px; }
.doc-filter-field__control { width: 100%; }
.doc-date-range { width: 160px; }

/* 列表 */
.doc-list-scroll {
  /* 只保留尺寸：滚动交给 t-table 内部（sticky-header + max-height:100%），
     外层写 overflow-y:auto 会让滚动条从容器顶端开始、侵入表头标题栏。 */
  flex: 1;
  max-height: 100%;
  min-width: 0;
  min-height: 0;
}
.doc-list-view { display: flex; flex-direction: column; }
.doc-list-header, .doc-list-row {
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
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-stroke);
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  .cell {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
.doc-list-row {
  position: relative;
  min-height: 52px;
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  font-size: 13px;
  color: var(--td-text-color-primary);
  transition: background-color 0.2s ease;
  &:last-child { border-bottom: 0; }
  &:hover { background: var(--td-bg-color-secondarycontainer); }
  &.selected { background: var(--td-brand-color-1); }
  &.is-pending { cursor: default; }
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
}
.cell-check { display: flex; align-items: center; justify-content: center; }
.cell-extractStatus { display: flex; align-items: center; justify-content: center; }
.cell-tags { display: flex; align-items: center; }
.row-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-mono { font-family: var(--app-font-family); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-status-tag { min-width: 64px; justify-content: center; }
.row-tag-chips {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-wrap: nowrap;
  cursor: pointer;

  .row-tag {
    max-width: 110px;

    :deep(.t-tag__text) {
      max-width: 100px;
      overflow: hidden;
      text-overflow: ellipsis;
      display: inline-block;
    }
  }
}

.row-tag-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 20px;
  min-width: 20px;
  padding: 0 4px;
  border-radius: 999px;
  border: 1px solid var(--td-component-stroke);
  color: var(--td-text-color-secondary);
  font-size: 10px;
}

.row-tag-add {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  border: 1px dashed var(--td-component-stroke);
  border-radius: 999px;
  padding: 0 6px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  white-space: nowrap;

  &:hover {
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
    border-style: solid;
  }
}
.icon-spin { animation: solar-icon-spin 1s linear infinite; }
@keyframes solar-icon-spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
.list-loading { display: flex; justify-content: center; padding: 24px; }
.list-empty { display: flex; justify-content: center; padding: 48px 0; }

/* 底部汇总 */
.doc-list-footer-summary {
  display: flex;
  gap: 24px;
  align-items: center;
  padding: 6px 2px 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
  &.with-toolbar { margin-bottom: 56px; }
  .summary-selected { color: var(--td-brand-color); }
}

/* 浮动工具栏 */
.doc-batch-bar-fixed {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: 32px;
  z-index: 999;
}
.batch-bar-inner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 14px;
  border-radius: 8px;
  background: var(--td-bg-color-container);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.14);
  border: 1px solid var(--td-component-stroke);
}
.batch-bar-left { display: flex; align-items: center; gap: 8px; }
.batch-bar-count { font-size: 13px; color: var(--td-text-color-primary); }
.batch-bar-actions { display: flex; align-items: center; gap: 6px; }
.batch-bar-fade-enter-active, .batch-bar-fade-leave-active { transition: opacity 0.2s ease, transform 0.2s ease; }
.batch-bar-fade-enter-from, .batch-bar-fade-leave-to { opacity: 0; transform: translate(-50%, 8px); }

/* 详情视图 */
.bill-detail-layout { display: flex; flex-direction: column; flex: 1; min-height: 0; }
.bill-detail-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0 12px;
  .bill-detail-title { font-size: 16px; font-weight: 600; color: var(--td-text-color-primary); }
}
.bill-detail-body {
  display: flex;
  flex: 1;
  min-height: 0;
  gap: 16px;
}
.bill-nav {
  width: 168px;
  flex-shrink: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  padding: 8px;
  overflow-y: auto;
}
.bill-nav-group { margin-bottom: 4px; }
.bill-nav-group-title {
  padding: 6px 10px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.bill-nav-item {
  padding: 8px 10px;
  border-radius: 4px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  &:hover { background: var(--td-bg-color-container-hover); }
  &.active { background: var(--td-brand-color-light); color: var(--td-brand-color); font-weight: 600; }
}
.bill-nav-item--child { padding-left: 22px; }
.bill-content { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 12px; overflow-y: auto; padding-right: 2px; }

/* 卡片 */
.bill-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  padding: 14px 16px;
}
.bill-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.bill-card-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.bill-card-hint { font-size: 12px; color: var(--td-text-color-secondary); }

/* 基础信息 */
.solar-info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 24px;
}
.solar-info-item { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
.solar-info-label { flex-shrink: 0; font-size: 12px; color: var(--td-text-color-secondary); min-width: 56px; text-align: right; }
.solar-info-value { font-size: 13px; color: var(--td-text-color-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* 指标卡 */
.overview-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.metric-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  padding: 14px 16px;
  .metric-label { font-size: 12px; color: var(--td-text-color-secondary); }
  .metric-value { font-size: 22px; font-weight: 600; color: var(--td-text-color-primary); margin-top: 6px; }
  .metric-unit { font-size: 12px; font-weight: 400; color: var(--td-text-color-secondary); margin-left: 4px; }
}

/* 两列 */
.overview-two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-items: start;
}

/* 分析 */
.solar-analysis { display: flex; flex-direction: column; gap: 10px; }
.solar-analysis-row { display: flex; align-items: baseline; gap: 10px; }
.solar-analysis-label { flex-shrink: 0; font-size: 12px; color: var(--td-text-color-secondary); min-width: 108px; text-align: right; }
.solar-analysis-value { font-size: 13px; color: var(--td-text-color-primary); }
.solar-analysis-note { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 420px; }

/* 概览表格 */
.os-table { display: flex; flex-direction: column; }
.os-row {
  display: grid;
  grid-template-columns: 1.4fr 0.9fr 0.9fr;
  gap: 8px;
  align-items: center;
  padding: 8px 4px;
  border-bottom: 1px solid var(--td-component-stroke);
  font-size: 13px;
  &.os-row--head { font-weight: 600; color: var(--td-text-color-secondary); font-size: 12px; }
  &.os-row--total { font-weight: 600; }
  &.os-row--link { cursor: pointer; &:hover { color: var(--td-brand-color); } }
}
.os-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.os-qty { text-align: right; }
.os-amount { text-align: right; font-family: 'JetBrains Mono', Consolas, monospace; }
.os-neg { color: var(--td-error-color); }

/* 明细表格 */
.fee-group-table { display: flex; flex-direction: column; }
.fg-row {
  display: grid;
  grid-template-columns: 1.4fr 0.9fr 0.9fr 0.9fr;
  gap: 8px;
  align-items: center;
  padding: 8px 4px;
  border-bottom: 1px solid var(--td-component-stroke);
  font-size: 13px;
  cursor: pointer;
  &:hover { background: var(--td-bg-color-container-hover); }
  &.fg-head { font-weight: 600; color: var(--td-text-color-secondary); font-size: 12px; cursor: default; }
  &.fg-total { font-weight: 600; cursor: default; }
}
.fg-empty { padding: 32px 0; text-align: center; color: var(--td-text-color-secondary); font-size: 13px; }
.fg-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* 详情抽屉 */
.utility-detail-drawer { display: flex; flex-direction: column; gap: 16px; }
.us-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2100;
  cursor: col-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  .us-resize-line {
    width: 2px;
    height: 40px;
    border-radius: 1px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity 0.15s ease, height 0.15s ease;
  }
  &:hover .us-resize-line, .us-resize-line:hover {
    opacity: 1;
    height: 80px;
  }
}
.detail-block { border: 1px solid var(--td-component-stroke); border-radius: 6px; overflow: hidden; }
.detail-block-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-secondarycontainer);
  cursor: pointer;
}
.detail-block-caret { color: var(--td-text-color-secondary); }
.detail-block-content { padding: 14px; }
.summary-lines { font-size: 13px; line-height: 1.7; color: var(--td-text-color-primary); white-space: pre-wrap; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.field-grid-item { display: flex; flex-direction: column; gap: 4px; }
.field-grid-item--full { grid-column: 1 / -1; }
.field-label { font-size: 12px; color: var(--td-text-color-secondary); }

/* 编辑抽屉 */
.edit-drawer-body { display: flex; flex-direction: column; gap: 14px; }

.edit-drawer-footer {
  display: flex;
  justify-content: flex-end;
  padding-top: 10px;
  border-top: 1px solid var(--td-component-stroke);
}
.edit-field { display: flex; flex-direction: column; gap: 4px; }
.edit-label { font-size: 12px; color: var(--td-text-color-secondary); }
.edit-note { font-size: 12px; color: var(--td-text-color-secondary); line-height: 1.6; }

</style>
