<template>
  <div class="utilities-management-container biz-page">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>能耗管理</h2>
        <p class="header-subtitle">市电、光伏账单自动解析归档；电、水、气按表计录入自动汇总</p>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && activeTab === 'electricity'" class="loading-area">
      <!-- 模块初始化加载：与上游项目完全一致（small、无文字、父容器居中） -->
      <t-loading size="small" />
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
      <!-- 单栏全宽布局（与发票管理一致）：侧边栏已取消，6 个菜单 1:1 转为顶部 KPI 卡片 -->
      <div class="utilities-content">
        <!-- 顶部 KPI 卡片栏：6 个业务卡 + 1 历史记录卡。
             必须放在各 tab 模板**之外**——放进 electricity tab 内的话，
             切到光伏/核算/清单卡片时整栏会消失，用户失去导航（真机踩过）。
             各 tab 自己的工具栏仍在各自模板里（不同 tab 的筛选条件不同）。 -->
        <div class="overview-group">
          <DashboardKpiGroup :loading="false" :cards="overviewCards" :active-type="activeTab"
            @card-click="onKpiCardClick" />
        </div>
          <template v-if="activeTab === 'electricity'">
          <div v-if="kbId" class="electricity-panel biz-main">
            <!-- ================= 列表视图 ================= -->
            <template v-if="!detailMode">
            <div class="biz-list-view">
            <!-- 筛选工具栏走中台 BusinessFilterBar：
                 左侧 = 搜索全部字段 + 账单月份 + 刷新；
                 右侧严格 [字段] → [设置] → [上传账单(绿底主按钮)]，与发票/合同/制度/通知一致。
                 旧版的「历史」图标按钮已删除：历史入口统一由顶部 KPI 的「历史记录」卡承担。 -->
            <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
              :columns="columnDefs" :visible-keys="visibleColKeys" :selected-count="selectedRowKeys.length"
              settings-text="设置" :primary-action="{ text: '上传账单', icon: 'upload' }"
              @columns-change="onColumnsChange" @reset-columns="resetColumns"
              @select-all-columns="selectAllColumns" @filter-change="applyFilter"
              @settings="settingsVisible = true" @primary="triggerUpload"
              @clear-selection="clearSelection">
  <template #batch-actions>
                  <!-- 浮动工具栏（第 17 轮统一口径）：重新解析 → 重新提取 → 打印清单 → 打印目录 → 删除。
                       · 「编辑数据」已删除：点行即进详情，工具栏再放一个入口只是重复；
                       · 「重新解析」= 重新解析源文件（解析完成后由轮询自动触发提取）；
                       · 「重新提取」= 复用已有解析结果直接重跑字段提取；
                       · 「打印」拆成「打印清单」（纵向结算单）与「打印目录」（横向汇总表）。 -->
                  <t-popconfirm theme="warning"
                    :content="`确定重新解析「${selectedSingle?.fileName || ''}」吗？将重新解析源文件，解析完成后自动提取。`"
                    :confirm-btn="{ content: '重新解析', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="handleReparse">
                    <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                      <template #icon><t-icon name="refresh" size="14px" /></template>
                      重新解析
                    </t-button>
                  </t-popconfirm>
                  <t-popconfirm theme="warning"
                    :content="`确定重新提取「${selectedSingle?.fileName || ''}」吗？将覆盖已有提取结果。`"
                    :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="handleReExtract">
                    <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                      <template #icon><t-icon name="file-copy" size="14px" /></template>
                      重新提取
                    </t-button>
                  </t-popconfirm>
                  <t-button theme="default" variant="outline" size="small" :loading="statementBusy"
                    :disabled="selectedRows.some(r => r.kind === 'pending')" @click="handlePrintStatement">
                    <template #icon><t-icon name="print" size="14px" /></template>
                    打印清单
                  </t-button>
                  <t-button theme="default" variant="outline" size="small" :loading="catalogBusy"
                    :disabled="selectedRows.some(r => r.kind === 'pending')" @click="handlePrintCatalog">
                    <template #icon><t-icon name="view-list" size="14px" /></template>
                    打印目录
                  </t-button>
                  <t-popconfirm theme="warning"
                    :content="`确定删除所选 ${selectedRowKeys.length} 条账单记录吗？源文件将移入删除历史。`"
                    :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="handleBatchDelete">
                    <t-button theme="danger" variant="outline" size="small" @click.stop>
                      <template #icon><t-icon name="delete" size="14px" /></template>
                      删除
                    </t-button>
                  </t-popconfirm>
                </template>
            </BusinessFilterBar>

            <!-- 进行中文件状态横幅（解析/提取实时进度，终态自动收起） -->
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
          :loading="listLoading"
          max-height="100%"
          sticky-header
          class="doc-table"
          :foot-data="footData"
          :rowspan-and-colspan-in-footer="footSpan"
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
            <!-- 底部摘要栏已下线（第 21 轮）：表尾合计行首列已含「合计（共 N 条）/（已选 K 条）」，
                 再挂一条「共 N 条」属重复且占纵向空间（用户截图红框）。 -->
            </div>
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
                <!-- 状态标签已移除：详情页是「看这一张账单」的专注视图，
                     提取状态在列表的「状态」列已经明确，标题栏再挂一枚徽标只是噪声。 -->
                <t-button variant="outline" size="small" class="bill-detail-edit" :disabled="!currentRow"
                  @click="openBillDetailDrawer">
                  <template #icon><t-icon name="edit" size="14px" /></template>
                  编辑字段
                </t-button>
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
                            <span class="os-amount" :class="{ 'os-neg': r.displayValue < 0 }" @click.stop="!r.total && feeEditingKey !== r.key && startFeeEdit(r)">
                              <template v-if="feeEditingKey === r.key">
                                <!-- 阻止冒泡交给外层 .os-amount（原生 span）：
                                     t-input 的 click 首参是 { e } 对象，@click.stop 会抛 TypeError -->
                                <t-input v-model="feeEditValue" size="small" class="os-fee-input"
                                  @blur="commitFeeOverride(r)" @enter="commitFeeOverride(r)" />
                              </template>
                              <template v-else>
                                <t-tooltip v-if="!r.total && hasFeeOverride(r.key)" content="手动修改，重提取后重置" placement="top">
                                  <span class="os-fee-val os-fee-val--manual">{{ fmtRate6(r.displayValue) }}</span>
                                </t-tooltip>
                                <span v-else class="os-fee-val" :class="{ 'os-fee-val--manual': r.total }">{{ fmtRate6(r.displayValue) }}</span>
                              </template>
                            </span>
                            <span class="os-amount" :class="{ 'os-neg': Number(r.billFee) < 0 }" @click.stop="!r.total && billFeeEditingKey !== (r as any).ovBillKey && startBillFeeEdit(r)">
                              <template v-if="billFeeEditingKey === (r as any).ovBillKey">
                                <t-input v-model="billFeeEditValue" size="small" class="os-fee-input"
                                  @blur="commitBillFeeOverride(r)" @enter="commitBillFeeOverride(r)" />
                              </template>
                              <template v-else>
                                <t-tooltip v-if="!r.total && hasBillFeeOverride((r as any).ovBillKey)" content="手动修改，重提取后重置" placement="top">
                                  <span class="os-fee-val os-fee-val--manual">{{ r.billFee ? fmtRate6(r.billFee) : '' }}</span>
                                </t-tooltip>
                                <span v-else class="os-fee-val">{{ r.billFee ? fmtRate6(r.billFee) : '' }}</span>
                              </template>
                            </span>
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
                            <span class="os-amount" :class="{ 'os-neg': Number(billTotalText) < 0 }">{{ billTotalText }}</span>
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
<span class="bill-card-hint">点击行可编辑</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>示数类型</span><span>上期示数</span><span>本期示数</span><span>倍率</span><span>抄见电量</span><span>变损</span><span>线损</span><span>加减</span><span>计费电量</span>
                        </div>
                        <div v-for="(r, i) in meterRows" :key="r.meter_type || i" class="meter-row" @click="openMeterEdit(i)">
                          <span class="meter-type">{{ r.meter_type }}</span>
                          <span class="row-mono">{{ r.prev != null ? fmtKwh(Number(r.prev)) : '' }}</span>
                          <span class="row-mono">{{ r.curr != null ? fmtKwh(Number(r.curr)) : '' }}</span>
                          <span class="row-mono">{{ displayMultiplier(r) }}</span>
                          <span class="row-mono">{{ r.reading_kwh != null ? fmtKwh(Number(r.reading_kwh)) : '' }}</span>
                          <span class="row-mono">{{ r.trans_loss != null ? fmtKwh(Number(r.trans_loss)) : '' }}</span>
                          <span class="row-mono">{{ r.line_loss != null ? fmtKwh(Number(r.line_loss)) : '' }}</span>
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
<span class="bill-card-hint">计费电量 = 本期电量 × 定比 + 加减，点击行编辑</span>
                      </div>
                      <div class="meter-table">
                        <div class="meter-row meter-head">
                          <span>示数类型</span><span>上期示数</span><span>本期示数</span><span>倍率</span><span>抄见电量</span><span>变损</span><span>线损</span><span>加减</span><span>计费电量</span>
                        </div>
                        <div v-for="(r, i) in residentMeterRows" :key="i" class="meter-row" @click="openResidentMeterEdit(i)">
                          <span class="meter-type">{{ r.meter_type }}</span>
                          <span class="row-mono">{{ r.prev != null ? fmtKwh(Number(r.prev)) : '' }}</span>
                          <span class="row-mono">{{ r.curr != null ? fmtKwh(Number(r.curr)) : '' }}</span>
                          <span class="row-mono">{{ r.multiplier != null ? fmtKwh(Number(r.multiplier)) : '' }}</span>
                          <span class="row-mono">{{ r.reading_kwh != null ? fmtKwh(Number(r.reading_kwh)) : '' }}</span>
                          <span class="row-mono">{{ r.trans_loss != null ? fmtKwh(Number(r.trans_loss)) : '' }}</span>
                          <span class="row-mono">{{ r.line_loss != null ? fmtKwh(Number(r.line_loss)) : '' }}</span>
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
<span class="bill-card-hint">共 {{ feeRowsOf(activeMenu).length }} 项，点击行编辑</span>
                      </div>
                      <div class="fee-group-table">
                        <div class="fg-row fg-head">
                          <span v-for="c in feeGroupCols(activeMenu)" :key="c.field_key">{{ c.label }}</span>
                        </div>
                        <div v-for="(it, i) in feeRowsOf(activeMenu)" :key="i" class="fg-row" @click="openFeeItemEdit(activeMenu, Number(i))">
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
<span class="bill-card-hint">点击行编辑</span>
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
<span class="bill-card-hint">点击行编辑</span>
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
            <UtilityMeterTab ref="meterTabRef" category="water" @open-settings="openChildSettings('水费核算')" />
          </template>
          <template v-else-if="activeTab === 'gas'">
            <UtilityMeterTab ref="meterTabRef" category="gas" @open-settings="openChildSettings('气费核算')" />
          </template>
          <template v-else-if="activeTab === 'electric-meter'">
            <UtilityMeterTab ref="meterTabRef" category="electricity" @open-settings="openChildSettings('电费核算')" />
          </template>
          <template v-else-if="activeTab === 'solar'">
            <SolarManagement @open-settings="openChildSettings('光伏账单')" />
          </template>
          <template v-else-if="activeTab === 'billing'">
            <TenantBilling ref="tenantTabRef" @open-settings="openChildSettings('费用清单')" />
          </template>
        </div>
      </div>


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
        <div class="edit-drawer-footer">
          <t-button theme="primary" size="small" @click="saveEditForm">保存</t-button>
        </div>
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
        <div class="edit-drawer-footer">
          <t-button theme="primary" size="small" @click="saveEditForm">保存</t-button>
        </div>
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
        <div class="edit-drawer-footer">
          <t-button theme="primary" size="small" @click="saveEditForm">保存</t-button>
        </div>
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
        <div class="edit-drawer-footer">
          <t-button theme="primary" size="small" @click="saveEditForm">保存</t-button>
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

    <!-- 设置：中台标准双 Tab（字段定义 + 提取规则），与发票设置 100% 对齐。
         旧的 12 宫格卡片抽屉（UtilitySettingsDrawer）已下线——分组改由
         「字段定义」里的分组胶囊条承载，配置统一存 fleet_categories(scope=energy)。 -->
    <!-- 设置抽屉：分类与顶部 6 张卡片 1:1，打开即定位到当前卡片对应的分类 -->
    <!-- 账单字段编辑抽屉：与发票/合同同款中台 BusinessSplitDetailDrawer。
         整页详情视图保留（分层分析），「编辑字段」在这里改，避免在整页里塞表单。 -->
    <BusinessSplitDetailDrawer v-model:visible="billDrawerVisible" :title="billDrawerTitle"
      :row="currentRow" :page-no="Number(currentRow?.item?.page) || 0" :form="billFormWrapper"
      :fields="detailFieldDefs" block-title="账单字段" :quality="detailQuality" :label-of="qualityLabel"
      :saving="billSaving" :storage-key="'weknora-utility-bill-drawer-width'"
      @save="saveBillDetail" />

    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" scope="energy"
      :initial-category="settingsCategory" :rule-categories="ENERGY_RULE_CATEGORIES"
      :kb-name="'日常事务-电费'" title="设置 · 能耗"
      :extra-tab-label="extraTabLabel" :extra-tab-value="extraTabValue"
      storage-key="weknora-energy-settings-drawer-width" @saved="onSettingsChanged">
      <!-- 专属设置（原工具栏「光伏设置 / 表计配置 / 租户设置」按钮的归宿）：
           工具栏严格 3 个按钮，专属面板作为统一设置抽屉的 Tab 内嵌。 -->
      <template #extra-tab>
        <SolarSettingsPanel v-if="activeTab === 'solar'" :kb-id="kbId || ''" @changed="onSettingsChanged" />
        <MeterSettingsPanel v-else-if="meterCategory" :category="meterCategory" @changed="onSettingsChanged" />
        <TenantSettingsPanel v-else-if="activeTab === 'billing'" :kb-id="kbId || ''" @changed="onSettingsChanged" />
      </template>
    </StandardSettingDrawer>

    <!-- 上传账单：与发票管理完全同一个 FleetUploadDialog（拖拽/多选/队列进度/重复文件智能恢复），
         分类固定为「市电账单」，提取走账单专用管线（extract-utility-bill）。
         上传即写 fleet_cert_type=市电账单，轮询据此认领，不再靠文件名猜。 -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="utility"
      :type-options="uploadTypeOptions" default-type="市电账单" title="上传账单" extract-mode="energy-bill"
      @done="onUploadDone" />

    <!-- 删除历史 -->
    <!-- 上传历史抽屉：复用中台 FleetUploadHistoryDrawer（三态统计卡 / 即输即搜 / 分类+日期筛选 /
         中文错误提示 / 双击文件名全屏预览），与发票、合同、制度、通知完全同款。
         旧的自写「删除历史」抽屉已下线。 -->
    <FleetUploadHistoryDrawer v-model:visible="uploadHistoryVisible" :kb-id="kbId || ''" scope="energy"
      @changed="onHistoryChanged" />

    <!-- 打印预览：全中台统一组件（打印清单 / 打印目录共用，标题与文件名按当前动作切换） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="printTitle" :url="printUrl"
      :data="printBytes" :download-name="printFileName" :loading-text="printLoadingText"
      @close="closePrint" />
  </div>
