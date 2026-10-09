<template>
  <div class="contract-management-container">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>合同管理</h2>
        <p class="header-subtitle">合同档案自动归档</p>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化合同管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-合同」知识库">
        <template #image><t-icon name="file-copy" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建合同知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="contract-main">
      <!-- KPI 概览卡（黄金基准 DashboardKpiGroup：轻量聚合自列表数据，不依赖后端统计接口） -->
      <DashboardKpiGroup :cards="overviewCards" @card-click="onOverviewCardClick" />
      <!-- 筛选工具栏（复用中台组件，直接位于列表容器，对齐发票基准：右组按钮由组件内部 margin-left:auto 贴右） -->
      <BusinessListToolbar v-model:keyword="keyword" search-placeholder="搜索全部字段"
          :type-options="contractTypeOptions" v-model:type-value="filterContractType" :type-clearable="searchActive"
          @refresh="applyFilter" @print="handleBatchPrint" :selected-count="selectedRowKeys.length"
          @clear-selection="clearSelection">
          <template #type-extra>
            <div class="doc-filter-field">
              <t-select v-model="filterFulfillStatus" :options="fulfillStatusOptions" placeholder="履约状态"
                class="doc-type-select doc-filter-field__control" clearable>
                <template #prefixIcon><t-icon name="check-circle" size="16px" /></template>
              </t-select>
            </div>
            <div class="doc-filter-field doc-filter-field--wide">
              <t-date-range-picker v-model="dateRange" placeholder="签订日期" class="doc-date-range doc-filter-field__control"
                clearable allow-input @change="applyFilter">
                <template #prefixIcon><t-icon name="time" size="16px" /></template>
              </t-date-range-picker>
            </div>
          </template>
          <template #columns>
            <BusinessColumnFilter :columns="effectiveColumns" v-model:visibleKeys="visibleColKeys" @reset="resetColumns" @select-all="selectAllColumns" />
          </template>
          <template #right-extra>
            <t-button variant="outline" size="small" @click="settingsVisible = true">
              <template #icon><t-icon name="setting" size="14px" /></template>
              设置
            </t-button>
            <t-button theme="primary" size="small" @click="uploadVisible = true">
              <template #icon><t-icon name="upload" size="14px" /></template>
              上传合同
            </t-button>
          </template>
          <template #batch-actions>
            <t-popconfirm theme="warning"
              :content="`确定重新解析并提取「${selectedSingle?.fileName || '该文件'}」吗？将覆盖已有提取结果。`"
              :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleReExtract">
              <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                <template #icon><t-icon name="refresh" size="14px" /></template>
                重新提取
              </t-button>
            </t-popconfirm>
            <t-button theme="default" variant="outline" size="small" @click="handleBatchPrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              单页打印
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handleBatchCatalog">
              <template #icon><t-icon name="file-paste" size="14px" /></template>
              列表打印
            </t-button>
            <!-- 删除确认：单气泡内嵌二选一（多页文件「删除此页/删除文件」，否则普通确认），杜绝二次弹窗 -->
            <t-popconfirm theme="warning" v-model:visible="delPopVisible" placement="top"
              :confirm-btn="null" :cancel-btn="null" :popup-props="{ overlayStyle: { width: 'auto' } }">
              <template #content>
                <div class="del-pop-body">
                  <div v-if="delMultiCount > 0" class="del-pop-title">
                    选中的合同涉及 {{ delMultiCount }} 份多页文件，请选择删除范围：
                  </div>
                  <div v-else class="del-pop-title">确定删除所选 {{ selectedRowKeys.length }} 个合同记录吗？删除后不可恢复。</div>
                  <div class="del-pop-actions">
                    <template v-if="delMultiCount > 0">
                      <t-button size="small" theme="danger" @click="confirmPageDelete">删除此页</t-button>
                      <t-button size="small" theme="danger" variant="outline" @click="confirmFileDelete">删除文件</t-button>
                      <t-button size="small" variant="text" @click="closeDelPop">取消</t-button>
                    </template>
                    <template v-else>
                      <t-button size="small" theme="danger" @click="confirmPageDelete">删除</t-button>
                      <t-button size="small" variant="text" @click="closeDelPop">取消</t-button>
                    </template>
                  </div>
                </div>
              </template>
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                删除记录
              </t-button>
            </t-popconfirm>
          </template>
        </BusinessListToolbar>

      <!-- 合同列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
        <div class="doc-list-view">
          <t-table
        ref="contractTableRef"
        :data="filteredRows"
        :columns="tableColumns"
        row-key="rowKey"
        size="small"
        :hover="true"
        :loading="listLoading"
        max-height="100%"
        sticky-header
        class="doc-table"
        :selected-row-keys="selectedRowKeys"
        select-on-change
        :row-class-name="({ row }: any) => selectedRowKeys.includes(row.rowKey) ? 'is-selected' : ''"
        @row-click="({ row }: any) => openDetail(row)"
        @select-change="(val: string[]) => onSelectChange(val)"
      >
        <template #contractNo="{ row }: any">
          <span class="row-contract-no" :title="row.contractNo || row.fileName">{{ row.contractNo }}</span>
        </template>
        <template #contractName="{ row }: any">
          <span class="row-text" :title="row.contractName || row.fileName">{{ row.contractName || row.fileName }}</span>
        </template>
        <template #contractType="{ row }: any">
          <span class="row-text" :title="row.contractType">{{ row.contractType }}</span>
        </template>
        <template #partyAName="{ row }: any">
          <span class="row-text" :title="row.partyAName">{{ row.partyAName }}</span>
        </template>
        <template #partyBName="{ row }: any">
          <span class="row-text" :title="row.partyBName">{{ row.partyBName }}</span>
        </template>
        <template #signDate="{ row }: any">
          <span class="row-mono">{{ row.signDate }}</span>
        </template>
        <template #expiryDate="{ row }: any">
          <span class="row-mono">{{ row.expiryDate }}</span>
        </template>
        <template #contractAmount="{ row }: any">
          <span class="row-mono row-amount">{{ formatAmount(row.contractAmount) }}</span>
        </template>
        <template #taxRate="{ row }: any">
          <span class="row-mono row-rate">{{ formatRate(row.taxRate) }}</span>
        </template>
        <template #extractStatus="{ row }: any">
          <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
            variant="light-outline" class="row-status-tag">
            <template v-if="statusOf(row).icon" #icon>
              <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
            </template>
            {{ statusOf(row).label }}
          </t-tag>
          <span v-else class="row-muted">--</span>
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
          <span v-else class="row-tag-chips is-clickable" @click.stop="openTagEdit(row)">
            <span class="row-tag-add">+ 标签</span>
          </span>
        </template>
        <template #fulfillStatus="{ row }: any">
          <t-tag v-if="row.fulfillStatus" size="small" :theme="fulfillStatusTheme(row.fulfillStatus)" variant="light-outline">
            {{ row.fulfillStatus }}
          </t-tag>
          <span v-else class="row-muted">--</span>
        </template>
        <template #partyATaxNo="{ row }: any">
          <span class="row-mono" :title="row.partyATaxNo">{{ row.partyATaxNo }}</span>
        </template>
        <template #partyBTaxNo="{ row }: any">
          <span class="row-mono" :title="row.partyBTaxNo">{{ row.partyBTaxNo }}</span>
        </template>
        <template #effectiveDate="{ row }: any">
          <span class="row-mono">{{ row.effectiveDate }}</span>
        </template>
        <template #paymentMethod="{ row }: any">
          <span class="row-text">{{ row.paymentMethod }}</span>
        </template>
        <template #handler="{ row }: any">
          <span class="row-text">{{ row.handler }}</span>
        </template>
        <template #department="{ row }: any">
          <span class="row-text">{{ row.department }}</span>
        </template>
        <template #fileName="{ row }: any">
          <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
        </template>
      </t-table>
    </div>
      <!-- 底部汇总（列表容器内部底部固定，不随表格滚动；发票基准同款位置与样式） -->
      <div class="doc-summary-bar">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 份</span>
        <span v-if="displaySummary.total" class="doc-summary-item">
          合同金额合计 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTotal) }}</span>
        </span>
      </div>
    </div>

    </div>

    <!-- 创建知识库向导 -->
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-合同" kb-desc="合同管理固定使用专用知识库，名称不可修改" description-placeholder="用于存放并解析合同文件，自动提取合同字段" default-description="用于存放并解析合同文件，自动提取合同字段" model-tip="提取模型将复用下方「对话模型」，用于解析合同字段。若列表为空，请先在系统设置中添加模型。" @created="onKbCreated" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab，黄金基准 StandardSettingDrawer） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" :scope="'contract'"
      kb-name="日常事务-合同" title="合同设置" @saved="() => reloadColumns(true)" />

    <!-- 上传弹窗（对齐发票/车队：分类选择 + 拖拽/选择/粘贴 + 进度 + 防重检测） -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="contract" title="上传合同"
      :type-options="uploadTypeOptions" @done="onUploadDone" />

    <!-- 历史记录（对齐发票基准：已上传文件抽屉，展示解析/提取状态与合同类型） -->
    <FleetUploadHistoryDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" scope="contract"
      @changed="onUploadHistoryChanged" />


    <!-- 合同详情抽屉（竖向区块，可拖宽，上下滚动） -->
