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
                <!-- 进行中的文件状态行（解析中/提取中/待提取） -->
                <div v-for="pf in pendingFiles" :key="pf.id" class="doc-pending-row">
                  <t-tag size="small" theme="warning" variant="light-outline" class="row-status-tag">
                    <template #icon><t-icon name="loading" class="icon-spin" /></template>
                    {{ pendingLabel(pf) }}
                  </t-tag>
                  <span class="pending-name">{{ pf.file_name }}</span>
                </div>
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
                  <div v-for="row in rows" :key="row.rowKey" class="doc-list-row" :style="gridStyle"
                    :class="{ selected: selectedRowKeys.includes(row.rowKey) }" role="row" @click="openDetail(row)">
                    <div class="cell cell-check" @click.stop>
                      <t-checkbox class="doc-list-check" size="small" :checked="selectedRowKeys.includes(row.rowKey)"
                        :disabled="row.kind === 'pending'" @change="(c: boolean) => toggleRow(row.rowKey, c)" />
                    </div>
                    <template v-for="col in visibleColDefs" :key="col.key">
                      <div class="cell" :class="`cell-${col.key}`">
                        <span v-if="col.fieldType === 'number' || col.fieldType === 'amount'" class="row-mono" :title="cellText(row, col.key)">
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
                      <span v-else class="row-tag-chips is-clickable" @click="openTagEdit(row)">
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
                <!-- 底部汇总（表格底部左侧） -->
                <div v-if="summary.total" class="doc-list-footer-summary" :class="{ 'with-toolbar': selectedRowKeys.length }">
                  <span>共 {{ summary.total }} 条</span>
                  <span>本期电量 {{ fmtKwh(summaryUsage) }} 千瓦时</span>
                  <span>本期电费 {{ fmtMoney(summaryAmount) }} 元</span>
                  <span v-if="selectedRowKeys.length" class="summary-selected">已选 {{ selectedRowKeys.length }} 条</span>
                </div>
              </div>
            </div>

            <!-- 浮动工具栏（选中时显示，自动避让汇总行） -->
            <div v-if="selectedRowKeys.length" class="floating-toolbar">
              <t-button variant="outline" size="small" @click="handleBatchEdit">
                <template #icon><t-icon name="edit-1" size="14px" /></template>
                编辑
              </t-button>
              <t-button variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handleReExtract">
                <template #icon><t-icon name="refresh" size="14px" /></template>
                重新提取
              </t-button>
              <t-button variant="outline" size="small" @click="handleBatchPrint">
                <template #icon><t-icon name="print" size="14px" /></template>
                打印
              </t-button>
              <t-button variant="outline" size="small" @click="handleBatchDelete">
                <template #icon><t-icon name="delete" size="14px" /></template>
                删除
              </t-button>
            </div>
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
                  <!-- 账单概况（概览页：基础信息 / 本期电量 / 本期电费 / 缴费截止 / 账单概况 / 用能分析） -->
                  <template v-if="activeMenu === 'overview'">
                    <!-- 1 基础信息 -->
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">基础信息</span>
                      </div>
                      <div class="overview-static">
                        <div class="ov-item" v-for="f in BASIC_INFO_FIELDS" :key="f.key">
                          <span class="ov-value" :title="String(basicInfo[f.key] ?? '')">{{ basicInfo[f.key] ?? '—' }}</span>
                          <span class="ov-label">{{ f.label }}</span>
                        </div>
                      </div>
                    </div>

                    <!-- 2-5 四大数据卡片 -->
                    <div class="overview-metrics">
                      <div class="metric-card">
                        <div class="metric-label">本期电量</div>
                        <div class="metric-value">{{ fmtKwh(metricKwh) }}<span class="metric-unit">千瓦时</span></div>
                      </div>
                      <div class="metric-card">
                        <div class="metric-label">本期电费</div>
                        <div class="metric-value">{{ fmtMoney(metricFee) }}<span class="metric-unit">元</span></div>
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
                      </div>
                      <div class="overview-summary">
                        <div class="os-table">
                          <div class="os-row os-row--head">
                            <span>项目</span><span>金额（元）</span><span>说明</span>
                          </div>
                          <div class="os-row" v-for="r in overviewRows" :key="r.key">
                            <span class="os-name">{{ r.label }}</span>
                            <span class="os-amount" :class="{ 'os-neg': r.value < 0 }">{{ fmtMoney(r.value) }}</span>
                            <span class="os-desc">{{ r.desc }}</span>
                          </div>
                          <div class="os-row os-row--total">
                            <span>本期电费</span>
                            <span class="os-amount" :class="{ 'os-neg': overviewTotal < 0 }">{{ fmtMoney(overviewTotal) }}</span>
                            <span class="os-desc">
                              账单标称 {{ fmtMoney(Number(editForm.total_amount) || 0) }}
                              <t-tag v-if="overviewDiff" size="small" theme="warning" variant="light" class="os-diff">
                                差异 {{ fmtMoney(overviewDiff) }}
                              </t-tag>
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>

                    <!-- 6 用能分析 -->
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">用能分析</span>
                        <span v-if="!prevEnergy.exists" class="bill-card-hint">暂无上期账单数据</span>
                      </div>
                      <div class="energy-analysis">
                        <div class="ea-grid">
                          <div class="ea-item">
                            <span class="ea-label">本期电量环比</span>
                            <strong class="ea-value">{{ momText }}</strong>
                          </div>
                          <div class="ea-item">
                            <span class="ea-label">功率因数</span>
                            <strong class="ea-value">{{ pfText }}</strong>
                          </div>
                          <div class="ea-item">
                            <span class="ea-label">平均电价</span>
                            <strong class="ea-value">{{ avgPriceText }}</strong>
                          </div>
                        </div>
                        <div class="ea-chart">
                          <div ref="energyChartRef" class="ea-echart"></div>
                        </div>
                        <!-- 分时电量占比对比 -->
                        <div class="ea-compare">
                          <div class="ea-compare-row ea-compare-head">
                            <span>时段</span><span>本期电量</span><span>本期占比</span>
                            <span>上期电量</span><span>上期占比</span><span>占比变化</span>
                          </div>
                          <div class="ea-compare-row" v-for="r in energyCompareRows" :key="r.key">
                            <span class="ea-c-label">{{ r.label }}</span>
                            <span class="ea-c-num">{{ fmtKwh(r.cur) }}</span>
                            <span class="ea-c-num">{{ r.curPct }}%</span>
                            <span class="ea-c-num">{{ prevEnergy.exists ? fmtKwh(r.prev) : '—' }}</span>
                            <span class="ea-c-num">{{ prevEnergy.exists ? r.prevPct + '%' : '—' }}</span>
                            <span class="ea-c-num" :class="{ 'ea-c-up': r.delta > 0, 'ea-c-down': r.delta < 0 }">
                              {{ prevEnergy.exists ? (r.delta > 0 ? '+' : '') + r.delta + '%' : '—' }}
                            </span>
                          </div>
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
                        <span class="bill-card-hint">点击行可在抽屉编辑电量，自动保存</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>时段</span><span>计费电量（千瓦时）</span><span>占比</span>
                        </div>
                        <div v-for="(r, i) in industrialMeterRows" :key="r.key" class="meter-row" @click="openMeterEdit(i, 'industrial')">
                          <span>{{ r.label }}</span><span class="row-mono">{{ fmtKwh(r.value) }}</span>
                          <span class="row-mono">{{ meterPct(r.value, industrialMeterTotal) }}</span>
                        </div>
                        <div class="meter-row meter-total">
                          <span>合计</span><span class="row-mono">{{ fmtKwh(industrialMeterTotal) }}</span><span>100%</span>
                        </div>
                      </div>
                    </div>
                  </template>
                  <!-- 居民电量明细 -->
                  <template v-else-if="activeMenu === 'residential-meter'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">电量明细 · 居民</span>
                        <span class="bill-card-hint">账单仅提供居民目录电量合计</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>项目</span><span>计费电量（千瓦时）</span><span>占比</span>
                        </div>
                        <div v-for="(r, i) in residentialMeterRows" :key="r.key" class="meter-row" @click="openMeterEdit(i, 'residential')">
                          <span>{{ r.label }}</span><span class="row-mono">{{ fmtKwh(r.value) }}</span>
                          <span class="row-mono">100%</span>
                        </div>
                        <div class="meter-row meter-total">
                          <span>合计</span><span class="row-mono">{{ fmtKwh(residentialMeterTotal) }}</span><span>100%</span>
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
                          <span>费用组成</span><span>时段</span><span>计费电量</span><span>计费标准</span><span>电费（元）</span>
                        </div>
                        <div v-for="(it, i) in feeRowsOf(activeMenu)" :key="i" class="fg-row" @click="openFeeItemEdit(activeMenu, i)">
                          <span class="fg-name" :title="it.name">{{ it.name }}</span>
                          <span>{{ it.period || '--' }}</span>
                          <span class="row-mono">{{ it.qty ? fmtKwh(it.qty) : '' }}</span>
                          <span class="row-mono">{{ it.rate ? fmtRate(it.rate) : '' }}</span>
                          <span class="row-mono" :class="{ 'os-neg': it.fee < 0 }">{{ it.fee || it.fee === 0 ? fmtMoney(it.fee) : '' }}</span>
                        </div>
                        <div v-if="!feeRowsOf(activeMenu).length" class="fg-empty">暂无数据</div>
                        <div v-if="feeRowsOf(activeMenu).length" class="fg-row fg-total">
                          <span>小计</span><span></span><span></span><span></span>
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
                        <t-button variant="outline" size="small" @click="openStaticEdit('capacity')">
                          <template #icon><t-icon name="edit-1" size="14px" /></template>
                          编辑
                        </t-button>
                      </div>
                      <div class="capacity-grid">
                        <div class="cap-item" v-for="f in CAPACITY_FIELDS" :key="f.key">
                          <span class="cap-label">{{ f.label }}</span>
                          <span class="cap-value">{{ fmtField(editForm[f.key], f.type) }}</span>
                        </div>
                      </div>
                    </div>
                  </template>

                  <!-- 功率因素调整电费 -->
                  <template v-else-if="activeMenu === 'pf-adjust'">
                    <div class="bill-card">
                      <div class="bill-card-head">
                        <span class="bill-card-title">功率因素调整电费</span>
                        <t-button variant="outline" size="small" @click="openStaticEdit('pf')">
                          <template #icon><t-icon name="edit-1" size="14px" /></template>
                          编辑
                        </t-button>
                      </div>
                      <div class="capacity-grid">
                        <div class="cap-item" v-for="f in PF_FIELDS" :key="f.key">
                          <span class="cap-label">{{ f.label }}</span>
                          <span class="cap-value">{{ fmtField(editForm[f.key], f.type) }}</span>
                        </div>
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
  getUtilityBasicInfo,
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
  { key: 'bill_period', label: '账单周期', fieldType: 'text', default: true, w: '1.2fr' },
  { key: 'total_kwh', label: '本期电量', fieldType: 'number', default: true, w: '0.9fr' },
  { key: 'total_amount', label: '本期电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'pf_adjust_amount', label: '力调电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'capacity_fee', label: '基本电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'market_amount', label: '购电电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'line_amount', label: '线损费用', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'trans_amount', label: '输配电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'sys_amount', label: '系统运行费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'govI_amount', label: '附加费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'catalog_amount', label: '目录电费（居民）', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'govR_amount', label: '附加费（居民）', fieldType: 'amount', default: true, w: '1fr' },
  // 详细字段
  { key: 'account_no', label: '户号', fieldType: 'text', default: false, w: '1fr' },
  { key: 'account_name', label: '户名', fieldType: 'text', default: false, w: '1.4fr' },
  { key: 'usage_category', label: '用电类别', fieldType: 'text', default: false, w: '1fr' },
  { key: 'voltage_level', label: '电压等级', fieldType: 'text', default: false, w: '0.9fr' },
  { key: 'avg_price', label: '平均电价', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'power_factor', label: '功率因素', fieldType: 'number', default: false, w: '0.8fr' },
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
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')} 1fr 1.2fr`,
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

const loadFieldConfigs = async () => {
  try {
    const res: any = await listUtilityFieldConfigs('electricity')
    const list = res?.data || res
    if (Array.isArray(list) && list.length) {
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
  if (['account_name'].includes(key)) return '1.6fr'
  if (['bill_period'].includes(key)) return '1.2fr'
  return '1fr'
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
const selectedRows = computed(() => rows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)))
const selectableRows = computed(() => rows.value.filter(r => r.kind !== 'pending'))
const isAllSelected = computed(() => selectableRows.value.length > 0 && selectableRows.value.every(r => selectedRowKeys.value.includes(r.rowKey)))
const someSelected = computed(() => selectedRowKeys.value.length > 0 && !isAllSelected.value)

function toggleSelectAll(checked: boolean) {
  selectedRowKeys.value = checked ? selectableRows.value.map(r => r.rowKey) : []
}
function toggleRow(key: string, checked: boolean) {
  if (checked) { if (!selectedRowKeys.value.includes(key)) selectedRowKeys.value.push(key) }
  else selectedRowKeys.value = selectedRowKeys.value.filter(k => k !== key)
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

// ---- 状态行 ----
const STATUS_MAP: Record<string, { label: string; theme: any; icon?: string; spin?: boolean }> = {
  parsing: { label: '解析中', theme: 'warning', icon: 'loading', spin: true },
  extracting: { label: '提取中', theme: 'warning', icon: 'loading', spin: true },
  pending: { label: '待提取', theme: 'default' },
  success: { label: '提取完成', theme: 'success', icon: 'check-circle' },
  manual: { label: '待补录', theme: 'warning', icon: 'edit-1' },
  failed: { label: '提取失败', theme: 'danger', icon: 'error-circle' },
}
const statusOf = (row: Row) => {
  const st = row.extractStatus || ''
  if (STATUS_MAP[st]) return STATUS_MAP[st]
  return { label: '--', theme: 'default' as const }
}
const pendingLabel = (pf: any) => {
  const ps = pf.parse_status || ''
  if (ps === 'parsing' || ps === 'pending') return '解析中'
  const meta = pf.custom_metadata || {}
  if (meta.extract_status === 'extracting' || pf.extract_status === 'extracting') return '提取中'
  return '待提取'
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
      // 进行中文件（解析中/提取中/待提取）
      pendingFiles.value = arr.filter((it: any) => {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        return ps === 'parsing' || ps === 'pending' ||
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
  }, 4000)
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
      const res: any = await uploadKnowledgeFile(kbId.value, f)
      const knowledge = res?.data || res
      const kid = knowledge?.id || knowledge?.knowledge_id
      MessagePlugin.success(`已上传 ${f.name}，等待解析`)
      if (kid) {
        // 解析完成后轮询自动提取
        extractingSet.add(kid)
      }
    } catch (e2: any) {
      MessagePlugin.error(`${f.name} 上传失败：${e2?.message || ''}`)
    }
  }
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
  loadBasicInfo()
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
    editForm.value = { ...item, fee_items: Array.isArray(item.fee_items) ? item.fee_items.map((f: any) => ({ ...f })) : [] }
    editFormSnapshot = JSON.stringify(editForm.value)
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  } catch {
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
const feeMenuOf = (key: string) => key in FEE_MENU_MAP
const feeRowsOf = (key: string) => (editForm.value.fee_items || []).filter(FEE_MENU_MAP[key])
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

// 概况静态字段
// 基本户信息（概览基础信息直接读取，设置中维护，概览不可编辑）
const BASIC_INFO_FIELDS = [
  { key: 'account_no', label: '户号' },
  { key: 'account_name', label: '户名' },
  { key: 'usage_category', label: '用电类别' },
  { key: 'voltage_level', label: '电压等级' },
  { key: 'market_attr', label: '市场化属性' },
  { key: 'supply_unit', label: '供电服务单位' },
  { key: 'address', label: '用电地址' },
]
const OVERVIEW_STATIC = BASIC_INFO_FIELDS.map(f => ({ ...f, type: 'text' }))
const basicInfo = ref<Record<string, string>>({})
const loadBasicInfo = async () => {
  try {
    const res: any = await getUtilityBasicInfo('electricity')
    const d = res?.data || {}
    basicInfo.value = {
      account_no: d.account_no || '', account_name: d.account_name || '',
      usage_category: d.usage_category || '', voltage_level: d.voltage_level || '',
      market_attr: d.market_attr || '', supply_unit: d.supply_unit || '', address: d.address || '',
    }
  } catch { /* 未配置时留空 */ }
}

// 容需量字段
const CAPACITY_FIELDS = [
  { key: 'capacity', label: '合同容量（kVA）', type: 'number' },
  { key: 'demand', label: '需量（kW）', type: 'number' },
  { key: 'capacity_price', label: '容量电价（元/kVA）', type: 'number' },
  { key: 'capacity_fee', label: '输配容量电费（元）', type: 'number' },
]

// 功率因数字段
const PF_FIELDS = [
  { key: 'power_factor', label: '功率因数', type: 'number' },
  { key: 'pf_standard', label: '考核标准', type: 'number' },
  { key: 'adjust_coefficient', label: '调整系数', type: 'number' },
  { key: 'pf_adjust_amount', label: '调整电费（元）', type: 'number' },
]

const sumFee = (arr: any[]) => Math.round(arr.reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0) * 100) / 100

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
  const industrial = Math.round((market + line + trans + sys + govI) * 100) / 100
  const residential = Math.round((catalog + govR) * 100) / 100
  return [
    { key: 'market', label: '市场化购电费', value: market, desc: `${feeRowsOf('industrial-market').length} 项` },
    { key: 'line', label: '上网环节线损费', value: line, desc: `${feeRowsOf('industrial-line').length} 项` },
    { key: 'trans', label: '输配电量电费', value: trans, desc: `${feeRowsOf('industrial-trans').length} 项` },
    { key: 'sys', label: '系统运行费', value: sys, desc: `${feeRowsOf('industrial-sys').length} 项` },
    { key: 'govI', label: '政府基金及附加（工商业）', value: govI, desc: `${feeRowsOf('industrial-gov').length} 项` },
    { key: 'industrial', label: '工商业电费小计', value: industrial, desc: '上述五项之和' },
    { key: 'catalog', label: '目录电费（居民）', value: catalog, desc: `${feeRowsOf('residential-catalog').length} 项` },
    { key: 'govR', label: '政府性基金及附加（居民）', value: govR, desc: `${feeRowsOf('residential-gov').length} 项` },
    { key: 'residential', label: '居民电费小计', value: residential, desc: '上述两项之和' },
    { key: 'capacity', label: '输配容（需）量电费', value: capacity, desc: '容量 × 容量电价' },
    { key: 'pf', label: '功率因数调整电费', value: pf, desc: '账单调整值' },
  ]
})
const overviewTotal = computed(() => {
  const industrial = overviewRows.value.find(r => r.key === 'industrial')?.value || 0
  const residential = overviewRows.value.find(r => r.key === 'residential')?.value || 0
  const capacity = overviewRows.value.find(r => r.key === 'capacity')?.value || 0
  const pf = overviewRows.value.find(r => r.key === 'pf')?.value || 0
  return Math.round((industrial + residential + capacity + pf) * 100) / 100
})
const overviewDiff = computed(() => {
  const nominal = Number(editForm.value.total_amount) || 0
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
const ovText = (f: { key: string; label: string }) => {
  const v = editForm.value[f.key]
  if (v === null || v === undefined || v === '') return ''
  if (f.key === 'bill_period_start' || f.key === 'bill_period_end') return String(v).slice(0, 7)
  return String(v)
}
// 用能分析
const momText = computed(() => {
  const prev = Number(editForm.value.prev_kwh) || 0
  const cur = Number(editForm.value.total_kwh) || 0
  if (!prev || !cur) return '—'
  const pct = Math.round(((cur - prev) / prev) * 1000) / 10
  return `${pct > 0 ? '+' : ''}${pct}%`
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
  }
})

// 分时占比对比表
const energyCompareRows = computed(() => {
  const curTotal = metricKwh.value
  const prevTotal = prevEnergy.value.total
  const pct = (v: number, total: number) => (total ? Math.round((v / total) * 1000) / 10 : 0)
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
  return cur.map(c => {
    const pv = prev[c.key] || 0
    const curPct = pct(c.v, curTotal)
    const prevPct = pct(pv, prevTotal)
    return {
      key: c.key, label: c.label,
      cur: c.v, curPct,
      prev: pv, prevPct,
      delta: prevEnergy.value.exists ? Math.round((curPct - prevPct) * 10) / 10 : 0,
    }
  })
})

// ---- 用能分析 ECharts（分时电量占比对比：本期 vs 上期） ----
const energyChartRef = ref<HTMLDivElement | null>(null)
let energyChart: echarts.ECharts | null = null
const renderEnergyChart = async () => {
  await nextTick()
  const el = energyChartRef.value
  if (!el) return
  if (!energyChart) energyChart = echarts.init(el)
  const rows = energyCompareRows.value
  const prevExists = prevEnergy.value.exists
  const pctLabel = (p: any) => `${p.value}%`
  energyChart.setOption({
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
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
      axisLabel: { color: '#57606a', fontSize: 12, formatter: '{value}%' },
      splitLine: { lineStyle: { color: '#eaeef2' } },
    },
    series: [
      {
        name: '本期占比', type: 'bar', data: rows.map(r => r.curPct), barWidth: 22,
        itemStyle: { color: '#0052d9', borderRadius: [3, 3, 0, 0] },
        label: { show: true, position: 'top', color: '#57606a', fontSize: 10, formatter: pctLabel },
      },
      {
        name: '上期占比', type: 'bar', data: rows.map(r => (prevExists ? r.prevPct : null)), barWidth: 22,
        itemStyle: { color: '#9ab6e8', borderRadius: [3, 3, 0, 0] },
        label: { show: prevExists, position: 'top', color: '#57606a', fontSize: 10, formatter: pctLabel },
      },
    ],
  })
}
watch([detailMode, activeMenu, () => currentRow.value, energyCompareRows], () => {
  if (detailMode.value && activeMenu.value === 'overview') renderEnergyChart()
})
onBeforeUnmount(() => { energyChart?.dispose(); energyChart = null })