</template>

<script setup lang="ts">
import { chartPalette } from './chartTheme'
import { grouped } from './formatNumber'
import {
  ref,
  reactive,
  computed,
  watch,
  nextTick,
  onMounted,
  onBeforeUnmount,
} from 'vue'
import { useTableHeaderFix } from './useTableHeaderFix'
import { ENERGY_OPEN_SETTINGS_EVENT } from './energyViewColumns'
import { countMeterRows, billingRowCount, historyStats, ENERGY_DATA_CHANGED_EVENT, notifyEnergyDataChanged } from './energyCounters'
import { useBusinessPolling } from '@/composables/useBusinessPolling'
import { listKnowledgeFiles, listBillingRecords } from '@/api/knowledge-base'
import { MessagePlugin } from 'tdesign-vue-next'
import * as echarts from 'echarts'
import {
  listKnowledgeBases,
  getKnowledgeDetails,
  updateBusinessMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractUtilityBill,
  listUtilityBillRecords,
  listSolarBillRecords,
  listUtilityMeterRecords,
  listBillingTenants,
  reparseKnowledge,
} from '@/api/knowledge-base'
import { generateCatalogPdf, generateStatementPdf, type CatalogColumn } from './useCatalogPdf'
import { buildUtilityBillStatementDoc } from './energyBillStatements'
import { buildFootRow, footSpanOf, summaryScopeRows, type SummaryColumn } from './listSummary'
// 逐列表尾汇总行样式（TDesign 表尾 td 拿不到 scoped 作用域）
import './styles/list-summary-footer.css'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import UtilitiesKbWizard from './UtilitiesKbWizard.vue'
import UtilityMeterTab from './UtilityMeterTab.vue'
import SolarManagement from './SolarManagement.vue'
import TenantBilling from './TenantBilling.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import SolarSettingsPanel from './SolarSettingsPanel.vue'
import MeterSettingsPanel from './MeterSettingsPanel.vue'
import TenantSettingsPanel from './TenantSettingsPanel.vue'
import { listFleetCategories } from '@/api/fleet'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import { normalizeBillingPeriod } from './billingPeriod'
import BusinessFilterBar from './BusinessFilterBar.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import type { KpiCard } from '@/components/business/DashboardKpiGroup.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import { parseExtractQuality, EMPTY_QUALITY, type ExtractQuality } from './extractQuality'
import { buildEnergyChartOption } from './utilityEnergyChart'
import { buildBillMetrics } from './utilityBillMetrics'
import { buildOverviewRows, overviewTotalOf, qtyLabelOf as qtyLabelOfUtil } from './utilityBillOverview'
import {
  BILL_MENUS,
  CAPACITY_FIELDS,
  PF_FIELDS,
  buildEnergyCompareRows,
  buildMeterRows,
  buildResidentMeterRows,
  feeMenuOf,
  feeRowsOf as feeRowsOfUtil,
  feeSubtotalOf as feeSubtotalOfUtil,
  matchRow as matchRowUtil,
  menuLabelOf,
  pickPrevEnergy,
  rowConfigsOf as rowConfigsOfUtil,
  ROW_GROUP_OF_MENU,
} from './utilityBillDetail'
import {
  FALLBACK_COLUMNS,
  colWidthPx,
  feeCellTextOf,
  feeGroupColsOf,
  displayMultiplierOf,
  makeAmountCell,
  type ColDef,
} from './utilityBillColumns'

// 表头右侧滚动条轨道遮罩（实测，四模块 + 能耗统一）
const { headerFixStyle, measureTableHeaderFix } = useTableHeaderFix()

const KB_NAME = '日常事务-电费'
const PAGE_SIZE = 30
const COLUMN_STORAGE_KEY = 'weknora-utility-electricity-columns-v5'
const COLUMN_STORAGE_VERSION = 5

// 列定义 / 列宽 / 金额渲染 / 费用子表列派生全部来自 utilityBillColumns：
// 这些是纯函数（不读响应式状态），抽出去后能直接单测口径，页面只做组装。
// group 字段用于费用分项归类：'industrial'/'residential' 是旧口径，
// 其余值来自 subs.group（overview/market/line/...），故放宽为 string。

/** 能耗页 6 个业务视图（原侧边栏菜单项，现为顶部 KPI 卡片）。 */
type EnergyTab = 'electricity' | 'solar' | 'electric-meter' | 'water' | 'gas' | 'billing'

const activeTab = ref<EnergyTab>('electricity')

// 表头右侧滚动条轨道遮罩的实测值（与发票管理同一套机制）：
// 滚动条属于滚动容器 UI 层，绘制层级高于 sticky 表头，靠表头背景盖不住，
// 必须在 .doc-list-view 上挂一块与表头同色的绝对定位色块。
const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const settingsVisible = ref(false)

