<template>
  <div class="solar-management-container">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>光伏账单</h2>
        <p class="header-subtitle">光伏发电账单自动解析归档，与电费共用「日常事务-电费」知识库</p>
      </div>
      <div class="header-actions">
        <t-button v-if="kbId" theme="primary" @click="triggerUpload">
          <template #icon><t-icon name="upload" /></template>
          上传光伏账单
        </t-button>
      </div>
      <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
        @change="onFileInputChange" />
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="loading-area">
      <t-loading size="large" text="正在初始化光伏管理..." />
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
        <!-- 筛选工具栏 -->
        <div class="doc-filter-bar">
          <div class="doc-filter-bar__leading">
            <div class="doc-filter-field doc-filter-field--wide">
              <t-date-picker v-model="monthFilter" mode="month" format="YYYY-MM" value-type="YYYY-MM"
                placeholder="账单月份" class="doc-date-range doc-filter-field__control" clearable allow-input
                @change="applyFilter">
                <template #prefixIcon><t-icon name="time" size="16px" /></template>
              </t-date-picker>
            </div>
            <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
              overlay-inner-class="contract-field-popup">
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
                  <t-checkbox-group v-model="visibleColKeys" class="field-popup-list">
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
            <t-tooltip content="设置" placement="bottom">
              <t-button variant="outline" size="small" @click="settingsVisible = true">
                <template #icon><t-icon name="setting" size="14px" /></template>
              </t-button>
            </t-tooltip>
            <t-tooltip content="删除历史" placement="bottom">
              <t-button variant="outline" size="small" @click="historyVisible = true">
                <template #icon><t-icon name="history" size="14px" /></template>
              </t-button>
            </t-tooltip>
          </div>
        </div>

        <!-- 列表 -->
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
              <div class="cell cell-extractStatus" role="columnheader">状态</div>
              <div class="cell cell-tags" role="columnheader">标签</div>
            </div>
            <div class="doc-list-body">
              <div v-for="row in displayRows" :key="row.rowKey" class="doc-list-row" :style="gridStyle"
                :class="{ selected: selectedRowKeys.includes(row.rowKey), 'is-pending': row.kind === 'pending' }" role="row"
                @click="handleRowClick(row)">
                <div class="cell cell-check" @click.stop>
                  <t-checkbox class="doc-list-check" size="small" :checked="selectedRowKeys.includes(row.rowKey)"
                    :disabled="row.kind === 'pending' && !['failed', 'parse_failed'].includes(row.extractStatus)" @change="(c: boolean) => toggleRow(row.rowKey, c)" />
                </div>
                <template v-for="col in visibleColDefs" :key="col.key">
                  <div class="cell" :class="`cell-${col.key}`">
                    <span v-if="row.kind === 'pending' && col.key === visibleColDefs[0]?.key" class="row-text" :title="row.fileName">{{ row.fileName }}</span>
                    <span v-else-if="col.fieldType === 'number' || col.fieldType === 'amount'" class="row-mono" :title="cellText(row, col.key)">
                      {{ cellText(row, col.key) }}
                    </span>
                    <span v-else class="row-text" :title="cellText(row, col.key)">{{ cellText(row, col.key) }}</span>
                  </div>
                </template>
                <div class="cell cell-extractStatus">
                  <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
                    variant="light-outline" class="row-status-tag">
                    <template v-if="statusOf(row).icon" #icon>
                      <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
                    </template>
                    {{ statusOf(row).label }}
                  </t-tag>
                </div>
                <div class="cell cell-tags" @click.stop>
                  <t-tooltip v-if="rowTags(row).length" :content="rowTags(row).map((t: any) => t.name).join('、')"
                    placement="top">
                    <div class="row-tag-chips is-clickable" @click="openTagEdit(row)">
                      <t-tag v-if="rowTags(row).length" size="small" variant="light-outline" class="row-tag">
                        {{ rowTags(row)[0].name }}
                      </t-tag>
                      <span v-if="rowTags(row).length > 1" class="row-tag-more">+{{ rowTags(row).length - 1 }}</span>
                    </div>
                  </t-tooltip>
                  <span v-else class="row-tag-chips is-clickable" @click="row.kind !== 'pending' && openTagEdit(row)">
                    <span class="row-tag-add">+ 标签</span>
                  </span>
                </div>
              </div>
              <div v-if="listLoading" class="list-loading">
                <t-loading size="small" text="加载中..." />
              </div>
              <div v-else-if="!rows.length && !pendingFiles.length" class="list-empty">
                <t-empty description="暂无数据" />
              </div>
            </div>
          </div>
        </div>
        <!-- 底部汇总 -->
        <div v-if="summary.total" class="doc-list-footer-summary" :class="{ 'with-toolbar': selectedRowKeys.length }">
          <span>共 {{ selectedRowKeys.length ? selectedRowKeys.length : summary.total }} 条</span>
          <span>发电量 {{ fmtKwh(summaryUsage) }} 千瓦时</span>
          <span>结算金额 {{ fmtMoney(summaryAmount) }} 元</span>
          <span v-if="selectedRowKeys.length" class="summary-selected">已选 {{ selectedRowKeys.length }} 条</span>
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
                  <div v-for="(it, i) in gridFeeRows" :key="i" class="fg-row" @click="openFeeEdit('grid', i)">
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
                  <div v-for="(it, i) in subsidyFeeRows" :key="i" class="fg-row" @click="openFeeEdit('subsidy', i)">
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

    <!-- 详情抽屉 -->
    <div v-if="detailVisible" class="us-resize-handle" :style="{ right: `${detailWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDetailResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer v-if="detailVisible" :visible="true" :header="detailTitle" :size="`${detailWidth}px`" :footer="false"
      :close-on-overlay-click="true" @close="closeDetail"
      @update:visible="(v: boolean) => (v || closeDetail())">
      <div class="utility-detail-drawer">
        <!-- 字段编辑 -->
        <section class="detail-block">
          <div class="detail-block-title">账单字段</div>
          <div class="detail-block-content">
            <div class="field-grid">
              <div v-for="f in detailFields" :key="f.key" class="field-grid-item" :class="{ 'field-grid-item--full': f.key === 'remark' }">
                <label class="field-label">{{ f.label }}</label>
                <t-textarea v-if="f.key === 'remark'" :model-value="String(detailForm[f.key] ?? '')"
                  :autosize="{ minRows: 3, maxRows: 6 }"
                  @update:model-value="(v: string) => { detailForm[f.key] = v }" />
                <t-input v-else :model-value="String(detailForm[f.key] ?? '')" :type="f.type === 'number' ? 'number' : 'text'" size="small"
                  @update:model-value="(v: string) => { detailForm[f.key] = f.type === 'number' ? Number(v) || 0 : v }" />
              </div>
            </div>
            <div class="edit-drawer-footer">
              <t-button theme="primary" size="small" @click="saveDetailFields">保存</t-button>
            </div>
          </div>
        </section>

        <!-- 源文件预览 -->
        <section class="detail-block">
          <div class="detail-block-title">源文件预览</div>
          <div class="detail-block-content">
            <DocumentPreview v-if="currentRow" :knowledge-id="currentRow.knowledgeId"
              :file-type="currentRow?.fileType || ''" :file-name="currentRow?.fileName || ''" :active="true" />
          </div>
        </section>
      </div>
    </t-drawer>

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

    <!-- 设置 -->
    <SolarSettingsDrawer :visible="settingsVisible" :kb-id="kbId || ''"
      @update:visible="settingsVisible = $event" @changed="onSettingsChanged" />

    <!-- 删除历史 -->
    <DeletedKnowledgeDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" module-name="能耗" />

    <!-- 打印预览弹窗 -->
    <teleport to="body">
      <div v-if="printVisible" class="utility-print-mask">
        <div class="utility-print-dialog">
          <div class="utility-print-header">
            <span class="utility-print-title">打印预览（{{ printCount }} 份）</span>
            <t-button variant="text" size="small" class="utility-print-close" @click="closePrint">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="utility-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" text="正在加载账单 PDF…" />
            </div>
          </div>
          <div class="utility-print-footer">
            <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
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
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  uploadKnowledgeFile,
  getKnowledgeDetails,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractSolarBill,
  reparseKnowledge,
  listSolarBillRecords,
  previewKnowledgeFile,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import UtilitiesKbWizard from './UtilitiesKbWizard.vue'
