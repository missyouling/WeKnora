<template>
  <div class="tenant-billing-container">
    <!-- 筛选工具栏（与电费/合同管理一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-select v-model="activeTenantId" placeholder="租户" filterable class="doc-filter-select doc-filter-field__control"
            :options="tenantOptions" @change="onTenantChange" />
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
                <t-checkbox v-for="col in COLUMN_DEFS" :key="col.key" :value="col.key" class="field-popup-item">
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
        <t-button theme="primary" size="small" @click="openReadingDrawer('')">
          <template #icon><t-icon name="add" /></template>
          新增记录
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
            {{ col.label }}
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
              <span v-else-if="col.key === 'total_fee'" class="row-mono strong">{{ fmtMoney(row.total_fee) }}</span>
              <span v-else-if="['total_kwh', 'line_loss'].includes(col.key)" class="row-mono">{{ fmtKwh(row[col.key]) }}</span>
              <span v-else-if="['industrial_fee', 'dorm_fee', 'water_fee', 'bill_total_amount'].includes(col.key)" class="row-mono">{{ fmtMoney(row[col.key]) }}</span>
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
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
      <span class="doc-summary-item">星达电量 <span class="doc-summary-val">{{ fmtKwh(summaryKwh) }}</span> 千瓦时</span>
      <span class="doc-summary-item">总应付 <span class="doc-summary-val">{{ fmtMoney(summaryFee) }}</span> 元</span>
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
            <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="openEditSelected">
              <template #icon><t-icon name="edit" size="14px" /></template>
              编辑
            </t-button>
            <t-dropdown @click="onPrintMenu">
              <t-button theme="default" variant="outline" size="small" :loading="catalogBusy">
                <template #icon><t-icon name="print" size="14px" /></template>
                打印
              </t-button>
              <template #dropdown>
                <t-dropdown-menu>
                  <t-dropdown-item value="catalog" :disabled="!printableRows.length">打印清单</t-dropdown-item>
                  <t-dropdown-item value="detail" :disabled="selectedRows.length !== 1">打印详情</t-dropdown-item>
                </t-dropdown-menu>
              </template>
            </t-dropdown>
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

    <!-- ================= 新增记录抽屉（分时读数） ================= -->
    <teleport to="body">
      <div v-if="readingVisible" class="doc-drawer-resize-handle" :style="{ right: readingWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onReadingResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="readingVisible" :visible="true" :header="readingTitle" :size="readingWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="tenant-reading-drawer"
        @close="readingVisible = false" @update:visible="(v: boolean) => (readingVisible = v)">
        <div class="reading-body">
          <div class="rec-grid">
            <div class="rec-field">
              <label>月份 <span class="required">*</span></label>
              <t-date-picker v-model="readingMonth" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份" />
            </div>
            <div class="rec-field">
              <label>电表 <span class="required">*</span></label>
              <t-select v-model="curMeterId" :options="meterEditOptions" filterable placeholder="选择电表" @change="onMeterChange" />
            </div>

            <div class="rec-field">
              <label>抄表日期</label>
              <t-date-picker v-model="readingDate" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选择日期" clearable />
            </div>
            <div class="rec-field">
              <label>抄表人</label>
              <t-input v-model="readingReader" placeholder="默认取表计管理人员" />
            </div>

            <div class="rec-field">
              <label>录入日期</label>
              <div class="readonly-val">{{ readingRecordDate || today }}</div>
            </div>
            <div class="rec-field">
              <label>倍率</label>
              <div class="readonly-val">{{ fmtNum(curMeter?.rate) }}×</div>
            </div>
          </div>

          <div class="period-grid">
            <div v-for="p in periods" :key="p.key" class="period-row" :class="{ 'row-invalid': rdCurInvalid(p.key) }">
              <span class="period-label">{{ p.label }}</span>
              <t-input class="period-input" :model-value="curForm[p.key + '_prev']" type="number" size="small"
                placeholder="起度" :status="rdCurInvalid(p.key) ? 'error' : ''"
                @update:model-value="(v: string) => setCurVal(p.key + '_prev', v)" />
              <span class="rr-sep">~</span>
              <t-input class="period-input" :model-value="curForm[p.key + '_curr']" type="number" size="small"
                placeholder="止度" :status="rdCurInvalid(p.key) ? 'error' : ''"
                @update:model-value="(v: string) => setCurVal(p.key + '_curr', v)" />
              <span class="period-kwh">{{ fmtKwh(curPeriodKwh(p.key)) }} 千瓦时</span>
            </div>
          </div>

          <div class="calc-val">
            <span>{{ curMeter?.name || '' }}合计</span>
            <b class="row-mono">{{ fmtKwh(curTotalKwh) }}</b>
            <span>千瓦时</span>
          </div>

          <div class="rec-field rec-field--wide">
            <label>备注</label>
            <t-textarea v-model="curForm.remark" :maxlength="500" placeholder="选填" />
          </div>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="readingVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="savingReadings" @click="saveCurrentAndNext">保存</t-button>
        </div>
      </t-drawer>
    </teleport>

    <!-- ================= 设置抽屉（租户管理 / 电表设置 / 分摊子项） ================= -->
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
          <t-tab-panel value="meters" label="电表设置">
            <div class="settings-panel">
              <div v-for="g in meterGroups" :key="g.owner" class="meter-section">
                <div class="meter-section-head">
                  <span class="meter-section-title">{{ g.owner || '未分组' }}</span>
                  <t-button variant="outline" size="small" @click="openAddMeter(g.owner)">
                    <template #icon><t-icon name="add" /></template>新增
                  </t-button>
                </div>
                <div class="meter-grid">
                  <!-- 新增表计：表单展开在分组顶部 -->
                  <div v-if="meterFormVisible && !meterForm.id && meterForm.owner_unit === g.owner" class="meter-form">
                    <div class="meter-form-title">新增电表</div>
                    <div class="form-grid">
                      <div class="form-item">
                        <label>别名 <span class="required">*</span></label>
                        <t-input v-model="meterForm.name" placeholder="如：总表1 / 分表1" />
                      </div>
                      <div class="form-item">
                        <label>表号</label>
                        <t-input v-model="meterForm.meter_no" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>倍率</label>
                        <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                      </div>
                      <div class="form-item">
                        <label>类型</label>
                        <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
                      </div>
                      <div class="form-item">
                        <label>归属单位</label>
                        <t-input v-model="meterForm.owner_unit" placeholder="如：星达铜业" />
                      </div>
                      <div class="form-item">
                        <label>使用单位</label>
                        <t-input v-model="meterForm.use_unit" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>管理人员</label>
                        <t-input v-model="meterForm.manager" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>联系方式</label>
                        <t-input v-model="meterForm.contact" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>抄表方式</label>
                        <t-radio-group v-model="meterForm.meter_mode">
                          <t-radio-button value="auto">自动抄表</t-radio-button>
                          <t-radio-button value="manual">手动抄表</t-radio-button>
                        </t-radio-group>
                      </div>
                      <div class="form-item">
                        <label>安装日期</label>
                        <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                      </div>
                      <div class="form-item form-item--full">
                        <label>备注</label>
                        <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
                      </div>
                    </div>
                    <div class="form-actions">
                      <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                      <t-button theme="primary" size="small" :loading="savingMeter" @click="submitMeter">保存</t-button>
                    </div>
                  </div>

                  <template v-for="m in g.meters" :key="m.id">
                    <div class="meter-card">
                      <div class="meter-card-head">
                        <span class="meter-card-name">{{ m.name }}</span>
                        <t-switch :model-value="!!m.enabled" size="small" @change="(v: any) => toggleMeter(m, v)" />
                        <span class="meter-card-actions">
                          <t-button variant="text" size="small" @click="openEditMeter(m)">
                            <template #icon><t-icon name="edit" size="15px" /></template>
                          </t-button>
                          <t-popconfirm theme="warning" :content="`确定删除电表「${m.name}」吗？`"
                            :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                            @confirm="removeMeter(m)">
                            <t-button variant="text" size="small" @click.stop>
                              <template #icon><t-icon name="delete" size="15px" /></template>
                            </t-button>
                          </t-popconfirm>
                        </span>
                      </div>
                      <div class="meter-card-grid">
                        <div class="meter-card-item"><span class="k">表号</span><span class="v">{{ m.meter_no || '—' }}</span></div>
                        <div class="meter-card-item"><span class="k">倍率</span><span class="v">{{ fmtNum(m.rate) }}</span></div>
                        <div class="meter-card-item"><span class="k">类型</span><span class="v">{{ m.meter_kind === 'normal' ? '普通' : '分时' }}</span></div>
                      </div>
                    </div>
                    <!-- 编辑表计：表单展开在当前卡片下方 -->
                    <div v-if="meterFormVisible && meterForm.id === m.id" class="meter-form">
                      <div class="meter-form-title">编辑电表</div>
                      <div class="form-grid">
                        <div class="form-item">
                          <label>别名 <span class="required">*</span></label>
                          <t-input v-model="meterForm.name" placeholder="如：总表1 / 分表1" />
                        </div>
                        <div class="form-item">
                          <label>表号</label>
                          <t-input v-model="meterForm.meter_no" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>倍率</label>
                          <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                        </div>
                        <div class="form-item">
                          <label>类型</label>
                          <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
                        </div>
                        <div class="form-item">
                          <label>归属单位</label>
                          <t-input v-model="meterForm.owner_unit" placeholder="如：星达铜业" />
                        </div>
                        <div class="form-item">
                          <label>使用单位</label>
                          <t-input v-model="meterForm.use_unit" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>管理人员</label>
                          <t-input v-model="meterForm.manager" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>联系方式</label>
                          <t-input v-model="meterForm.contact" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>抄表方式</label>
                          <t-radio-group v-model="meterForm.meter_mode">
                            <t-radio-button value="auto">自动抄表</t-radio-button>
                            <t-radio-button value="manual">手动抄表</t-radio-button>
                          </t-radio-group>
                        </div>
                        <div class="form-item">
                          <label>安装日期</label>
                          <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                        </div>
                        <div class="form-item form-item--full">
                          <label>备注</label>
                          <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
                        </div>
                      </div>
                      <div class="form-actions">
                        <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                        <t-button theme="primary" size="small" :loading="savingMeter" @click="submitMeter">保存</t-button>
                      </div>
                    </div>
                  </template>
                  <div v-if="!g.meters.length && !(meterFormVisible && !meterForm.id && meterForm.owner_unit === g.owner)" class="meter-empty">暂无电表</div>
                </div>
              </div>
            </div>
          </t-tab-panel>
          <t-tab-panel value="items" label="分摊子项">
            <div class="settings-panel">
              <div class="item-groups">
                <div v-for="g in itemGroups" :key="g.category" class="item-group">
                  <div class="item-group-head" @click="toggleItemGroup(g.category)">
                    <span class="item-group-name">{{ g.category }}</span>
                    <span class="item-group-count">{{ g.enabledCount }}/{{ g.items.length }}</span>
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
                    <p class="field-hint item-hint">子项引用自市电账单，不支持新增与删除</p>
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
        :footer="false" :close-on-overlay-click="true" destroy-on-close class="tenant-record-drawer"
        @close="recordVisible = false" @update:visible="(v: boolean) => (recordVisible = v)">
        <div v-if="recordDetail" class="record-detail">
          <div class="record-overview">
            <div class="ro-item">
              <span class="ro-label">星达电量</span>
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
          <div class="record-items">
            <div class="record-items-head">
              <span>项目</span>
              <span>时段</span>
              <span>电量</span>
              <span>单价</span>
              <span>费用</span>
            </div>
            <div v-for="(it, i) in recordDetail.items" :key="i" class="record-items-row" :class="{ 'os-neg': Number(it.fee) < 0 }">
              <span>{{ it.name }}</span>
              <span>{{ it.period || '—' }}</span>
              <span class="row-mono">{{ fmtKwh(it.qty) }}</span>
              <span class="row-mono">{{ fmtRate(it.rate) }}</span>
              <span class="row-mono">{{ fmtMoney(it.fee) }}</span>
            </div>
            <div class="record-items-row record-items-total">
              <span>合计</span>
              <span></span>
              <span></span>
              <span></span>
              <span class="row-mono">{{ fmtMoney(recordDetail.record.total_fee) }}</span>
            </div>
          </div>
        </div>
        <template #footer>
          <t-button variant="outline" size="small" @click="recordVisible = false">关闭</t-button>
          <t-button theme="primary" size="small" :loading="printBusy" @click="printRecord">打印</t-button>
        </template>
      </t-drawer>
    </teleport>

    <!-- 打印预览 -->
    <div v-if="printVisible" class="tenant-print-mask">
      <div class="tenant-print-dialog">
        <div class="tenant-print-header">
          <span class="tenant-print-title">{{ printTitle }} 打印预览</span>
          <div class="tenant-print-actions">
            <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
            <t-button theme="primary" size="small" @click="doBrowserPrint">打印</t-button>
          </div>
        </div>
        <div class="tenant-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading"><t-loading size="small" /></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, createBillingTenant, getBillingTenant, updateBillingTenant, deleteBillingTenant,
  listBillingTimeMeters, createBillingTimeMeter, updateBillingTimeMeter, deleteBillingTimeMeter,
  getBillingTimeReading, saveBillingTimeReading,
  saveBillingTenantItems,
  listBillingRecords, generateBillingRecord, getBillingRecord, deleteBillingRecord,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