// 字段配置（来自后端 /utilities/field-configs?category=electricity，动态加载）
const columnDefs = ref<ColDef[]>(FALLBACK_COLUMNS)
const visibleColKeys = ref<string[]>(loadStoredColumns())
const visibleColDefs = computed(() => columnDefs.value.filter(c => visibleColKeys.value.includes(c.key)))

/**
 * 表尾逐列汇总（中台统一口径，见 listSummary）。
 *
 * 列口径 = 当前**可见列**（用户关掉的列不参与），行口径 = 当前筛选后的
 * displayRows（「搜索全部字段 / 账单月份」过滤之后），与用户看到的表格严格一致。
 */
const summaryColumns = computed<SummaryColumn[]>(() =>
  visibleColDefs.value.map((c) => ({ key: c.key, label: c.label, fieldType: c.fieldType })),
)
/**
 * 表尾统计范围：**勾选优先，否则全部可见行**（第 20 轮）。
 * 勾选后汇总收敛到所选行，首列文案切成「已选 K 条」。
 */
const summaryScope = computed(() =>
  summaryScopeRows(displayRows.value, selectedRowKeys.value, (r: any) => String(r?.rowKey ?? '')),
)
const footData = computed(() => {
  const { rows, selectedCount } = summaryScope.value
  return rows.length ? [buildFootRow(summaryColumns.value, rows, { selectedCount })] : []
})
const footSpan = computed(() => footSpanOf(summaryColumns.value))
const tableColumns = computed(() => {
  // 列结构与发票/合同/制度/通知逐项一致：
  //   [复选框] → [序号] → 业务列… → 状态 → 标签
  // 序号列此前缺失，用户没法快速和「第 N 条」对话（反馈问题/对数时很费劲）。
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 56, align: 'center' },
  ]
  for (const c of visibleColDefs.value) {
    // 账单周期 / 时间类列关闭 ellipsis 兜底宽度，避免 "2026-09-0..." 截断；
    // 其余长文本列保留 ellipsis + 悬浮全文。
    const narrow = c.key === 'bill_period'
    cols.push({
      colKey: c.key,
      title: c.label,
      ellipsis: !narrow,
      width: colWidthPx(c),
      fieldType: c.fieldType,
      cell: cellOf(c),
    })
  }
  cols.push({ colKey: 'extractStatus', title: '状态', width: 110 })
  cols.push({ colKey: 'tags', title: '标签', width: 140 })
  return cols
})
// 金额列渲染器：实现与口径都在 utilityBillColumns.makeAmountCell，
// 这里只把本页的 fmtAmount 注入进去（避免模块反向依赖 formatNumber）。
const cellOf = (c: ColDef) => makeAmountCell(c, fmtAmount)
function onTableSelectChange(val: any) { selectedRowKeys.value = Array.isArray(val) ? val : [] }

/**
 * 读取列显隐存档。
 *
 * 存档里的 key 必须仍存在于当前 `columnDefs`（= subs 派生结果）：
 * 用户在「字段配置」删掉一个字段后，旧存档里的 key 就成了幽灵，
 * 若原样返回，`visibleColDefs` 过滤时虽然渲不出来，但 `[☰ 字段]` 的
 * 「全选/重置」仍会以 columnDefs 为准 —— 所以这里直接剔除并把存档回写，
 * 保证「重置」只会落到当前 subs 里真实存在的字段。
 */
function loadStoredColumns(): string[] {
  const defaults = columnDefs.value.filter(c => c.default).map(c => c.key)
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY)
    if (raw) {
      const obj = JSON.parse(raw)
      // 带版本校验：旧版本或结构不符时采用新默认集
      if (obj && obj.v === COLUMN_STORAGE_VERSION && Array.isArray(obj.keys) && obj.keys.length) {
        const valid = obj.keys.filter((k: string) => columnDefs.value.some(c => c.key === k))
        if (valid.length) {
          // 存档里有已删除字段 → 就地清洗，避免下次打开又被读回来
          if (valid.length !== obj.keys.length) {
            try {
              localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify({ v: COLUMN_STORAGE_VERSION, keys: valid }))
            } catch { /* ignore */ }
          }
          return valid
        }
        // 全部失效（字段被删光）→ 回落到当前默认列
        return defaults
      }
    }
  } catch { /* ignore */ }
  return defaults
}
function selectAllColumns() { visibleColKeys.value = columnDefs.value.map(c => c.key) }
function resetColumns() { visibleColKeys.value = columnDefs.value.filter(c => c.default).map(c => c.key) }
function persistColumns() {
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify({ v: COLUMN_STORAGE_VERSION, keys: visibleColKeys.value })) } catch { /* ignore */ }
}
watch(visibleColKeys, () => persistColumns(), { deep: true })

const allFieldConfigs = ref<any[]>([])
// 费用子表三件事全部委托 utilityBillColumns（口径在模块内，可单测）：
//   feeGroupCols  —— 按 subs.group + default_visible 派生列，三级兜底顺序不变；
//   feeCellText   —— 数值走 fmtRate6、金额走 fmtMoney，**0 必须上屏**；
//   displayMultiplier —— 倍率只用提取值，不推算。
const feeGroupCols = (group: string) => feeGroupColsOf(allFieldConfigs.value, group)
const feeCellText = (it: any, col: any) =>
  feeCellTextOf(it, col, { rate6: fmtRate6, money: fmtMoney })
const displayMultiplier = (r: any) => displayMultiplierOf(r, fmtRate6)

const loadFieldConfigs = async () => {
  // 字段配置的唯一权威源：fleet_categories(scope=energy, name=市电账单) 的 subs。
  //
  // 改造前读 utility_field_configs —— 那份表只有 7 个分组 56 条，
  // 其余分组靠后端内存兜底，且**提取器完全读不到**（配了也提不出来）。
  // 现在统一到中台 subs 后，字段定义 / 提取规则 / 列表列 / 详情抽屉同源。
  try {
    const res: any = await listFleetCategories({ scope: 'energy' })
    const cats = res?.data || res || []
    const cat = (Array.isArray(cats) ? cats : []).find((c: any) => c.name === '市电账单')
    const subs: any[] = Array.isArray(cat?.subs) ? cat.subs : []
    if (!subs.length) return

    // 供费用分项子表（feeGroupCols）与详情分表复用：保持旧结构形态，
    // 下游消费点（2 处 filter(c => c.group === group)）无需改动。
    allFieldConfigs.value = subs
      .filter((s: any) => String(s?.name || '').trim())
      .map((s: any) => ({
        field_key: String(s.name).trim(),
        label: String(s.label || '').trim() || String(s.name).trim(),
        field_type: s.data_type || 'text',
        default_visible: s.is_default === true,
        group: String(s.group || ''),
        deleted_at: null,
      }))

    // 主列表口径：**subs 是唯一权威源**。
    //
    // 旧实现在这里做 `FALLBACK_COLUMNS.map()`，subs 里找不到就 `return { ...fb }`
    // 保留硬编码兜底列 —— 于是用户在「字段配置」里删掉的字段（如「上期电量」）
    // 会从 FALLBACK_COLUMNS 里复活，点「重置」更是必现（真机复现）。
    // 现在：subs 一旦有配置，列集合与顺序 100% 由 subs 决定；
    // FALLBACK_COLUMNS 只在 subs 为空（后端未迁移/用户从未配置）时整表兜底。
    const isEnabled = (s: any) => s.enabled !== false
    const labelOfSub = (s: any) => String(s.label || '').trim() || String(s.name).trim()
    const fbByKey = new Map(FALLBACK_COLUMNS.map((f) => [f.key, f]))

    const primary: ColDef[] = []
    const seen = new Set<string>()
    for (const s of subs) {
      const n = String(s?.name || '').trim()
      if (!n || seen.has(n)) continue
      if (!isEnabled(s)) continue            // 用户禁用 → 不出列
      const g = String(s.group || '')
      if (g && g !== 'overview') continue     // 子表字段不进主列表
      seen.add(n)
      const fb = fbByKey.get(n)
      primary.push({
        key: n,
        label: labelOfSub(s),
        fieldType: s.data_type || fb?.fieldType || 'text',
        default: s.is_default === true,
        w: fb ? fb.w : '1fr',
        // subs.width 是列宽唯一权威源：>0 精确固定（clamp 60~400），0/未配置走 w 兜底
        width: Number(s.width) || 0,
        group: g || undefined,
      })
    }

    columnDefs.value = primary
    // 默认列变更后重算可见列（保留用户已存储的偏好；存档缺默认列时自动重置）
    visibleColKeys.value = loadStoredColumns()
    persistColumns()
  } catch { /* 字段配置加载失败用内置默认 */ }
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
  /** 归一到 YYYY-MM 的账单周期（mapRow 写入，标题栏与搜索都用它） */
  bill_period?: string
}
const rows = ref<Row[]>([])
const summary = ref<{ total: number; sumKwh: number; sumAmount: number }>({ total: 0, sumKwh: 0, sumAmount: 0 })
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const monthFilter = ref('')
const keyword = ref('')

// ---- 中台筛选工具栏（BusinessFilterBar）----
//
// 声明式筛选：keyword 渲染在搜索位，month 渲染成月粒度选择器。
// 旧版是手写 t-date-picker + BusinessListToolbar 插槽，按钮顺序与样式都自己维护，
// 结果和发票/合同越走越偏（上传按钮白底、位置在设置左边、右侧还多一个历史图标）。
const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword' },
  { key: 'month', type: 'month', placeholder: '账单月份' },
])
// 扁平筛选态与既有 ref 双向桥接；组件只发不可变更新
const filterState = computed<FilterState>({
  get: () => ({ keyword: keyword.value, month: monthFilter.value }),
  set: (v) => {
    keyword.value = String(v?.keyword ?? '')
    monthFilter.value = String(v?.month ?? '')
  },
})

