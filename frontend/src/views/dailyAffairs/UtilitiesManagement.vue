<template>
  <div class="utilities-management-container">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>能耗管理</h2>
        <p class="header-subtitle">电费账单自动解析归档；水费、气费按月录入多表计自动汇总</p>
      </div>
      <div class="header-actions" v-if="activeTab === 'electricity'">
        <t-button v-if="kbId" theme="primary" @click="triggerUpload">
          <template #icon><t-icon name="upload" /></template>
          上传电费账单
        </t-button>
      </div>
      <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
        @change="onFileInputChange" />
    </div>

    <!-- 加载中 -->
    <div v-if="loading && activeTab === 'electricity'" class="loading-area">
      <t-loading size="large" text="正在初始化电费管理..." />
    </div>

    <!-- KB 不存在：空状态（仅电费 Tab 需要知识库） -->
    <div v-else-if="activeTab === 'electricity' && !kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-电费」知识库">
        <template #image><t-icon name="dashboard" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建电费知识库</t-button>
    </div>

    <!-- 主界面 -->
    <div v-else class="utilities-main">
      <div class="utilities-layout">
        <!-- 侧边栏菜单 -->
        <div class="utilities-sidebar">
          <div class="utilities-side-item" :class="{ active: activeTab === 'electricity' }" @click="switchTab('electricity')">
            <t-icon name="chart-bubble" size="16px" /><span>电费</span>
          </div>
          <div class="utilities-side-item" :class="{ active: activeTab === 'water' }" @click="switchTab('water')">
            <t-icon name="dashboard" size="16px" /><span>水费</span>
          </div>
          <div class="utilities-side-item" :class="{ active: activeTab === 'gas' }" @click="switchTab('gas')">
            <t-icon name="windy" size="16px" /><span>气费</span>
          </div>
        </div>
        <!-- 内容区 -->
        <div class="utilities-content">
          <template v-if="activeTab === 'electricity'">
          <div v-if="kbId" class="electricity-panel">
            <!-- ================= 列表视图 ================= -->
            <template v-if="!detailMode">
            <!-- 筛选工具栏 -->
            <div class="doc-filter-bar">
              <div class="doc-filter-bar__leading">
                <div class="doc-filter-field doc-filter-field--search">
                  <t-input v-model="keyword" placeholder="搜索全部字段" clearable class="doc-search doc-filter-field__control"
                    @enter="applyFilter" @clear="applyFilter">
                    <template #prefixIcon><t-icon name="search" size="16px" /></template>
                  </t-input>
                </div>
                <div class="doc-filter-field doc-filter-field--wide">
                  <t-date-range-picker v-model="dateRange" placeholder="账单周期" class="doc-date-range doc-filter-field__control"
                    clearable allow-input @change="applyFilter">
                    <template #prefixIcon><t-icon name="time" size="16px" /></template>
                  </t-date-range-picker>
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
                        <!-- 进行中/失败行：文件名显示在第一列，其余列留空，保证与表头对齐（与发票模块一致） -->
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
                <!-- 底部汇总（表格底部左侧；选中时按选中统计并避让浮动工具栏） -->
                <div v-if="summary.total" class="doc-list-footer-summary" :class="{ 'with-toolbar': selectedRowKeys.length }">
                  <span>共 {{ selectedRowKeys.length ? selectedRowKeys.length : summary.total }} 条</span>
                  <span>电量 {{ fmtKwh(summaryUsage) }} 千瓦时</span>
                  <span>电费 {{ fmtMoney(summaryAmount) }} 元</span>
                  <span v-if="selectedRowKeys.length" class="summary-selected">已选 {{ selectedRowKeys.length }} 条</span>
                </div>
              </div>
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

            <!-- ================= 详情视图（分层菜单 + 内容区） ================= -->
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
                  <div v-for="m in BILL_MENUS" :key="m.key" class="bill-nav-group">
                    <template v-if="m.group">
                      <div class="bill-nav-group-title">{{ m.label }}</div>
                      <div v-for="child in m.children" :key="child.key" class="bill-nav-item bill-nav-item--child"
                        :class="{ active: activeMenu === child.key }" @click="activeMenu = child.key">
                        {{ child.label }}
                      </div>
                    </template>
                    <div v-else class="bill-nav-item" :class="{ active: activeMenu === m.key }" @click="activeMenu = m.key">
                      {{ m.label }}
                    </div>
                  </div>
                </aside>
                <!-- 右侧内容区 -->
                <div class="bill-content">
                  <!-- 账单概况（概览页：本期电量 / 本期电费 / 缴费截止 / 账单概况 / 用能分析） -->
                  <template v-if="activeMenu === 'overview'">
                    <!-- 四大数据卡片 -->
                    <div class="overview-metrics">
                      <div class="metric-card" :title="metricKwhTip">
                        <div class="metric-label">本期电量</div>
                        <div class="metric-value">{{ fmtKwh(metricKwh) }}<span class="metric-unit">千瓦时</span></div>
                        <span class="ea-delta" :class="metricKwhDeltaClass" v-if="prevEnergy.exists">
                          <span class="ea-arrow">{{ metricKwhDeltaArrow }}</span><span class="ea-sub">{{ metricKwhDeltaSub }}</span>
                        </span>
                      </div>
                      <div class="metric-card" :title="metricFeeTip">
                        <div class="metric-label">本期电费</div>
                        <div class="metric-value">{{ fmtMoney(metricFee) }}<span class="metric-unit">元</span></div>
                        <span class="ea-delta" :class="metricFeeDeltaClass" v-if="prevEnergy.exists">
                          <span class="ea-arrow">{{ metricFeeDeltaArrow }}</span><span class="ea-sub">{{ metricFeeDeltaSub }}</span>
                        </span>
                      </div>
                      <div class="metric-card">
                        <div class="metric-label">账单周期</div>
                        <div class="metric-value metric-value--date">{{ metricPeriod }}</div>
                      </div>
                      <div class="metric-card">
                        <div class="metric-label">缴费截止日期</div>
                        <div class="metric-value metric-value--date">{{ metricDue }}</div>
                      </div>
                    </div>

                    <!-- 6-7 账单概况 / 用能分析（一行两卡片） -->
                    <div class="overview-two-col">

                    <!-- 5 账单概况 -->
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">账单概况</span>
                        <span class="bill-card-hint">点击行查看明细</span>
                      </div>
                      <div class="os-table">
                          <div class="os-row os-row--head">
                            <span>费用组成</span><span class="os-qty">计费数量</span><span class="os-amount">电费</span><span class="os-amount">账单电费</span><span class="os-state">对比状态</span>
                          </div>
                          <div class="os-row" :class="{ 'os-row--link': r.menuKey, 'os-row--total': r.total }"
                            v-for="r in overviewRows" :key="r.key" @click="gotoMenu(r.menuKey)">
                            <span class="os-name">{{ r.label }}</span>
                            <span class="os-qty">{{ qtyLabelOf(r) }}</span>
                            <span class="os-amount" :class="{ 'os-neg': r.displayValue < 0 }" @click.stop="feeEditingKey !== r.key && startFeeEdit(r)">
                              <template v-if="feeEditingKey === r.key">
                                <t-input v-model="feeEditValue" size="small" class="os-fee-input" @click.stop
                                  @blur="commitFeeOverride(r)" @enter="commitFeeOverride(r)" />
                              </template>
                              <template v-else>
                                <t-tooltip v-if="hasFeeOverride(r.key)" content="手动修改，重提取后重置" placement="top">
                                  <span class="os-fee-val os-fee-val--manual">{{ fmtRate6(r.displayValue) }}</span>
                                </t-tooltip>
                                <span v-else class="os-fee-val">{{ fmtRate6(r.displayValue) }}</span>
                              </template>
                            </span>
                            <span class="os-amount">{{ r.billFee ? fmtRate6(r.billFee) : '' }}</span>
                            <span class="os-state">
                              <t-tag v-if="r.billFee" :theme="feeClose(r.displayValue, r.billFee) ? 'success' : 'danger'" variant="light" size="small">
                                {{ feeClose(r.displayValue, r.billFee) ? '正常' : '异常' }}
                              </t-tag>
                            </span>
                          </div>
                          <div class="os-row os-row--total">
                            <span>本期电费</span>
                            <span class="os-qty">—</span>
                            <span class="os-amount" :class="{ 'os-neg': overviewTotal < 0 }">{{ fmtRate6(overviewTotal) }}</span>
                            <span class="os-amount">{{ billTotalText }}</span>
                            <span class="os-state">
                              <t-tooltip v-if="!billTotalOk" content="差值 {{ fmtRate6(overviewDiff) }}：子项计算含容需量/力调，提取值若为旧口径则不含，重提取后一致" placement="top">
                                <t-tag theme="danger" variant="light" size="small">异常</t-tag>
                              </t-tooltip>
                              <t-tag v-else theme="success" variant="light" size="small">正常</t-tag>
                            </span>
                          </div>
                        </div>
                    </div>

                    <!-- 6 用能分析 -->
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">用能分析</span>
                        <span v-if="!prevEnergy.exists" class="bill-card-hint">暂无上期账单数据</span>
                      </div>
                      <div class="ea-grid">
                          <div class="ea-item" :title="momTip">
                            <span class="ea-label">本期电量环比</span>
                            <strong class="ea-value">{{ momText }}</strong>
                            <span class="ea-delta" :class="momDeltaClass" v-if="prevEnergy.exists">
                              <span class="ea-arrow">{{ momDeltaArrow }}</span><span class="ea-sub">{{ momDeltaSub }}</span>
                            </span>
                          </div>
                          <div class="ea-item" :title="pfTip">
                            <span class="ea-label">功率因数</span>
                            <strong class="ea-value">{{ pfText }}</strong>
                            <span class="ea-delta" :class="pfDeltaClass" v-if="prevEnergy.exists">
                              <span class="ea-arrow">{{ pfDeltaArrow }}</span><span class="ea-sub">{{ pfDeltaSub }}</span>
                            </span>
                          </div>
                          <div class="ea-item" :title="avgTip">
                            <span class="ea-label">平均电价</span>
                            <strong class="ea-value">{{ avgPriceText }}</strong>
                            <span class="ea-delta" :class="avgDeltaClass" v-if="prevEnergy.exists">
                              <span class="ea-arrow">{{ avgDeltaArrow }}</span><span class="ea-sub">{{ avgDeltaSub }}</span>
                            </span>
                          </div>
                        </div>
                        <div class="ea-chart">
                          <div ref="energyChartRef" class="ea-echart"></div>
                        </div>
                        <!-- 分时电量占比对比 -->
                        <div class="ea-compare">
                          <div class="ea-compare-row ea-compare-head">
                            <span>时段</span><span class="ea-c-num">本期电量</span><span class="ea-c-num">本期占比</span>
                            <span class="ea-c-num">上期电量</span><span class="ea-c-num">上期占比</span><span class="ea-c-num">占比变化</span>
                          </div>
                          <div class="ea-compare-row" v-for="r in energyCompareRows" :key="r.key">
                            <span class="ea-c-label">{{ r.label }}</span>
                            <span class="ea-c-num">{{ fmtKwh(r.cur) }}</span>
                            <span class="ea-c-num">{{ r.curPct.toFixed(2) }}%</span>
                            <span class="ea-c-num">{{ prevEnergy.exists ? fmtKwh(r.prev) : '—' }}</span>
                            <span class="ea-c-num">{{ prevEnergy.exists ? r.prevPct.toFixed(2) + '%' : '—' }}</span>
                            <span class="ea-c-num" :class="{ 'ea-c-up': r.delta > 0, 'ea-c-down': r.delta < 0 }">
                              {{ prevEnergy.exists ? (r.delta > 0 ? '+' : '') + r.delta + '%' : '—' }}
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- 工商业电量明细 -->
                  <template v-else-if="activeMenu === 'industrial-meter'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">电量明细 · 工商业</span>
                        <span class="bill-card-hint">点击行可编辑，自动保存</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>示数类型</span><span>上期示数</span><span>本期示数</span><span>倍率</span><span>抄见电量</span><span>变损</span><span>线损</span><span>加减</span><span>计费电量</span>
                        </div>
                        <div v-for="(r, i) in meterRows" :key="r.meter_type || i" class="meter-row" @click="openMeterEdit(i)">
                          <span class="meter-type">{{ r.meter_type }}</span>
                          <span class="row-mono">{{ r.prev || r.prev === 0 ? fmtKwh(r.prev) : '' }}</span>
                          <span class="row-mono">{{ r.curr || r.curr === 0 ? fmtKwh(r.curr) : '' }}</span>
                          <span class="row-mono">{{ displayMultiplier(r) }}</span>
                          <span class="row-mono">{{ r.reading_kwh || r.reading_kwh === 0 ? fmtKwh(r.reading_kwh) : '' }}</span>
                          <span class="row-mono">{{ r.trans_loss || r.trans_loss === 0 ? fmtKwh(r.trans_loss) : '' }}</span>
                          <span class="row-mono">{{ r.line_loss || r.line_loss === 0 ? fmtKwh(r.line_loss) : '' }}</span>
                          <span class="row-mono">{{ r.adjust || r.adjust === 0 ? fmtKwh(r.adjust) : '' }}</span>
                          <span class="row-mono">{{ r.bill_kwh ? fmtKwh(r.bill_kwh) : '' }}</span>
                        </div>
                        <div class="meter-row meter-total">
                          <span>合计</span><span></span><span></span><span></span><span></span><span></span><span></span><span></span>
                          <span class="row-mono">{{ fmtKwh(meterTotal) }}</span>
                        </div>
                      </div>
                    </div>
                  </template>
                  <!-- 居民电量明细 -->
                  <template v-else-if="activeMenu === 'residential-meter'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">电量明细 · 居民</span>
                        <span class="bill-card-hint">计费电量 = 本期电量 × 定比 + 加减，点击行编辑，自动保存</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>示数类型</span><span>上期示数</span><span>本期示数</span><span>倍率</span><span>抄见电量</span><span>变损</span><span>线损</span><span>加减</span><span>计费电量</span>
                        </div>
                        <div v-for="(r, i) in residentMeterRows" :key="i" class="meter-row" @click="openResidentMeterEdit(i)">
                          <span class="meter-type">{{ r.meter_type }}</span>
                          <span class="row-mono">{{ r.prev || r.prev === 0 ? fmtKwh(r.prev) : '' }}</span>
                          <span class="row-mono">{{ r.curr || r.curr === 0 ? fmtKwh(r.curr) : '' }}</span>
                          <span class="row-mono">{{ r.multiplier || r.multiplier === 0 ? fmtKwh(r.multiplier) : '' }}</span>
                          <span class="row-mono">{{ r.reading_kwh || r.reading_kwh === 0 ? fmtKwh(r.reading_kwh) : '' }}</span>
                          <span class="row-mono">{{ r.trans_loss || r.trans_loss === 0 ? fmtKwh(r.trans_loss) : '' }}</span>
                          <span class="row-mono">{{ r.line_loss || r.line_loss === 0 ? fmtKwh(r.line_loss) : '' }}</span>
                          <span class="row-mono">{{ r.adjust || r.adjust === 0 ? fmtKwh(r.adjust) : '' }}</span>
                          <span class="row-mono">{{ r.bill_kwh ? fmtKwh(r.bill_kwh) : '' }}</span>
                        </div>
                        <div v-if="!residentMeterRows.length" class="fg-empty">暂无数据</div>
                        <div v-if="residentMeterRows.length" class="meter-row meter-total">
                          <span>合计</span><span></span><span></span><span></span><span></span><span></span><span></span><span></span>
                          <span class="row-mono">{{ fmtKwh(residentBillTotal) }}</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- 费用子项通用表格 -->
                  <template v-else-if="feeMenuOf(activeMenu)">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">{{ menuLabel(activeMenu) }}</span>
                        <span class="bill-card-hint">共 {{ feeRowsOf(activeMenu).length }} 项，点击行编辑，自动保存</span>
                      </div>
                      <div class="fee-group-table">
                        <div class="fg-row fg-head">
                          <span v-for="c in feeGroupCols(activeMenu)" :key="c.field_key">{{ c.label }}</span>
                        </div>
                        <div v-for="(it, i) in feeRowsOf(activeMenu)" :key="i" class="fg-row" @click="openFeeItemEdit(activeMenu, i)">
                          <span v-for="c in feeGroupCols(activeMenu)" :key="c.field_key"
                            :class="{ 'fg-name': c.field_key === 'name', 'row-mono': c.field_type !== 'text', 'os-neg': c.field_key === 'fee' && Number(it.fee) < 0 }"
                            :title="c.field_key === 'name' ? it.name : ''">
                            {{ feeCellText(it, c) }}
                          </span>
                        </div>
                        <div v-if="!feeRowsOf(activeMenu).length" class="fg-empty">暂无数据</div>
                        <div v-if="feeRowsOf(activeMenu).length" class="fg-row fg-total">
                          <span>小计</span>
                          <span v-for="c in feeGroupCols(activeMenu).slice(1)" :key="c.field_key" />
                          <span class="row-mono" :class="{ 'os-neg': feeSubtotalOf(activeMenu) < 0 }">{{ fmtMoney(feeSubtotalOf(activeMenu)) }}</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- 输配容（需）量电费 -->
                  <template v-else-if="activeMenu === 'capacity'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">输配容（需）量电费</span>
                        <span class="bill-card-hint">点击行编辑，自动保存</span>
                      </div>
                      <div class="detail-table cols-8">
                        <div class="fg-row fg-head">
                          <span>需量值</span><span>需量电价</span><span>输配需量电费</span><span>月每千伏安用电量</span><span>折扣需量电费</span><span>容量</span><span>容量电价</span><span>输配容量电费</span>
                        </div>
                        <div v-for="(r, i) in capacityRows" :key="i" class="fg-row" @click="openDetailEdit('capacity', i)">
                          <span class="row-mono">{{ r.demand || r.demand === 0 ? fmtRate6(r.demand) : '' }}</span>
                          <span class="row-mono">{{ r.demand_price || r.demand_price === 0 ? fmtRate6(r.demand_price) : '' }}</span>
                          <span class="row-mono">{{ r.demand_fee || r.demand_fee === 0 ? fmtRate6(r.demand_fee) : '' }}</span>
                          <span class="row-mono">{{ r.kwh_per_kva || r.kwh_per_kva === 0 ? fmtRate6(r.kwh_per_kva) : '' }}</span>
                          <span class="row-mono">{{ r.discount_demand_fee || r.discount_demand_fee === 0 ? fmtRate6(r.discount_demand_fee) : '' }}</span>
                          <span class="row-mono">{{ r.capacity || r.capacity === 0 ? fmtRate6(r.capacity) : '' }}</span>
                          <span class="row-mono">{{ r.capacity_price || r.capacity_price === 0 ? fmtRate6(r.capacity_price) : '' }}</span>
                          <span class="row-mono">{{ r.capacity_fee || r.capacity_fee === 0 ? fmtRate6(r.capacity_fee) : '' }}</span>
                        </div>
                        <div v-if="!capacityRows.length" class="fg-empty">暂无数据</div>
                      </div>
                    </div>
                  </template>

                  <!-- 功率因素调整电费 -->
                  <template v-else-if="activeMenu === 'pf-adjust'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">功率因素调整电费</span>
                        <span class="bill-card-hint">点击行编辑，自动保存</span>
                      </div>
                      <div class="detail-table cols-8">
                        <div class="fg-row fg-head">
                          <span>项目</span><span>功率因素实际值</span><span>功率因素标准</span><span>调整系数</span><span>参与调整有功电量</span><span>参与调整无功电量</span><span>参与调整电费金额</span><span>功率因素调整电费</span>
                        </div>
                        <div v-for="(r, i) in pfRows" :key="i" class="fg-row" @click="openDetailEdit('pf', i)">
                          <span class="fg-name" :title="r.project">{{ r.project }}</span>
                          <span class="row-mono">{{ r.power_factor || r.power_factor === 0 ? fmtRate6(r.power_factor) : '' }}</span>
                          <span class="row-mono">{{ r.pf_standard || r.pf_standard === 0 ? fmtRate6(r.pf_standard) : '' }}</span>
                          <span class="row-mono">{{ r.adjust_ratio || r.adjust_ratio === 0 ? fmtRate6(r.adjust_ratio) : '' }}</span>
                          <span class="row-mono">{{ r.pf_active_kwh || r.pf_active_kwh === 0 ? fmtKwh(r.pf_active_kwh) : '' }}</span>
                          <span class="row-mono">{{ r.pf_reactive_kwh || r.pf_reactive_kwh === 0 ? fmtKwh(r.pf_reactive_kwh) : '' }}</span>
                          <span class="row-mono">{{ r.pf_fee_base || r.pf_fee_base === 0 ? fmtRate6(r.pf_fee_base) : '' }}</span>
                          <span class="row-mono" :class="{ 'os-neg': r.adjust_fee < 0 }">{{ r.adjust_fee || r.adjust_fee === 0 ? fmtRate6(r.adjust_fee) : '' }}</span>
                        </div>
                        <div v-if="!pfRows.length" class="fg-empty">暂无数据</div>
                      </div>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>
          </template>
          <template v-else-if="activeTab === 'water'">
            <UtilityMeterTab category="water" />
          </template>
          <template v-else>
            <UtilityMeterTab category="gas" />
          </template>
        </div>
      </div>
    </div>

    <!-- 详情抽屉 -->
    <t-drawer v-if="detailVisible" :visible="true" :header="detailTitle" :size="`${drawerWidth}px`" :footer="false"
      :close-on-overlay-click="true" @close="closeDetail"
      @update:visible="(v: boolean) => (v || closeDetail())">
      <div class="utility-detail-drawer">
        <!-- 摘要 -->
        <section class="detail-block">
          <div class="detail-block-title" @click="summaryExpanded = !summaryExpanded">
            <span>摘要</span>
            <t-icon :name="summaryExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="detail-block-caret" />
          </div>
          <div v-if="summaryExpanded" class="detail-block-content">
            <div class="summary-lines">{{ summaryLines || '暂无摘要' }}</div>
          </div>
        </section>

        <!-- 字段编辑 -->
        <section class="detail-block">
          <div class="detail-block-title">账单字段</div>
          <div class="detail-block-content">
            <div class="field-grid">
              <div v-for="cfg in detailFieldDefs" :key="cfg.field_key" class="field-grid-item">
                <label class="field-label">{{ cfg.label }}</label>
                <t-input v-if="cfg.fieldType === 'text'" :model-value="fieldValue(cfg)" size="small"
                  @update:model-value="(v: string) => setFieldValue(cfg, v)" />
                <t-date-picker v-else-if="cfg.fieldType === 'date'" :model-value="fieldValue(cfg)" size="small"
                  format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable
                  @change="(v: any) => setFieldValue(cfg, v)" />
                <t-input v-else :model-value="fieldValue(cfg)" size="small" type="number"
                  @update:model-value="(v: string) => setFieldValue(cfg, v)" />
              </div>
            </div>
            <div class="auto-save-tip">字段修改后将自动保存{{ autoSaving ? '（保存中...）' : '' }}</div>
          </div>
        </section>

        <!-- 费用明细（可编辑，总账联动，可折叠） -->
        <section class="detail-block">
          <div class="detail-block-title" @click="feeExpanded = !feeExpanded">
            <span>费用明细</span>
            <span class="fee-count">{{ editForm.fee_items?.length || 0 }} 项</span>
            <t-icon :name="feeExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="detail-block-caret" />
          </div>
          <div v-if="feeExpanded" class="detail-block-content">
            <div class="fee-table">
              <div class="fee-row fee-head">
                <span>类别</span><span>费用组成</span><span>时段</span><span>电量</span><span>标准</span><span>电费</span><span></span>
              </div>
              <div v-for="(it, i) in editForm.fee_items" :key="i" class="fee-row">
                <t-input v-model="it.category" size="small" placeholder="类别" />
                <t-input v-model="it.name" size="small" placeholder="费用组成" />
                <t-input v-model="it.period" size="small" placeholder="时段" />
                <t-input v-model="it.qty" size="small" type="number" />
                <t-input v-model="it.rate" size="small" type="number" />
                <t-input v-model="it.fee" size="small" type="number" />
                <t-button variant="text" size="small" shape="square" @click="removeFeeItem(i)">
                  <template #icon><t-icon name="delete" size="14px" /></template>
                </t-button>
              </div>
            </div>
            <div class="fee-actions">
              <t-button variant="text" size="small" @click="addFeeItem">
                <template #icon><t-icon name="add" size="14px" /></template>
                添加明细
              </t-button>
              <span class="fee-total">合计 {{ fmtMoney(feeTotal) }} 元</span>
            </div>
            <div class="auto-save-tip">明细编辑后自动保存，总账按明细重算</div>
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

    <!-- 电量明细编辑抽屉 -->
    <t-drawer v-if="meterEditVisible" :visible="true" :header="'编辑电量 · ' + meterEditTitle" :size="'420px'" :footer="false"
      @close="meterEditVisible = false"
      @update:visible="(v: boolean) => (meterEditVisible = v)">
      <div class="edit-drawer-body">
        <div class="edit-field" v-for="f in meterEditFields" :key="f.key">
          <label class="edit-label">{{ f.label }}</label>
          <t-input :model-value="String(meterEditForm[f.key] ?? '')" type="number" size="small"
            @update:model-value="(v: string) => (meterEditForm[f.key] = Number(v) || 0)" />
        </div>
        <div class="auto-save-tip">修改后自动保存</div>
      </div>
    </t-drawer>

    <!-- 明细列表通用编辑抽屉（居民电量/容需量/功率因素） -->
    <t-drawer v-if="detailEditVisible" :visible="true" :header="detailEditTitle" :size="'460px'" :footer="false"
      :close-on-overlay-click="true" @close="detailEditVisible = false"
      @update:visible="(v: boolean) => (detailEditVisible = v)">
      <div class="edit-drawer-body">
        <div class="edit-field" v-for="f in detailEditFields" :key="f.key">
          <label class="edit-label">{{ f.label }}</label>
          <t-input :model-value="String(detailEditForm[f.key] ?? '')" :type="f.type === 'text' ? 'text' : 'number'" size="small"
            @update:model-value="(v: string) => (detailEditForm[f.key] = f.type === 'text' ? v : Number(v) || 0)" />
        </div>
        <div class="auto-save-tip">修改后自动保存</div>
      </div>
    </t-drawer>

    <!-- 费用行编辑抽屉 -->
    <t-drawer v-if="feeItemEditVisible" :visible="true" :header="'编辑费用 · ' + feeItemEditName" :size="'460px'" :footer="false"
      @close="feeItemEditVisible = false"
      @update:visible="(v: boolean) => (feeItemEditVisible = v)">
      <div class="edit-drawer-body">
        <div class="edit-field">
          <label class="edit-label">费用类别</label>
          <t-input :model-value="feeItemEditForm.category ?? ''" size="small" disabled />
        </div>
        <div class="edit-field">
          <label class="edit-label">费用组成</label>
          <t-input :model-value="feeItemEditForm.name ?? ''" size="small" disabled />
        </div>
        <div class="edit-field">
          <label class="edit-label">分时时段</label>
          <t-input :model-value="feeItemEditForm.period ?? ''" size="small" disabled />
        </div>
        <div class="edit-field">
          <label class="edit-label">计费电量（千瓦时）</label>
          <t-input :model-value="String(feeItemEditForm.qty ?? '')" type="number" size="small"
            @update:model-value="(v: string) => { feeItemEditForm.qty = Number(v) || 0; recalcFeeItem() }" />
        </div>
        <div class="edit-field">
          <label class="edit-label">计费标准（元/千瓦时）</label>
          <t-input :model-value="String(feeItemEditForm.rate ?? '')" type="number" size="small"
            @update:model-value="(v: string) => { feeItemEditForm.rate = Number(v) || 0; recalcFeeItem() }" />
        </div>
        <div class="edit-field">
          <label class="edit-label">电费（元）</label>
          <t-input :model-value="String(feeItemEditForm.fee ?? '')" type="number" size="small" disabled
            :placeholder="feeItemEditableFee ? '' : '非电量×标准项，金额保持解析值'" />
        </div>
        <div class="edit-field">
          <label class="edit-label">说明</label>
          <div class="edit-note">{{ feeItemEditableFee ? '电费 = 电量 × 标准，自动计算' : '该费用项无法由电量×标准推导（返还/调整类），金额保留账单原值' }}</div>
        </div>
        <div class="auto-save-tip">修改后自动保存</div>
      </div>
    </t-drawer>

    <!-- 静态字段编辑抽屉 -->
    <t-drawer v-if="staticEditVisible" :visible="true" :header="staticEditTitle" :size="'520px'" :footer="false"
      @close="staticEditVisible = false"
      @update:visible="(v: boolean) => (staticEditVisible = v)">
      <div class="edit-drawer-body">
        <div class="field-grid">
          <div v-for="f in staticEditFields" :key="f.key" class="field-grid-item">
            <label class="field-label">{{ f.label }}</label>
            <t-input v-if="f.type === 'text'" :model-value="String(staticEditForm[f.key] ?? '')" size="small"
              @update:model-value="(v: string) => (staticEditForm[f.key] = v)" />
            <t-date-picker v-else-if="f.type === 'date'" :model-value="String(staticEditForm[f.key] ?? '')" size="small"
              format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable
              @change="(v: any) => (staticEditForm[f.key] = v || '')" />
            <t-input v-else :model-value="String(staticEditForm[f.key] ?? '')" type="number" size="small"
              @update:model-value="(v: string) => (staticEditForm[f.key] = Number(v) || 0)" />
          </div>
        </div>
        <div class="auto-save-tip">修改后自动保存</div>
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
    <UtilitySettingsDrawer :visible="settingsVisible" :kb-id="kbId || ''" category="electricity"
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
              <t-loading size="small" text="正在合并账单 PDF…" />
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
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import * as echarts from 'echarts'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  uploadKnowledgeFile,
  getKnowledgeDetails,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractUtilityBill,
  listUtilityBillRecords,
  listUtilityFieldConfigs,
  previewKnowledgeFile,
  reparseKnowledge,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import UtilitiesKbWizard from './UtilitiesKbWizard.vue'
