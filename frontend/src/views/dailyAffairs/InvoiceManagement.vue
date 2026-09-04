<template>
  <div class="invoice-management-container">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>发票管理</h2>
        <p class="header-subtitle">上传发票文件，自动识别发票字段并归档，支持查询、编辑、打印、下载与删除</p>
      </div>
      <div class="header-actions">
        <t-button v-if="kbId" theme="primary" @click="triggerUpload">
          <template #icon><t-icon name="upload" /></template>
          上传发票
        </t-button>
      </div>
      <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
        @change="onFileInputChange" />
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化发票管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-发票」知识库">
        <template #image><t-icon name="money-circle" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建发票知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="invoice-main">
      <!-- 筛选工具栏（复用原项目文档列表样式） -->
      <div class="doc-filter-bar">
        <div class="doc-filter-bar__leading">
          <div class="doc-filter-field doc-filter-field--search">
            <t-input v-model="keyword" placeholder="搜索全部字段" clearable class="doc-search doc-filter-field__control"
              @enter="onKeywordChange" @clear="onKeywordChange">
              <template #prefixIcon><t-icon name="search" size="16px" /></template>
            </t-input>
          </div>
          <div class="doc-filter-field">
            <t-select v-model="filterInvoiceType" :options="invoiceTypeOptions" placeholder="发票类型"
              class="doc-type-select doc-filter-field__control" clearable @change="applyFilter">
              <template #prefixIcon><t-icon name="file" size="16px" /></template>
            </t-select>
          </div>
          <div class="doc-filter-field">
            <t-select v-model="taxRateFilter" :options="taxRateOptions" placeholder="税率" filterable
              class="doc-type-select doc-filter-field__control" clearable @change="applyFilter">
              <template #prefixIcon><t-icon name="percent" size="16px" /></template>
            </t-select>
          </div>
          <div class="doc-filter-field doc-filter-field--wide">
            <t-date-range-picker v-model="dateRange" placeholder="开票日期" class="doc-date-range doc-filter-field__control"
              clearable allow-input @change="applyFilter">
              <template #prefixIcon><t-icon name="time" size="16px" /></template>
            </t-date-range-picker>
          </div>
          <!-- 字段筛选（列显隐设置） -->
          <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
            overlay-inner-class="invoice-field-popup">
            <t-button variant="outline" size="small">
              <template #icon><t-icon name="view-list" size="14px" /></template>
              字段
            </t-button>
            <template #content>
              <div class="field-popup-content">
                <div class="field-popup-head">
                  <span class="field-popup-title">显示字段</span>
                  <div class="field-popup-actions">
                    <t-button variant="text" size="small" @click="selectAllColumns">全选</t-button>
                    <t-button variant="text" size="small" @click="resetColumns">重置</t-button>
                  </div>
                </div>
                <t-checkbox-group v-model="visibleColKeys" class="field-popup-list" @change="persistColumns">
                  <t-checkbox v-for="col in columnDefs" :key="col.key" :value="col.key" class="field-popup-item">
                    {{ col.label }}
                  </t-checkbox>
                </t-checkbox-group>
              </div>
            </template>
          </t-popup>
          <t-button variant="outline" size="small" @click="applyFilter">
            <template #icon><t-icon name="refresh" size="14px" /></template>
          </t-button>
        </div>
      </div>

      <!-- 发票列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
        <div class="doc-list-view">
          <div class="doc-list-header" :style="gridStyle" role="row">
            <div class="cell cell-check" role="columnheader" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
                :disabled="!selectableRows.length" title="全选" @change="toggleSelectAll" />
            </div>
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
              {{ col.label }}
            </div>
          </div>
          <div class="doc-list-body">
            <div v-for="row in filteredRows" :key="row.rowKey" class="doc-list-row" :style="gridStyle"
              :class="{ selected: selectedRowKeys.includes(row.rowKey) }" role="row" @click="openDetail(row)">
              <div class="cell cell-check" @click.stop>
                <t-checkbox class="doc-list-check" size="small" :checked="selectedRowKeys.includes(row.rowKey)"
                  :disabled="row.kind === 'pending'" @change="(c: boolean) => toggleRow(row.rowKey, c)" />
              </div>
              <!-- 发票号码（同一上传文件的多张发票显示绿色页码标签） -->
              <div v-if="colVisible('invoiceNo')" class="cell cell-invoiceNo">
                <span class="row-invoice-no" :title="row.invoiceNo || row.fileName">{{ row.invoiceNo }}</span>
                <span v-if="pageBadgeOf(row)" class="row-page-badge">{{ pageBadgeOf(row) }}</span>
              </div>
              <!-- 开票日期 -->
              <div v-if="colVisible('invoiceDate')" class="cell cell-invoiceDate">
                <span class="row-mono">{{ row.invoiceDate }}</span>
              </div>
              <!-- 发票类型 -->
              <div v-if="colVisible('invoiceType')" class="cell cell-invoiceType">
                <span class="row-text" :title="row.invoiceType">{{ row.invoiceType }}</span>
              </div>
              <!-- 金额 -->
              <div v-if="colVisible('amount')" class="cell cell-amount">
                <span class="row-mono row-amount">{{ formatAmount(row.amount) }}</span>
              </div>
              <!-- 税率（展示金额最大项税率；多档时 tooltip 显示全部档位） -->
              <div v-if="colVisible('taxRate')" class="cell cell-taxRate">
                <t-tooltip v-if="rateVariants(row).length > 1" :content="rateVariants(row).map(r => formatRate(r)).join('、')">
                  <span class="row-mono row-rate">{{ formatRate(row.taxRate) }}</span>
                </t-tooltip>
                <span v-else class="row-mono row-rate">{{ formatRate(row.taxRate) }}</span>
              </div>
              <!-- 税额 -->
              <div v-if="colVisible('tax')" class="cell cell-tax">
                <span class="row-mono">{{ formatAmount(row.tax) }}</span>
              </div>
              <!-- 价税合计 -->
              <div v-if="colVisible('totalAmount')" class="cell cell-totalAmount">
                <span class="row-mono row-amount">{{ formatAmount(row.totalAmount) }}</span>
              </div>
              <!-- 购买方 -->
              <div v-if="colVisible('buyerName')" class="cell cell-buyerName">
                <span class="row-text" :title="row.buyerName">{{ row.buyerName }}</span>
              </div>
              <!-- 销售方 -->
              <div v-if="colVisible('sellerName')" class="cell cell-sellerName">
                <span class="row-text" :title="row.sellerName">{{ row.sellerName }}</span>
              </div>
              <!-- 开票人 -->
              <div v-if="colVisible('issuer')" class="cell cell-issuer">
                <span class="row-text">{{ row.issuer }}</span>
              </div>
              <!-- 备注 -->
              <div v-if="colVisible('remark')" class="cell cell-remark">
                <span class="row-text" :title="row.remark">{{ row.remark }}</span>
              </div>
              <!-- 状态 -->
              <div v-if="colVisible('extractStatus')" class="cell cell-extractStatus">
                <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
                  variant="light-outline" class="row-status-tag">
                  <template v-if="statusOf(row).icon" #icon>
                    <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
                  </template>
                  {{ statusOf(row).label }}
                </t-tag>
                <span v-else class="row-muted">--</span>
              </div>
              <!-- 标签（显示 1 个 + N） -->
              <div v-if="colVisible('tags')" class="cell cell-tags" @click.stop>
                <t-tooltip v-if="rowTags(row).length" :content="rowTags(row).map((t: any) => t.name).join('、')"
                  placement="top">
                  <div class="row-tag-chips is-clickable" @click="openTagEdit(row)">
                    <t-tag v-if="rowTags(row).length" size="small" variant="light-outline" class="row-tag">
                      {{ rowTags(row)[0].name }}
                    </t-tag>
                    <span v-if="rowTags(row).length > 1" class="row-tag-more">+{{ rowTags(row).length - 1 }}</span>
                  </div>
                </t-tooltip>
                <span v-else class="row-tag-chips is-clickable" @click="openTagEdit(row)">
                  <span class="row-tag-add">+ 标签</span>
                </span>
              </div>
              <!-- 销售方税号 -->
              <div v-if="colVisible('sellerTaxNo')" class="cell cell-sellerTaxNo">
                <span class="row-mono" :title="row.sellerTaxNo">{{ row.sellerTaxNo }}</span>
              </div>
              <!-- 购买方税号 -->
              <div v-if="colVisible('buyerTaxNo')" class="cell cell-buyerTaxNo">
                <span class="row-mono" :title="row.buyerTaxNo">{{ row.buyerTaxNo }}</span>
              </div>
              <!-- 文件名 -->
              <div v-if="colVisible('fileName')" class="cell cell-fileName">
                <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
              </div>
            </div>
            <div v-if="!filteredRows.length && !listLoading" class="doc-empty-state">
              <t-empty description="暂无发票，请点击右上角「上传发票」" />
            </div>
            <div v-if="loadingMore" class="doc-load-more">
              <t-loading size="small" text="加载中..." />
            </div>
          </div>
        </div>
      </div>

      <!-- 底部汇总（选中记录时显示选中发票汇总，未选中显示全部；选中时避让底部工具栏） -->
      <div class="doc-summary-bar" :class="{ 'is-batch-visible': selectedRowKeys.length }">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 条</span>
        <span v-if="displaySummary.total" class="doc-summary-item">
          金额 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumAmount) }}</span>
        </span>
        <span v-if="displaySummary.total" class="doc-summary-item">
          税额 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTax) }}</span>
        </span>
        <span v-if="displaySummary.total" class="doc-summary-item">
          价税合计 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTotal) }}</span>
        </span>
      </div>

      <!-- 底部浮动工具栏（选中行时显示；打印弹窗打开时隐藏，避免浮于弹窗之上） -->
      <transition name="batch-bar-fade">
        <div v-if="selectedRowKeys.length && !printVisible" class="doc-batch-bar-fixed" role="region">
          <div class="batch-bar-inner">
            <div class="batch-bar-left">
              <span class="batch-bar-count">已选 {{ selectedRowKeys.length }} 项</span>
              <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
                清除
              </t-button>
            </div>
            <div class="batch-bar-actions">
              <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handlePageExtract">
                <template #icon><t-icon name="refresh" size="14px" /></template>
                页面提取
              </t-button>
              <t-popconfirm theme="warning"
                :content="`确定重新解析并提取「${selectedSingle?.fileName || '该文件'}」的所有页面发票吗？将覆盖已有提取结果。`"
                :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleFileExtract">
                <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                  <template #icon><t-icon name="refresh" size="14px" /></template>
                  单文件提取
                </t-button>
              </t-popconfirm>
              <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handleBatchEdit">
                <template #icon><t-icon name="edit" size="14px" /></template>
                编辑数据
              </t-button>
              <t-button theme="default" variant="outline" size="small" @click="handleBatchPrint">
                <template #icon><t-icon name="print" size="14px" /></template>
                打印
              </t-button>
              <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedRowKeys.length} 个发票文件吗？删除后不可恢复。`"
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
    </div>

    <!-- 创建知识库向导 -->
    <InvoiceKbWizard v-model:visible="wizardVisible" @created="onKbCreated" />

    <!-- 发票详情抽屉（三 tab，可拖宽，竖向滚动） -->
    <div v-if="detailVisible" class="doc-drawer-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
      <div class="doc-drawer-resize-line" />
    </div>
    <t-drawer v-model:visible="detailVisible" :header="detailTitle" :size="`${drawerWidth}px`" :footer="false"
      destroy-on-close class="invoice-detail-drawer">
      <div class="invoice-detail-body">
        <!-- 摘要 -->
        <section class="detail-block">
          <div class="detail-block-title">摘要</div>
          <div class="detail-block-content">
            <template v-if="currentDetail?.description">
              <div v-if="summaryState" class="summary-status">
                <t-tag :theme="summaryState.theme" variant="light" size="small">
                  <template v-if="summaryState.icon" #icon>
                    <t-icon :name="summaryState.icon" :class="{ 'icon-spin': summaryState.spin }" />
                  </template>
                  {{ summaryState.label }}
                </t-tag>
              </div>
              <!-- 摘要：复用知识库文档抽屉样式（线框 + 展开/折叠 + 每条字段一行） -->
              <div class="summary_wrapper" :class="{ 'summary_clickable': summaryOverflow || summaryExpanded }"
                @click="(summaryOverflow || summaryExpanded) && (summaryExpanded = !summaryExpanded)">
                <div ref="summaryRef" :class="['summary_content', { 'summary_collapsed': !summaryExpanded }]">{{
                  summaryLines
                }}</div>
                <div v-if="(summaryOverflow && !summaryExpanded) || summaryExpanded" class="summary_fade"
                  :class="{ 'summary_fade_expanded': summaryExpanded }">
                  <t-icon :name="summaryExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="summary_fade_icon" />
                </div>
              </div>
            </template>
            <t-empty v-else description="暂无摘要" />
          </div>
        </section>

        <!-- 发票字段 -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">发票信息</div>
                <div class="field-grid">
                  <t-form-item label="发票号码" label-width="100px">
                    <t-input v-model="editForm.invoice_no" placeholder="" />
                  </t-form-item>
                  <t-form-item label="开票日期" label-width="100px">
                    <t-date-picker v-model="editForm.invoice_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input style="width: 100%" />
                  </t-form-item>
                  <t-form-item label="发票类型" label-width="100px">
                    <t-select v-model="editForm.invoice_type" :options="invoiceTypeOptions" clearable allow-create
                      filterable placeholder="选择或输入类型" style="width: 100%" />
                  </t-form-item>
                  <t-form-item label="金额" label-width="100px">
                    <t-input v-model="editForm.amount" placeholder="" @input="(v: string) => (editForm.amount = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="税额" label-width="100px">
                    <t-input v-model="editForm.tax" placeholder="" @input="(v: string) => (editForm.tax = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="价税合计" label-width="100px">
                    <t-input v-model="editForm.total_amount" placeholder="" @input="(v: string) => (editForm.total_amount = sanitizeNum(v))" />
                  </t-form-item>
                  <t-form-item label="开票人" label-width="100px">
                    <t-input v-model="editForm.issuer" placeholder="" />
                  </t-form-item>
                  <t-form-item label="作废标记" label-width="100px">
                    <t-switch v-model="editForm.void_flag" size="small" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">销售方</div>
                <div class="field-grid">
                  <t-form-item label="名称" label-width="100px">
                    <t-input v-model="editForm.seller_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="统一社会信用代码" label-width="100px">
                    <t-input v-model="editForm.seller_tax_no" placeholder="" />
                  </t-form-item>
                  <t-form-item label="地址" label-width="100px">
                    <t-input v-model="editForm.seller_address" placeholder="" />
                  </t-form-item>
                  <t-form-item label="电话" label-width="100px">
                    <t-input v-model="editForm.seller_phone" placeholder="" />
                  </t-form-item>
                  <t-form-item label="开户行" label-width="100px">
                    <t-input v-model="editForm.seller_bank" placeholder="" />
                  </t-form-item>
                  <t-form-item label="账号" label-width="100px">
                    <t-input v-model="editForm.seller_account" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">购买方</div>
                <div class="field-grid">
                  <t-form-item label="名称" label-width="100px">
                    <t-input v-model="editForm.buyer_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="统一社会信用代码" label-width="100px">
                    <t-input v-model="editForm.buyer_tax_no" placeholder="" />
                  </t-form-item>
                  <t-form-item label="地址" label-width="100px">
                    <t-input v-model="editForm.buyer_address" placeholder="" />
                  </t-form-item>
                  <t-form-item label="电话" label-width="100px">
                    <t-input v-model="editForm.buyer_phone" placeholder="" />
                  </t-form-item>
                  <t-form-item label="开户行" label-width="100px">
                    <t-input v-model="editForm.buyer_bank" placeholder="" />
                  </t-form-item>
                  <t-form-item label="账号" label-width="100px">
                    <t-input v-model="editForm.buyer_account" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">备注</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="备注" label-width="100px">
                    <t-textarea v-model="editForm.remark" :autosize="{ minRows: 2, maxRows: 5 }" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <!-- 明细 items -->
              <div class="items-section">
                <div class="items-header">
                  <span class="items-title">项目明细</span>
                  <t-button size="small" variant="outline" @click="addItemRow">
                    <template #icon><t-icon name="add" size="14px" /></template>
                    添加明细
                  </t-button>
                </div>
                <div v-if="(editForm.items || []).length" class="items-table">
                  <div class="items-row items-row--head">
                    <span class="item-cell item-name">项目名称</span>
                    <span class="item-cell item-num">数量</span>
                    <span class="item-cell item-num">单价</span>
                    <span class="item-cell item-num">税率</span>
                    <span class="item-cell item-op"></span>
                  </div>
                  <div v-for="(it, idx) in editForm.items || []" :key="idx" class="items-row">
                    <t-input v-model="it.name" size="small" class="item-cell item-name" placeholder="" />
                    <t-input :value="numToStr(it.qty)" size="small" class="item-cell item-num" placeholder=""
                      @input="(v: string) => (it.qty = parseNum(v))" />
                    <t-input :value="numToStr(it.price)" size="small" class="item-cell item-num" placeholder=""
                      @input="(v: string) => (it.price = parseNum(v))" />
                    <t-input :value="it.tax_rate" size="small" class="item-cell item-num" placeholder="" suffix="%"
                      @input="(v: string) => (it.tax_rate = sanitizeNum(v))" />
                    <div class="item-cell item-op">
                      <t-button variant="text" theme="danger" size="small" @click="removeItemRow(idx)">
                        <t-icon name="delete" />
                      </t-button>
                    </div>
                  </div>
                </div>
                <div v-else class="items-empty">暂无明细</div>
              </div>

              <div class="detail-save-hint">
                <t-icon name="check-circle" size="14px" />
                <span>字段修改后将自动保存{{ autoSaving ? '（保存中...）' : '' }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 源文件预览 -->
        <section class="detail-block">
          <div class="detail-block-title">源文件预览</div>
          <div class="detail-block-content">
            <DocumentPreview v-if="currentRow" :knowledge-id="currentRow.knowledgeId"
              :file-type="currentRow.fileType" :file-name="currentRow.fileName" :active="true" />
          </div>
        </section>
      </div>
    </t-drawer>

    <!-- 标签编辑（复用原项目组件） -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理抽屉（复用原项目组件） -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 打印预览弹窗（自定义实现，完全可控；弃用 TDesign dialog） -->
    <teleport to="body">
      <div v-if="printVisible" class="invoice-print-mask">
        <div class="invoice-print-dialog">
          <div class="invoice-print-header">
            <span class="invoice-print-title">打印预览（{{ printCount }} 张）</span>
            <t-button variant="text" size="small" class="invoice-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="invoice-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" text="正在合并发票 PDF…" />
            </div>
          </div>
          <div class="invoice-print-footer">
            <t-button variant="outline" size="small" @click="printVisible = false">关闭</t-button>
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
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  uploadKnowledgeFile,
  getKnowledgeDetails,
  delKnowledgeDetails,
  batchDeleteKnowledge,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractInvoice,
  extractInvoicePage,
  deleteInvoicePage,
  listInvoiceRecords,
  listInvoiceTaxRates,
  previewKnowledgeFile,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import InvoiceKbWizard from './InvoiceKbWizard.vue'

const KB_NAME = '日常事务-发票'
const ACCEPT_TYPES = ['pdf', 'jpg', 'jpeg', 'png']
const PAGE_SIZE = 20

// 发票类型枚举（编辑/筛选推荐值，allow-create 兼容提取值）
const INVOICE_TYPES = ['专用发票', '普通发票', '医疗收据', '财政收据', '其它票据']

// 字段定义（列显隐设置）
interface ColumnDef {
  key: string
  label: string
  default: boolean
  w: string
}
const COLUMN_DEFS: ColumnDef[] = [
  { key: 'invoiceNo', label: '发票号码', default: true, w: '1.5fr' },
  { key: 'invoiceDate', label: '开票日期', default: true, w: '1.1fr' },
  { key: 'invoiceType', label: '发票类型', default: true, w: '1fr' },
  { key: 'amount', label: '金额', default: true, w: '1.1fr' },
  { key: 'taxRate', label: '税率', default: true, w: '0.7fr' },
  { key: 'tax', label: '税额', default: true, w: '1fr' },
  { key: 'totalAmount', label: '价税合计', default: true, w: '1.2fr' },
  { key: 'buyerName', label: '购买方', default: true, w: '1.5fr' },
  { key: 'sellerName', label: '销售方', default: true, w: '1.5fr' },
  { key: 'issuer', label: '开票人', default: true, w: '0.9fr' },
  { key: 'remark', label: '备注', default: false, w: '1.3fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'sellerTaxNo', label: '销售方税号', default: false, w: '1.3fr' },
  { key: 'buyerTaxNo', label: '购买方税号', default: false, w: '1.3fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const COLUMN_STORAGE_KEY = 'weknora-invoice-list-columns'

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const fileInputRef = ref<HTMLInputElement>()

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

interface InvoiceItem {
  invoice_no?: string
  invoice_code?: string
  invoice_date?: string
  invoice_type?: string
  total_amount?: number
  amount?: number
  tax?: number
  tax_rate?: number
  seller_name?: string
  seller_tax_no?: string
  seller_address?: string
  seller_phone?: string
  seller_bank?: string
  seller_account?: string
  buyer_name?: string
  buyer_tax_no?: string
  buyer_address?: string
  buyer_phone?: string
  buyer_bank?: string
  buyer_account?: string
  issuer?: string
  remark?: string
  items?: Array<{ name?: string; qty?: number; price?: number; tax_rate?: number }>
  void_flag?: boolean
  duplicate?: boolean
  page?: number
}

interface InvoiceRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'invoice' | 'not_invoice' | 'unknown'
  invoiceNo?: string
  invoiceDate?: string
  invoiceType?: string
  totalAmount?: number
  amount?: number
  tax?: number
  sellerName?: string
  sellerTaxNo?: string
  sellerAddress?: string
  sellerPhone?: string
  sellerBank?: string
  sellerAccount?: string
  buyerName?: string
  buyerTaxNo?: string
  buyerAddress?: string
  buyerPhone?: string
  buyerBank?: string
  buyerAccount?: string
  issuer?: string
  remark?: string
  items?: InvoiceItem['items']
  voidFlag?: boolean
  duplicate?: boolean
  page?: number
  taxRate?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}

interface StatusInfo {
  label: string
  theme: 'success' | 'warning' | 'danger' | 'primary' | 'default'
  icon?: string
  spin?: boolean
}

// ---- 列表 ----
const items = ref<KnowledgeItem[]>([])
const invoiceRows = ref<InvoiceRow[]>([])
// 进行中的文件（解析中/提取中/待提取），在发票级列表顶部以状态行展示
const pendingFiles = ref<KnowledgeItem[]>([])
const invoiceSummary = ref<{ total: number; sumAmount: number; sumTax: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTax: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const filterInvoiceType = ref('')
const taxRateFilter = ref<number | string>('')
const taxRateOptions = ref<Array<{ value: string; label: string }>>([])
const dateRange = ref<Array<string>>([])
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const extractFailed = ref<Set<string>>(new Set())

// 列显隐
const visibleColKeys = ref<string[]>(loadStoredColumns())
const fieldPopupVisible = ref(false)
const columnDefs = COLUMN_DEFS
const visibleColDefs = computed(() => COLUMN_DEFS.filter(c => visibleColKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))

function loadStoredColumns(): string[] {
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) return arr.filter((k: string) => COLUMN_DEFS.some(c => c.key === k))
    }
  } catch { /* ignore */ }
  return COLUMN_DEFS.filter(c => c.default).map(c => c.key)
}
function colVisible(key: string) { return visibleColKeys.value.includes(key) }
function selectAllColumns() { visibleColKeys.value = COLUMN_DEFS.map(c => c.key) }
function resetColumns() { visibleColKeys.value = COLUMN_DEFS.filter(c => c.default).map(c => c.key) }
function persistColumns() {
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
// 字段显隐变化即持久化：t-checkbox-group 的 @change 在部分勾选交互下不触发，
// 用 watch 兜底，确保取消列（如备注）后硬刷新不恢复默认。
watch(visibleColKeys, () => persistColumns(), { deep: true })

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<InvoiceRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<InvoiceRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({ items: [] })
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null
let autoSaveDirty = false
const autoSaving = ref(false)

// 摘要（复用知识库文档抽屉样式：线框 + 展开/折叠 + 每条字段一行）
const summaryExpanded = ref(false)
const summaryRef = ref<HTMLElement>()
const summaryOverflow = ref(false)
const summaryLines = computed(() => {
  const d = currentDetail.value?.description || ''
  if (!d) return ''
  // 以 "-" 作为每条字段起点分行展示，避免全部堆在一行
  return d.replace(/-(?=[^\s-])/g, '\n-').trim()
})
const checkSummaryOverflow = () => {
  const el = summaryRef.value
  if (!el) { summaryOverflow.value = false; return }
  summaryOverflow.value = el.scrollHeight > el.clientHeight + 1
}
watch(summaryRef, () => checkSummaryOverflow())
watch(() => currentDetail.value?.description, () => {
  summaryExpanded.value = false
  nextTick(() => checkSummaryOverflow())
})

// 抽屉宽度（可拖动，localStorage 记忆）
const DRAWER_WIDTH_KEY = 'weknora-invoice-drawer-width'
const DRAWER_DEFAULT_WIDTH = 654
const DRAWER_MIN_WIDTH = 480
const drawerWidth = ref(DRAWER_DEFAULT_WIDTH)
const drawerResizing = ref(false)
let drawerResizeStartX = 0
let drawerResizeStartWidth = 0

// 打印预览
const printVisible = ref(false)
const printCount = ref(0)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null

// ---- KB 检测 ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      loadDrawerWidth()
      await loadTags()
      await loadTaxRates()
      await cleanNonInvoiceFiles()
      await loadFiles()
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
    loadDrawerWidth()
    await loadTags()
    await loadTaxRates()
    await cleanNonInvoiceFiles()
    await loadFiles()
    startPolling()
  }
}

// ---- 存量非发票清理：扫描知识库中已标记 not_invoice 的文件并自动删除 ----
let cleaningNonInvoice = false
const cleanNonInvoiceFiles = async () => {
  if (!kbId.value || cleaningNonInvoice) return
  cleaningNonInvoice = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_invoice' || it.custom_metadata?.extract_status === 'not_invoice')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已自动移除 ${bad.length} 个非发票文件：${bad.map((b: any) => b.file_name || b.title).join('、')}`)
      loadTaxRates()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonInvoice = false }
}