// ---- 顶部概览卡（DashboardKpiGroup）----
// 历史记录卡的文件数是「已上传文件」口径（含解析/提取失败件），与发票一致，
// 因此单独拉一次文件列表统计；列表接口是记录口径，数不出上传文件数。
const uploadHistoryVisible = ref(false)
const overviewFileStats = ref<{ total: number; failed: number }>({ total: 0, failed: 0 })
// ---- 顶部概览卡（DashboardKpiGroup）：6 个业务卡 + 1 历史记录卡 ----
//
// 侧边栏取消后，原来那 6 个菜单项 1:1 转为顶部卡片：
// 点击前 6 张任意一张 → switchTab 切换下方主区域；第 7 张「历史记录」只开抽屉。
// 高亮由 KpiCard 的 action=type + typeValue 与 :active-type="activeTab" 比对得出，
// 因此不需要自己维护选中态，切到哪个 tab 哪张卡就亮。
const ENERGY_TABS: { key: EnergyTab; label: string; icon: string; unit: string }[] = [
  { key: 'electricity', label: '市电账单', icon: 'chart-bubble', unit: '份' },
  { key: 'solar', label: '光伏账单', icon: 'chart-radar', unit: '份' },
  { key: 'electric-meter', label: '电费核算', icon: 'thunder', unit: '条' },
  { key: 'water', label: '水费核算', icon: 'dashboard', unit: '条' },
  { key: 'gas', label: '气费核算', icon: 'windy', unit: '条' },
  { key: 'billing', label: '费用清单', icon: 'bill', unit: '条' },
]
// 各 tab 的记录数（独立于列表接口，避免为了显示数字把整页数据拉回来）
const tabCounts = reactive<Record<string, number>>({
  electricity: 0, solar: 0, 'electric-meter': 0, water: 0, gas: 0, billing: 0,
})
// KpiCard.action 在前 6 张是 'type'（随 activeTab 高亮），末卡是 'history'（只开抽屉），
// 因此这里显式声明成字面量联合，避免 TS 把整组窄化成单一字面量后拒绝 history。
type EnergyKpiCard = KpiCard & { typeValue?: EnergyTab }
const overviewCards = computed<EnergyKpiCard[]>(() => {
  const st = overviewFileStats.value
  const cards: EnergyKpiCard[] = ENERGY_TABS.map((t) => ({
    key: `tab-${t.key}`,
    label: t.label,
    icon: t.icon,
    value: `${Number(tabCounts[t.key] || 0)}`,
    unit: t.unit,
    sub: '',
    cls: '',
    action: 'type',
    typeValue: t.key,
  }))
  cards.push({
    key: 'history', label: '历史记录', icon: 'history',
    value: `${st.total}`, unit: '份',
    sub: st.failed > 0 ? `${st.failed} 份异常文件` : '无异常文件',
    cls: st.failed > 0 ? 'is-warn' : '',
    action: 'history',
  })
  return cards
})
/** 顶部 6 张卡片 ↔ 设置分类名的 1:1 映射（顺序即卡片顺序）。 */
const ENERGY_TAB_CATEGORY: Record<EnergyTab, string> = {
  electricity: '市电账单',
  solar: '光伏账单',
  'electric-meter': '电费核算',
  water: '水费核算',
  gas: '气费核算',
  billing: '费用清单',
}
/**
 * 子组件工具栏「设置」要定位的分类（光伏账单 / 电费核算 / …）。
 *
 * 6 个卡片视图各自的工具栏都在子组件里，设置抽屉只在本页挂一份：
 * 子组件广播 ENERGY_OPEN_SETTINGS_EVENT，这里接住并把抽屉定位到那个分类。
 * 抽屉关闭后清空，避免下一次从 KPI 卡片进入时还停在上次的分类。
 */
const settingsCategoryOverride = ref('')
const settingsCategory = computed(() =>
  settingsCategoryOverride.value || ENERGY_TAB_CATEGORY[activeTab.value] || '市电账单')
function onEnergyOpenSettings(e: Event) {
  const detail = (e as CustomEvent<{ category?: string }>).detail || {}
  settingsCategoryOverride.value = String(detail.category || '')
  settingsVisible.value = true
}
watch(settingsVisible, (v) => { if (!v) settingsCategoryOverride.value = '' })
/**
 * 允许配置「提取规则」的分类白名单。
 *
 * 只有市电账单与光伏账单是 AI 从文件里提取的；电费/水费/气费核算与费用清单是
 * 录入 + 公式算出来的计算表，没有可提取的字段，规则页对它们没有意义。
 */
const ENERGY_RULE_CATEGORIES = ['市电账单', '光伏账单']

/**
 * 专属设置 Tab 的标题：与工具栏「设置」按钮的抽屉内页签一一对应。
 *
 * 市电账单没有专属面板（字段定义 + 提取规则已经覆盖它的全部配置），
 * 光伏账单 = 光伏设置、三张核算表 = 表计设置、费用清单 = 清单设置。
 * 这几个面板原本是工具栏上独立的第 4 个按钮，现在作为抽屉 Tab 内嵌。
 */
const ENERGY_EXTRA_TAB_LABEL: Record<EnergyTab, string> = {
  electricity: '',
  solar: '光伏设置',
  'electric-meter': '表计设置',
  water: '表计设置',
  gas: '表计设置',
  billing: '清单设置',
}
const extraTabLabel = computed(() => ENERGY_EXTRA_TAB_LABEL[activeTab.value] || '')
const extraTabValue = computed(() => {
  if (activeTab.value === 'solar') return 'solar'
  if (activeTab.value === 'billing') return 'billing'
  return 'meter'
})
/** 三张核算表的介质（表计设置面板据此切换电表/水表/气表）。 */
const meterCategory = computed(() => {
  if (activeTab.value === 'electric-meter') return 'electricity'
  if (activeTab.value === 'water') return 'water'
  if (activeTab.value === 'gas') return 'gas'
  return ''
})

/**
 * 子视图工具栏「设置」→ 打开统一设置抽屉并定位到该分类。
 *
 * 用 emit 而不是 window 事件：子视图都在 v-if 分支里，事件监听挂在页面根部，
 * 直接 emit 链路最短、也不会因为广播名不一致而静默失联。
 */
const meterTabRef = ref<any>(null)
const tenantTabRef = ref<any>(null)

function openChildSettings(category: string) {
  settingsCategoryOverride.value = String(category || '')
  settingsVisible.value = true
}

const onKpiCardClick = (card: any) => {
  if (card?.action === 'history') {
    uploadHistoryVisible.value = true
    return
  }
  if (card?.typeValue) switchTab(card.typeValue)
}
// 拉取 6 个 tab 的记录数。任一失败都不影响其它卡片与列表（各自 catch）。
//
// **口径铁律**：卡片数字 === 切到该卡片后列表底部「共 N 条」。
//   市电/光伏 = 账单记录数（列表一行一账单，failed 文件本就不落 records）；
//   电/水/气核算 = record→item **展平后的抄表明细行数**（列表一行一表计，
//     原口径取 records.length：3 个月显示 3 条、列表却是 77 行——真机踩过）；
//   费用清单 = 月份并集行数（与 TenantBilling.buildRows 同一份纯函数）。
async function loadTabCounts() {
  if (!kbId.value) return
  const jobs: [EnergyTab, () => Promise<number>][] = [
    ['electricity', async () => {
      const res: any = await listUtilityBillRecords(kbId.value, { page_size: 500 })
      return Number(res?.total || (res?.data || []).length || 0)
    }],
    ['solar', async () => {
      const res: any = await listSolarBillRecords(kbId.value, { page_size: 500 })
      return Number(res?.total || (res?.data || []).length || 0)
    }],
    ['electric-meter', async () => countMeterRowsOf('electricity')],
    ['water', async () => countMeterRowsOf('water')],
    ['gas', async () => countMeterRowsOf('gas')],
    ['billing', billingRowCountOf],
  ]
  await Promise.all(jobs.map(async ([tab, fn]) => {
    try {
      tabCounts[tab] = await fn()
    } catch {
      tabCounts[tab] = 0
    }
  }))
}

/** 核算卡片 = 展平明细行数（与 UtilityMeterTab 列表同一口径）。 */
async function countMeterRowsOf(category: string): Promise<number> {
  const res: any = await listUtilityMeterRecords({ category })
  const recData = res?.data || {}
  const records = recData?.records || res?.records || []
  return countMeterRows(Array.isArray(records) ? records : [])
}

/**
 * 费用清单卡片 = buildRows 的月份并集行数。
 *
 * 原口径取 listBillingTenants().length（租户数 = 1），列表却是按月展开的
 * 账期行（真机：卡片 1 条 / 列表 18 条）。这里用与 TenantBilling 完全相同的
 * 数据源集合（市电∪光伏∪rec∪电∪气∪水读数）计算，避免两套逻辑漂移。
 */
async function billingRowCountOf(): Promise<number> {
  const res: any = await listBillingTenants()
  const tenants = res?.data || []
  const tid = (Array.isArray(tenants) && tenants.length) ? tenants[0].id : ''
  const tasks: Promise<any>[] = [
    listUtilityBillRecords(kbId.value, { page_size: 500 }),
    listSolarBillRecords(kbId.value, { page_size: 500 }),
    listUtilityMeterRecords({ category: 'electricity' }),
    listUtilityMeterRecords({ category: 'gas' }),
    listUtilityMeterRecords({ category: 'water' }),
  ]
  if (tid) tasks.push(listBillingRecords(tid))
  const [billRes, solarRes, elecRes, gasRes, waterRes, recRes] = await Promise.all(
    tasks.map((t) => t.catch(() => null)),
  )
  const flat = (res: any) => {
    const recData = res?.data || {}
    const records = recData?.records || res?.records || []
    const flatRows: any[] = []
    for (const rec of (Array.isArray(records) ? records : [])) {
      for (const it of (rec.items || [])) flatRows.push({ month: rec.month, ...it })
    }
    return flatRows
  }
  return billingRowCount({
    utilityBills: billRes?.data || [],
    solarBills: solarRes?.data || [],
    records: recRes?.data || [],
    elecRows: flat(elecRes),
    gasRows: flat(gasRes),
    waterReadings: flat(waterRes),
  })
}
async function loadOverviewFileStats() {
  if (!kbId.value) return
  try {
    // 与 FleetUploadHistoryDrawer.reload() 同口径：一次拉全 + fleet_history_hidden 过滤，
    // 「全部文件/异常文件」两张计数必须与抽屉状态卡 100% 相等（曾取 page_size=200 单页，
    // 库超过 200 就少算；也曾用未过滤的 res.total，多算了隐藏文件）。
    const collected: any[] = []
    let p = 1
    for (;;) {
      const res: any = await listKnowledgeFiles(kbId.value, { page: p, page_size: 200 })
      const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
      collected.push(...arr)
      const totalCnt = Number(res?.total || 0)
      if (arr.length === 0 || collected.length >= totalCnt || collected.length >= 2000) break
      p++
    }
    const st = historyStats(collected)
    overviewFileStats.value = { total: st.total, failed: st.abnormal }
  } catch { /* KPI 降级：保持 0，不影响列表 */ }
}
const onHistoryChanged = () => {
  void loadFiles(true)
  void loadOverviewFileStats()
  void loadTabCounts()
}