import UtilityMeterTab from './UtilityMeterTab.vue'
import UtilitySettingsDrawer from './UtilitySettingsDrawer.vue'
import DeletedKnowledgeDrawer from './DeletedKnowledgeDrawer.vue'

const KB_NAME = '日常事务-电费'
const PAGE_SIZE = 30
const COLUMN_STORAGE_KEY = 'weknora-utility-electricity-columns-v5'
const COLUMN_STORAGE_VERSION = 4

interface ColDef { key: string; label: string; fieldType: string; default: boolean; w: string; group?: 'industrial' | 'residential' }
const FALLBACK_COLUMNS: ColDef[] = [
  // 默认字段
  { key: 'bill_period', label: '账单周期', fieldType: 'text', default: true, w: '1.1fr' },
  { key: 'total_kwh', label: '本期电量', fieldType: 'number', default: true, w: '1.15fr' },
  { key: 'total_amount', label: '本期电费', fieldType: 'amount', default: true, w: '1.25fr' },
  { key: 'pf_adjust_amount', label: '力调电费', fieldType: 'amount', default: true, w: '1.2fr' },
  { key: 'capacity_fee', label: '基本电费', fieldType: 'amount', default: true, w: '1.15fr' },
  { key: 'market_amount', label: '购电电费', fieldType: 'amount', default: true, w: '1.2fr' },
  { key: 'line_amount', label: '线损费用', fieldType: 'amount', default: true, w: '1.15fr' },
  { key: 'trans_amount', label: '输配电费', fieldType: 'amount', default: true, w: '1.15fr' },
  { key: 'sys_amount', label: '系统运行费', fieldType: 'amount', default: true, w: '1.2fr' },
  { key: 'govI_amount', label: '附加费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'catalog_amount', label: '目录电费（居民）', fieldType: 'amount', default: true, w: '1.3fr' },
  { key: 'govR_amount', label: '附加费（居民）', fieldType: 'amount', default: true, w: '1.15fr' },
  // 详细字段
  { key: 'account_no', label: '户号', fieldType: 'text', default: false, w: '1.1fr' },
  { key: 'account_name', label: '户名', fieldType: 'text', default: false, w: '1.4fr' },
  { key: 'usage_category', label: '用电类别', fieldType: 'text', default: false, w: '1.2fr' },
  { key: 'voltage_level', label: '电压等级', fieldType: 'text', default: false, w: '1.1fr' },
  { key: 'avg_price', label: '平均电价', fieldType: 'number', default: false, w: '1.1fr' },
  { key: 'power_factor', label: '功率因素', fieldType: 'number', default: false, w: '1fr' },
]

const activeTab = ref<'electricity' | 'water' | 'gas'>('electricity')
const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const settingsVisible = ref(false)
const historyVisible = ref(false)
const fileInputRef = ref<HTMLInputElement>()

// 字段配置（来自后端 /utilities/field-configs?category=electricity，动态加载）
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
      // 带版本校验：旧版本或结构不符时采用新默认集
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

const allFieldConfigs = ref<any[]>([])
// 费用菜单默认列（分组字段配置缺省时兜底）
const DEFAULT_FEE_COLS = [
  { field_key: 'name', label: '费用组成', field_type: 'text' },
  { field_key: 'period', label: '时段', field_type: 'text' },
  { field_key: 'qty', label: '计费电量', field_type: 'number' },
  { field_key: 'rate', label: '计费标准', field_type: 'number' },
  { field_key: 'fee', label: '电费', field_type: 'amount' },
]
// 分组菜单列：按字段配置（group + 默认显示）渲染，设置中增删改后自动同步
const feeGroupCols = (group: string) => {
  const list = allFieldConfigs.value.filter((c: any) => c.group === group && c.deleted_at == null)
  if (!list.length) return DEFAULT_FEE_COLS
  const cols = list.filter((c: any) => c.default_visible).sort((a: any, b: any) => a.sort_order - b.sort_order)
  if (!cols.length) return list.sort((a: any, b: any) => a.sort_order - b.sort_order)
  return cols
}
const feeCellText = (it: any, col: any) => {
  const v = it[col.field_key]
  if (col.field_type === 'number') return v || v === 0 ? fmtRate6(v) : ''
  if (col.field_type === 'amount') return v || v === 0 ? fmtMoney(v) : ''
  if (v === undefined || v === null || v === '') return ''
  return String(v)
}
// 电量明细倍率：仅使用提取值
const displayMultiplier = (r: any) => {
  if (r.multiplier !== undefined && r.multiplier !== null && Number(r.multiplier) !== 0) return fmtRate6(r.multiplier)
  return ''
}

const loadFieldConfigs = async () => {
  try {
    const res: any = await listUtilityFieldConfigs('electricity')
    const list = res?.data || res
    if (Array.isArray(list) && list.length) {
      allFieldConfigs.value = list
      // 仅保留内置默认集字段与用户自定义字段，过滤历史废弃字段
      const validKeys = new Set(FALLBACK_COLUMNS.map(f => f.key))
      const fromServer = list
        .filter((c: any) => validKeys.has(c.field_key) || !!c.is_custom)
        .map((c: any) => {
        const fb = FALLBACK_COLUMNS.find(f => f.key === c.field_key)
        return {
          key: c.field_key,
          // 内置字段以新默认集的标签/默认显隐为准；自定义字段沿用后端配置
          label: fb ? fb.label : c.label,
          fieldType: c.field_type || 'text',
          default: fb ? fb.default : !!c.default_visible,
          w: colWidth(c.field_key),
          group: fb?.group,
        }
      })
      // 后端配置缺内置字段时合并补全，保证新增默认字段可用
      const keys = new Set(fromServer.map((c: any) => c.key))
      const merged = [...fromServer, ...FALLBACK_COLUMNS.filter(f => !keys.has(f.key))]
      // 严格按内置默认集排序（用户要求的列顺序），自定义字段排在最后
      const orderMap = new Map(FALLBACK_COLUMNS.map((f, i) => [f.key, i]))
      merged.sort((a: any, b: any) => {
        const ia = orderMap.has(a.key) ? orderMap.get(a.key)! : FALLBACK_COLUMNS.length
        const ib = orderMap.has(b.key) ? orderMap.get(b.key)! : FALLBACK_COLUMNS.length
        return ia - ib
      })
      columnDefs.value = merged
      // 默认列变更后重算可见列（保留用户已存储的偏好，仅当存储为空时使用新默认）
      visibleColKeys.value = loadStoredColumns()
    }
  } catch { /* 字段配置加载失败用内置默认 */ }
}
function colWidth(key: string): string {
  const fb = FALLBACK_COLUMNS.find(f => f.key === key)
  return fb ? fb.w : '1fr'
}

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
  page?: number
}
const rows = ref<Row[]>([])
const pendingFiles = ref<any[]>([])
const summary = ref<{ total: number; sumKwh: number; sumAmount: number }>({ total: 0, sumKwh: 0, sumAmount: 0 })
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const dateRange = ref<Array<string>>([])
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const extractFailed = ref<Set<string>>(new Set())
const listScrollRef = ref<HTMLElement>()

