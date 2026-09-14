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
        <t-button theme="primary" size="small" @click="openReadingDrawer('', 'meter')">
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
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
      <span class="doc-summary-item">总电量 <span class="doc-summary-val">{{ fmtKwh(summaryKwh) }}</span> 千瓦时</span>
      <span class="doc-summary-item">总电费 <span class="doc-summary-val">{{ fmtMoney(summaryFee) }}</span> 元</span>
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
          <div class="reading-type-switch">
            <t-radio-group v-model="readingType" variant="default-filled" size="small">
              <t-radio-button value="meter">电表</t-radio-button>
              <t-radio-button value="water">水表</t-radio-button>
            </t-radio-group>
          </div>
          <div v-if="readingType === 'meter'" class="rec-grid">
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

          <div v-if="readingType === 'meter' && isTimeMeter" class="period-grid">
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
          <div v-else-if="readingType === 'meter'" class="period-grid">
            <div class="period-row" :class="{ 'row-invalid': rdCurInvalid('deep') }">
              <span class="period-label">读数</span>
              <t-input class="period-input" :model-value="curForm['deep_prev']" type="number" size="small"
                placeholder="起度" :status="rdCurInvalid('deep') ? 'error' : ''"
                @update:model-value="(v: string) => setCurVal('deep_prev', v)" />
              <span class="rr-sep">~</span>
              <t-input class="period-input" :model-value="curForm['deep_curr']" type="number" size="small"
                placeholder="止度" :status="rdCurInvalid('deep') ? 'error' : ''"
                @update:model-value="(v: string) => setCurVal('deep_curr', v)" />
              <span class="period-kwh">{{ fmtKwh(curPeriodKwh('deep')) }} 千瓦时</span>
            </div>
          </div>

          <!-- 水表抄表 -->
          <div v-if="readingType === 'water'" class="rec-grid">
            <div class="rec-field">
              <label>月份 <span class="required">*</span></label>
              <t-date-picker v-model="readingMonth" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份" />
            </div>
            <div class="rec-field">
              <label>水表 <span class="required">*</span></label>
              <t-select v-model="curWaterMeterId" :options="waterMeterEditOptions" filterable placeholder="选择水表" @change="onWaterMeterChange" />
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
              <div class="readonly-val">{{ fmtNum(curWaterMeter?.rate) }}×</div>
            </div>
            <div class="rec-field">
              <label>起度 <span class="required">*</span></label>
              <t-input v-model="waterForm.prev" type="number" size="small" placeholder="起度"
                :status="Number(waterForm.curr) > 0 && Number(waterForm.curr) < Number(waterForm.prev) ? 'error' : ''" />
            </div>
            <div class="rec-field">
              <label>止度 <span class="required">*</span></label>
              <t-input v-model="waterForm.curr" type="number" size="small" placeholder="止度"
                :status="Number(waterForm.curr) > 0 && Number(waterForm.curr) < Number(waterForm.prev) ? 'error' : ''" />
            </div>
            <div class="rec-field">
              <label>单价（元/吨）</label>
              <t-input v-model="waterForm.price" type="number" size="small" placeholder="默认取表计单价" />
            </div>
            <div class="rec-field">
              <label>用水量</label>
              <div class="readonly-val">{{ fmtKwh(waterUsage) }} 吨</div>
            </div>
            <div class="rec-field rec-field--wide">
              <label>水费</label>
              <div class="readonly-val">{{ fmtMoney(waterAmount) }} 元</div>
            </div>
          </div>

          <div v-if="readingType === 'meter'" class="calc-val">
            <span>{{ curMeter?.name || '' }}合计</span>
            <b class="row-mono">{{ fmtKwh(curTotalKwh) }}</b>
            <span>千瓦时</span>
          </div>
          <div v-else class="calc-val">
            <span>{{ curWaterMeter?.name || '' }}合计</span>
            <b class="row-mono">{{ fmtKwh(waterUsage) }}</b>
            <span>吨</span>
            <b class="row-mono">{{ fmtMoney(waterAmount) }}</b>
            <span>元</span>
          </div>

          <div class="rec-field rec-field--wide">
            <label>备注</label>
            <t-textarea v-model="curForm.remark" :maxlength="500" placeholder="选填" />
          </div>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="readingVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="savingReadings" @click="readingType === 'water' ? saveWaterAndNext() : saveCurrentAndNext()">保存</t-button>
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
              <div v-for="g in (meterGroups.length ? meterGroups : [{ owner: '', meters: [] }])" :key="g.owner || 'empty'" class="meter-section">
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
          <t-tab-panel value="water-meters" label="水表设置">
            <div class="settings-panel">
              <div v-for="g in (waterMeterGroups.length ? waterMeterGroups : [{ owner: '', meters: [] }])" :key="g.owner || 'empty'" class="meter-section">
                <div class="meter-section-head">
                  <span class="meter-section-title">{{ g.owner || '未分组' }}</span>
                  <t-button variant="outline" size="small" @click="openAddWaterMeter(g.owner)">
                    <template #icon><t-icon name="add" /></template>新增
                  </t-button>
                </div>
                <div class="meter-grid">
                  <div v-if="waterMeterFormVisible && !waterMeterForm.id && waterMeterForm.owner_unit === g.owner" class="meter-form">
                    <div class="meter-form-title">新增水表</div>
                    <div class="form-grid">
                      <div class="form-item">
                        <label>别名 <span class="required">*</span></label>
                        <t-input v-model="waterMeterForm.name" placeholder="如：总表1 / 分表1" />
                      </div>
                      <div class="form-item">
                        <label>表号</label>
                        <t-input v-model="waterMeterForm.meter_no" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>倍率</label>
                        <t-input v-model.number="waterMeterForm.rate" type="number" placeholder="默认 1" />
                      </div>
                      <div class="form-item">
                        <label>类型</label>
                        <t-select v-model="waterMeterForm.meter_kind" :options="waterMeterKindOptions" />
                      </div>
                      <div class="form-item">
                        <label>默认单价（元/吨）</label>
                        <t-input v-model.number="waterMeterForm.price" type="number" placeholder="如：5.22" />
                      </div>
                      <div class="form-item">
                        <label>归属单位</label>
                        <t-input v-model="waterMeterForm.owner_unit" placeholder="如：星达铜业" />
                      </div>
                      <div class="form-item">
                        <label>使用单位</label>
                        <t-input v-model="waterMeterForm.use_unit" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>管理人员</label>
                        <t-input v-model="waterMeterForm.manager" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>联系方式</label>
                        <t-input v-model="waterMeterForm.contact" placeholder="选填" />
                      </div>
                      <div class="form-item">
                        <label>抄表方式</label>
                        <t-radio-group v-model="waterMeterForm.meter_mode">
                          <t-radio-button value="auto">自动抄表</t-radio-button>
                          <t-radio-button value="manual">手动抄表</t-radio-button>
                        </t-radio-group>
                      </div>
                      <div class="form-item">
                        <label>安装日期</label>
                        <t-date-picker v-model="waterMeterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                      </div>
                      <div class="form-item form-item--full">
                        <label>备注</label>
                        <t-textarea v-model="waterMeterForm.remark" :maxlength="500" placeholder="选填" />
                      </div>
                    </div>
                    <div class="form-actions">
                      <t-button variant="outline" size="small" @click="waterMeterFormVisible = false">取消</t-button>
                      <t-button theme="primary" size="small" :loading="savingWaterMeter" @click="submitWaterMeter">保存</t-button>
                    </div>
                  </div>

                  <template v-for="m in g.meters" :key="m.id">
                    <div class="meter-card">
                      <div class="meter-card-head">
                        <span class="meter-card-name">{{ m.name }}</span>
                        <t-switch :model-value="!!m.enabled" size="small" @change="(v: any) => toggleWaterMeter(m, v)" />
                        <span class="meter-card-actions">
                          <t-button variant="text" size="small" @click="openEditWaterMeter(m)">
                            <template #icon><t-icon name="edit" size="15px" /></template>
                          </t-button>
                          <t-popconfirm theme="warning" :content="`确定删除水表「${m.name}」吗？`"
                            :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                            @confirm="removeWaterMeter(m)">
                            <t-button variant="text" size="small" @click.stop>
                              <template #icon><t-icon name="delete" size="15px" /></template>
                            </t-button>
                          </t-popconfirm>
                        </span>
                      </div>
                      <div class="meter-card-grid">
                        <div class="meter-card-item"><span class="k">表号</span><span class="v">{{ m.meter_no || '—' }}</span></div>
                        <div class="meter-card-item"><span class="k">倍率</span><span class="v">{{ fmtNum(m.rate) }}</span></div>
                        <div class="meter-card-item"><span class="k">单价</span><span class="v">{{ fmtNum(m.price) }} 元</span></div>
                        <div class="meter-card-item"><span class="k">类型</span><span class="v">{{ waterMeterKindLabel(m.meter_kind) }}</span></div>
                      </div>
                    </div>
                    <div v-if="waterMeterFormVisible && waterMeterForm.id === m.id" class="meter-form">
                      <div class="meter-form-title">编辑水表</div>
                      <div class="form-grid">
                        <div class="form-item">
                          <label>别名 <span class="required">*</span></label>
                          <t-input v-model="waterMeterForm.name" placeholder="如：总表1 / 分表1" />
                        </div>
                        <div class="form-item">
                          <label>表号</label>
                          <t-input v-model="waterMeterForm.meter_no" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>倍率</label>
                          <t-input v-model.number="waterMeterForm.rate" type="number" placeholder="默认 1" />
                        </div>
                        <div class="form-item">
                          <label>类型</label>
                          <t-select v-model="waterMeterForm.meter_kind" :options="waterMeterKindOptions" />
                        </div>
                        <div class="form-item">
                          <label>默认单价（元/吨）</label>
                          <t-input v-model.number="waterMeterForm.price" type="number" placeholder="如：5.22" />
                        </div>
                        <div class="form-item">
                          <label>归属单位</label>
                          <t-input v-model="waterMeterForm.owner_unit" placeholder="如：星达铜业" />
                        </div>
                        <div class="form-item">
                          <label>使用单位</label>
                          <t-input v-model="waterMeterForm.use_unit" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>管理人员</label>
                          <t-input v-model="waterMeterForm.manager" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>联系方式</label>
                          <t-input v-model="waterMeterForm.contact" placeholder="选填" />
                        </div>
                        <div class="form-item">
                          <label>抄表方式</label>
                          <t-radio-group v-model="waterMeterForm.meter_mode">
                            <t-radio-button value="auto">自动抄表</t-radio-button>
                            <t-radio-button value="manual">手动抄表</t-radio-button>
                          </t-radio-group>
                        </div>
                        <div class="form-item">
                          <label>安装日期</label>
                          <t-date-picker v-model="waterMeterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                        </div>
                        <div class="form-item form-item--full">
                          <label>备注</label>
                          <t-textarea v-model="waterMeterForm.remark" :maxlength="500" placeholder="选填" />
                        </div>
                      </div>
                      <div class="form-actions">
                        <t-button variant="outline" size="small" @click="waterMeterFormVisible = false">取消</t-button>
                        <t-button theme="primary" size="small" :loading="savingWaterMeter" @click="submitWaterMeter">保存</t-button>
                      </div>
                    </div>
                  </template>
                  <div v-if="!g.meters.length && !(waterMeterFormVisible && !waterMeterForm.id && waterMeterForm.owner_unit === g.owner)" class="meter-empty">暂无水表</div>
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
import { ref, computed, onMounted, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, createBillingTenant, getBillingTenant, updateBillingTenant, deleteBillingTenant,
  listBillingTimeMeters, createBillingTimeMeter, updateBillingTimeMeter, deleteBillingTimeMeter,
  getBillingTimeReading, saveBillingTimeReading,
  listBillingWaterMeters, createBillingWaterMeter, updateBillingWaterMeter, deleteBillingWaterMeter,
  getBillingWaterReading, saveBillingWaterReading, listBillingWaterReadings,
  saveBillingTenantItems,
  listBillingRecords, generateBillingRecord, getBillingRecord, deleteBillingRecord,
  listKnowledgeBases,
  listUtilityBillRecords, listSolarBillRecords, listUtilityMeterRecords,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

// ---- 使用单位(房东/租户) ----
const OWNER_UNIT = '重庆星达'
const BILL_KB_NAME = '日常事务-电费'
const useUnit = ref(OWNER_UNIT)
const kbId = ref('')

// ---- 列定义(按使用单位双视图) ----
interface ColDef { key: string; label: string; default: boolean; w: string }
const COL_DEFS_OWNER: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr' },
  { key: 'unit', label: '使用单位', default: true, w: '0.9fr' },
  { key: 'total_kwh', label: '总电量', default: true, w: '0.9fr' },
  { key: 'total_fee', label: '总电费', default: true, w: '1fr' },
  { key: 'solar_gen', label: '光伏发电量', default: true, w: '0.9fr' },
  { key: 'solar_grid', label: '上网电量', default: true, w: '0.9fr' },
  { key: 'ind_kwh', label: '用电量（工业）', default: true, w: '1fr' },
  { key: 'ind_price', label: '均价（工业）', default: true, w: '0.9fr' },
  { key: 'ind_fee', label: '电费（工业）', default: false, w: '1fr' },
  { key: 'res_kwh', label: '电量（居民）', default: true, w: '0.9fr' },
  { key: 'res_fee', label: '电费（居民）', default: false, w: '0.9fr' },
  { key: 'dorm_kwh', label: '电量（宿舍）', default: true, w: '0.9fr' },
  { key: 'dorm_fee', label: '电费（宿舍）', default: false, w: '0.9fr' },
  { key: 'water_ind_usage', label: '用水量（工业）', default: true, w: '1fr' },
  { key: 'water_ind_fee', label: '水费（工业）', default: false, w: '0.9fr' },
  { key: 'water_dorm_usage', label: '用水量（宿舍）', default: true, w: '1fr' },
  { key: 'water_dorm_fee', label: '水费（宿舍）', default: false, w: '0.9fr' },
  { key: 'water_fire_usage', label: '用水量（消防）', default: true, w: '1fr' },
  { key: 'water_fire_fee', label: '水费（消防）', default: false, w: '0.9fr' },
  { key: 'gas_usage', label: '用气量', default: true, w: '0.9fr' },
  { key: 'gas_fee', label: '气费', default: false, w: '0.9fr' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr' },
]
const COL_DEFS_TENANT: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr' },
  { key: 'unit', label: '使用单位', default: true, w: '0.9fr' },
  { key: 'total_kwh', label: '总电量', default: false, w: '0.9fr' },
  { key: 'total_fee', label: '总电费', default: false, w: '1fr' },
  { key: 'ratio', label: '分摊比例', default: false, w: '0.9fr' },
  { key: 'ind_kwh', label: '用电量（工业）', default: true, w: '0.9fr' },
  { key: 'ind_fee', label: '电费（工业）', default: true, w: '1fr' },
  { key: 'ind_price', label: '均价（工业）', default: true, w: '0.9fr' },
  { key: 'dorm_kwh', label: '电量（居民）', default: true, w: '0.9fr' },
  { key: 'dorm_fee', label: '电费（居民）', default: true, w: '0.9fr' },
  { key: 'water_ind_usage', label: '用水量（工业）', default: true, w: '1fr' },
  { key: 'water_ind_fee', label: '水费（工业）', default: true, w: '0.9fr' },
  { key: 'water_dorm_usage', label: '用水量（居民）', default: true, w: '1fr' },
  { key: 'water_dorm_fee', label: '水费（居民）', default: true, w: '0.9fr' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr' },
]
const isOwnerView = computed(() => useUnit.value === OWNER_UNIT)
const activeColDefs = computed(() => (isOwnerView.value ? COL_DEFS_OWNER : COL_DEFS_TENANT))
const colStorageKey = computed(() => `weknora-tenant-billing-cols-${isOwnerView.value ? 'owner' : 'tenant'}-v2`)
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
      listBillingWaterMeters(tenantId),
      listBillingWaterReadings({}),
    )
    const [billRes, solarRes, recRes, elecRes, gasRes, wmRes, wrRes]: any[] = await Promise.all(tasks)
    utilityBills.value = Array.isArray(billRes?.data || billRes?.list) ? (billRes.data || billRes.list) : []
    solarBills.value = Array.isArray(solarRes?.data || solarRes?.list) ? (solarRes.data || solarRes.list) : []
    records.value = recRes?.data || []
    elecRows.value = flattenMeterRecords(elecRes)
    gasRows.value = flattenMeterRecords(gasRes)
    waterMeters.value = wmRes?.data || []
    waterReadings.value = wrRes?.data || []
    buildRows()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

