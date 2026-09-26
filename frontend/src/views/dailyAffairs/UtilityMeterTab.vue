<template>
  <div class="utility-meter-tab">
    <!-- 筛选工具栏（与电费/合同管理一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-date-picker v-model="filters.month" mode="month" placeholder="月份" format="YYYY-MM" value-type="YYYY-MM" clearable
            class="doc-date-picker doc-filter-field__control" @change="load" />
        </div>
        <div class="doc-filter-field">
          <t-select v-model="filters.kind" placeholder="表计类型" clearable class="doc-filter-select doc-filter-field__control"
            :options="kindFilterOptions" @change="load" />
        </div>
        <div class="doc-filter-field">
          <t-select v-model="filters.useUnit" placeholder="使用单位" clearable filterable class="doc-filter-select doc-filter-field__control"
            :options="useUnitOptions" @change="load" />
        </div>
        <t-button variant="outline" size="small" @click="load">
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
                <t-checkbox v-for="col in categoryCols(COLUMN_DEFS)" :key="col.key" :value="col.key" class="field-popup-item">
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
        <t-button theme="primary" size="small" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          新增记录
        </t-button>
      </div>
    </div>

    <!-- 列表（自绘 grid，数据少时只包裹记录行，超出滚动） -->
    <div class="doc-list-scroll meter-list-scroll" ref="listScrollRef">
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
          <template v-for="row in displayRows" :key="row.item_id || row.key">
            <div class="doc-list-row"
              :class="{ 'row-selected': selectedKeys.has(row.key) }" :style="gridStyle" role="row" @click="onRowClick(row)">
              <div class="cell cell-check" @click.stop>
                <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(row.key)" @change="(v: any) => toggleSelect(row, v)" />
              </div>
              <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`">
                <span v-if="col.key === 'month'" class="row-mono">{{ row.month }}</span>
                <span v-else-if="col.key === 'meter'" class="row-text" :title="row.meter_alias">
                  <span v-if="isTimeRow(row)" class="row-expand-toggle" @click.stop="toggleExpand(row)">{{ expandedKeys.has(row.key) ? '▾' : '▸' }}</span>
                  {{ row.meter_alias }}
                </span>
                <span v-else-if="col.key === 'start_reading'" class="row-mono">{{ fmtNum(row.start_reading) }}</span>
                <span v-else-if="col.key === 'end_reading'" class="row-mono">{{ fmtNum(row.end_reading) }}</span>
                <span v-else-if="col.key === 'rate'" class="row-mono">{{ fmtNum(row.rate) }}</span>
                <span v-else-if="col.key === 'usage'" class="row-mono">{{ fmtNum(row.usage) }}</span>
                <span v-else-if="col.key === 'unit_price'" class="row-mono" :class="{ 'row-dash': isTimeRow(row) }">{{ isTimeRow(row) ? '—' : fmtUnitPrice(row.unit_price) }}</span>
                <span v-else-if="col.key === 'garbage_fee'" class="row-mono">{{ fmtMoney(row.garbage_fee) }}</span>
                <span v-else-if="col.key === 'secondary_water_fee'" class="row-mono">{{ fmtMoney(row.secondary_water_fee) }}</span>
                <span v-else-if="col.key === 'sewage_fee'" class="row-mono">{{ fmtMoney(row.sewage_fee) }}</span>
                <span v-else-if="col.key === 'subsidy'" class="row-mono" :class="{ 'os-neg': Number(row.subsidy) < 0 }">{{ isTimeRow(row) ? '—' : fmtMoney(row.subsidy) }}</span>
                <span v-else-if="col.key === 'amount'" class="row-mono" :class="{ 'row-dash': isTimeRow(row) }">{{ isTimeRow(row) ? '—' : fmtMoney(row.amount) }}</span>
                <span v-else-if="col.key === 'reading_date'" class="row-mono">{{ row.reading_date || '' }}</span>
                <span v-else-if="col.key === 'reader'" class="row-text" :title="String(row.reader ?? '')">{{ row.reader || '' }}</span>
                <span v-else-if="col.key === 'meter_no'" class="row-mono" :title="String(row.meter_no ?? '')">{{ row.meter_no || '' }}</span>
                <span v-else-if="col.key === 'use_unit'" class="row-text" :title="String(row.use_unit ?? '')">{{ row.use_unit || '' }}</span>
                <span v-else-if="col.key === 'default_unit_price'" class="row-mono">{{ fmtUnitPrice(row.default_unit_price) }}</span>
                <span v-else-if="col.key === 'meter_mode'" class="row-text">{{ row.meter_mode === 'auto' ? '自动抄表' : row.meter_mode === 'manual' ? '手动抄表' : '' }}</span>
                <span v-else-if="col.key === 'install_date'" class="row-mono">{{ row.install_date || '' }}</span>
                <span v-else class="row-text" :title="String(row.remark ?? '')">{{ row.remark }}</span>
              </div>
            </div>
            <!-- 分时表：行内展开四时段明细（参考电量明细布局） -->
            <div v-if="isTimeRow(row) && expandedKeys.has(row.key)" class="doc-list-expand" :style="gridStyle">
              <div class="expand-inner">
                <div class="expand-grid">
                  <div class="expand-cell expand-head">时段</div>
                  <div class="expand-cell expand-head">起度</div>
                  <div class="expand-cell expand-head">止度</div>
                  <div class="expand-cell expand-head">倍率</div>
                  <div class="expand-cell expand-head">时段电量</div>
                </div>
                <div v-for="p in periodsOf(row)" :key="p.name" class="expand-grid expand-grid--row">
                  <div class="expand-cell expand-name">{{ p.name }}</div>
                  <div class="expand-cell expand-mono">{{ fmtNum(p.prev) }}</div>
                  <div class="expand-cell expand-mono">{{ fmtNum(p.curr) }}</div>
                  <div class="expand-cell expand-mono">{{ fmtNum(row.rate) }}</div>
                  <div class="expand-cell expand-mono">{{ fmtNum(p.usage) }} {{ unitLabel }}</div>
                </div>
                <div class="expand-grid expand-grid--row expand-grid--total">
                  <div class="expand-cell expand-name">合计</div>
                  <div class="expand-cell expand-mono" />
                  <div class="expand-cell expand-mono" />
                  <div class="expand-cell expand-mono" />
                  <div class="expand-cell expand-mono">{{ fmtNum(row.usage) }} {{ unitLabel }}</div>
                </div>
              </div>
            </div>
          </template>
          <div v-if="!loading && !displayRows.length" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部汇总（列表容器外固定显示；选中时按选中统计并避让浮动工具栏，样式与发票管理一致） -->
    <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
      <span class="doc-summary-item">总用量 <span class="doc-summary-val">{{ fmtNum(summaryUsage) }}</span> {{ unitLabel }}</span>
      <span class="doc-summary-item">总金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
    </div>

    <!-- 底部浮动工具栏（选中行时显示；打印弹窗打开时隐藏） -->
    <transition name="batch-bar-fade">
      <div v-if="selectedKeys.size && !printVisible" class="doc-batch-bar-fixed" role="region">
        <div class="batch-bar-inner">
          <div class="batch-bar-left">
            <span class="batch-bar-count">已选 {{ selectedKeys.size }} 项</span>
            <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
              清除
            </t-button>
          </div>
          <div class="batch-bar-actions">
            <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="openEditSelected">
              <template #icon><t-icon name="edit" size="14px" /></template>
              编辑
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
            <t-popconfirm theme="warning"
              :content="`确定删除所选 ${selectedKeys.size} 条记录吗？`"
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

    <!-- 编辑/新增抽屉 -->
    <teleport to="body">
      <div v-if="drawerVisible" class="doc-drawer-resize-handle" :style="{ right: drawerWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="drawerVisible" :visible="true" :header="drawerTitle" :size="drawerWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="meter-record-drawer"
        @close="onDrawerClose" @update:visible="(v: boolean) => (drawerVisible = v)">
        <div class="meter-drawer-body">
          <div class="rec-grid">
            <div class="rec-field">
              <label>月份 <span class="required">*</span></label>
              <t-date-picker v-model="form.month" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份" />
              <p v-if="duplicateWarning" class="field-error-text">该月该{{ meterLabel }}已有记录，可直接编辑</p>
            </div>
            <div class="rec-field">
              <label>抄表日期</label>
              <t-date-picker v-model="form.readingDate" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选择日期" clearable />
            </div>
            <div class="rec-field">
              <label>使用单位</label>
              <t-select v-model="form.useUnit" :options="useUnits.map(u => ({ label: u, value: u }))" filterable
                :disabled="!!editingItemId" @change="onUseUnitChange" />
            </div>
            <div class="rec-field">
              <label>{{ meterLabel }} <span class="required">*</span></label>
              <t-select v-model="form.meterId" :placeholder="'选择' + meterLabel" :options="meterEditOptions" filterable @change="onMeterChange" />
              <p v-if="duplicateWarning" class="field-error-text">该月该{{ meterLabel }}已有记录，可直接编辑</p>
            </div>

            <div class="rec-field">
              <label>抄表人</label>
              <t-input v-model="form.reader" placeholder="默认取表计管理人员" />
            </div>
            <div class="rec-field">
              <label>录入日期</label>
              <div class="readonly-val">{{ form.recordDate || today }}</div>
            </div>

            <template v-if="isTimeMeter">
              <div class="rec-field rec-field--wide">
                <label>尖 <span class="required">*</span></label>
                <div class="rec-period-pair">
                  <t-input v-model.number="form.deepPrev" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
                  <span class="rec-period-sep">→</span>
                  <t-input v-model.number="form.deepCurr" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
                </div>
              </div>
              <div class="rec-field rec-field--wide">
                <label>峰 <span class="required">*</span></label>
                <div class="rec-period-pair">
                  <t-input v-model.number="form.peakPrev" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
                  <span class="rec-period-sep">→</span>
                  <t-input v-model.number="form.peakCurr" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
                </div>
              </div>
              <div class="rec-field rec-field--wide">
                <label>平 <span class="required">*</span></label>
                <div class="rec-period-pair">
                  <t-input v-model.number="form.flatPrev" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
                  <span class="rec-period-sep">→</span>
                  <t-input v-model.number="form.flatCurr" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
                </div>
              </div>
              <div class="rec-field rec-field--wide">
                <label>谷 <span class="required">*</span></label>
                <div class="rec-period-pair">
                  <t-input v-model.number="form.valleyPrev" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
                  <span class="rec-period-sep">→</span>
                  <t-input v-model.number="form.valleyCurr" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
                </div>
                <p v-if="readingInvalid" class="field-error-text">各时段止度不得小于起度</p>
              </div>
            </template>
            <template v-else>
              <div class="rec-field" :class="{ 'field-invalid': readingInvalid }">
                <label>起度 <span class="required">*</span></label>
                <t-input v-model.number="form.startReading" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
                <p v-if="readingInvalid" class="field-error-text">止度不得小于起度</p>
              </div>
              <div class="rec-field" :class="{ 'field-invalid': readingInvalid }">
                <label>止度 <span class="required">*</span></label>
                <t-input v-model.number="form.endReading" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
                <p v-if="readingInvalid" class="field-error-text">止度不得小于起度</p>
              </div>
            </template>

            <div class="rec-field">
              <label>倍率</label>
              <div class="readonly-val">{{ fmtNum(formRate) }}×</div>
            </div>
            <div class="rec-field" :class="{ 'field-invalid': unitPriceDiff }">
              <label>单价 <span class="required">*</span></label>
              <t-input v-model.number="form.unitPrice" type="number" placeholder="单价" :status="unitPriceDiff ? 'error' : ''" />
              <p v-if="unitPriceDiff" class="field-error-text">与配置默认单价 {{ fmtUnitPrice(currentMeter?.default_unit_price) }} 不同</p>
            </div>
            <template v-if="props.category === 'water'">
              <div class="rec-field">
                <label>垃圾处置费</label>
                <t-input v-model.number="form.garbageFee" type="number" placeholder="默认 13 元/套" />
              </div>
              <div class="rec-field">
                <label>二次供水费</label>
                <t-input v-model.number="form.secondaryWaterFee" type="number" placeholder="选填" />
              </div>
              <div class="rec-field">
                <label>污水处理费</label>
                <t-input v-model.number="form.sewageFee" type="number" placeholder="选填" />
              </div>
            </template>
            <div class="rec-field">
              <label>补差</label>
              <!-- type=number 会导致负号输入被吞,改文本输入+数字键盘;聚焦全选方便直接覆盖;非法字符由 watch 过滤 -->
              <t-input v-model="form.subsidy" type="text" inputmode="decimal" placeholder="补差金额，可为负"
                @focus="(e: any) => { const t = e?.target || e; t?.select?.() }" />
            </div>
            <div class="rec-field" v-if="!isTimeMeter">
              <label>{{ categoryLabel }}（自动计算）</label>
              <div class="calc-val-lg">{{ fmtMoney(formAmount) }} 元</div>
              <p class="field-hint">{{ usageLabel }} {{ fmtNum(formUsage) }} {{ unitLabel }}</p>
            </div>
            <div class="rec-field" v-else>
              <label>电费</label>
              <div class="readonly-val">按租户分摊核算</div>
              <p class="field-hint">{{ usageLabel }} {{ fmtNum(formUsage) }} {{ unitLabel }}</p>
            </div>

            <div class="rec-field rec-field--wide">
              <label>备注</label>
              <t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" />
            </div>
          </div>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="drawerVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
        </div>
      </t-drawer>
    </teleport>

    <!-- 设置抽屉 -->
    <teleport to="body">
      <div v-if="settingsVisible" class="doc-drawer-resize-handle" :style="{ right: settingsWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onSettingsResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="settingsVisible" :visible="true" :header="meterLabel + '配置'" :size="settingsWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="meter-settings-drawer"
        @close="settingsVisible = false" @update:visible="(v: boolean) => (settingsVisible = v)">
        <div class="meter-settings-body">
          <div class="settings-head">
            <t-button theme="primary" size="small" @click="openMeterForm(null)">
              <template #icon><t-icon name="add" /></template>
              新增{{ meterLabel }}
            </t-button>
            <t-button variant="outline" size="small" @click="kindManageVisible = true">
              <template #icon><t-icon name="setting" /></template>
              用途管理
            </t-button>
          </div>

          <div v-if="meters.length" class="meter-search">
            <t-input v-model="meterSearch" placeholder="搜索别名 / 表号" clearable>
              <template #prefix-icon><t-icon name="search" size="14px" /></template>
            </t-input>
          </div>

          <div v-if="!meters.length && !meterFormVisible" class="meter-empty">
            <t-icon name="setting" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无{{ meterLabel }}，点击新增{{ meterLabel }}开始配置</span>
          </div>

          <div class="meter-table-wrap">
            <!-- 新增表计：表单展开在顶部 -->
            <div v-if="meterFormVisible && !meterForm.id" class="meter-form">
              <div class="meter-form-title">新增{{ meterLabel }}</div>
              <div class="form-grid">
                <div class="form-item">
                  <label>别名 <span class="required">*</span></label>
                  <t-input ref="meterAliasInput" v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
                </div>
                <div class="form-item">
                  <label>表号</label>
                  <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
                </div>
                <div class="form-item">
                  <label>类型</label>
                  <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" :placeholder="'选择类型'" />
                </div>
                <div class="form-item">
                  <label>用途</label>
                  <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
                </div>
                <div class="form-item">
                  <label>归属单位</label>
                  <t-input v-model="meterForm.owner_unit" placeholder="选填"  />
                </div>
                <div class="form-item">
                  <label>倍率</label>
                  <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                </div>
                <div class="form-item">
                  <label>默认单价</label>
                  <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
                </div>
                <div class="form-item">
                  <label>使用单位</label>
                  <t-input v-model="meterForm.use_unit" placeholder="选填"  />
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
                <div class="form-item">
                  <label>启用</label>
                  <t-switch v-model="meterForm.enabled" />
                </div>
              </div>
              <div class="form-item form-item--full">
                <label>备注</label>
                <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
              </div>
              <div class="meter-form-actions">
                <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
              </div>
            </div>

            <!-- 分组标签:宿舍 / 工商业 / 自定义用途(仅导航;表格在下方单一渲染,保证拖拽稳定) -->
            <t-tabs v-if="meterGroups.length" v-model="activeMeterGroup" class="meter-settings-tabs">
              <t-tab-panel v-for="g in meterGroups" :key="g.key" :value="g.key" :label="g.label + ' ' + g.items.length" />
            </t-tabs>

            <div v-if="activeGroupItems.length" :ref="setMeterSortableRef" class="meter-table">
              <div class="meter-table-head">
                <span class="meter-drag-th"><t-icon name="move" size="14px" /></span><span>别名</span><span>表号</span><span>类型</span><span>倍率</span><span>单价</span><span>归属单位</span><span>状态</span><span>操作</span>
              </div>
              <template v-for="m in activeGroupItems" :key="m.id">
                <div class="meter-table-row" :data-id="m.id" :class="{ editing: meterFormVisible && meterForm.id === m.id }" @click="toggleMeterEdit(m)">
                  <span class="meter-drag-handle" title="拖动排序" @click.stop><t-icon name="move" size="14px" /></span>
                  <span class="mtr-alias">{{ m.alias }}</span>
                  <span class="mtr-mono">{{ m.meter_no || '—' }}</span>
                  <span class="mtr-type">{{ meterTypeLabel(m.meter_type) }} · {{ meterKindLabel(m.meter_kind) }}</span>
                  <span class="mtr-mono">{{ fmtNum(m.rate) }}</span>
                  <span class="mtr-mono">{{ fmtUnitPrice(m.default_unit_price) }} 元/{{ unitLabel }}</span>
                  <span class="mtr-owner">{{ m.owner_unit || '—' }}</span>
                  <span class="mtr-switch" @click.stop>
                    <t-switch :model-value="!!m.enabled" size="small" @change="(v: any) => toggleEnabled(m, v)" />
                  </span>
                  <span class="meter-row-actions" @click.stop>
                    <t-popconfirm theme="warning" :content="`确定删除${meterLabel}「${m.alias}」吗？`"
                      :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                      @confirm="deleteMeter(m)">
                      <t-button variant="text" size="small" @click.stop>
                        <template #icon><t-icon name="delete" size="15px" /></template>
                      </t-button>
                    </t-popconfirm>
                  </span>
                </div>
                <!-- 编辑表计：点击行展开，再点折叠 -->
                <div v-if="meterFormVisible && meterForm.id === m.id" class="meter-form meter-form--inline">
                  <div class="meter-form-title">编辑{{ meterLabel }}</div>
                  <div class="form-grid">
                    <div class="form-item">
                      <label>别名 <span class="required">*</span></label>
                      <t-input v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
                    </div>
                    <div class="form-item">
                      <label>表号</label>
                      <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
                    </div>
                    <div class="form-item">
                      <label>类型</label>
                      <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" :placeholder="'选择类型'" />
                    </div>
                    <div class="form-item">
                      <label>用途</label>
                      <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
                    </div>
                    <div class="form-item">
                      <label>归属单位</label>
                      <t-input v-model="meterForm.owner_unit" placeholder="选填"  />
                    </div>
                    <div class="form-item">
                      <label>倍率</label>
                      <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                    </div>
                    <div class="form-item">
                      <label>默认单价</label>
                      <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
                    </div>
                    <div class="form-item">
                      <label>使用单位</label>
                      <t-input v-model="meterForm.use_unit" placeholder="选填"  />
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
                      <div class="form-item">
                        <label>启用</label>
                        <t-switch v-model="meterForm.enabled" />
                      </div>
                    </div>
                    <div class="form-item form-item--full">
                      <label>备注</label>
                      <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
                    </div>
                    <div class="meter-form-actions">
                      <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                      <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
                    </div>
                  </div>
                </template>
              </div>
            <div v-if="!activeGroupItems.length" class="meter-empty">
              <t-icon name="setting" size="28px" class="meter-empty-icon" />
              <span class="meter-empty-text">暂无{{ meterLabel }}</span>
            </div>
          </div>
        </div>
      </t-drawer>

      <!-- 用途管理:内置 宿舍/工商业 可改名;公租房及其它自定义可删改增 -->
      <t-dialog v-model:visible="kindManageVisible" :header="'用途管理(' + meterLabel + ')'" :footer="false" width="480px">
        <div class="kind-manage-body">
          <p class="kind-manage-tip">内置用途(宿舍/工商业)驱动分摊计算,可改名不可删除;公租房及其它自定义用途可删除、可修改、可新增,仅分组展示不参与租户核算计算。</p>
          <div v-for="k in meterKinds" :key="k.value" class="kind-manage-row">
            <t-input v-model="k.label" size="small" @enter="() => {}">
              <template #prefix-icon><span class="kind-value-tag">{{ k.value }}</span></template>
            </t-input>
            <t-popconfirm v-if="!(k as any).builtin" theme="warning" :content="`确定删除用途「${k.label}」吗？`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="removeCustomKind(k.value)">
              <t-button variant="text" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </t-popconfirm>
            <span v-else class="kind-builtin-tag">内置</span>
          </div>
          <t-button variant="outline" size="small" block @click="addCustomKind">
            <template #icon><t-icon name="add" /></template>
            新增用途
          </t-button>
          <div class="kind-manage-actions">
            <t-button variant="outline" size="small" @click="kindManageVisible = false">关闭</t-button>
            <t-button theme="primary" size="small" :loading="savingKinds" @click="saveKinds">保存</t-button>
          </div>
        </div>
      </t-dialog>
    </teleport>

    <!-- 打印预览弹窗 -->
    <div v-if="printVisible" class="meter-print-mask">
      <div class="meter-print-dialog">
        <div class="meter-print-header">
          <span class="meter-print-title">{{ categoryLabel }}目录预览（{{ printCount }} 条）</span>
          <t-button variant="text" size="small" class="meter-print-close" @click="closePrint">
            <t-icon name="close" size="16px" />
          </t-button>
        </div>
        <div class="meter-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading">
            <t-loading size="small" :text="'正在生成目录…'" />
          </div>
        </div>
        <div class="meter-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import Sortable from 'sortablejs'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeterRecords,
  createUtilityMeterRecord,
  updateUtilityMeterRecord,
  deleteUtilityMeterRecord,
  listUtilityMeters,
  createUtilityMeter,
  updateUtilityMeter,
  deleteUtilityMeter,
  sortUtilityMeters,
  listUtilityKinds,
  createUtilityKind,
  updateUtilityKind,
  deleteUtilityKind,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

const props = defineProps<{
  category: 'water' | 'gas' | 'electricity'
}>()

const categoryLabel = computed(() => (props.category === 'water' ? '水费' : props.category === 'electricity' ? '电费' : '气费'))
const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))
const usageLabel = computed(() => (props.category === 'water' ? '用水量' : props.category === 'electricity' ? '用电量' : '用气量'))

// ---- 列定义 ----
interface ColDef { key: string; label: string; default: boolean; w: string; tip?: string; only?: 'water' }
const categoryCols = (defs: ColDef[]) => defs.filter(c => !c.only || c.only === props.category)
const COLUMN_DEFS: ColDef[] = [
  { key: 'month', label: '月份', default: true, w: '1fr', tip: '抄表所属月份' },
  { key: 'meter', label: meterLabel.value, default: true, w: '1.2fr', tip: '表计别名' },
  { key: 'start_reading', label: '起度', default: true, w: '0.9fr', tip: '上期止度自动带入' },
  { key: 'end_reading', label: '止度', default: true, w: '0.9fr', tip: '本期抄表读数(须 ≥ 起度)' },
  { key: 'rate', label: '倍率', default: props.category !== 'gas', w: '0.7fr', tip: '表计配置倍率(不可修改)' },
  { key: 'usage', label: usageLabel.value, default: true, w: '1fr', tip: `(止度 − 起度) × 倍率` },
  { key: 'unit_price', label: '单价', default: true, w: '1.2fr', tip: '默认取表计配置单价(最多三位小数)' },
  // 水费附加费用（仅水费；电费/气费无此列）
  { key: 'garbage_fee', label: '垃圾处置费', default: false, w: '0.9fr', tip: '水费附加,新增默认 13 元/套', only: 'water' },
  { key: 'secondary_water_fee', label: '二次供水费', default: false, w: '0.9fr', tip: '水费附加,手工填写', only: 'water' },
  { key: 'sewage_fee', label: '污水处理费', default: false, w: '0.9fr', tip: '水费附加,手工填写', only: 'water' },
  { key: 'subsidy', label: '补差', default: false, w: '0.8fr', tip: '手工填写,可为正负数' },
  { key: 'amount', label: categoryLabel.value, default: true, w: '1fr', tip: `${usageLabel.value} × 单价 + 附加费 + 补差` },
  { key: 'remark', label: '备注', default: false, w: '2fr', tip: '手工填写' },
  // 默认显示：抄表信息
  { key: 'reading_date', label: '抄表日期', default: true, w: '1fr', tip: '实际抄表日期' },
  { key: 'reader', label: '抄表人', default: true, w: '1fr', tip: '默认取表计配置管理人员' },
  { key: 'use_unit', label: '使用单位', default: true, w: '1.2fr', tip: '表计使用单位' },
  { key: 'meter_mode', label: '抄表方式', default: true, w: '0.9fr', tip: '自动抄表/手动抄表' },
  // 详细字段：表计档案参数
  { key: 'meter_no', label: '表号', default: false, w: '1fr', tip: '表计编号' },
  { key: 'default_unit_price', label: '默认单价', default: false, w: '0.9fr', tip: '表计配置单价' },
  { key: 'install_date', label: '安装日期', default: false, w: '1.1fr', tip: '表计安装日期' },
]
const STORAGE_KEY = computed(() => `weknora-utility-meter-${props.category}-columns-v3`)
const visibleKeys = ref<string[]>(loadStoredKeys())
const visibleColDefs = computed(() => categoryCols(COLUMN_DEFS).filter(c => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))

function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        // 仅保留本组件认识的字段，避免跨页面污染
        const valid = arr.filter(k => categoryCols(COLUMN_DEFS).some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return categoryCols(COLUMN_DEFS).filter(c => c.default).map(c => c.key)
}

const fieldPopupVisible = ref(false)
const selectAllColumns = () => {
  visibleKeys.value = categoryCols(COLUMN_DEFS).map(c => c.key)
  persistColumns()
}
const resetColumns = () => {
  visibleKeys.value = categoryCols(COLUMN_DEFS).filter(c => c.default).map(c => c.key)
  persistColumns()
}
const persistColumns = () => {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ }
}

// ---- 数据 ----
const meters = ref<any[]>([])
const rows = ref<any[]>([]) // 展开后的扁平行
const displayRows = ref<any[]>([])
const loading = ref(true)
const filters = ref<{ month?: string; kind?: string; useUnit?: string }>({ month: undefined, kind: undefined, useUnit: undefined })

const kindFilterOptions = computed(() => [
  ...meterKinds.value.map(k => ({ label: k.label, value: k.value })),
])

const useUnitOptions = computed(() => {
  const set = new Set<string>()
  meters.value.forEach((m: any) => { if (m.use_unit) set.add(m.use_unit) })
  return Array.from(set).map(v => ({ label: v, value: v }))
})
const meterSearch = ref('')
const filteredMeters = computed(() => {
  const kw = meterSearch.value.trim().toLowerCase()
  if (!kw) return meters.value
  return meters.value.filter((m: any) =>
    (m.alias || '').toLowerCase().includes(kw) || (m.meter_no || '').toLowerCase().includes(kw))
})
// 用途体系:内置 宿舍/工商业(锁定 value 驱动租户核算计算,label 可改名) + 用户自定义
// 公租房降为自定义用途(可删除/修改/新增),仅分组展示、不参与租户核算计算
const KIND_BUILTIN = reactive([
  { value: 'dorm', label: '宿舍', builtin: true },
  { value: 'production', label: '工商业', builtin: true },
])
const kindsStoreKey = 'weknora-utility-kinds'
type KindItem = { id?: string; value: string; label: string }
const customKinds = ref<KindItem[]>([])
const meterKinds = computed(() => [...KIND_BUILTIN, ...customKinds.value])
const meterKindOptions = computed(() => meterKinds.value.map(k => ({ label: k.label, value: k.value })))
const meterKindLabel = (k: string) => meterKinds.value.find(o => o.value === k)?.label || '未分类'

// 从后端加载自定义用途；首次使用且后端为空时迁移旧 localStorage 数据（保留 value/label）
const loadKinds = async () => {
  try {
    const res: any = await listUtilityKinds(props.category)
    const list = res?.data || []
    if (Array.isArray(list) && list.length) {
      // 后端已持久化内置用途：内置 value 过滤出 customKinds，内置 label 同步到 KIND_BUILTIN
      customKinds.value = list
        .filter((k: any) => !KIND_BUILTIN.some(b => b.value === k.value))
        .map((k: any) => ({ id: k.id, value: k.value, label: k.label }))
      for (const k of list) {
        const b = KIND_BUILTIN.find(x => x.value === k.value)
        if (b && k.label) b.label = k.label
      }
      return
    }
    // 迁移旧本地数据
    let old: any[] = []
    try { old = JSON.parse(localStorage.getItem(kindsStoreKey) || '[]') } catch { /* ignore */ }
    const olds = Array.isArray(old)
      ? old.filter((k: any) => k && k.value && k.label && !KIND_BUILTIN.some(b => b.value === k.value))
      : []
    for (const k of olds) {
      try {
        const created: any = await createUtilityKind(props.category, { value: k.value, label: k.label })
        const item = created?.data || created
        if (item?.value) customKinds.value.push({ id: item.id, value: item.value, label: item.label })
      } catch { /* ignore */ }
    }
    localStorage.removeItem(kindsStoreKey)
  } catch (e) {
    console.error('load kinds failed', e)
  }
}

const addCustomKind = async () => {
  const n = customKinds.value.length + 1
  const value = `custom${n}`
  const label = `自定义${n}`
  try {
    const res: any = await createUtilityKind(props.category, { value, label })
    const item = res?.data || res
    if (item?.id) {
      customKinds.value.push({ id: item.id, value: item.value, label: item.label })
    } else {
      customKinds.value.push({ value, label })
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '新增用途失败')
  }
}
const removeCustomKind = async (value: string) => {
  // 引用检查:有表计正在使用该用途时禁止删除
  const used = meters.value.filter(m => String(m.meter_kind) === value)
  if (used.length) {
    MessagePlugin.warning(`该用途已被 ${used.length} 个表计引用,请先删除或修改这些表计的用途后再删除`)
    return
  }
  const item = customKinds.value.find(k => k.value === value)
  if (!item) return
  if (item.id) {
    try {
      await deleteUtilityKind(item.id)
    } catch (e: any) {
      MessagePlugin.error(e?.message || '删除失败')
      return
    }
  }
  customKinds.value = customKinds.value.filter(k => k.value !== value)
}
const savingKinds = ref(false)
// 保存用途配置：内置用途 upsert 到后端（改名持久化），自定义用途按 id 更新
const saveKinds = async () => {
  savingKinds.value = true
  try {
    for (const k of meterKinds.value) {
      const label = (k.label || '').trim()
      if (!label) {
        MessagePlugin.warning('用途名称不能为空')
        return
      }
      if ((k as any).builtin) {
        await createUtilityKind(props.category, { value: k.value, label })
      } else if ((k as any).id) {
        if (label !== k.label) k.label = label
        await updateUtilityKind((k as any).id, { label })
      }
    }
    MessagePlugin.success('用途配置已保存')
    kindManageVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingKinds.value = false
  }
}

// 设置抽屉分组标签:按用途配置动态生成(宿舍/公租房/工商业...),始终显示全部用途
const meterGroups = computed(() => {
  const groups = meterKinds.value.map(k => ({ key: k.value, label: k.label, items: [] as any[] }))
  for (const m of filteredMeters.value) {
    const g = groups.find(g => g.key === m.meter_kind)
    ;(g || groups[groups.length - 1]).items.push(m)
  }
  return groups
})
const activeMeterGroup = ref<string>('dorm')
const activeGroupItems = computed(() => meterGroups.value.find(g => g.key === activeMeterGroup.value)?.items || [])
watch(meterGroups, (gs) => {
  if (!gs.find(g => g.key === activeMeterGroup.value)) {
    activeMeterGroup.value = (gs[0]?.key as any) || 'dorm'
  }
})
// 点击行展开编辑，再次点击折叠
const toggleMeterEdit = (m: any) => {
  if (meterFormVisible.value && meterForm.value.id === m.id) {
    meterFormVisible.value = false
    return
  }
  openMeterForm(m)
}

const load = async () => {
  loading.value = true
  try {
    const [recRes, meterRes]: any[] = await Promise.all([
      listUtilityMeterRecords({ category: props.category }),
      listUtilityMeters({ category: props.category }),
    ])
    const recData = recRes?.data || {}
    const records = recData?.records || recRes?.records || []
    const mets = meterRes?.data || recData?.meters || meterRes || []
    meters.value = Array.isArray(mets) ? mets : []
    await loadKinds()
    const meterMap = new Map(meters.value.map((m: any) => [m.id, m]))
    const flat: any[] = []
    for (const rec of records) {
      for (const it of (rec.items || [])) {
        const meter = meterMap.get(it.meter_id)
        flat.push({
          record_id: rec.id,
          item_id: it.id,
          key: `${rec.id}__${it.id}`,
          month: rec.month,
          record_date: rec.record_date || '',
          reading_date: it.reading_date || '',
          reader: it.reader || '',
          meter_id: it.meter_id,
          meter_alias: meter?.alias || it.meter_name || '未配置',
          meter_no: meter?.meter_no || '',
          meter_type: meter?.meter_type || '',
          meter_kind: meter?.meter_kind || 'other',
          use_unit: meter?.use_unit || '',
          default_unit_price: meter?.default_unit_price ?? '',
          meter_mode: meter?.meter_mode || '',
          install_date: meter?.install_date || '',
          start_reading: it.start_reading,
          end_reading: it.end_reading,
          deep_prev: it.deep_prev,
          deep_curr: it.deep_curr,
          peak_prev: it.peak_prev,
          peak_curr: it.peak_curr,
          flat_prev: it.flat_prev,
          flat_curr: it.flat_curr,
          valley_prev: it.valley_prev,
          valley_curr: it.valley_curr,
          rate: meter?.rate ?? it.rate ?? '',
          usage: it.usage,
          unit_price: it.unit_price,
          garbage_fee: it.garbage_fee,
          secondary_water_fee: it.secondary_water_fee,
          sewage_fee: it.sewage_fee,
          subsidy: it.subsidy,
          amount: it.amount,
          remark: it.remark || '',
        })
      }
    }
    rows.value = flat
    // 构建 表计→月份→止度 索引（新增记录时上月止度自动填充起度）
    const idx = new Map<string, Map<string, number>>()
    for (const r of flat) {
      if (!r.meter_id) continue
      let m = idx.get(r.meter_id)
      if (!m) { m = new Map(); idx.set(r.meter_id, m) }
      m.set(r.month, Number(r.end_reading) || 0)
    }
    meterMonthEnds.value = idx
    // 抄表人兜底：本地无记录时，取最近一次已保存记录的抄表人作为默认
    if (!lastReader.value) {
      const last = [...flat].reverse().find((r: any) => (r.reader || '').trim())
      if (last?.reader) lastReader.value = last.reader
    }
    applyFilters()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const applyFilters = () => {
  let list = rows.value
  if (filters.value.month) list = list.filter(r => r.month === filters.value.month)
  if (filters.value.kind) list = list.filter(r => r.meter_kind === filters.value.kind)
  if (filters.value.useUnit) list = list.filter(r => r.use_unit === filters.value.useUnit)
  // 同一周期内默认排序:按用途配置顺序(宿舍→公租房→工商业...);跨月保持数据原序
  const order = new Map(meterKinds.value.map((k, i) => [k.value, i]))
  list = [...list].sort((a, b) => {
    if (a.month !== b.month) return 0
    return (order.get(a.meter_kind) ?? 99) - (order.get(b.meter_kind) ?? 99)
  })
  displayRows.value = list
}

// ---- 汇总 ----
const summary = computed(() => {
  const base = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return { total: base.length }
})
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => rows.value.filter(r => selectedKeys.value.has(r.key)))
const summaryUsage = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.usage) || 0), 0) * 100) / 100
})
const summaryAmount = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.amount) || 0), 0) * 100) / 100
})

// ---- 分时表：行内四时段展开 ----
const expandedKeys = ref<Set<string>>(new Set())
const isTimeRow = (row: any) => row.meter_type === 'time'
const periodsOf = (row: any) => {
  const rate = Number(row.rate) > 0 ? Number(row.rate) : 1
  const mk = (name: string, prev: number, curr: number) => ({
    name,
    prev: Number(prev) || 0,
    curr: Number(curr) || 0,
    usage: Math.round((Number(curr) - Number(prev)) * rate * 100) / 100,
  })
  return [
    mk('尖', row.deep_prev, row.deep_curr),
    mk('峰', row.peak_prev, row.peak_curr),
    mk('平', row.flat_prev, row.flat_curr),
    mk('谷', row.valley_prev, row.valley_curr),
  ]
}
const toggleExpand = (row: any) => {
  const next = new Set(expandedKeys.value)
  if (next.has(row.key)) next.delete(row.key)
  else next.add(row.key)
  expandedKeys.value = next
}

const toggleSelect = (row: any, checked: any) => {
  const key = row.key
  const next = new Set(selectedKeys.value)
  if (checked) next.add(key)
  else next.delete(key)
  selectedKeys.value = next
}
const isAllSelected = computed(() => displayRows.value.length > 0 && displayRows.value.every(r => selectedKeys.value.has(r.key)))
const someSelected = computed(() => displayRows.value.some(r => selectedKeys.value.has(r.key)) && !isAllSelected.value)
const toggleSelectAll = (checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) displayRows.value.forEach(r => next.add(r.key))
  else displayRows.value.forEach(r => next.delete(r.key))
  selectedKeys.value = next
}
// 单击行：单选该行并打开编辑抽屉（与发票管理一致）
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.key])
  openEdit(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 编辑抽屉 ----
const drawerVisible = ref(false)
const saving = ref(false)
const editingRecordId = ref('')
const editingItemId = ref('')
const editingOriginalMonth = ref('') // 编辑行原月份（改月时仅迁移该行）
const editingOriginalMeterId = ref('') // 编辑行原表计（改表时删除原行避免产生空记录）
const lastRecordId = ref('')
const recordItems = ref<any[]>([]) // 当前编辑 record 的原始 items（编辑时保留其它行）
interface MeterForm {
  month: string; useUnit: string; meterId: string; readingDate: string; reader: string; recordDate: string
  startReading: number; endReading: number
  deepPrev: number; deepCurr: number; peakPrev: number; peakCurr: number
  flatPrev: number; flatCurr: number; valleyPrev: number; valleyCurr: number
  unitPrice: number; subsidy: string | number; remark: string
  garbageFee: number; secondaryWaterFee: number; sewageFee: number
}
const emptyForm = (): MeterForm => ({
  month: '', useUnit: '', meterId: '', readingDate: '', reader: '', recordDate: '',
  startReading: 0, endReading: 0,
  deepPrev: 0, deepCurr: 0, peakPrev: 0, peakCurr: 0,
  flatPrev: 0, flatCurr: 0, valleyPrev: 0, valleyCurr: 0,
  unitPrice: 0, subsidy: '', remark: '',
  garbageFee: props.category === 'water' ? 13 : 0, secondaryWaterFee: 0, sewageFee: 0,
})
const form = ref<MeterForm>(emptyForm())
// 补差输入过滤:仅保留数字、负号(仅开头)、小数点(仅一个),直接输入 -0.16 不被吞
watch(() => form.value.subsidy, (v) => {
  if (typeof v === 'string' && /[^0-9.\-]/.test(v)) {
    form.value.subsidy = v.replace(/[^0-9.\-]/g, '')
  }
  if (typeof v === 'string' && /(?!^)-|\.\d*\./.test(v)) {
    form.value.subsidy = v.replace(/(?!^)-/g, '').replace(/(\..*)\./g, '$1')
  }
})
const today = (() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
})()
// 最近一次录入的抄表人：本地持久化，新增记录自动带出避免手动输入
const lastReaderKey = 'weknora-utility-last-reader'
const lastReader = ref<string>('')
try { lastReader.value = localStorage.getItem(lastReaderKey) || '' } catch { /* ignore */ }
const rememberReader = (r: string) => {
  const v = (r || '').trim()
  if (!v) return
  lastReader.value = v
  try { localStorage.setItem(lastReaderKey, v) } catch { /* ignore */ }
}
const drawerTitle = computed(() => (editingItemId.value ? '编辑' + categoryLabel.value + '记录' : '新增' + categoryLabel.value + '记录'))

// 系统内使用单位（去重，保持稳定顺序，默认取第一项）
const useUnits = computed(() => {
  const set = new Map<string, string>()
  for (const m of meters.value as any[]) {
    const u = (m.use_unit || '').trim()
    if (u) set.set(u, u)
  }
  return Array.from(set.keys())
})

// 新增/编辑模式选项：按使用单位过滤 + 新增时隐藏当月已录入的表计
const meterEditOptions = computed(() => {
  const u = (form.value.useUnit || '').trim()
  const opts = meters.value
    .filter((m: any) => {
      if (!m.enabled) return false
      if (u && (m.use_unit || '').trim() !== u) return false
      return true
    })
    .map((m: any) => ({ label: m.alias, value: m.id }))
  if (!editingItemId.value) {
    const recorded = new Set<string>()
    if (form.value.month) {
      for (const m of meters.value as any[]) {
        if (meterMonthEnds.value.get(m.id)?.has(form.value.month)) recorded.add(m.id)
      }
    }
    return opts.filter(o => !recorded.has(o.value))
  }
  // 编辑时若当前表计已停用或不在使用单位内，保留在选项中
  if (form.value.meterId && !opts.some(o => o.value === form.value.meterId)) {
    const cur = meters.value.find((m: any) => m.id === form.value.meterId)
    if (cur) opts.push({ label: cur.alias, value: cur.id })
  }
  return opts
})

// 切换使用单位：清空表计选择（读数由表计 watch 重置）
const onUseUnitChange = () => {
  if (editingItemId.value) return
  if (form.value.meterId) {
    const cur = meters.value.find((m: any) => m.id === form.value.meterId) as any
    if (cur && (cur.use_unit || '').trim() !== (form.value.useUnit || '').trim()) {
      form.value.meterId = ''
    }
  }
}

const currentMeter = computed(() => meters.value.find((m: any) => m.id === form.value.meterId))
const isTimeMeter = computed(() => currentMeter.value?.meter_type === 'time')
const formRate = computed(() => Number(currentMeter.value?.rate) > 0 ? Number(currentMeter.value?.rate) : 1)
const formUsage = computed(() => {
  const rate = formRate.value
  if (isTimeMeter.value) {
    const d = Number(form.value.deepCurr) - Number(form.value.deepPrev)
    const p = Number(form.value.peakCurr) - Number(form.value.peakPrev)
    const f = Number(form.value.flatCurr) - Number(form.value.flatPrev)
    const v = Number(form.value.valleyCurr) - Number(form.value.valleyPrev)
    return Math.round((d + p + f + v) * rate * 100) / 100
  }
  return Math.round((Number(form.value.endReading) - Number(form.value.startReading)) * rate * 100) / 100
})
const formAmount = computed(() => {
  const base = formUsage.value * Number(form.value.unitPrice)
  const fees = props.category === 'water'
    ? (Number(form.value.garbageFee) || 0) + (Number(form.value.secondaryWaterFee) || 0) + (Number(form.value.sewageFee) || 0)
    : 0
  return Math.round((base + fees + (Number(form.value.subsidy) || 0)) * 100) / 100
})

// ---- 录入校验：差异标红提醒不拦截，止度<起度（起度>止度）标红且保存拦截；起度=止度视为当月无用量，合法 ----
const readingInvalid = computed(() => {
  if (isTimeMeter.value) {
    return Number(form.value.deepCurr) < Number(form.value.deepPrev) ||
      Number(form.value.peakCurr) < Number(form.value.peakPrev) ||
      Number(form.value.flatCurr) < Number(form.value.flatPrev) ||
      Number(form.value.valleyCurr) < Number(form.value.valleyPrev)
  }
  const s = Number(form.value.startReading) || 0
  const e = Number(form.value.endReading) || 0
  return s > e
})
const duplicateWarning = computed(() => {
  if (editingItemId.value) return false
  if (!form.value.month || !form.value.meterId) return false
  return meterMonthEnds.value.get(form.value.meterId)?.has(form.value.month) || false
})
const unitPriceDiff = computed(() => {
  const up = Number(form.value.unitPrice)
  const def = Number(currentMeter.value?.default_unit_price)
  return !!form.value.meterId && up > 0 && def > 0 && up !== def
})
// 设置抽屉表计按居民/工商业分组表格展示(原卡片网格列数逻辑移除)

const onMeterChange = () => {
  // 切换表计：新增/连续录入模式清空读数便于录入；编辑模式仅回填单价与抄表人，
  // 保留原读数（修正表号场景数据应跟随记录，而不是产生空记录）
  if (!editingItemId.value) {
    form.value.startReading = 0
    form.value.endReading = 0
    form.value.deepPrev = 0
    form.value.deepCurr = 0
    form.value.peakPrev = 0
    form.value.peakCurr = 0
    form.value.flatPrev = 0
    form.value.flatCurr = 0
    form.value.valleyPrev = 0
    form.value.valleyCurr = 0
  }
  if (currentMeter.value) {
    form.value.unitPrice = Number(currentMeter.value.default_unit_price) || 0
    // 抄表人：表计管理人员优先，未配置则沿用最近一次录入的抄表人
    form.value.reader = currentMeter.value.manager || lastReader.value || form.value.reader || ''
  }
}

const openCreate = () => {
  editingRecordId.value = ''
  editingItemId.value = ''
  editingOriginalMonth.value = ''
  editingOriginalMeterId.value = ''
  lastRecordId.value = ''
  recordItems.value = []
  const f = emptyForm()
  f.readingDate = today
  f.recordDate = today
  // 默认月份=上月(如抄表日期 9 月 → 默认 2026-08)
  f.month = prevMonthOf(today.slice(0, 7))
  f.useUnit = useUnits.value[0] || ''
  // 抄表人默认沿用最近一次录入的抄表人，避免每次手动输入
  f.reader = lastReader.value || ''
  form.value = f
  drawerVisible.value = true
}

// ---- 新增记录：上月止度自动填充本月起度 ----
const meterMonthEnds = ref<Map<string, Map<string, number>>>(new Map())
const prevMonthOf = (month: string) => {
  const [y, m] = month.split('-').map(Number)
  if (!y || !m) return ''
  if (m === 1) return `${y - 1}-12`
  return `${y}-${String(m - 1).padStart(2, '0')}`
}
// 连续录入时跳过自动填充（起度/单价由保存逻辑显式写入）
const skipAutoFill = ref(false)
watch([() => form.value.month, () => form.value.meterId], () => {
  if (editingItemId.value) return // 编辑模式不填充
  if (skipAutoFill.value) { skipAutoFill.value = false; return } // 连续录入：已显式设置
  if (!form.value.month || !form.value.meterId) return
  const ends = meterMonthEnds.value.get(form.value.meterId)
  const prevEnd = ends?.get(prevMonthOf(form.value.month))
  if (prevEnd) {
    form.value.startReading = prevEnd
  } else {
    form.value.startReading = 0
  }
})

const openEditSelected = () => {
  const row = selectedRows.value[0]
  if (!row) return
  openEdit(row)
}

const openEdit = (row: any) => {
  editingRecordId.value = row.record_id
  editingItemId.value = row.item_id
  editingOriginalMonth.value = row.month
  editingOriginalMeterId.value = row.meter_id || ''
  const rec = rowsOfRecord(row.record_id)
  recordItems.value = rec.map((r: any) => ({
    item_id: r.item_id,
    meter_id: r.meter_id,
    reading_date: r.reading_date || '',
    reader: r.reader || '',
    start_reading: Number(r.start_reading) || 0,
    end_reading: Number(r.end_reading) || 0,
    deep_prev: Number(r.deep_prev) || 0,
    deep_curr: Number(r.deep_curr) || 0,
    peak_prev: Number(r.peak_prev) || 0,
    peak_curr: Number(r.peak_curr) || 0,
    flat_prev: Number(r.flat_prev) || 0,
    flat_curr: Number(r.flat_curr) || 0,
    valley_prev: Number(r.valley_prev) || 0,
    valley_curr: Number(r.valley_curr) || 0,
    unit_price: Number(r.unit_price) || 0,
    garbage_fee: Number(r.garbage_fee) || 0,
    secondary_water_fee: Number(r.secondary_water_fee) || 0,
    sewage_fee: Number(r.sewage_fee) || 0,
    subsidy: Number(r.subsidy) || 0,
    remark: r.remark || '',
  }))
  form.value = {
    month: row.month,
    useUnit: (meters.value as any[]).find((m: any) => m.id === row.meter_id)?.use_unit || row.use_unit || '',
    meterId: row.meter_id || '',
    readingDate: row.reading_date || today,
    reader: row.reader || '',
    recordDate: row.record_date || today,
    startReading: Number(row.start_reading) || 0,
    endReading: Number(row.end_reading) || 0,
    deepPrev: Number(row.deep_prev) || 0,
    deepCurr: Number(row.deep_curr) || 0,
    peakPrev: Number(row.peak_prev) || 0,
    peakCurr: Number(row.peak_curr) || 0,
    flatPrev: Number(row.flat_prev) || 0,
    flatCurr: Number(row.flat_curr) || 0,
    valleyPrev: Number(row.valley_prev) || 0,
    valleyCurr: Number(row.valley_curr) || 0,
    unitPrice: Number(row.unit_price) || 0,
    garbageFee: Number(row.garbage_fee) || 0,
    secondaryWaterFee: Number(row.secondary_water_fee) || 0,
    sewageFee: Number(row.sewage_fee) || 0,
    subsidy: Number(row.subsidy) || 0,
    remark: row.remark || '',
  }
  drawerVisible.value = true
}

function rowsOfRecord(recordId: string) {
  return rows.value.filter(r => r.record_id === recordId)
}

const save = async () => {
  if (!form.value.month) {
    MessagePlugin.warning('请选择月份')
    return
  }
  if (!form.value.meterId) {
    MessagePlugin.warning(`请选择${meterLabel}`)
    return
  }
  if (readingInvalid.value) {
    MessagePlugin.error('止度不得小于起度')
    return
  }
  // 新增时重复校验：同月同表已有记录则拦截，避免重复录入
  if (!editingItemId.value && !lastRecordId.value && duplicateWarning.value) {
    MessagePlugin.warning(`该月该${meterLabel}已有记录，请在列表中选择该记录进行编辑`)
    return
  }
  saving.value = true
  try {
    const editedItem = {
      meter_id: form.value.meterId,
      reading_date: form.value.readingDate || '',
      reader: form.value.reader || '',
      start_reading: Number(form.value.startReading) || 0,
      end_reading: Number(form.value.endReading) || 0,
      deep_prev: Number(form.value.deepPrev) || 0,
      deep_curr: Number(form.value.deepCurr) || 0,
      peak_prev: Number(form.value.peakPrev) || 0,
      peak_curr: Number(form.value.peakCurr) || 0,
      flat_prev: Number(form.value.flatPrev) || 0,
      flat_curr: Number(form.value.flatCurr) || 0,
      valley_prev: Number(form.value.valleyPrev) || 0,
      valley_curr: Number(form.value.valleyCurr) || 0,
      unit_price: Number(form.value.unitPrice) || 0,
      garbage_fee: Number(form.value.garbageFee) || 0,
      secondary_water_fee: Number(form.value.secondaryWaterFee) || 0,
      sewage_fee: Number(form.value.sewageFee) || 0,
      subsidy: Number(form.value.subsidy) || 0,
      remark: form.value.remark || '',
    }
    const isNew = !editingItemId.value
    if (editingItemId.value) {
      // 单行改月份：仅迁移该行到目标月，其它行保留原月（不整月搬家）
      const monthChanged = form.value.month !== editingOriginalMonth.value
      if (monthChanged && recordItems.value.length > 1) {
        const others = recordItems.value.filter((it: any) => it.item_id !== editingItemId.value)
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: editingOriginalMonth.value,
          record_date: form.value.recordDate || today,
          remark: '',
          items: others.map((it: any) => ({
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: Number(it.start_reading) || 0,
            end_reading: Number(it.end_reading) || 0,
            deep_prev: Number(it.deep_prev) || 0,
            deep_curr: Number(it.deep_curr) || 0,
            peak_prev: Number(it.peak_prev) || 0,
            peak_curr: Number(it.peak_curr) || 0,
            flat_prev: Number(it.flat_prev) || 0,
            flat_curr: Number(it.flat_curr) || 0,
            valley_prev: Number(it.valley_prev) || 0,
            valley_curr: Number(it.valley_curr) || 0,
            unit_price: Number(it.unit_price) || 0,
            garbage_fee: Number(it.garbage_fee) || 0,
            secondary_water_fee: Number(it.secondary_water_fee) || 0,
            sewage_fee: Number(it.sewage_fee) || 0,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          })),
          // 该行迁往新月份：从原 record 删除
          delete_item_ids: [editingItemId.value],
        })
        await createUtilityMeterRecord({
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items: [editedItem],
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      } else {
        // 更新：整 record 全量提交，替换编辑行、保留其它行
        const items = recordItems.value.map((it: any) => {
          if (it.item_id === editingItemId.value) return editedItem
          return {
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: it.start_reading,
            end_reading: it.end_reading,
            deep_prev: it.deep_prev,
            deep_curr: it.deep_curr,
            peak_prev: it.peak_prev,
            peak_curr: it.peak_curr,
            flat_prev: it.flat_prev,
            flat_curr: it.flat_curr,
            valley_prev: it.valley_prev,
            valley_curr: it.valley_curr,
            unit_price: it.unit_price,
            garbage_fee: Number(it.garbage_fee) || 0,
            secondary_water_fee: Number(it.secondary_water_fee) || 0,
            sewage_fee: Number(it.sewage_fee) || 0,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          }
        })
        // 表计变更：删除原表计行，避免「新表新增 + 旧表残留」产生空记录
        const meterChanged = !!editingOriginalMeterId.value && editingOriginalMeterId.value !== form.value.meterId
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items,
          delete_item_ids: meterChanged ? [editingItemId.value] : [],
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      }
    } else if (lastRecordId.value) {
      // 连续录入后续条：同月 record 追加 item（同月唯一，后端要求 update）
      const existing = rowsOfRecord(lastRecordId.value)
      const items = [
        ...existing.map((r: any) => ({
          meter_id: r.meter_id,
          reading_date: r.reading_date || '',
          reader: r.reader || '',
          start_reading: Number(r.start_reading) || 0,
          end_reading: Number(r.end_reading) || 0,
          deep_prev: Number(r.deep_prev) || 0,
          deep_curr: Number(r.deep_curr) || 0,
          peak_prev: Number(r.peak_prev) || 0,
          peak_curr: Number(r.peak_curr) || 0,
          flat_prev: Number(r.flat_prev) || 0,
          flat_curr: Number(r.flat_curr) || 0,
          valley_prev: Number(r.valley_prev) || 0,
          valley_curr: Number(r.valley_curr) || 0,
          unit_price: Number(r.unit_price) || 0,
          garbage_fee: Number(r.garbage_fee) || 0,
          secondary_water_fee: Number(r.secondary_water_fee) || 0,
          sewage_fee: Number(r.sewage_fee) || 0,
          subsidy: Number(r.subsidy) || 0,
          remark: r.remark || '',
        })),
        editedItem,
      ]
      await updateUtilityMeterRecord(lastRecordId.value, {
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items,
      })
      MessagePlugin.success('已新增')
    } else {
      const res: any = await createUtilityMeterRecord({
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items: [editedItem],
      })
      const rec = res?.data?.data || res?.data || res
      lastRecordId.value = rec?.id || ''
      MessagePlugin.success('已新增')
    }
    const nextMeter = (() => {
      if (!isNew) return undefined
      // 连续录入：同使用单位、启用且当月未录入的表，按序找下一张（无则循环回第一张未录入的）
      const u = (form.value.useUnit || '').trim()
      const candidates = meters.value.filter((m: any) => {
        if (!m.enabled) return false
        if (u && (m.use_unit || '').trim() !== u) return false
        return !meterMonthEnds.value.get(m.id)?.has(form.value.month)
      })
      if (!candidates.length) return undefined
      const idx = candidates.findIndex((m: any) => m.id === form.value.meterId)
      if (idx >= 0) return candidates[idx + 1] || candidates[0]
      return candidates[0]
    })()
    if (nextMeter) {
      // 连续录入：抽屉不关闭，按启用表顺序切到下一张，月份复用、抄表日期=第一条、
      // 起度=下一张表自身上月止度（无则空）、单价=上一条、抄表人=下张表管理人员
      skipAutoFill.value = true
      const f = emptyForm()
      f.month = form.value.month
      f.useUnit = form.value.useUnit
      f.meterId = nextMeter.id
      f.readingDate = form.value.readingDate || today
      f.recordDate = form.value.recordDate || today
      f.reader = nextMeter.manager || lastReader.value || form.value.reader || ''
      const nextEnds = meterMonthEnds.value.get(nextMeter.id)
      const prevEnd = nextEnds?.get(prevMonthOf(form.value.month))
      f.startReading = prevEnd ? Number(prevEnd) : 0
      f.unitPrice = Number(form.value.unitPrice) || 0
      form.value = f
      editingRecordId.value = ''
      editingItemId.value = ''
      recordItems.value = []
    } else {
      drawerVisible.value = false
      clearSelection()
      lastRecordId.value = ''
    }
    await load()
    rememberReader(form.value.reader)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const onDrawerClose = () => { drawerVisible.value = false }

// ---- 删除 ----
const handleDelete = async () => {
  const targets = selectedRows.value
  if (!targets.length) return
  // 按 record 分组：单 item record 直接删 record；多 item record 重提交去掉目标行
  const groups = new Map<string, any[]>()
  for (const r of targets) {
    const arr = groups.get(r.record_id) || []
    arr.push(r)
    groups.set(r.record_id, arr)
  }
  try {
    for (const [recordId, itemRows] of groups) {
      const all = rowsOfRecord(recordId)
      const targetItemIds = new Set(itemRows.map(r => r.item_id))
      const remaining = all.filter(r => !targetItemIds.has(r.item_id))
      if (!remaining.length) {
        await deleteUtilityMeterRecord(recordId)
      } else {
        await updateUtilityMeterRecord(recordId, {
          category: props.category,
          month: remaining[0].month,
          record_date: remaining[0].record_date || '',
          remark: '',
          delete_item_ids: Array.from(targetItemIds),
          items: remaining.map((r: any) => ({
            meter_id: r.meter_id,
            reading_date: r.reading_date || '',
            reader: r.reader || '',
            start_reading: Number(r.start_reading) || 0,
            end_reading: Number(r.end_reading) || 0,
            deep_prev: Number(r.deep_prev) || 0,
            deep_curr: Number(r.deep_curr) || 0,
            peak_prev: Number(r.peak_prev) || 0,
            peak_curr: Number(r.peak_curr) || 0,
            flat_prev: Number(r.flat_prev) || 0,
            flat_curr: Number(r.flat_curr) || 0,
            valley_prev: Number(r.valley_prev) || 0,
            valley_curr: Number(r.valley_curr) || 0,
            unit_price: Number(r.unit_price) || 0,
            garbage_fee: Number(r.garbage_fee) || 0,
            secondary_water_fee: Number(r.secondary_water_fee) || 0,
            sewage_fee: Number(r.sewage_fee) || 0,
            remark: r.remark || '',
          })),
        })
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 打印 ----
const printVisible = ref(false)
const printBusy = ref(false)
const catalogBusy = ref(false)
const printLoaded = ref(false)
const printUrl = ref('')
const printCount = ref(0)

const catalogValueOf = (r: any, key: string): string => {
  switch (key) {
    case 'month': return r.month
    case 'meter': return r.meter_alias
    case 'start_reading': return fmtNum(r.start_reading)
    case 'end_reading': return fmtNum(r.end_reading)
    case 'rate': return fmtNum(r.rate)
    case 'usage': return `${fmtNum(r.usage)} ${unitLabel.value}`
    case 'unit_price': return fmtUnitPrice(r.unit_price)
    case 'subsidy': return r.subsidy === '' || r.subsidy == null ? '' : fmtMoney(r.subsidy)
    case 'amount': return `${fmtMoney(r.amount)} 元`
    case 'reading_date': return r.reading_date || ''
    case 'reader': return r.reader || ''
    case 'meter_no': return r.meter_no || ''
    case 'use_unit': return r.use_unit || ''
    case 'default_unit_price': return r.default_unit_price === '' || r.default_unit_price == null ? '' : fmtNum(r.default_unit_price)
    case 'meter_mode': return r.meter_mode === 'auto' ? '自动抄表' : r.meter_mode === 'manual' ? '手动抄表' : (r.meter_mode || '')
    case 'install_date': return r.install_date || ''
    case 'remark': return r.remark || ''
    default: return ''
  }
}

const handlePrint = async () => {
  const rowsToPrint = selectedRows.value
  if (!rowsToPrint.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = visibleColDefs.value.map((c: any) => ({
      key: c.key,
      label: c.label,
      value: (r: any) => catalogValueOf(r, c.key),
    }))
    const bytes = await generateCatalogPdf({ title: `${categoryLabel.value}目录`, columns: cols, rows: rowsToPrint })
    printCount.value = rowsToPrint.length
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printLoaded.value = false
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 水表配置设置抽屉 ----
const settingsVisible = ref(false)
const savingMeter = ref(false)
const meterFormVisible = ref(false)
const kindManageVisible = ref(false)
const meterForm = ref<any>({})
// 计量层级按类别: 电表 普通/分时; 水表 总表/分表/消防; 气表 普通
const meterTypeOptions = computed(() => {
  if (props.category === 'electricity') return [
    { label: '普通', value: 'normal' },
    { label: '分时', value: 'time' },
  ]
  if (props.category === 'water') return [
    { label: '总表', value: 'total' },
    { label: '分表', value: 'sub' },
    { label: '消防', value: 'fire' },
  ]
  return [{ label: '普通', value: 'normal' }]
})
const meterTypeLabel = (t: string) => meterTypeOptions.value.find(o => o.value === t)?.label || '普通'
const meterAliasInput = ref()
const settingsWidth = ref<string>(loadSettingsWidth())
function loadSettingsWidth(): string {
  try {
    const v = parseInt(localStorage.getItem('weknora-utility-meter-settings-width') || '')
    if (!Number.isNaN(v)) return clampWidth(v, 520) + 'px'
  } catch { /* ignore */ }
  return '680px'
}

const openSettings = async () => {
  settingsVisible.value = true
  meterFormVisible.value = false
  if (!meters.value.length) {
    try {
      const res: any = await listUtilityMeters({ category: props.category })
      const mets = res?.data || res || []
      meters.value = Array.isArray(mets) ? mets : []
    } catch { /* ignore */ }
  }
}

const openMeterForm = (m: any) => {
  // 新增默认用途:水表(总表/分表/消防均属工商业)默认工商业;电表/气表默认居民
  const defaultKind = props.category === 'water' ? 'production' : 'dorm'
  // 新增默认类型:电表/气表默认普通;水表类型(总表/分表/消防)由用户按需选择
  const defaultType = props.category === 'water' ? '' : 'normal'
  meterForm.value = m ? {
    id: m.id,
    alias: m.alias || '',
    meter_no: m.meter_no || '',
    // 水表历史脏数据 normal 不在合法类型内,编辑时置空引导重选
    meter_type: props.category === 'water' && !['total', 'sub', 'fire'].includes(m.meter_type)
      ? '' : (m.meter_type || ''),
    meter_kind: m.meter_kind || defaultKind,
    owner_unit: m.owner_unit || '',
    rate: Number(m.rate) > 0 ? m.rate : 1,
    default_unit_price: Number(m.default_unit_price) || 0,
    use_unit: m.use_unit || '',
    manager: m.manager || '',
    contact: m.contact || '',
    meter_mode: m.meter_mode || 'manual',
    install_date: m.install_date || '',
    remark: m.remark || '',
    enabled: m.enabled !== false,
  } : {
    id: '', alias: '', meter_no: nextMeterNo(), meter_type: defaultType,
    meter_kind: defaultKind, owner_unit: '', rate: 1, default_unit_price: 0,
    use_unit: '', manager: '', contact: '', meter_mode: 'manual',
    install_date: '', remark: '', enabled: true,
  }
  meterFormVisible.value = true
  nextTick(() => { if (!m) (meterAliasInput.value as any)?.focus?.() })
}

const saveMeter = async () => {
  if (!meterForm.value.alias?.trim()) {
    MessagePlugin.warning('请填写别名')
    return
  }
  savingMeter.value = true
  try {
    const payload = {
      category: props.category,
      alias: meterForm.value.alias.trim(),
      meter_no: meterForm.value.meter_no || '',
      meter_type: meterForm.value.meter_type || '',
      meter_kind: meterForm.value.meter_kind || 'dorm',
      owner_unit: meterForm.value.owner_unit || '',
      rate: Number(meterForm.value.rate) > 0 ? Number(meterForm.value.rate) : 1,
      default_unit_price: Number(meterForm.value.default_unit_price) || 0,
      use_unit: meterForm.value.use_unit || '',
      manager: meterForm.value.manager || '',
      contact: meterForm.value.contact || '',
      meter_mode: meterForm.value.meter_mode || 'manual',
      install_date: meterForm.value.install_date || '',
      remark: meterForm.value.remark || '',
      enabled: meterForm.value.enabled !== false,
    }
    const isEdit = !!meterForm.value.id
    if (isEdit) {
      await updateUtilityMeter(meterForm.value.id, payload)
      MessagePlugin.success('已保存')
    } else {
      await createUtilityMeter(payload)
      MessagePlugin.success('已新增')
    }
    await loadMetersOnly()
    if (isEdit) {
      // 编辑保存后关闭
      meterFormVisible.value = false
    } else {
      // 连续新增：保持表单展开，表号自动递增，清空其余字段并聚焦别名
      meterForm.value = {
        id: '',
        alias: '',
        meter_no: nextMeterNo(),
        meter_type: props.category === 'water' ? '' : 'normal',
        meter_kind: props.category === 'water' ? 'production' : 'dorm',
        owner_unit: '',
        rate: '',
        default_unit_price: '',
        use_unit: '',
        manager: '',
        contact: '',
        meter_mode: 'manual',
        install_date: '',
        remark: '',
        enabled: true,
      }
      meterFormVisible.value = true
      nextTick(() => { (meterAliasInput.value as any)?.focus?.() })
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}
const meterNoPrefix = () => (props.category === 'water' ? 'W' : props.category === 'electricity' ? 'E' : 'G')
const nextMeterNo = () => {
  let max = 0
  for (const m of meters.value) {
    const mm = String(m.meter_no || '').match(/^[A-Za-z]*(\d+)$/)
    if (mm) max = Math.max(max, parseInt(mm[1], 10))
  }
  return meterNoPrefix() + String(max + 1).padStart(3, '0')
}

const toggleEnabled = async (m: any, v: any) => {
  try {
    await updateUtilityMeter(m.id, { ...m, enabled: !!v })
    m.enabled = !!v
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}

const deleteMeter = async (m: any) => {
  try {
    await deleteUtilityMeter(m.id)
    MessagePlugin.success('已删除')
    meters.value = meters.value.filter(x => x.id !== m.id)
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

const loadMetersOnly = async () => {
  try {
    const res: any = await listUtilityMeters({ category: props.category })
    const mets = res?.data || res || []
    meters.value = Array.isArray(mets) ? mets : []
    sortMetersLocal()
  } catch { /* ignore */ }
}

// ---- 表计配置拖动排序（sort_order 持久化） ----
// .meter-table 位于 t-tab-panel 内(懒渲染,同一时刻仅激活面板存在),直接 DOM 查询取当前面板容器
const meterSortableEl = ref<HTMLElement | null>(null)
const setMeterSortableRef = (el: any) => {
  meterSortableEl.value = Array.isArray(el) ? (el[0] || null) : (el || null)
}
let sortableInst: any = null
const sortMetersLocal = () => {
  meters.value = [...meters.value].sort((a: any, b: any) =>
    (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0) ||
    (a.created_at || '').localeCompare(b.created_at || ''))
}
const initMeterSortable = () => {
  const el = meterSortableEl.value || document.querySelector('.meter-table') as HTMLElement | null
  if (!el) return
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
  try {
    sortableInst = Sortable.create(el, {
      draggable: '.meter-table-row',
      handle: '.meter-drag-handle',
      animation: 150,
      ghostClass: 'meter-row-ghost',
      onEnd: persistMeterSort,
    })
  } catch (e) {
    console.error('[Sortable] init failed:', e)
  }
}
const persistMeterSort = async () => {
  const el = meterSortableEl.value || document.querySelector('.meter-table') as HTMLElement | null
  if (!el) return
  const ids = [...el.querySelectorAll('.meter-table-row')]
    .map(r => r.getAttribute('data-id')).filter(Boolean) as string[]
  if (!ids.length) return
  try {
    await sortUtilityMeters(props.category, ids)
    // 重新加载表计:后端已把全局 sort_order 重整(未拖动分组顺延),
    // 保证新增记录下拉顺序与设置分组内顺序一致
    await loadMetersOnly()
    MessagePlugin.success('排序已保存')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '排序保存失败')
    await loadMetersOnly()
  }
}
watch([activeMeterGroup, () => activeGroupItems.value], async () => {
  await nextTick()
  setTimeout(initMeterSortable, 120)
})
// 设置抽屉 v-if 懒渲染:打开抽屉后 DOM 才存在,必须重新初始化拖拽
watch(settingsVisible, async (v) => {
  if (v) {
    await nextTick()
    setTimeout(initMeterSortable, 150)
  }
})
onBeforeUnmount(() => {
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
})

// ---- 抽屉宽度拖动 ----
const drawerWidth = ref<string>(loadDrawerWidth())
function loadDrawerWidth(): string {
  try {
    const v = parseInt(localStorage.getItem(`weknora-utility-${props.category}-drawer-width`) || '')
    if (!Number.isNaN(v)) return clampWidth(v, 480) + 'px'
  } catch { /* ignore */ }
  return '640px'
}
let dragging = false
let startX = 0
let startW = 640
let draggingS = false
let startXS = 0
let startWS = 680
const clampWidth = (w: number, min: number) => Math.min(Math.floor(window.innerWidth * 0.95), Math.max(min, w))

// 记录抽屉宽度拖动（发票管理同款：独立手柄 + document 监听）
const onDrawerResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  dragging = true
  startX = e.clientX
  startW = parseInt(drawerWidth.value) || 640
  document.addEventListener('mousemove', onDrawerResizeMove)
  document.addEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = 'col-resize'
}
const onDrawerResizeMove = (e: MouseEvent) => {
  if (!dragging) return
  drawerWidth.value = clampWidth(startW + (startX - e.clientX), 480) + 'px'
}
const onDrawerResizeEnd = () => {
  if (!dragging) return
  dragging = false
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = ''
  try { localStorage.setItem(`weknora-utility-${props.category}-drawer-width`, drawerWidth.value) } catch { /* ignore */ }
}

// 设置抽屉宽度拖动
const onSettingsResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  draggingS = true
  startXS = e.clientX
  startWS = parseInt(settingsWidth.value) || 680
  document.addEventListener('mousemove', onSettingsResizeMove)
  document.addEventListener('mouseup', onSettingsResizeEnd)
  document.body.style.cursor = 'col-resize'
}
const onSettingsResizeMove = (e: MouseEvent) => {
  if (!draggingS) return
  settingsWidth.value = clampWidth(startWS + (startXS - e.clientX), 520) + 'px'
}
const onSettingsResizeEnd = () => {
  if (!draggingS) return
  draggingS = false
  document.removeEventListener('mousemove', onSettingsResizeMove)
  document.removeEventListener('mouseup', onSettingsResizeEnd)
  document.body.style.cursor = ''
  try { localStorage.setItem('weknora-utility-meter-settings-width', settingsWidth.value) } catch { /* ignore */ }
}

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
// 单价统一保留最多三位小数,末尾 0 不显示(如 3.4 / 5.22 / 2.196)
const fmtUnitPrice = (v: any) => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return String(Math.round(n * 1000) / 1000)
}

onMounted(() => { load() })
onBeforeUnmount(() => {
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.removeEventListener('mousemove', onSettingsResizeMove)
  document.removeEventListener('mouseup', onSettingsResizeEnd)
})
</script>

<style lang="less" scoped>
.utility-meter-tab {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding-top: 4px;
}

/* 筛选工具栏（与合同管理一致） */
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

    .doc-date-picker {
      width: 160px;
    }

    .doc-filter-select {
      width: 160px;
    }
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

/* 列表组件样式（与合同/发票/知识库列表保持一致，scoped 自包含） */
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
}

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

  .cell {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
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

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.row-selected {
    background: var(--td-brand-color-light);
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
.row-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-mono {
  font-family: var(--app-font-family);
}

.os-neg { color: var(--td-error-color); }
.row-dash { color: var(--td-text-color-placeholder); }
.row-expand-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  margin-right: 2px;
  font-size: 12px;
  line-height: 1;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  border-radius: 4px;
  user-select: none;
  &:hover { background: var(--td-bg-color-container-hover); color: var(--td-brand-color); }
}

/* 分时表行内四时段展开（参考电量明细布局） */
.doc-list-expand {
  display: contents;
}
.doc-list-expand .expand-inner {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container-hover);
  border-bottom: 1px solid var(--td-component-stroke);
}
.expand-grid {
  display: grid;
  grid-template-columns: 1fr 0.9fr 0.9fr 0.7fr 1.1fr;
  min-width: 0;
}
.expand-grid--row {
  border-top: 1px solid var(--td-component-stroke);
}
.expand-grid--row:hover { background: var(--td-bg-color-container); }
.expand-grid--total {
  border-top: 1px solid var(--td-component-stroke);
  font-weight: 600;
  background: var(--td-bg-color-container);
}
.expand-cell {
  padding: 6px 12px;
  font-size: 12px;
  line-height: 18px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.expand-head {
  color: var(--td-text-color-placeholder);
  background: var(--td-bg-color-secondarycontainer);
  font-weight: 600;
}
.expand-name { color: var(--td-text-color-primary); }
.expand-mono { font-family: var(--app-font-family); color: var(--td-text-color-primary); }

.meter-list-scroll {
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

  .meter-empty-icon {
    color: var(--td-text-color-placeholder);
  }

  .meter-empty-text {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}

/* 底部汇总（与电费/光伏一致） */
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

/* 抽屉 resize 手柄（发票管理同款，teleport 到 body 避免被抽屉遮罩遮挡） */
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

/* 浮动工具栏（与电费/合同一致） */
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

    .batch-bar-count {
      font-size: 13px;
      color: var(--td-text-color-primary);
    }

    .batch-bar-clear {
      color: var(--td-brand-color);
    }
  }

  .batch-bar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(6px);
}

/* 编辑抽屉 */
.meter-drawer-body {
  padding: 4px 0 24px;
}

/* 新增/编辑记录：两列紧凑布局 */
.rec-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 20px;
}
.rec-field {
  min-width: 0;

  /* 编辑框宽度统一：输入/选择/日期组件占满列宽 */
  :deep(.t-input__wrap),
  :deep(.t-select__wrap),
  :deep(.t-date-picker) {
    width: 100%;
    min-width: 0;
  }

  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;
  }
  .reading-row { display: flex; align-items: center; gap: 8px; }
  .reading-input { flex: 1; }
  .reading-sep { color: var(--td-text-color-placeholder); flex: 0 0 auto; }
  .field-error-text {
    font-size: 12px;
    color: var(--td-error-color);
    margin-top: 4px;
    line-height: 1.4;
  }
}
.rec-field--wide {
  grid-column: 1 / -1;
}
/* 分时电表：时段起/止并排，任意宽度不截断 */
.rec-period-pair {
  display: grid;
  grid-template-columns: 1fr 24px 1fr;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-width: 0;
  .t-input__wrap { width: 100%; min-width: 0; }
}
.rec-period-sep {
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  user-select: none;
}
.readonly-val {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 13px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-component);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  padding: 0 10px;
}
.calc-val-lg {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-brand-color);
  font-variant-numeric: tabular-nums;
}
.field-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 4px;
  line-height: 1.4;
}
.calc-row {
  display: flex;
  align-items: center;
  gap: 16px;
  min-height: 30px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .calc-rate { color: var(--td-brand-color); }
  .calc-val { color: var(--td-text-color-primary); font-weight: 600; font-variant-numeric: tabular-nums; }
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 14px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 0 0 30%;
  max-width: 30%;
  padding-right: 16px;

  label {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;

    .required {
      color: var(--td-error-color);
    }
  }

  .desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex: 1;
  min-width: 0;
}

.reading-row {
  display: flex;
  align-items: center;
  gap: 10px;

  .reading-input {
    flex: 1;
  }

  .reading-sep {
    color: var(--td-text-color-placeholder);
  }
}

.calc-row {
  display: flex;
  gap: 24px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--td-bg-color-container-hover);
  font-size: 13px;
  color: var(--td-text-color-primary);

  .calc-val {
    font-size: 15px;
    color: var(--td-brand-color);
    font-family: var(--app-font-family);
  }
}

.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 用途管理 */
.kind-manage-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.kind-manage-tip {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
  margin: 0 0 4px;
}
.kind-manage-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
.kind-manage-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.kind-manage-row .t-input {
  flex: 1;
}
.kind-value-tag {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  margin-right: 4px;
}
.kind-builtin-tag {
  width: 32px;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

/* 水表配置抽屉 */
.meter-search {
  margin-bottom: 12px;
}
.meter-settings-body {
  padding: 4px 0 24px;
}

.settings-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.meter-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 分组标签:复用租户核算设置标签卡样式(选中绿色下划线) */
.meter-settings-tabs {
  margin-bottom: 10px;

  :deep(.t-tabs__header) {
    margin-bottom: 8px;
  }

  :deep(.t-tabs__nav-item) {
    font-size: 13px;
  }

  /* 下划线贴合文字:消除 wrapper 左右内边距/外边距,标签间保留间距 */
  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0;
    margin: 0;
  }

  :deep(.t-tabs__nav-item:not(:first-child) .t-tabs__nav-item-wrapper) {
    margin-left: 8px;
  }

  :deep(.t-tabs__content) {
    overflow: visible;
  }
}

.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);

  .meter-table-head,
  .meter-table-row {
    display: grid;
    grid-template-columns: 0.4fr 1.4fr 1fr 1.2fr 0.7fr 1.2fr 1fr 0.7fr 1.2fr;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    font-size: 12px;
  }

  .meter-drag-th {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
  }

  .meter-table-head {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .meter-table-row {
    border-bottom: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    cursor: pointer;
    transition: background 0.15s;

    .meter-drag-handle {
      display: flex;
      align-items: center;
      justify-content: center;
      color: var(--td-text-color-placeholder);
      cursor: grab;

      &:active { cursor: grabbing; }
    }

    &:hover { background: var(--td-bg-color-secondarycontainer); }
    &.editing { background: var(--td-brand-color-light); }
    &:last-child { border-bottom: none; }

    .mtr-alias {
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mtr-mono {
      font-variant-numeric: tabular-nums;
      color: var(--td-text-color-primary);
    }
    .mtr-type {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--td-text-color-secondary);
    }
    .mtr-owner {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mtr-switch {
      display: flex;
      align-items: center;
      justify-self: start;

      .t-switch { width: auto; }
    }

    .meter-row-actions {
      display: flex;
      align-items: center;
      gap: 2px;
      justify-content: flex-start;
    }
  }

  .meter-row-ghost {
    opacity: 0.45;
    background: var(--td-brand-color-light);
  }
}

.meter-form {
  margin-top: 0;
  grid-column: 1 / -1;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 14px;
  background: var(--td-bg-color-secondarycontainer);

  &.meter-form--inline {
    margin: 0;
    border-left: none;
    border-right: none;
    border-bottom: none;
    border-radius: 0;
    grid-column: auto;
  }

  .meter-form-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    color: var(--td-text-color-primary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
  }

  .form-item {
    label {
      display: block;
      font-size: 12px;
      color: var(--td-text-color-secondary);
      margin-bottom: 4px;

      .required {
        color: var(--td-error-color);
      }
    }

    &--full {
      margin-top: 12px;
    }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }
}

/* 打印弹窗 */
.meter-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3100;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}

.meter-print-dialog {
  width: min(960px, 92vw);
  max-width: 100%;
  height: min(720px, 88vh);
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}

.meter-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex: 0 0 auto;

  .meter-print-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.meter-print-body {
  flex: 1;
  min-height: 0;
  background: var(--td-bg-color-container);
  overflow: auto;
}

.print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}

.print-preview-loading {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.meter-print-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--td-component-stroke);
  flex: 0 0 auto;
}
</style>