const summaryUsage = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.total_kwh) || 0), 0)
})
const summaryAmount = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.total_amount) || 0), 0)
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
// 行点击：提取/解析失败行切换选中，其余打开详情
const handleRowClick = (row: Row) => {
  if (row.kind === 'pending') {
    if (['failed', 'parse_failed'].includes(row.extractStatus)) toggleRow(row.rowKey, !selectedRowKeys.value.includes(row.rowKey))
    return
  }
  openDetail(row)
}

const mapRow = (r: any): Row => ({
  rowKey: r.row_key || `${r.knowledge_id}-${r.page || 0}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: r.extract_status === 'manual' ? 'manual' : 'bill',
  item: r.item || {},
  tags: r.tags || [],
  page: r.page,
})

// 进行中文件（解析中/提取中/待提取）合并进列表行，与合同模块一致
const pendingRows = computed(() => pendingFiles.value.map((pf: any) => ({
  rowKey: `pending-${pf.id}`,
  knowledgeId: pf.id,
  fileName: pf.file_name || pf.title || '',
  extractStatus: pendingLabel(pf),
  extractError: pf.custom_metadata?.extract_error || '',
  kind: 'pending',
  item: {},
  tags: [],
  page: 0,
})))
const displayRows = computed(() => [...pendingRows.value, ...rows.value])

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
    const res: any = await listUtilityBillRecords(kbId.value, {
      q: keyword.value || undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
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
    loadAllRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

// 全量电费记录（概览上期对比数据源）
const allRecords = ref<any[]>([])
const loadAllRecords = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listUtilityBillRecords(kbId.value, { page: 1, page_size: 500 })
    allRecords.value = Array.isArray(res?.data || res?.list) ? (res.data || res.list) : []
  } catch {
    allRecords.value = []
  }
}

const onListScroll = () => {
  const el = listScrollRef.value
  if (!el || !hasMore.value || loadingMore.value) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 80) loadFiles()
}

// ---- 状态行（与发票模块一致：绿色 loading 动态样式） ----
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
  // 提取进行中以前端 Set 为准（与发票模块 extractInFlight 一致）
  if (extractingSet.has(pf.id)) return 'extracting'
  if (meta.extract_status === 'failed') return 'failed'
  if (meta.extract_status === 'extracting' || pf.extract_status === 'extracting') return 'extracting'
  return 'pending'
}

// 轮询：解析完成后自动触发提取；提取中动态刷新状态
let pollTimer: ReturnType<typeof setInterval> | null = null
let extractingSet = new Set<string>()
const startPolling = () => {
  stopPolling()
  pollTimer = setInterval(async () => {
    if (!kbId.value) return
    try {
      const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
      const data = res?.data || res?.list || []
      const arr = Array.isArray(data) ? data : []
      // 进行中/失败文件（解析中/提取中/待提取/解析失败/提取失败）
      pendingFiles.value = arr.filter((it: any) => {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        return ps === 'parsing' || ps === 'pending' || ps === 'failed' ||
          meta.extract_status === 'failed' ||
          meta.extract_status === 'extracting' || it.extract_status === 'extracting' ||
          (meta.extract_status == null && ps === 'completed' && !meta.kind)
      })
      // 已解析完成但尚未提取的文件自动触发提取
      for (const it of arr) {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        if (ps === 'completed' && !meta.kind && !meta.extract_status && !extractingSet.has(it.id)) {
          extractingSet.add(it.id)
          try {
            await extractUtilityBill(kbId.value, it.id)
            loadFiles(true)
          } catch { /* 失败下轮重试 */ } finally {
            extractingSet.delete(it.id)
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
      loadDrawerWidth()
      await loadFieldConfigs()
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
    loadDrawerWidth()
    await loadFieldConfigs()
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
      const res: any = await uploadKnowledgeFile(kbId.value, { file: f })
      const knowledge = res?.data || res
      const kid = knowledge?.id || knowledge?.knowledge_id
      // 解析完成后由轮询自动触发提取，无需在此登记
      void kid
    } catch (e2: any) {
      const msg = e2?.message || ''
      if (msg.includes('already exists') || msg.includes('文件重复')) {
        MessagePlugin.warning(`${f.name} 已存在，忽略重复上传`)
      } else {
        MessagePlugin.error(`${f.name} 上传失败：${msg}`)
      }
    }
  }
  MessagePlugin.success(`已上传 ${files.length} 个文件，正在解析...`)
  setTimeout(() => loadFiles(true), 1500)
}

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<Row | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')
const loadTags = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeTags(kbId.value, { page: 1, page_size: 100 })
    const pageData = (res?.data || {}) as { data?: any[]; total?: number }
    tagList.value = (pageData.data || []).map((tag: any) => ({ ...tag, id: String(tag.id) }))
  } catch { /* ignore */ }
}
const rowTags = (row: Row) => {
  const arr = row.tags || []
  return Array.isArray(arr) ? arr : []
}
const openTagEdit = (row: Row) => {
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
const onTagManageChanged = () => { loadTags(); loadFiles(true) }

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<Row | null>(null)
const editForm = ref<Record<string, any>>({})
const detailFieldDefs = computed(() => columnDefs.value)
const summaryExpanded = ref(true)
const feeExpanded = ref(false)
const autoSaving = ref(false)
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null
let autoSaveDirty = false
let editFormSnapshot = ''
let feeItemsSnapshot = ''

const detailTitle = computed(() => currentRow.value?.fileName || '账单详情')

const currentStatus = computed<{ label: string; theme: string; icon?: string; spin?: boolean }>(() =>
  currentRow.value ? statusOf(currentRow.value) : { label: '--', theme: 'default' })

const fieldValue = (cfg: ColDef) => editForm.value?.[cfg.key] ?? ''
const setFieldValue = (cfg: ColDef, v: any) => {
  if (v === undefined || v === null) return
  editForm.value[cfg.key] = v
}

const summaryLines = computed(() => {
  const d = currentRow.value?.item?.remark || ''
  return d ? d.replace(/-(?=[^\s-])/g, '\n-').trim() : ''
})

const openDetail = async (row: Row) => {
  currentRow.value = row
  await loadEditForm(row)
  // 电费账单 → 进入分层详情视图
  detailMode.value = true
  activeMenu.value = 'overview'
}

const loadEditForm = async (row: Row) => {
  autoSaveDirty = false
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const records = Array.isArray(meta.records) ? meta.records : []
    const idx = Math.max(0, (row.page || 1) - 1)
    const item = records[idx] || row.item || {}
    feeOverrides.value = (item.overview_overrides && typeof item.overview_overrides === 'object') ? { ...item.overview_overrides } : {}
    editForm.value = { ...item, fee_items: Array.isArray(item.fee_items) ? item.fee_items.map((f: any) => ({ ...f })) : [] }
    editFormSnapshot = JSON.stringify(editForm.value)
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  } catch {
    feeOverrides.value = {}
    editForm.value = { ...row.item, fee_items: Array.isArray(row.item?.fee_items) ? row.item.fee_items.map((f: any) => ({ ...f })) : [] }
    editFormSnapshot = JSON.stringify(editForm.value)
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  }
}

const closeDetail = () => {
  detailVisible.value = false
  currentRow.value = null
}

// ---- 详情视图（分层菜单） ----
const detailMode = ref(false)
const activeMenu = ref('overview')

interface BillMenu { key: string; label: string; group?: boolean; children?: { key: string; label: string }[] }
const BILL_MENUS: BillMenu[] = [
  { key: 'overview', label: '账单概况' },
  {
    key: 'industrial', label: '工商业电费', group: true, children: [
      { key: 'industrial-meter', label: '电量明细' },
      { key: 'industrial-market', label: '市场化购电费' },
      { key: 'industrial-line', label: '上网环节线损费' },
      { key: 'industrial-trans', label: '输配电量电费' },
      { key: 'industrial-sys', label: '系统运行费' },
      { key: 'industrial-gov', label: '政府基金及附加' },
    ],
  },
  {
    key: 'residential', label: '居民电费', group: true, children: [
      { key: 'residential-meter', label: '电量明细' },
      { key: 'residential-catalog', label: '目录电费' },
      { key: 'residential-gov', label: '政府性基金及附加' },
    ],
  },
  { key: 'capacity', label: '输配容（需）量电费' },
  { key: 'pf-adjust', label: '功率因素调整电费' },
]

const exitDetail = () => {
  detailMode.value = false
  activeMenu.value = 'overview'
  loadFiles(true)
}

const FEE_MENU_MAP: Record<string, (it: any) => boolean> = {
  'industrial-market': (it) => (it.category || '').includes('市场化购电'),
  'industrial-line': (it) => (it.category || '').includes('上网环节线损'),
  'industrial-trans': (it) => (it.category || '').includes('输配电量'),
  'industrial-sys': (it) => (it.category || '').includes('系统运行'),
  'industrial-gov': (it) => (it.category || '').includes('政府性基金') && Number(it.qty || 0) >= 100000 && !(it.name || '').includes('功率因数'),
  'residential-catalog': (it) => (it.category || '').includes('目录电费'),
  'residential-gov': (it) => (it.category || '').includes('政府性基金') && Number(it.qty || 0) < 100000 && !(it.name || '').includes('功率因数'),
  'pf-adjust': (it) => (it.category || '').includes('功率因数') || (it.name || '').includes('功率因数'),
}
const feeMenuOf = (key: string) => key in FEE_MENU_MAP && key !== 'pf-adjust' // pf-adjust 独立明细列表
// 费用菜单 → 行配置分组（行字段配置驱动各费用表格的行）
const ROW_GROUP_OF_MENU: Record<string, string> = {
  'industrial-market': 'market-rows',
  'industrial-trans': 'trans-rows',
  'industrial-sys': 'sys-rows',
  'industrial-gov': 'gov-industrial-rows',
  'residential-catalog': 'catalog-rows',
  'residential-gov': 'gov-residential-rows',
}
// 分时时段（仅括号内为这些时段时，配置 key 才拆分为 名称+时段；如"零售交易电费(尖峰)"）
const ROW_PERIODS = ['尖峰', '峰', '平', '谷']
const normRowName = (s: string) => (s || '').trim().replace(/[（(]/g, '(').replace(/[）)]/g, ')')
const parseRowKey = (k: string) => {
  const m = /^(.+?)\(([^)]+)\)$/.exec(k || '')
  if (m && ROW_PERIODS.includes(m[2])) return { name: m[1], period: m[2] }
  return { name: k || '', period: '' }
}
const feeRowsOf = (key: string) => {
  const list = (editForm.value.fee_items || []).filter(FEE_MENU_MAP[key])
  const rg = ROW_GROUP_OF_MENU[key]
  const cfgs = (rg ? rowConfigsOf(rg) : []).filter((c: any) => c.default_visible !== false)
  if (!cfgs.length) return list // 无行配置 → 原样展示提取结果
  // 按行配置过滤 + 排序 + 改名（未配置的行隐藏；分时子项按 名称(时段) 一一对应）
  return cfgs.map((cfg: any) => {
    const want = parseRowKey(cfg.field_key)
    const hit = list.find((it: any) => {
      const n = normRowName(it.name)
      const p = (it.period || '').trim()
      if (want.period) return n === normRowName(want.name) && p === want.period
      return n === normRowName(want.name)
    })
    if (!hit) return null
    return { ...hit, name: cfg.label || cfg.field_key }
  }).filter(Boolean)
}
const feeSubtotalOf = (key: string) => Math.round(feeRowsOf(key).reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0) * 100) / 100
const menuLabel = (key: string) => {
  for (const m of BILL_MENUS) {
    if (m.children) {
      const c = m.children.find(x => x.key === key)
      if (c) return c.label
    }
  }
  return key
}

// 容需量字段（编辑抽屉）
const CAPACITY_FIELDS = [
  { key: 'demand', label: '需量值（kW）', type: 'number' },
  { key: 'demand_price', label: '需量电价（元/kW）', type: 'number' },
  { key: 'demand_fee', label: '输配需量电费（元）', type: 'number' },
  { key: 'kwh_per_kva', label: '月每千伏安用电量（kWh/kVA）', type: 'number' },
  { key: 'discount_demand_fee', label: '折扣需量电费（元）', type: 'number' },
  { key: 'capacity', label: '容量（kVA）', type: 'number' },
  { key: 'capacity_price', label: '容量电价（元/kVA）', type: 'number' },
  { key: 'capacity_fee', label: '输配容量电费（元）', type: 'number' },
]

// 功率因数字段（编辑抽屉）
const PF_FIELDS = [
  { key: 'project', label: '项目', type: 'text' },
  { key: 'power_factor', label: '功率因素实际值', type: 'number' },
  { key: 'pf_standard', label: '功率因素标准', type: 'number' },
  { key: 'adjust_ratio', label: '调整系数', type: 'number' },
  { key: 'pf_active_kwh', label: '参与调整有功电量（kWh）', type: 'number' },
  { key: 'pf_reactive_kwh', label: '参与调整无功电量（kvarh）', type: 'number' },
  { key: 'pf_fee_base', label: '参与调整电费金额（元）', type: 'number' },
  { key: 'adjust_fee', label: '功率因素调整电费（元）', type: 'number' },
]

// 居民电量明细（与工商业电量明细一致 9 列；行=定比0.015，计费电量=本期电量×定比+加减，差异可手动修改）
const residentMeterRows = computed(() => {
  const totalKwh = Number(editForm.value.total_kwh) || 0
  const list = editForm.value.residential_readings
  const arr = Array.isArray(list) && list.length ? list : []
  const build = (cfgLabel: string, hit: any) => {
    const ratio = Number(hit?.ratio) || 0.015
    const adjust = Number(hit?.adjust) || 0
    const autoBill = totalKwh > 0 ? Math.round((totalKwh * ratio + adjust) * 100) / 100 : 0
    const bill = hit?.bill_kwh_manual ? (Number(hit.bill_kwh) || 0) : autoBill
    return { meter_type: cfgLabel, prev: '', curr: '', multiplier: '', reading_kwh: '', trans_loss: '', line_loss: '', adjust, bill_kwh: bill }
  }
  const cfgs = residentRowConfigs.value
  if (!cfgs.length) {
    return [build('定比0.015', arr[0] || { ratio: 0.015, adjust: 0, bill_kwh: 0 })]
  }
  return cfgs.map((cfg: any, ci: number) => {
    const hit = arr[ci] || arr[0] || null
    return build(cfg.label || cfg.field_key, hit)
  })
})
const residentBillTotal = computed(() => residentMeterRows.value.reduce((s: number, r: any) => s + (Number(r.bill_kwh) || 0), 0))
const residentMeterEditIdx = ref(-1)
let residentEditSnapshot = ''
const openResidentMeterEdit = (idx: number) => {
  const r = residentMeterRows.value[idx] || {}
  residentMeterEditIdx.value = idx
  detailEditTarget = 'resident'
  detailEditIdx = idx
  detailEditTitle.value = '编辑居民电量明细'
  detailEditFields.value = [
    { key: 'adjust', label: '加减（千瓦时）', type: 'number' },
    { key: 'bill_kwh', label: '计费电量（千瓦时）', type: 'number' },
  ]
  detailEditForm.value = { adjust: r.adjust ?? 0, bill_kwh: r.bill_kwh ?? 0 }
  residentEditSnapshot = JSON.stringify(detailEditForm.value)
  detailEditVisible.value = true
}
const capacityRow = computed(() => editForm.value.capacity_detail || null)
const pfRow = computed(() => editForm.value.pf_detail || null)
// 列表展示行（容量/功率因数均为单行）
const capacityRows = computed(() => (capacityRow.value ? [capacityRow.value] : []))
const pfRows = computed(() => (pfRow.value ? [pfRow.value] : []))

const sumFee = (arr: any[]) => Math.round(arr.reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0) * 100) / 100
// 类别计费数量：取该类别行中最大计费电量（子项分时段的 qty 各为其段电量，类别口径用整表电量代表值）
const sumQty = (arr: any[]) => {
  if (!arr.length) return 0
  return Math.round(Math.max(...arr.map((it: any) => Number(it.qty) || 0)) * 10) / 10
}
const sumBillFee = (arr: any[]) => Math.round(arr.reduce((s: number, it: any) => s + (Number(it.fee_amount) || 0), 0) * 100) / 100

const overviewRows = computed(() => {
  const edit = editForm.value
  const market = sumFee(feeRowsOf('industrial-market'))
  const line = sumFee(feeRowsOf('industrial-line'))
  const trans = sumFee(feeRowsOf('industrial-trans'))
  const sys = sumFee(feeRowsOf('industrial-sys'))
  const govI = sumFee(feeRowsOf('industrial-gov'))
  const catalog = sumFee(feeRowsOf('residential-catalog'))
  const govR = sumFee(feeRowsOf('residential-gov'))
  const capacity = Number(edit.capacity_fee) || 0
  const pf = Number(edit.pf_adjust_amount) || 0
  const marketBill = sumBillFee(feeRowsOf('industrial-market'))
  const lineBill = sumBillFee(feeRowsOf('industrial-line'))
  const transBill = sumBillFee(feeRowsOf('industrial-trans'))
  const sysBill = sumBillFee(feeRowsOf('industrial-sys'))
  const govIBill = sumBillFee(feeRowsOf('industrial-gov'))
  const catalogBill = sumBillFee(feeRowsOf('residential-catalog'))
  const govRBill = sumBillFee(feeRowsOf('residential-gov'))
  // 工商业小计 = 市场化+线损+输配量+系统+政府基金(工商业)+输配容(需)量电费（与账单口径一致）
  const industrial = Math.round((market + line + trans + sys + govI + capacity) * 100) / 100
  const residential = Math.round((catalog + govR) * 100) / 100
  const industrialBill = Math.round((marketBill + lineBill + transBill + sysBill + govIBill + capacity) * 100) / 100
  const residentialBill = Math.round((catalogBill + govRBill) * 100) / 100
  const m = (key: string) => sumQty(feeRowsOf(key))
  const withOv = (key: string, value: number) => {
    const ov = feeOverrides.value[key]
    return { value, displayValue: ov !== undefined ? ov : value }
  }
  return [
    { key: 'market', label: '市场化购电费', ...withOv('market', market), billFee: marketBill, qty: m('industrial-market'), menuKey: 'industrial-market' },
    { key: 'line', label: '上网环节线损费', ...withOv('line', line), billFee: lineBill, qty: m('industrial-line'), menuKey: 'industrial-line' },
    { key: 'trans', label: '输配电量电费', ...withOv('trans', trans), billFee: transBill, qty: m('industrial-trans'), menuKey: 'industrial-trans' },
    { key: 'sys', label: '系统运行费', ...withOv('sys', sys), billFee: sysBill, qty: m('industrial-sys'), menuKey: 'industrial-sys' },
    { key: 'govI', label: '政府基金及附加（工商业）', ...withOv('govI', govI), billFee: govIBill, qty: m('industrial-gov'), menuKey: 'industrial-gov' },
    { key: 'industrial', label: '工商业电费小计', ...withOv('industrial', industrial), billFee: industrialBill, qty: -1, menuKey: '', total: true },
    { key: 'catalog', label: '目录电费（居民）', ...withOv('catalog', catalog), billFee: catalogBill, qty: m('residential-catalog'), menuKey: 'residential-catalog' },
    { key: 'govR', label: '政府性基金及附加（居民）', ...withOv('govR', govR), billFee: govRBill, qty: m('residential-gov'), menuKey: 'residential-gov' },
    { key: 'residential', label: '居民电费小计', ...withOv('residential', residential), billFee: residentialBill, qty: -1, menuKey: '', total: true },
    { key: 'capacity', label: '输配容（需）量电费', ...withOv('capacity', capacity), billFee: capacity, qty: Number(edit.capacity) || 0, menuKey: 'capacity' },
    { key: 'pf', label: '功率因数调整电费', ...withOv('pf', pf), billFee: pf, qty: -1, menuKey: 'pf-adjust' },
  ]
})
const qtyLabelOf = (r: any) => {
  if (r.qty < 0) return ''
  if (r.key === 'capacity') return r.qty ? `${fmtKwh(r.qty)} kVA` : ''
  return r.qty ? `${fmtKwh(r.qty)} 千瓦时` : ''
}
const gotoMenu = (key: string) => {
  if (!key) return
  activeMenu.value = key
}
const overviewTotal = computed(() => {
  const vals = overviewRows.value
  const industrial = vals.find(r => r.key === 'industrial')?.displayValue || 0
  const residential = vals.find(r => r.key === 'residential')?.displayValue || 0
  // 容量电费已计入工商业小计，功率因数调整电费单独计
  const pf = vals.find(r => r.key === 'pf')?.displayValue || 0
  return Math.round((industrial + residential + pf) * 100) / 100
})
// 账单电费（解析提取）与汇总对比：优先合计电费 grand_total（新口径，含容需量/力调），旧数据回退 total_amount
const billTotal = computed(() => Number(editForm.value.grand_total) || Number(editForm.value.total_amount) || 0)
const billTotalText = computed(() => (billTotal.value ? fmtRate6(billTotal.value) : ''))
const billTotalOk = computed(() => {
  if (!billTotal.value) return true
  return feeClose(overviewTotal.value, billTotal.value)
})
const overviewDiff = computed(() => {
  const nominal = billTotal.value
  if (!nominal) return 0
  return Math.round((overviewTotal.value - nominal) * 100) / 100
})

// ---- 概览页指标卡（数据均来自子项计算 / 提取字段，非直接填充汇总） ----
const metricKwh = computed(() => Number(editForm.value.total_kwh) || 0)
const metricFee = computed(() => overviewTotal.value)
const metricPeriod = computed(() => {
  const v = editForm.value.bill_period_start || editForm.value.bill_period_end
  return v ? String(v).slice(0, 7) : '—'
})
const metricDue = computed(() => {
  const v = editForm.value.due_date || editForm.value.pay_deadline
  return v ? String(v).slice(0, 10) : '—'
})
// 指标卡环比（本期电量 / 本期电费，均与上期对比）
const metricKwhDelta = computed(() => {
  if (!prevEnergy.value.exists) return 0
  return metricKwh.value - prevEnergy.value.total
})
const metricKwhDeltaArrow = computed(() => (metricKwhDelta.value > 0 ? '↑' : metricKwhDelta.value < 0 ? '↓' : '—'))
const metricKwhDeltaClass = computed(() => (metricKwhDelta.value > 0 ? 'ea-delta--up' : metricKwhDelta.value < 0 ? 'ea-delta--down' : 'ea-delta--flat'))
const metricKwhDeltaSub = computed(() => {
  if (metricKwhDelta.value === 0) return '持平'
  const sign = metricKwhDelta.value > 0 ? '+' : '-'
  return `${sign}${fmtKwh(Math.abs(metricKwhDelta.value))} 千瓦时`
})
const metricKwhTip = computed(() => {
  if (!prevEnergy.value.exists) return '暂无上期数据'
  const cur = metricKwh.value, prev = prevEnergy.value.total
  const pct = prev ? ((cur - prev) / prev) * 100 : 0
  return `本期电量 ${fmtKwh(cur)} 千瓦时，上期 ${fmtKwh(prev)} 千瓦时\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
})
const metricFeeDelta = computed(() => {
  if (!prevEnergy.value.exists) return 0
  return metricFee.value - prevEnergy.value.fee
})
const metricFeeDeltaArrow = computed(() => (metricFeeDelta.value > 0 ? '↑' : metricFeeDelta.value < 0 ? '↓' : '—'))
const metricFeeDeltaClass = computed(() => (metricFeeDelta.value > 0 ? 'ea-delta--up' : metricFeeDelta.value < 0 ? 'ea-delta--down' : 'ea-delta--flat'))
const metricFeeDeltaSub = computed(() => {
  if (metricFeeDelta.value === 0) return '持平'
  const sign = metricFeeDelta.value > 0 ? '+' : '-'
  return `${sign}${fmtMoney(Math.abs(metricFeeDelta.value))} 元`
})
const metricFeeTip = computed(() => {
  if (!prevEnergy.value.exists) return '暂无上期数据'
  const cur = metricFee.value, prev = prevEnergy.value.fee
  const pct = prev ? ((cur - prev) / prev) * 100 : 0
  return `本期电费 ${fmtMoney(cur)} 元，上期 ${fmtMoney(prev)} 元\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
})
const ovText = (f: { key: string; label: string }) => {
  const v = editForm.value[f.key]
  if (v === null || v === undefined || v === '') return ''
  if (f.key === 'bill_period_start' || f.key === 'bill_period_end') return String(v).slice(0, 7)
  return String(v)
}
// 用能分析
const momText = computed(() => {
  const prev = prevEnergy.value.exists ? prevEnergy.value.total : 0
  const cur = Number(editForm.value.total_kwh) || 0
  if (!prev || !cur) return '—'
  const pct = Math.round(((cur - prev) / prev) * 1000) / 10
  return `${pct > 0 ? '+' : ''}${pct}%`
})
const momDelta = computed(() => {
  const prev = prevEnergy.value.exists ? prevEnergy.value.total : 0
  const cur = Number(editForm.value.total_kwh) || 0
  if (!prev || !cur) return 0
  return cur - prev
})
const momDeltaArrow = computed(() => (momDelta.value > 0 ? '↑' : momDelta.value < 0 ? '↓' : ''))
const momDeltaClass = computed(() => (momDelta.value > 0 ? 'ea-delta--up' : momDelta.value < 0 ? 'ea-delta--down' : 'ea-delta--flat'))
const momDeltaSub = computed(() => {
  if (momDelta.value === 0) return '持平'
  return `${fmtKwh(Math.abs(momDelta.value))} 千瓦时`
})
const momTip = computed(() => {
  const cur = Number(editForm.value.total_kwh) || 0
  const prev = prevEnergy.value.total
  if (!prev || !cur) return '暂无上期数据'
  const pct = ((cur - prev) / prev) * 100
  return `本期电量 ${fmtKwh(cur)} 千瓦时，上期 ${fmtKwh(prev)} 千瓦时\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
})
const peakValleyText = computed(() => {
  const peak = (Number(editForm.value.deep_peak_kwh) || 0) + (Number(editForm.value.peak_kwh) || 0)
  const valley = Number(editForm.value.valley_kwh) || 0
  if (!peak && !valley) return '—'
  return `${peak}:${valley}`
})
const pfText = computed(() => {
  const v = editForm.value.power_factor
  return v === null || v === undefined || v === '' ? '—' : String(v)
})
const pfDelta = computed(() => {
  const cur = Number(editForm.value.power_factor) || 0
  const prev = Number(prevEnergy.value.power_factor) || 0
  if (!cur && !prev) return 0
  return Math.round((cur - prev) * 1000) / 1000
})
const pfDeltaArrow = computed(() => (pfDelta.value > 0 ? '↑' : pfDelta.value < 0 ? '↓' : ''))
const pfDeltaClass = computed(() => (pfDelta.value > 0 ? 'ea-delta--up' : pfDelta.value < 0 ? 'ea-delta--down' : 'ea-delta--flat'))
const pfDeltaSub = computed(() => (pfDelta.value === 0 ? '持平' : `${Math.abs(pfDelta.value).toFixed(2)}`))
const pfTip = computed(() => {
  const cur = Number(editForm.value.power_factor) || 0
  const prev = Number(prevEnergy.value.power_factor) || 0
  if (!cur && !prev) return '暂无上期数据'
  return `本期功率因数 ${cur}，上期 ${prev}\n变化 = ${pfDelta.value > 0 ? '+' : ''}${pfDelta.value.toFixed(2)}`
})
const demandText = computed(() => {
  const v = Number(editForm.value.demand) || 0
  return v ? `${fmtKwh(v)} kW` : '—'
})
const avgPriceText = computed(() => {
  const v = editForm.value.avg_price
  if (v !== null && v !== undefined && v !== '') return String(Number(v))
  const cur = Number(editForm.value.total_kwh) || 0
  if (!cur) return '—'
  return String(Math.round((metricFee.value / cur) * 10000) / 10000)
})
const avgPriceDelta = computed(() => {
  const cur = Number(avgPriceText.value) || 0
  const prev = prevEnergy.value.avg_price
  if (!cur && !prev) return 0
  return Math.round((cur - prev) * 10000) / 10000
})
const avgDeltaArrow = computed(() => (avgPriceDelta.value > 0 ? '↑' : avgPriceDelta.value < 0 ? '↓' : ''))
const avgDeltaClass = computed(() => (avgPriceDelta.value > 0 ? 'ea-delta--up' : avgPriceDelta.value < 0 ? 'ea-delta--down' : 'ea-delta--flat'))
const avgDeltaSub = computed(() => (avgPriceDelta.value === 0 ? '持平' : `${Math.abs(avgPriceDelta.value).toFixed(4)} 元/千瓦时`))
const avgTip = computed(() => {
  const cur = Number(avgPriceText.value) || 0
  const prev = prevEnergy.value.avg_price
  if (!cur && !prev) return '暂无上期数据'
  const pct = prev ? ((cur - prev) / prev) * 100 : 0
  return `平均电价 = 本期电费 ÷ 本期电量 = ${cur} 元/千瓦时\n上期 ${prev} 元/千瓦时，变化 ${avgPriceDelta.value > 0 ? '+' : ''}${avgPriceDelta.value.toFixed(4)}（${pct.toFixed(2)}%）`
})
const energyBars = computed(() => {
  const raw = [
    { key: 'deep_peak', label: '尖', v: Number(editForm.value.deep_peak_kwh) || 0 },
    { key: 'peak', label: '峰', v: Number(editForm.value.peak_kwh) || 0 },
    { key: 'flat', label: '平', v: Number(editForm.value.flat_kwh) || 0 },
    { key: 'valley', label: '谷', v: Number(editForm.value.valley_kwh) || 0 },
  ]
  const max = Math.max(...raw.map(b => b.v), 1)
  return raw.map(b => ({ ...b, pct: Math.max(Math.round((b.v / max) * 100), 2) }))
})