// ---- 列定义 ----
interface ColDef { key: string; label: string; default: boolean; w: string }
const COLUMN_DEFS: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr' },
  { key: 'total_kwh', label: '星达电量', default: true, w: '1fr' },
  { key: 'line_loss', label: '线损', default: true, w: '0.9fr' },
  { key: 'ratio', label: '分摊比例', default: true, w: '0.9fr' },
  { key: 'industrial_fee', label: '工业电费', default: true, w: '1fr' },
  { key: 'dorm_fee', label: '宿舍电费', default: true, w: '1fr' },
  { key: 'water_fee', label: '水费', default: true, w: '0.9fr' },
  { key: 'total_fee', label: '总应付', default: true, w: '1.1fr' },
  { key: 'bill_total_amount', label: '市电电费', default: false, w: '1fr' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr' },
]
const STORAGE_KEY = 'weknora-tenant-billing-columns-v1'
const visibleKeys = ref<string[]>(loadStoredKeys())
const visibleColDefs = computed(() => COLUMN_DEFS.filter(c => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))
function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        const valid = arr.filter(k => COLUMN_DEFS.some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return COLUMN_DEFS.filter(c => c.default).map(c => c.key)
}
const fieldPopupVisible = ref(false)
const selectAllColumns = () => { visibleKeys.value = COLUMN_DEFS.map(c => c.key); persistColumns() }
const resetColumns = () => { visibleKeys.value = COLUMN_DEFS.filter(c => c.default).map(c => c.key); persistColumns() }
const persistColumns = () => { try { localStorage.setItem(STORAGE_KEY, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ } }

// ---- 租户与账单 ----
const loading = ref(false)
const tenants = ref<any[]>([])
const activeTenantId = ref('')
const tenantOptions = computed(() => tenants.value.map(t => ({ label: t.name, value: t.id })))
const filters = ref<{ month?: string }>({ month: undefined })
const records = ref<any[]>([])
const displayRows = ref<any[]>([])

const loadTenants = async () => {
  loading.value = true
  try {
    const res: any = await listBillingTenants()
    tenants.value = res.data || []
    if (tenants.value.length && !activeTenantId.value) {
      activeTenantId.value = tenants.value[0].id
    }
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户失败')
  } finally {
    loading.value = false
  }
}

const loadRecords = async () => {
  if (!activeTenantId.value) return
  try {
    const res: any = await listBillingRecords(activeTenantId.value)
    records.value = res.data || []
    let list = records.value
    if (filters.value.month) list = list.filter(r => r.month === filters.value.month)
    displayRows.value = list
    clearSelection()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

const onTenantChange = async () => {
  clearSelection()
  await loadRecords()
}

// ---- 汇总 ----
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => records.value.filter(r => selectedKeys.value.has(r.id)))
const summary = computed(() => ({ total: (selectedKeys.value.size ? selectedRows.value : displayRows.value).length }))
const summaryKwh = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_kwh) || 0), 0) * 100) / 100
})
const summaryFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_fee) || 0), 0) * 100) / 100
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
  openRecord(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }
const openEditSelected = () => {
  const r = selectedRows.value[0]
  if (r) openReadingDrawer(r.month)
}

// ---- 浮动工具栏：打印清单 / 打印详情 / 删除 ----
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
      title: `租户月度账单目录`,
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
const onPrintMenu = (data: any) => {
  if (data?.value === 'catalog') handlePrint()
  else if (data?.value === 'detail') printSelectedDetail()
}
const printSelectedDetail = async () => {
  const r = selectedRows.value[0]
  if (!r) return
  catalogBusy.value = true
  try {
    const res: any = await getBillingRecord(r.id)
    const rec = res.data.record
    const items = res.data.items || []
    const columns: CatalogColumn[] = [
      { key: 'name', label: '项目', value: (r: any) => r.name },
      { key: 'period', label: '时段', value: (r: any) => r.period || '—' },
      { key: 'qty', label: '电量', value: (r: any) => fmtKwh(r.qty) },
      { key: 'rate', label: '单价', value: (r: any) => fmtRate(r.rate) },
      { key: 'fee', label: '费用（元）', value: (r: any) => fmtMoney(r.fee) },
    ]
    const totalRow = { name: '合计', period: '', qty: 0, rate: 0, fee: rec.total_fee }
    const bytes = await generateCatalogPdf({
      title: `${rec.month} 租户电费账单明细`,
      columns,
      rows: [...items, totalRow],
    })
    showPrint(`${rec.month} 电费账单详情`, bytes)
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

// ---- 新增记录抽屉（分时读数：单表连续录入） ----
const readingVisible = ref(false)
const readingWidth = ref('820px')
const savingReadings = ref(false)
const readingMonth = ref('')
const readingTitle = computed(() => {
  if (!readingMonth.value) return '新增记录'
  const total = readingQueue.value.length
  return `${readingMonth.value} 分时读数${total ? `（${curIdx.value + 1}/${total}）` : ''}`
})
const readingDate = ref('')
const readingReader = ref('')
const readingRecordDate = ref('')
const curForm = ref<Record<string, any>>({})
const curIdx = ref(0)
const curMeterId = ref('')
const periods = [
  { key: 'deep', label: '尖' },
  { key: 'peak', label: '峰' },
  { key: 'flat', label: '平' },
  { key: 'valley', label: '谷' },
]
const tenantName = computed(() => tenants.value.find((t: any) => t.id === activeTenantId.value)?.name || '')
const timeMeters = computed(() => allMeters.value.filter((m: any) => m.enabled && (!m.meter_kind || m.meter_kind === 'time')))
const readingQueue = computed(() => {
  const g: Record<string, any[]> = {}
  timeMeters.value.forEach((m: any) => {
    const owner = m.owner_unit || '未分组'
    ;(g[owner] = g[owner] || []).push(m)
  })
  return Object.keys(g).sort((a, b) => (a === '星达铜业' ? -1 : b === '星达铜业' ? 1 : 0))
    .flatMap(k => g[k])
})
const meterEditOptions = computed(() =>
  readingQueue.value.map((m: any) => ({ label: `${m.owner_unit || ''} · ${m.name}`.replace(/^ · /, ''), value: m.id })),
)
const curMeter = computed(() => readingQueue.value.find((m: any) => m.id === curMeterId.value) || null)
const today = new Date().toISOString().slice(0, 10)

const prevMonthOf = (month: string): string => {
  const [y, mo] = month.split('-').map(Number)
  return `${mo === 1 ? y - 1 : y}-${String(mo === 1 ? 12 : mo - 1).padStart(2, '0')}`
}

const openReadingDrawer = async (month: string) => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  readingMonth.value = month || currentMonth()
  readingWidth.value = `${Math.min(920, Math.floor(window.innerWidth * 0.94))}px`
  readingDate.value = today
  readingRecordDate.value = today
  readingVisible.value = true
  await loadMetersForReading()
  await switchMeterTo(0)
}

const currentMonth = (): string => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const loadMetersForReading = async () => {
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    allMeters.value = res.data?.meters || []
  } catch { /* ignore */ }
}