// 列显隐：BusinessFilterBar 的列筛选器回写
function onColumnsChange(keys: string[]) {
  visibleColKeys.value = Array.isArray(keys) ? keys : []
}
const selectedRowKeys = ref<string[]>([])
const listScrollRef = ref<HTMLElement>()

// 表尾汇总口径 = 当前**可见**行（已按关键词/月份过滤）：筛选后合计必须同步收敛。
// 逐列合计统一由 listSummary 计算（见 footData），这里只保留「共 N 条」。
/** 底部「共 N 条」：与列表同口径（选中优先，否则可见行数） */
// 底部摘要栏已下线（第 21 轮），条数由表尾合计行首列承载。
const selectedRows = computed(() => displayRows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)))
const selectedSingle = computed(() => (selectedRows.value.length === 1 ? selectedRows.value[0] : null))
const clearSelection = () => { selectedRowKeys.value = [] }

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

const mapRow = (r: any): Row => {
  const it = r.item || {}
  // 费用分类聚合：按「分」做整数累加，最后再折回元。
  // 直接用浮点累加会让 0.1+0.2 这类误差累积到小数点后十位，
  // 而列表列由 TDesign 默认单元格渲染（不经 fmtRate6），脏值会原样上屏
  // ——真机踩过：「输配电费」列显示 53441.77999999996。
  const feeCents: Record<string, number> = {}
  const addCents = (key: string, fee: number) => {
    feeCents[key] = (feeCents[key] || 0) + Math.round(fee * 100)
  }
  for (const fd of (it.fee_items || [])) {
    const cat: string = fd.category || ''
    const fee = Number(fd.fee) || 0
    if (cat.startsWith('(1)')) addCents('market_amount', fee)
    else if (cat.startsWith('(2)')) addCents('line_amount', fee)
    else if (cat.startsWith('(3)')) addCents('trans_amount', fee)
    else if (cat.startsWith('(4)')) addCents('sys_amount', fee)
    else if (cat.startsWith('(5)')) addCents('catalog_amount', fee)
    else if (cat.startsWith('(6)')) {
      if (cat.includes('工商')) addCents('govI_amount', fee)
      else if (cat.includes('居民')) addCents('govR_amount', fee)
    }
  }
  const feeAgg: Record<string, number> = {}
  for (const k of Object.keys(feeCents)) feeAgg[k] = feeCents[k] / 100
  return {
    rowKey: r.row_key || `${r.knowledge_id}-${r.page || 0}`,
    knowledgeId: r.knowledge_id || '',
    fileName: r.file_name || r.knowledge_title || '',
    fileType: r.file_type || '',
    extractStatus: r.extract_status || '',
    extractError: r.extract_error || '',
    kind: r.extract_status === 'manual' ? 'manual' : 'bill',
    item: it,
    ...it,
    ...feeAgg,
    // 账单周期统一成 YYYY-MM：
    //   优先用后端派生的 bill_period（已是 YYYY-MM），
    //   老数据没有该字段时用起止日现场归一，最后才退回 start 原文。
    // 绝不能拼接 `${start} ~ ${end}` —— 那正是列表出现 "2026-09-0..." 截断的原因。
    bill_period: normalizeBillingPeriod(it.bill_period)
      || normalizeBillingPeriod(it.bill_period_start)
      || normalizeBillingPeriod(it.bill_period_end)
      || '',
    tags: r.tags || [],
    page: r.page,
  }
}

// 进行中文件（解析中/提取中/待提取）合并进列表行，与合同模块一致。
//
// 解析失败 / 提取失败的文件**不进主列表**：它们没有任何业务数据，混在账单里只会
// 渲染出一条「⊗ 解析失败」的空壳。异常文件统一在「历史记录 → 异常文件」里
// 展示与重提，与发票/合同管理口径一致。
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
    page: 0,
  })))
// 行级全字段检索文本：把这条账单里所有可读内容拼成一个大字符串，再做包含匹配。
//
// 为什么不用逐列判断：字段是可配置的（用户在「字段定义」里加字段后列表就多一列），
// 逐列写死会漏；而"搜索全部字段"的语义就是"我看到的值都能搜到"。
// 覆盖：20 个概况字段 + 费用明细（含类别/名称/时段/电量/标准/电费）+
// 电量明细 + 标签 + 源文件名 + 提取错误文案。
function rowSearchText(r: any): string {
  const parts: string[] = [String(r.fileName || ''), String(r.title || ''), String(r.extractStatus || ''), String(r.extractError || '')]
  const it = r.item || {}
  for (const [k, v] of Object.entries(it)) {
    if (v === null || v === undefined) continue
    if (k === 'fee_items' || k === 'meter_readings' || k === 'residential_readings' || k === 'gateways') continue
    if (typeof v === 'object') continue
    parts.push(String(v))
  }
  for (const fd of (it.fee_items || [])) {
    parts.push(String(fd.category || ''), String(fd.name || ''), String(fd.period || ''), String(fd.fee ?? ''), String(fd.qty ?? ''), String(fd.rate ?? ''))
  }
  for (const mr of (it.meter_readings || [])) {
    parts.push(String(mr.meter_type || ''), String(mr.bill_kwh ?? ''), String(mr.reading_kwh ?? ''))
  }
  for (const t of (r.tags || [])) parts.push(String(t))
  // 归一后的账单周期也要能搜（用户多半按 2026-08 找）
  parts.push(String(r.bill_period || ''))
  return parts.join(' ').toLowerCase()
}

const displayRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const month = monthFilter.value
  let all = [...pendingRows.value, ...rows.value]
  // 月份 = 账单账期（bill_period，YYYY-MM）；解析中的文件没有账期，按月筛选时不占位
  if (month) all = all.filter((r: any) => r.kind !== 'pending' && String(r.bill_period || '').startsWith(month))
  if (!kw) return all
  // 多关键词（空格分隔）取 AND：输入「星达 2026-08」能精确收敛
  const tokens = kw.split(/\s+/).filter(Boolean)
  return all.filter((r) => {
    const hay = rowSearchText(r)
    return tokens.every((t) => hay.includes(t))
  })
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
    // 月份不交给接口：账期是账单内业务字段（bill_period），接口的 date_from/date_to
    // 打的是通用日期，跨月账单（如 8/26~9/25）会被整条漏掉。见 displayRows。
    const res: any = await listUtilityBillRecords(kbId.value, {
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
  if (extractInFlight.value.has(pf.id)) return 'extracting'
  if (meta.extract_status === 'failed') return 'failed'
  if (meta.extract_status === 'extracting' || pf.extract_status === 'extracting') return 'extracting'
  return 'pending'
}

// 轮询：解析完成后自动触发提取；提取中动态刷新状态
/**
 * 是否「本页该管」的电费账单文件。
 *
 * 铁律：市电与光伏**共用一个知识库**，本页轮询又是页面级常驻，只要过滤放松一点，
 * 光伏文件在上传后、分类打标落地前的一瞬间就会被本页抢走，调用
 * extract-utility-bill 写成 kind=utility_bill —— 结果是「光伏列表里没有它、
 * 市电列表多一条垃圾行」，还会污染 loadUtilityBillByMonth 的按月取数
 * （真机事故：2026.09光伏账单.pdf 774e6b74）。
 *
 * 所以认领条件收紧为「显式声明是电费」，任一光伏痕迹即排除：
 *   ① 分类/类型/文件名出现「光伏」→ 不是本页的；
 *   ② kind=solar_bill / bill_kind=solar → 不是本页的；
 *   ③ 只有 kind=utility_bill / bill_kind=electricity / fleet_cert_type=市电账单
 *     才认领；完全没打标的一律不碰（等上传流程把标签写进去，或用户手动重提）。
 */
function isElectricityBillFile(item: any): boolean {
  const meta = item?.custom_metadata || {}
  const name = String(item?.file_name || item?.title || '')
  if (meta.kind === 'solar_bill' || meta.bill_kind === 'solar' || meta.fleet_cert_type === '光伏账单') return false
  if (/光伏/.test(name)) return false
  return meta.kind === 'utility_bill' || meta.bill_kind === 'electricity' || meta.fleet_cert_type === '市电账单'
}

const { extractInFlight, extractFailed, pendingFiles, scanOnce, start: startPolling, stop: stopPolling } = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractUtilityBill(kid, fileId),
  // 只在本 Tab 生效：切到光伏/核算/清单时本页不再扫描，从根上避免跨分类抢提
  filter: (item: any) => activeTab.value === 'electricity' && isElectricityBillFile(item),
  onPendingFiles: () => { /* pendingRows computed 直接读 pendingFiles */ },
  // 每轮刷新列表 + 卡片计数：提取落库后 records 会变，卡片不跟着刷就永远停在旧数
  onTick: () => { loadFiles(); void loadTabCounts() },
  // 整批结束（在途清零）：立刻刷一次列表与卡片，用户不用等下一轮 3s
  onSettled: async () => { await loadFiles(true); void loadTabCounts() },
})

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
      void loadOverviewFileStats()
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
    void loadOverviewFileStats()
    await loadTags()
    await loadFiles(true)
    startPolling()
  }
}

