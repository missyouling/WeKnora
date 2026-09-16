<template>
  <div class="tenant-billing-container">
    <!-- 筛选工具栏（与电费/合同管理一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-select v-model="useUnit" placeholder="使用单位" filterable class="doc-filter-select doc-filter-field__control"
            :options="useUnitOptions" @change="onUseUnitChange" />
        </div>
        <div class="doc-filter-field">
          <t-date-picker v-model="filters.month" mode="month" placeholder="账单月份" format="YYYY-MM" value-type="YYYY-MM"
            clearable class="doc-date-picker doc-filter-field__control" @change="loadRecords" />
        </div>
        <t-button variant="outline" size="small" @click="loadRecords">
          <template #icon><t-icon name="refresh" size="14px" /></template>
        </t-button>
      </div>
      <div class="doc-filter-bar__trailing">
        <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
          overlay-inner-class="meter-field-popup">
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
              <t-checkbox-group v-model="visibleKeys" class="field-popup-list" @change="persistColumns">
                <t-checkbox v-for="col in activeColDefs" :key="col.key" :value="col.key" class="field-popup-item">
                  {{ col.label }}
                </t-checkbox>
              </t-checkbox-group>
            </div>
          </template>
        </t-popup>
        <t-button variant="outline" size="small" @click="openSettings">
          <template #icon><t-icon name="setting" size="14px" /></template>
          设置
        </t-button>
        <t-button theme="primary" variant="outline" size="small" :loading="generateBusy" :disabled="isOwnerView"
          title="为当前筛选范围内未生成账单的月份生成月度账单（数据齐全时自动生成）" @click="handleGenerateAll">
          <template #icon><t-icon name="refresh" size="14px" /></template>
          生成账单
        </t-button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="doc-list-scroll tenant-list-scroll">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <div class="cell cell-check" role="columnheader" @click.stop>
            <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
              :disabled="!displayRows.length" title="全选" @change="toggleSelectAll" />
          </div>
          <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
            <t-tooltip :content="col.tip || ''" placement="top" :show-arrow="true" :destroy-on-close="false">
              <span class="col-tip">{{ col.label }}</span>
            </t-tooltip>
          </div>
        </div>
        <div class="doc-list-body">
          <div v-for="row in displayRows" :key="row.id" class="doc-list-row"
            :class="{ 'row-selected': selectedKeys.has(row.id) }" :style="gridStyle" role="row" @click="onRowClick(row)">
            <div class="cell cell-check" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(row.id)" @change="(v: any) => toggleSelect(row, v)" />
            </div>
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`">
              <span v-if="col.key === 'month'" class="row-mono">{{ row.month }}</span>
              <span v-else-if="col.key === 'ratio'" class="row-mono">{{ fmtRatio(row.ratio) }}</span>
              <span v-else-if="col.key === 'ind_price'" class="row-mono">{{ fmtPrice(row.ind_price) }}</span>
              <span v-else-if="MONEY_KEYS.includes(col.key)" class="row-mono strong">{{ fmtMoney(row[col.key]) }}</span>
              <span v-else-if="KWH_KEYS.includes(col.key)" class="row-mono">{{ fmtKwh(row[col.key]) }}</span>
              <span v-else class="row-text" :title="String(row[col.key] ?? '')">{{ row[col.key] || '' }}</span>
            </div>
          </div>
          <div v-if="!loading && !displayRows.length" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部汇总 -->
    <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <template v-if="isOwnerView">
        <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
        <span class="doc-summary-item">总电量 <span class="doc-summary-val">{{ fmtKwh(summaryKwh) }}</span> 千瓦时</span>
        <span class="doc-summary-item">总电费 <span class="doc-summary-val">{{ fmtMoney(summaryFee) }}</span> 元</span>
      </template>
      <template v-else>
        <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条记录</span>
        <span class="doc-summary-item">总应付电费 <span class="doc-summary-val">{{ fmtMoney(summaryPayableFee) }}</span> 元</span>
        <span class="doc-summary-item">总应付水费 <span class="doc-summary-val">{{ fmtMoney(summaryWaterFee) }}</span> 元</span>
      </template>
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
            <t-popconfirm theme="warning"
              :content="`确定重新生成所选 ${regeneratableRows.length} 个月度账单吗？将按最新数据覆盖现有账单`"
              :confirm-btn="{ content: '重新生成', theme: 'primary' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleRegenerate">
              <t-button theme="primary" variant="outline" size="small" :disabled="!regeneratableRows.length" :loading="generateBusy" @click.stop>
                <template #icon><t-icon name="refresh" size="14px" /></template>
                重新生成
              </t-button>
            </t-popconfirm>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" :disabled="!printableRows.length" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印清单
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

    <!-- ================= 设置抽屉（租户信息 / 分摊子项） ================= -->
    <teleport to="body">
      <div v-if="settingsVisible" class="doc-drawer-resize-handle" :style="{ right: settingsWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onSettingsResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="settingsVisible" :visible="true" :size="settingsWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="tenant-settings-drawer"
        @close="settingsVisible = false" @update:visible="(v: boolean) => (settingsVisible = v)">
        <template #header>
          <div class="settings-header">
            <span class="settings-header-title">设置</span>
            <div class="settings-header-actions">
              <t-button variant="outline" size="small" @click="openCreateTenant">
                <template #icon><t-icon name="add" /></template>新增租户
              </t-button>
            </div>
          </div>
        </template>
        <t-tabs v-model="settingsTab" class="settings-tabs">
          <t-tab-panel value="tenant" label="租户信息">
            <div class="settings-panel">
              <div class="tenant-cards">
                <!-- 新增租户表单：展开在网格顶部 -->
                <div v-if="tenantFormVisible && !tenantForm.id" class="tenant-form">
                  <div class="tenant-form-title">新增租户</div>
                  <div class="form-grid">
                    <div class="form-item">
                      <label>租户名 <span class="required">*</span></label>
                      <t-input v-model="tenantForm.name" placeholder="如：持睿汽车" />
                    </div>
                    <div class="form-item">
                      <label>租户编号</label>
                      <t-input v-model="tenantForm.tenant_no" placeholder="选填" />
                    </div>
                    <div class="form-item">
                      <label>分摊方式</label>
                      <t-select v-model="tenantForm.allocation_mode" :options="allocationOptions" />
                    </div>
                    <div class="form-item">
                      <label>租赁日期</label>
                      <t-date-picker v-model="tenantForm.lease_start" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                    </div>
                    <div class="form-item">
                      <label>租赁年限（年）</label>
                      <t-input v-model.number="tenantForm.lease_years" type="number" placeholder="选填" />
                    </div>
                    <div class="form-item">
                      <label>单位联系人</label>
                      <t-input v-model="tenantForm.contact" placeholder="选填" />
                    </div>
                    <div class="form-item">
                      <label>联系电话</label>
                      <t-input v-model="tenantForm.phone" placeholder="选填" />
                    </div>
                    <div class="form-item form-item--full">
                      <label>备注</label>
                      <t-textarea v-model="tenantForm.remark" :maxlength="500" placeholder="选填" />
                    </div>
                  </div>
                  <div class="form-actions">
                    <t-button variant="outline" size="small" @click="tenantFormVisible = false">取消</t-button>
                    <t-button theme="primary" size="small" :loading="savingTenant" @click="saveTenantForm">保存</t-button>
                  </div>
                </div>

                <template v-for="t in tenants" :key="t.id">
                  <div class="tenant-card">
                    <div class="tenant-card-head">
                      <span class="tenant-card-name">{{ t.name }}</span>
                      <span class="tenant-card-no">{{ t.tenant_no || '—' }}</span>
                      <span class="tenant-card-actions">
                        <t-button variant="text" size="small" @click="openTenantForm(t)">
                          <template #icon><t-icon name="edit" size="15px" /></template>
                        </t-button>
                        <t-popconfirm theme="warning" :content="`确定删除租户「${t.name}」吗？`"
                          :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                          @confirm="removeTenant(t)">
                          <t-button variant="text" size="small" @click.stop>
                            <template #icon><t-icon name="delete" size="15px" /></template>
                          </t-button>
                        </t-popconfirm>
                      </span>
                    </div>
                    <div class="tenant-card-grid">
                      <div class="tenant-card-item"><span class="k">分摊方式</span><span class="v">{{ t.allocation_mode || '按比例分摊' }}</span></div>
                      <div class="tenant-card-item"><span class="k">联系人</span><span class="v">{{ t.contact || '—' }}</span></div>
                      <div class="tenant-card-item"><span class="k">联系电话</span><span class="v">{{ t.phone || '—' }}</span></div>
                    </div>
                  </div>
                  <!-- 编辑租户表单：展开在当前卡片下方 -->
                  <div v-if="tenantFormVisible && tenantForm.id === t.id" class="tenant-form">
                    <div class="tenant-form-title">编辑租户</div>
                    <div class="form-grid">
                      <div class="form-item">
                        <label>租户名 <span class="required">*</span></label>
                        <t-input v-model="tenantForm.name" placeholder="如：持睿汽车" />
                      </div>
                      <div class="form-item">
                        <label>租户编号</label>
                        <t-input v-model="tenantForm.tenant_no" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>分摊方式</label>
                        <t-select v-model="tenantForm.allocation_mode" :options="allocationOptions" />
                      </div>
                      <div class="form-item">
                        <label>租赁日期</label>
                        <t-date-picker v-model="tenantForm.lease_start" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                      </div>
                      <div class="form-item">
                        <label>租赁年限（年）</label>
                        <t-input v-model.number="tenantForm.lease_years" type="number" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>单位联系人</label>
                        <t-input v-model="tenantForm.contact" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>联系电话</label>
                        <t-input v-model="tenantForm.phone" placeholder="选填" />
                      </div>
                      <div class="form-item form-item--full">
                        <label>备注</label>
                        <t-textarea v-model="tenantForm.remark" :maxlength="500" placeholder="选填" />
                      </div>
                    </div>
                    <div class="form-actions">
                      <t-button variant="outline" size="small" @click="tenantFormVisible = false">取消</t-button>
                      <t-button theme="primary" size="small" :loading="savingTenant" @click="saveTenantForm">保存</t-button>
                    </div>
                  </div>
                </template>
                <div v-if="!tenants.length" class="meter-empty">暂无租户，点击右上角「新增租户」开始配置</div>
              </div>
            </div>
          </t-tab-panel>
          <t-tab-panel value="items" label="分摊子项">
            <div class="settings-panel">
              <div class="item-groups">
                <div v-for="g in itemGroups" :key="g.category" class="item-group">
                  <div class="item-group-head" @click="toggleItemGroup(g.category)">
                    <span class="item-group-name">{{ g.category }}</span>
                    <t-icon :name="expandedItemGroup === g.category ? 'chevron-down' : 'chevron-right'" class="item-group-arrow" />
                  </div>
                  <div v-if="expandedItemGroup === g.category" class="item-group-body">
                    <div class="item-row item-row--head">
                      <span class="item-name">子项名称</span>
                      <span class="item-op">分摊</span>
                    </div>
                    <div v-for="(it, i) in g.items" :key="i" class="item-row">
                      <span class="item-name" :title="it.item_name">{{ it.item_name }}</span>
                      <span class="item-op">
                        <t-switch size="small" :model-value="!!it.enabled" @change="(v: boolean) => toggleItem(it, v)" />
                      </span>
                    </div>
                    <div v-if="!g.items.length" class="item-empty">暂无子项</div>
                  </div>
                </div>
                <div v-if="!itemGroups.length" class="item-empty">暂无子项，生成账单后自动从市电账单引入</div>
              </div>
            </div>
          </t-tab-panel>
          
        </t-tabs>
      </t-drawer>
    </teleport>

    <!-- 账单详情抽屉 -->
    <teleport to="body">
      <div v-if="recordVisible" class="doc-drawer-resize-handle" :style="{ right: recordWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onRecordResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="recordVisible" :visible="true" :header="`${recordDetail?.record?.month || ''} 账单明细`" :size="recordWidth"
        :close-on-overlay-click="true" destroy-on-close class="tenant-record-drawer"
        @close="recordVisible = false" @update:visible="(v: boolean) => (recordVisible = v)">
        <div v-if="recordDetail" class="record-detail">
          <div class="rd-print-title">{{ recordDetail.record.month }} 账单明细</div>
          <div class="record-overview">
            <div class="ro-item">
              <span class="ro-label">使用电量</span>
              <span class="ro-value row-mono">{{ fmtKwh(recordDetail.record.total_kwh) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">线损</span>
              <span class="ro-value row-mono">{{ fmtKwh(recordDetail.record.line_loss) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">分摊比例</span>
              <span class="ro-value row-mono">{{ fmtRatio(recordDetail.record.ratio) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">市电本期电费</span>
              <span class="ro-value row-mono">{{ fmtMoney(recordDetail.record.bill_total_amount) }}</span>
            </div>
          </div>

          <!-- 电量明细卡片（分时电表×时段） -->
          <div v-if="recordDetail.meters?.length" class="rd-card">
            <div class="rd-card-title">电量明细</div>
            <div class="record-items rd-meter-grid">
              <div class="record-items-head rd-meter-head">
                <span>分时电表</span>
                <span>起度</span>
                <span>止度</span>
                <span>倍率</span>
                <span>使用电量</span>
                <span>损耗</span>
                <span>加减电量</span>
                <span>计费电量</span>
                <span>差额分摊电量</span>
              </div>
              <div v-for="(mr, i) in recordDetail.meters" :key="i" class="record-items-row rd-meter-row">
                <span>{{ mr.meter_name }}<span class="rd-period">{{ mr.period }}</span></span>
                <span class="row-mono">{{ fmtNum(mr.prev) }}</span>
                <span class="row-mono">{{ fmtNum(mr.curr) }}</span>
                <span class="row-mono">{{ fmtNum(mr.rate) }}</span>
                <span class="row-mono">{{ fmtKwh(mr.usage) }}</span>
                <span class="row-mono">{{ fmtKwh(mr.line_loss) }}</span>
                <span class="row-mono">{{ fmtKwh(mr.adjust) }}</span>
                <span class="row-mono">{{ fmtKwh(mr.bill_kwh) }}</span>
                <span class="row-mono">{{ fmtKwh(mr.diff_kwh) }}</span>
              </div>
              <div class="record-items-row record-items-total rd-meter-row">
                <span>汇总</span>
                <span></span><span></span><span></span>
                <span class="row-mono">{{ fmtKwh(recordMeterSum('usage')) }}</span>
                <span class="row-mono">{{ fmtKwh(recordMeterSum('line_loss')) }}</span>
                <span class="row-mono">{{ fmtKwh(recordMeterSum('adjust')) }}</span>
                <span class="row-mono">{{ fmtKwh(recordMeterSum('bill_kwh')) }}</span>
                <span class="row-mono">{{ fmtKwh(recordMeterSum('diff_kwh')) }}</span>
              </div>
            </div>
          </div>

          <!-- 费用明细（按大项分组+汇总） -->
          <div class="rd-card">
            <div class="rd-card-title">费用明细</div>
            <div v-for="g in feeGroups" :key="g.category" class="rd-fee-group">
              <div class="rd-fee-group-head">
                <span class="rd-fee-group-name">{{ g.category }}</span>
                <span class="rd-fee-group-sum">小计 {{ fmtMoney(g.sum) }}</span>
              </div>
              <div class="record-items">
                <div class="record-items-head">
                  <span>项目</span>
                  <span>时段</span>
                  <span>电量</span>
                  <span>单价</span>
                  <span>费用</span>
                </div>
                <div v-for="(it, i) in g.rows" :key="i" class="record-items-row" :class="{ 'os-neg': Number(it.fee) < 0 }">
                  <span>{{ it.name }}</span>
                  <span>{{ it.period || '—' }}</span>
                  <span class="row-mono">{{ fmtKwh(it.qty) }}</span>
                  <span class="row-mono">{{ fmtRate(it.rate) }}</span>
                  <span class="row-mono">{{ fmtMoney(it.fee) }}</span>
                </div>
              </div>
            </div>

            <!-- 总电费清单及汇总 -->
            <div class="rd-fee-group">
              <div class="rd-fee-group-head">
                <span class="rd-fee-group-name">总电费清单及汇总</span>
                <span class="rd-fee-group-sum">合计 {{ fmtMoney(totalElecFee) }}</span>
              </div>
              <div class="record-items">
                <div class="record-items-row">
                  <span>工业电费（分摊）</span><span>—</span>
                  <span class="row-mono">{{ fmtKwh(recordDetail.record.total_kwh) }}</span>
                  <span class="row-mono">—</span>
                  <span class="row-mono">{{ fmtMoney(recordDetail.record.industrial_fee) }}</span>
                </div>
                <div class="record-items-row">
                  <span>宿舍电费</span><span>—</span>
                  <span class="row-mono">{{ fmtKwh(recordDetail.record.dorm_kwh) }}</span>
                  <span class="row-mono">{{ fmtRate(recordDetail.record.dorm_kwh ? recordDetail.record.dorm_fee / recordDetail.record.dorm_kwh : 0) }}</span>
                  <span class="row-mono">{{ fmtMoney(recordDetail.record.dorm_fee) }}</span>
                </div>
                <div class="record-items-row record-items-total">
                  <span>合计</span><span></span><span></span><span></span>
                  <span class="row-mono">{{ fmtMoney(totalElecFee) }}</span>
                </div>
              </div>
            </div>

            <!-- 水费清单及汇总（逐表） -->
            <div v-if="recordDetail.waters?.length" class="rd-fee-group">
              <div class="rd-fee-group-head">
                <span class="rd-fee-group-name">水费清单及汇总</span>
                <span class="rd-fee-group-sum">小计 {{ fmtMoney(recordDetail.record.water_fee) }}</span>
              </div>
              <div class="record-items rd-water-grid">
                <div class="record-items-head rd-water-head">
                  <span>水表</span><span>起度</span><span>止度</span><span>用量</span><span>单价</span><span>水费</span>
                </div>
                <div v-for="(wr, i) in recordDetail.waters" :key="i" class="record-items-row rd-water-row">
                  <span>{{ wr.meter_name }}</span>
                  <span class="row-mono">{{ fmtNum(wr.prev) }}</span>
                  <span class="row-mono">{{ fmtNum(wr.curr) }}</span>
                  <span class="row-mono">{{ fmtKwh(wr.usage) }}</span>
                  <span class="row-mono">{{ fmtRate(wr.price) }}</span>
                  <span class="row-mono">{{ fmtMoney(wr.fee) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
        <template #footer>
          <t-button variant="outline" size="small" @click="recordVisible = false">关闭</t-button>
          <t-button theme="primary" size="small" @click="printRecord">打印</t-button>
        </template>
      </t-drawer>
    </teleport>

    <!-- 打印预览 -->
    <div v-if="printVisible" class="tenant-print-mask">
      <div class="tenant-print-dialog">
        <div class="tenant-print-header">
          <span class="tenant-print-title">{{ printTitle }} 打印预览</span>
        </div>
        <div class="tenant-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading"><t-loading size="small" /></div>
        </div>
        <div class="tenant-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, createBillingTenant, getBillingTenant, updateBillingTenant, deleteBillingTenant,
  saveBillingTenantItems,
  listBillingRecords, generateBillingRecord, getBillingRecord, deleteBillingRecord,
  listKnowledgeBases,
  listUtilityBillRecords, listSolarBillRecords, listUtilityMeterRecords, listUtilityMeters,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

// ---- 使用单位(房东/租户) ----
const OWNER_UNIT = '重庆星达'
const BILL_KB_NAME = '日常事务-电费'
const useUnit = ref(OWNER_UNIT)
const kbId = ref('')

// ---- 列定义(按使用单位双视图) ----
interface ColDef { key: string; label: string; default: boolean; w: string; tip?: string }
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
  { key: 'remark', label: '备注', default: false, w: '1.5fr', tip: '手工填写' },
]
const isOwnerView = computed(() => useUnit.value === OWNER_UNIT)
const activeColDefs = computed(() => (isOwnerView.value ? COL_DEFS_OWNER : COL_DEFS_TENANT))
const colStorageKey = computed(() => `weknora-tenant-billing-cols-${isOwnerView.value ? 'owner' : 'tenant'}-v3`)
const visibleKeys = ref<string[]>([])
function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(colStorageKey.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        const valid = arr.filter(k => activeColDefs.value.some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return activeColDefs.value.filter(c => c.default).map(c => c.key)
}
const syncColumnKeys = () => { visibleKeys.value = loadStoredKeys() }
syncColumnKeys()
watch(useUnit, syncColumnKeys)
const visibleColDefs = computed(() => activeColDefs.value.filter(c => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))
const fieldPopupVisible = ref(false)
const selectAllColumns = () => { visibleKeys.value = activeColDefs.value.map(c => c.key); persistColumns() }
const resetColumns = () => { visibleKeys.value = activeColDefs.value.filter(c => c.default).map(c => c.key); persistColumns() }
const persistColumns = () => { try { localStorage.setItem(colStorageKey.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ } }

// ---- 租户与账单 ----
const loading = ref(false)
const tenants = ref<any[]>([])
const activeTenantId = ref('')
const tenantOptions = computed(() => tenants.value.map(t => ({ label: t.name, value: t.id })))
const useUnitOptions = computed(() => {
  const units = new Set<string>([OWNER_UNIT])
  tenants.value.forEach((t: any) => { if (t.name) units.add(t.name) })
  return Array.from(units).map(v => ({ label: v, value: v }))
})
const filters = ref<{ month?: string }>({ month: undefined })
const records = ref<any[]>([])
const displayRows = ref<any[]>([])

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
    await loadAllData()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户失败')
  } finally {
    loading.value = false
  }
}

// 并行加载所有数据源并计算行
const loadAllData = async () => {
  await loadKb()
  const tenantId = activeTenantId.value
  if (!tenantId) { records.value = []; displayRows.value = []; return }
  try {
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
    const [billRes, solarRes, recRes, elecRes, gasRes, wmRes, wrRes]: any[] = await Promise.all(tasks)
    utilityBills.value = Array.isArray(billRes?.data || billRes?.list) ? (billRes.data || billRes.list) : []
    solarBills.value = Array.isArray(solarRes?.data || solarRes?.list) ? (solarRes.data || solarRes.list) : []
    records.value = recRes?.data || []
    elecRows.value = flattenMeterRecords(elecRes)
    gasRows.value = flattenMeterRecords(gasRes)
    waterMeters.value = Array.isArray(wmRes?.data) ? wmRes.data : (Array.isArray(wmRes) ? wmRes : [])
    waterReadings.value = flattenWaterRecords(wrRes)
    buildRows()
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
        meter_kind: meter?.meter_kind || 'dorm',
        meter_type: meter?.meter_type || '',
        use_unit: meter?.use_unit || (it as any).use_unit || '',
        usage: Number(it.usage) || 0,
        unit_price: Number(it.unit_price ?? meter?.default_unit_price) || 0,
        amount: Number(it.amount) || 0,
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
// 电费(定比)= 账单提取 residential_amount(目录电费+政府性基金及附加)；
// 旧账单无该字段时兜底 = catalog_amount + 政府性基金及附加(居民)
const residentInfoOf = (bill: any) => {
  const item = bill?.item || bill || {}
  const rows = Array.isArray(item.residential_readings) ? item.residential_readings : []
  const kwh = Math.round(rows.reduce((s: number, r: any) => s + (Number(r.bill_kwh) || 0), 0) * 100) / 100
  const gov = (Array.isArray(item.fee_items) ? item.fee_items : [])
    .filter((f: any) => String(f.category || '').includes('政府性基金') && Number(f.qty || 0) < 100000 && !String(f.name || '').includes('功率因数'))
    .reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0)
  const fee = Math.round((Number(item.residential_amount) || (Number(item.catalog_amount) || 0) + (Number(gov) || 0)) * 100) / 100
  return { kwh, fee }
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
    const gasOf = () => gasRows.value.filter(r => r.month === month && r.meter_kind !== 'public')
    const base: Record<string, any> = {
      id: `row-${month}`, month, unit: useUnit.value, hasRec: !!rec,
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
      const gas = gasOf()
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
    } else {
      // ===== 持睿(租户)视图 =====
      const totalKwh = Number(bill?.item?.total_kwh) || 0
      const totalFee = Number(bill?.item?.grand_total ?? bill?.item?.total_amount) || 0
      const dorm = dormOf(tenantName)
      const wInd = tenantWater(tenantName, month, false)
      const wDorm = tenantWater(tenantName, month, true)
      const indKwh = Number(rec?.total_kwh) || 0
      const indFee = Number(rec?.industrial_fee) || 0
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
      })
    }
    base.remark = rec?.remark || ''
    return base
  })
  displayRows.value = filters.value.month ? list.filter(r => r.month === filters.value.month) : list
  clearSelection()
}

const loadRecords = async () => {
  await loadAllData()
}

const onTenantChange = async () => {
  clearSelection()
  await loadAllData()
}

const onUseUnitChange = () => {
  clearSelection()
  syncColumnKeys()
  buildRows()
}

// ---- 汇总 ----
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => displayRows.value.filter(r => selectedKeys.value.has(r.id)))
const summary = computed(() => ({ total: (selectedKeys.value.size ? selectedRows.value : displayRows.value).length }))
const summaryKwh = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_kwh) || 0), 0) * 100) / 100
})
const summaryFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_fee) || 0), 0) * 100) / 100
})
const summaryPayableFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.ind_fee) || 0) + (Number(r.dorm_fee) || 0), 0) * 100) / 100
})
const summaryWaterFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.water_ind_fee) || 0) + (Number(r.water_dorm_fee) || 0), 0) * 100) / 100
})
const toggleSelect = (row: any, checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) next.add(row.id)
  else next.delete(row.id)
  selectedKeys.value = next
}
const isAllSelected = computed(() => displayRows.value.length > 0 && displayRows.value.every(r => selectedKeys.value.has(r.id)))
const someSelected = computed(() => displayRows.value.some(r => selectedKeys.value.has(r.id)) && !isAllSelected.value)
const toggleSelectAll = (checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) displayRows.value.forEach(r => next.add(r.id))
  else displayRows.value.forEach(r => next.delete(r.id))
  selectedKeys.value = next
}
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.id])
  if (!isOwnerView.value) {
    const rec = records.value.find((r: any) => r.month === row.month)
    if (rec) openRecord(rec)
  }
}
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 生成月度账单（仅租户视图） ----
const generateBusy = ref(false)
const regeneratableRows = computed(() => {
  if (isOwnerView.value) return []
  return selectedRows.value.filter((r: any) => r.hasRec)
})
// 手动生成：对当前筛选范围未生成账单且市电账单齐备的月份生成；缺项提示并拦截
const handleGenerateAll = async () => {
  if (isOwnerView.value) {
    MessagePlugin.warning('仅租户视图可生成账单')
    return
  }
  if (!activeTenantId.value) return
  const pending = displayRows.value.filter((r: any) => !r.hasRec)
  if (!pending.length) {
    MessagePlugin.warning('当前月份均已生成账单，如需重算请选中记录使用「重新生成」')
    return
  }
  const missingBill = pending.filter((r: any) => !r.billReady)
  const missingReading = pending.filter((r: any) => r.billReady && !r.readingReady)
  if (missingBill.length) {
    MessagePlugin.error(`以下月份缺少市电账单，无法生成：${missingBill.map((r: any) => r.month).join('、')}`)
    return
  }
  if (missingReading.length) {
    MessagePlugin.error(`以下月份缺少星达分表读数，无法生成：${missingReading.map((r: any) => r.month).join('、')}`)
    return
  }
  generateBusy.value = true
  try {
    const okMonths: string[] = []
    const fail: string[] = []
    for (const row of pending) {
      try {
        await generateBillingRecord(activeTenantId.value, { month: row.month })
        okMonths.push(row.month)
      } catch (e: any) {
        fail.push(`${row.month}（${e?.message || '未知错误'}）`)
      }
    }
    if (okMonths.length) MessagePlugin.success(`已生成 ${okMonths.length} 个月度账单：${okMonths.join('、')}`)
    if (fail.length) MessagePlugin.warning(`以下月份生成失败：${fail.join('；')}`)
    await loadAllData()
  } finally {
    generateBusy.value = false
  }
}
// 浮动工具栏：重新生成（覆盖现有账单）
const handleRegenerate = async () => {
  if (!activeTenantId.value || !regeneratableRows.value.length) return
  generateBusy.value = true
  try {
    const months = regeneratableRows.value.map((r: any) => r.month)
    for (const month of months) {
      await generateBillingRecord(activeTenantId.value, { month })
    }
    MessagePlugin.success(`已重新生成 ${months.length} 个月度账单`)
    clearSelection()
    await loadAllData()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新生成失败')
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
  for (const row of pending) {
    try {
      await generateBillingRecord(activeTenantId.value, { month: row.month })
    } catch { /* 缺项由手动生成提示 */ }
  }
  await loadAllData()
}

// ---- 浮动工具栏：打印清单 ----
const catalogBusy = ref(false)
const printableRows = computed(() => (selectedKeys.value.size ? selectedRows.value : displayRows.value))
const handlePrint = async () => {
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
    showPrint(`${arr[0].month} 账单目录（${arr.length} 条）`, bytes)
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
    clearSelection()
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 抽屉拖动调宽(宽度持久化) ----
const DRAWER_W_KEYS = {
  reading: 'tenant_reading_width',
  settings: 'tenant_settings_width',
  record: 'tenant_record_width',
}
const loadDrawerWidth = (key: string, def: string) => {
  try {
    const v = localStorage.getItem(key)
    const n = v ? parseInt(v, 10) : 0
    return n >= 520 && n <= 1100 ? `${n}px` : def
  } catch { return def }
}

// ---- 设置抽屉 ----
const settingsVisible = ref(false)
const settingsWidth = ref(loadDrawerWidth(DRAWER_W_KEYS.settings, '860px'))
const settingsTab = ref('tenant')
const detail = ref<any>(null)
const itemsForm = ref<{ category: string; item_key: string; item_name: string; enabled: boolean }[]>([])

const allocationOptions = [{ label: '按比例分摊', value: '按比例分摊' }]

const openSettings = async () => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  settingsVisible.value = true
  settingsTab.value = 'tenant'
  tenantFormVisible.value = false
  await loadDetail()
}

const loadDetail = async () => {
  if (!activeTenantId.value) return
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    detail.value = res.data
    const d = res.data
    itemsForm.value = (d.items || []).map((it: any) => ({
      category: it.category || '其他费用', item_key: it.item_key, item_name: it.item_name, enabled: !!it.enabled,
    }))
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户详情失败')
  }
}

// ---- 租户卡片(多租户) ----
const tenantFormVisible = ref(false)
const savingTenant = ref(false)
const tenantForm = ref({ id: '', name: '', tenant_no: '', allocation_mode: '按比例分摊', lease_start: '', lease_years: 0, contact: '', phone: '', remark: '' })

const emptyTenantForm = () => ({ id: '', name: '', tenant_no: '', allocation_mode: '按比例分摊', lease_start: '', lease_years: 0, contact: '', phone: '', remark: '' })
const openCreateTenant = async () => {
  settingsVisible.value = true
  settingsTab.value = 'tenant'
  tenantForm.value = emptyTenantForm()
  tenantFormVisible.value = true
  await loadTenants()
}
const openTenantForm = (t: any) => {
  tenantForm.value = {
    id: t.id, name: t.name || '', tenant_no: t.tenant_no || '',
    allocation_mode: t.allocation_mode || '按比例分摊', lease_start: t.lease_start || '',
    lease_years: Number(t.lease_years) || 0, contact: t.contact || '', phone: t.phone || '', remark: t.remark || '',
  }
  tenantFormVisible.value = true
}
const saveTenantForm = async () => {
  if (!tenantForm.value.name.trim()) {
    MessagePlugin.warning('请输入租户名')
    return
  }
  savingTenant.value = true
  try {
    const payload: Record<string, unknown> = {
      name: tenantForm.value.name.trim(),
      tenant_no: tenantForm.value.tenant_no,
      allocation_mode: tenantForm.value.allocation_mode || '按比例分摊',
      lease_start: tenantForm.value.lease_start || null,
      lease_years: Number(tenantForm.value.lease_years) || 0,
      contact: tenantForm.value.contact,
      phone: tenantForm.value.phone,
      remark: tenantForm.value.remark,
    }
    if (tenantForm.value.id) {
      await updateBillingTenant(tenantForm.value.id, payload)
    } else {
      const res: any = await createBillingTenant(payload)
      if (!activeTenantId.value) activeTenantId.value = res.data.id
    }
    MessagePlugin.success('已保存')
    tenantFormVisible.value = false
    await loadTenants()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingTenant.value = false
  }
}
const removeTenant = async (t: any) => {
  try {
    await deleteBillingTenant(t.id)
    MessagePlugin.success('已删除')
    if (activeTenantId.value === t.id) {
      activeTenantId.value = ''
      tenants.value = tenants.value.filter(x => x.id !== t.id)
      if (tenants.value.length) activeTenantId.value = tenants.value[0].id
    }
    await loadTenants()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 分摊子项（大类分组 + 内联展开 + 子项开关） ----
const expandedItemGroup = ref('')

const itemGroups = computed(() => {
  const g: Record<string, any[]> = {}
  itemsForm.value.forEach((it) => {
    const cat = it.category || '其他费用'
    ;(g[cat] = g[cat] || []).push(it)
  })
  return Object.entries(g).map(([category, items]) => ({
    category,
    items,
  }))
})
const toggleItemGroup = (category: string) => {
  expandedItemGroup.value = expandedItemGroup.value === category ? '' : category
}
const toggleItem = async (it: any, v: boolean) => {
  it.enabled = !!v
  await saveItems()
}
const saveItems = async () => {
  if (!activeTenantId.value) return
  try {
    const items = itemsForm.value
      .filter((it: any) => it.item_name.trim())
      .map((it: any) => ({
        category: it.category || '其他费用',
        item_key: `${(it.category || '其他费用').trim()}|${it.item_name.trim()}`, item_name: it.item_name.trim(), enabled: !!it.enabled,
      }))
    await saveBillingTenantItems(activeTenantId.value, { items })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存子项失败')
  }
}

// ---- 分时电表 CRUD(按归属单位分组 + 内联表单) ----
// ---- 账单详情与打印 ----
const recordVisible = ref(false)
const recordWidth = ref(loadDrawerWidth(DRAWER_W_KEYS.record, '760px'))
const recordDetail = ref<any>(null)
const printVisible = ref(false)
const printUrl = ref('')
const printTitle = ref('')

const openRecord = async (r: any) => {
  try {
    const res: any = await getBillingRecord(r.id)
    recordDetail.value = res.data
    recordVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

// 直接打印明细页面（浏览器打印，默认纵向 A4）
// 长内容可多页分页：克隆明细到 body 顶层(脱离抽屉 fixed 容器)后打印,每个大项分组尽量不跨页
const printRecord = () => {
  if (!recordDetail.value) return
  const src = document.querySelector('.record-detail') as HTMLElement | null
  if (!src) return
  const clone = src.cloneNode(true) as HTMLElement
  clone.id = 'rd-print-clone'
  clone.style.cssText = 'position:absolute;left:0;top:0;width:100%;background:#fff;padding:16px;box-sizing:border-box;'
  document.body.appendChild(clone)
  window.print()
  setTimeout(() => { clone.remove() }, 200)
}

const showPrint = (title: string, bytes: ArrayBuffer | Uint8Array) => {
  printTitle.value = title
  if (printUrl.value) URL.revokeObjectURL(printUrl.value)
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
  printVisible.value = true
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 抽屉拖动调宽(宽度持久化) ----
const onResize = (e: MouseEvent, widthRef: { value: string }, storageKey: string) => {
  const startX = e.clientX
  const startW = parseFloat(widthRef.value)
  const maxW = Math.min(1100, Math.floor(window.innerWidth * 0.95))
  const move = (ev: MouseEvent) => {
    const w = Math.min(maxW, Math.max(520, startW + (startX - ev.clientX)))
    widthRef.value = `${w}px`
  }
  const up = () => {
    try { localStorage.setItem(storageKey, widthRef.value) } catch { /* ignore */ }
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}
const onSettingsResizeStart = (e: MouseEvent) => onResize(e, settingsWidth, DRAWER_W_KEYS.settings)
const onRecordResizeStart = (e: MouseEvent) => onResize(e, recordWidth, DRAWER_W_KEYS.record)

// ---- 账单明细分组（按大项排序 + 汇总） ----
const FEE_CATEGORY_ORDER = ['市场化购电费', '上网环节线损', '输配电', '系统运行费', '政府性基金及附加', '居民', '基本电费', '功率因素调整电费']
const feeGroups = computed(() => {
  const items = (recordDetail.value?.items || []).filter((it: any) => it.category !== '水费')
  const map = new Map<string, any[]>()
  const order: string[] = []
  for (const it of items) {
    const cat = it.category || '其他费用'
    if (!map.has(cat)) { map.set(cat, []); order.push(cat) }
    map.get(cat)!.push(it)
  }
  order.sort((a, b) => {
    const ia = FEE_CATEGORY_ORDER.findIndex(c => a.includes(c))
    const ib = FEE_CATEGORY_ORDER.findIndex(c => b.includes(c))
    return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
  })
  return order.map(cat => ({
    category: cat,
    rows: map.get(cat)!,
    sum: Math.round(map.get(cat)!.reduce((s, it) => s + (Number(it.fee) || 0), 0) * 100) / 100,
  }))
})
const recordMeterSum = (key: string): number =>
  Math.round((recordDetail.value?.meters || []).reduce((s: number, mr: any) => s + (Number(mr[key]) || 0), 0) * 100) / 100
// 总电费 = 工业(分摊) + 宿舍,不含水费(水费归水费清单)
const totalElecFee = computed(() =>
  Math.round(((Number(recordDetail.value?.record?.total_fee) || 0) - (Number(recordDetail.value?.record?.water_fee) || 0)) * 100) / 100)

// ---- 格式化 ----
const fmtMoney = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
const fmtKwh = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const fmtNum = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return String(parseFloat(n.toFixed(4)))
}
const fmtRate = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}
const fmtRatio = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return (n * 100).toFixed(2) + '%'
}
const MONEY_KEYS = ['total_fee', 'ind_fee', 'res_fee', 'dorm_fee', 'water_ind_fee', 'water_dorm_fee', 'water_fire_fee', 'gas_fee', 'solar_amount']
const KWH_KEYS = ['total_kwh', 'ind_kwh', 'res_kwh', 'dorm_kwh', 'water_ind_usage', 'water_dorm_usage', 'water_fire_usage', 'gas_usage', 'solar_gen', 'solar_grid']
const printLoaded = ref(false)
const cellText = (key: string, row: any): string => {
  if (key === 'ratio') return fmtRatio(row.ratio)
  if (key === 'ind_price') return fmtPrice(row.ind_price)
  if (MONEY_KEYS.includes(key)) return fmtMoney(row[key])
  if (KWH_KEYS.includes(key)) return fmtKwh(row[key])
  return row[key] || ''
}
const fmtPrice = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(4)))
}

onMounted(() => {
  loadTenants()
})
</script>

<style lang="less" scoped>
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
    .doc-date-picker { width: 140px; }
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
.os-neg { color: var(--td-error-color, #d54941); }
.strong { font-weight: 600; }

.tenant-list-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.meter-empty {
  padding: 40px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);
  .meter-empty-icon { color: var(--td-text-color-placeholder); }
  .meter-empty-text { font-size: 13px; color: var(--td-text-color-placeholder); }
}

/* 底部汇总 */
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
  &.is-batch-visible { margin-bottom: 72px; }
}

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
.tenant-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  .tenant-print-dialog {
    width: min(960px, 92vw);
    height: min(720px, 88vh);
    background: var(--td-bg-color-container);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    .tenant-print-header {
      display: flex;
      align-items: center;
      padding: 10px 16px;
      border-bottom: 1px solid var(--td-component-stroke);
      .tenant-print-title { font-size: 14px; font-weight: 600; }
    }
    .tenant-print-body {
      flex: 1;
      min-height: 0;
      .print-preview-frame { width: 100%; height: 100%; border: none; }
      .print-preview-loading { height: 100%; display: flex; align-items: center; justify-content: center; }
    }
    .tenant-print-footer {
      display: flex;
      justify-content: flex-end;
      padding: 10px 16px;
      border-top: 1px solid var(--td-component-stroke);
    }
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
    background: #fff !important;
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