const loadMeterForm = async (m: any) => {
  const form: Record<string, any> = { remark: '' }
  periods.forEach(p => {
    form[p.key + '_prev'] = 0
    form[p.key + '_curr'] = 0
  })
  // 本月已有读数（编辑场景）
  try {
    const cur: any = await getBillingTimeReading(m.id, readingMonth.value)
    if (cur.data && Object.keys(cur.data).length) {
      periods.forEach(p => {
        form[p.key + '_prev'] = Number(cur.data[p.key + '_prev']) || 0
        form[p.key + '_curr'] = Number(cur.data[p.key + '_curr']) || 0
      })
      form.remark = cur.data.remark || ''
    }
  } catch { /* ignore */ }
  // 本月无记录时，用上月止度预填起度
  if (!form.deep_prev && !form.peak_prev && !form.flat_prev && !form.valley_prev) {
    try {
      const prev: any = await getBillingTimeReading(m.id, prevMonthOf(readingMonth.value))
      if (prev.data && Object.keys(prev.data).length) {
        periods.forEach(p => {
          form[p.key + '_prev'] = Number(prev.data[p.key + '_curr']) || 0
        })
      }
    } catch { /* ignore */ }
  }
  readingReader.value = m.manager || ''
  curForm.value = form
}

const switchMeterTo = async (idx: number) => {
  const q = readingQueue.value
  if (!q.length) return
  const i = Math.max(0, Math.min(idx, q.length - 1))
  curIdx.value = i
  curMeterId.value = q[i].id
  await loadMeterForm(q[i])
}