<SettingDrawer v-model:visible="detailVisible" :title="detailTitle" width="700px" :storage-key="'weknora-contract-drawer-width'" hide-footer destroy-on-close class="contract-detail-drawer">
      <div class="contract-detail-body">
        <!-- 合同字段（手动保存模式：不再自动保存，底部「保存/取消」统一提交） -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">合同信息</div>
                <div class="field-grid">
                  <t-form-item label="合同编号" label-width="110px">
                    <t-input v-model="editForm.contract_no" placeholder="无编号时自动生成" />
                  </t-form-item>
                  <t-form-item label="合同名称" label-width="110px">
                    <t-input v-model="editForm.contract_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="合同类型" label-width="110px">
                    <t-select v-model="editForm.contract_type" :options="contractTypeRaw" clearable filterable allow-create
                      placeholder="选择类型" />
                  </t-form-item>
                  <t-form-item label="签订日期" label-width="110px">
                    <t-date-picker v-model="editForm.sign_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="生效日期" label-width="110px">
                    <t-date-picker v-model="editForm.effective_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="到期日期" label-width="110px">
                    <t-date-picker v-model="editForm.expiry_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="签订地点" label-width="110px">
                    <t-input v-model="editForm.sign_place" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">甲方</div>
                <div class="field-grid">
                  <t-form-item label="名称" label-width="110px">
                    <t-input v-model="editForm.party_a_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="统一社会信用代码" label-width="110px">
                    <t-input v-model="editForm.party_a_tax_no" placeholder="" />
                  </t-form-item>
                  <t-form-item label="地址" label-width="110px">
                    <t-input v-model="editForm.party_a_address" placeholder="" />
                  </t-form-item>
                  <t-form-item label="电话" label-width="110px">
                    <t-input v-model="editForm.party_a_phone" placeholder="" />
                  </t-form-item>
                  <t-form-item label="开户行" label-width="110px">
                    <t-input v-model="editForm.party_a_bank" placeholder="" />
                  </t-form-item>
                  <t-form-item label="账号" label-width="110px">
                    <t-input v-model="editForm.party_a_account" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">乙方</div>
                <div class="field-grid">
                  <t-form-item label="名称" label-width="110px">
                    <t-input v-model="editForm.party_b_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="统一社会信用代码" label-width="110px">
                    <t-input v-model="editForm.party_b_tax_no" placeholder="" />
                  </t-form-item>
                  <t-form-item label="地址" label-width="110px">
                    <t-input v-model="editForm.party_b_address" placeholder="" />
                  </t-form-item>
                  <t-form-item label="电话" label-width="110px">
                    <t-input v-model="editForm.party_b_phone" placeholder="" />
                  </t-form-item>
                  <t-form-item label="开户行" label-width="110px">
                    <t-input v-model="editForm.party_b_bank" placeholder="" />
                  </t-form-item>
                  <t-form-item label="账号" label-width="110px">
                    <t-input v-model="editForm.party_b_account" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">金额信息</div>
                <div class="field-grid">
                  <t-form-item label="合同金额" label-width="110px">
                    <t-input v-model="editForm.contract_amount" placeholder="" @input="(v: string) => (editForm.contract_amount = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="税率" label-width="110px">
                    <t-input :value="editForm.tax_rate_display" placeholder="" suffix="%"
                      @input="(v: string) => (editForm.tax_rate_display = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="付款方式" label-width="110px">
                    <t-select v-model="editForm.payment_method" :options="paymentMethodOptions" clearable
                      placeholder="一次性/分期/按进度" />
                  </t-form-item>
                  <t-form-item label="质保金" label-width="110px">
                    <t-input v-model="editForm.quality_bond" placeholder="" @input="(v: string) => (editForm.quality_bond = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="违约金" label-width="110px">
                    <t-input v-model="editForm.liquidated_damages" placeholder="" @input="(v: string) => (editForm.liquidated_damages = sanitizeNum(v))" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">项目内容</div>
                <div class="field-grid">
                  <t-form-item label="标的" label-width="110px">
                    <t-input v-model="editForm.subject" placeholder="" />
                  </t-form-item>
                  <t-form-item label="数量" label-width="110px">
                    <t-input v-model="editForm.quantity" placeholder="" @input="(v: string) => (editForm.quantity = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="单价" label-width="110px">
                    <t-input v-model="editForm.unit_price" placeholder="" @input="(v: string) => (editForm.unit_price = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="履行期限" label-width="110px">
                    <t-input v-model="editForm.performance_period" placeholder="" />
                  </t-form-item>
                  <t-form-item label="经办人" label-width="110px">
                    <t-input v-model="editForm.handler" placeholder="" />
                  </t-form-item>
                  <t-form-item label="部门" label-width="110px">
                    <t-input v-model="editForm.department" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">备注</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="备注" label-width="110px">
                    <t-textarea v-model="editForm.remark" :autosize="{ minRows: 2, maxRows: 5 }" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="detail-save-hint">
                <t-icon name="check-circle" size="14px" />
                <span>修改后点击底部「保存」提交{{ saving ? '（保存中...）' : '' }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 底部操作：手动保存/取消（黄金基准发票同款心智） -->
        <div class="detail-footer-actions">
          <t-button variant="outline" size="medium" @click="detailVisible = false">取消</t-button>
          <t-button theme="primary" size="medium" :loading="saving" @click="saveContractDetail">保存</t-button>
        </div>

        <!-- 源文件预览 -->
        <section class="detail-block">
          <div class="detail-block-title">源文件预览</div>
          <div class="detail-block-content">
            <DocumentPreview v-if="currentRow" :knowledge-id="currentRow.knowledgeId"
              :file-type="currentRow?.fileType || ''" :file-name="currentRow?.fileName || ''" :active="true" />
          </div>
        </section>
      </div>
    </SettingDrawer>

    <!-- 标签编辑（复用原项目组件） -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理抽屉（复用原项目组件） -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 打印预览弹窗（自定义实现，完全可控；弃用 TDesign dialog） -->
    <teleport to="body">
      <div v-if="printVisible" class="contract-print-mask">
        <div class="contract-print-dialog">
          <div class="contract-print-header">
            <span class="contract-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 份）` }}</span>
            <t-button variant="text" size="small" class="contract-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="contract-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并合同 PDF…'" />
            </div>
          </div>
          <div class="contract-print-footer">
            <t-button variant="outline" size="small" @click="printVisible = false">关闭</t-button>
            <t-button v-if="printMode === 'catalog'" variant="outline" size="small" :disabled="!printUrl"
              @click="downloadCatalogPdf">
              <template #icon><t-icon name="download" size="14px" /></template>
              下载
            </t-button>
            <t-button theme="primary" size="small" :loading="printBusy" :disabled="!printUrl" @click="doPrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
          </div>
        </div>
      </div>
    </teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { listFleetCategories } from '@/api/fleet'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import type { KpiCard } from '@/components/business/DashboardKpiGroup.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessListToolbar from './BusinessListToolbar.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { colWidthOf } from './columnWidth'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  delKnowledgeDetails,
  batchDeleteKnowledge,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractBusinessDocument,
  deleteContractPage,
  listContractRecords,
  previewKnowledgeFile,
  reparseKnowledge,
  getRecognitionConfig,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import BusinessColumnFilter from './BusinessColumnFilter.vue'
import { useBusinessPolling } from '@/composables/useBusinessPolling'

const KB_NAME = '日常事务-合同'
const PAGE_SIZE = 20

// 履约状态为系统按到期日期自动派生的动态字段（非提取字段，不落库人工值）：
// 已超期（早于今天）/ 即将到期（未来 30 天内）/ 履行中（其余，含未填到期日期）
const FULFILL_STATUSES = ['履行中', '即将到期', '已超期']
const EXPIRY_WARN_DAYS = 30
// 按到期日期派生履约状态（黄金基准：状态列自动计算，不依赖模型提取值）
const calcFulfillStatus = (expiryDate?: string): string => {
  if (!expiryDate) return ''
  const d = new Date(expiryDate)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  now.setHours(0, 0, 0, 0)
  const diff = Math.round((d.getTime() - now.getTime()) / 86400000)
  if (diff < 0) return '已超期'
  if (diff <= EXPIRY_WARN_DAYS) return '即将到期'
  return '履行中'
}
// 付款方式枚举
const PAYMENT_METHODS = ['一次性', '分期', '按进度']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
const DEFAULT_CONTRACT_COLUMNS: ColumnDef[] = [
  { key: 'contractNo', label: '合同编号', default: true, w: '1.4fr' },
  { key: 'contractName', label: '合同名称', default: true, w: '1.6fr' },
  { key: 'contractType', label: '合同类型', default: true, w: '1fr' },
  { key: 'partyAName', label: '甲方', default: true, w: '1.4fr' },
  { key: 'partyBName', label: '乙方', default: true, w: '1.4fr' },
  { key: 'signDate', label: '签订日期', default: true, w: '1.1fr' },
  { key: 'expiryDate', label: '到期日期', default: true, w: '1.1fr' },
  { key: 'contractAmount', label: '合同金额', default: true, w: '1.2fr' },
  { key: 'taxRate', label: '税率', default: true, w: '0.7fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'fulfillStatus', label: '履约状态', default: true, w: '1fr' },
  { key: 'partyATaxNo', label: '甲方税号', default: false, w: '1.3fr' },
  { key: 'partyBTaxNo', label: '乙方税号', default: false, w: '1.3fr' },
  { key: 'effectiveDate', label: '生效日期', default: false, w: '1.1fr' },
  { key: 'paymentMethod', label: '付款方式', default: false, w: '1fr' },
  { key: 'handler', label: '经办人', default: false, w: '0.9fr' },
  { key: 'department', label: '部门', default: false, w: '0.9fr' },
  // 详情字段（默认不占列表宽度，与编辑抽屉表单同源，可在字段筛选器中开启）
  { key: 'signPlace', label: '签订地点', default: false, w: '1.2fr' },
  { key: 'partyAAddress', label: '甲方地址', default: false, w: '1.6fr' },
  { key: 'partyAPhone', label: '甲方电话', default: false, w: '1fr' },
  { key: 'partyABank', label: '甲方开户行', default: false, w: '1.4fr' },
  { key: 'partyAAccount', label: '甲方账号', default: false, w: '1.3fr' },
  { key: 'partyBAddress', label: '乙方地址', default: false, w: '1.6fr' },
  { key: 'partyBPhone', label: '乙方电话', default: false, w: '1fr' },
  { key: 'partyBBank', label: '乙方开户行', default: false, w: '1.4fr' },
  { key: 'partyBAccount', label: '乙方账号', default: false, w: '1.3fr' },
  { key: 'qualityBond', label: '质保金', default: false, w: '1fr' },
  { key: 'liquidatedDamages', label: '违约金', default: false, w: '1fr' },
  { key: 'subject', label: '合同标的', default: false, w: '1.6fr' },
  { key: 'quantity', label: '数量', default: false, w: '0.7fr' },
  { key: 'unitPrice', label: '单价', default: false, w: '1fr' },
  { key: 'performancePeriod', label: '履行期限', default: false, w: '1.2fr' },
  { key: 'remark', label: '备注', default: false, w: '1.6fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns, colVisible,
  selectedRowKeys, onSelectChange, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-contract-list-columns',
  defaultColumns: DEFAULT_CONTRACT_COLUMNS,
})

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)

interface KnowledgeItem {
  id: string
  file_name?: string
  file_type?: string
  title?: string
  description?: string
  parse_status?: string
  summary_status?: string
  tags?: any[]
  custom_metadata?: any
  [key: string]: any
}

interface ContractItem {
  contract_no?: string
  contract_name?: string
  contract_type?: string
  sign_date?: string
  effective_date?: string
  expiry_date?: string
  sign_place?: string
  party_a_name?: string
  party_a_tax_no?: string
  party_a_address?: string
  party_a_phone?: string
  party_a_bank?: string
  party_a_account?: string
  party_b_name?: string
  party_b_tax_no?: string
  party_b_address?: string
  party_b_phone?: string
  party_b_bank?: string
  party_b_account?: string
  contract_amount?: number
  tax_rate?: number
  payment_method?: string
  quality_bond?: number
  liquidated_damages?: number
  subject?: string
  quantity?: number
  unit_price?: number
  performance_period?: string
  handler?: string
  department?: string
  remark?: string
  fulfill_status?: string
  page?: number
}

interface ContractRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'contract' | 'not_contract' | 'unknown' | 'pending'
  contractNo?: string
  contractName?: string
  contractType?: string
  signDate?: string
  effectiveDate?: string
  expiryDate?: string
  signPlace?: string
  partyAName?: string
  partyATaxNo?: string
  partyAAddress?: string
  partyAPhone?: string
  partyABank?: string
  partyAAccount?: string
  partyBName?: string
  partyBTaxNo?: string
  partyBAddress?: string
  partyBPhone?: string
  partyBBank?: string
  partyBAccount?: string
  contractAmount?: number
  taxRate?: number
  paymentMethod?: string
  qualityBond?: number
  liquidatedDamages?: number
  subject?: string
  quantity?: number
  unitPrice?: number
  performancePeriod?: string
  handler?: string
  department?: string
  remark?: string
  fulfillStatus?: string
  page?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}