// ---- 上传 ----
// 统一走中台上传弹窗：原生 <input type=file> 没有分类锁定、没有队列进度、没有
// 重复文件智能恢复，也无法在「创建记录的第一时间」写分类 → 轮询认领时会串台。
const uploadVisible = ref(false)
const uploadTypeOptions = [{ label: '市电账单', value: '市电账单' }]
const triggerUpload = () => { uploadVisible.value = true }
/** 弹窗内整批任务全部结束：立刻刷新列表与卡片（不等下一轮 3s 轮询）。 */
const onUploadDone = async () => {
  await loadFiles(true)
  void loadTabCounts()
  void scanOnce()
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
const currentRow = ref<Row | null>(null)
// 账单字段编辑抽屉（BSD）：与发票同款底座，字段跟随「字段定义（subs）」
const billDrawerVisible = ref(false)
const billSaving = ref(false)
const billDrawerTitle = computed(() => {
  const r = currentRow.value
  if (!r) return '账单详情'
  const period = String(r.bill_period || r.item?.bill_period || '').trim()
  return period ? `${period} 市电账单` : (r.fileName || '账单详情')
})
// BSD 表单契约 { data, remarkParts, items }：data 直接指向 editForm，
// v-model 写 form.data[key] 即落回 editForm，保存链路完全复用 saveEditForm。
const billFormWrapper = computed(() => ({ data: editForm.value, remarkParts: {}, items: [] }))

function openBillDetailDrawer() {
  if (!currentRow.value) return
  billDrawerVisible.value = true
}
/** 抽屉保存：复用整页详情的保存链路（金额反算/明细覆盖/购销方归一都在后端）。 */
async function saveBillDetail() {
  billSaving.value = true
  try {
    await saveEditForm()
    billDrawerVisible.value = false
    notifyEnergyDataChanged()
  } finally {
    billSaving.value = false
  }
}
const editForm = ref<Record<string, any>>({})
// 不向 BSD 传 :dirty：账单字段编辑即时可保存（billFormWrapper.data 直接指向 editForm）。
/**
 * BSD 抽屉字段：主列表口径的 subs 列（与列表列同源，改字段定义后同步）。
 * ColDef 用 key / fieldType，DetailFieldMeta 用 name / dataType —— 必须显式映射，
 * 直接把 columnDefs 丢进去会渲染出一屏没有 name 的空行。
 */
const detailFieldDefs = computed<Array<{ name: string; label: string; dataType?: string }>>(() =>
  columnDefs.value.map((c) => ({
    name: c.key,
    label: c.label,
    dataType: c.fieldType === 'amount' || c.fieldType === 'number' ? 'number'
      : c.fieldType === 'date' ? 'date' : 'text',
  })),
)
const qualityLabel = (f: string) => f
const autoSaving = ref(false)
let autoSaveDirty = false
let feeItemsSnapshot = ''


// 详情页标题栏不再挂状态徽标（列表的「状态」列已表达同一信息），
// 因此这里不再维护 currentStatus 计算属性。

const openDetail = async (row: Row) => {
  currentRow.value = row
  await loadEditForm(row)
  // 电费账单 → 进入分层详情视图
  detailMode.value = true
  activeMenu.value = 'overview'
}

// 提取质量与溯源：详情接口的 custom_metadata（按账单页定位记录 index）
const detailQuality = ref<ExtractQuality>(EMPTY_QUALITY)

const loadEditForm = async (row: Row) => {
  autoSaveDirty = false
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const records = Array.isArray(meta.records) ? meta.records : []
    const idx = Math.max(0, (row.page || 1) - 1)
    detailQuality.value = parseExtractQuality(meta, records.length > 1 ? idx : undefined)
    const item = records[idx] || row.item || {}
    feeOverrides.value = (item.overview_overrides && typeof item.overview_overrides === 'object') ? { ...item.overview_overrides } : {}
    editForm.value = { ...item, fee_items: Array.isArray(item.fee_items) ? item.fee_items.map((f: any) => ({ ...f })) : [] }
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  } catch {
    // 详情拉取失败：质量面板保持静默，不阻塞手工编辑
    detailQuality.value = EMPTY_QUALITY
    feeOverrides.value = {}
    editForm.value = { ...row.item, fee_items: Array.isArray(row.item?.fee_items) ? row.item.fee_items.map((f: any) => ({ ...f })) : [] }
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  }
}


// ---- 详情视图（分层菜单） ----
const detailMode = ref(false)
const activeMenu = ref('overview')

/** 退出详情：回到列表并重载（详情里可能改过金额/分类，列表必须重新拉）。 */
const exitDetail = () => {
  detailMode.value = false
  activeMenu.value = 'overview'
  loadFiles(true)
}

// 详情视图的菜单结构、费用归属判定、行配置匹配、容需量/功率因数字段
// 全部来自 utilityBillDetail（纯函数，可单测）；这里只保留与响应式状态
// 相关的包装：把 allFieldConfigs / editForm 喂进去。
// 详情视图的菜单结构、费用归属判定、行配置匹配、容需量/功率因数字段
// 全部来自 utilityBillDetail（纯函数，可单测）；这里只保留与响应式状态
// 相关的**同名包装**：把 allFieldConfigs / editForm 喂进去。
// 保留原名（而不是全站改名）是因为模板里 feeRowsOf(activeMenu) / feeMenuOf /
// menuLabel / BILL_MENUS 出现十余次，改名纯属机械操作且容易漏掉模板里的引用。
const feeRowsOf = (key: string) => feeRowsOfUtil(editForm.value.fee_items || [], key, rowConfigsOfUtil(allFieldConfigs.value, ROW_GROUP_OF_MENU[key] || ''))
const feeSubtotalOf = (key: string) => feeSubtotalOfUtil(editForm.value.fee_items || [], key, rowConfigsOfUtil(allFieldConfigs.value, ROW_GROUP_OF_MENU[key] || ''))
const menuLabel = (key: string) => menuLabelOf(key)
const residentMeterRows = computed(() => buildResidentMeterRows(editForm.value, residentRowConfigs.value))
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

// 账单概况的 11 行构造 + 合计全部在 utilityBillOverview（纯函数，可单测）。
// 这里只把「本页的取行方式」注入进去：rowsOf = feeRowsOf（已按行配置过滤）。
//
// 口径提醒（改动必须同步改单测）：
//   · 类别电量取**最大值**而不是求和（分时子项的 qty 是各时段电量，相加会翻倍）；
//   · 概览合计 = 工商业小计 + 居民小计 + 功率因数调整电费。
const overviewRows = computed(() => buildOverviewRows({
  feeItems: editForm.value.fee_items || [],
  rowsOf: (k: string) => feeRowsOf(k),
  editForm: editForm.value,
  overrides: feeOverrides.value,
}))
const qtyLabelOf = (r: any) => qtyLabelOfUtil(r, fmtKwh)
/** 点击概况行跳到对应子表（menuKey 为空的行不可跳）。 */
const gotoMenu = (key: string) => {
  if (!key) return
  activeMenu.value = key
}
const overviewTotal = computed(() => overviewTotalOf(overviewRows.value))

// 账单电费（解析提取）与汇总对比：优先合计电费 grand_total（新口径，含容需量/力调），旧数据回退 total_amount
const billTotal = computed(() => Number(editForm.value.grand_total) || Number(editForm.value.total_amount) || 0)
const billTotalText = computed(() => (billTotal.value ? fmtRate6(billTotal.value) : ''))
const billTotalOk = computed(() => {
  const v = billTotal.value
  if (!v) return true
  return feeClose(overviewTotal.value, v)
})
// 误报：模板第 308 行在属性字符串内引用（`content="差值 {{ fmtRate6(overviewDiff) }}..."`），
// vue-eslint-parser 不解析属性内的 mustache，故在此豁免。
// eslint-disable-next-line @typescript-eslint/no-unused-vars
const overviewDiff = computed(() => {
  const nominal = billTotal.value
  if (!nominal) return 0
  return Math.round((overviewTotal.value - nominal) * 100) / 100
})

// ---- 上期分时数据（按账单周期取最近上一期） ----
// 取数口径在 utilityBillDetail.pickPrevEnergy（字符串比较 YYYY-MM-DD，避开时区坑）。
const prevEnergy = computed(() => pickPrevEnergy(allRecords.value, editForm.value.bill_period_start || ''))

// 分时占比对比表（本期占比 = 时段电量 ÷ 本期总电量 × 100%；
// 尖/峰/谷按公式，**平段取余量**，保证四舍五入后四段合计恰好 100%）
const energyCompareRows = computed(() => buildEnergyCompareRows(editForm.value, prevEnergy.value))

// ---- 用能分析 ECharts（分时电量占比对比：本期 vs 上期） ----
// option 的构造（含配色口径、空柱处理、纵轴上限公式）在 utilityEnergyChart；
// 这里只保留实例的 DOM 生命周期（创建 / resize / dispose）——那是与数据无关的部分。
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
  energyChart.setOption(buildEnergyChartOption(energyCompareRows.value, prevEnergy.value.exists, chartPalette()))
}
watch([detailMode, activeMenu, () => currentRow.value, energyCompareRows, () => editForm.value], () => {
  if (detailMode.value && activeMenu.value === 'overview') renderEnergyChart()
}, { deep: true })
// 关闭详情时销毁实例，避免 v-if 重建后图表挂载到旧容器不显示
watch(detailMode, (v) => {
  if (!v) { energyChart?.dispose(); energyChart = null }
})
onBeforeUnmount(() => { energyChart?.dispose(); energyChart = null })

// 电量明细：行配置派生 + 行匹配 + 排序全部在 utilityBillDetail
// （buildMeterRows 会按配置逐行输出，匹配不到给空串占位而非 0）。
const rowConfigsOf = (group: string) => rowConfigsOfUtil(allFieldConfigs.value, group)
const meterRowConfigs = computed(() => rowConfigsOf('meter-rows').filter((c: any) => c.default_visible !== false))
const residentRowConfigs = computed(() => rowConfigsOf('resident-meter-rows').filter((c: any) => c.default_visible !== false))
const matchRow = (row: any, cfg: any) => matchRowUtil(row, cfg)
const meterRows = computed(() => buildMeterRows(editForm.value, meterRowConfigs.value))
const meterTotal = computed(() => meterRows.value.reduce((s, r) => s + (Number(r.bill_kwh) || 0), 0))

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