// 水/气记录展平(与 UtilityMeterTab 一致)
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
        use_unit: meter?.use_unit || (it as any).use_unit || '',
        usage: Number(it.usage) || 0,
        unit_price: Number(it.unit_price ?? meter?.default_unit_price) || 0,
        amount: Number(it.amount) || 0,
      })
    }
  }
  return flat
}

const sumBy = (arr: any[], key: string): number => Math.round(arr.reduce((s, r) => s + (Number(r[key]) || 0), 0) * 100) / 100

// 市电账单居民电量/电费
const residentInfoOf = (bill: any) => {
  const item = bill?.item || bill || {}
  const rows = Array.isArray(item.residential_readings) ? item.residential_readings : []
  const kwh = Math.round(rows.reduce((s: number, r: any) => s + (Number(r.bill_kwh) || 0), 0) * 100) / 100
  const gov = (Array.isArray(item.fee_items) ? item.fee_items : [])
    .filter((f: any) => String(f.category || '').includes('政府性基金') && Number(f.qty || 0) < 100000 && !String(f.name || '').includes('功率因数'))
    .reduce((s: number, f: any) => s + (Number(f.fee) || 0), 0)
  const fee = Math.round(((Number(item.catalog_amount) || 0) + (Number(gov) || 0)) * 100) / 100
  return { kwh, fee }
}