const onMeterChange = async (id: string) => {
  const q = readingQueue.value
  const i = q.findIndex((m: any) => m.id === id)
  if (i >= 0) {
    curIdx.value = i
    await loadMeterForm(q[i])
  }
}

const curPeriodKwh = (p: string): number => {
  const m = curMeter.value
  const f = curForm.value
  if (!m || !f) return 0
  return Math.max(0, (Number(f[p + '_curr']) || 0) - (Number(f[p + '_prev']) || 0)) * (m.rate || 1)
}
const curTotalKwh = computed(() => periods.reduce((s, p) => s + curPeriodKwh(p.key), 0))
const setCurVal = (key: string, v: string) => {
  curForm.value[key] = Number(v) || 0
}
const rdCurInvalid = (p: string): boolean => {
  const f = curForm.value
  if (!f) return false
  const prev = Number(f[p + '_prev']) || 0
  const curr = Number(f[p + '_curr']) || 0
  return curr > 0 && prev > 0 && curr < prev
}

const saveCurrentAndNext = async () => {
  const m = curMeter.value
  if (!readingMonth.value) {
    MessagePlugin.warning('请选择月份')
    return
  }
  if (!m) {
    MessagePlugin.warning('请选择电表')
    return
  }
  // 止度不得小于起度
  for (const p of periods) {
    const prev = Number(curForm.value[p.key + '_prev']) || 0
    const curr = Number(curForm.value[p.key + '_curr']) || 0
    if (curr < prev) {
      MessagePlugin.warning(`${m.name} ${p.label} 时段止度不得小于起度`)
      return
    }
  }
  savingReadings.value = true
  try {
    const { remark, ...reads } = curForm.value
    await saveBillingTimeReading(m.id, { month: readingMonth.value, ...reads, remark: remark || '' })
    const q = readingQueue.value
    const hasNext = curIdx.value + 1 < q.length
    if (hasNext) {
      await switchMeterTo(curIdx.value + 1)
      return
    }
    // 全部表已保存，生成当月账单
    MessagePlugin.success('读数已保存')
    try {
      await generateBillingRecord(activeTenantId.value, { month: readingMonth.value })
      MessagePlugin.success('账单已生成')
    } catch (e: any) {
      MessagePlugin.warning(e?.message || '账单生成失败，请检查分表读数与市电账单')
    }
    readingVisible.value = false
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingReadings.value = false
  }
}