// ---- 列表 ----
const items = ref<KnowledgeItem[]>([])
const contractRows = ref<ContractRow[]>([])
// 进行中的文件（解析中/提取中/待提取），在合同级列表顶部以状态行展示
const contractSummary = ref<{ total: number; sumAmount: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
// 请求序号（黄金基准发票同款）：类型/筛选切换会 reset 并作废旧的在途请求，
// 旧筛选响应返回时按序号丢弃，避免轮询/翻页的旧数据污染新筛选结果。
let listSeq = 0
let loadTimer: ReturnType<typeof setTimeout> | null = null
const keyword = ref('')
const filterContractType = ref('')
const filterFulfillStatus = ref('')
// 类型下拉（六同步：以 categories(scope=contract) 为权威源，回退识别规则配置类型）
const contractTypeRaw = computed<Array<{ value: string; label: string }>>(() => {
  const catNames = contractCats.value
    .filter((c: any) => c.enabled !== false)
    .map((c: any) => c.name)
    .filter((n: string) => n && n.trim())
  const base = catNames.length ? catNames : kbTypes.value
  return base.map((v: string) => ({ value: v, label: v }))
})
// 工具栏筛选下拉（六同步：与发票基准一致，只列具体分类、无聚合项，始终选中一项、禁清空）
const contractTypeOptions = computed<Array<{ value: string; label: string }>>(() => contractTypeRaw.value)
const dateRange = ref<Array<string>>([])

// 列显隐

const tableColumns = computed(() => {
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 46 }, { colKey: 'serial-number', title: '', width: 44 }]
  const rows = filteredRows.value.filter(r => r.kind !== 'pending')
  for (const c of visibleColDefs.value) {
    // 列宽：字段配置 width>0 固定（clamp 60~400）；0/未配置按内容自适应（封顶 150）
    const w = colWidthOf(c.width, c.label, rows.map(r => colValue(r, c.key)))
    cols.push({ colKey: c.key, title: c.label, ellipsis: true, width: w })
  }
  return cols
})


// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<ContractRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<ContractRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({})
const saving = ref(false)

// 抽屉宽度（可拖动，localStorage 记忆）
const DRAWER_WIDTH_KEY = 'weknora-contract-drawer-width'
// 打印预览
const printVisible = ref(false)
const historyVisible = ref(false)
const settingsVisible = ref(false)
const printCount = ref(0)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)
const printMode = ref<'print' | 'catalog'>('print')
const catalogBusy = ref(false)


// ---- KB 检测 ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      await loadTags()
      await loadTypeOptions()
      await loadContractCats()
      await cleanNonContractFiles()
      await loadFiles()
      loadUploadStats()
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
    await loadTypeOptions()
    await loadContractCats()
    await cleanNonContractFiles()
    await loadFiles()
    loadUploadStats()
    startPolling()
  }
}

// ---- 存量非合同清理：扫描知识库中已标记 not_contract 的文件并自动删除 ----
let cleaningNonContract = false
const cleanNonContractFiles = async () => {
  if (!kbId.value || cleaningNonContract) return
  cleaningNonContract = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_contract' || it.custom_metadata?.extract_status === 'not_contract')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非合同文件（旧数据清理）`)
      loadTypeOptions()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonContract = false }
}