// 电量明细
const industrialMeterRows = computed(() => [
  { key: 'deep_peak_kwh', label: '尖峰', value: Number(editForm.value.deep_peak_kwh) || 0 },
  { key: 'peak_kwh', label: '峰', value: Number(editForm.value.peak_kwh) || 0 },
  { key: 'flat_kwh', label: '平', value: Number(editForm.value.flat_kwh) || 0 },
  { key: 'valley_kwh', label: '谷', value: Number(editForm.value.valley_kwh) || 0 },
])
const industrialMeterTotal = computed(() => industrialMeterRows.value.reduce((s, r) => s + r.value, 0))
const residentialMeterRows = computed(() => [
  { key: 'residential_kwh', label: '目录电量', value: Number(editForm.value.residential_kwh) || 0 },
])
const residentialMeterTotal = computed(() => residentialMeterRows.value.reduce((s, r) => s + r.value, 0))
const meterPct = (v: number, total: number) => (total ? `${(v / total * 100).toFixed(1)}%` : '--')

// 电量明细编辑
const meterEditVisible = ref(false)
const meterEditForm = ref<Record<string, any>>({})
const meterEditTitle = ref('')
const meterEditFields = ref<{ key: string; label: string }[]>([])
const openMeterEdit = (idx: number, group: 'industrial' | 'residential') => {
  if (group === 'industrial') {
    const r = industrialMeterRows.value[idx]
    meterEditTitle.value = `工商业 · ${r.label}时段`
    meterEditFields.value = [{ key: r.key, label: `${r.label}时段电量（千瓦时）` }]
    meterEditForm.value = { [r.key]: Number(editForm.value[r.key]) || 0 }
  } else {
    const r = residentialMeterRows.value[idx]
    meterEditTitle.value = `居民 · ${r.label}`
    meterEditFields.value = [{ key: r.key, label: `${r.label}（千瓦时）` }]
    meterEditForm.value = { [r.key]: Number(editForm.value[r.key]) || 0 }
  }
  meterEditVisible.value = true
}
watch(meterEditForm, () => {
  if (!meterEditVisible.value || !autoSaveDirty) return
  const k = meterEditFields.value[0]?.key
  if (!k) return
  editForm.value[k] = Number(meterEditForm.value[k]) || 0
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
const openStaticEdit = (scope: 'overview' | 'capacity' | 'pf') => {
  if (scope === 'overview') {
    staticEditTitle.value = '编辑账单概况'
    staticEditFields.value = OVERVIEW_STATIC
  } else if (scope === 'capacity') {
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
    updated.remark = editForm.value.remark || ''
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
  extractInFlight.value.add(row[0].knowledgeId)
  extractFailed.value.delete(row[0].knowledgeId)
  try {
    await reparseKnowledge(row[0].knowledgeId)
    MessagePlugin.success(`已触发「${row[0].fileName}」重新解析与提取`)
    setTimeout(() => loadFiles(true), 1500)
  } catch (e: any) {
    extractFailed.value.add(row[0].knowledgeId)
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(row[0].knowledgeId)
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
  const confirm = await MessagePlugin.confirm(`确认删除选中的 ${rowsSel.length} 条账单记录？源文件将移入删除历史。`, {
    confirmBtn: '删除', cancelBtn: '取消',
  })
  if (!confirm) return
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
const feeItemsOf = (row: Row): any[] => (Array.isArray(row.item?.fee_items) ? row.item.fee_items : [])
const sumFeeBy = (row: Row, pred: (it: any) => boolean): number =>
  Math.round(feeItemsOf(row).filter(pred).reduce((s, it) => s + (Number(it.fee) || 0), 0) * 100) / 100
const feeTextOf = (row: Row, pred: (it: any) => boolean): string => {
  const v = sumFeeBy(row, pred)
  return v === 0 ? '' : String(v)
}
const cellText = (row: Row, key: string): string => {
  if (key === 'bill_period') {
    const raw = row.item?.bill_period_start || row.item?.bill_period_end
    if (!raw) return ''
    return String(raw).slice(0, 7)
  }
  if (key === 'market_amount') return feeTextOf(row, it => String(it.category || '').includes('市场化购电'))
  if (key === 'line_amount') return feeTextOf(row, it => String(it.category || '').includes('上网环节线损'))
  if (key === 'trans_amount') return feeTextOf(row, it => String(it.category || '').includes('输配电量'))
  if (key === 'sys_amount') return feeTextOf(row, it => String(it.category || '').includes('系统运行'))
  if (key === 'govI_amount') return feeTextOf(row, it => String(it.category || '').includes('政府性基金') && (Number(it.qty) || 0) >= 100000 && !String(it.name || '').includes('功率因数'))
  if (key === 'catalog_amount') return feeTextOf(row, it => String(it.category || '').includes('目录电费'))
  if (key === 'govR_amount') return feeTextOf(row, it => String(it.category || '').includes('政府性基金') && (Number(it.qty) || 0) < 100000 && !String(it.name || '').includes('功率因数'))
  if (key === 'pf_adjust_amount') {
    const direct = row.item?.[key]
    if (direct !== null && direct !== undefined && direct !== '') return String(direct)
    return feeTextOf(row, it => String(it.name || '').includes('功率因数'))
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

.floating-toolbar {
  position: absolute;
  right: 24px;
  bottom: 24px;
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, .12);
  z-index: 20;
}

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
  height: calc(100vh - 168px);
  min-height: 420px;
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
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  padding: 16px;
}

.bill-card {
  margin-bottom: 16px;

  .bill-card-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 14px;

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

/* 概况 */
.overview-static {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px 24px;
  padding: 8px 4px;

  .ov-item {
    display: flex;
    flex-direction: column;
    gap: 5px;
    min-width: 0;

    .ov-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      order: 2;
    }

    .ov-value {
      font-size: 15px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      order: 1;
    }
  }
}

/* 概览页指标卡 */
.overview-metrics {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 12px;

  .metric-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 16px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-container);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);

    .metric-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }

    .metric-value {
      font-size: 26px;
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
        font-size: 18px;
        color: var(--td-text-color-primary);
      }
    }
  }
}