// ---- 设置抽屉 ----
const settingsVisible = ref(false)
const settingsWidth = ref('860px')
const settingsTab = ref('tenant')
const detail = ref<any>(null)
const itemsForm = ref<{ category: string; item_key: string; item_name: string; enabled: boolean }[]>([])

const allocationOptions = [{ label: '按比例分摊', value: '按比例分摊' }]
const meterKindOptions = [{ label: '分时', value: 'time' }, { label: '普通', value: 'normal' }]

const openSettings = async () => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  settingsVisible.value = true
  settingsTab.value = 'tenant'
  tenantFormVisible.value = false
  meterFormVisible.value = false
  await loadDetail()
}

const loadDetail = async () => {
  if (!activeTenantId.value) return
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    detail.value = res.data
    const d = res.data
    allMeters.value = d.meters || []
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
    enabledCount: items.filter((i: any) => i.enabled).length,
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
        item_key: it.item_name.trim(), item_name: it.item_name.trim(), enabled: !!it.enabled,
      }))
    await saveBillingTenantItems(activeTenantId.value, { items })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存子项失败')
  }
}

// ---- 分时电表 CRUD(按归属单位分组 + 内联表单) ----
const meterFormVisible = ref(false)
const savingMeter = ref(false)
const allMeters = ref<any[]>([])
const meterForm = ref<any>({ id: '', name: '', meter_no: '', rate: 1, meter_kind: 'time', owner_unit: '', use_unit: '', manager: '', contact: '', meter_mode: 'manual', install_date: '', remark: '' })