// ---- 上期分时数据（按账单周期取最近上一期） ----
const prevEnergy = computed(() => {
  const curStart = editForm.value.bill_period_start || ''
  const prevs = allRecords.value
    .map(r => r.item || {})
    .filter(m => m.bill_period_start && String(m.bill_period_start) < curStart)
    .sort((a: any, b: any) => String(b.bill_period_start).localeCompare(String(a.bill_period_start)))
  const p = prevs[0] || {}
  const exists = !!p.bill_period_start
  return {
    exists,
    period: exists ? String(p.bill_period_start).slice(0, 7) : '',
    deep: Number(p.deep_peak_kwh) || 0,
    peak: Number(p.peak_kwh) || 0,
    flat: Number(p.flat_kwh) || 0,
    valley: Number(p.valley_kwh) || 0,
    total: Number(p.total_kwh) || 0,
    fee: Number(p.total_amount) || Number(p.grand_total) || 0,
    power_factor: p.power_factor === null || p.power_factor === undefined || p.power_factor === '' ? 0 : Number(p.power_factor),
    avg_price: p.avg_price === null || p.avg_price === undefined || p.avg_price === '' ? 0 : Number(p.avg_price),
  }
})

// 分时占比对比表（本期占比 = 时段电量 ÷ 本期总电量 × 100%；尖/峰/谷按公式，平段取余量，保证四舍五入后合计 100%）
const energyCompareRows = computed(() => {
  const curTotal = Number(editForm.value.total_kwh) || 0
  const prevTotal = prevEnergy.value.total
  const pct = (v: number, total: number) => (total ? Math.round((v / total) * 10000) / 100 : 0)
  const cur = [
    { key: 'deep', label: '尖', v: Number(editForm.value.deep_peak_kwh) || 0 },
    { key: 'peak', label: '峰', v: Number(editForm.value.peak_kwh) || 0 },
    { key: 'flat', label: '平', v: Number(editForm.value.flat_kwh) || 0 },
    { key: 'valley', label: '谷', v: Number(editForm.value.valley_kwh) || 0 },
  ]
  const prev = {
    deep: prevEnergy.value.deep, peak: prevEnergy.value.peak,
    flat: prevEnergy.value.flat, valley: prevEnergy.value.valley,
  }
  const round2 = (n: number) => Math.round(n * 100) / 100
  const curPcts = cur.map(c => ({ key: c.key, pct: pct(c.v, curTotal) }))
  const prevPcts = cur.map(c => ({ key: c.key, pct: prevTotal ? pct(prev[c.key] || 0, prevTotal) : 0 }))
  const sumExcl = (arr: { key: string; pct: number }[], excl: string) =>
    arr.filter(a => a.key !== excl).reduce((s, a) => s + a.pct, 0)
  const curFlat = curTotal ? Math.max(0, round2(100 - sumExcl(curPcts, 'flat'))) : 0
  const prevFlat = prevTotal ? Math.max(0, round2(100 - sumExcl(prevPcts, 'flat'))) : 0
  const curPctOf = (key: string) => (key === 'flat' ? curFlat : curPcts.find(p => p.key === key)!.pct)
  const prevPctOf = (key: string) => (key === 'flat' ? prevFlat : prevPcts.find(p => p.key === key)!.pct)
  return cur.map(c => {
    const pv = prev[c.key] || 0
    const curPct = curPctOf(c.key)
    const prevPct = prevPctOf(c.key)
    return {
      key: c.key, label: c.label,
      cur: c.v, curPct,
      prev: pv, prevPct,
      delta: prevTotal ? Math.round((curPct - prevPct) * 10) / 10 : 0,
    }
  })
})