// ---- 合同类型下拉（六同步基准同款）：分类以 categories(scope=contract) 为权威源（含自定义分类），
// 未入库时回退识别规则配置的类型列表；静态枚举已废弃，不再追加写死类型 ----
const kbTypes = ref<string[]>([])
const loadTypeOptions = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    if (Array.isArray(c?.types) && c.types.length) kbTypes.value = c.types.filter(Boolean)
  } catch { /* 保持现状 */ }
  ensureContractType()
}

// ---- 标签 ----
const loadTags = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeTags(kbId.value, { page: 1, page_size: 100 })
    const pageData = (res?.data || {}) as { data?: any[]; total?: number }
    tagList.value = (pageData.data || []).map((tag: any) => ({ ...tag, id: String(tag.id) }))
  } catch { /* 标签加载失败不阻塞 */ }
}

const openTagEdit = (row: ContractRow) => {
  tagTarget.value = row
  tagDialogVisible.value = true
}

const tagTargetTags = computed(() => (tagTarget.value ? rowTags(tagTarget.value) : []))

const onTagEditConfirm = async (tagIds: string[]) => {
  if (!tagTarget.value) return
  try {
    await updateKnowledgeTagBatch({ updates: { [tagTarget.value.knowledgeId]: tagIds } })
    MessagePlugin.success('标签已更新')
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '标签更新失败')
  }
}

const onTagCreated = () => { loadTags() }
const openTagManage = () => { tagManageVisible.value = true }
const onTagManageChanged = () => {
  loadTags()
  loadFiles(true)
}