const emptyMeterForm = (ownerUnit: string) => ({
  id: '', name: '', meter_no: '', rate: 1, meter_kind: 'time', owner_unit: ownerUnit || '',
  use_unit: '', manager: '', contact: '', meter_mode: 'manual', install_date: '', remark: '',
})
const meterGroups = computed(() => {
  const g: Record<string, any[]> = {}
  allMeters.value.forEach((m: any) => {
    const owner = m.owner_unit || '未分组'
    ;(g[owner] = g[owner] || []).push(m)
  })
  return Object.entries(g).map(([owner, meters]) => ({ owner, meters }))
})
const openAddMeter = (ownerUnit: string) => {
  meterForm.value = emptyMeterForm(ownerUnit)
  meterFormVisible.value = true
}
const openEditMeter = (m: any) => {
  meterForm.value = {
    id: m.id, name: m.name || '', meter_no: m.meter_no || '',
    rate: Number(m.rate) || 1, meter_kind: m.meter_kind === 'normal' ? 'normal' : 'time',
    owner_unit: m.owner_unit || '', use_unit: m.use_unit || '',
    manager: m.manager || '', contact: m.contact || '',
    meter_mode: m.meter_mode === 'auto' ? 'auto' : 'manual',
    install_date: m.install_date || '', remark: m.remark || '',
  }
  meterFormVisible.value = true
}
const submitMeter = async () => {
  if (!meterForm.value.name.trim()) {
    MessagePlugin.warning('请输入别名')
    return
  }
  savingMeter.value = true
  try {
    const payload: Record<string, unknown> = {
      name: meterForm.value.name.trim(),
      meter_no: meterForm.value.meter_no,
      meter_kind: meterForm.value.meter_kind || 'time',
      owner_unit: meterForm.value.owner_unit,
      use_unit: meterForm.value.use_unit,
      manager: meterForm.value.manager,
      contact: meterForm.value.contact,
      meter_mode: meterForm.value.meter_mode || 'manual',
      install_date: meterForm.value.install_date || null,
      remark: meterForm.value.remark,
      rate: Number(meterForm.value.rate) || 1,
    }
    if (meterForm.value.id) {
      await updateBillingTimeMeter(meterForm.value.id, payload)
    } else {
      await createBillingTimeMeter(activeTenantId.value, payload)
    }
    meterFormVisible.value = false
    MessagePlugin.success('已保存')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}
const toggleMeter = async (m: any, v: boolean) => {
  try {
    await updateBillingTimeMeter(m.id, { enabled: v })
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}
const removeMeter = async (m: any) => {
  try {
    await deleteBillingTimeMeter(m.id)
    MessagePlugin.success('已删除')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 账单详情与打印 ----
const recordVisible = ref(false)
const recordWidth = ref('760px')
const recordDetail = ref<any>(null)
const printVisible = ref(false)
const printUrl = ref('')
const printBusy = ref(false)
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

const printRecord = async () => {
  if (!recordDetail.value) return
  printBusy.value = true
  try {
    const rec = recordDetail.value.record
    const items = recordDetail.value.items || []
    const columns: CatalogColumn[] = [
      { key: 'name', label: '项目', value: (r: any) => r.name },
      { key: 'period', label: '时段', value: (r: any) => r.period || '—' },
      { key: 'qty', label: '电量', value: (r: any) => fmtKwh(r.qty) },
      { key: 'rate', label: '单价', value: (r: any) => fmtRate(r.rate) },
      { key: 'fee', label: '费用（元）', value: (r: any) => fmtMoney(r.fee) },
    ]
    const totalRow = { name: '合计', period: '', qty: 0, rate: 0, fee: rec.total_fee }
    const bytes = await generateCatalogPdf({
      title: `${rec.month} 租户电费账单`,
      columns,
      rows: [...items, totalRow],
    })
    showPrint(`${rec.month} 租户电费账单`, bytes)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印生成失败')
  } finally {
    printBusy.value = false
  }
}

const showPrint = (title: string, bytes: ArrayBuffer) => {
  printTitle.value = title
  if (printUrl.value) URL.revokeObjectURL(printUrl.value)
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
  printVisible.value = true
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}
const doBrowserPrint = () => {
  const frame = document.querySelector('.tenant-print-body iframe') as HTMLIFrameElement
  if (frame?.contentWindow) frame.contentWindow.print()
}

// ---- 抽屉拖动调宽 ----
const onResize = (e: MouseEvent, widthRef: { value: string }) => {
  const startX = e.clientX
  const startW = parseFloat(widthRef.value)
  const maxW = Math.min(1100, Math.floor(window.innerWidth * 0.95))
  const move = (ev: MouseEvent) => {
    const w = Math.min(maxW, Math.max(520, startW + (startX - ev.clientX)))
    widthRef.value = `${w}px`
  }
  const up = () => {
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}
const onReadingResizeStart = (e: MouseEvent) => onResize(e, readingWidth)
const onSettingsResizeStart = (e: MouseEvent) => onResize(e, settingsWidth)
const onRecordResizeStart = (e: MouseEvent) => onResize(e, recordWidth)

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
const cellText = (key: string, row: any): string => {
  if (key === 'ratio') return fmtRatio(row.ratio)
  if (['total_kwh', 'line_loss'].includes(key)) return fmtKwh(row[key])
  if (['industrial_fee', 'dorm_fee', 'water_fee', 'bill_total_amount', 'total_fee'].includes(key)) return fmtMoney(row[key])
  return row[key] || ''
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
    grid-template-columns: 34px 1fr 18px 1fr 140px;
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
    .period-kwh { font-size: 12px; color: var(--td-text-color-secondary); text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
      .item-group-count { font-size: 12px; color: var(--td-text-color-secondary); font-variant-numeric: tabular-nums; }
      .item-group-arrow { color: var(--td-text-color-placeholder); }
    }
    .item-group-body {
      border-top: 1px solid var(--td-component-stroke);
      padding: 10px 14px;
      .item-hint { margin-top: 10px; }
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
      justify-content: space-between;
      padding: 10px 16px;
      border-bottom: 1px solid var(--td-component-stroke);
      .tenant-print-title { font-size: 14px; font-weight: 600; }
      .tenant-print-actions { display: flex; gap: 8px; }
    }
    .tenant-print-body {
      flex: 1;
      min-height: 0;
      .print-preview-frame { width: 100%; height: 100%; border: none; }
      .print-preview-loading { height: 100%; display: flex; align-items: center; justify-content: center; }
    }
  }
}
</style>