// ---- 税率下拉：加载该知识库下所有发票出现过的去重税率 ----
const loadTaxRates = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listInvoiceTaxRates(kbId.value)
    const list = res?.data || res?.list || []
    const arr = Array.isArray(list) ? list : []
    taxRateOptions.value = arr.map((r: any) => ({ value: Number(r.tax_rate), label: formatRate(r.tax_rate) }))
  } catch { /* 税率加载失败不阻塞 */ }
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

const rowTags = (row: InvoiceRow) => {
  const arr = row.tags || []
  return Array.isArray(arr) ? arr : []
}

const openTagEdit = (row: InvoiceRow) => {
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

// ---- 列表加载（发票级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    invoiceRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) {
    return
  } else {
    loadingMore.value = true
  }
  try {
    const res: any = await listInvoiceRecords(kbId.value, {
      q: keyword.value || undefined,
      invoice_type: filterInvoiceType.value || undefined,
      tax_rate: taxRateFilter.value === '' || taxRateFilter.value === null || taxRateFilter.value === undefined
        ? undefined : Number(taxRateFilter.value),
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    invoiceSummary.value = {
      total,
      sumAmount: Number(res?.sum_amount || 0),
      sumTax: Number(res?.sum_tax || 0),
      sumTotal: Number(res?.sum_total || 0),
    }
    const existing = new Set(invoiceRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapInvoiceRecord).filter(r => !existing.has(r.rowKey))
    invoiceRows.value = [...invoiceRows.value, ...fresh]
    hasMore.value = invoiceRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '发票列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

// 把后端发票级记录（InvoiceRecord）映射为前端行
const mapInvoiceRecord = (r: any): InvoiceRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.invoice_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'invoice',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  invoiceNo: r.invoice_no || '',
  invoiceDate: r.invoice_date || '',
  invoiceType: r.invoice_type || '',
  totalAmount: numOrUndef(r.total_amount),
  amount: numOrUndef(r.amount),
  tax: numOrUndef(r.tax),
  taxRate: numOrUndef(r.tax_rate),
  sellerName: r.seller_name || '',
  sellerTaxNo: r.seller_tax_no || '',
  sellerAddress: r.seller_address || '',
  sellerPhone: r.seller_phone || '',
  sellerBank: r.seller_bank || '',
  sellerAccount: r.seller_account || '',
  buyerName: r.buyer_name || '',
  buyerTaxNo: r.buyer_tax_no || '',
  buyerAddress: r.buyer_address || '',
  buyerPhone: r.buyer_phone || '',
  buyerBank: r.buyer_bank || '',
  buyerAccount: r.buyer_account || '',
  issuer: r.issuer || '',
  remark: r.remark || '',
  items: Array.isArray(r.items) ? r.items : [],
  voidFlag: !!r.void_flag,
  duplicate: !!r.duplicate,
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 发票级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

const parseCustomMetadata = (item: KnowledgeItem): InvoiceRow[] => {
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
  if (kind === 'not_invoice') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_invoice', extractError, kind, ...base }]
  }
  const invoices = Array.isArray(meta.invoices) ? meta.invoices : []
  if (invoices.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return invoices.map((inv: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: invoices.length > 1 ? `${fileName}（第${idx + 1}张）` : fileName,
    multiIndex: invoices.length > 1 ? `${idx + 1}/${invoices.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    invoiceNo: inv.invoice_no || '',
    invoiceDate: inv.invoice_date || '',
    invoiceType: inv.invoice_type || '',
    totalAmount: numOrUndef(inv.total_amount),
    amount: numOrUndef(inv.amount),
    tax: numOrUndef(inv.tax),
    taxRate: numOrUndef(inv.tax_rate),
    page: Number(inv.page) || 0,
    sellerName: inv.seller_name || '',
    sellerTaxNo: inv.seller_tax_no || '',
    sellerAddress: inv.seller_address || '',
    sellerPhone: inv.seller_phone || '',
    sellerBank: inv.seller_bank || '',
    sellerAccount: inv.seller_account || '',
    buyerName: inv.buyer_name || '',
    buyerTaxNo: inv.buyer_tax_no || '',
    buyerAddress: inv.buyer_address || '',
    buyerPhone: inv.buyer_phone || '',
    buyerBank: inv.buyer_bank || '',
    buyerAccount: inv.buyer_account || '',
    issuer: inv.issuer || '',
    remark: inv.remark || '',
    items: Array.isArray(inv.items) ? inv.items : [],
    voidFlag: !!inv.void_flag,
    duplicate: !!inv.duplicate,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端发票级接口完成，前端直接使用返回的分页数据；
// 顶部合并"进行中文件"状态行（解析中/提取中/待提取），提取完成即消失
const pendingRows = computed<InvoiceRow[]>(() => pendingFiles.value.map((k) => {
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
  } as InvoiceRow
}))
const filteredRows = computed(() => [...pendingRows.value, ...invoiceRows.value])

// 同一上传文件内的多张发票：按文件分组计数，用于生成绿色页码标签
const fileInvoiceCounts = computed(() => {
  const map = new Map<string, number>()
  for (const r of invoiceRows.value) {
    if (!r.knowledgeId) continue
    map.set(r.knowledgeId, (map.get(r.knowledgeId) || 0) + 1)
  }
  return map
})
// 仅同一文件含多张发票时显示：有真实页码显示 P{n}，否则显示"第n张/共m张"
const pageBadgeOf = (row: InvoiceRow): string => {
  if (!row.knowledgeId) return ''
  const count = fileInvoiceCounts.value.get(row.knowledgeId) || 0
  if (count <= 1) return ''
  if (row.page && row.page >= 1) return `P${row.page}`
  const same = invoiceRows.value.filter(r => r.knowledgeId === row.knowledgeId)
  const idx = same.findIndex(r => r.rowKey === row.rowKey)
  return idx >= 0 ? `${idx + 1}/${count}` : ''
}

const invoiceTypeOptions = computed(() => INVOICE_TYPES.map(v => ({ value: v, label: v })))

// ---- 状态列（复用原项目"绿色 loading 动态"样式） ----
const extractStatusOf = (row: InvoiceRow): string => {
  const ps = row.parseStatus
  if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return 'parsing'
  if (ps === 'failed') return 'parse_failed'
  if (ps === 'completed') {
    if (extractInFlight.value.has(row.knowledgeId)) return 'processing'
    // 本地失败标记优先（后端尚未落 failed 状态时避免无限"待提取"）
    if (extractFailed.value.has(row.knowledgeId)) return 'failed'
    const es = row.extractStatus
    if (!es || es === 'pending' || es === 'processing') return 'pending'
    if (es === 'success') return 'success'
    if (es === 'failed') return 'failed'
    if (es === 'not_invoice') return 'not_invoice'
  }
  return ''
}

const statusOf = (row: InvoiceRow): StatusInfo => {
  const s = extractStatusOf(row)
  switch (s) {
    case 'parsing': return { label: '解析中', theme: 'primary', icon: 'loading', spin: true }
    case 'parse_failed': return { label: '解析失败', theme: 'danger', icon: 'close-circle' }
    case 'pending': return { label: '待提取', theme: 'primary', icon: 'loading', spin: true }
    case 'processing': return { label: '提取中', theme: 'primary', icon: 'loading', spin: true }
    case 'success': return { label: '提取成功', theme: 'success' }
    case 'failed': return { label: '提取失败', theme: 'danger', icon: 'close-circle' }
    case 'not_invoice': return { label: '非发票', theme: 'default' }
    default: return { label: '待解析', theme: 'default' }
  }
}

const summaryState = computed<StatusInfo | null>(() => {
  const ss = currentDetail.value?.summary_status
  if (!ss) return null
  if (ss === 'pending' || ss === 'processing') return { label: '摘要生成中', theme: 'primary', icon: 'loading', spin: true }
  if (ss === 'completed') return { label: '摘要已生成', theme: 'success' }
  if (ss === 'failed') return { label: '摘要生成失败', theme: 'danger' }
  return null
})

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
const clearSelection = () => { selectedRowKeys.value = [] }
const selectedRows = computed(() => {
  const byKey = new Map(invoiceRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as InvoiceRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中发票的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  let sumAmount = 0
  let sumTax = 0
  let sumTotal = 0
  for (const r of rows) {
    sumAmount += Number(r.amount) || 0
    sumTax += Number(r.tax) || 0
    sumTotal += Number(r.totalAmount) || 0
  }
  return { total: rows.length, sumAmount, sumTax, sumTotal }
})
const displaySummary = computed(() => selectedSummary.value || invoiceSummary.value)

const applyFilter = () => { loadFiles(true) }
const onKeywordChange = () => { loadFiles(true) }

// 懒加载
const listScrollRef = ref<HTMLElement>()
const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200 && hasMore.value && !listLoading.value && !loadingMore.value) {
    loadFiles()
  }
}

// ---- 上传 ----
const triggerUpload = () => { fileInputRef.value?.click() }
const onFileInputChange = (e: Event) => {
  const input = e.target as HTMLInputElement
  if (input.files?.length) handleUploadFiles(Array.from(input.files))
  input.value = ''
}

const handleUploadFiles = async (files: File[]) => {
  const valid = files.filter(f => {
    const ext = (f.name.split('.').pop() || '').toLowerCase()
    return ACCEPT_TYPES.includes(ext)
  })
  const invalidCount = files.length - valid.length
  if (invalidCount) MessagePlugin.warning(`已忽略 ${invalidCount} 个不支持的文件（仅支持 PDF/JPG/PNG）`)
  if (!valid.length) return
  try {
    for (const file of valid) await uploadKnowledgeFile(kbId.value, { file })
    MessagePlugin.success(`已上传 ${valid.length} 个发票文件，正在解析...`)
    await loadFiles(true)
    ensurePolling()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '上传失败')
  }
}

// ---- 轮询解析 + 提取 ----
const startPolling = () => ensurePolling()
const ensurePolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(async () => { await pollTick() }, 3000)
}

// 轮询刷新：
// 1) probeExtract 用 knowledge 级列表探测"已解析完成但尚未提取"的新上传文件并触发提取；
// 2) refreshInvoiceRows 刷新发票级聚合列表，把提取完成的发票行实时合并进列表。
let refreshBusy = false
const probeExtract = async () => {
  if (!kbId.value || refreshBusy) return
  refreshBusy = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr: KnowledgeItem[] = Array.isArray(data) ? data : []
    const needExtract: KnowledgeItem[] = []
    const pend: KnowledgeItem[] = []
    for (const item of arr) {
      const ps = item.parse_status
      if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') {
        pend.push(item)
        continue
      }
      if (ps === 'completed') {
        const es = item.custom_metadata?.extract_status
        if ((!es || es === 'pending' || es === 'processing') &&
            !extractInFlight.value.has(item.id) && !extractFailed.value.has(item.id)) {
          needExtract.push(item)
        }
        if (es && es !== 'pending' && es !== 'processing') extractFailed.value.delete(item.id)
        if (!es || es === 'pending' || es === 'processing') pend.push(item)
      }
    }
    // 已有发票行（提取完成）的文件不再显示进行中状态行
    const withRows = new Set(invoiceRows.value.map(r => r.knowledgeId))
    pendingFiles.value = pend.filter(k => !withRows.has(k.id))
    for (const item of needExtract.slice(0, 5)) {
      extractInFlight.value.add(item.id)
      try {
        const r: any = await extractInvoice(kbId.value, item.id)
        // 后端判定非发票并已自动删除该文件 → 提示并刷新税率
        if (r?.data?.removed) {
          MessagePlugin.info(`「${item.file_name || item.title}」不是电子发票，已自动移除`)
          loadTaxRates()
        }
      } catch {
        extractFailed.value.add(item.id)
      }
    }
    for (const id of Array.from(extractInFlight.value)) {
      const it = arr.find(k => k.id === id)
      if (it?.custom_metadata?.extract_status) extractInFlight.value.delete(id)
    }
  } catch { /* 轮询探测失败静默，下轮重试 */ }
  finally { refreshBusy = false }
}

// 刷新发票级列表（合并更新已加载行，提取完成的新数据实时出现）
let invoiceRefreshBusy = false
const refreshInvoiceRows = async () => {
  if (!kbId.value || invoiceRefreshBusy) return
  invoiceRefreshBusy = true
  try {
    const res: any = await listInvoiceRecords(kbId.value, {
      q: keyword.value || undefined,
      invoice_type: filterInvoiceType.value || undefined,
      tax_rate: taxRateFilter.value === '' || taxRateFilter.value === null || taxRateFilter.value === undefined
        ? undefined : Number(taxRateFilter.value),
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: 1,
      page_size: Math.max(invoiceRows.value.length, PAGE_SIZE),
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    invoiceSummary.value = {
      total,
      sumAmount: Number(res?.sum_amount || 0),
      sumTax: Number(res?.sum_tax || 0),
      sumTotal: Number(res?.sum_total || 0),
    }
    const byKey = new Map(invoiceRows.value.map(r => [r.rowKey, r]))
    const fresh: InvoiceRow[] = []
    for (const r of arr) {
      const row = mapInvoiceRecord(r)
      const old = byKey.get(row.rowKey)
      if (old) {
        // 已存在行：合并最新字段（提取完成后从空变有值）
        const idx = invoiceRows.value.findIndex(x => x.rowKey === row.rowKey)
        if (idx >= 0) invoiceRows.value[idx] = { ...old, ...row }
      } else {
        fresh.push(row)
      }
    }
    if (fresh.length) invoiceRows.value = [...fresh, ...invoiceRows.value]
  } catch { /* 发票列表刷新失败静默 */ }
  finally { invoiceRefreshBusy = false }
}

const pollTick = async () => {
  if (!kbId.value || document.hidden) return
  await probeExtract()
  await refreshInvoiceRows()
}
const stopPolling = () => {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- 抽屉宽度拖动（复刻原项目） ----
function drawerMaxWidth() { return Math.min(1600, Math.max(DRAWER_MIN_WIDTH, Math.floor(window.innerWidth * 0.95))) }
function clampDrawerWidth(w: number) { return Math.max(DRAWER_MIN_WIDTH, Math.min(drawerMaxWidth(), w)) }
function loadDrawerWidth() {
  try {
    const raw = localStorage.getItem(DRAWER_WIDTH_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isNaN(parsed)) drawerWidth.value = clampDrawerWidth(parsed)
  } catch { /* ignore */ }
}
function onDrawerResizeStart(e: MouseEvent) {
  drawerResizing.value = true
  drawerResizeStartX = e.clientX
  drawerResizeStartWidth = drawerWidth.value
  document.addEventListener('mousemove', onDrawerResizeMove)
  document.addEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}
function onDrawerResizeMove(e: MouseEvent) {
  const delta = drawerResizeStartX - e.clientX
  drawerWidth.value = clampDrawerWidth(drawerResizeStartWidth + delta)
}
function onDrawerResizeEnd() {
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  drawerResizing.value = false
  try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
}

// ---- 详情抽屉 ----
const openDetail = async (row: InvoiceRow) => {
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
        const match = parsed.find(p => row.page && p.page === row.page)
          || parsed.find(p => p.invoiceNo === row.invoiceNo) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.invoiceNo ? `发票详情 · ${currentRow.value.invoiceNo}` : '发票详情'
)

// ---- 字段编辑 + 自动保存 ----
const fillEditForm = () => {
  const r = currentRow.value
  autoSaveDirty = false
  if (!r) return
  editForm.value = {
    invoice_no: r.invoiceNo || '',
    invoice_date: r.invoiceDate || '',
    invoice_type: r.invoiceType || '',
    total_amount: numToStr(r.totalAmount),
    amount: numToStr(r.amount),
    tax: numToStr(r.tax),
    seller_name: r.sellerName || '',
    seller_tax_no: r.sellerTaxNo || '',
    seller_address: r.sellerAddress || '',
    seller_phone: r.sellerPhone || '',
    seller_bank: r.sellerBank || '',
    seller_account: r.sellerAccount || '',
    buyer_name: r.buyerName || '',
    buyer_tax_no: r.buyerTaxNo || '',
    buyer_address: r.buyerAddress || '',
    buyer_phone: r.buyerPhone || '',
    buyer_bank: r.buyerBank || '',
    buyer_account: r.buyerAccount || '',
    issuer: r.issuer || '',
    remark: r.remark || '',
    void_flag: !!r.voidFlag,
    items: Array.isArray(r.items) ? r.items.map(it => ({
      name: it.name || '',
      qty: numToStr(it.qty),
      price: numToStr(it.price),
      // 税率以百分比显示（0.03 → "3"），保存时再转回小数
      tax_rate: it.tax_rate === null || it.tax_rate === undefined ? '' : String(Number(it.tax_rate) * 100),
    })) : [],
  }
  autoSaveDirty = true
}

const addItemRow = () => {
  editForm.value.items = [...(editForm.value.items || []), { name: '', qty: '', price: '', tax_rate: '' }]
}
const removeItemRow = (idx: number) => {
  editForm.value.items = (editForm.value.items || []).filter((_: any, i: number) => i !== idx)
}

watch(editForm, () => {
  if (!autoSaveDirty || !currentRow.value) return
  if (autoSaveTimer) clearTimeout(autoSaveTimer)
  autoSaveTimer = setTimeout(() => { saveEditForm() }, 1200)
}, { deep: true })

const saveEditForm = async () => {
  const now = currentRow.value
  if (!now) return
  autoSaving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const invoices = Array.isArray(meta.invoices) ? [...meta.invoices] : []
    const updated: any = {
      invoice_no: editForm.value.invoice_no || '',
      invoice_date: editForm.value.invoice_date || '',
      invoice_type: editForm.value.invoice_type || '',
      total_amount: toNumber(editForm.value.total_amount),
      amount: toNumber(editForm.value.amount),
      tax: toNumber(editForm.value.tax),
      seller_name: editForm.value.seller_name || '',
      seller_tax_no: editForm.value.seller_tax_no || '',
      seller_address: editForm.value.seller_address || '',
      seller_phone: editForm.value.seller_phone || '',
      seller_bank: editForm.value.seller_bank || '',
      seller_account: editForm.value.seller_account || '',
      buyer_name: editForm.value.buyer_name || '',
      buyer_tax_no: editForm.value.buyer_tax_no || '',
      buyer_address: editForm.value.buyer_address || '',
      buyer_phone: editForm.value.buyer_phone || '',
      buyer_bank: editForm.value.buyer_bank || '',
      buyer_account: editForm.value.buyer_account || '',
      issuer: editForm.value.issuer || '',
      remark: editForm.value.remark || '',
      void_flag: !!editForm.value.void_flag,
      items: Array.isArray(editForm.value.items)
        ? editForm.value.items.map((it: any) => {
            const taxRate = toNumber(it.tax_rate)
            return {
              name: it.name || '',
              qty: toNumber(it.qty),
              price: toNumber(it.price),
              // 税率由百分比字符串转回小数（"3" → 0.03）
              tax_rate: taxRate === undefined ? undefined : taxRate / 100,
            }
          })
        : [],
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = invoices.findIndex((inv: any, i: number) =>
        i === nowPage - 1 || Number(inv.page) === nowPage || inv.invoice_no === now.invoiceNo)
    }
    if (targetIdx < 0) targetIdx = invoices.findIndex((inv: any) => inv.invoice_no === now.invoiceNo)
    if (targetIdx >= 0) invoices[targetIdx] = { ...invoices[targetIdx], ...updated, page: nowPage || invoices[targetIdx]?.page }
    else invoices.push({ ...updated, page: nowPage || 0 })
    const newMeta = { ...meta, kind: 'invoice', invoices, extract_status: 'success', extract_error: '' }
    await updateKnowledgeMetadata(now.knowledgeId, newMeta)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated }
    // 同步列表行，保证编辑（如备注）后列表立即刷新
    const listIdx = invoiceRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      invoiceRows.value[listIdx] = {
        ...invoiceRows.value[listIdx],
        invoiceNo: updated.invoice_no,
        invoiceDate: updated.invoice_date,
        invoiceType: updated.invoice_type,
        amount: updated.amount,
        tax: updated.tax,
        totalAmount: updated.total_amount,
        buyerName: updated.buyer_name,
        sellerName: updated.seller_name,
        issuer: updated.issuer,
        remark: updated.remark,
        voidFlag: updated.void_flag,
        items: updated.items,
      }
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    autoSaving.value = false
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

const formatAmount = (v?: number) =>
  v === undefined || v === null ? '' : `¥ ${Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`

// 税率以百分比显示且不保留小数（0.03 → 3%）
const formatRate = (v?: number) => {
  if (v === undefined || v === null) return ''
  return `${Math.round(Number(v) * 100)}%`
}

// 发票内所有不同税率档（用于 tooltip 展示全部档位；列表展示金额最大项税率）
const rateVariants = (row: InvoiceRow): number[] => {
  const set = new Set<number>()
  for (const it of row.items || []) {
    const r = Number(it?.tax_rate)
    if (Number.isFinite(r) && r > 0) set.add(Math.round(r * 10000) / 10000)
  }
  if (set.size === 0) {
    const t = Number(row.taxRate)
    if (Number.isFinite(t) && t > 0) set.add(Math.round(t * 10000) / 10000)
  }
  return [...set]
}

// ---- 浮动工具栏 ----
const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

// 页面提取：只重新提取选中行所在页（单选）
const handlePageExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('页面提取仅支持单选，请选中一张发票'); return }
  if (!row.page || row.page < 1) {
    MessagePlugin.warning('该发票暂无页码信息，请先执行「单文件提取」')
    return
  }
  extractInFlight.value.add(row.knowledgeId)
  try {
    await extractInvoicePage(kbId.value, row.knowledgeId, row.page)
    MessagePlugin.success(`已重新提取第 ${row.page} 张发票`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '页面提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
  }
}

// 单文件提取：重新解析并提取选中行对应源文件的全部页面（单选，二次确认）
const handleFileExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('单文件提取仅支持单选，请选中一张发票'); return }
  extractInFlight.value.add(row.knowledgeId)
  extractFailed.value.delete(row.knowledgeId)
  try {
    await extractInvoice(kbId.value, row.knowledgeId)
    MessagePlugin.success(`已触发「${row.fileName}」全量重新提取`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    extractFailed.value.add(row.knowledgeId)
    MessagePlugin.error(e?.message || '单文件提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
  }
}

const handleBatchEdit = () => {
  const rows = selectedRows.value
  if (rows.length !== 1) {
    MessagePlugin.info('请选中单行后编辑，或点击列表中的发票行进入编辑')
    return
  }
  openDetail(rows[0])
}

// 打印：把所有选中发票的页面合并为一个 PDF，单 iframe 预览打印
// （跨文件、同文件任意组合均支持；图片发票自动转 PDF 页；无页码记录回退整文件页）
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
      if (!blob) { notes.push(`${r.invoiceNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式发票：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pageCount = pdf.getPageCount()
        if (r.page && r.page >= 1) {
          if (r.page <= pageCount) {
            const [pg] = await out.copyPages(pdf, [r.page - 1])
            out.addPage(pg)
            mergedPages++
          } else {
            notes.push(`${r.invoiceNo || r.fileName}（页码 ${r.page} 超出文件 ${pageCount} 页，已并入整文件）`)
            const pages = await out.copyPages(pdf, pdf.getPageIndices())
            pages.forEach(p => out.addPage(p))
            mergedPages += pages.length
          }
        } else {
          notes.push(`${r.invoiceNo || r.fileName}（无页码，已并入整文件）`)
          const pages = await out.copyPages(pdf, pdf.getPageIndices())
          pages.forEach(p => out.addPage(p))
          mergedPages += pages.length
        }
      } catch {
        notes.push(`${r.invoiceNo || r.fileName}（页抽取失败，已并入整文件）`)
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
      MessagePlugin.error('未获取到可打印的发票页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    printUrl.value = URL.createObjectURL(new Blob([bytes], { type: 'application/pdf' }))
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

// 删除记录：有页码的行按页删除该发票（同文件其它发票保留）；无页码（历史数据）删整份文件
const handleBatchDelete = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    const fileDeleteIds = new Set<string>()
    const pageDeleteIds: Array<{ knowledgeId: string; page: number }> = []
    for (const r of rows) {
      if (r.page && r.page >= 1) pageDeleteIds.push({ knowledgeId: r.knowledgeId, page: r.page })
      else fileDeleteIds.add(r.knowledgeId)
    }
    // 同文件多页删除时，先删大页码再删小页码（后端删后重排页码）
    pageDeleteIds.sort((a, b) => b.page - a.page)
    for (const pd of pageDeleteIds) {
      try {
        const res: any = await deleteInvoicePage(kbId.value, pd.knowledgeId, pd.page)
        if (res?.deleted_file) fileDeleteIds.add(pd.knowledgeId)
      } catch (e: any) {
        MessagePlugin.error(e?.message || `删除发票（${pd.page}）失败`)
        return
      }
    }
    if (fileDeleteIds.size) {
      await batchDeleteKnowledge(kbId.value, Array.from(fileDeleteIds))
    }
    MessagePlugin.success('删除成功')
    selectedRowKeys.value = []
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 生命周期 ----
onMounted(() => { loadKb() })
onBeforeUnmount(() => {
  stopPolling()
  if (autoSaveTimer) clearTimeout(autoSaveTimer)
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
})
</script>

<style scoped lang="less">
.invoice-management-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 24px 32px;
  overflow-y: auto;
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

.invoice-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

/* ---- 筛选工具栏 ---- */
.doc-filter-bar {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  &__leading { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; flex: 1; }
  .doc-filter-field {
    display: flex; align-items: center;
    &--search { min-width: 220px; }
    &--wide { min-width: 260px; }
    .doc-search { width: 220px; }
    .doc-type-select { width: 130px; }
    .doc-date-range { width: 260px; }
  }
}

/* ---- 字段筛选弹层 ---- */
:global(.invoice-field-popup) {
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
  flex: 0 1 auto; /* 高度随内容自适应：1 条就包 1 条，多条向下扩展 */
  max-height: 100%;
  min-width: 0;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

/* ---- 底部汇总 ---- */
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
  &.is-batch-visible { margin-bottom: 72px; } /* 选中时给底部浮动工具栏让位 */
}

.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
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

.cell-invoiceNo, .cell-buyerName, .cell-sellerName, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-invoice-no {
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
.row-muted { color: var(--td-text-color-disabled, #bbb); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-status-tag :deep(.t-icon) { margin-right: 2px; }
.icon-spin { animation: doc-list-spin 0.9s linear infinite; }

.row-tag-chips {
  display: inline-flex; align-items: center; gap: 4px; flex-wrap: nowrap; cursor: pointer;
  .row-tag { max-width: 110px; :deep(.t-tag__text) { max-width: 100px; overflow: hidden; text-overflow: ellipsis; display: inline-block; } }
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

.doc-empty-state { padding: 40px 0; display: flex; justify-content: center; }
.doc-load-more { display: flex; justify-content: center; padding: 12px 0; }
.doc-load-end {
  text-align: center; padding: 10px 0; font-size: 12px; color: var(--td-text-color-placeholder);
}

/* ---- 底部浮动工具栏 ---- */
.doc-batch-bar-fixed {
  position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 50;
  width: 100%; max-width: 700px; padding: 0 4px; box-sizing: border-box;
}
.batch-bar-inner {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  padding: 8px 12px; background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke); border-radius: 8px; box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12);
}
.batch-bar-left { display: flex; align-items: center; gap: 4px; min-width: 0; flex: 1; }
.batch-bar-count { font-size: 13px; font-weight: 500; color: var(--td-text-color-secondary); white-space: nowrap; }
.batch-bar-clear { flex-shrink: 0; padding: 0 6px !important; height: 28px !important; font-size: 12px; color: var(--td-text-color-secondary) !important; &:hover { color: var(--td-brand-color) !important; } }
.batch-bar-actions { flex-shrink: 0; display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; }
.batch-bar-fade-enter-active, .batch-bar-fade-leave-active { transition: transform 0.2s ease, opacity 0.2s ease; }
.batch-bar-fade-enter-from, .batch-bar-fade-leave-to { opacity: 0; transform: translate(-50%, 6px); }

/* ---- 详情抽屉：竖向区块 + 可拖宽（复用知识库文档抽屉上下滚动样式） ---- */
.invoice-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.invoice-detail-body {
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
.field-grid { display: grid; grid-template-columns: 1fr 1fr; column-gap: 20px; row-gap: 12px; }
.field-grid--full { grid-template-columns: 1fr; }
.field-grid :deep(.t-form__item) { margin-bottom: 0; }

.items-section { border-top: 1px solid var(--td-component-stroke); padding-top: 14px; margin-top: 4px;
  .items-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px;
    .items-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); } }
}
.items-row { display: grid; grid-template-columns: 2fr 1fr 1fr 1fr 44px; gap: 8px; align-items: center; margin-bottom: 8px;
  &--head { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
}
.item-cell { min-width: 0; }
.item-op { display: flex; justify-content: center; }
.items-empty { font-size: 13px; color: var(--td-text-color-placeholder); padding: 12px 0; }
.detail-save-hint { display: flex; align-items: center; gap: 6px; margin-top: 16px; font-size: 12px; color: var(--td-text-color-placeholder); }

/* 抽屉 resize 手柄（复刻原项目） */
.doc-drawer-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2001;
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

/* ---- 打印预览（合并单个 PDF，单 iframe 全高预览；白色背景、页脚固定） ---- */
.invoice-print-dialog :deep(.t-dialog) {
  display: flex;
  flex-direction: column;
  height: 82vh !important;
  max-height: 92vh !important;
  background: #fff;
}
.invoice-print-dialog :deep(.t-dialog__wrap) {
  align-items: center;
}
.invoice-print-dialog :deep(.t-dialog__header) { color: var(--td-text-color-primary); }
.invoice-print-dialog :deep(.t-dialog__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 8px 24px 0;
  background: #fff;
}
.invoice-print-dialog :deep(.t-dialog__footer) {
  padding: 12px 24px;
  background: #fff;
  border-top: 1px solid var(--td-component-stroke);
}
.print-preview-area {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  .print-preview-frame {
    width: 100%;
    flex: 1;
    min-height: 0;
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    background: #fff;
  }
  .print-preview-loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
    font-size: 14px;
  }
}
.print-preview-footer { display: flex; justify-content: center; gap: 8px; }
</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.invoice-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.invoice-print-dialog {
  width: min(900px, calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  height: 82vh;
  max-height: 92vh;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.invoice-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: #fff;
}
.invoice-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.invoice-print-close {
  flex-shrink: 0;
}
.invoice-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: #fff;
}
.invoice-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.invoice-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.invoice-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: #fff;
  border-top: 1px solid var(--td-component-stroke);
}
</style>