// ---- 列表加载（合同级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  // 防重检查必须在 ++listSeq 之前：非 reset（翻页/onTick 轮询）若在 reset 请求进行中空转，
  // 绝不能递增请求序号，否则进行中的 reset 请求会被误判为「过期响应」而丢弃，
  // 其 finally 不再清理 listLoading、超时 timer 回调也因 seq 不匹配而失效 → 永久转圈。
  if (!reset && (listLoading.value || loadingMore.value)) return
  const mySeq = ++listSeq
  if (reset) {
    // reset（类型/筛选切换/手动刷新）接管并重置超时计时器
    if (loadTimer) { clearTimeout(loadTimer); loadTimer = null }
    page.value = 1
    contractRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
    loadTimer = setTimeout(() => {
      if (listLoading.value && mySeq === listSeq) {
        listLoading.value = false
        loadingMore.value = false
        MessagePlugin.warning('列表加载超时，请稍后重试')
      }
    }, 15000)
  } else {
    loadingMore.value = true
  }
  try {
    const res: any = await listContractRecords(kbId.value, {
      q: keyword.value || undefined,
      contract_type: filterContractType.value || undefined,
      // 履约状态为前端按到期日期派生的动态字段，后端无存储值可过滤，交由前端筛选
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    // 过期响应丢弃：此期间列表已按新筛选重置，旧结果（如轮询/翻页的旧类型数据）不得混入
    if (mySeq !== listSeq) return
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    contractSummary.value = {
      total,
      sumAmount: Number(res?.sum_amount || 0),
      sumTotal: Number(res?.sum_total || 0),
    }
    const existing = new Set(contractRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapContractRecord).filter(r => !existing.has(r.rowKey))
    contractRows.value = [...contractRows.value, ...fresh]
    hasMore.value = contractRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    if (mySeq !== listSeq) return
    MessagePlugin.error(e?.message || '合同列表加载失败')
  } finally {
    if (loadTimer && mySeq === listSeq) { clearTimeout(loadTimer); loadTimer = null }
    // 仅最新请求可清理 loading 标志；过期响应保持其父请求的进行中状态
    if (mySeq === listSeq) {
      listLoading.value = false
      loadingMore.value = false
    }
  }
}

// 把后端合同级记录（ContractRecord）映射为前端行
const mapContractRecord = (r: any): ContractRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.contract_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'contract',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  contractNo: r.contract_no || '',
  contractName: r.contract_name || '',
  contractType: r.contract_type || '',
  signDate: r.sign_date || '',
  effectiveDate: r.effective_date || '',
  expiryDate: r.expiry_date || '',
  signPlace: r.sign_place || '',
  partyAName: r.party_a_name || '',
  partyATaxNo: r.party_a_tax_no || '',
  partyAAddress: r.party_a_address || '',
  partyAPhone: r.party_a_phone || '',
  partyABank: r.party_a_bank || '',
  partyAAccount: r.party_a_account || '',
  partyBName: r.party_b_name || '',
  partyBTaxNo: r.party_b_tax_no || '',
  partyBAddress: r.party_b_address || '',
  partyBPhone: r.party_b_phone || '',
  partyBBank: r.party_b_bank || '',
  partyBAccount: r.party_b_account || '',
  contractAmount: numOrUndef(r.contract_amount),
  taxRate: numOrUndef(r.tax_rate),
  paymentMethod: r.payment_method || '',
  qualityBond: numOrUndef(r.quality_bond),
  liquidatedDamages: numOrUndef(r.liquidated_damages),
  subject: r.subject || '',
  quantity: numOrUndef(r.quantity),
  unitPrice: numOrUndef(r.unit_price),
  performancePeriod: r.performance_period || '',
  handler: r.handler || '',
  department: r.department || '',
  remark: r.remark || '',
  fulfillStatus: calcFulfillStatus(r.expiry_date),
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 合同级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

const parseCustomMetadata = (item: KnowledgeItem): ContractRow[] => {
  const meta = item.custom_metadata
  const fileName = item.file_name || item.title || item.id
  const parseStatus = item.parse_status || ''
  const summaryStatus = item.summary_status || ''
  const fileType = item.file_type || ''
  const tags = item.tags || []
  const description = item.description || ''
  const base = { parseStatus, summaryStatus, fileType, tags, description }
  if (!meta || typeof meta !== 'object') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: '', kind: 'unknown', ...base }]
  }
  const kind = meta.kind || 'unknown'
  const extractStatus = meta.extract_status || ''
  const extractError = meta.extract_error || ''
  if (kind === 'not_contract') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_contract', extractError, kind, ...base }]
  }
  const contracts = Array.isArray(meta.contracts) ? meta.contracts : []
  if (contracts.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return contracts.map((ct: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: contracts.length > 1 ? `${fileName}（第${idx + 1}份）` : fileName,
    multiIndex: contracts.length > 1 ? `${idx + 1}/${contracts.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    contractNo: ct.contract_no || '',
    contractName: ct.contract_name || '',
    contractType: ct.contract_type || '',
    signDate: ct.sign_date || '',
    effectiveDate: ct.effective_date || '',
    expiryDate: ct.expiry_date || '',
    signPlace: ct.sign_place || '',
    partyAName: ct.party_a_name || '',
    partyATaxNo: ct.party_a_tax_no || '',
    partyAAddress: ct.party_a_address || '',
    partyAPhone: ct.party_a_phone || '',
    partyABank: ct.party_a_bank || '',
    partyAAccount: ct.party_a_account || '',
    partyBName: ct.party_b_name || '',
    partyBTaxNo: ct.party_b_tax_no || '',
    partyBAddress: ct.party_b_address || '',
    partyBPhone: ct.party_b_phone || '',
    partyBBank: ct.party_b_bank || '',
    partyBAccount: ct.party_b_account || '',
    contractAmount: numOrUndef(ct.contract_amount),
    taxRate: numOrUndef(ct.tax_rate),
    paymentMethod: ct.payment_method || '',
    qualityBond: numOrUndef(ct.quality_bond),
    liquidatedDamages: numOrUndef(ct.liquidated_damages),
    subject: ct.subject || '',
    quantity: numOrUndef(ct.quantity),
    unitPrice: numOrUndef(ct.unit_price),
    performancePeriod: ct.performance_period || '',
    handler: ct.handler || '',
    department: ct.department || '',
    remark: ct.remark || '',
    fulfillStatus: calcFulfillStatus(ct.expiry_date),
    page: Number(ct.page) || 0,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端合同级接口完成，前端直接使用返回的分页数据；
// 顶部合并"进行中文件"状态行（解析中/提取中/待提取），提取完成即消失
const pendingRows = computed<ContractRow[]>(() => pendingFiles.value.map((k) => {
  const ps = k.parse_status || ''
  const es = k.custom_metadata?.extract_status || ''
  let extractStatus = ''
  if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') extractStatus = 'parsing'
  else if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) extractStatus = 'pending'
  return {
    rowKey: `pending-${k.id}`,
    knowledgeId: k.id,
    fileName: k.file_name || k.title || k.id,
    parseStatus: ps,
    extractStatus,
    extractError: k.custom_metadata?.extract_error || '',
    kind: 'pending',
    tags: Array.isArray(k.tags) ? k.tags : [],
  } as ContractRow
}))
// 履约状态为前端派生字段：筛选在已加载数据上按派生状态匹配（后端不传 fulfill_status）
const fulfillFilteredRows = computed(() => {
  if (!filterFulfillStatus.value) return contractRows.value
  return contractRows.value.filter(r => calcFulfillStatus(r.expiryDate) === filterFulfillStatus.value)
})
const filteredRows = computed(() => [...pendingRows.value, ...fulfillFilteredRows.value])

const fulfillStatusOptions = computed(() => FULFILL_STATUSES.map(v => ({ value: v, label: v })))
const paymentMethodOptions = computed(() => PAYMENT_METHODS.map(v => ({ value: v, label: v })))

// 履约状态标签主题（派生三态：履行中 success / 即将到期 warning / 已超期 danger）
const fulfillStatusTheme = (s: string): 'success' | 'warning' | 'default' | 'danger' => {
  switch (s) {
    case '履行中': return 'success'
    case '即将到期': return 'warning'
    case '已超期': return 'danger'
    default: return 'default'
  }
}


// ---- 多选 ----
const selectableRows = computed(() => filteredRows.value.filter(r => r.kind !== 'pending'))
const isAllSelected = computed(() =>
  selectableRows.value.length > 0 && selectableRows.value.every(r => selectedRowKeys.value.includes(r.rowKey))
)
const someSelected = computed(() => {
  const sel = selectableRows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)).length
  return sel > 0 && sel < selectableRows.value.length
})
const toggleSelectAll = (checked: boolean) => {
  selectedRowKeys.value = checked ? selectableRows.value.map(r => r.rowKey) : []
}
const toggleRow = (rowKey: string, checked: boolean) => {
  if (checked) {
    if (!selectedRowKeys.value.includes(rowKey)) selectedRowKeys.value = [...selectedRowKeys.value, rowKey]
  } else {
    selectedRowKeys.value = selectedRowKeys.value.filter(k => k !== rowKey)
  }
}
const selectedRows = computed(() => {
  const byKey = new Map(contractRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as ContractRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中合同的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  let sumTotal = 0
  for (const r of rows) {
    sumTotal += Number(r.contractAmount) || 0
  }
  return { total: rows.length, sumAmount: sumTotal, sumTotal }
})
const displaySummary = computed(() => {
  if (selectedSummary.value) return selectedSummary.value
  // 履约状态筛选为前端派生过滤：汇总按过滤后的已加载行现算（与列表可见行一致）
  if (filterFulfillStatus.value) {
    const rows = fulfillFilteredRows.value
    let sum = 0
    for (const r of rows) sum += Number(r.contractAmount) || 0
    return { total: rows.length, sumAmount: sum, sumTotal: sum }
  }
  return contractSummary.value
})

const applyFilter = () => { loadFiles(true) }
const onKeywordChange = () => { loadFiles(true) }

// ---- 筛选联动闭环（黄金基准发票同款） ----
// 多选清除：先调用 t-table 实例 clearSelected（内部清空并 emit select-change 同步外部），
// 再兜底清空 selectedRowKeys，杜绝 TDesign 受控反向写回旧 key 导致浮条不消失
const contractTableRef = ref()
const clearSelectionSafe = () => {
  try { contractTableRef.value?.clearSelected?.() } catch { /* ignore */ }
  clearSelection()
}
// 搜索态：关键词非空时类型下拉可临时清空（全局搜索优先）
const searchActive = computed(() => !!keyword.value.trim())
// 类型筛选锁定（发票基准同款）：始终保证有一项具体分类被选中（禁清空）；
// 搜索态放行（由 keyword watch 负责临时清空与恢复）；当前选中项合法则保留（含路由/上传回填），
// 否则按分类顺序选中第一个启用分类。
const ensureContractType = () => {
  if (searchActive.value) return
  const valid = new Set(contractTypeOptions.value.map((o) => o.value))
  if (filterContractType.value && valid.has(filterContractType.value)) return
  const cats = contractCats.value.filter((c: any) => c.enabled !== false).map((c: any) => c.name)
  filterContractType.value = (cats.length ? cats[0] : contractTypeOptions.value[0]?.value) || ''
}
// 类型切换：清空多选（防跨类型脏数据残留）+ 重建当前分类列 + 立即刷新列表（发票基准同款）
watch(filterContractType, () => {
  clearSelectionSafe()
  reloadColumns()
  applyFilter()
})
// 履约状态筛选（前端派生过滤）：切换时清空多选残留并重载列表
watch(filterFulfillStatus, () => {
  clearSelectionSafe()
  applyFilter()
})
// 关键词防抖 300ms 后刷新；输入时临时清空类型筛选（全局搜索），清空词后恢复原类型。
// 履约状态与签订日期筛选保持不动（任务明确要求）。
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let searchRestoreType = ''
watch(keyword, (v) => {
  if (v && v.trim()) {
    if (searchRestoreType === '' && filterContractType.value) searchRestoreType = filterContractType.value
    if (filterContractType.value) filterContractType.value = ''
  } else {
    if (searchRestoreType) {
      filterContractType.value = searchRestoreType
      searchRestoreType = ''
    }
  }
  if (keywordTimer) clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => loadFiles(true), 300)
})

// ---- KPI 概览卡（黄金基准 DashboardKpiGroup：全部合同 → 各合同类型卡 → 即将到期 → 历史记录） ----
const overviewCards = computed<KpiCard[]>(() => {
  const rows = filteredRows.value.filter(r => r.kind !== 'pending')
  const total = contractSummary.value.total || rows.length
  const now = new Date()
  const monthPrefix = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const monthNew = rows.filter(r => (r.signDate || '').startsWith(monthPrefix)).length
  const cards: KpiCard[] = [
    { key: 'total', label: '全部合同', icon: 'file-copy', value: String(total), sub: `本月新增 ${monthNew} 份`, theme: 'brand' },
  ]
  // 各合同类型卡片：按下拉枚举动态渲染（不含「全部合同」），点击联动类型筛选（发票基准同款 action/typeValue）
  for (const t of contractTypeRaw.value) {
    const cnt = rows.filter(r => r.contractType === t.value).length
    cards.push({ key: `type-${t.value}`, label: t.label, icon: 'file-1', value: String(cnt), unit: '份', theme: 'neutral', action: 'type', typeValue: t.value })
  }
  // 即将到期（未来 30 天内）：warning 主题色警示
  const expiring = rows.filter(r => calcFulfillStatus(r.expiryDate) === '即将到期').length
  cards.push({ key: 'expiring', label: '即将到期', icon: 'time', value: String(expiring), unit: '份', theme: 'warning', action: 'expiring' })
  // 历史记录（已上传文件，对齐发票基准 fileCount/failed）：异常文件数 sub 展示，点击打开已上传文件抽屉
  const hs = uploadStats.value
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: String(hs.total), unit: '份', sub: hs.failed ? `${hs.failed} 份异常` : '无异常文件', theme: hs.failed > 0 ? 'warning' : 'neutral', action: 'history' })
  return cards
})
// 卡片点击联动（发票基准同款 action 分流：类型卡切筛选；即将到期卡切履约筛选；历史记录卡开抽屉）
const onOverviewCardClick = (card: KpiCard) => {
  if (card.action === 'type' && card.typeValue) {
    filterContractType.value = card.typeValue // watch 触发 clearSelectionSafe + applyFilter
  } else if (card.action === 'expiring') {
    filterFulfillStatus.value = '即将到期'
  } else if (card.action === 'history') {
    historyVisible.value = true
  }
}

// 懒加载
const listScrollRef = ref<HTMLElement>()
const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200 && hasMore.value && !listLoading.value && !loadingMore.value) {
    loadFiles()
  }
}

// ---- 上传（对齐发票/车队：FleetUploadDialog 弹窗，支持拖拽/选择/剪贴板粘贴 + 防重检测） ----
const uploadVisible = ref(false)
// 上传弹窗分类选项：contract categories（含自定义分类），复合值 contract__名称
const contractCats = ref<any[]>([])
const uploadTypeOptions = computed(() => {
  const cats = contractCats.value.filter((c: any) => c.enabled !== false)
  if (!cats.length) return [{ label: '服务合同', value: 'contract__服务合同' }]
  return cats.map((c: any) => ({ label: c.name, value: `contract__${c.name}` }))
})
const loadContractCats = async () => {
  try {
    const res: any = await listFleetCategories({ scope: 'contract' })
    const cats = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    contractCats.value = cats
  } catch { contractCats.value = [] }
  // 六同步：分类重载后若当前选中项已失效（禁用/改名/删除），回退第一个启用分类
  ensureContractType()
}
const onUploadDone = async (typeName?: string) => {
  // 上传完成刷新列表（合同记录的 contract_type 为 OCR 提取的业务类型，与上传所选字段分类不强制映射，
  // 不主动切换类型筛选，避免提取结果与分类名不一致时列表被过滤为空）
  await loadFiles(true)
}

// 历史记录（已上传文件）计数：KPI「历史记录」卡数据源（对齐发票基准 fileCount/failed，
// 前端轻量聚合：文件总数 + 解析/提取异常数，异常数按已加载页统计）
const uploadStats = ref<{ total: number; failed: number }>({ total: 0, failed: 0 })
const loadUploadStats = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    const total = Number(res?.total ?? arr.length ?? 0)
    const failed = arr.filter((it: any) =>
      it.parse_status === 'failed' || (it.custom_metadata || {}).extract_status === 'failed'
    ).length
    uploadStats.value = { total, failed }
  } catch { uploadStats.value = { total: 0, failed: 0 } }
}
// 历史记录抽屉变更（删除/重新解析）后同步刷新列表与卡片计数
const onUploadHistoryChanged = async () => {
  await loadFiles(true)
  loadUploadStats()
}

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'contract'),
  onPendingFiles: (files) => {
    const withRows = new Set(contractRows.value.map(r => r.knowledgeId))
    pendingFiles.value = files.filter(k => !withRows.has(k.id))
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是合同文件，已移至删除历史，可在删除历史中恢复`)
    loadTypeOptions()
  },
  onTick: () => loadFiles(),
})