import SolarSettingsDrawer from './SolarSettingsDrawer.vue'
import DeletedKnowledgeDrawer from './DeletedKnowledgeDrawer.vue'

const KB_NAME = '日常事务-电费'
const PAGE_SIZE = 30
const COLUMN_STORAGE_KEY = 'weknora-solar-columns-v1'
const COLUMN_STORAGE_VERSION = 1

interface ColDef { key: string; label: string; fieldType: string; default: boolean; w: string }
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
const settingsVisible = ref(false)
const historyVisible = ref(false)
const fileInputRef = ref<HTMLInputElement>()

// 字段配置
const columnDefs = ref<ColDef[]>(FALLBACK_COLUMNS)
const visibleColKeys = ref<string[]>(loadStoredColumns())
const fieldPopupVisible = ref(false)
const visibleColDefs = computed(() => columnDefs.value.filter(c => visibleColKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')} 72px 88px`,
}))

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
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const taggingInFlight = ref<Set<string>>(new Set())
const listScrollRef = ref<HTMLElement>()

const summaryUsage = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.generation_kwh) || 0), 0)
})
const summaryAmount = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.settlement_amount) || 0), 0)
})
const selectedRows = computed(() => displayRows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)))
const selectedSingle = computed(() => (selectedRows.value.length === 1 ? selectedRows.value[0] : null))
const clearSelection = () => { selectedRowKeys.value = [] }
const selectableRows = computed(() => displayRows.value.filter(r => r.kind !== 'pending' || ['failed', 'parse_failed'].includes(r.extractStatus)))
const isAllSelected = computed(() => selectableRows.value.length > 0 && selectableRows.value.every(r => selectedRowKeys.value.includes(r.rowKey)))
const someSelected = computed(() => selectedRowKeys.value.length > 0 && !isAllSelected.value)