// ---- 用能分析 ECharts（分时电量占比对比：本期 vs 上期） ----
const energyChartRef = ref<HTMLDivElement | null>(null)
let energyChart: echarts.ECharts | null = null
const renderEnergyChart = async () => {
  await nextTick()
  const el = energyChartRef.value
  // 容器未渲染或尺寸为 0（v-if 切换/布局未稳定）时延迟重试
  if (!el || !el.clientWidth) {
    if (detailMode.value && activeMenu.value === 'overview') setTimeout(renderEnergyChart, 80)
    return
  }
  if (!energyChart || energyChart.getDom() !== el) {
    // 容器因 v-if 切换重建（如从明细菜单返回概览）时，旧实例仍挂在已销毁容器上，需重建
    energyChart?.dispose()
    energyChart = echarts.init(el)
  } else {
    // 容器尺寸变化（如概览布局调整）后同步图表尺寸
    energyChart.resize()
  }
  const rows = energyCompareRows.value
  const prevExists = prevEnergy.value.exists
  const pctLabel = (p: any) => `${p.value}%`
  energyChart.setOption({
    animation: true,
    animationDuration: 600,
    animationEasing: 'cubicOut',
    grid: { left: 8, right: 8, top: 42, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      formatter: (ps: any) => {
        const arr = Array.isArray(ps) ? ps : [ps]
        return arr.map((p: any) => `${p.seriesName}：${p.value}%`).join('<br/>')
      },
    },
    legend: { data: ['本期占比', '上期占比'], top: 0, itemWidth: 12, itemHeight: 8, textStyle: { fontSize: 12, color: '#57606a' } },
    xAxis: {
      type: 'category', data: rows.map(r => r.label),
      axisLine: { lineStyle: { color: '#d0d7de' } }, axisLabel: { color: '#57606a', fontSize: 12 },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      max: (v: any) => Math.ceil((v.max * 1.15) / 10) * 10,
      axisLabel: { color: '#57606a', fontSize: 12, formatter: '{value}%' },
      splitLine: { lineStyle: { color: '#eaeef2' } },
    },
    series: [
      {
        name: '本期占比', type: 'bar', data: rows.map(r => r.curPct), barWidth: 22,
        itemStyle: { color: '#0052d9', borderRadius: [3, 3, 0, 0] },
        label: { show: true, position: 'top', distance: 4, color: '#0052d9', fontSize: 10, formatter: pctLabel },
      },
      {
        name: '上期占比', type: 'bar', data: rows.map(r => (prevExists ? r.prevPct : null)), barWidth: 22,
        itemStyle: { color: '#9ab6e8', borderRadius: [3, 3, 0, 0] },
        label: { show: prevExists, position: 'insideTop', distance: 2, color: '#fff', fontSize: 10, formatter: pctLabel },
      },
    ],
  })
}
watch([detailMode, activeMenu, () => currentRow.value, energyCompareRows, () => editForm.value], () => {
  if (detailMode.value && activeMenu.value === 'overview') renderEnergyChart()
}, { deep: true })
// 关闭详情时销毁实例，避免 v-if 重建后图表挂载到旧容器不显示
watch(detailMode, (v) => {
  if (!v) { energyChart?.dispose(); energyChart = null }
})
onBeforeUnmount(() => { energyChart?.dispose(); energyChart = null })