/* 账单概况 + 用能分析 一行两卡片 */
.overview-two-col {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

/* 用能分析 */
.energy-analysis {
  .ea-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 16px;

    .ea-item {
      display: flex;
      flex-direction: column;
      gap: 6px;
      padding: 12px 14px;
      border: 1px solid var(--td-component-stroke);
      border-radius: 8px;
      background: var(--td-bg-color-container);

      .ea-label {
        font-size: 12px;
        color: var(--td-text-color-secondary);
      }

      .ea-value {
        font-size: 16px;
        color: var(--td-text-color-primary);
        font-variant-numeric: tabular-nums;
      }
    }
  }

  .ea-chart {
    height: 220px;
    padding: 8px 10px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    margin-bottom: 14px;

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
      grid-template-columns: 1fr 1fr 1fr 1fr 1fr 1fr;
      padding: 8px 14px;
      font-size: 12px;
      border-bottom: 1px solid var(--td-component-stroke);

      &:last-child { border-bottom: none; }
    }

    .ea-compare-head {
      background: var(--td-bg-color-container-hover);
      color: var(--td-text-color-secondary);
      font-weight: 500;
    }

    .ea-c-label { font-weight: 500; color: var(--td-text-color-primary); }
    .ea-c-num { text-align: right; font-variant-numeric: tabular-nums; }
    .ea-c-up { color: var(--td-error-color); }
    .ea-c-down { color: var(--td-success-color); }
  }
}

.overview-summary {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;

  .os-head {
    padding: 10px 14px;
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    border-bottom: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container-hover);
  }

  .os-table {
    .os-row {
      display: grid;
      grid-template-columns: 240px 160px 1fr;
      gap: 8px;
      padding: 8px 14px;
      border-bottom: 1px solid var(--td-component-stroke);
      font-size: 13px;
      align-items: center;

      &:last-child { border-bottom: none; }

      &--head {
        font-size: 12px;
        color: var(--td-text-color-secondary);
        background: var(--td-bg-color-container-hover);
      }

      &--total {
        background: var(--td-brand-color-light);
        font-weight: 600;

        .os-name { color: var(--td-brand-color); }
      }

      .os-name { color: var(--td-text-color-primary); }
      .os-amount { font-variant-numeric: tabular-nums; }
      .os-desc { font-size: 12px; color: var(--td-text-color-secondary); }

      .os-neg { color: var(--td-error-color); }
      .os-diff { margin-left: 6px; }
    }
  }
}

/* 电量明细 */
.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;

  .meter-row {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 13px;
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