function toggleSelectAll(checked: boolean) {
  selectedRowKeys.value = checked ? selectableRows.value.map(r => r.rowKey) : []
}
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

const mapRow = (r: any): Row => ({
  rowKey: r.row_key || `${r.knowledge_id}-0`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: r.extract_status === 'manual' ? 'manual' : 'bill',
  item: r.item || {},
  tags: r.tags || [],
})

const pendingRows = computed(() => pendingFiles.value.map((pf: any) => ({
  rowKey: `pending-${pf.id}`,
  knowledgeId: pf.id,
  fileName: pf.file_name || pf.title || '',
  extractStatus: pendingLabel(pf),
  extractError: pf.custom_metadata?.extract_error || '',
  kind: 'pending',
  item: {},
  tags: [],
})))
const displayRows = computed(() => [...pendingRows.value, ...rows.value])

const meterReading = (row: Row) => {
  const g = row.item?.gateways?.[0]
  return g?.readings?.[0] || null
}
const cellText = (row: Row, key: string): string => {
  if (row.kind === 'pending') return ''
  if (key === 'bill_period') return metricPeriodOf(row.item)
  if (key === 'meter_prev') { const r = meterReading(row); return r?.prev !== undefined && r?.prev !== null && r?.prev !== '' ? fmtKwh(Number(r.prev)) : '' }
  if (key === 'meter_curr') { const r = meterReading(row); return r?.curr !== undefined && r?.curr !== null && r?.curr !== '' ? fmtKwh(Number(r.curr)) : '' }
  if (key === 'meter_ratio') return displayMultiplier(meterReading(row))
  if (key === 'meter_reading') { const r = meterReading(row); return r?.reading_kwh !== undefined && r?.reading_kwh !== null && r?.reading_kwh !== '' ? fmtKwh(Number(r.reading_kwh)) : '' }
  if (key === 'meter_bill') { const r = meterReading(row); return r?.bill_kwh !== undefined && r?.bill_kwh !== null && r?.bill_kwh !== '' ? fmtKwh(Number(r.bill_kwh)) : '' }
  const v = row.item?.[key]
  if (v === undefined || v === null || v === '') return ''
  const col = columnDefs.value.find(c => c.key === key)
  if (col?.fieldType === 'number') return fmtKwh(Number(v))
  if (col?.fieldType === 'amount') return fmtMoney(Number(v))
  return String(v)
}
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
    const res: any = await listSolarBillRecords(kbId.value, {
      date_from: monthFilter.value ? `${monthFilter.value}-01` : undefined,
      date_to: monthFilter.value ? `${monthFilter.value}-31` : undefined,
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
const startPolling = () => {
  stopPolling()
  pollTimer = setInterval(async () => {
    if (!kbId.value) return
    try {
      const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
      const data = res?.data || res?.list || []
      const arr = Array.isArray(data) ? data : []
      // 光伏相关文件：仅处理打标 bill_kind=solar 或已提取 kind=solar_bill 的文件，
      // 电费文件（bill_kind=electricity / kind=utility_bill / 无标记）一律跳过
      pendingFiles.value = arr.filter((it: any) => {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        if (meta.kind === 'utility_bill') return false
        if (meta.bill_kind && meta.bill_kind !== 'solar') return false
        return ps === 'parsing' || ps === 'pending' || ps === 'failed' ||
          meta.extract_status === 'failed' ||
          meta.extract_status === 'extracting' || it.extract_status === 'extracting' ||
          (meta.extract_status == null && ps === 'completed' && meta.bill_kind === 'solar')
      })
      // 已解析完成且已打标 solar 但尚未提取的文件自动触发提取；
      // 文件名含「光伏」但未打标未提取的自动补打标后提取（自愈，避免上传打标失败导致列表不可见）
      for (const it of arr) {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        if (meta.kind === 'utility_bill') continue
        if (meta.bill_kind && meta.bill_kind !== 'solar') continue
        const isSolarName = /光伏/.test(it.file_name || '')
        if (ps === 'completed' && isSolarName && !meta.kind && !meta.bill_kind && !meta.extract_status && !taggingInFlight.value.has(it.id)) {
          taggingInFlight.value.add(it.id)
          try {
            await updateKnowledgeMetadata(it.id, { custom_metadata: { bill_kind: 'solar' } })
          } catch { /* 打标失败下轮重试 */ } finally {
            taggingInFlight.value.delete(it.id)
          }
        }
        if (ps === 'completed' && meta.bill_kind === 'solar' && !meta.kind && !meta.extract_status && !extractInFlight.value.has(it.id)) {
          extractInFlight.value.add(it.id)
          try {
            await extractSolarBill(kbId.value, it.id)
            loadFiles(true)
          } catch { /* 失败下轮重试 */ } finally {
            extractInFlight.value.delete(it.id)
          }
        }
      }
    } catch { /* 轮询失败静默 */ }
  }, 3000)
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
const triggerUpload = () => fileInputRef.value?.click()
const onFileInputChange = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  if (!files.length || !kbId.value) return
  for (const f of files) {
    try {
      // 上传后立即打标 bill_kind=solar（custom_metadata，列表接口可靠返回），
      // 与电费文件在共享知识库中互不干扰
      const res: any = await uploadKnowledgeFile(kbId.value, { file: f })
      const kid = res?.data?.id || res?.id
      if (kid) {
        try {
          const kd: any = await getKnowledgeDetails(kid)
          let meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
          // 兼容历史双层结构：{custom_metadata: {...}} 取内层再打标
          if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
          await updateKnowledgeMetadata(kid, { custom_metadata: { ...meta, bill_kind: 'solar' } })
        } catch { /* 打标失败则文件保持未标记，轮询会自动补打标并提取 */ console.warn('[solar] 上传后打标失败，交由轮询自愈', f.name) }
      }
      // 解析完成后由轮询自动触发提取
    } catch (e2: any) {
      const msg = e2?.message || ''
      if (msg.includes('already exists') || msg.includes('文件重复')) {
        // 重复上传：若知识库中已有同名文件但尚未成为光伏记录（未打标/未提取），
        // 自动补打标 bill_kind=solar，由轮询自动重新提取，避免文件"存在但列表不可见"
        const dup = e2?.response?.data?.data
        const dupId = dup?.id || dup?.knowledge_id
        let recovered = false
        if (dupId) {
          try {
            const kd: any = await getKnowledgeDetails(dupId)
            let meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
            if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
            const kind = meta?.kind || ''
            if (kind !== 'solar_bill') {
              await updateKnowledgeMetadata(dupId, { custom_metadata: { ...meta, bill_kind: 'solar' } })
              recovered = true
            }
          } catch { /* 恢复失败则仅提示已存在 */ }
        }
        if (recovered) {
          MessagePlugin.warning(`${f.name} 已存在，已自动补齐并重新提取`)
          loadFiles(true)
        } else {
          MessagePlugin.warning(`${f.name} 已存在，忽略重复上传`)
        }
      } else {
        MessagePlugin.error(`上传「${f.name}」失败：${msg}`)
      }
    }
  }
  await loadFiles(true)
}

// ---- 详情 ----
const detailVisible = ref(false)
const detailMode = ref(false)
const currentRow = ref<Row | null>(null)
const activeMenu = ref<'overview' | 'grid-fee' | 'subsidy-fee' | 'meter'>('overview')
const detailForm = ref<Record<string, any>>({})

const DETAIL_WIDTH_KEY = 'weknora-solar-detail-width'
const detailWidth = ref(Number(localStorage.getItem(DETAIL_WIDTH_KEY)) || 560)
const onDetailResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  const startX = e.clientX
  const startW = detailWidth.value
  const onMove = (ev: MouseEvent) => {
    const w = Math.min(960, Math.max(480, startW - (ev.clientX - startX)))
    detailWidth.value = w
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    try { localStorage.setItem(DETAIL_WIDTH_KEY, String(detailWidth.value)) } catch { /* ignore */ }
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

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

const openDetail = async (row: Row) => {
  currentRow.value = row
  detailForm.value = { ...(row.item || {}) }
  detailVisible.value = true
  try {
    const pv: any = await previewKnowledgeFile(row.knowledgeId)
    printUrl.value = pv?.data?.url || pv?.url || ''
  } catch { printUrl.value = '' }
}
const closeDetail = () => {
  detailVisible.value = false
}
// 字段失焦自动保存（与合同/发票一致；数字字段清洗后仅更新该字段）
const saveDetailField = async (f: any) => {
  if (!currentRow.value || !kbId.value) return
  const key = f.key
  if (f.type === 'number') {
    const n = Number(detailForm.value[key])
    detailForm.value[key] = Number.isFinite(n) ? n : 0
  }
  try {
    const kid = currentRow.value.knowledgeId
    const kd: any = await getKnowledgeDetails(kid)
    let meta = kd?.data?.custom_metadata || kd?.custom_metadata || {}
    if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
    meta.kind = 'solar_bill'
    meta.records = meta.records || []
    if (meta.records.length) {
      meta.records[0] = { ...(meta.records[0] || {}), [key]: detailForm.value[key] }
    } else {
      meta.records = [{ ...detailForm.value, gateways: currentRow.value.item?.gateways || [] }]
    }
    await updateKnowledgeMetadata(kid, { custom_metadata: meta })
    currentRow.value.item = { ...(currentRow.value.item || {}), ...detailForm.value }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
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
    if (meta.records.length) {
      meta.records[0] = { ...(meta.records[0] || {}), ...detailForm.value }
    } else {
      meta.records = [{ ...detailForm.value, gateways: currentRow.value.item?.gateways || [] }]
    }
    await updateKnowledgeMetadata(kid, { custom_metadata: meta })
    // 本地行同步
    currentRow.value.item = { ...(currentRow.value.item || {}), ...detailForm.value }
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
  if (!currentRow.value || !kbId.value || !feeEditTarget.value) return
  const { group, index } = feeEditTarget.value
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
    await updateKnowledgeMetadata(kid, { custom_metadata: meta })
    currentRow.value.item = { ...currentRow.value.item, gateways: [...(it.value.gateways || [])] }
    MessagePlugin.success('已保存')
    feeEditVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

// ---- 浮动工具栏 ----
const handleReExtract = async () => {
  const row = selectedSingle.value
  if (!row || !kbId.value) return
  try {
    // 与电费/合同一致：重新解析并提取（解析完成后由轮询自动触发字段提取）
    await reparseKnowledge(row.knowledgeId)
    MessagePlugin.success(`已触发「${row.fileName}」重新解析与提取`)
    setTimeout(() => loadFiles(true), 1500)
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
    await updateKnowledgeTagBatch(kbId.value, { knowledge_ids: [tagTargetRow.knowledgeId], tag_ids: tags.map((t: any) => t.id || t.tag_id || t) })
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
  const ext = String(r.fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const printVisible = ref(false)
const printUrl = ref('')
const printCount = ref(1)
const printLoaded = ref(false)
const printBusy = ref(false)
const closePrint = () => {
  printVisible.value = false
  printUrl.value = ''
}
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement
  if (!frame) return
  printBusy.value = true
  try {
    frame.contentWindow?.focus()
    frame.contentWindow?.print()
  } catch {
    try {
      const w = window.open('', '_blank')
      w?.document.write(`<iframe src="${printUrl.value}" style="width:100%;height:100%"></iframe>`)
      w?.print()
    } catch (e: any) {
      MessagePlugin.error(e?.message || '打印失败')
    }
  } finally {
    printBusy.value = false
  }
}
const handleBatchPrint = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  printBusy.value = true
  printUrl.value = ''
  printLoaded.value = false
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
          const ext = String(r.fileType || '').toLowerCase()
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
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    if (notes.length) setTimeout(() => MessagePlugin.warning(notes.join('；')), 300)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
    printVisible.value = false
  } finally {
    printBusy.value = false
  }
}

// ---- 设置 ----
const onSettingsChanged = () => {
  loadFiles(true)
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

onMounted(() => { loadKb() })
onBeforeUnmount(() => { stopPolling() })
</script>

<style lang="less" scoped>
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
}
.doc-filter-field { display: flex; align-items: center; }
.doc-filter-field--search { flex: 1; min-width: 200px; max-width: 320px; }
.doc-filter-field--wide { width: 160px; }
.doc-filter-field__control { width: 100%; }
.field-popup-content { min-width: 200px; padding: 8px; }
.field-popup-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.field-popup-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
.field-popup-actions { display: flex; gap: 4px; }
.field-popup-list { display: flex; flex-direction: column; gap: 6px; max-height: 320px; overflow-y: auto; }
.field-popup-item { margin: 0; }

/* 列表 */
.doc-list-scroll {
  flex: 0 1 auto;
  max-height: 100%;
  min-width: 0;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
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
.row-tag-chips { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; }
.row-tag { max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
.row-tag-more { font-size: 12px; color: var(--td-text-color-secondary); }
.row-tag-add { font-size: 12px; color: var(--td-brand-color); cursor: pointer; }
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

/* 打印 */
.utility-print-mask {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.utility-print-dialog {
  display: flex;
  flex-direction: column;
  width: min(920px, 92vw);
  height: min(820px, 90vh);
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
}
.utility-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
}
.utility-print-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.utility-print-body { flex: 1; min-height: 0; display: flex; }
.print-preview-frame { width: 100%; height: 100%; border: none; background: #fff; }
.print-preview-loading { display: flex; align-items: center; justify-content: center; width: 100%; }
.utility-print-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px 16px;
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
}
</style>