// 电量明细
const META_ORDER = ['正向有功（总）', '正向有功（尖峰）', '正向有功（峰）', '正向有功（平）', '正向有功（谷）', '正向无功（总）']
// 行配置：按设置中「电量明细行」分组渲染（field_key=默认行文本，label 可改名）
const rowConfigsOf = (group: string) => {
  const list = allFieldConfigs.value.filter((c: any) => c.group === group && c.deleted_at == null)
  return [...list].sort((a: any, b: any) => (a.sort_order || 0) - (b.sort_order || 0))
}
const meterRowConfigs = computed(() => rowConfigsOf('meter-rows').filter((c: any) => c.default_visible !== false))
const residentRowConfigs = computed(() => rowConfigsOf('resident-meter-rows').filter((c: any) => c.default_visible !== false))
const matchRow = (row: any, cfg: any) => {
  const t = row?.meter_type || row?.project || ''
  if (!t) return false
  return t === cfg.field_key || t === cfg.label || t.includes(cfg.field_key) || cfg.field_key.includes(t)
}
// 电量明细：账单「电量明细」表逐行（示数类型/上期/本期/倍率/抄见/变损/线损/加减/计费电量）
const meterRows = computed(() => {
  let list: any[]
  const mr = editForm.value.meter_readings
  if (Array.isArray(mr) && mr.length) {
    list = mr
  } else {
    // 旧数据兜底：仅计费电量有值
    const e = editForm.value
    const row = (meter_type: string, bill_kwh: number) => ({ meter_type, prev: 0, curr: 0, multiplier: 1, reading_kwh: 0, trans_loss: 0, line_loss: 0, adjust: 0, bill_kwh: Number(bill_kwh) || 0 })
    list = [
      row('正向有功（总）', e.total_kwh),
      row('正向有功（尖峰）', e.deep_peak_kwh),
      row('正向有功（峰）', e.peak_kwh),
      row('正向有功（平）', e.flat_kwh),
      row('正向有功（谷）', e.valley_kwh),
      row('正向无功（总）', e.reactive_kwh),
    ]
  }
  const cfgs = meterRowConfigs.value
  if (!cfgs.length) {
    return [...list].sort((a, b) => {
      const ia = META_ORDER.indexOf(a.meter_type)
      const ib = META_ORDER.indexOf(b.meter_type)
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib)
    })
  }
  return cfgs.map(cfg => {
    const hit = list.find(m => matchRow(m, cfg))
    if (hit) return { ...hit, meter_type: cfg.label }
    return { meter_type: cfg.label, prev: '', curr: '', multiplier: '', reading_kwh: '', trans_loss: '', line_loss: '', adjust: '', bill_kwh: '' }
  })
})
const meterTotal = computed(() => meterRows.value.reduce((s, r) => s + (Number(r.bill_kwh) || 0), 0))
const meterPct = (v: number, total: number) => (total ? `${(v / total * 100).toFixed(2)}%` : '--')

// 电量明细编辑（9 字段）
const meterEditVisible = ref(false)
const meterEditForm = ref<Record<string, any>>({})
const meterEditTitle = ref('')
const meterEditFields = ref<{ key: string; label: string }[]>([])
let meterEditIdx = -1
const openMeterEdit = (idx: number) => {
  const r = meterRows.value[idx]
  if (!r) return
  meterEditTitle.value = r.meter_type || meterRowConfigs.value[idx]?.label || '电量明细'
  meterEditFields.value = [
    { key: 'prev', label: '上期示数' },
    { key: 'curr', label: '本期示数' },
    { key: 'multiplier', label: '倍率' },
    { key: 'reading_kwh', label: '抄见电量' },
    { key: 'trans_loss', label: '变损' },
    { key: 'line_loss', label: '线损' },
    { key: 'adjust', label: '加减' },
    { key: 'bill_kwh', label: '计费电量' },
  ]
  meterEditForm.value = { ...r }
  meterEditIdx = idx
  meterEditVisible.value = true
}
watch(meterEditForm, () => {
  if (!meterEditVisible.value || !autoSaveDirty) return
  if (meterEditIdx < 0) return
  let list = editForm.value.meter_readings
  if (!Array.isArray(list)) {
    list = []
    editForm.value.meter_readings = list
  }
  const cfg = meterRowConfigs.value[meterEditIdx]
  const title = meterEditForm.value.meter_type || cfg?.label || cfg?.field_key || `行${meterEditIdx + 1}`
  let row = list.find((m: any) => matchRow(m, { field_key: title, label: title }))
  if (!row) {
    row = { meter_type: title, prev: 0, curr: 0, multiplier: 1, reading_kwh: 0, trans_loss: 0, line_loss: 0, adjust: 0, bill_kwh: 0 }
    list.push(row)
  }
  Object.keys(meterEditForm.value).forEach(k => { row[k] = meterEditForm.value[k] })
}, { deep: true })

// 费用行编辑
const feeItemEditVisible = ref(false)
const feeItemEditForm = ref<Record<string, any>>({})
const feeItemEditName = ref('')
let feeItemEditTarget = ''
let feeItemEditIdx = -1
const feeItemEditableFee = computed(() => {
  const qty = Number(feeItemEditForm.value.qty) || 0
  const rate = Number(feeItemEditForm.value.rate) || 0
  return qty > 0 && rate > 0
})
const recalcFeeItem = () => {
  const qty = Number(feeItemEditForm.value.qty) || 0
  const rate = Number(feeItemEditForm.value.rate) || 0
  if (qty > 0 && rate > 0) feeItemEditForm.value.fee = Math.round(qty * rate * 100) / 100
}
const openFeeItemEdit = (menuKey: string, idx: number) => {
  const list = feeRowsOf(menuKey)
  const it = list[idx]
  feeItemEditTarget = menuKey
  feeItemEditName.value = it?.name || '费用项'
  feeItemEditForm.value = { ...it }
  feeItemEditIdx = (editForm.value.fee_items || []).indexOf(it)
  feeItemEditVisible.value = true
}
watch(feeItemEditForm, () => {
  if (!feeItemEditVisible.value || !autoSaveDirty) return
  if (feeItemEditIdx < 0) return
  const items = editForm.value.fee_items || []
  if (feeItemEditIdx >= items.length) return
  Object.keys(feeItemEditForm.value).forEach(k => { items[feeItemEditIdx][k] = feeItemEditForm.value[k] })
}, { deep: true })

// 静态字段编辑（概况 / 容需量 / 功率因数）
const staticEditVisible = ref(false)
const staticEditForm = ref<Record<string, any>>({})
const staticEditTitle = ref('')
const staticEditFields = ref<{ key: string; label: string; type: string }[]>([])
const openStaticEdit = (scope: 'capacity' | 'pf') => {
  if (scope === 'capacity') {
    staticEditTitle.value = '编辑输配容（需）量电费'
    staticEditFields.value = CAPACITY_FIELDS
  } else {
    staticEditTitle.value = '编辑功率因素调整电费'
    staticEditFields.value = PF_FIELDS
  }
  const form: Record<string, any> = {}
  staticEditFields.value.forEach(f => {
    if (f.type === 'number') form[f.key] = Number(editForm.value[f.key]) || 0
    else form[f.key] = editForm.value[f.key] ?? ''
  })
  staticEditForm.value = form
  staticEditVisible.value = true
}
watch(staticEditForm, () => {
  if (!staticEditVisible.value || !autoSaveDirty) return
  staticEditFields.value.forEach(f => {
    editForm.value[f.key] = staticEditForm.value[f.key]
  })
}, { deep: true })