// 水表读数按 (meterId, month) 索引
const waterReadingMap = computed(() => {
  const m = new Map<string, any>()
  waterReadings.value.forEach((r: any) => m.set(`${r.meter_id}__${r.month}`, r))
  return m
})

// 汇总指定类型水表某月用量/费用
const waterAgg = (kind: string, owner: string, month: string) => {
  const meters = waterMeters.value.filter((m: any) =>
    m.enabled !== false && m.meter_kind === kind &&
    (owner === '' || m.owner_unit === owner || m.use_unit === owner),
  )
  let usage = 0
  let fee = 0
  for (const m of meters) {
    const r = waterReadingMap.value.get(`${m.id}__${month}`)
    if (!r) continue
    const u = Math.max(0, (Number(r.curr) || 0) - (Number(r.prev) || 0)) * (Number(m.rate) || 1)
    const price = Number(r.price) > 0 ? Number(r.price) : (Number(m.price) || 0)
    usage += u
    fee += u * price
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
    // 宿舍电表(utility 电费页表,按使用单位)
    const elecOf = (unit: string) => elecRows.value.filter(r => r.month === month && r.use_unit === unit)
    const dormOf = (unit: string) => {
      const arr = elecOf(unit)
      return { kwh: sumBy(arr, 'usage'), fee: Math.round(arr.reduce((s, r) => s + (Number(r.usage) || 0) * (Number(r.unit_price) || 0), 0) * 100) / 100 }
    }
    const gasOf = () => gasRows.value.filter(r => r.month === month)
    const base: Record<string, any> = { id: `row-${month}`, month, unit: useUnit.value }

    if (isOwnerView.value) {
      // ===== 星达(房东)视图 =====
      const totalKwh = Number(bill?.item?.total_kwh) || 0
      const totalFee = Number(bill?.item?.grand_total ?? bill?.item?.total_amount) || 0
      const res = residentInfoOf(bill?.item)
      const dorm = dormOf(OWNER_UNIT)
      const tenantDorm = dormOf(tenantName)
      const wInd = waterAgg('total', OWNER_UNIT, month)
      const wDorm = waterAgg('dorm', OWNER_UNIT, month)
      const wFire = waterAgg('fire', OWNER_UNIT, month)
      const gas = gasOf()
      const indKwh = Math.round((totalKwh - (Number(rec?.total_kwh) || 0) - res.kwh - tenantDorm.kwh) * 100) / 100
      const indFee = Math.round((totalFee - (Number(rec?.industrial_fee) || 0) - res.fee) * 100) / 100
      Object.assign(base, {
        total_kwh: totalKwh,
        total_fee: totalFee,
        solar_gen: sumBy(solarArr.map((s: any) => ({ v: s.item?.generation_kwh })), 'v'),
        solar_grid: sumBy(solarArr.map((s: any) => ({ v: s.item?.grid_kwh })), 'v'),
        ind_kwh: indKwh,
        ind_fee: indFee,
        ind_price: indKwh > 0 ? Math.round(indFee / indKwh * 10000) / 10000 : 0,
        res_kwh: res.kwh,
        res_fee: res.fee,
        dorm_kwh: dorm.kwh,
        dorm_fee: dorm.fee,
        water_ind_usage: wInd.usage, water_ind_fee: wInd.fee,
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
      const wInd = waterAgg('industry', tenantName, month)
      const wDorm = waterAgg('dorm', tenantName, month)
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

// ---- 新增记录抽屉（抄表记录：分时/普通电表单表连续录入） ----
const readingVisible = ref(false)
const readingWidth = ref(loadDrawerWidth(DRAWER_W_KEYS.reading, '820px'))
const savingReadings = ref(false)
const readingMonth = ref('')
const readingTitle = '新增抄表记录'
const readingDate = ref('')
const readingReader = ref('')
const readingRecordDate = ref('')
const curForm = ref<Record<string, any>>({ month: '', remark: '' })
const curIdx = ref(0)
const curMeterId = ref('')
// 录入类型:meter 电表 | water 水表
const readingType = ref<'meter' | 'water'>('meter')
const periods = [
  { key: 'deep', label: '尖' },
  { key: 'peak', label: '峰' },
  { key: 'flat', label: '平' },
  { key: 'valley', label: '谷' },
]
const tenantName = computed(() => tenants.value.find((t: any) => t.id === activeTenantId.value)?.name || '')
const enabledMeters = computed(() => allMeters.value.filter((m: any) => m.enabled))
const readingQueue = computed(() => {
  const g: Record<string, any[]> = {}
  enabledMeters.value.forEach((m: any) => {
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
const isTimeMeter = computed(() => {
  const m = curMeter.value
  return !!m && (!m.meter_kind || m.meter_kind === 'time')
})
const today = new Date().toISOString().slice(0, 10)

// ---- 水表抄表(单起止+单价,连续录入) ----
const enabledWaterMeters = computed(() => allWaterMeters.value.filter((m: any) => m.enabled))
const waterReadingQueue = computed(() => {
  const g: Record<string, any[]> = {}
  enabledWaterMeters.value.forEach((m: any) => {
    const owner = m.owner_unit || '未分组'
    ;(g[owner] = g[owner] || []).push(m)
  })
  return Object.keys(g).sort((a, b) => (a === '星达铜业' ? -1 : b === '星达铜业' ? 1 : 0))
    .flatMap(k => g[k])
})
const waterMeterEditOptions = computed(() =>
  waterReadingQueue.value.map((m: any) => ({ label: `${m.owner_unit || ''} · ${m.name}`.replace(/^ · /, ''), value: m.id })),
)
const curWaterIdx = ref(0)
const curWaterMeterId = ref('')
const waterForm = ref<{ prev: number; curr: number; price: number }>({ prev: 0, curr: 0, price: 0 })
const curWaterMeter = computed(() => waterReadingQueue.value.find((m: any) => m.id === curWaterMeterId.value) || null)
const waterUsage = computed(() => {
  const m = curWaterMeter.value
  if (!m) return 0
  return Math.max(0, (Number(waterForm.value.curr) || 0) - (Number(waterForm.value.prev) || 0)) * (Number(m.rate) || 1)
})
const waterAmount = computed(() => Math.round(waterUsage.value * waterPrice.value * 100) / 100)
const waterPrice = computed(() => {
  const p = Number(waterForm.value.price)
  if (p > 0) return p
  return Number(curWaterMeter.value?.price) || 0
})
const loadWaterForm = async (m: any) => {
  const form = { prev: 0, curr: 0, price: Number(m.price) || 0 }
  try {
    const cur: any = await getBillingWaterReading(m.id, readingMonth.value)
    if (cur.data && Object.keys(cur.data).length) {
      form.prev = Number(cur.data.prev) || 0
      form.curr = Number(cur.data.curr) || 0
      form.price = Number(cur.data.price) || form.price
    }
  } catch { /* ignore */ }
  // 本月无记录,同表上月止度预填起度
  if (!(form.prev > 0) && !(form.curr > 0)) {
    try {
      const prev: any = await getBillingWaterReading(m.id, prevMonthOf(readingMonth.value))
      if (prev.data && Object.keys(prev.data).length) {
        form.prev = Number(prev.data.curr) || 0
      }
    } catch { /* ignore */ }
  }
  readingReader.value = m.manager || ''
  waterForm.value = form
}
const switchWaterTo = async (idx: number) => {
  const q = waterReadingQueue.value
  if (!q.length) return
  const i = Math.max(0, Math.min(idx, q.length - 1))
  curWaterIdx.value = i
  curWaterMeterId.value = q[i].id
  await loadWaterForm(q[i])
}
const onWaterMeterChange = async (id: string) => {
  const q = waterReadingQueue.value
  const i = q.findIndex((m: any) => m.id === id)
  if (i >= 0) {
    curWaterIdx.value = i
    await loadWaterForm(q[i])
  }
}
const saveWaterAndNext = async () => {
  const m = curWaterMeter.value
  if (!readingMonth.value) { MessagePlugin.warning('请选择月份'); return }
  if (!m) { MessagePlugin.warning('请选择水表'); return }
  const prev = Number(waterForm.value.prev) || 0
  const curr = Number(waterForm.value.curr) || 0
  if (curr < prev) { MessagePlugin.warning(`${m.name} 止度不得小于起度`); return }
  savingReadings.value = true
  try {
    await saveBillingWaterReading(m.id, { month: readingMonth.value, prev, curr, price: waterPrice.value })
    const q = waterReadingQueue.value
    if (curWaterIdx.value + 1 < q.length) {
      await switchWaterTo(curWaterIdx.value + 1)
      return
    }
    MessagePlugin.success('读数已保存')
    readingVisible.value = false
    await loadAllData()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingReadings.value = false
  }
}

const prevMonthOf = (month: string): string => {
  const [y, mo] = month.split('-').map(Number)
  return `${mo === 1 ? y - 1 : y}-${String(mo === 1 ? 12 : mo - 1).padStart(2, '0')}`
}

const openReadingDrawer = async (month: string, type: 'meter' | 'water' = 'meter') => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  readingType.value = type
  readingMonth.value = month || currentMonth()
  readingDate.value = today
  readingRecordDate.value = today
  readingVisible.value = true
  await loadMetersForReading()
  if (type === 'water') {
    await switchWaterTo(0)
  } else {
    await switchMeterTo(0)
  }
}

const currentMonth = (): string => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const loadMetersForReading = async () => {
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    allMeters.value = res.data?.meters || []
    allWaterMeters.value = res.data?.water_meters || []
  } catch { /* ignore */ }
}

const loadMeterForm = async (m: any) => {
  const form: Record<string, any> = { remark: '' }
  if (m.meter_kind && m.meter_kind !== 'time') {
    // 普通电表:单起止(复用 deep 字段)
    form.deep_prev = 0
    form.deep_curr = 0
  } else {
    periods.forEach(p => {
      form[p.key + '_prev'] = 0
      form[p.key + '_curr'] = 0
    })
  }
  // 本月已有读数（编辑场景）
  try {
    const cur: any = await getBillingTimeReading(m.id, readingMonth.value)
    if (cur.data && Object.keys(cur.data).length) {
      if (m.meter_kind && m.meter_kind !== 'time') {
        form.deep_prev = Number(cur.data.deep_prev) || 0
        form.deep_curr = Number(cur.data.deep_curr) || 0
      } else {
        periods.forEach(p => {
          form[p.key + '_prev'] = Number(cur.data[p.key + '_prev']) || 0
          form[p.key + '_curr'] = Number(cur.data[p.key + '_curr']) || 0
        })
      }
      form.remark = cur.data.remark || ''
    }
  } catch { /* ignore */ }
  // 本月无记录时，用上月止度预填起度
  const hasPrev = m.meter_kind && m.meter_kind !== 'time'
    ? form.deep_prev > 0
    : !!(form.deep_prev || form.peak_prev || form.flat_prev || form.valley_prev)
  if (!hasPrev) {
    try {
      const prev: any = await getBillingTimeReading(m.id, prevMonthOf(readingMonth.value))
      if (prev.data && Object.keys(prev.data).length) {
        if (m.meter_kind && m.meter_kind !== 'time') {
          form.deep_prev = Number(prev.data.deep_curr) || 0
        } else {
          periods.forEach(p => {
            form[p.key + '_prev'] = Number(prev.data[p.key + '_curr']) || 0
          })
        }
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
  const checkKeys = m.meter_kind && m.meter_kind !== 'time' ? ['deep'] : periods.map((p: any) => p.key)
  for (const k of checkKeys) {
    const prev = Number(curForm.value[k + '_prev']) || 0
    const curr = Number(curForm.value[k + '_curr']) || 0
    if (curr < prev) {
      MessagePlugin.warning(`${m.name} ${k === 'deep' ? '' : periods.find((p: any) => p.key === k)?.label + ' '}止度不得小于起度`)
      return
    }
  }
  savingReadings.value = true
  try {
    const { remark, ...reads } = curForm.value
    // 普通电表只保留单起止(deep),其余时段清零
    const payload = { month: readingMonth.value, ...reads, remark: remark || '' }
    if (m.meter_kind && m.meter_kind !== 'time') {
      payload.peak_prev = 0; payload.peak_curr = 0
      payload.flat_prev = 0; payload.flat_curr = 0
      payload.valley_prev = 0; payload.valley_curr = 0
    }
    await saveBillingTimeReading(m.id, payload)
    const q = readingQueue.value
    const hasNext = curIdx.value + 1 < q.length
    if (hasNext) {
      await switchMeterTo(curIdx.value + 1)
      return
    }
    // 全部表已保存，有分时表则生成当月账单
    const hasTimeMeter = enabledMeters.value.some((x: any) => !x.meter_kind || x.meter_kind === 'time')
    MessagePlugin.success('读数已保存')
    if (hasTimeMeter) {
      try {
        await generateBillingRecord(activeTenantId.value, { month: readingMonth.value })
        MessagePlugin.success('账单已生成')
      } catch (e: any) {
        MessagePlugin.warning(e?.message || '账单生成失败，请检查分表读数与市电账单')
      }
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
const settingsWidth = ref(loadDrawerWidth(DRAWER_W_KEYS.settings, '860px'))
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
    allWaterMeters.value = d.water_meters || []
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

// ---- 水表 CRUD(复刻电表:按归属单位分组 + 内联表单 + 默认单价) ----
const waterMeterFormVisible = ref(false)
const savingWaterMeter = ref(false)
const allWaterMeters = ref<any[]>([])
const waterMeterForm = ref<any>({ id: '', name: '', meter_no: '', rate: 1, meter_kind: 'total', owner_unit: '', use_unit: '', manager: '', contact: '', meter_mode: 'manual', install_date: '', remark: '', price: 0 })

const emptyWaterMeterForm = (ownerUnit: string) => ({
  id: '', name: '', meter_no: '', rate: 1, meter_kind: 'total', owner_unit: ownerUnit || '',
  use_unit: '', manager: '', contact: '', meter_mode: 'manual', install_date: '', remark: '', price: 0,
})
const waterMeterKindOptions = [
  { label: '总表', value: 'total' },
  { label: '工业', value: 'industry' },
  { label: '宿舍', value: 'dorm' },
  { label: '消防', value: 'fire' },
]
const waterMeterKindLabel = (k: string) => waterMeterKindOptions.find(o => o.value === k)?.label || k
const waterMeterGroups = computed(() => {
  const g: Record<string, any[]> = {}
  allWaterMeters.value.forEach((m: any) => {
    const owner = m.owner_unit || '未分组'
    ;(g[owner] = g[owner] || []).push(m)
  })
  return Object.entries(g).map(([owner, meters]) => ({ owner, meters }))
})
const openAddWaterMeter = (ownerUnit: string) => {
  waterMeterForm.value = emptyWaterMeterForm(ownerUnit)
  waterMeterFormVisible.value = true
}
const openEditWaterMeter = (m: any) => {
  waterMeterForm.value = {
    id: m.id, name: m.name || '', meter_no: m.meter_no || '',
    rate: Number(m.rate) || 1, meter_kind: m.meter_kind || 'total',
    owner_unit: m.owner_unit || '', use_unit: m.use_unit || '',
    manager: m.manager || '', contact: m.contact || '',
    meter_mode: m.meter_mode === 'auto' ? 'auto' : 'manual',
    install_date: m.install_date || '', remark: m.remark || '',
    price: Number(m.price) || 0,
  }
  waterMeterFormVisible.value = true
}
const submitWaterMeter = async () => {
  if (!waterMeterForm.value.name.trim()) {
    MessagePlugin.warning('请输入别名')
    return
  }
  savingWaterMeter.value = true
  try {
    const payload: Record<string, unknown> = {
      name: waterMeterForm.value.name.trim(),
      meter_no: waterMeterForm.value.meter_no,
      meter_kind: waterMeterForm.value.meter_kind || 'total',
      owner_unit: waterMeterForm.value.owner_unit,
      use_unit: waterMeterForm.value.use_unit,
      manager: waterMeterForm.value.manager,
      contact: waterMeterForm.value.contact,
      meter_mode: waterMeterForm.value.meter_mode || 'manual',
      install_date: waterMeterForm.value.install_date || null,
      remark: waterMeterForm.value.remark,
      rate: Number(waterMeterForm.value.rate) || 1,
      price: Number(waterMeterForm.value.price) || 0,
    }
    if (waterMeterForm.value.id) {
      await updateBillingWaterMeter(waterMeterForm.value.id, payload)
    } else {
      await createBillingWaterMeter(activeTenantId.value, payload)
    }
    waterMeterFormVisible.value = false
    MessagePlugin.success('已保存')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingWaterMeter.value = false
  }
}
const toggleWaterMeter = async (m: any, v: boolean) => {
  try {
    await updateBillingWaterMeter(m.id, { enabled: v })
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}
const removeWaterMeter = async (m: any) => {
  try {
    await deleteBillingWaterMeter(m.id)
    MessagePlugin.success('已删除')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 账单详情与打印 ----
const recordVisible = ref(false)
const recordWidth = ref(loadDrawerWidth(DRAWER_W_KEYS.record, '760px'))
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
const onReadingResizeStart = (e: MouseEvent) => onResize(e, readingWidth, DRAWER_W_KEYS.reading)
const onSettingsResizeStart = (e: MouseEvent) => onResize(e, settingsWidth, DRAWER_W_KEYS.settings)
const onRecordResizeStart = (e: MouseEvent) => onResize(e, recordWidth, DRAWER_W_KEYS.record)

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
const MONEY_KEYS = ['total_fee', 'ind_fee', 'res_fee', 'dorm_fee', 'water_ind_fee', 'water_dorm_fee', 'water_fire_fee', 'gas_fee']
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