const { extractStatusOf, statusOf, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'contract', notLabel: '非合同',
})


// ---- 详情抽屉 ----
const openDetail = async (row: ContractRow) => {
  if (row.kind === 'pending') return
  currentRow.value = row
  currentDetail.value = null
  detailVisible.value = true
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    if (detail && typeof detail === 'object') {
      currentDetail.value = detail
      if (detail.custom_metadata) {
        const parsed = parseCustomMetadata(detail)
        // 合同一份文件一份：优先按合同编号匹配详情（合同不按页匹配）
        const match = parsed.find(p => p.contractNo && p.contractNo === row.contractNo)
          || parsed.find(p => p.contractName === row.contractName) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.contractNo ? `合同详情 · ${currentRow.value.contractNo}` : '合同详情'
)

// ---- 字段编辑 + 手动保存（黄金基准发票同款：底部「保存/取消」统一提交） ----
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  editForm.value = {
    contract_no: r.contractNo || '',
    contract_name: r.contractName || '',
    contract_type: r.contractType || '',
    sign_date: r.signDate || '',
    effective_date: r.effectiveDate || '',
    expiry_date: r.expiryDate || '',
    sign_place: r.signPlace || '',
    party_a_name: r.partyAName || '',
    party_a_tax_no: r.partyATaxNo || '',
    party_a_address: r.partyAAddress || '',
    party_a_phone: r.partyAPhone || '',
    party_a_bank: r.partyABank || '',
    party_a_account: r.partyAAccount || '',
    party_b_name: r.partyBName || '',
    party_b_tax_no: r.partyBTaxNo || '',
    party_b_address: r.partyBAddress || '',
    party_b_phone: r.partyBPhone || '',
    party_b_bank: r.partyBBank || '',
    party_b_account: r.partyBAccount || '',
    contract_amount: numToStr(r.contractAmount),
    tax_rate_display: r.taxRate === null || r.taxRate === undefined ? '' : String(Number(r.taxRate) * 100),
    payment_method: r.paymentMethod || '',
    quality_bond: numToStr(r.qualityBond),
    liquidated_damages: numToStr(r.liquidatedDamages),
    subject: r.subject || '',
    quantity: numToStr(r.quantity),
    unit_price: numToStr(r.unitPrice),
    performance_period: r.performancePeriod || '',
    handler: r.handler || '',
    department: r.department || '',
    remark: r.remark || '',
  }
}

const saveContractDetail = async () => {
  const now = currentRow.value
  if (!now) return
  saving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const contracts = Array.isArray(meta.contracts) ? [...meta.contracts] : []
    const taxRateVal = toNumber(editForm.value.tax_rate_display)
    const updated: any = {
      contract_no: editForm.value.contract_no || '',
      contract_name: editForm.value.contract_name || '',
      contract_type: editForm.value.contract_type || '',
      sign_date: editForm.value.sign_date || '',
      effective_date: editForm.value.effective_date || '',
      expiry_date: editForm.value.expiry_date || '',
      sign_place: editForm.value.sign_place || '',
      party_a_name: editForm.value.party_a_name || '',
      party_a_tax_no: editForm.value.party_a_tax_no || '',
      party_a_address: editForm.value.party_a_address || '',
      party_a_phone: editForm.value.party_a_phone || '',
      party_a_bank: editForm.value.party_a_bank || '',
      party_a_account: editForm.value.party_a_account || '',
      party_b_name: editForm.value.party_b_name || '',
      party_b_tax_no: editForm.value.party_b_tax_no || '',
      party_b_address: editForm.value.party_b_address || '',
      party_b_phone: editForm.value.party_b_phone || '',
      party_b_bank: editForm.value.party_b_bank || '',
      party_b_account: editForm.value.party_b_account || '',
      contract_amount: toNumber(editForm.value.contract_amount),
      // 税率由百分比字符串转回小数（"3" → 0.03）
      tax_rate: taxRateVal === undefined ? undefined : taxRateVal / 100,
      payment_method: editForm.value.payment_method || '',
      quality_bond: toNumber(editForm.value.quality_bond),
      liquidated_damages: toNumber(editForm.value.liquidated_damages),
      subject: editForm.value.subject || '',
      quantity: toNumber(editForm.value.quantity),
      unit_price: toNumber(editForm.value.unit_price),
      performance_period: editForm.value.performance_period || '',
      handler: editForm.value.handler || '',
      department: editForm.value.department || '',
      remark: editForm.value.remark || '',
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = contracts.findIndex((ct: any, i: number) =>
        i === nowPage - 1 || Number(ct.page) === nowPage || ct.contract_no === now.contractNo)
    }
    if (targetIdx < 0) targetIdx = contracts.findIndex((ct: any) => ct.contract_no === now.contractNo)
    if (targetIdx >= 0) contracts[targetIdx] = { ...contracts[targetIdx], ...updated, page: nowPage || contracts[targetIdx]?.page }
    else contracts.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const hasContractData = contracts.some((c: any) =>
      (c.contract_no || '').trim() || (c.contract_name || '').trim() || (c.contract_type || '').trim() ||
      (c.party_a_name || '').trim() || (c.party_b_name || '').trim() || Number(c.contract_amount) > 0)
    const nextStatus = hasContractData ? 'success' : 'manual'
    const nextError = hasContractData ? '' : '人工入库待编辑，请补充字段'
    const newMeta = { ...meta, kind: 'contract', contracts, extract_status: nextStatus, extract_error: nextError }
    await updateKnowledgeMetadata(now.knowledgeId, newMeta)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑后列表立即刷新
    const listIdx = contractRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      contractRows.value[listIdx] = {
        ...contractRows.value[listIdx],
        extractStatus: nextStatus,
        extractError: nextError,
        contractNo: updated.contract_no,
        contractName: updated.contract_name,
        contractType: updated.contract_type,
        signDate: updated.sign_date,
        effectiveDate: updated.effective_date,
        expiryDate: updated.expiry_date,
        partyAName: updated.party_a_name,
        partyBName: updated.party_b_name,
        contractAmount: updated.contract_amount,
        taxRate: updated.tax_rate,
        paymentMethod: updated.payment_method,
        fulfillStatus: calcFulfillStatus(updated.expiry_date),
        handler: updated.handler,
        department: updated.department,
        remark: updated.remark,
      }
    }
    MessagePlugin.success('保存成功')
    detailVisible.value = false
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const toNumber = (v: any): number | undefined => {
  if (v === '' || v === null || v === undefined) return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 数字输入辅助：只允许数字与小数点，按实际输入显示（不补零、不保留多余小数）
const sanitizeNum = (v: string): string => {
  let s = (v ?? '').replace(/[^\d.]/g, '')
  const firstDot = s.indexOf('.')
  if (firstDot >= 0) s = s.slice(0, firstDot + 1) + s.slice(firstDot + 1).replace(/\./g, '')
  return s
}
const parseNum = (v: string): number | null => {
  const s = sanitizeNum(v)
  if (s === '' || s === '.') return null
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}
const numToStr = (v: any): string =>
  v === null || v === undefined || v === '' ? '' : String(v)

// 金额一律不显示货币符号（全系统统一规则，与发票/车队基准一致）
const formatAmount = (v?: number) =>
  v === undefined || v === null ? '' : Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

// 税率以百分比显示且不保留小数（0.03 → 3%）
const formatRate = (v?: number) => {
  if (v === undefined || v === null) return ''
  return `${Math.round(Number(v) * 100)}%`
}

// ---- 浮动工具栏 ----
const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

// 重新提取：重新解析选中行对应源文件，解析完成后自动触发合同字段提取
// （合同一个文件一般只有一份合同，无需按页提取；打印也整文档预览）。
const handleReExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('重新提取仅支持单选，请选中一份合同'); return }
  extractInFlight.value.add(row.knowledgeId)
  extractFailed.value.delete(row.knowledgeId)
  try {
    await reparseKnowledge(row.knowledgeId)
    MessagePlugin.success(`已触发「${row.fileName}」重新解析与提取`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    extractFailed.value.add(row.knowledgeId)
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
  }
}

// 打印：合同一个文件一般只有一份合同，打印预览整个文档（含多页），
// 支持跨文件任意组合合并为一个 PDF，单 iframe 预览打印。
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff']
const isImageRow = (r: any, blob: any) => {
  const ext = String(r.fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const handleBatchPrint = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  printBusy.value = true
  printUrl.value = ''
  printLoaded.value = false
  printCount.value = rows.length
  printVisible.value = true
  try {
    const out = await PDFDocument.create()
    const notes: string[] = []
    let mergedPages = 0
    for (const r of rows) {
      const blob: any = await previewKnowledgeFile(r.knowledgeId)
      if (!blob) { notes.push(`${r.contractNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式合同：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        // 合同打印整个源文件（一份合同可能多页，如盖章扫描件）
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pageCount = pdf.getPageCount()
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
        if (pageCount > 1) notes.push(`${r.contractNo || r.fileName}（共 ${pageCount} 页）`)
      } catch {
        notes.push(`${r.contractNo || r.fileName}（页抽取失败，已并入整文件）`)
        try {
          const src = await blob.arrayBuffer()
          const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
          const pages = await out.copyPages(pdf, pdf.getPageIndices())
          pages.forEach(p => out.addPage(p))
          mergedPages += pages.length
        } catch { /* 整文件也失败则跳过 */ }
      }
    }
    if (mergedPages === 0) {
      MessagePlugin.error('未获取到可打印的合同页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    if (notes.length) {
      setTimeout(() => MessagePlugin.warning(notes.join('；')), 300)
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
    printVisible.value = false
  } finally {
    printBusy.value = false
  }
}

// 打印：调用预览 iframe 内的 PDF 查看器打印（打印完整合并 PDF，
// 无水印、无裁剪、含全部选中页面）
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement | null
  if (frame?.contentWindow) {
    try { frame.contentWindow.print(); return } catch { /* fallback */ }
  }
  window.print()
}

// 目录生成：把选中记录按「当前列表展示字段」生成表格式清单 PDF（A4 横/纵自适应），
// 复用打印弹窗预览，支持打印与下载
const catalogValueOf = (row: ContractRow, key: string): string => {
  switch (key) {
    case 'contractNo': return row.contractNo || ''
    case 'contractName': return row.contractName || ''
    case 'contractType': return row.contractType || ''
    case 'partyAName': return row.partyAName || ''
    case 'partyBName': return row.partyBName || ''
    case 'signDate': return row.signDate || ''
    case 'expiryDate': return row.expiryDate || ''
    case 'contractAmount': return formatAmount(row.contractAmount)
    case 'taxRate': return formatRate(row.taxRate)
    case 'extractStatus': { const s = statusOf(row).label; return s === '--' ? '' : s }
    case 'tags': return rowTags(row).map((t: any) => t.name).join('、')
    case 'fulfillStatus': return row.fulfillStatus || ''
    case 'partyATaxNo': return row.partyATaxNo || ''
    case 'partyBTaxNo': return row.partyBTaxNo || ''
    case 'effectiveDate': return row.effectiveDate || ''
    case 'paymentMethod': return row.paymentMethod || ''
    case 'handler': return row.handler || ''
    case 'department': return row.department || ''
    case 'fileName': return row.fileName || ''
    default: return String((row as any)[key] ?? '')
  }
}
const handleBatchCatalog = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = [
      ...visibleColDefs.value.map((c: any) => ({ key: c.key, label: c.label, value: (r: any) => catalogValueOf(r, c.key) })),
    ]
    const bytes = await generateCatalogPdf({ title: '合同目录', columns: cols, rows })
    printMode.value = 'catalog'
    printCount.value = rows.length
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const downloadCatalogPdf = () => {
  if (!printUrl.value) return
  const a = document.createElement('a')
  a.href = printUrl.value
  a.download = `合同目录-${Date.now()}.pdf`
  a.click()
}

// ---- 删除（黄金基准发票同款：单气泡内嵌二选一，杜绝二次弹窗） ----
const delPopVisible = ref(false)
const closeDelPop = () => { delPopVisible.value = false }
// 选中记录涉及的多页文件数量（同一上传文件含多份合同）
const delMultiCount = computed(() => {
  const set = new Set<string>()
  for (const r of selectedRows.value) {
    if (!r.knowledgeId) continue
    if (r.page && r.page >= 1) set.add(r.knowledgeId)
  }
  return set.size
})
// 仅删除选中的合同页（同文件其它合同保留）
const doPageDelete = async (rows: ContractRow[]) => {
  const pageDeleteIds: Array<{ knowledgeId: string; page: number }> = []
  const fileDeleteIds = new Set<string>()
  for (const r of rows) {
    if (r.page && r.page >= 1) pageDeleteIds.push({ knowledgeId: r.knowledgeId, page: r.page })
    else fileDeleteIds.add(r.knowledgeId)
  }
  // 同文件多页删除时，先删大页码再删小页码（后端删后重排页码）
  pageDeleteIds.sort((a, b) => b.page - a.page)
  for (const pd of pageDeleteIds) {
    const res: any = await deleteContractPage(kbId.value, pd.knowledgeId, pd.page)
    if (res?.deleted_file) fileDeleteIds.add(pd.knowledgeId)
  }
  if (fileDeleteIds.size) await batchDeleteKnowledge(kbId.value, Array.from(fileDeleteIds))
}
// 删除整份上传文件（含其全部合同记录，从知识库完全删除）
const doFileDelete = async (rows: ContractRow[]) => {
  const ids = new Set(rows.map(r => r.knowledgeId).filter(Boolean))
  if (ids.size) await batchDeleteKnowledge(kbId.value, Array.from(ids))
}
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  clearSelectionSafe()
  await loadFiles(true)
}
const confirmPageDelete = async () => {
  closeDelPop()
  try { await doPageDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}
const confirmFileDelete = async () => {
  closeDelPop()
  try { await doFileDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---- 生命周期 ----
// 六同步：字段/分类配置变更后重建动态列 + 重载分类（KPI 类型卡/工具栏下拉/编辑抽屉类型下拉/
// 上传弹窗分类六处同源）+ 刷新记录（KPI 计数来自列表数据），与发票基准一致
const onCategoriesChanged = () => {
  reloadColumns(true)
  loadContractCats()
  loadFiles(true)
}
// 从后端 categories（scope=contract）重建动态列；无配置时回退内置默认。
// 列按「当前选中分类」的字段配置生成（六同步：列表/字段筛选器/编辑抽屉/打印同源），
// 多分类下切换类型时列表列跟随当前分类（不再假设单一「合同」分类）。
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    // 分类缓存：切换类型时直接用已拉取的分类重建列（省一次请求、切换更快）；
    // 仅首次进入或设置保存/六同步事件（force=true）时重新请求后端
    let cats: any[] = force ? [] : contractCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'contract' })
      cats = res?.data || []
      contractCats.value = cats
    }
    const curCat = cats.find((c: any) => c.name === filterContractType.value && c.enabled !== false)
    const baseCat = curCat || cats[0]
    if (baseCat?.subs?.length) {
      customColumns.value = baseCat.subs
        .filter((s: any) => s.enabled !== false)
        .map((s: any) => {
          const builtin = DEFAULT_CONTRACT_COLUMNS.find((b: any) => b.key === s.name)
          return { key: s.name, label: builtin?.label || s.name, default: s.is_default === true, w: builtin?.w || '1fr', width: Number(s.width) || 0 }
        })
      // 六同步：切换分类或设置保存后，按当前分类默认表头重建字段筛选器勾选。
      // 首轮加载（lastColType 为空）时保留用户既有 localStorage 存档，但若存档缺当前分类
      // 的默认列则补齐当前分类默认列（发票基准同款门控）
      if (!lastColType.value) {
        const defaults = customColumns.value.filter((c: any) => c.default).map((c: any) => c.key)
        if (defaults.some((k: string) => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterContractType.value) {
        resetColumns()
      }
      lastColType.value = filterContractType.value
    }
    // 类型筛选锁定：分类重载后若当前选中项失效（被禁用/改名），回退第一个启用分类
    ensureContractType()
  } catch { /* 后端未配置时回退内置默认 */ }
}
onMounted(() => {
  loadKb()
  loadContractCats()
  reloadColumns()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})
</script>

<style scoped lang="less">
.contract-management-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 24px 32px;
  overflow: hidden;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;

  .header-title {
    h2 { font-size: 20px; font-weight: 600; color: var(--td-text-color-primary); margin: 0 0 6px 0; }
    .header-subtitle { font-size: 14px; color: var(--td-text-color-secondary); margin: 0; }
  }
}

.loading-area, .empty-area {
  flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 16px; min-height: 300px;
}

.contract-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

/* ---- 工具栏 #type-extra 插槽内的履约状态/日期控件 ---- */
.doc-filter-field {
  display: flex; align-items: center;
  &--wide { min-width: 260px; }
  .doc-type-select { width: 130px; }
  .doc-date-range { width: 260px; }
}

/* ---- 字段筛选弹层 ---- */
:global(.contract-field-popup) {
  padding: 0 !important;
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

/* ---- 列表 ---- */
@keyframes doc-list-spin { to { transform: rotate(360deg); } }

.doc-list-scroll {
  /* 发票基准同款：滚动必须由 t-table 内部承担（max-height 100% + sticky-header 表头固定），
     容器自身不滚动，否则表头会被滚走、底部摘要 sticky 定位错乱 */
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

/* ---- 底部汇总（发票基准 .doc-summary-bar 逐字同款：sticky 于列表容器底部，紧贴表格，极小垂直空间） ---- */
.doc-summary-bar {
  position: sticky;
  bottom: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.06);
  .doc-summary-count { font-weight: 600; color: var(--td-text-color-primary); }
  .doc-summary-item {
    display: inline-flex; align-items: baseline; gap: 6px;
    .doc-summary-val { font-variant-numeric: tabular-nums; color: var(--td-text-color-primary); font-weight: 600; }
  }
}

.doc-list-view {
  position: relative;
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 表格高度受限于容器，滚动由 .t-table__content 内部承担（max-height 100% + sticky 表头），发票基准同款 */
.doc-list-view :deep(.t-table) {
  flex: 1;
  min-height: 0;
  height: 100%;
  max-height: 100%;
}

/* scrollbar-gutter:stable 保证滚动条占位宽度恒定，列与滚动条不错位 */
.doc-list-view :deep(.t-table__content) {
  scrollbar-gutter: stable;
}

/* 表头 th 背景与内容区统一为 container 令牌，与下方表头右侧遮罩同令牌 → 主题切换同步变色，不产生色差 */
.doc-list-view :deep(.t-table__header--fixed > tr > th) {
  background-color: var(--td-bg-color-container);
}

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
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-stroke);
  .cell { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
  &:hover:not(.selected) { background: var(--td-bg-color-secondarycontainer); }
  &.selected { background: var(--td-brand-color-1); }
}

.cell {
  display: flex;
  align-items: center;
  justify-content: center; /* 表头/内容居中 */
  min-width: 0;
  padding: 0 8px;
  text-align: center;
}

.cell-check {
  justify-content: center;
  position: sticky;
  left: 0;
  z-index: 2;
  background: transparent; /* 跟随行/容器背景，避免白色块 */
  padding: 0;
}

.cell-contractNo, .cell-contractName, .cell-partyAName, .cell-partyBName, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-contract-no {
  min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font-size: 13px; font-weight: 600; color: var(--td-text-color-primary);
}
.row-page-badge {
  flex-shrink: 0; margin-left: 6px; height: 18px; padding: 0 6px; border-radius: 999px;
  background: var(--td-success-color-light); color: var(--td-success-color); font-size: 11px; line-height: 18px;
  white-space: nowrap;
}
.row-text { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-mono { font-variant-numeric: tabular-nums; font-size: 12px; color: var(--td-text-color-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.row-amount { font-size: 13px; color: var(--td-text-color-primary); font-weight: 500; }
.row-rate { font-size: 12px; color: var(--td-text-color-primary); }
.row-muted { color: var(--td-text-color-disabled); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-status-tag :deep(.t-icon) { margin-right: 2px; }
.icon-spin { animation: doc-list-spin 0.9s linear infinite; }

.row-tag-chips {
  display: inline-flex; align-items: center; gap: 4px; flex-wrap: nowrap; cursor: pointer;
}
.row-tag-more {
  display: inline-flex; align-items: center; justify-content: center; height: 20px; min-width: 20px; padding: 0 4px;
  border-radius: 999px; border: 1px solid var(--td-component-stroke); color: var(--td-text-color-secondary); font-size: 10px;
}
.row-tag-add {
  font-size: 11px; color: var(--td-text-color-placeholder); border: 1px dashed var(--td-component-stroke);
  border-radius: 999px; padding: 0 6px; height: 20px; display: inline-flex; align-items: center; white-space: nowrap;
  &:hover { border-color: var(--td-brand-color); color: var(--td-brand-color); border-style: solid; }
}

.doc-load-more { display: flex; justify-content: center; padding: 12px 0; }

/* ---- 详情抽屉：竖向区块 + 可拖宽（复用知识库文档抽屉上下滚动样式） ---- */
.contract-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.contract-detail-body {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.detail-block { width: 100%; }
.detail-block-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 12px;
  &::before { content: ''; width: 3px; height: 14px; background: var(--td-brand-color); border-radius: 2px; flex-shrink: 0; }
}
.detail-block-content { width: 100%; }

// 摘要（复用知识库文档抽屉样式：线框 + 展开/折叠 + 每条字段一行）
.summary_wrapper {
  position: relative;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  &.summary_clickable { cursor: pointer; }
}
.summary_content {
  padding: 12px;
  color: var(--td-text-color-primary);
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
  white-space: pre-wrap;
  &.summary_collapsed { max-height: 4.5em; overflow: hidden; }
}
.summary_fade {
  display: flex;
  justify-content: center;
  padding-bottom: 4px;
  pointer-events: none;
  &:not(.summary_fade_expanded) {
    position: absolute;
    bottom: 0; left: 0; right: 0;
    height: 28px;
    background: linear-gradient(transparent, var(--td-bg-color-container) 80%);
    border-radius: 0 0 6px 6px;
    align-items: flex-end;
  }
}
.summary_fade_icon { color: var(--td-text-color-placeholder); }

.detail-fields { padding-bottom: 16px; }

.field-group { margin-bottom: 18px; }
.field-group-title {
  display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600;
  color: var(--td-text-color-primary); margin-bottom: 12px;
  &::before { content: ''; width: 3px; height: 14px; background: var(--td-brand-color); border-radius: 2px; flex-shrink: 0; }
}
.field-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); column-gap: 20px; row-gap: 12px; }
.field-grid--full { grid-template-columns: 1fr; }
.field-grid :deep(.t-form__item) { margin-bottom: 0; }

.detail-save-hint { display: flex; align-items: center; gap: 6px; margin-top: 16px; font-size: 12px; color: var(--td-text-color-placeholder); }

/* ---- 删除气泡：内嵌二选一（黄金基准发票同款） ---- */
.del-pop-body {
  min-width: 220px;
  .del-pop-title { font-size: 13px; color: var(--td-text-color-primary); line-height: 1.5; margin-bottom: 12px; }
  .del-pop-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; }
}

/* ---- 详情抽屉底部操作：手动保存/取消 ---- */
.detail-footer-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 8px;
}

</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.contract-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: var(--wk-batch-bar-z);
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.contract-print-dialog {
  width: min(900px, calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  height: 82vh;
  max-height: 92vh;
  background: var(--td-bg-color-container);
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.contract-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: var(--td-bg-color-container);
}
.contract-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.contract-print-close {
  flex-shrink: 0;
}
.contract-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.contract-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.contract-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.contract-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