// 明细列表通用编辑抽屉（居民电量 / 容需量 / 功率因素）
const detailEditVisible = ref(false)
const detailEditForm = ref<Record<string, any>>({})
const detailEditTitle = ref('')
const detailEditFields = ref<{ key: string; label: string; type: string }[]>([])
let detailEditTarget: 'resident' | 'capacity' | 'pf' = 'resident'
let detailEditIdx = -1
const openDetailEdit = (target: 'resident' | 'capacity' | 'pf', idx = 0) => {
  detailEditTarget = target
  detailEditIdx = idx
  if (target === 'capacity') {
    detailEditTitle.value = '编辑输配容（需）量电费'
    detailEditFields.value = CAPACITY_FIELDS
    detailEditForm.value = { ...(capacityRow.value || {}) }
  } else {
    detailEditTitle.value = '编辑功率因素调整电费'
    detailEditFields.value = PF_FIELDS
    detailEditForm.value = { ...(pfRow.value || {}) }
  }
  detailEditVisible.value = true
}
watch(detailEditForm, () => {
  if (!detailEditVisible.value || !autoSaveDirty) return
  if (detailEditTarget === 'resident') {
    let list = editForm.value.residential_readings
    if (!Array.isArray(list)) { list = []; editForm.value.residential_readings = list }
    const cfg = residentRowConfigs.value[residentMeterEditIdx.value >= 0 ? residentMeterEditIdx.value : detailEditIdx]
    const title = cfg?.label || cfg?.field_key || '定比0.015'
    let row = list[0] || list.find((r: any) => matchRow(r, { field_key: title, label: title }))
    if (!row) {
      row = { project: title, kwh: Number(editForm.value.total_kwh) || 0, ratio: 0.015, adjust: 0, bill_kwh: 0 }
      list.push(row)
    }
    if (JSON.stringify(detailEditForm.value) !== residentEditSnapshot) {
      if (detailEditForm.value.adjust !== undefined) row.adjust = detailEditForm.value.adjust
      if (detailEditForm.value.bill_kwh !== undefined) {
        row.bill_kwh = detailEditForm.value.bill_kwh
        row.bill_kwh_manual = true
      }
    }
  } else if (detailEditTarget === 'capacity') {
    const d = { ...(editForm.value.capacity_detail || {}), ...detailEditForm.value }
    if (d.demand > 0 && d.demand_price > 0) d.demand_fee = Math.round(d.demand * d.demand_price * 100) / 100
    if (d.capacity > 0 && d.capacity_price > 0) d.capacity_fee = Math.round(d.capacity * d.capacity_price * 100) / 100
    editForm.value.capacity_detail = d
  } else {
    editForm.value.pf_detail = { ...(editForm.value.pf_detail || {}), ...detailEditForm.value }
  }
}, { deep: true })

const fmtField = (v: any, type: string) => {
  if (v === undefined || v === null || v === '') return ''
  if (type === 'number') return Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 4 })
  return String(v)
}
const fmtRate = (v: any) => {
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
}

const onFieldEdited = () => { /* 数值字段 change 即进入自动保存流程 */ }

watch(editForm, () => {
  if (!autoSaveDirty || !currentRow.value) return
  if (JSON.stringify(editForm.value) === editFormSnapshot) return
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
    const records = Array.isArray(meta.records) ? [...meta.records] : []
    const idx = Math.max(0, (now.page || 1) - 1)
    const updated: Record<string, any> = {}
    columnDefs.value.forEach(c => {
      const v = editForm.value[c.key]
      if (v !== undefined) updated[c.key] = v
    })
    updated.fee_items = Array.isArray(editForm.value.fee_items) ? editForm.value.fee_items : []
    updated.meter_readings = Array.isArray(editForm.value.meter_readings) ? editForm.value.meter_readings : []
    updated.residential_readings = Array.isArray(editForm.value.residential_readings) ? editForm.value.residential_readings : []
    if (editForm.value.capacity_detail) updated.capacity_detail = editForm.value.capacity_detail
    if (editForm.value.pf_detail) updated.pf_detail = editForm.value.pf_detail
    updated.remark = editForm.value.remark || ''
    updated.overview_overrides = { ...feeOverrides.value }
    if (records[idx]) records[idx] = { ...records[idx], ...updated }
    else records.push({ ...updated })
    // 总账按明细重算：仅当费用明细实际变化时
    const feeChanged = JSON.stringify(updated.fee_items) !== feeItemsSnapshot
    const feeSum = updated.fee_items.reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0)
    const hasItem = updated.fee_items.length > 0
    const nextMeta = { ...meta, kind: 'utility_bill', records, extract_status: 'success', extract_error: '' }
    if (feeChanged && hasItem) {
      nextMeta.records[idx].total_amount = Math.round(feeSum * 100) / 100
      nextMeta.records[idx].grand_total = Math.round(feeSum * 100) / 100
      editForm.value.total_amount = nextMeta.records[idx].total_amount
      editForm.value.grand_total = nextMeta.records[idx].grand_total
      MessagePlugin.info('总账已按明细重算')
    }
    feeItemsSnapshot = JSON.stringify(updated.fee_items)
    await updateKnowledgeMetadata(now.knowledgeId, nextMeta)
    editFormSnapshot = JSON.stringify(editForm.value)
    // 同步列表行
    const listIdx = rows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) rows.value[listIdx] = { ...rows.value[listIdx], item: { ...rows.value[listIdx].item, ...updated } }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    autoSaving.value = false
  }
}

// 费用明细操作
const feeTotal = computed(() =>
  (editForm.value.fee_items || []).reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0))
const addFeeItem = () => {
  if (!Array.isArray(editForm.value.fee_items)) editForm.value.fee_items = []
  editForm.value.fee_items.push({ category: '', name: '', period: '', qty: 0, rate: 0, fee: 0 })
}
const removeFeeItem = (i: number) => {
  editForm.value.fee_items.splice(i, 1)
}

// ---- 抽屉宽度拖动 ----
const DRAWER_WIDTH_KEY = 'weknora-utility-drawer-width'
const DRAWER_DEFAULT_WIDTH = 720
const DRAWER_MIN_WIDTH = 560
const drawerWidth = ref(DRAWER_DEFAULT_WIDTH)
const drawerResizing = ref(false)
let drawerResizeStartX = 0
let drawerResizeStartWidth = 0
function loadDrawerWidth() {
  try {
    const v = Number(localStorage.getItem(DRAWER_WIDTH_KEY))
    drawerWidth.value = v >= DRAWER_MIN_WIDTH ? v : DRAWER_DEFAULT_WIDTH
  } catch { /* ignore */ }
}

// ---- 浮动工具栏 ----
const handleBatchEdit = () => {
  const r = selectedRows.value
  if (r.length !== 1) { MessagePlugin.info('请选中单行后编辑，或点击列表中的账单行进入编辑'); return }
  openDetail(r[0])
}
const handleReExtract = async () => {
  const row = selectedRows.value
  if (row.length !== 1) { MessagePlugin.info('重新提取仅支持单选'); return }
  const r = row[0]
  extractInFlight.value.add(r.knowledgeId)
  extractFailed.value.delete(r.knowledgeId)
  try {
    // 与合同管理一致：重新解析并提取（解析完成后由轮询自动触发字段提取）
    await reparseKnowledge(r.knowledgeId)
    MessagePlugin.success(`已触发「${r.fileName}」重新解析与提取`)
    setTimeout(() => loadFiles(true), 1500)
  } catch (e: any) {
    extractFailed.value.add(r.knowledgeId)
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(r.knowledgeId)
  }
}

// ---- 打印（合并多份账单为一个 PDF，iframe 预览） ----
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff']
const isImageRow = (r: any, blob: any) => {
  const ext = String(r.fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const printVisible = ref(false)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)
const printCount = ref(0)

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
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement | null
  if (frame?.contentWindow) {
    try { frame.contentWindow.print(); return } catch { /* fallback */ }
  }
  window.print()
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 删除 ----
const handleBatchDelete = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  try {
    for (const r of rowsSel) {
      // 删除记录 = 删除知识文件（软删进删除历史）
      const { delKnowledgeDetails } = await import('@/api/knowledge-base')
      await delKnowledgeDetails(r.knowledgeId)
    }
    MessagePlugin.success('已删除')
    selectedRowKeys.value = []
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 筛选 ----
const applyFilter = () => loadFiles(true)

// ---- 格式化 ----
const fmtKwh = (v: any) => {
  const n = Number(v) || 0
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
/** 明细电费/单价：最多 6 位小数，去尾 0 */
const fmtRate6 = (v: any) => {
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
}
const feeItemsOf = (row: Row): any[] => (Array.isArray(row.item?.fee_items) ? row.item.fee_items : [])

// ---- 账单概况：容差判定 + 电费列手动覆盖 ----
/** 计算值与账单提取值对比：允许 ±0.02 元（四舍五入误差）判为正常 */
const feeClose = (a: number, b: number) => Math.abs(a - b) <= 0.02
/** 手动覆盖的计算值（key → 金额），保存到记录 overview_overrides；重提取后重置 */
const feeOverrides = ref<Record<string, number>>({})
const feeEditingKey = ref('')
const feeEditValue = ref('')
const startFeeEdit = (r: any) => {
  feeEditingKey.value = r.key
  feeEditValue.value = String(r.displayValue ?? '')
  // 显示编辑框后自动聚焦：用户未先点击输入框直接点击外部时也能触发失焦保存
  nextTick(() => {
    const el = document.querySelector('.os-fee-input input') as HTMLInputElement | null
    el?.focus()
  })
}
const commitFeeOverride = async (r: any) => {
  feeEditingKey.value = ''
  if (feeEditValue.value === '') return
  const v = Number(feeEditValue.value)
  if (!Number.isFinite(v)) return
  feeOverrides.value = { ...feeOverrides.value, [r.key]: Math.round(v * 100) / 100 }
  autoSaveDirty = true
  await saveEditForm()
}
const hasFeeOverride = (key: string) => feeOverrides.value[key] !== undefined
const sumFeeBy = (row: Row, pred: (it: any) => boolean): number =>
  Math.round(feeItemsOf(row).filter(pred).reduce((s, it) => s + (Number(it.fee) || 0), 0) * 100) / 100
const feeTextOf = (row: Row, pred: (it: any) => boolean): string => {
  const v = sumFeeBy(row, pred)
  return v === 0 ? '' : String(v)
}
// 列表费用列展示账单标称金额（fee_amount），与账单概况「账单电费」一致
const sumBillFeeBy = (row: Row, pred: (it: any) => boolean): number =>
  Math.round(feeItemsOf(row).filter(pred).reduce((s, it) => s + (Number(it.fee_amount) || 0), 0) * 100) / 100
const billFeeTextOf = (row: Row, pred: (it: any) => boolean): string => {
  const v = sumBillFeeBy(row, pred)
  return v === 0 ? '' : String(v)
}
const cellText = (row: Row, key: string): string => {
  if (key === 'bill_period') {
    const raw = row.item?.bill_period_start || row.item?.bill_period_end
    if (!raw) return ''
    return String(raw).slice(0, 7)
  }
  if (key === 'market_amount') return billFeeTextOf(row, it => String(it.category || '').includes('市场化购电'))
  if (key === 'line_amount') return billFeeTextOf(row, it => String(it.category || '').includes('上网环节线损'))
  if (key === 'trans_amount') return billFeeTextOf(row, it => String(it.category || '').includes('输配电量'))
  if (key === 'sys_amount') return billFeeTextOf(row, it => String(it.category || '').includes('系统运行'))
  if (key === 'govI_amount') return billFeeTextOf(row, it => String(it.category || '').includes('政府性基金') && (Number(it.qty) || 0) >= 100000 && !String(it.name || '').includes('功率因数'))
  if (key === 'catalog_amount') return billFeeTextOf(row, it => String(it.category || '').includes('目录电费'))
  if (key === 'govR_amount') return billFeeTextOf(row, it => String(it.category || '').includes('政府性基金') && (Number(it.qty) || 0) < 100000 && !String(it.name || '').includes('功率因数'))
  if (key === 'pf_adjust_amount') {
    const direct = row.item?.[key]
    if (direct !== null && direct !== undefined && direct !== '') return String(direct)
    return billFeeTextOf(row, it => String(it.name || '').includes('功率因数'))
  }
  const v = row.item?.[key]
  if (v === null || v === undefined || v === '') return ''
  if (typeof v === 'number') {
    if (key.includes('price') || key === 'avg_price' || key === 'power_factor' || key === 'adjust_ratio') {
      return String(Math.round(v * 10000) / 10000)
    }
    return Number.isInteger(v) ? String(v) : String(Math.round(v * 100) / 100)
  }
  return String(v)
}

// ---- Tab 切换 ----
const onTabChange = () => { /* 子组件自行加载 */ }
const switchTab = (tab: 'electricity' | 'water' | 'gas') => {
  if (activeTab.value === tab) return
  activeTab.value = tab
  onTabChange()
}
// 设置变更后：重载字段配置并刷新列表
const onSettingsChanged = async () => {
  await loadFieldConfigs()
  loadFiles(true)
}

onMounted(() => { loadKb() })
onBeforeUnmount(() => { stopPolling() })
</script>

<style lang="less" scoped>
.utilities-management-container {
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

/* ---- 筛选工具栏（与合同管理一致） ---- */
.doc-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;

  &__leading {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1;
  }

  .doc-filter-field {
    display: flex;
    align-items: center;

    &--search {
      min-width: 220px;
    }

    &--wide {
      min-width: 260px;
    }

    .doc-search {
      width: 220px;
    }

    .doc-date-range {
      width: 260px;
    }
  }
}

/* ---- 字段筛选弹层（复用合同管理样式） ---- */
:global(.contract-field-popup) {
  padding: 0 !important;
}

.field-popup-content {
  width: 240px;
  padding: 12px;
  box-sizing: border-box;

  .field-popup-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;

    .field-popup-title {
      font-size: 13px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .field-popup-actions {
      display: flex;
      gap: 0;
    }
  }

  .field-popup-list {
    display: flex;
    flex-direction: column;
    max-height: 320px;
    overflow-y: auto;
  }
}

/* ---- 标签列（与合同管理一致） ---- */
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

.loading-area {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.utilities-main {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.utilities-layout {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 12px;
}

.utilities-sidebar {
  width: 112px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
}

.utilities-side-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  border-radius: 6px;
  font-size: 14px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: all 0.2s;
}

.utilities-side-item:hover {
  background: var(--td-bg-color-container-hover);
  color: var(--td-text-color-primary);
}

.utilities-side-item.active {
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-weight: 500;
}

.utilities-content {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.electricity-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 8px;
}

/* 底部汇总行（表格底部左侧） */
.doc-list-footer-summary {
  display: flex;
  gap: 24px;
  align-items: center;
  padding: 8px 16px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  border-radius: 0 0 9px 9px;

  &.with-toolbar {
    padding-bottom: 48px;
  }

  .summary-selected {
    color: var(--td-brand-color);
  }
}

.doc-list-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  border-radius: 9px;
}

/* 列表组件样式（与合同/发票/知识库列表保持一致，scoped 自包含） */
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.doc-list-group-header,
.doc-list-header,
.doc-list-row {
  display: grid;
  align-items: center;
  padding: 0 16px;
  min-width: 100%;
  box-sizing: border-box;
}

.doc-list-group-header {
  height: 28px;
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-component-stroke);

  .cell-group {
    justify-content: flex-start;
    gap: 6px;

    .group-label {
      color: var(--td-brand-color);
    }

    .group-count {
      font-size: 11px;
      font-weight: 400;
      color: var(--td-text-color-placeholder);
    }
  }
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

  &:hover:not(.selected) {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.selected {
    background: var(--td-brand-color-1);
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
  justify-content: flex-start;
}

.cell-extractStatus {
  justify-content: flex-start;
}

.cell-tags {
  justify-content: flex-start;
  gap: 4px;
  overflow: visible;
  white-space: nowrap;
}

.row-mono,
.row-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-mono {
  font-family: var(--app-font-family);
}

.row-status-tag {
  white-space: nowrap;
}

.row-tag {
  white-space: nowrap;
}

.doc-pending-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container-hover);

  .pending-name {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.list-loading {
  padding: 24px;
  display: flex;
  justify-content: center;
}

.list-empty {
  padding: 40px 0;
}

/* ---- 底部浮动工具栏（复刻合同管理模块） ---- */
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

.utility-detail-drawer {
  padding: 4px 0 24px;

  .detail-block {
    margin-bottom: 20px;

    .detail-block-title {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      padding: 10px 12px;
      border: 1px solid var(--td-component-stroke);
      border-radius: 6px 6px 0 0;
      background: var(--td-bg-color-container);
      cursor: pointer;

      .detail-block-caret {
        margin-left: auto;
        color: var(--td-text-color-secondary);
      }

      .fee-count {
        font-size: 12px;
        font-weight: 400;
        color: var(--td-text-color-secondary);
      }
    }

    .detail-block-content {
      padding: 12px;
      border: 1px solid var(--td-component-stroke);
      border-top: none;
      border-radius: 0 0 6px 6px;
      background: var(--td-bg-color-container);
    }
  }
}

.field-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 16px;

  .field-grid-item {
    display: flex;
    flex-direction: column;
    gap: 4px;

    .field-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}

.auto-save-tip {
  margin-top: 10px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.summary-lines {
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 1.6;
  white-space: pre-line;
  word-break: break-all;
}

.fee-table {
  .fee-row {
    display: grid;
    grid-template-columns: 1.2fr 1.6fr 0.9fr 0.8fr 0.8fr 1fr auto;
    gap: 6px;
    align-items: center;
    margin-bottom: 6px;

    &.fee-head {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      padding: 0 0 4px;
      border-bottom: 1px solid var(--td-component-stroke);
    }
  }
}

.fee-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 8px;

  .fee-total {
    font-size: 13px;
    color: var(--td-text-color-primary);
  }
}

/* 打印弹窗 */
.utility-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, .45);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
}

.utility-print-dialog {
  width: min(100%, 1100px);
  height: min(92vh, 860px);
  background: var(--td-bg-color-container);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.utility-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);

  .utility-print-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.utility-print-body {
  flex: 1;
  min-height: 0;
  background: var(--td-bg-color-container);

  .print-preview-frame {
    width: 100%;
    height: 100%;
    border: none;
    display: block;
  }

  .print-preview-loading {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
}

.utility-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--td-component-stroke);
}

.icon-spin {
  animation: t-spin 1s linear infinite;
}

@keyframes t-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ===== 电费账单详情视图（分层菜单） ===== */
.bill-detail-layout {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.bill-detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 2px 0 10px;

  .bill-detail-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 420px;
  }
}