// 保存方式：详情/编辑抽屉手动保存（点击「保存」按钮持久化）；行内编辑失焦保存走 commitFeeOverride/commitBillFeeOverride
const saveEditForm = async () => {
  const now = currentRow.value
  if (!now) return
  autoSaving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    let meta = detail?.custom_metadata || {}
    // 兼容历史双层结构：{custom_metadata: {...}} 取内层再保存
    if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
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
    // records 等为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由
    await updateBusinessMetadata(kbId.value, now.knowledgeId, nextMeta)
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
// ---- 抽屉宽度拖动 ----
const DRAWER_WIDTH_KEY = 'weknora-utility-drawer-width'
const DRAWER_DEFAULT_WIDTH = 720
const DRAWER_MIN_WIDTH = 560
const drawerWidth = ref(DRAWER_DEFAULT_WIDTH)
function loadDrawerWidth() {
  try {
    const v = Number(localStorage.getItem(DRAWER_WIDTH_KEY))
    drawerWidth.value = v >= DRAWER_MIN_WIDTH ? v : DRAWER_DEFAULT_WIDTH
  } catch { /* ignore */ }
}

// ---- 浮动工具栏 ----
// 「编辑数据」按钮已下线（第 17 轮）：点行即进详情，工具栏里再放一个编辑入口
// 既与行点击重复，也让工具栏在窄屏下换行。原 handleBatchEdit 随之移除。
/**
 * 「重新解析」：重新跑文档解析（PDF → 文本/表格），解析完成后由轮询自动触发字段提取。
 *
 * 与「重新提取」的分工（第 17 轮产品口径）：
 *   · 重新解析 = 怀疑**解析**结果坏了（扫描件识别错、表格串行）时用，慢但彻底；
 *   · 重新提取 = 解析结果没问题、只想按最新字段规则重算，直接调 extract-utility-bill。
 */
const handleReparse = async () => {
  const row = selectedRows.value
  if (row.length !== 1) { MessagePlugin.info('重新解析仅支持单选'); return }
  const r = row[0]
  extractInFlight.value.add(r.knowledgeId)
  extractFailed.value.delete(r.knowledgeId)
  try {
    await reparseKnowledge(r.knowledgeId)
    MessagePlugin.success(`已触发「${r.fileName}」重新解析，解析完成后将自动提取`)
    // 重解析后文件立刻进入解析态，状态条必须同步亮起
    void scanOnce()
    setTimeout(() => { loadFiles(true); void scanOnce() }, 1500)
  } catch (e: any) {
    extractFailed.value.set(r.knowledgeId, Date.now())
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    extractInFlight.value.delete(r.knowledgeId)
  }
}

const handleReExtract = async () => {
  const row = selectedRows.value
  if (row.length !== 1) { MessagePlugin.info('重新提取仅支持单选'); return }
  const r = row[0]
  extractInFlight.value.add(r.knowledgeId)
  extractFailed.value.delete(r.knowledgeId)
  try {
    // 复用已有解析结果直接重跑字段提取：比「重新解析」快一个量级
    await extractUtilityBill(kbId.value, r.knowledgeId)
    MessagePlugin.success(`已触发「${r.fileName}」重新提取`)
    void scanOnce()
    setTimeout(() => { loadFiles(true); void scanOnce() }, 1500)
  } catch (e: any) {
    extractFailed.value.set(r.knowledgeId, Date.now())
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(r.knowledgeId)
  }
}

// ---- 打印（清单 / 目录 两种版式，共用统一预览弹窗） ----
//
// 「打印清单」= 纵向结算单：账单概况 + 各分项细项（按账单分类逐表）+ 电量明细 + 合计；
// 「打印目录」= 横向汇总表：当前列表**已启用的表头列** × 勾选行。
// 旧版「打印」是把源 PDF 直接合并，与「清单」在业务上完全不是一回事，
// 而且合并件里全是扫描页，租户拿不到结构化金额。
const printVisible = ref(false)
const printUrl = ref('')
const printBytes = ref<Uint8Array | ArrayBuffer | null>(null)
const printCount = ref(0)
const printTitle = ref('打印预览')
const printFileName = ref('preview.pdf')
const printLoadingText = ref('正在生成 PDF…')
const statementBusy = ref(false)
const catalogBusy = ref(false)

/** 列表单元格的**可打印文本**：与屏幕显示同源（金额两位小数、数值去尾 0）。 */
function cellTextOf(c: ColDef, row: any): string {
  const v = row?.[c.key]
  if (v === undefined || v === null || v === '') return ''
  if (c.fieldType === 'amount') return fmtAmount(v)
  if (c.fieldType === 'number') return fmtRate6(v)
  return String(v)
}

const openPrint = (title: string, fileName: string, loading: string, count: number) => {
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
  printBytes.value = null
  printTitle.value = title
  printFileName.value = fileName
  printLoadingText.value = loading
  printCount.value = count
  printVisible.value = true
}
const finishPrint = (bytes: Uint8Array) => {
  printBytes.value = bytes
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
}

/** 打印清单：按所选账单逐份生成「概况 + 分项细项 + 电量明细」。 */
const handlePrintStatement = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  statementBusy.value = true
  openPrint(`账单清单（${rowsSel.length} 份）`, `${rowsSel.length}份市电账单清单.pdf`, '正在生成账单清单…', rowsSel.length)
  try {
    const bytes = await generateStatementPdf({ doc: buildUtilityBillStatementDoc(rowsSel) })
    finishPrint(bytes)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印清单生成失败')
    printVisible.value = false
  } finally {
    statementBusy.value = false
  }
}

/** 打印目录：当前已启用表头列 × 勾选行（横向 A4，列多时自动缩字号不裁列）。 */
const handlePrintCatalog = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  catalogBusy.value = true
  openPrint(`账单目录（${rowsSel.length} 条）`, `${rowsSel.length}条市电账单目录.pdf`, '正在生成账单目录…', rowsSel.length)
  try {
    const columns: CatalogColumn[] = visibleColDefs.value.map((c) => ({
      key: c.key,
      label: c.label,
      value: (r: any) => cellTextOf(c, r),
    }))
    const bytes = await generateCatalogPdf({
      title: '市电账单目录',
      columns,
      rows: rowsSel,
    })
    finishPrint(bytes)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印目录生成失败')
    printVisible.value = false
  } finally {
    catalogBusy.value = false
  }
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
  printBytes.value = null
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
// 列表列格式化统一走 formatNumber.grouped：先按分收敛再千分位，
// 抹掉浮点累加残留（如 53441.77999999996 / 43316.740000000005）。
const fmtKwh = (v: any) => grouped(v)
const fmtMoney = (v: any) => grouped(v, { fixed: true })

// ---- 概览页指标卡（数据均来自子项计算 / 提取字段，非直接填充汇总） ----
// 位置说明：本块依赖 prevEnergy（上期数据）、overviewTotal（子项合计）
// 与 fmtKwh/fmtMoney（格式化），因此**必须排在这些之后**。
// computed 虽是惰性求值，但 `const` 有 TDZ —— 声明在前会让首次求值落在
// 未初始化区，报 ReferenceError（开发模式下 SSR/预渲染时会真踩到）。
// 派生计算全部在 utilityBillMetrics（纯函数，可单测边界：无上期 / 除零 / 持平）。
// 这里只做**同名展开**：模板里用的是 metricKwh / metricKwhDelta / metricKwhTip
// 这类扁平名字，展开后模板零改动。
//
// 口径提醒：metricFee 的输入是 overviewTotal（概览各子项合计），
// 不是票面 grand_total —— 两处算法一旦分叉，卡片和明细表就对不上。
const metrics = computed(() => buildBillMetrics(
  editForm.value,
  overviewTotal.value,
  prevEnergy.value,
  { kwh: fmtKwh, money: fmtMoney },
))
// 说明：`*.delta.delta`（环比原始差值）模板里没有直接用到 ——
// 卡片只展示箭头 + 副文案（sub），悬浮说明里也带了具体数值。
// 因此这里不导出裸差值，避免出现「声明了但没人用」的死变量。
const metricPeriod = computed(() => metrics.value.period)
const metricDue = computed(() => metrics.value.due)
const metricKwh = computed(() => metrics.value.kwh.text)
const metricKwhDeltaArrow = computed(() => metrics.value.kwh.delta.arrow)
const metricKwhDeltaClass = computed(() => metrics.value.kwh.delta.cls)
const metricKwhDeltaSub = computed(() => metrics.value.kwh.delta.sub)
const metricKwhTip = computed(() => metrics.value.kwh.delta.tip)
const metricFee = computed(() => metrics.value.fee.text)
const metricFeeDeltaArrow = computed(() => metrics.value.fee.delta.arrow)
const metricFeeDeltaClass = computed(() => metrics.value.fee.delta.cls)
const metricFeeDeltaSub = computed(() => metrics.value.fee.delta.sub)
const metricFeeTip = computed(() => metrics.value.fee.delta.tip)
// 用能分析
const momText = computed(() => metrics.value.mom.text)
const momDeltaArrow = computed(() => metrics.value.mom.delta.arrow)
const momDeltaClass = computed(() => metrics.value.mom.delta.cls)
const momDeltaSub = computed(() => metrics.value.mom.delta.sub)
const momTip = computed(() => metrics.value.mom.delta.tip)
const pfText = computed(() => metrics.value.pf.text)
const pfDeltaArrow = computed(() => metrics.value.pf.delta.arrow)
const pfDeltaClass = computed(() => metrics.value.pf.delta.cls)
const pfDeltaSub = computed(() => metrics.value.pf.delta.sub)
const pfTip = computed(() => metrics.value.pf.delta.tip)
const avgPriceText = computed(() => metrics.value.avgPrice.text)
const avgDeltaArrow = computed(() => metrics.value.avgPrice.delta.arrow)
const avgDeltaClass = computed(() => metrics.value.avgPrice.delta.cls)
const avgDeltaSub = computed(() => metrics.value.avgPrice.delta.sub)
const avgTip = computed(() => metrics.value.avgPrice.delta.tip)