.bill-detail-body {
  display: flex;
  flex: 1;
  min-height: 0;
  gap: 12px;
}

.bill-nav {
  width: 190px;
  flex-shrink: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 8px 6px;
  overflow-y: auto;
  background: var(--td-bg-color-container);

  .bill-nav-group-title {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    padding: 10px 10px 6px;
    font-weight: 600;
  }

  .bill-nav-item {
    padding: 8px 12px;
    border-radius: 6px;
    font-size: 13px;
    color: var(--td-text-color-primary);
    cursor: pointer;
    margin-bottom: 2px;
    transition: background-color .15s, color .15s;

    &:hover {
      background: var(--td-bg-color-container-hover);
    }

    &.active {
      background: var(--td-brand-color-light);
      color: var(--td-brand-color);
      font-weight: 600;
    }

    &--child {
      padding-left: 24px;
      font-size: 13px;
    }
  }
}

.bill-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  padding: 16px 16px 0;

  /* 卡片间间距统一用 margin-top，最后一张卡贴底，与左侧菜单底边线对齐 */
  > * + * {
    margin-top: 16px;
  }
}

.bill-card {
  margin-bottom: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 14px 16px;
  background: var(--td-bg-color-container);
  transition: box-shadow .2s, border-color .2s;

  &:hover {
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
    border-color: var(--td-brand-color);
  }

  .bill-card-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 10px;

    .bill-card-title {
      font-size: 15px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .bill-card-hint {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      margin-left: auto;
    }
  }
}

/* 概览页指标卡 */
.overview-metrics {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 8px;

  .metric-card {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 12px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-container);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
    transition: box-shadow .2s, border-color .2s;

    &:hover {
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
      border-color: var(--td-brand-color);
    }

    .metric-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }

    .metric-value {
      font-size: 20px;
      font-weight: 600;
      color: var(--td-brand-color);
      font-variant-numeric: tabular-nums;
      line-height: 1.2;

      .metric-unit {
        font-size: 13px;
        font-weight: 400;
        color: var(--td-text-color-secondary);
        margin-left: 6px;
      }

      &--date {
        font-size: 16px;
        color: var(--td-text-color-primary);
      }
    }
  }
}

/* 账单概况 + 用能分析 一行两卡片（等高；组件自然紧凑排列，底部允许留白） */
.overview-two-col {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: stretch;
  flex: 1;
  min-height: 0;

  > .bill-card {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  > .bill-card:nth-child(2) {
    /* 组件间保持自然间距，不强行撑开 */
    .ea-chart {
      height: 210px;
      margin-bottom: 14px;
    }
  }
}

/* 用能分析 */
.ea-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-bottom: 10px;

    .ea-item {
      display: flex;
      flex-direction: column;
      gap: 2px;
      padding: 6px 10px;
      border: 1px solid var(--td-component-stroke);
      border-radius: 8px;
      background: var(--td-bg-color-container);
      transition: box-shadow .2s, border-color .2s;

      &:hover {
        box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
        border-color: var(--td-brand-color);
      }

      .ea-label {
        font-size: 12px;
        color: var(--td-text-color-secondary);
      }

      .ea-value {
        font-size: 14px;
        color: var(--td-text-color-primary);
        font-variant-numeric: tabular-nums;
      }

      .ea-delta {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        font-size: 12px;
        font-variant-numeric: tabular-nums;
        min-height: 16px;

        .ea-arrow { font-size: 13px; line-height: 1; }
      }

      .ea-delta--up { color: var(--td-error-color); }
      .ea-delta--down { color: var(--td-success-color); }
      .ea-delta--flat { color: var(--td-text-color-secondary); }
    }
  }

  .ea-chart {
    padding: 6px 10px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    margin-bottom: 10px;

    .ea-echart {
      width: 100%;
      height: 100%;
    }
  }

  /* 分时占比对比表 */
  .ea-compare {
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    overflow: hidden;

    .ea-compare-row {
      display: grid;
      grid-template-columns: 0.7fr 1.15fr 1fr 1.15fr 1fr 1fr;
      padding: 5px 12px;
      font-size: 12px;
      white-space: nowrap;
      border-bottom: 1px solid var(--td-component-stroke);

      &:last-child { border-bottom: none; }
    }

    .ea-compare-head {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
      font-weight: 500;
      font-size: 11px;
    }

    .ea-c-label { font-weight: 500; color: var(--td-text-color-primary); }
    .ea-c-num { text-align: right; font-variant-numeric: tabular-nums; }
    .ea-c-up { color: var(--td-error-color); }
    .ea-c-down { color: var(--td-success-color); }
  }

/* 账单概况表（卡片较窄，横向滚动保证数据完整） */
.os-table {
  display: flex;
  flex-direction: column;
  min-width: 440px;
  overflow-x: auto;

  .os-row {
    display: grid;
    grid-template-columns: 1fr 90px 80px 80px 60px;
    gap: 8px;
    padding: 5px 12px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 12px;
    align-items: center;

      &:last-child { border-bottom: none; }

      &--head {
        font-size: 12px;
        color: var(--td-text-color-secondary);
        background: var(--td-bg-color-container-hover);
      }

      &--link {
        cursor: pointer;
        transition: background-color .15s;

        &:hover {
          background: var(--td-bg-color-container-hover);

          .os-name { color: var(--td-brand-color); }
        }
      }

      &--total {
        background: var(--td-brand-color-light);
        font-weight: 600;

        .os-name { color: var(--td-brand-color); }
      }

      .os-name { color: var(--td-text-color-primary); }
      .os-qty { font-size: 12px; color: var(--td-text-color-secondary); text-align: right; font-variant-numeric: tabular-nums; }
      .os-amount { text-align: right; font-variant-numeric: tabular-nums; }
      .os-state { text-align: center; }
      .os-desc { font-size: 12px; color: var(--td-text-color-secondary); }

      .os-neg { color: var(--td-error-color); }
      .os-diff { margin-left: 6px; }
      .os-fee-val { cursor: text; border-radius: 4px; padding: 1px 4px; margin-right: -4px; &:hover { background: var(--td-bg-color-container-hover); } }
      .os-fee-val--manual { color: var(--td-brand-color); text-decoration: underline dashed 1px; text-underline-offset: 3px; }
      .os-fee-input { width: 96px; text-align: right; }
  }
}

/* 电量明细 */
.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow-x: auto;

  .meter-row {
    display: grid;
    grid-template-columns: 1.2fr repeat(8, minmax(0, 0.75fr));
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 13px;
    min-width: 900px;
    cursor: pointer;
    transition: background-color .15s;

    &:last-child { border-bottom: none; }

    &:hover { background: var(--td-bg-color-container-hover); }

    &.meter-head {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
      font-size: 12px;
      cursor: default;
    }

    &.meter-total {
      background: var(--td-brand-color-light);
      font-weight: 600;
      cursor: default;
    }
  }
}

/* 费用子项表格 */
.fee-group-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;

  .fg-row {
    display: grid;
    grid-template-columns: 2fr 1fr 1.2fr 1.2fr 1.2fr;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 13px;
    align-items: center;
    cursor: pointer;
    transition: background-color .15s;

    &:last-child { border-bottom: none; }

    &:hover { background: var(--td-bg-color-container-hover); }

    &.fg-head {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
      font-size: 12px;
      cursor: default;
    }

    &.fg-total {
      background: var(--td-brand-color-light);
      font-weight: 600;
      cursor: default;
    }

    .fg-name {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .fg-empty {
    padding: 24px;
    text-align: center;
    color: var(--td-text-color-placeholder);
    font-size: 13px;
  }
}

/* 明细列表（居民电量/容需量/功率因素） */
.detail-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow-x: auto;

  .fg-row {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 13px;
    align-items: center;
    min-width: 620px;
    cursor: pointer;
    transition: background-color .15s;

    &:last-child { border-bottom: none; }

    &:hover { background: var(--td-bg-color-container-hover); }

    &.fg-head {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
      font-size: 12px;
      cursor: default;
      white-space: nowrap;
    }

    &.fg-total {
      background: var(--td-brand-color-light);
      font-weight: 600;
      cursor: default;
    }

    .fg-name {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  &.cols-8 .fg-row {
    grid-template-columns: repeat(8, minmax(0, 1fr));
    min-width: 1120px;
  }

  .fg-empty {
    padding: 24px;
    text-align: center;
    color: var(--td-text-color-placeholder);
    font-size: 13px;
  }
}

/* 容需量 / 功率因数 */
.capacity-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;

  .cap-item {
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 6px;

    .cap-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }

    .cap-value {
      font-size: 15px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      font-variant-numeric: tabular-nums;
    }
  }
}

/* 编辑抽屉 */
.edit-drawer-body {
  padding: 8px 2px 24px;
}

.edit-field {
  margin-bottom: 16px;

  .edit-label {
    display: block;
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin-bottom: 6px;
  }

  .edit-note {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    line-height: 1.5;
  }
}
</style>