/** 金额列（空值不占位） */
const fmtAmount = (v: any) => grouped(v, { fallback: '' })
/** 明细电费/单价：最多 6 位小数，去尾 0 */
const fmtRate6 = (v: any) => {
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 6 })
}

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

/** 账单电费列手动编辑（与电费列同机制，存 overview_overrides；重提取后重置） */
const billFeeEditingKey = ref('')
const billFeeEditValue = ref('')
const startBillFeeEdit = (r: any) => {
  billFeeEditingKey.value = r.ovBillKey
  const cur = r.ovBillKey === '__bill_total'
    ? (feeOverrides.value['__bill_total'] !== undefined ? feeOverrides.value['__bill_total'] : billTotal.value)
    : (feeOverrides.value[r.ovBillKey] !== undefined ? feeOverrides.value[r.ovBillKey] : (r.billFee ?? ''))
  billFeeEditValue.value = String(cur ?? '')
  nextTick(() => {
    const el = document.querySelector('.os-fee-input input') as HTMLInputElement | null
    el?.focus()
  })
}
const commitBillFeeOverride = async (r: any) => {
  billFeeEditingKey.value = ''
  if (billFeeEditValue.value === '') return
  const v = Number(billFeeEditValue.value)
  if (!Number.isFinite(v)) return
  feeOverrides.value = { ...feeOverrides.value, [r.ovBillKey]: Math.round(v * 100) / 100 }
  autoSaveDirty = true
  await saveEditForm()
}
const hasBillFeeOverride = (key: string) => feeOverrides.value[key] !== undefined
// ---- Tab 切换 ----
const onTabChange = () => { /* 子组件自行加载 */ }
const switchTab = (tab: EnergyTab) => {
  if (activeTab.value === tab) return
  activeTab.value = tab
  onTabChange()
}
// 设置变更后：重载字段配置并刷新列表
const onSettingsChanged = async () => {
  await loadFieldConfigs()
  loadFiles(true)
  // 专属设置 Tab 里改了表计 / 租户：背后的列表与记录表单要跟着刷新
  await meterTabRef.value?.reloadMeters?.()
  await tenantTabRef.value?.reloadTenants?.()
}

// 子视图（核算/清单/光伏）增删改落库后广播：卡片计数必须跟着重算，
// 否则「保存了一条抄表 → 列表 78 行、卡片还停在 77」。
function onEnergyDataChanged() { void loadTabCounts() }
/**
 * 字段定义保存后：只重建列定义（`loadFieldConfigs` 是一次很小的分类请求），
 * **不重拉账单列表** —— 改列配置不影响任何业务数据，重拉几百 KB 纯属浪费。
 */
function onCategoriesChanged() { void loadFieldConfigs() }

onMounted(() => {
  nextTick(measureTableHeaderFix)
  window.addEventListener(ENERGY_OPEN_SETTINGS_EVENT, onEnergyOpenSettings)
  window.addEventListener(ENERGY_DATA_CHANGED_EVENT, onEnergyDataChanged)
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
  // kbId 由 loadKb() 异步解析：先跑 loadTabCounts 会全部返回 0
  // （那时 kbId 还是空串，账单/光伏接口直接短路）。等它就绪再计数。
  void loadKb().then(() => loadTabCounts())
})
onBeforeUnmount(() => {
  stopPolling()
  window.removeEventListener(ENERGY_OPEN_SETTINGS_EVENT, onEnergyOpenSettings)
  window.removeEventListener(ENERGY_DATA_CHANGED_EVENT, onEnergyDataChanged)
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})
</script>

<style lang="less" scoped>
@import './styles/business-list-shared.css';
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

  &__trailing {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .doc-filter-field {
    display: flex;
    align-items: center;

    &--search {
      flex: 1;
      min-width: 200px;
      max-width: 320px;
    }

    .doc-filter-field__control {
      width: 100%;
    }

    .doc-search {
      width: 200px;
    }

    .doc-date-range {
      width: 160px;
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
  padding: 16px 24px;
  box-sizing: border-box;
}

/* 顶部 KPI 卡片栏：能耗页是 7 张（6 业务 + 历史记录），发票/合同只有 3~4 张。
   中台卡片默认 flex:0 0 auto + min-width:200 + 左右 20px 内边距，7 张在 1920 下
   横向溢出（真机截图第 7 张「历史记录」被裁到只剩图标）。这里只在能耗页
   把卡片改成等分（flex:1 1 0）+ 收紧内边距，7 张稳定落在同一行。 */
.overview-group :deep(.dash-kpi-group) {
  gap: 10px;
}

.overview-group :deep(.dash-kpi-card) {
  flex: 1 1 0;
  min-width: 150px;
  padding: 16px 12px;
  gap: 10px;
}

/* 列表滚动容器：边框/圆角/背景/阴影一律走四模块共享样式（.biz-page .doc-list-scroll），
   不再自定义 —— 此前这里写了 overflow-y:auto + border-radius:9px，与发票的
   overflow:hidden + var(--td-radius-large) 漂移，底栏也就被挤到了容器外面。 */

/* 表头右侧滚动条轨道遮罩：滚动条绘制层级高于 sticky 表头，表头背景盖不住，
   在滚动容器父层挂一块与表头同色的绝对定位色块（宽度=实测滚动条宽、高度=实测表头高），
   垂直滚动条便只在表体区域可见。与发票管理同一套机制、同一组 CSS 变量。 */

/* 列表组件样式（与合同/发票/知识库列表保持一致，scoped 自包含） */
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
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

.bill-detail-edit { margin-left: auto; }
.bill-detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 0 8px;

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
  padding: 6px;
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
  min-height: 0;
  /* 详情页的右侧不是「一张大卡片」，而是若干张并列的卡片：
     这里只负责布局，绝不画边框/背景/内边距。
     之前这里是有 border + #fff 背景 + 16px 内边距的，于是顶部 4 张指标卡
     与下方两张卡片被套在一个大白框里（截图里的红箭头），既是「框套框」，
     又平白吃掉 34px 纵向空间、并把顶部卡片压低到与左侧菜单不齐平。 */
  overflow-y: auto;
  display: flex;
  flex-direction: column;

  /* 卡片之间的唯一间距来源 */
  > * + * {
    margin-top: 12px;
  }
}

.bill-card {
  margin-bottom: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 12px 14px;
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
    margin-bottom: 8px;

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
  /* 间距统一交给 .bill-content 的 margin-top，这里不再叠加 margin-bottom */
  gap: 10px;
  margin-bottom: 0;

  .metric-card {
    display: flex;
    flex-direction: column;
    gap: 1px;
    /* 10px 上下：4 张卡总高从 81px 收到 ~74px，同时保持数值可读 */
    padding: 9px 14px;
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

    /* 指标卡环比：增长红 / 减少绿（与用能分析卡一致） */
    .ea-delta--up { color: var(--td-error-color); }
    .ea-delta--down { color: var(--td-success-color); }
    .ea-delta--flat { color: var(--td-text-color-secondary); }
  }
}

/* 账单概况 + 用能分析 一行两卡片（彼此等高、底边齐平；不撑满底层容器，底部自然留白） */
.overview-two-col {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: stretch;

  > .bill-card {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  > .bill-card:nth-child(2) {
    /* 组件间保持自然间距，不强行撑开 */
    .ea-chart {
      /* 两栏是 stretch 等高：这一栏更高的部分会顶高整行。
         图表是最高的一块，210px 时整行被顶到 514px（一屏放不下），
         收到 160px 后整行约 455px，与左栏的表格高度基本持平。 */
      height: 160px;
      margin-bottom: 8px;
    }
  }
}

/* 用能分析 */
.ea-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-bottom: 8px;

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
    padding: 4px 8px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    margin-bottom: 8px;

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
      padding: 4px 12px;
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
    /* 13 行 × 2px ≈ 26px：单行 28 → 26px，整表从 375 收到 ~350 */
    padding: 4px 12px;
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

.edit-drawer-footer {
  display: flex;
  justify-content: flex-end;
  padding-top: 10px;
  border-top: 1px solid var(--td-component-stroke);
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

<style>
/* 无作用域：TDesign 内部的 td 不带页面的 data-v 属性，scoped 规则命中不到。
 *
 * 可展开行（电费/水费/气费核算的分时明细）TDesign 会加 .t-table__row--folded，
 * 并把 td 背景设成 --td-bg-color-secondarycontainer（#f3f3f3）。发票管理没有可展开行
 * 所以是纯白；不覆盖的话这三张表整表发灰，与发票并排看非常明显。
 */
.biz-page .doc-list-view .t-table__row--folded > td {
  background-color: var(--td-bg-color-container);
}

/* 表头底色统一为 container（白）
 *
 * TDesign 的 sticky / 固定表头会把 th 背景改成 --td-bg-color-secondarycontainer
 * （#f3f3f3）：发票/合同没有开 header-fixed，是纯白；能耗 6 个视图都开了
 * sticky-header，于是表头是灰色。而右侧那块「滚动条轨道遮罩」（.doc-list-view::after）
 * 用的是 container 白 —— 灰表头 + 白遮罩 = 表头最右端露出一块白方块 + 一条竖线，
 * 就是截图里红框指的东西。把 th 拉回白底，遮罩自然隐形，表头也 100% 通栏。
 *
 * 选择器要盖住两种形态：thead 自带 .t-table__header--fixed，以及普通 th。
 */
.biz-page .doc-list-view .t-table th,
.biz-page .doc-list-view .t-table thead > tr > th {
  background-color: var(--td-bg-color-container);
}

/* 遮罩底边与表头底边同色，避免表头右侧 1px 断线 */
.biz-page .doc-list-view::after {
  border-bottom-color: var(--td-component-border);
}
</style>
