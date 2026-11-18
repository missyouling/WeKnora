<!--
  人事管理（第 45 轮）：花名册 / 社保管理 / 公积金管理 单页三卡片。

  骨架 100% 照抄 FleetRecordList.vue（日常事务中台）：
    ① 顶部 DashboardKpiGroup（恰好 3 张 type 卡，无任何汇总 / 历史卡）；
    ② 中部 BusinessFilterBar（#batch-actions 浮动条 + #right-extra 业务按钮）；
    ③ 下部 doc-list-scroll > doc-list-view > t-table（勾选感知合计行）+ 各类抽屉。

  表头首帧一次成型：切换 tab / 二级视图时先把 headerReady 置 false 撤掉表格，
  await Promise.all([字段配置, 列表数据]) 后一次性置 true —— 绝不先渲染
  「只有复选框」的半套列（第 39 轮门禁）。
-->
<template>
  <div class="biz-page hr-page" :data-hr-tab="activeTab">
    <!-- ① 顶部 KPI：3 张卡，包裹 div 带 data-hr-kpi-group 便于真机断言 -->
    <div class="overview-group" data-hr-kpi-group>
      <DashboardKpiGroup :loading="false" :skeleton="false" :cards="kpiCards" :active-type="activeTab"
        @card-click="onCardClick" />
    </div>

    <div class="biz-list-view">
      <!-- 第 57 轮：花名册合并「在职管理 / 离职管理」双标签；
           公积金标签顺序调整为「职工明细」在前、「月度账单」在后；
           社保四标签与设置抽屉四胶囊严格 1:1。 -->
      <div v-if="activeTab === 'hr-roster' || activeTab === 'hr-fund' || activeTab === 'hr-social'
        || activeTab === 'hr-contract' || activeTab === 'hr-dorm'"
        class="hr-subview-bar" data-hr-subview-bar>
        <t-radio-group v-if="activeTab === 'hr-roster'" v-model="rosterStatusView" size="small"
          @change="onRosterViewChange">
          <t-radio-button value="在职">在职人员</t-radio-button>
          <t-radio-button value="离职">离职人员</t-radio-button>
        </t-radio-group>
        <t-radio-group v-else-if="activeTab === 'hr-contract'" v-model="contractSubView" size="small"
          @change="onContractSubViewChange">
          <t-radio-button value="labor">劳动合同</t-radio-button>
          <t-radio-button value="service">劳务合同</t-radio-button>
        </t-radio-group>
        <t-radio-group v-else-if="activeTab === 'hr-fund'" v-model="fundSubView" size="small"
          @change="onFundSubViewChange">
          <t-radio-button value="ledger">职工明细</t-radio-button>
          <t-radio-button value="bills">月度账单</t-radio-button>
        </t-radio-group>
        <t-radio-group v-else-if="activeTab === 'hr-social'" v-model="socialSubView" size="small"
          @change="onSocialSubViewChange">
          <t-radio-button value="receipt">回盘表</t-radio-button>
          <t-radio-button value="normal">正常记录</t-radio-button>
          <t-radio-button value="abnormal">补退记录</t-radio-button>
          <t-radio-button value="bills">月度账单</t-radio-button>
        </t-radio-group>
        <!-- 第 61 轮：宿舍管理五标签（与设置抽屉五胶囊 1:1） -->
        <t-radio-group v-else v-model="dormSubView" size="small" data-hr-dorm-tabs
          @change="onDormViewChange">
          <t-radio-button v-for="v in HR_DORM_VIEWS" :key="v.key" :value="v.key">{{ v.label }}</t-radio-button>
        </t-radio-group>
      </div>

      <!-- ② 筛选工具栏 -->
      <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
        :hide-search="toolbarHideSearch"
        :columns="columnDefsForFilter" :visible-keys="visibleColKeys" :selected-count="selectedKeys.size"
        settings-text="设置" :primary-action="primaryAction" @columns-change="onColumnsChange"
        @reset-columns="resetColumns" @select-all-columns="selectAllColumns" @filter-change="applyFilter"
        @settings="settingsVisible = true" @primary="onPrimaryAction" @clear-selection="clearSelection">
        <template #type-extra>
          <!-- 花名册：在职 / 离职共用部门筛选（离职标签不再有独立 tab） -->
          <t-select v-if="activeTab === 'hr-roster'" v-model="rosterDeptFilter"
            :options="rosterDeptSelectOptions" placeholder="部门" :clearable="false"
            class="hr-status-select" @change="onRosterDeptChange" />
          <!-- 第 56 轮：删除「原始记录 / 汇总记录」下拉，改由上方四标签切换。
               第 57 轮：回盘表新增「参保状态」筛选（默认参保缴费）；月度账单只留月份选择器。 -->
          <template v-if="activeTab === 'hr-social'">
            <t-select v-if="socialSubView === 'receipt'" v-model="siPensionStatusFilter"
              :options="SI_PENSION_STATUS_OPTIONS" placeholder="参保状态" :clearable="false"
              class="hr-status-select" @change="applyFilter" />
            <t-select v-if="socialSubView !== 'bills'" v-model="siDeptFilter"
              :options="siDeptOptions" placeholder="全部部门" :clearable="false"
              class="hr-status-select" @change="applyFilter" />
            <t-select v-if="socialSubView === 'normal' || socialSubView === 'abnormal'"
              v-model="siSchemeFilter" :options="SI_INSURANCE_OPTIONS" placeholder="全部险种" :clearable="false"
              class="hr-status-select" @change="applyFilter" />
            <t-date-picker v-if="socialSubView === 'bills'"
              v-model="siBillMonthFilter" mode="month" format="YYYY-MM" value-type="YYYY-MM"
              placeholder="账单月份" :clearable="false" allow-input class="hr-month-picker"
              @change="applyFilter" />
            <!-- 第 58 轮：回盘表是「人员增减申请」全量回盘，没有账期维度，
                 工具栏只留 搜索 + 参保状态 + 部门 + 刷新（图 5 要求删除月份框）。 -->
            <t-date-picker v-else-if="socialSubView === 'normal' || socialSubView === 'abnormal'"
              v-model="siMonthFilter" mode="month" format="YYYY-MM" value-type="YYYY-MM"
              placeholder="账单月份" :clearable="false" allow-input class="hr-month-picker"
              @change="applyFilter" />
          </template>
          <!-- 劳动合同：只保留部门筛选（第 57 轮删除合同状态下拉） -->
          <t-select v-if="activeTab === 'hr-contract'" v-model="rosterDeptFilter"
            :options="rosterDeptSelectOptions" placeholder="部门" :clearable="false"
            class="hr-status-select" @change="onContractFilterChange" />
          <!-- 第 59 轮：合同状态筛选（劳动合同默认「正常」，劳务合同默认「全部状态」） -->
          <t-select v-if="activeTab === 'hr-contract'" v-model="contractStatusView"
            :options="CONTRACT_STATUS_OPTIONS" placeholder="合同状态" :clearable="false"
            class="hr-status-select" data-hr-contract-status @change="applyFilter" />
          <!-- 公积金：职工明细给账户状态；月度账单只留月份选择器 -->
          <t-select v-if="activeTab === 'hr-fund' && fundSubView === 'ledger'" v-model="fundStatusView"
            :options="FUND_STATUS_OPTIONS" placeholder="账户状态" :clearable="false"
            class="hr-status-select" @change="onFundStatusChange" />
          <t-date-picker v-if="activeTab === 'hr-fund' && fundSubView === 'bills'"
            v-model="fundBillMonthFilter" mode="month" format="YYYY-MM" value-type="YYYY-MM"
            placeholder="账单月份" :clearable="false" allow-input class="hr-month-picker"
            @change="onFundBillMonthChange" />
          <!-- 第 61 轮：宿舍费用信息 = 月份 + 房源类型；住宿记录 = 房源类型 -->
          <template v-if="activeTab === 'hr-dorm'">
            <t-date-picker v-if="dormSubView === 'fee'" v-model="dormMonthFilter"
              mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="账单月份"
              clearable allow-input class="hr-month-picker" data-hr-dorm-month
              @change="applyFilter" />
            <t-select v-if="dormSubView === 'stay'" v-model="dormRoomTypeFilter"
              :options="DORM_ROOM_TYPE_OPTIONS" placeholder="全部房源" :clearable="false"
              class="hr-status-select" data-hr-dorm-room-type @change="applyFilter" />
            <t-select v-if="dormSubView === 'dorm' || dormSubView === 'public'"
              v-model="dormStatusFilter" :options="DORM_STATUS_FILTER_OPTIONS" placeholder="全部状态"
              :clearable="false" class="hr-status-select" data-hr-dorm-status @change="applyFilter" />
            <t-select v-if="dormSubView === 'asset'" v-model="assetStatusFilter"
              :options="ASSET_STATUS_FILTER_OPTIONS" placeholder="全部状态" :clearable="false"
              class="hr-status-select" data-hr-dorm-asset-status @change="applyFilter" />
            <template v-if="dormSubView === 'fee'">
              <t-select v-model="dormDeptFilter" :options="siDeptOptions" placeholder="全部部门"
                :clearable="false" class="hr-status-select" data-hr-dorm-dept @change="applyFilter" />
              <t-select v-model="dormRoomTypeFilterFee" :options="ROOM_TYPE_FEE_OPTIONS"
                placeholder="全部房源" :clearable="false" class="hr-status-select"
                data-hr-dorm-room-type-fee @change="applyFilter" />
            </template>
          </template>
        </template>
        <template #batch-actions>
          <!-- display:contents 让按钮仍是 .batch-bar-actions 的直接 flex 项，不额外产生布局层 -->
          <div class="hr-batch-actions" data-hr-batch-bar>
            <t-button theme="default" variant="outline" size="small" :loading="exporting"
              data-hr-export @click="handleExportExcel">
              <template #icon><t-icon name="download" size="14px" /></template>
              导出 Excel
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="printBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印清单
            </t-button>
            <!-- 公积金：封存 / 启封（第 59 轮：按选中记录状态互斥显示 + 二次确认气泡） -->
            <template v-if="activeTab === 'hr-fund' && fundSubView === 'ledger'">
              <t-popconfirm v-if="fundSealMode === 'seal'" theme="warning"
                :content="`确定要封存选中的 ${selectedRowKeys.length} 名职工公积金账户吗？`"
                @confirm="handleSeal(true)">
                <t-button theme="default" variant="outline" size="small"
                  :disabled="!selectedRowKeys.length" data-hr-fund-seal>
                  <template #icon><t-icon name="lock-on" size="14px" /></template>
                  封存
                </t-button>
              </t-popconfirm>
              <t-popconfirm v-else-if="fundSealMode === 'unseal'" theme="warning"
                :content="`确定要启封选中的 ${selectedRowKeys.length} 名职工公积金账户吗？`"
                @confirm="handleSeal(false)">
                <t-button theme="default" variant="outline" size="small"
                  :disabled="!selectedRowKeys.length" data-hr-fund-unseal>
                  <template #icon><t-icon name="lock-off" size="14px" /></template>
                  启封
                </t-button>
              </t-popconfirm>
            </template>
            <!-- 第 61 轮：住宿记录批量办理调宿 / 退宿（自动重算可用床位与宿舍状态） -->
            <template v-if="activeTab === 'hr-dorm' && dormSubView === 'stay'">
              <t-button theme="default" variant="outline" size="small"
                :disabled="!selectedRowKeys.length" data-hr-dorm-transfer
                @click="openDormTransfer">
                <template #icon><t-icon name="swap" size="14px" /></template>
                办理调宿
              </t-button>
              <t-button theme="default" variant="outline" size="small"
                :disabled="!selectedRowKeys.length" data-hr-dorm-checkout
                @click="openDormCheckout">
                <template #icon><t-icon name="logout" size="14px" /></template>
                办理退宿
              </t-button>
            </template>
            <!-- 第 57 轮职责边界（合并进花名册双标签）：
                 在职管理 = 在职池 → 只能「办理离职」；
                 离职管理 = 离职池 → 只能「恢复在职」，且必须二次确认。 -->
            <!-- 第 55 轮：每人离职证明独立，办理离职只支持单条记录 -->
            <t-tooltip :content="selectedRowKeys.length > 1
              ? '每位员工离职证明不同，请单选 1 名员工逐一办理离职' : '为所选员工办理离职'"
              placement="top">
              <t-button v-if="activeTab === 'hr-roster' && rosterStatusView === '在职'"
                theme="default" variant="outline" size="small"
                :disabled="selectedRowKeys.length !== 1" @click="handleResign">
                <template #icon><t-icon name="logout" size="14px" /></template>
                办理离职
              </t-button>
            </t-tooltip>
            <t-popconfirm v-if="activeTab === 'hr-roster' && rosterStatusView === '离职'"
              theme="warning" :content="restoreConfirmText" placement="top"
              :confirm-btn="{ content: '恢复在职', theme: 'primary' }" :cancel-btn="{ content: '取消' }"
              @confirm="handleRestore">
              <t-button theme="default" variant="outline" size="small"
                :disabled="!selectedRowKeys.length" @click.stop>
                <template #icon><t-icon name="rollback" size="14px" /></template>
                恢复在职
              </t-button>
            </t-popconfirm>
            <!-- 第 56 轮：社保「生成汇总」下线，改为右侧常驻「生成账单」 -->
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedRowKeys.length} 条记录吗？删除后不可恢复。`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleBatchDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                批量删除
              </t-button>
            </t-popconfirm>
          </div>
        </template>

        <!-- 第 57 轮：导入 / 生成账单严格按标签显隐。
             回盘表 / 正常记录 / 异常记录 / 职工明细 → 只导入不生成；
             社保月度账单 / 公积金月度账单 → 只生成不导入。
             第 58 轮：两者都排在「设置」之后（after-settings 插槽），
             右侧顺序恒为 [☰ 字段] + [⚙ 设置] + [导入 | 生成账单]。 -->
        <template #after-settings>
          <t-button v-if="toolbarShowImport" :theme="toolbarImportPrimary ? 'primary' : 'default'"
            :variant="toolbarImportPrimary ? 'base' : 'outline'" size="small"
            data-hr-import @click="onImportClick">
            <template #icon><t-icon name="upload" size="14px" /></template>
            导入
          </t-button>
          <!-- 第 61 轮：没有任何新增/导入时，生成账单就是本页唯一实心绿底主按钮 -->
          <t-button v-if="toolbarShowGenerateBill" theme="primary" size="small"
            data-hr-generate-bill @click="handleGenerateBill">
            <template #icon><t-icon name="file-add" size="14px" /></template>
            {{ activeTab === 'hr-dorm' ? '生成费用明细' : '生成账单' }}
          </t-button>
        </template>
      </BusinessFilterBar>

      <!-- ③ 列表 -->
      <div class="doc-list-scroll" :data-hr-row-count="rows.length">
        <!-- 表头就绪前只渲染占位块：绝不先渲染半套列 -->
        <div v-if="!headerReady" class="doc-list-view">
          <div class="hr-empty-placeholder" />
        </div>
        <div v-else :class="listViewClass" :style="headerFixStyle">
          <t-table :data="rows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="tableLoading" :max-height="'100%'"
            :foot-data="tableFootData" :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header
            class="doc-table hr-table"
            :row-class-name="({ row }: any) => selectedKeys.has(hrRowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <!-- 4 个系统状态列：**只读彩色 Tag**（具名插槽 #<colKey>）。
                 TDesign 1.19 的 cell 回调不注入 h，用 cell+h(Tag) 会渲染成空白；
                 同一个插槽还会被复用于表头，此时必须输出纯文本列标题，
                 否则表头会变成「—」或一枚彩色 Tag。 -->
            <template #在职状态="{ row }: any">
              <span v-if="isHeaderContext(row)">在职状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '在职状态'))" variant="light" size="small">
                {{ colValue(row, '在职状态') }}
              </t-tag>
            </template>
            <template #账期状态="{ row }: any">
              <span v-if="isHeaderContext(row)">账期状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '账期状态'))" variant="light" size="small">
                {{ colValue(row, '账期状态') }}
              </t-tag>
            </template>
            <template #参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">参保状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '参保状态'))" variant="light" size="small">
                {{ colValue(row, '参保状态') }}
              </t-tag>
            </template>
            <template #缴存状态="{ row }: any">
              <span v-if="isHeaderContext(row)">缴存状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '缴存状态'))" variant="light" size="small">
                {{ colValue(row, '缴存状态') }}
              </t-tag>
            </template>
            <!-- 账期 / 账单月份：合法 YYYY-MM 的单元格挂 data-hr-month-cell 便于真机断言倒序 -->
            <template #账期="{ row }: any">
              <span v-if="isHeaderContext(row)">账期</span>
              <span v-else-if="isHrMonth(colValue(row, '账期'))" :data-hr-month-cell="colValue(row, '账期')">
                {{ colValue(row, '账期') }}
              </span>
              <span v-else>{{ colValue(row, '账期') }}</span>
            </template>
            <template #月份="{ row }: any">
              <span v-if="isHeaderContext(row)">月份</span>
              <span v-else-if="isHrMonth(colValue(row, '月份'))" :data-hr-month-cell="colValue(row, '月份')">
                {{ colValue(row, '月份') }}
              </span>
              <span v-else>{{ colValue(row, '月份') }}</span>
            </template>
            <!-- 离职证明：有附件显示可点击查看链接，无则 —。
                 表头上下文必须输出纯文本列标题（TDesign 会复用该插槽渲染 th）。 -->
            <template #离职证明="{ row }: any">
              <span v-if="isHeaderContext(row)">离职证明</span>
              <t-link v-else-if="proofPathOf(row)" theme="primary" @click.stop="previewProof(row)">查看</t-link>
              <span v-else></span>
            </template>
            <!-- 第 47 轮：合同状态三色 Tag（正常绿 / 即将到期橙 / 到期红） -->
            <template #合同状态="{ row }: any">
              <span v-if="isHeaderContext(row)">合同状态</span>
              <t-tag v-else-if="contractStatusOfRow(row)" :theme="contractStatusTheme(contractStatusOfRow(row))"
                variant="light" size="small">
                {{ contractStatusOfRow(row) }}
              </t-tag>
              <span v-else></span>
            </template>
            <!-- 第 54 轮：公积金账户状态三色 Tag（正常绿 / 封存橙 / 提取销户灰） -->
            <template #账户状态="{ row }: any">
              <span v-if="isHeaderContext(row)">账户状态</span>
              <t-tag v-else-if="colValue(row, '账户状态')" :theme="fundStatusTheme(colValue(row, '账户状态'))"
                variant="light" size="small">
                {{ colValue(row, '账户状态') }}
              </t-tag>
              <span v-else></span>
            </template>
            <!-- 第 55 轮：月度账单状态 Tag（公积金 / 社保共用，已申报 = 绿） -->
            <template #宿舍状态="{ row }: any">
              <span v-if="isHeaderContext(row)">宿舍状态</span>
              <t-tag v-else-if="colValue(row, '宿舍状态')" :theme="hrDormStatusTheme(colValue(row, '宿舍状态'))"
                variant="light" size="small">
                {{ colValue(row, '宿舍状态') }}
              </t-tag>
            </template>
            <template #住宿状态="{ row }: any">
              <span v-if="isHeaderContext(row)">住宿状态</span>
              <t-tag v-else-if="colValue(row, '住宿状态')" :theme="hrDormStatusTheme(colValue(row, '住宿状态'))"
                variant="light" size="small">
                {{ colValue(row, '住宿状态') }}
              </t-tag>
            </template>
            <template #资产状态="{ row }: any">
              <span v-if="isHeaderContext(row)">资产状态</span>
              <t-tag v-else-if="colValue(row, '资产状态')" :theme="hrDormStatusTheme(colValue(row, '资产状态'))"
                variant="light" size="small">
                {{ colValue(row, '资产状态') }}
              </t-tag>
            </template>
            <template #维修状态="{ row }: any">
              <span v-if="isHeaderContext(row)">维修状态</span>
              <t-tag v-else-if="colValue(row, '维修状态')" :theme="hrDormRepairTheme(colValue(row, '维修状态'))"
                variant="light" size="small">
                {{ colValue(row, '维修状态') }}
              </t-tag>
            </template>
            <template #房源类型="{ row }: any">
              <span v-if="isHeaderContext(row)">房源类型</span>
              <t-tag v-else-if="colValue(row, '房源类型')" :theme="hrDormRoomTypeTheme(colValue(row, '房源类型'))"
                variant="light" size="small">
                {{ colValue(row, '房源类型') }}
              </t-tag>
            </template>
            <template #账单状态="{ row }: any">
              <span v-if="isHeaderContext(row)">账单状态</span>
              <t-tag v-else-if="colValue(row, '账单状态')" :theme="billStatusTheme(colValue(row, '账单状态'))"
                variant="light" size="small">
                {{ colValue(row, '账单状态') }}
              </t-tag>
              <span v-else></span>
            </template>
            <!-- 第 57 轮：回盘表五险参保状态改为**三色字体**（不是 Tag）：
                 参保缴费 #00a870 绿 / 终止缴费 #ed7b2f 橙 / 暂停缴费走 --td-text-color-primary 令牌。 -->
            <template #养老参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">养老参保状态</span>
              <span v-else :class="siStatusTextClass(colValue(row, '养老参保状态'))">{{ colValue(row, '养老参保状态') }}</span>
            </template>
            <template #失业参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">失业参保状态</span>
              <span v-else :class="siStatusTextClass(colValue(row, '失业参保状态'))">{{ colValue(row, '失业参保状态') }}</span>
            </template>
            <template #医疗参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">医疗参保状态</span>
              <span v-else :class="siStatusTextClass(colValue(row, '医疗参保状态'))">{{ colValue(row, '医疗参保状态') }}</span>
            </template>
            <template #工伤参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">工伤参保状态</span>
              <span v-else :class="siStatusTextClass(colValue(row, '工伤参保状态'))">{{ colValue(row, '工伤参保状态') }}</span>
            </template>
            <template #生育参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">生育参保状态</span>
              <span v-else :class="siStatusTextClass(colValue(row, '生育参保状态'))">{{ colValue(row, '生育参保状态') }}</span>
            </template>
            <!-- 社保原始记录：记录性质 Tag（正常 / 异常补退） -->
            <template #记录性质="{ row }: any">
              <span v-if="isHeaderContext(row)">记录性质</span>
              <t-tag v-else :theme="String(colValue(row, '记录性质')).includes('异常') ? 'warning' : 'success'"
                variant="light" size="small">
                {{ colValue(row, '记录性质') }}
              </t-tag>
            </template>
            <!-- 第 58 轮：正常 / 异常记录的 补退类型、记录类型 用红绿字体（不是 Tag） -->
            <template #补退类型="{ row }: any">
              <span v-if="isHeaderContext(row)">补退类型</span>
              <span v-else :class="siAdjustTextClass(colValue(row, '补退类型'))">{{ colValue(row, '补退类型') }}</span>
            </template>
            <template #记录类型="{ row }: any">
              <span v-if="isHeaderContext(row)">记录类型</span>
              <span v-else :class="siAdjustTextClass(colValue(row, '记录类型'))">{{ colValue(row, '记录类型') }}</span>
            </template>
            <template #empty>
              <BusinessEmptyState v-if="!tableLoading" compact :icon="emptyIcon"
                :description="emptyText" :hint="emptyHint" />
              <div v-else class="hr-empty-placeholder" />
            </template>
          </t-table>
        </div>
      </div>
    </div>

    <!-- 详情抽屉：深拷贝草稿 + 脏检查只标脏不自动保存，必须点「保存」才调 API -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="detailRow"
      :page-no="0" :form="editForm" :fields="drawerFields"
      :groups="drawerGroups" :sub-fields="[]"
      :supports-sub-fields="false" :dirty="detailDirty" :saving="saving"
      storage-key="weknora-hr-detail-width" :show-preview="false"
      @save="saveDetail" @delete="onDetailDelete">
      <!-- 第 48 轮：部门 / 职务变更后重算工号建议值（用户手改过则不覆盖） -->
      <template #field-部门>
        <!-- 第 55 轮：部门在花名册是原生字段，在离职/合同/公积金里是跨表引用 → 只读 -->
        <t-select v-model="editForm.data['部门']" clearable filterable placeholder="选择部门"
          :disabled="activeTab !== 'hr-roster'" @change="onDeptOrTitleChange">
          <t-option v-for="d in rosterDeptOptions" :key="d" :label="d" :value="d" />
        </t-select>
      </template>
      <template #field-职务>
        <t-input v-model="editForm.data['职务']" placeholder="职务" @change="onDeptOrTitleChange" />
      </template>
      <!-- 第 62 轮：宿舍管理抽屉的联动控件（房型→床位、配套多选、员工/房源带出费用项） -->
      <template #field-房型>
        <t-select v-model="editForm.data['房型']" :options="ROOM_TYPE_OPTIONS" placeholder="选择房型"
          clearable data-hr-dorm-room-type @change="onDormRoomTypeChange" />
      </template>
      <template #field-配套设施>
        <t-select v-model="editForm.data['配套设施']" :options="dormFacilityOptions.map((v) => ({ label: v, value: v }))"
          multiple filterable placeholder="从资产档案选择设施" data-hr-dorm-facility />
      </template>
      <template #field-员工姓名>
        <t-select v-model="editForm.data['员工姓名']" :options="dormEmployeeOptions" filterable
          placeholder="搜索选择员工" data-hr-dorm-employee @change="onStayEmployeeChange" />
      </template>
      <template #field-床位号>
        <t-select v-model="editForm.data['床位号']" :options="DORM_BED_OPTIONS" placeholder="选择床位"
          data-hr-dorm-bed />
      </template>
      <!-- 第 57 轮：离职证明附件控制条。
           严禁把 /api/v1/fleet/... 这类后端内部路径暴露在输入框里。
           第 58 轮：抽屉内**删除「查看」**（预览只在列表「离职证明」列的查看入口）。
           第 60 轮：已上传时改用 t-popconfirm 气泡确认（图 1 的模态框已删除）。
           气泡的 @confirm 是**同步**回调，内部直接 click 隐藏 input，浏览器保留
           user activation；包进 setTimeout / await 会让文件选择框被静默拦截。 -->
      <template #field-离职证明>
        <div class="hr-proof-field">
          <t-tag v-if="proofPathOf(editForm.data)" theme="success" variant="light" size="small"
            class="hr-proof-tag">
            <template #icon><t-icon name="file" size="14px" /></template>
            已上传离职证明
          </t-tag>
          <span v-else class="hr-proof-empty">未上传</span>
          <t-popconfirm v-if="proofPathOf(editForm.data)" theme="warning" placement="left"
            content="该员工已上传离职证明，重新上传后原来的图片将会被覆盖，是否继续？"
            :confirm-btn="{ content: '确定', theme: 'primary' }" :cancel-btn="{ content: '取消' }"
            data-hr-proof-pop @confirm="pickProof">
            <t-button variant="outline" size="small" class="hr-proof-btn" :loading="proofUploading"
              data-hr-proof-upload @click.stop>
              <template #icon><t-icon name="upload" size="14px" /></template>
              上传
            </t-button>
          </t-popconfirm>
          <t-button v-else variant="outline" size="small" class="hr-proof-btn" :loading="proofUploading"
            data-hr-proof-upload @click.stop="pickProof">
            <template #icon><t-icon name="upload" size="14px" /></template>
            上传
          </t-button>
          <input ref="proofInputRef" type="file" hidden accept=".pdf,.jpg,.jpeg,.png"
            @change="onProofFileChange" />
        </div>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 第 52 轮：100% 复用中台 StandardSettingDrawer + FleetCertPanel。
         花名册取消胶囊分组（字段仍按 6 组顺序连续展示，group 供编辑抽屉分块）；
         离职管理 / 劳动合同复用 FleetCertPanel 既有的跨表字段搜索引用机制。 -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="''" :scope="hrScopeOf(activeTab)"
      :initial-category="initialCategory" :title="settingsTitle"
      :initial-group="activeTab === 'hr-dorm' ? HR_DORM_GROUP_OF[dormSubView] : ''"
      fields-tab-label="字段设置"
      :hide-group-tabs="false" :single-category="true"
      :ref-sources="refSources"
      storage-key="weknora-hr-settings-drawer-width" :show-rules="false"
      @saved="onSettingsSaved" />

    <!-- 打印预览（统一中台组件） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="printTitle" :url="printUrl" :data="printData"
      :download-name="`${printTitle}.pdf`" @close="closePrint" />
    <!-- 第 55 轮：月度账单详情抽屉的独立打印预览 -->
    <PrintPreviewDialog v-model:visible="billPrintVisible" :title="billPrintTitle" :url="billPrintUrl"
      :data="billPrintData" :download-name="`${billPrintTitle}.pdf`" @close="closeBillPrint" />

    <!-- 第 47 轮：社保多文件自适应导入（正常 + 异常一起选） -->
    <input ref="siFileInputRef" type="file" multiple accept=".xlsx,.xls,.csv" class="hr-file-input"
      @change="onSiFilesChange" />

    <!-- 第 52 轮：三套独立 Excel 导入模板（花名册 / 离职管理 / 劳动合同）共用弹窗 -->
    <RosterImportDialog v-model:visible="rosterImportVisible" :module="importModule" @imported="onRosterImported" />
    <!-- 第 58 轮：社保三类导入统一复用中台 StandardImportDialog（回盘表固定覆盖更新） -->
    <StandardImportDialog :visible="socialImportVisible" :title="`导入${socialImportLabel}`"
      :module-label="socialImportLabel" :show-template="socialImportKind === 'receipt'"
      template-hint="下载社保回盘表标准模板" :multiple="socialImportKind !== 'receipt'"
      :modes="[{ label: '覆盖更新', value: 'overwrite' }]" default-mode="overwrite"
      :on-download="downloadSocialTemplate" :on-import="runSocialImport"
      @update:visible="(v: boolean) => socialImportVisible = v" @imported="onSocialImported" />
    <!-- 第 61 轮：宿舍档案四类导入（中文表头模板 + 拖拽上传） -->
    <StandardImportDialog :visible="dormImportVisible"
      :title="`导入${HR_DORM_GROUP_OF[dormSubView]}`"
      :module-label="HR_DORM_GROUP_OF[dormSubView]" show-template
      :template-hint="`下载${HR_DORM_GROUP_OF[dormSubView]}模板`" :multiple="false"
      :modes="DORM_IMPORT_MODES" default-mode="merge"
      :on-download="downloadDormTemplate" :on-import="runDormImport"
      @update:visible="(v: boolean) => dormImportVisible = v" @imported="onDormImported" />
    <ResignEmployeeDialog v-model:visible="resignVisible" :ids="selectedRowKeys"
      :employee-name="resignTargetName" @done="onResignDone" />

    <!-- 第 55 轮：页内附件预览器（严禁 window.open）
         第 58 轮：关闭即 revokeObjectURL，blob 不常驻到页面卸载。 -->
    <FilePreviewDialog :visible="previewVisible" :src="previewSrc"
      :file-name="previewName" :file-type="previewType" title="离职证明预览"
      @update:visible="onPreviewVisibleChange" />

    <!-- 第 55 轮：生成账单月份选择 + 重复月份二次确认 -->
    <t-dialog :visible="billMonthVisible" header="生成月度账单" width="420px" placement="center"
      @close="billMonthVisible = false" @confirm="confirmGenerateBill">
      <div class="hr-bill-month-row">
        <span>账单月份</span>
        <t-date-picker v-model="billMonth" mode="month" format="YYYY-MM" value-type="YYYY-MM"
          placeholder="选择月份" clearable allow-input />
      </div>
      <template #footer>
        <t-button variant="outline" size="small" @click="billMonthVisible = false">取消</t-button>
        <t-button theme="primary" size="small" :disabled="!billMonth" @click="confirmGenerateBill">确定</t-button>
      </template>
    </t-dialog>
    <t-dialog :visible="billOverwriteVisible" header="重复账单确认" width="460px" placement="center"
      @close="billOverwriteVisible = false">
      <div class="hr-bill-confirm">{{ billOverwriteText }}</div>
      <template #footer>
        <t-button variant="outline" size="small" @click="billOverwriteVisible = false">取消</t-button>
        <t-button theme="primary" size="small" @click="doGenerateBill(true)">确定覆盖</t-button>
      </template>
    </t-dialog>

    <!-- 第 57 轮：月度账单由抽屉改为**大尺寸居中弹窗**（社保 / 公积金统一设计），
         顶部三标签「个人明细 | 单位明细 | 汇总数据」，标签下方标准工具栏
         （左侧搜索/部门/条件险种，右侧打印 + 导出 Excel），表格合计行恒贴底。 -->
    <t-dialog v-model:visible="billDetailVisible" :header="billDetailTitle" width="min(1200px, 94vw)"
      placement="center" destroy-on-close class="hr-bill-dialog" data-hr-bill-dialog>
      <div class="hr-bill-detail">
        <div class="hr-bill-tabs">
          <t-radio-group v-model="billDetailTab" size="small" @change="onBillDetailTabChange">
            <t-radio-button value="personal">个人明细</t-radio-button>
            <t-radio-button value="unit">单位明细</t-radio-button>
            <t-radio-button value="summary">汇总数据</t-radio-button>
          </t-radio-group>
        </div>
        <div class="hr-bill-filters">
          <!-- 个人 / 单位明细：搜索 + 部门 + 险种（无细分险种列的宽表自动隐藏险种筛选） -->
          <template v-if="billDetailTab !== 'summary'">
            <t-input v-model="billKeyword" placeholder="搜索姓名 / 部门"
              clearable class="hr-bill-search" data-hr-bill-search>
              <template #prefixIcon><t-icon name="search" size="16px" /></template>
            </t-input>
            <t-select v-model="billDept" :options="billDeptOptions" placeholder="全部部门"
              :clearable="false" class="hr-bill-select" data-hr-bill-dept />
            <t-select v-if="billDetailKind === 'social'" v-model="billScheme"
              :options="SI_BILL_SCHEME_OPTIONS" placeholder="全部险种" :clearable="false"
              class="hr-bill-select" data-hr-bill-scheme />
            <span class="hr-bill-count">共 {{ billDetailKind === 'social' ? siBillPeople : billFiltered.length }} 人
              · {{ billSubjectRows.length }} 条</span>
          </template>
          <!-- 汇总数据：只需搜索部门 -->
          <template v-else>
            <t-input v-model="billDeptKeyword" placeholder="搜索部门" clearable
              class="hr-bill-search" data-hr-bill-dept-search>
              <template #prefixIcon><t-icon name="search" size="16px" /></template>
            </t-input>
            <span class="hr-bill-count">共 {{ billSummaryRows.length }} 个部门</span>
          </template>
          <span class="hr-bill-spacer" />
        </div>
        <!-- 第 61 轮：三段分离式表格（表头 / 滚动数据区 / 合计行各自独立容器）。
             垂直滚动条只出现在中间数据区，绝不贯穿表头与合计行（图 1 的问题）。 -->
        <div class="hr-bill-table-wrap">
          <DetailScrollTable v-if="billDetailTab === 'summary'" name="bill-summary"
            :columns="billSummaryColumns" :rows="billSummaryRows" :foot="billSummaryFoot"
            row-key="id" data-hr-bill-summary-table />
          <DetailScrollTable v-else name="bill-subject" :columns="billSubjectColumns"
            :rows="billSubjectRows" :foot="billSubjectFoot" row-key="id"
            data-hr-bill-subject-table />
        </div>
      </div>
      <!-- 右下角操作栏：导出 Excel → 打印 → 关闭（与能耗打印预览同款） -->
      <template #footer>
        <div class="hr-bill-footer">
          <t-button theme="default" variant="outline" size="small" :loading="exporting"
            data-hr-bill-export @click="exportBillExcel">
            <template #icon><t-icon name="download" size="14px" /></template>
            导出 Excel
          </t-button>
          <t-button theme="default" variant="outline" size="small" @click="printBillDetail">
            <template #icon><t-icon name="print" size="14px" /></template>
            打印
          </t-button>
          <t-button theme="primary" size="small" data-hr-bill-close @click="billDetailVisible = false">
            关闭
          </t-button>
        </div>
      </template>
    </t-dialog>

    <!-- 第 61 轮：宿舍费用分摊明细（个人 / 房间 / 部门三标签，三段分离表格） -->
    <t-dialog v-model:visible="dormBillVisible" :header="dormBillTitle" width="min(1360px, 96vw)"
      placement="center" destroy-on-close data-hr-dorm-bill-dialog>
      <div class="hr-bill-detail">
        <div class="hr-bill-tabs">
          <t-radio-group v-model="dormBillTab" size="small" data-hr-dorm-bill-tabs>
            <t-radio-button value="personal">全员扣款明细</t-radio-button>
            <t-radio-button value="dorm">员工宿舍水电</t-radio-button>
            <t-radio-button value="public">公租房水电气房租</t-radio-button>
            <t-radio-button value="dept">按部门汇总</t-radio-button>
          </t-radio-group>
          <span class="hr-bill-spacer" />
          <span class="hr-bill-count">聚合花名册 / 住宿 / 能耗 / 社保 / 公积金 · 按入住天数分摊</span>
        </div>
        <div class="hr-bill-table-wrap">
          <DetailScrollTable :name="`dorm-${dormBillTab}`" :columns="dormActiveBill.cols"
            :rows="dormActiveBill.rows" :foot="dormActiveBill.foot" row-key="id" />
        </div>
      </div>
      <template #footer>
        <div class="hr-bill-footer">
          <t-button theme="default" variant="outline" size="small" :loading="exporting"
            data-hr-dorm-bill-export @click="exportDormBill">
            <template #icon><t-icon name="download" size="14px" /></template>
            导出 Excel
          </t-button>
          <t-button theme="default" variant="outline" size="small" @click="printDormBill">
            <template #icon><t-icon name="print" size="14px" /></template>
            打印
          </t-button>
          <t-button theme="primary" size="small" data-hr-dorm-bill-close @click="dormBillVisible = false">
            关闭
          </t-button>
        </div>
      </template>
    </t-dialog>

    <!-- 办理调宿 -->
    <t-dialog :visible="dormTransferVisible" header="办理调宿" width="460px" placement="center"
      data-hr-dorm-transfer-dialog @close="dormTransferVisible = false">
      <div class="hr-bill-month-row"><span>目标房源</span>
        <t-select v-model="dormTransferRoom" :options="dormRoomSelectOptions" filterable
          placeholder="搜索并选择房源" class="hr-bill-grow" />
      </div>
      <div class="hr-bill-month-row"><span>床位号</span>
        <t-select v-model="dormTransferBed" :options="dormTransferBedOptions" placeholder="选择床位" clearable
          class="hr-bill-grow" />
      </div>
      <template #footer>
        <t-button theme="default" variant="outline" size="small" @click="dormTransferVisible = false">取消</t-button>
        <t-button theme="primary" size="small" data-hr-dorm-transfer-confirm @click="confirmDormTransfer">确定</t-button>
      </template>
    </t-dialog>

    <!-- 办理退宿 -->
    <t-dialog :visible="dormCheckoutVisible" header="办理退宿" width="460px" placement="center"
      data-hr-dorm-checkout-dialog @close="dormCheckoutVisible = false">
      <div class="hr-bill-month-row"><span>退宿日期</span>
        <t-date-picker v-model="dormCheckoutDate" mode="date" format="YYYY-MM-DD" value-type="YYYY-MM-DD"
          placeholder="选择日期" allow-input />
      </div>
      <div class="hr-bill-month-row"><span>退宿原因</span>
        <t-select v-model="dormCheckoutReason" :options="CHECKOUT_REASONS" placeholder="选择原因"
          class="hr-bill-grow" />
      </div>
      <template #footer>
        <t-button theme="default" variant="outline" size="small" @click="dormCheckoutVisible = false">取消</t-button>
        <t-button theme="primary" size="small" data-hr-dorm-checkout-confirm @click="confirmDormCheckout">确定</t-button>
      </template>
    </t-dialog>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessEmptyState from './BusinessEmptyState.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import FilePreviewDialog from '@/components/business/FilePreviewDialog.vue'
import DetailScrollTable from '@/components/business/DetailScrollTable.vue'
import {
  HR_DORM_VIEWS, HR_DORM_FIELDS, HR_DORM_GROUP_OF, hrDormColumns, hrDormModuleOf,
  hrDormStatusTheme, hrDormRoomTypeTheme, hrDormRepairTheme, hrDormDrawerGroups, hrDormBedsOfType,
  HR_DORM_INTEGER_FIELDS, hrDormChargeColumns,
} from './hrDorm'
import type { HrDormView } from './hrDorm'
import StandardImportDialog from '@/components/business/StandardImportDialog.vue'
import { listFleetCategories } from '@/api/fleet'
import { useTableFlatState, flatClassOf } from './useTableFlatState'
import { useTableHeaderFix } from './useTableHeaderFix'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import { buildFootRow, footSpanOf, summaryScopeRows, type SummaryColumn } from './listSummary'
import './styles/list-summary-footer.css'
import { generateCatalogPdf } from './useCatalogPdf'
import { grouped } from './formatNumber'
import {
  HR_TABS, buildHrKpiCards, hrScopeOf, hrColumnDefs, hrBillColumnDefs,
  sortMonthsDesc, isHrMonth, hrFieldSeeds, hrRowKey,
  hrRosterGroupColumns, hrResignColumns, hrContractColumns, hrColumnsFromSubs,
  hrSocialColumnsOf, hrFundColumns, contractKindOf,
  contractStatusOf,
  contractStatusTheme,
} from './hrCore'
import type { HrTab, HrColumn } from './hrCore'
import { groupLabelOf, type RosterStatusFilter } from './hrRosterGroups'
import {
  hrModuleGroups, hrModuleFields, hrEnumsOf, hrDataTypeOf, hrParseIdCard,
  HR_FUND_BILL_GROUP, HR_FUND_DETAIL_GROUP, HR_FUND_BILL_FIELDS, HR_FUND_FIELDS,
  HR_ROSTER_GROUPS, HR_RESIGN_FIELDS,
  HR_SOCI_RECEIPT_FIELDS, HR_SOCI_NORMAL_FIELDS, HR_SOCI_ABNORMAL_FIELDS, HR_SOCI_BILL_FIELDS,
  HR_SOCI_DEFAULT_HEADERS, hrSocialGroupOf,
  hrSeniorityOf, hrRegularDateOf, hrCleanForm, hrValidateForm, hrCleanValue,
  HR_DERIVED_FIELDS,
  type HrModule,
} from './hrFieldSchema'
import RosterImportDialog from './RosterImportDialog.vue'
import ResignEmployeeDialog from './ResignEmployeeDialog.vue'
import * as hrApi from './hrApi'
// 第 58 轮：账单人数 / 行序号口径抽成纯函数（可单测，打印导出处共用同一套）
import { billPeopleCountOf, isAdjustmentRow, siBillPeopleOf } from './hrBill'
import type { FilterFieldSpec } from './businessFilter'

// ---------------------------------------------------------------------------
// 基础状态
// ---------------------------------------------------------------------------
const activeTab = ref<HrTab>('hr-roster')
// 第 55 轮：默认激活「月度账单」
const fundSubView = ref<'ledger' | 'bills'>('bills')
/** 第 56 轮：社保四标签，默认激活「月度账单」；与设置抽屉四胶囊严格 1:1。 */
type SocialSubView = 'receipt' | 'normal' | 'abnormal' | 'bills'
const socialSubView = ref<SocialSubView>('bills')
/** 表头门禁：false 时不渲染 t-table，保证第 1 帧列数一次成型。 */
const headerReady = ref(false)
const loading = ref(false)
const rows = ref<any[]>([])
const selectedKeys = ref<Set<string>>(new Set())
const selectedRowKeys = computed(() => Array.from(selectedKeys.value))
const filterState = ref<Record<string, any>>({ keyword: '' })
const kpiCounts = ref<Record<string, number>>({})
const fundSettings = ref<any>(null)
const fieldConfig = ref<Record<string, any[]>>({})
/**
 * 第 52 轮：花名册列表固定只查询在职人员，状态筛选框已从工具栏移除。
 * 保留该状态仅为兼容既有列显隐/抽屉派生函数，恒为「在职」且不再对外暴露修改入口。
 */
const rosterStatusView = ref<RosterStatusFilter>('在职')
/** 第 58 轮：劳动合同 / 劳务合同双标签（按合同类型关键字分流）。 */
const contractSubView = ref<'labor' | 'service'>('labor')

/**
 * 人事 5 个列表的分类名 + scope（第 53 轮：互相引用时的候选目录）。
 * 与后端 hrCategorySeeds 的分类名严格一致，否则 SourceFieldPicker 取不到 subs。
 */
const HR_REF_CATALOG: readonly { scope: string; category: string }[] = [
  { scope: 'hr-roster', category: '花名册' },
  { scope: 'hr-resign', category: '离职管理' },
  { scope: 'hr-contract', category: '劳动合同' },
  { scope: 'hr-social', category: '社保管理' },
  { scope: 'hr-fund', category: '公积金管理' },
]
/** 当前 Tab 的候选引用源：除自己之外的人事分类（避免把本表明字段再引一遍）。 */
const refSources = computed(() =>
  HR_REF_CATALOG.filter((s) => s.scope !== hrScopeOf(activeTab.value)),
)

/** 当前导入弹窗对应的模板模块（社保走多文件自适应，不走本弹窗）。 */
const importModule = computed<HrModule>(() =>
  activeTab.value === 'hr-resign' ? 'resign'
    : activeTab.value === 'hr-contract' ? 'contract'
      : activeTab.value === 'hr-fund' ? 'fund' : 'roster')

/** 组织机构单位名（合同公司 / 签约公司下拉数据源）。 */
const rosterUnitOptions = ref<string[]>([])
const contractStatusView = ref<string>('正常')
/** 第 59 轮：合同状态筛选项（劳动合同默认「正常」，劳务合同默认「全部状态」）。 */
const CONTRACT_STATUS_OPTIONS = [
  { label: '全部状态', value: '全部状态' }, { label: '正常', value: '正常' },
  { label: '未签订', value: '未签订' }, { label: '即将到期', value: '即将到期' },
  { label: '到期', value: '到期' }, { label: '已解除', value: '已解除' },
]
/** 第 54 轮：公积金账户状态筛选（全部 / 正常 / 封存 / 提取销户）。 */
const fundStatusView = ref<string>('正常')
/**
 * 第 59 轮：封存 / 启封互斥。
 * 选中记录全是「正常」→ 只给封存；全是「封存」→ 只给启封；混合或其它状态都不给。
 */
const fundSealMode = computed<'seal' | 'unseal' | ''>(() => {
  if (activeTab.value !== 'hr-fund' || fundSubView.value !== 'ledger') return ''
  const picked = rows.value.filter((r) => selectedRowKeys.value.includes(String(r?.id ?? '')))
  if (!picked.length) return ''
  const statuses = new Set(picked.map((r) => String(r['账户状态'] ?? r.status ?? '').trim()))
  if (statuses.size !== 1) return ''
  const st = [...statuses][0]
  if (st === '正常' || st === 'active') return 'seal'
  if (st === '封存' || st === 'sealed') return 'unseal'
  return ''
})
const FUND_STATUS_OPTIONS = [
  { label: '全部状态', value: '全部状态' },
  { label: '正常', value: '正常' },
  { label: '封存', value: '封存' },
  { label: '提取销户', value: '提取销户' },
]
/** 第 48 轮：花名册部门筛选（空 = 全部部门），在职/离职共用。 */
const rosterDeptFilter = ref<string>('')
const rosterDeptOptions = ref<string[]>([])
/** 第 56 轮：默认自动选中最近一个月（真实数据账期 2026-11）。 */
function currentMonth(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}
const siMonthFilter = ref<string>(currentMonth())
const siDeptFilter = ref<string>('')
const siSchemeFilter = ref<string>('')
/** 第 57 轮：回盘表参保状态筛选（默认「参保缴费」）。 */
const SI_PENSION_STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '参保缴费', value: '参保缴费' },
  { label: '暂停缴费', value: '暂停缴费' },
  { label: '终止缴费', value: '终止缴费' },
]
const siPensionStatusFilter = ref<string>('参保缴费')
/** 月度账单标签：左侧工具栏只留月份选择器。 */
const siBillMonthFilter = ref<string>('')
const fundBillMonthFilter = ref<string>('')
const siDeptOptions = computed(() => [
  { label: '全部部门', value: '' },
  ...Array.from(new Set(rows.value.map((r) => String(r?.['部门'] || '').trim()).filter(Boolean)))
    .sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'))
    .map((d) => ({ label: d, value: d })),
])
/** 保险类型筛选：与字段契约「险种类型」枚举完全一致。 */
const SI_INSURANCE_OPTIONS = [
  { label: '全部险种', value: '' },
  { label: '养老保险', value: '养老保险' },
  { label: '医疗保险', value: '医疗保险' },
  { label: '生育保险', value: '生育保险' },
  { label: '大额医疗', value: '大额医疗' },
  { label: '工伤保险', value: '工伤保险' },
  { label: '失业保险', value: '失业保险' },
]
/** 合同状态：优先取后端派生的 custom_data.合同状态，否则前端现算。 */
function contractStatusOfRow(row: any): string {
  const v = row?.合同状态 ?? row?.custom_data?.['合同状态'] ?? row?.contract_status
  if (v) return String(v)
  const pickField = (...keys: string[]) => {
    for (const k of keys) {
      const val = row?.[k] ?? row?.custom_data?.[k] ?? row?.data?.[k]
      if (val !== undefined && val !== null && val !== '') return val
    }
    return ''
  }
  // 新字段名优先，兼容存量「现合同…」键
  return contractStatusOf(
    pickField('合同结束日期', '现合同结束日期'),
    pickField('合同类型', '现合同类型'),
  )
}

const rosterImportVisible = ref(false)
const resignVisible = ref(false)
/** 单选办理离职时传入弹窗，用于顶部「正在为「XXX」办理离职」提示。 */
const resignTargetName = computed(() => {
  if (selectedRowKeys.value.length !== 1) return ''
  const row = rows.value.find((r) => String(r.id) === String(selectedRowKeys.value[0]))
  return String(row?.姓名 || row?.name || row?.职工姓名 || '')
})

// ---- 第 55 轮：页内附件预览器（替代 window.open） ----
const previewVisible = ref(false)
const previewSrc = ref('')
const previewName = ref('')
const previewType = ref('')
let previewUrl = ''

// ---- 第 55 轮：月度账单生成 / 详情抽屉 ----
const billPrintVisible = ref(false)
const billPrintTitle = ref('公积金缴存明细')
const billPrintUrl = ref('')
const billPrintData = ref<Uint8Array | null>(null)
const billMonthVisible = ref(false)
const billMonth = ref<string>('')
const billOverwriteVisible = ref(false)
const billOverwriteText = ref('')
const billDetailVisible = ref(false)
const billDetailTitle = ref('')
const billRows = ref<any[]>([])
const billKeyword = ref('')
const billDept = ref('')
const billScheme = ref('全部')
/** 第 57 轮：账单弹窗三标签（个人明细 | 单位明细 | 汇总数据）。
     社保默认「个人明细」，公积金默认「汇总数据」（与需求一致）。 */
const billDetailKind = ref<'fund' | 'social'>('fund')
const billDetailTab = ref<'personal' | 'unit' | 'summary'>('summary')
const billDeptKeyword = ref('')
const SI_BILL_SCHEME_OPTIONS = [
  { label: '全部险种', value: '全部' },
  { label: '养老', value: '养老' }, { label: '医疗+生育', value: '医疗+生育' },
  { label: '大额医疗', value: '大额医疗' }, { label: '工伤', value: '工伤' },
  { label: '失业', value: '失业' },
]
/** 险种筛选 → 该险种有金额即保留（按分判断，空值不显示横线）。 */
const SI_BILL_SCHEME_KEYS: Record<string, string[]> = {
  养老: ['养老个人', '养老单位'],
  '医疗+生育': ['医疗个人', '医疗+生育单位'],
  大额医疗: ['大额医疗'],
  工伤: ['工伤单位'],
  失业: ['失业个人', '失业单位'],
}
const billDeptOptions = computed(() => [
  { label: '全部部门', value: '' },
  ...Array.from(new Set(billRows.value.map((r) => String(r['部门'] || r.department || '').trim()).filter(Boolean)))
    .map((d) => ({ label: d, value: d })),
])

const tabLabel = computed(() => HR_TABS.find((t) => t.key === activeTab.value)?.label || '人事管理')
const settingsTitle = computed(() => `${tabLabel.value}设置`)
const initialCategory = computed(() => tabLabel.value)
// 第 47 轮：抽屉标题用「员工详情 / 合同详情」，不再拼「花名册详情」，
// 且 BusinessSplitDetailDrawer 已不再渲染 [P1] 页码徽标（人事模块无 PDF 分页）。
const detailTitle = computed(() => {
  const name = String(detailRow.value?.姓名 || detailRow.value?.name || '').trim()
  // 第 58 轮：社保正常 / 异常记录抽屉有独立标题（图 6 要求「社保记录详情 · XXX」）
  const socialRecord = activeTab.value === 'hr-social'
    && (socialSubView.value === 'normal' || socialSubView.value === 'abnormal')
  // 第 62 轮：宿舍五标签的抽屉标题必须精准匹配（严禁「新增员工 / 员工详情」）
  if (activeTab.value === 'hr-dorm') {
    const add = !detailRow.value
    const d = editForm.value?.data || {}
    const key = String(d['宿舍编号'] || d['房屋编号'] || d['员工姓名'] || d['资产名称'] || d['姓名'] || '').trim()
    const base = dormSubView.value === 'dorm' ? (add ? '新增宿舍' : '宿舍详情')
      : dormSubView.value === 'public' ? (add ? '新增公租房' : '公租房详情')
        : dormSubView.value === 'stay' ? (add ? '安排入住' : '住宿详情')
          : dormSubView.value === 'asset' ? (add ? '新增资产' : '资产详情')
            : (add ? '新增费用记录' : '费用详情')
    return key ? `${base} · ${key}` : base
  }
  const base = !detailRow.value ? '新增员工'
    : socialRecord ? '社保记录详情'
      : activeTab.value === 'hr-contract' ? '合同详情'
        : rosterStatusView.value === '离职' ? '离职详情' : '员工详情'
  return name ? `${base} · ${name}` : base
})

/**
 * 第 57 轮：打印预览标题必须与当前子视图精准匹配（严禁笼统叫「社保管理清单」）。
 * 同时作为 PDF 页眉大标题与下载文件名。
 */
const printTitle = computed(() => {
  // 第 59 轮：统一清单标题口径（弹窗标题 / PDF 页眉 / 页脚 / 文件名同源）
  if (activeTab.value === 'hr-roster') {
    return rosterStatusView.value === '离职' ? '花名册离职人员清单' : '花名册在职人员清单'
  }
  if (activeTab.value === 'hr-contract') {
    return contractSubView.value === 'service' ? '员工劳务合同清单' : '员工劳动合同清单'
  }
  if (activeTab.value === 'hr-social') {
    return {
      receipt: '社保缴费回盘表清单',
      normal: '社保缴费正常记录清单',
      abnormal: '社保缴费补退记录清单',
      bills: '社保缴费月度账单清单',
    }[socialSubView.value] || '社保缴费清单'
  }
  if (activeTab.value === 'hr-fund') {
    return fundSubView.value === 'bills' ? '公积金缴存月度账单清单' : '公积金缴存职工明细清单'
  }
  return '人事管理清单'
})

/** 第 57 轮：导入 / 生成账单严格按标签显隐（账单标签只生成不导入，其余只导入不生成）。 */
const toolbarShowImport = computed(() => {
  if (activeTab.value === 'hr-social') return socialSubView.value !== 'bills'
  if (activeTab.value === 'hr-fund') return fundSubView.value !== 'bills'
  if (activeTab.value === 'hr-dorm') return dormSubView.value !== 'fee'
  return true
})
const toolbarShowGenerateBill = computed(() => {
  if (activeTab.value === 'hr-social') return socialSubView.value === 'bills'
  if (activeTab.value === 'hr-fund') return fundSubView.value === 'bills'
  if (activeTab.value === 'hr-dorm') return dormSubView.value === 'fee'
  return false
})
/**
 * 第 61 轮（唯一实心绿底铁律）：没有「+ 新增」时，导入就是本页主按钮。
 * 花名册在职、宿舍四个档案页有新增 → 导入保持白底描边；其余标签导入为实心绿底。
 */
const toolbarImportPrimary = computed(() => {
  if (activeTab.value === 'hr-dorm') return false
  if (activeTab.value === 'hr-roster') return rosterStatusView.value !== '在职'
  return true
})
/** 第 57 轮：月度账单标签把搜索框换成月份选择器（其余标签保留搜索框）。 */
const toolbarHideSearch = computed(() =>
  (activeTab.value === 'hr-social' && socialSubView.value === 'bills')
  || (activeTab.value === 'hr-fund' && fundSubView.value === 'bills')
  || (activeTab.value === 'hr-dorm' && dormSubView.value === 'fee'))

/** 恢复在职二次确认文案（单选带姓名，多选带人数）。 */
const restoreConfirmText = computed(() => {
  const names = rows.value
    .filter((r) => selectedKeys.value.has(hrRowKey(r)))
    .map((r) => String(r?.姓名 || r?.name || ''))
    .filter(Boolean)
  if (names.length === 1) return `确定将员工「${names[0]}」恢复为在职状态吗？`
  return `确定将所选 ${names.length} 名员工恢复为在职状态吗？`
})

/**
 * 工具栏「新增」按钮（第 57 轮）。
 *
 * 只有花名册的在职管理需要新增员工：离职管理是转移来的，社保四标签来自
 * 真实文件导入 / 账单生成，合同随员工 1:1 生成，公积金来自网厅导入。
 */
const primaryAction = computed(() => {
  if (activeTab.value === 'hr-roster' && rosterStatusView.value === '在职') {
    return { text: '新增', icon: 'add' }
  }
  // 第 61 轮：宿舍四个档案页都有「+ 新增」；费用信息没有（只生成账单）。
  if (activeTab.value === 'hr-dorm' && dormSubView.value !== 'fee') {
    return { text: '新增', icon: 'add' }
  }
  return undefined
})

const kpiCards = computed(() => buildHrKpiCards(kpiCounts.value))

// ---------------------------------------------------------------------------
// 列定义 / 列显隐
// ---------------------------------------------------------------------------
/** 当前 Tab 的字段配置 subs（六同步唯一权威源）。 */
function subsOfTab(tab: HrTab): any[] {
  const cats = fieldConfig.value[tab]
  const cat = Array.isArray(cats)
    ? (cats.find((c: any) => String(c?.builtin_key ?? '') === hrScopeOf(tab)) || cats[0])
    : null
  const subs = Array.isArray(cat?.subs) ? cat.subs : []
  // 第 57 轮：花名册分类含「在职管理 48 + 离职管理 14」两个胶囊，
  // 当前子视图必须各取自己的 subs，否则在职表头会混进 14 个离职字段。
  if (tab === 'hr-roster') {
    const group = rosterStatusView.value === '离职' ? '离职人员' : '在职人员'
    const scoped = subs.filter((x: any) => String(x?.group || '') === group)
    if (scoped.length) return scoped
    // 兜底（后端 subs 未带 group 或加载失败）：按字段契约名单过滤
    const names = new Set(
      (rosterStatusView.value === '离职' ? HR_RESIGN_FIELDS : hrModuleFields('roster')).map((d) => d.name),
    )
    return subs.filter((x: any) => names.has(String(x?.name || '').trim()))
  }
  // 第 55 轮：公积金设置是「职工明细 / 月度账单」双胶囊；两个视图必须各取自己的 subs，
  // 否则职工明细表头会混进 8 个账单列（六同步失效）。
  if (tab === 'hr-fund') {
    const group = fundSubView.value === 'bills' ? HR_FUND_BILL_GROUP : HR_FUND_DETAIL_GROUP
    const scoped = subs.filter((x: any) => String(x?.group || '') === group)
    return scoped.length ? scoped : subs.filter((x: any) => !x?.group)
  }
  // 第 56 轮：社保四胶囊同样必须各取自己的 subs，否则回盘表的 28 个字段
  // 会混进正常记录 / 月度账单表头（六同步失效）。
  if (tab === 'hr-social') {
    const group = hrSocialGroupOf(socialSubView.value)
    const scoped = subs.filter((x: any) => String(x?.group || '') === group)
    return scoped.length ? scoped : subs.filter((x: any) => !x?.group)
  }
  return subs
}

/**
 * 第 52 轮：花名册 / 离职管理 / 劳动合同三个模块的列全集全部来自
 * 「设置 → 字段设置」的 subs；用户在设置里新增的跨模块引用字段会立刻
 * 出现在列表表头，实现六同步。subs 未就绪时回退字段契约派生列。
 */
// ---------------------------------------------------------------------------
// 第 61 轮：宿舍管理五标签（宿舍档案 / 公租房档案 / 住宿记录 / 资产档案 / 费用信息）
// ---------------------------------------------------------------------------
const dormSubView = ref<HrDormView>('dorm')
const dormMonthFilter = ref('')
const dormRoomTypeFilter = ref('')
const DORM_ROOM_TYPE_OPTIONS = [
  { label: '全部房源', value: '' },
  { label: '公司宿舍', value: '公司宿舍' },
  { label: '公租房', value: '公租房' },
]
const DORM_BED_OPTIONS = ['1号床', '2号床', '3号床', '4号床', '5号床', '6号床', '7号床', '8号床']
  .map((v) => ({ label: v, value: v }))
const dormColumns = computed(() => hrDormColumns(dormSubView.value, subsOfTab('hr-dorm')))

// 费用分摊明细弹窗
const dormBillVisible = ref(false)
const dormBillTitle = ref('')
const dormBillTab = ref<'personal' | 'dorm' | 'public' | 'dept'>('personal')
const dormBillItems = ref<any[]>([])

// 办理调宿 / 退宿
// 第 62 轮：费用信息筛选 + 住宿联动下拉
const dormDeptFilter = ref('')
const dormRoomTypeFilterFee = ref('')
const ROOM_TYPE_FEE_OPTIONS = [
  { label: '全部房源', value: '' },
  { label: '公租房', value: '公租房' },
  { label: '员工宿舍', value: '员工宿舍' },
  { label: '未住宿', value: '未住宿' },
]
const DORM_STATUS_FILTER_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '空闲', value: '空闲' },
  { label: '部分入住', value: '部分入住' },
  { label: '满员', value: '满员' },
  { label: '维修中', value: '维修中' },
]
const ASSET_STATUS_FILTER_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '正常', value: '正常' },
  { label: '损坏', value: '损坏' },
  { label: '报废', value: '报废' },
]
const dormStatusFilter = ref('')
const assetStatusFilter = ref('')
const DORM_DELETE_ROUTE: Record<string, 'rooms' | 'publics' | 'stays' | 'assets'> = {
  dorm: 'rooms',
  public: 'publics',
  stay: 'stays',
  asset: 'assets',
  fee: 'rooms',
}
const DORM_IMPORT_MODES = [
  { label: '合并更新', value: 'merge' },
  { label: '仅新增', value: 'insert' },
  { label: '仅更新', value: 'update' },
]
const CHECKOUT_REASONS = ['离职退宿', '个人外住', '调宿退宿', '工作调动', '其他原因']
  .map((v) => ({ label: v, value: v }))
const ROOM_TYPE_OPTIONS = ['单人间', '双人间', '夫妻房', '四人间', '六人间', '八人间']
  .map((v) => ({ label: v, value: v }))
const dormEmployeeList = ref<any[]>([])
const dormRoomList = ref<any[]>([])
const dormFacilityOptions = ref<string[]>([])
const dormTransferVisible = ref(false)
const dormTransferRoom = ref('')
const dormTransferBed = ref('')
const dormCheckoutVisible = ref(false)
const dormCheckoutDate = ref('')
const dormCheckoutReason = ref('')
const dormImportVisible = ref(false)

const currentColumns = computed<HrColumn[]>(() => {
  // 第 57 轮：花名册在职/离职共用 hr-roster scope，列集合按子视图切换
  if (activeTab.value === 'hr-roster') {
    const fallback = rosterStatusView.value === '离职' ? hrResignColumns() : hrRosterGroupColumns()
    return hrColumnsFromSubs(subsOfTab('hr-roster'), fallback)
  }
  if (activeTab.value === 'hr-contract') {
    return hrColumnsFromSubs(subsOfTab('hr-contract'), hrContractColumns())
  }
  // 第 56 轮：社保四标签各自的列全集全部由四胶囊字段契约派生（六同步），
  // 用户在设置里新增/引用的字段追加在固定骨架之后。
  if (activeTab.value === 'hr-social') {
    return withExtraSubs(hrSocialColumnsOf(socialSubView.value), subsOfTab('hr-social'))
  }
  if (activeTab.value === 'hr-dorm') return dormColumns.value
  if (activeTab.value === 'hr-fund' && fundSubView.value === 'bills') return hrBillColumnDefs()
  if (activeTab.value === 'hr-fund') {
    // 第 54 轮：18 个网厅原生字段为骨架，字段设置里新增/引用的字段追加在后。
    return withExtraSubs(hrFundColumns(), subsOfTab('hr-fund'))
  }
  return hrColumnDefs(activeTab.value)
})

/**
 * 固定列 + subs 里「固定列之外」的新增/引用字段。
 *
 * 社保与公积金的列表骨架是代码固定的（账期/险种/缴存基数…），不能整体改成
 * subs 派生（会把业务必需列冲掉）；但用户在字段设置里主动添加的字段必须
 * 能出列（六同步）。这里只追加差集，顺序排在固定列之后。
 */
function withExtraSubs(fixed: HrColumn[], subs: readonly any[]): HrColumn[] {
  const fixedNames = new Set(fixed.map((c) => c.colKey))
  const extra = hrColumnsFromSubs(subs, []).filter(
    (c) => c.colKey !== 'row-select' && !fixedNames.has(c.colKey),
  )
  return extra.length ? [...fixed, ...extra] : fixed
}
const businessColumns = computed(() => currentColumns.value.filter((c) => c.colKey !== 'row-select'))
/**
 * `[☰ 字段]` 可选全集：三个 HR 模块只能勾「启用字段」（后端 `enabled !== false`）。
 * 引用字段的 subs.name 是 `src_…`，列 colKey 是 label，故键集合以 label 优先。
 */
const configKeys = computed(() => {
  const subs = subsOfTab(activeTab.value)
  const nameOf = (s: any) => String(s?.label || s?.name || '').trim()
  return {
    defaults: subs.filter((s) => s && s.enabled !== false && s.is_default === true).map(nameOf).filter(Boolean),
    enabled: subs.filter((s) => s && s.enabled !== false).map(nameOf).filter(Boolean),
  }
})
const columnDefsForFilter = computed(() => {
  // 第 53 轮：5 个 HR 列表都能在「字段」筛选器里勾选新增/引用字段
  const isHrCfg = ['hr-roster', 'hr-resign', 'hr-contract', 'hr-social', 'hr-fund', 'hr-dorm'].includes(activeTab.value)
  const enabled = isHrCfg && configKeys.value.enabled.length
    ? new Set(configKeys.value.enabled) : null
  return businessColumns.value
    .filter((c) => !enabled || enabled.has(c.colKey))
    .map((c) => ({ key: c.colKey, label: c.title }))
})

// 四态视图各自保存列显隐，切换状态时互不污染。
// 第 57 轮：花名册列契约改为「在职 48 / 离职 14」双胶囊，版本升 v4；
// 社保持续 v4（第 56 轮列契约），其余模块保持 v3。
const STORAGE_KEY = computed(() => {
  const view = activeTab.value === 'hr-roster' ? rosterStatusView.value
    : activeTab.value === 'hr-social' ? socialSubView.value
      : activeTab.value === 'hr-contract' ? contractStatusView.value
        : activeTab.value === 'hr-dorm' ? dormSubView.value
          : 'all'
  const version = activeTab.value === 'hr-social' || activeTab.value === 'hr-roster' ? 'v4' : 'v3'
  return `weknora-hr-${activeTab.value}-${fundSubView.value}-${socialSubView.value}-${view}-cols-${version}`
})
const visibleColKeys = ref<string[]>([])
const visibleCols = computed(() => businessColumns.value.filter((c) => visibleColKeys.value.includes(c.colKey)))

function persistColumns() {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
/**
 * `[☰ 字段]` 的「重置」= 精确回到**后端** `defaultHeader` 字段集合。
 *
 * 严禁回全选或前端静态数组：用户可能刚在设置里关掉了几个默认表头，
 * 重置却把它们全勾回来，等于把上一次保存冲掉。
 */
function resetColumns() {
  // 第 57 轮：花名册（在职/离职）与社保四视图也严格按各自胶囊的 defaultHeader 重置
  const isHrCfg = ['hr-roster', 'hr-contract', 'hr-social', 'hr-fund', 'hr-dorm'].includes(activeTab.value)
  if (isHrCfg) {
    // 后端 subs 尚未加载时也不能回退成「全列」：字段契约里带 defaultHeader
    // 的字段就是静态默认表头，与后端 seed 同源（六同步首帧不闪全表）。
    const staticDefs = activeTab.value === 'hr-fund'
      ? (fundSubView.value === 'bills' ? HR_FUND_BILL_FIELDS : HR_FUND_FIELDS)
      : activeTab.value === 'hr-social'
        ? (socialSubView.value === 'receipt' ? HR_SOCI_RECEIPT_FIELDS
          : socialSubView.value === 'normal' ? HR_SOCI_NORMAL_FIELDS
            : socialSubView.value === 'abnormal' ? HR_SOCI_ABNORMAL_FIELDS
              : HR_SOCI_BILL_FIELDS)
        : activeTab.value === 'hr-roster' && rosterStatusView.value === '离职'
          ? HR_RESIGN_FIELDS
          : hrModuleFields('roster')
    const defaults = configKeys.value.defaults.length
      ? configKeys.value.defaults
      : activeTab.value === 'hr-social'
        ? HR_SOCI_DEFAULT_HEADERS[hrSocialGroupOf(socialSubView.value)]
        : staticDefs.filter((d) => d.defaultHeader).map((d) => d.name)
    visibleColKeys.value = defaults.filter((k) => businessColumns.value.some((c) => c.colKey === k))
  } else {
    visibleColKeys.value = businessColumns.value.map((c) => c.colKey)
  }
  persistColumns()
}
function selectAllColumns() { resetColumns() }
function onColumnsChange(keys: string[]) { visibleColKeys.value = keys; persistColumns() }
function initColumns() {
  const allKeys = businessColumns.value.map((c) => c.colKey)
  const isHrCfg = ['hr-roster', 'hr-contract', 'hr-social', 'hr-fund', 'hr-dorm'].includes(activeTab.value)
  const saved = localStorage.getItem(STORAGE_KEY.value)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      const matched = allKeys.filter((k) => Array.isArray(arr) && arr.includes(k))
      // 第 52 轮：旧存档可能含已删除分组/字段，只保留当前列全集里仍存在的键，
      // 且存档若已空（字段精简后全被过滤）就回落到后端默认表头。
      if (matched.length) { visibleColKeys.value = matched; return }
    } catch { /* ignore */ }
  }
  if (isHrCfg && configKeys.value.defaults.length) {
    visibleColKeys.value = configKeys.value.defaults.filter((k) => allKeys.includes(k))
    return
  }
  resetColumns()
}
watch(STORAGE_KEY, () => { initColumns() })

const tableColumns = computed(() => {
  const cols: any[] = []
  const vis = currentColumns.value.filter(
    (c) => c.colKey === 'row-select' || visibleColKeys.value.includes(c.colKey),
  )
  vis.forEach((c, i) => {
    // 勾选列：不带 fixed（第 38 轮起全站取消冻结列）
    if (c.colKey === 'row-select') {
      cols.push({ colKey: 'row-select', type: 'multiple', width: 44 })
      return
    }
      const values = rows.value.slice(0, 30).map((r) => c.value?.(r))
      const cfgWidth = Number(c.width) || 0
      const w = Math.max(colWidthOf(cfgWidth, c.title, values, c.dataType), Number(c.minWidth) || 0)
      const base: any = {
        colKey: c.colKey,
        title: c.title,
        // 用户显式配了列宽（>0）时关闭省略号：18 位身份证/19 位卡号要在 250px 里
        // 完整显示，不能再被 TEllipsis 截成 `5101111969122542...`。
        ellipsis: cfgWidth > 0 ? false : ellipsisOf(c),
        // 状态 / 月份列走具名插槽；其余用 cell 返回字符串（TDesign 1.19 不注入 h）
        cell: c.slot ? undefined : (_h: any, params: any) => c.value?.(params?.row) ?? '',
        dataType: c.dataType,
      }
      if (cfgWidth > 0) {
        // 第 53 轮硬性要求：配置了列宽就 width 与 minWidth 同时钉死，
        // 既不被自适应覆盖，也不被「最后一列拉伸」吃掉。
        base.width = cfgWidth
        base.minWidth = cfgWidth
      } else if (i === vis.length - 1) base.minWidth = w
      else base.width = w
    const cls = cellClassOf(c)
    if (cls) base.className = cls
    cols.push(base)
  })
  return cols
})
const tableScroll = computed(() => tableScrollOf(visibleCols.value))

// ---------------------------------------------------------------------------
// 合计行（勾选感知；loading 时不渲染）
// ---------------------------------------------------------------------------
// 从 currentColumns 派生，保证四态动态列 / 账单子视图与合计列严格同源，
// 不会出现「表头换了但合计列还是旧集合」的错位。
const summaryCols = computed<SummaryColumn[]>(() =>
  businessColumns.value
    .filter((c) => visibleColKeys.value.includes(c.colKey))
    .map((c) => ({
      key: c.colKey,
      label: c.title,
      // 比例列（noSum）不进合计：5% × N 人加成 60% 是脏数据
      fieldType: c.dataType === 'number' && !c.noSum ? 'number' : 'text',
      value: (row: any) => c.value?.(row) ?? '',
    })),
)
const footSpan = computed(() => footSpanOf(summaryCols.value))
const summaryScope = computed(() => summaryScopeRows(rows.value, selectedKeys.value, hrRowKey))
const footData = computed(() => [buildFootRow(summaryCols.value, summaryScope.value.rows, {
  selectKey: 'row-select',
  selectedCount: summaryScope.value.selectedCount,
})])
const tableLoading = computed(() => loading.value)
const tableFootData = computed(() => (tableLoading.value ? [] : footData.value))

const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
])

// ---------------------------------------------------------------------------
// 展示辅助
// ---------------------------------------------------------------------------
/**
 * TDesign 1.19 会把 `#<colKey>` 具名插槽**复用于表头渲染**，此时 row 为空，
 * 若照常走取值函数，表头就会显示成占位符「—」（真机截图实测）。
 * 表头上下文一律返回列标题，数据行才取真实值。
 */
function isHeaderContext(row: any): boolean {
  return !row || (typeof row === 'object' && Object.keys(row).length === 0)
}
function colValue(row: any, colKey: string): string {
  const c = currentColumns.value.find((x) => x.colKey === colKey)
  if (!c) return '—'
  if (isHeaderContext(row)) return c.title
  return c.value?.(row) ?? '—'
}
/** 公积金账户状态：正常=success / 封存=warning / 提取销户=default（灰）。 */
function fundStatusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '').trim()
  if (s === '正常') return 'success'
  if (s === '封存' || s === '待补录') return 'warning'
  if (s === '提取销户') return 'default'
  return 'default'
}
/**
 * 第 57 轮：回盘表五险参保状态用**三色字体**（不是 Tag，避免状态列被 Tag 撑宽）：
 * 参保缴费 #00a870 绿 / 终止缴费 #ed7b2f 橙 / 暂停缴费走 --td-text-color-primary 令牌。
 */
function siStatusTextClass(v: string): string {
  const s = String(v ?? '').trim()
  if (s === '参保缴费') return 'hr-si-status hr-si-status--active'
  if (s === '终止缴费') return 'hr-si-status hr-si-status--ended'
  if (s === '暂停缴费') return 'hr-si-status hr-si-status--paused'
  return 'hr-si-status'
}
/**
 * 第 58 轮：补退类型 / 记录类型统一红绿字（不是 Tag）。
 * 正常类 → 绿 #00a870；补缴 / 退费 / 调基 / 异常 → 红 #e34d59。
 */
function siAdjustTextClass(v: string): string {
  const s = String(v ?? '').trim()
  if (!s) return ''
  return /正常/.test(s) ? 'hr-si-adjust hr-si-adjust--ok' : 'hr-si-adjust hr-si-adjust--bad'
}
/** 月度账单状态：已申报/已汇缴 = 绿，待申报/待汇缴 = 橙，作废 = 灰。 */
function billStatusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '').trim()
  if (/已申报|已汇缴|已完成|正常/.test(s)) return 'success'
  if (/待申报|待汇缴|待/.test(s)) return 'warning'
  return 'default'
}
function statusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '')
  if (/在职|正常|已缴|生效|已汇总|已完成|正常缴存|active/.test(s)) return 'success'
  if (/离职|封存|停缴|暂停|过期|异常|作废/.test(s)) return 'danger'
  if (/待|进行|处理中|未|pending/.test(s)) return 'warning'
  return 'default'
}

const emptyIcon = 'search-error'
const emptyText = computed(() => '暂无数据')
const emptyHint = computed(() =>
  activeTab.value === 'hr-social'
    ? '切换上方四个标签查看回盘表与月度账单；点「导入」上传社保申报文件'
    : '点击右上角「设置」可调整字段，或先导入数据',
)

// ---------------------------------------------------------------------------
// 数据加载 + headerReady 门禁
// ---------------------------------------------------------------------------
function extractItems(res: any): any[] {
  const d = res?.data
  if (Array.isArray(d)) return d
  if (Array.isArray(d?.items)) return d.items
  if (Array.isArray(d?.list)) return d.list
  if (Array.isArray(res)) return res
  return []
}

async function loadFieldConfig(tab: HrTab) {
  try {
    const res: any = await listFleetCategories({ scope: hrScopeOf(tab) })
    fieldConfig.value = { ...fieldConfig.value, [tab]: Array.isArray(res?.data) ? res.data : [] }
  } catch {
    // 分类接口未就绪时用同步种子兜底，绝不让门禁卡住表头
    fieldConfig.value = { ...fieldConfig.value, [tab]: hrFieldSeeds(tab) as any }
  }
}

/** 第 57 轮：列表请求序号（防竞态覆盖）。 */
let listRequestSeq = 0

async function loadList(tab: HrTab) {
  // 第 57 轮：请求序号守卫 —— 快速切换标签时，先发的慢响应绝不能覆盖后发的快响应
  // （公积金「职工明细 96 条」偶发盖到「月度账单 1 条」就是这个问题）。
  const seq = ++listRequestSeq
  loading.value = true
  try {
    let items: any[] = []
    const kw = String(filterState.value?.keyword || '').trim() || undefined
    if (tab === 'hr-roster') {
      // 第 57 轮：在职 / 离职共用同一个 hr-roster scope，status 由子标签决定
      const res: any = await hrApi.listRosterEmployees({
        q: kw,
        department: rosterDeptFilter.value || undefined,
        status: rosterStatusView.value,
      })
      items = extractItems(res)
    } else if (tab === 'hr-contract') {
      const res: any = await hrApi.listContracts({
        q: kw,
        department: rosterDeptFilter.value || undefined,
        contract_status: contractStatusView.value === '全部状态' ? undefined : contractStatusView.value,
      })
      // 第 58 轮：劳务人员（劳务/返聘/实习/借调）单独成页，互不混排。
      items = extractItems(res).filter((r: any) => contractKindOf(r) === contractSubView.value)
    } else if (tab === 'hr-social') {
      // 第 56 轮：四标签各自独立列表，全部走新的社保四视图 API
      const v = socialSubView.value
      if (v === 'receipt') {
        items = extractItems(await hrApi.listSocialReceipts({
          q: kw,
          department: siDeptFilter.value || undefined,
          pension_status: siPensionStatusFilter.value || undefined,
        }))
      } else if (v === 'bills') {
        const list = sortMonthsDesc(extractItems(await hrApi.listSocialBills()), '账单周期')
        items = siBillMonthFilter.value
          ? list.filter((r: any) => String(r.账单周期 || r.month_label || '') === siBillMonthFilter.value)
          : list
      } else {
        items = extractItems(await hrApi.listSocialRecords({
          record_type: v, q: kw,
          department: siDeptFilter.value || undefined,
          scheme: siSchemeFilter.value || undefined,
          month: siMonthFilter.value || undefined,
        }))
      }
    } else if (tab === 'hr-dorm') {
      // 第 61 轮：费用信息 = 月度账单；其余四个档案页 = 通用宿舍记录表。
      if (dormSubView.value === 'fee') {
        // 第 62 轮：费用信息 = 全员月度扣款明细（跨模块聚合）
        const res: any = await hrApi.listDormCharges({
          month: dormMonthFilter.value || undefined,
          department: dormDeptFilter.value || undefined,
          room_type: dormRoomTypeFilterFee.value || undefined,
          q: kw,
        })
        items = extractItems(res)
        const d = res?.data || res || {}
        if (!dormMonthFilter.value && d.month) dormMonthFilter.value = d.month
      } else {
        items = extractItems(await hrApi.listDormRecords({
          module: hrDormModuleOf(dormSubView.value),
          q: kw,
        }))
        if (dormSubView.value === 'stay' && dormRoomTypeFilter.value) {
          items = items.filter((r: any) => String(r.房源类型 || '').trim() === dormRoomTypeFilter.value)
        }
        if ((dormSubView.value === 'dorm' || dormSubView.value === 'public') && dormStatusFilter.value) {
          items = items.filter((r: any) => String(r.宿舍状态 || '').trim() === dormStatusFilter.value)
        }
        if (dormSubView.value === 'asset' && assetStatusFilter.value) {
          items = items.filter((r: any) => String(r.资产状态 || '').trim() === assetStatusFilter.value)
        }
      }
    } else if (fundSubView.value === 'bills') {
      const list = sortMonthsDesc(extractItems(await hrApi.listFundBills()), 'month_label')
      items = fundBillMonthFilter.value
        ? list.filter((r: any) => String(r.month_label || r.账单周期 || '') === fundBillMonthFilter.value)
        : list
    } else {
      items = extractItems(await hrApi.listFundRecords(
        fundStatusView.value === '全部状态' ? undefined : fundStatusView.value,
      ))
    }
    if (seq !== listRequestSeq) return
    // TDesign 的 row-key="id" 是列名，要求每行都有 id 字段。后端三个模块主键都是
    // string id；万一某视图缺 id，用 hrRowKey(row) 补一个，保证 TDesign 的选中键、
    // selectedKeys 与合计行的 hrRowKey 三者口径统一（都等于该行 id）。
    rows.value = items.map((r: any) => ({ ...(r || {}), id: r?.id ?? hrRowKey(r) }))
    // 第 57 轮：所有 HR 卡片主数字一律由 reloadStats 的后端统计给出 ——
    // 花名册=在职人数、社保/公积金=月度账单条数，绝不能被子视图列表行数覆盖
    // （公积金「1 条」跳成「96 条」的真机 Bug 就出在这里）。
  } catch (e: any) {
    if (seq !== listRequestSeq) return
    rows.value = []
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    if (seq === listRequestSeq) loading.value = false
  }
}

async function loadFundSettings() {
  try {
    const res: any = await hrApi.getFundSettings()
    fundSettings.value = res?.data || res || null
  } catch { fundSettings.value = null }
}

async function reloadStats() {
  try {
    const res: any = await hrApi.getRosterStats()
    const d = res?.data || {}
    kpiCounts.value = {
      // 花名册卡 = 在职人数（d.roster）；d.total 是全员（含离职），不能拿来当花名册数
      'hr-roster': Number(d.roster ?? d.active ?? d.total ?? kpiCounts.value['hr-roster'] ?? 0) || 0,
      // 第 57 轮：离职人数只作为花名册卡副文案「离职 X 人」，不再是独立卡片
      resigned: Number(d.resigned ?? d.resign ?? 0) || 0,
      'hr-contract': Number(d.contract ?? kpiCounts.value['hr-contract'] ?? 0) || 0,
      'hr-social': Number(d.social ?? d.periods ?? kpiCounts.value['hr-social'] ?? 0) || 0,
      'hr-fund': Number(d.fund ?? d.records ?? kpiCounts.value['hr-fund'] ?? 0) || 0,
      // 第 54 轮：合同异常计数（未签订 / 即将到期 / 到期）供卡副文案
      contract_unsigned: Number(d.contract_unsigned ?? 0) || 0,
      contract_expiring: Number(d.contract_expiring ?? 0) || 0,
      contract_expired: Number(d.contract_expired ?? 0) || 0,
      // 第 55 轮：公积金全员联动提醒
      fund_pending: Number(d.fund_pending ?? 0) || 0,
      fund_to_seal: Number(d.fund_to_seal ?? 0) || 0,
      // 第 56 轮：社保全员联动提醒（待补录社保 / 离职待减员）
      social_pending: Number(d.social_pending ?? 0) || 0,
      social_to_seal: Number(d.social_to_seal ?? 0) || 0,
    }
    // 第 61 轮：宿舍卡片只显示房源总条数（宿舍 + 公租房），无异常副文案
    const dormRes: any = await hrApi.getDormStats().catch(() => null)
    if (dormRes) {
      const dd = dormRes.data || dormRes
      kpiCounts.value['hr-dorm'] = Number(dd.total ?? 0) || 0
    }
  } catch { /* 统计失败不阻塞列表 */ }
}

/**
 * 切换一级 Tab：**先撤表头 → 并行等字段配置与列表 → 一次性置 true**。
 * 这是「表头列数第 1 帧一次成型」的硬保证。
 */
async function switchTab(tab: HrTab) {
  headerReady.value = false
  activeTab.value = tab
  // 第 59 轮：公积金默认「月度账单」（标签第一位），社保默认「月度账单」，
  // 花名册默认「在职人员」。
  fundSubView.value = 'bills'
  fundStatusView.value = '正常'
  socialSubView.value = 'bills'
  dormSubView.value = 'dorm'
  // 第 62 轮：费用信息默认展示系统最新真实数据月 2026-10
  dormMonthFilter.value = '2026-10'
  dormDeptFilter.value = ''
  dormRoomTypeFilterFee.value = ''
  dormStatusFilter.value = ''
  assetStatusFilter.value = ''
  dormRoomTypeFilter.value = ''
  rosterStatusView.value = '在职'
  siDeptFilter.value = ''
  siSchemeFilter.value = ''
  siPensionStatusFilter.value = '参保缴费'
  selectedKeys.value = new Set()
  rosterDeptFilter.value = ''
  filterState.value = { keyword: '' }
  rows.value = []
  initColumns()
  if (tab === 'hr-fund') void loadFundSettings()
  await Promise.all([loadFieldConfig(tab), loadList(tab)])
  // 第 62 轮：subs 到位后必须重算一次可见列 —— initColumns 在 loadFieldConfig 之前
  // 跑时 configKeys.defaults 还是空的，宿舍等新标签会只剩勾选列（真机 1 列表格）。
  initColumns()
  headerReady.value = true
}

function onCardClick(card: any) {
  if (card.action !== 'type' || !card.typeValue) return
  if (activeTab.value === card.typeValue) return
  void switchTab(card.typeValue as HrTab)
}

/**
 * 第 46 轮：切换「在职状态」→ 先撤表头，再一次性换成该状态的专属列集合，
 * 并按状态重新拉列表（后端 status 过滤）。保证表头第 1 帧一次成型。
 */
/** 第 47 轮：统一「导入」入口（花名册 / 劳动合同走花名册模板，社保走多文件自适应）。 */
/** 第 58 轮：社保三类导入统一走中台 StandardImportDialog（回盘表固定覆盖）。 */
const socialImportVisible = ref(false)
const socialImportKind = ref<'receipt' | 'normal' | 'abnormal'>('receipt')

function onImportClick() {
  // 第 61 轮：宿舍四个档案页各有一套中文表头模板
  if (activeTab.value === 'hr-dorm') {
    if (dormSubView.value === 'fee') {
      MessagePlugin.info('费用账单由系统生成，不支持导入')
      return
    }
    dormImportVisible.value = true
    return
  }
  if (activeTab.value === 'hr-social') {
    if (socialSubView.value === 'bills') {
      MessagePlugin.info('月度账单由系统生成，请切换到回盘表 / 正常记录 / 异常记录后导入')
      return
    }
    socialImportKind.value = socialSubView.value as 'receipt' | 'normal' | 'abnormal'
    socialImportVisible.value = true
    return
  }
  if (activeTab.value === 'hr-fund' && fundSubView.value === 'bills') {
    MessagePlugin.info('月度账单由系统生成，请切换到「职工明细」后导入网厅文件')
    return
  }
  rosterImportVisible.value = true
}

/** 社保导入弹窗文案 / 模板 / 执行。 */
const socialImportLabel = computed(() => ({
  receipt: '社保回盘表', normal: '社保正常记录', abnormal: '社保异常记录',
}[socialImportKind.value]))

async function downloadSocialTemplate(): Promise<Blob> {
  const blob = await hrApi.getHrTemplate('social')
  return new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
}

async function runSocialImport(files: File[], _mode: string): Promise<number> {
  const fd = new FormData()
  if (socialImportKind.value === 'receipt') {
    fd.append('file', files[0])
    const res: any = await hrApi.importSocialReceipts(fd)
    const d = res?.data || res || {}
    await onSocialImported()
    return Number(d.rows || 0)
  }
  for (const f of files) fd.append('files', f)
  fd.append('record_type', socialImportKind.value)
  if (siMonthFilter.value) fd.append('month', siMonthFilter.value)
  const res: any = await hrApi.importSocialRecords(fd)
  const d = res?.data || res || {}
  await onSocialImported()
  return Number(d.rows || 0)
}

/** 导入完成后重拉列表与卡片，保持与既有导入路径一致。 */
async function onSocialImported() {
  await Promise.all([loadList('hr-social'), reloadStats()])
}

/** 第 57 轮：花名册在职 / 离职双标签切换（同 scope，不同 status 与列集合）。 */
/** 第 58 轮：劳动合同 / 劳务合同切换（同样走首帧一次成型门禁）。 */
async function onContractSubViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  rosterDeptFilter.value = ''
  filterState.value = { keyword: '' }
  // 劳动合同默认只看「正常」，劳务合同默认看全部状态（第 59 轮）
  contractStatusView.value = contractSubView.value === 'service' ? '全部状态' : '正常'
  initColumns()
  await Promise.all([loadFieldConfig('hr-contract'), loadList('hr-contract')])
  headerReady.value = true
}

async function onRosterViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  rosterDeptFilter.value = ''
  filterState.value = { keyword: '' }
  initColumns()
  await Promise.all([loadFieldConfig('hr-roster'), loadList('hr-roster')])
  headerReady.value = true
}

/** 第 56 轮：社保四标签切换同样走首帧一次成型门禁。 */
async function onSocialSubViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  siDeptFilter.value = ''
  siSchemeFilter.value = ''
  initColumns()
  await Promise.all([loadFieldConfig(activeTab.value), loadList(activeTab.value)])
  headerReady.value = true
}

async function onContractFilterChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  await loadList('hr-contract')
  headerReady.value = true
}

/** 公积金账户状态切换：先撤表头再换行集，避免首帧半套列。 */
async function onFundStatusChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  await loadList('hr-fund')
  headerReady.value = true
}

/** 第 57 轮：公积金月度账单按月份筛选（纯前端过滤，保证只出现合法账单行）。 */
function onFundBillMonthChange() {
  void loadList('hr-fund')
}

const siFileInputRef = ref<HTMLInputElement | null>(null)

/**
 * 第 56 轮：社保导入按当前标签自动分流。
 * 回盘表 → 单文件《人员增减申请回盘表》解析并回写花名册个人社保账号；
 * 正常 / 异常记录 → 多文件自适应（文件名推断险种、单位个人、费款所属期）。
 */
async function onSiFilesChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  if (!files.length) return
  if (socialSubView.value === 'bills') {
    MessagePlugin.warning('请在「回盘表 / 正常记录 / 异常记录」标签下导入源文件')
    return
  }
  const fd = new FormData()
  loading.value = true
  try {
    let message = ''
    if (socialSubView.value === 'receipt') {
      fd.append('file', files[0])
      const res: any = await hrApi.importSocialReceipts(fd)
      message = `已导入 ${Number(res?.data?.rows ?? res?.rows ?? 0)} 条回盘记录`
    } else {
      for (const f of files) fd.append('files', f)
      fd.append('record_type', socialSubView.value === 'abnormal' ? 'abnormal' : 'normal')
      fd.append('month', siMonthFilter.value || currentMonth())
      const res: any = await hrApi.importSocialRecords(fd)
      const d = res?.data || res || {}
      message = `已导入 ${Number(d.files ?? 0)} 个文件 / ${Number(d.rows ?? 0)} 条记录`
    }
    MessagePlugin.success(message)
    await onSocialSubViewChange()
    await reloadStats()
  } catch (err: any) {
    MessagePlugin.error(err?.message || '导入失败')
  } finally {
    loading.value = false
  }
}

/** 公积金二级切换同样走门禁，避免列集换了一半先渲染一帧。 */
async function onFundSubViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await Promise.all([loadFieldConfig(activeTab.value), loadList(activeTab.value)])
  headerReady.value = true
}

let filterTimer: ReturnType<typeof setTimeout> | null = null
let filterWatchReady = false
watch(filterState, () => {
  if (!filterWatchReady) return
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => { filterTimer = null; void loadList(activeTab.value) }, 300)
}, { deep: true })
function applyFilter() { void loadList(activeTab.value) }

function onTableSelectChange(val: any) {
  const arr = Array.isArray(val) ? val : []
  selectedKeys.value = new Set(arr.map(String))
}
function clearSelection() { selectedKeys.value = new Set() }
const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(hrRowKey(r))))

const { isTableFlat } = useTableFlatState({ loading, rows })
const listViewClass = computed(() => flatClassOf(isTableFlat.value))
const { headerFixStyle } = useTableHeaderFix()

// ---------------------------------------------------------------------------
// 详情抽屉（深拷贝草稿 + 手存）
// ---------------------------------------------------------------------------
const detailVisible = ref(false)
const detailRow = ref<any>(null)
const detailDirty = ref(false)
const saving = ref(false)
const editForm = ref<any>({ data: {}, remarkParts: {}, items: [] })
/**
 * 最近一次由系统自动生成的工号。
 *
 * 用于区分「用户手改」与「系统建议值」：两者相等才允许随部门重算。
 * 每次打开抽屉必须重置（否则上一个员工的建议值会残留在下一个新增会话里）。
 */
let lastAutoEmployeeId = ''
const editBaseline = ref('')
/** 部门下拉选项（全部部门 + 组织机构部门名）。 */
const rosterDeptSelectOptions = computed(() => [
  { label: '全部部门', value: '' },
  ...rosterDeptOptions.value.map((d) => ({ label: d, value: d })),
])

/**
 * 部门下拉选项 = 组织机构里单位的部门拍平（第 50 轮）。
 *
 * 必须走 `/org/units` 而不是旧的 `/org`：旧接口是单公司视角，
 * 多单位场景下会漏挂单位、顺序也与组织机构面板不一致。
 * 顺序严格按「单位顺序 → 单位内部门顺序」，与设置里的拖拽结果一致。
 */
async function loadRosterDepts() {
  try {
    const res: any = await hrApi.getOrgUnits()
    const d = res?.data || res || {}
    const units = Array.isArray(d.units) ? d.units : []
    const seen = new Set<string>()
    const out: string[] = []
    const unitNames: string[] = []
    for (const u of units) {
      const un = String(u?.name || '').trim()
      if (un && !unitNames.includes(un)) unitNames.push(un)
      for (const dept of Array.isArray(u?.departments) ? u.departments : []) {
        const name = String(dept?.name || '').trim()
        if (!name || seen.has(name)) continue
        seen.add(name)
        out.push(name)
      }
    }
    rosterDeptOptions.value = out
    rosterUnitOptions.value = unitNames
  } catch {
    rosterDeptOptions.value = []
    rosterUnitOptions.value = []
  }
}

async function onRosterDeptChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await loadList('hr-roster')
  headerReady.value = true
}

/** 派生/系统字段：详情抽屉里只读（自动跟随身份证 / 入职日期 / 合同到期计算）。 */
const READONLY_DETAIL_FIELDS = new Set([
  ...HR_DERIVED_FIELDS,
  '在职状态', '员工状态', '账期状态', '参保状态', '缴存状态', '状态',
  '合同状态', '账户状态', '账单状态',
  // 第 55 轮：跨模块引用字段在**非花名册**模块一律只读（工号/姓名/部门/岗位/
  // 证件号码/联系电话/入职日期/甲方三字段），花名册里它们才是原生可编辑字段。
  '个人社保账号', '个人公积金账号',
  '统一社会信用代码', '住所', '法定代表人',
])

/**
 * 社保正常 / 异常记录抽屉里仍只读的字段（第 58 轮）。
 *
 * 身份维度（月份 / 个人编号 / 姓名 / 身份证 / 险种）与系统派生的应缴合计不可改；
 * 缴费基数、单位/个人比例与金额、补退类型允许人工修正（与申报回盘常有分差）。
 */
const HR_SOCIAL_RECORD_LOCKED = new Set([
  '账单月份', '个人编号', '姓名', '身份证号', '险种类型', '记录类型', '源文件', '应缴合计',
])

/** 非花名册模块里来自花名册的引用字段（一律只读）。 */
const HR_REF_ONLY_FIELDS = new Set([  '工号', '姓名', '部门', '岗位', '证件号码', '联系电话', '入职日期',
  '统一社会信用代码', '住所', '法定代表人',
])

/** 当前抽屉对应的字段契约模块（社保/公积金没有 HR 字段契约，走单块渲染）。 */
const activeModule = computed<HrModule | null>(() => {
  // 第 57 轮：花名册在职 = roster 契约，离职 = resign 契约（同一个 scope 两个子视图）
  if (activeTab.value === 'hr-roster') return rosterStatusView.value === '离职' ? 'resign' : 'roster'
  if (activeTab.value === 'hr-contract') return 'contract'
  // 第 55 轮：职工明细抽屉按 18 字段契约渲染（枚举全部 t-select）
  if (activeTab.value === 'hr-fund' && fundSubView.value === 'ledger') return 'fund'
  return null
})

/**
 * 第 57 轮：抽屉字段顺序 100% 跟随「设置」面板当前 subs 的上下顺序。
 *
 * 旧实现按 schema 静态数组顺序渲染，用户在设置里拖动排序后抽屉纹丝不动。
 * 这里把 subs 的「显示名（label，= 列 colKey）」与「数据身份（name）」成对取出：
 * 显示名用于灌值/渲染，name 用于回查字段契约（改名后仍能命中类型与枚举）。
 */
const orderedSubPairs = computed(() => {
  const subs = subsOfTab(activeTab.value)
  return subs
    .filter((s: any) => s && s.enabled !== false)
    .map((s: any) => ({
      label: String(s.label || s.name || '').trim(),
      key: String(s.name || '').trim(),
      // 第 59 轮：把胶囊 group 带上，抽屉才能只取当前标签的字段（图 3 字段重复根因）
      group: String(s.group || '').trim(),
    }))
    .filter((p) => !!p.label)
})

/**
 * 第 52 轮：花名册抽屉严格按 6 大分组（在职信息 → 基本信息 → 联系信息 →
 * 紧急联系人情况 → 教育情况 → 工资社保信息）分块展示；字段顺序、显隐严格与
 * 「设置 → 字段设置」里启用的字段六同步（第 57 轮：组内顺序也按 subs 顺序）。
 *
 * 控件全部复用 BusinessSplitDetailDrawer 的 dataType / options 分支：
 * 日期 → t-date-picker，内置枚举 → t-select filterable，只读派生 → disabled。
 */
const drawerGroups = computed(() => {
  // 第 61 轮：宿舍编辑抽屉按当前标签的胶囊字段渲染（枚举/文本域/只读派生列）
  if (activeTab.value === 'hr-dorm' && dormSubView.value !== 'fee') {
    // 第 62 轮：编辑抽屉两分组（档案 / 费用项目），字段顺序跟 settings subs 一致
    const enabled = new Set(configKeys.value.enabled)
    const subs = subsOfTab('hr-dorm').filter((x: any) => String(x?.group || '') === HR_DORM_GROUP_OF[dormSubView.value])
    const rank = new Map<string, number>()
    subs.forEach((x: any, i: number) => rank.set(String(x?.name || ''), i))
    const adding = !detailRow.value
    return hrDormDrawerGroups(dormSubView.value).map((g) => ({
      key: g.key,
      label: g.label,
      fields: g.fields
        .filter((f) => enabled.size === 0 || enabled.has(f.name))
        // 第 62 轮：安排入住时不显示「退宿日期 / 退宿原因」（退宿走办理退宿弹窗）
        .filter((f) => !(adding && dormSubView.value === 'stay'
          && (f.name === '退宿日期' || f.name === '退宿原因')))
        .map((f) => {
          // 第 62 轮：斜杠字段（宿舍编号/房屋地址、所属公司/部门、管理员/舍长）不能做
          // 具名插槽（Vue 模板禁止 attr 名带「/」），改为在这里注入动态 options，
          // 由抽屉内置的可搜索下拉渲染；房型/床位号等静态枚举同样走 options。
          const dynOptions: Record<string, string[]> = {
            '所属公司/部门': dormUnitOptions.map((o) => o.value),
            '租赁单位': dormUnitOptions.map((o) => o.value),
            '管理员/舍长': dormEmployeeNameOptions.value.map((o) => o.value),
            '员工姓名': dormEmployeeNameOptions.value.map((o) => o.value),
            '宿舍编号/房屋地址': dormAvailableRoomOptions.value.map((o) => o.value),
            '所属宿舍': dormRoomSelectOptions.value.map((o) => o.value),
            '责任人': dormEmployeeNameOptions.value.map((o) => o.value),
          }
          const opts = dynOptions[f.name] || f.options
          return {
            name: f.name,
            label: (() => {
              const sub = subs.find((x: any) => String(x?.name || '') === f.name)
              const lb = String(sub?.label || '').trim()
              return lb || businessColumns.value.find((c) => c.colKey === f.name)?.title || f.name
            })(),
            dataType: f.type === 'textarea' ? 'textarea' : f.type === 'number' ? 'number'
              : f.type === 'date' ? 'date' : 'text',
            readonly: !!f.readonly,
            ...(opts?.length ? { options: opts } : {}),
          }
        })
        .sort((a, b) => (rank.get(a.name) ?? 999) - (rank.get(b.name) ?? 999)),
    }))
  }
  const mod = activeModule.value
  // 第 58 轮：社保「正常记录 / 异常记录」抽屉按当前胶囊的 16 字段契约渲染。
  // 之前 activeModule 为 null → 抽屉退回 businessColumns 且 data 全空（真机图 6）。
  if (!mod && activeTab.value === 'hr-social'
    && (socialSubView.value === 'normal' || socialSubView.value === 'abnormal')) {
    const fields = socialSubView.value === 'normal' ? HR_SOCI_NORMAL_FIELDS : HR_SOCI_ABNORMAL_FIELDS
    const enabled = new Set(configKeys.value.enabled)
    const list = fields
      .filter((f) => enabled.size === 0 || enabled.has(f.name))
      .map((f) => {
        const title = businessColumns.value.find((c) => c.colKey === f.name)?.title || f.name
        return {
          name: f.name,
          label: title,
          dataType: hrDataTypeOf(f),
          // 身份维度（姓名 / 身份证 / 险种 / 账单月份）与跨表部门只读，
          // 金额 / 比例 / 补退类型可编辑（保存走 PUT /social/records/:id）。
          readonly: !!f.source || HR_SOCIAL_RECORD_LOCKED.has(f.name),
          ...(f.options?.length ? { options: f.options } : {}),
        }
      })
    return [{ key: 'record', label: '社保记录', fields: list }]
  }
  if (!mod) return undefined
  const defs = mod === 'roster'
    ? HR_ROSTER_GROUPS.flatMap((g) => g.fields)
    : hrModuleFields(mod)
  const defOf = (p: { label: string; key: string }) =>
    defs.find((d) => d.name === p.key) || defs.find((d) => d.name === p.label)
  let pairs = orderedSubPairs.value.filter((p) => !!defOf(p))
  // subs 未就绪（首帧 / 接口失败）时回退字段契约顺序，绝不让抽屉空掉
  if (!pairs.length) pairs = defs.map((d) => ({ label: d.name, key: d.name, group: '' }))
  if (mod !== 'roster') pairs = pairs.filter((p) => !!defOf(p))
  // 第 59 轮：抽屉只取「当前激活胶囊」的字段，并按键名去重。
  // 劳动合同 / 劳务合同两组字段同名，旧实现把两组拼在一起 → 图 3 整套字段重复一遍。
  const wantGroup = mod === 'contract'
    ? (contractSubView.value === 'service' ? '劳务合同' : '劳动合同')
    : mod === 'fund' ? (fundSubView.value === 'bills' ? HR_FUND_BILL_GROUP : HR_FUND_DETAIL_GROUP) : ''
  if (wantGroup) {
    const scoped = pairs.filter((p) => !p.group || p.group === wantGroup)
    if (scoped.length) pairs = scoped
  }
  const seenPair = new Set<string>()
  pairs = pairs.filter((p) => {
    const k = p.label
    if (seenPair.has(k)) return false
    seenPair.add(k)
    return true
  })

  const groups: { key: string; label: string; fields: string[] }[] = []
  if (mod === 'roster') {
    // 组顺序 = 该组第一个字段在 subs 里出现的顺序；组内顺序 = subs 顺序
    for (const p of pairs) {
      const label = groupLabelOf(p.label) || '在职信息'
      let g = groups.find((x) => x.key === label)
      if (!g) { g = { key: label, label, fields: [] }; groups.push(g) }
      g.fields.push(p.label)
    }
  } else {
    const want = mod === 'fund'
      ? (fundSubView.value === 'bills' ? HR_FUND_BILL_GROUP : HR_FUND_DETAIL_GROUP)
      : hrModuleGroups(mod)[0]?.key || mod
    const label = hrModuleGroups(mod).find((g) => g.key === want)?.label || want
    groups.push({ key: want, label, fields: pairs.map((p) => p.label) })
  }
  return groups.filter((g) => g.fields.length).map((g) => ({
    key: g.key,
    label: g.label,
    fields: g.fields.map((name: string) => {
      const c = businessColumns.value.find((x) => x.colKey === name)
      const pair = pairs.find((p) => p.label === name)
      const def = pair ? defOf(pair) : defs.find((d) => d.name === name)
      const staticOptions = hrEnumsOf(name)
      const options = def?.dynamic === 'department' ? rosterDeptOptions.value
        : def?.dynamic === 'unit' ? rosterUnitOptions.value
          : (staticOptions.length ? staticOptions : (def?.options || staticOptions))
      return {
        name,
        label: c?.title || name,
        // 日期 / 多行文本 / 附件一律按字段契约类型渲染（第 57 轮不再降级成 text）
        dataType: def ? hrDataTypeOf(def) : (c?.dataType || 'text'),
        // 第 55 轮：带 source 的跨表引用字段一律只读，抽屉只允许编辑本页原生字段
        readonly: READONLY_DETAIL_FIELDS.has(name) || def?.type === 'system' || !!def?.source
          || (mod !== 'roster' && HR_REF_ONLY_FIELDS.has(name)),
        ...(options.length ? { options } : {}),
      }
    }),
  }))
})

const drawerFields = computed(() => {
  if (drawerGroups.value?.length) return drawerGroups.value.flatMap((g) => g.fields)
  return businessColumns.value.map((c) => ({
    name: c.colKey,
    label: c.title,
    dataType: c.dataType,
    readonly: READONLY_DETAIL_FIELDS.has(c.colKey),
  }))
})

// ---------------------------------------------------------------------------
// 身份证自动推导 + 入职日期派生（第 52 轮）
// ---------------------------------------------------------------------------
// 只写草稿对象，绝不 watch(form) 实时写回业务行、更不自动调保存接口。
/** 清洗证件号 + 推导出生日期 / 性别 / 年龄（幂等，打开抽屉时也调用一次）。 */
function applyIdCardDerivation(): void {
  const data = editForm.value?.data
  if (!data) return
  const cleaned = hrCleanValue('证件号码', data['证件号码'])
  if (cleaned !== data['证件号码']) data['证件号码'] = cleaned
  const info = hrParseIdCard(cleaned)
  if (!info) return
  data['出生日期'] = info.birthDate
  data['性别'] = info.gender
  data['年龄'] = String(info.age)
}

/** 按入职日期推导司龄；转正日期仅在为空时按试用期推算（不覆盖手填）。 */
function applyHireDerivation(): void {
  const data = editForm.value?.data
  if (!data) return
  const hire = data['入职日期']
  if (!String(hire || '').trim()) return
  data['司龄'] = hrSeniorityOf(hire)
  if (data['试用期'] && !String(data['转正日期'] || '').trim()) {
    data['转正日期'] = hrRegularDateOf(hire, data['试用期'])
  }
}

watch(() => editForm.value?.data?.['证件号码'], () => {
  if (activeModule.value === 'roster') applyIdCardDerivation()
})
watch(
  () => [editForm.value?.data?.['入职日期'], editForm.value?.data?.['试用期']] as const,
  () => { if (activeModule.value === 'roster') applyHireDerivation() },
)

async function openDetail(row: any) {
  // 第 62 轮：宿舍抽屉依赖员工/房源/资产下拉，打开前确保已加载
  if (activeTab.value === 'hr-dorm') await loadDormHelpers()
  // 费用信息行 → 四标签扣款明细弹窗（全员 / 宿舍水电 / 公租房 / 部门）
  if (activeTab.value === 'hr-dorm' && dormSubView.value === 'fee') {
    void openDormBill(row)
    return
  }
  // 第 55 轮：公积金月度账单行 → 打开当月职工缴存明细抽屉
  if (activeTab.value === 'hr-fund' && fundSubView.value === 'bills') {
    void openBillDetail(row)
    return
  }
  // 第 56 轮：社保月度账单行 → 打开社保扣款明细抽屉
  if (activeTab.value === 'hr-social' && socialSubView.value === 'bills') {
    void openSIBillDetail(row)
    return
  }
  // 社保回盘表 / 正常 / 异常记录：回盘表只读；正常与异常记录允许受控编辑并保存
  if (activeTab.value === 'hr-social') {
    const data: Record<string, any> = {}
    for (const c of businessColumns.value) {
      const key = String(c.colKey || '')
      if (!key) continue
      const raw = c.value?.(row) ?? row?.[key] ?? row?.custom_data?.[key] ?? ''
      data[key] = raw === '—' || raw === null || raw === undefined ? '' : String(raw)
    }
    // 兜底：数据库里可能还有值但不在当前可见列里（用户隐藏了列），仍灌进草稿
    for (const [k, v] of Object.entries(row || {})) {
      if (k === 'custom_data' || k === 'id') continue
      if (data[k] === undefined && (typeof v === 'string' || typeof v === 'number')) {
        data[k] = String(v)
      }
    }
    detailRow.value = row
    editForm.value = { data, remarkParts: {}, items: [] }
    editBaseline.value = JSON.stringify(editForm.value)
    detailDirty.value = false
    detailVisible.value = true
    return
  }
  detailRow.value = row
  // 第 53 轮：按**抽屉字段全集**灌值，而不是可见列（businessColumns）。
  // 用户把「部门」「职务」从表头隐藏后，旧实现草稿里没有这两个键，
  // 打开已有员工时部门显示为空、工号也无法随部门重算。
  const data: Record<string, any> = {}
  const fields = drawerFields.value.length ? drawerFields.value : businessColumns.value
  for (const f of fields) {
    const name = String((f as any).name || '')
    if (!name) continue
    const c = businessColumns.value.find((x) => x.colKey === name)
    const raw = c?.value?.(row)
      ?? row?.[name]
      ?? row?.custom_data?.[name]
      ?? row?.data?.[name]
      ?? ''
    const v = raw === '—' ? '' : raw
    data[name] = typeof v === 'string' ? v : String(v ?? '')
  }
  editForm.value = { data, remarkParts: {}, items: [] }
  // 新会话：清掉上一轮的系统建议值，否则「用户手改过」判定会把新员工的
  // 空工号当成手改值，选部门后不再自动生成。
  lastAutoEmployeeId = ''
  // 打开抽屉时先跑一次派生（幂等），再取基线 —— 避免 watch 在基线之后触发
  // 把刚打开的抽屉误标为「已修改」。
  if (activeModule.value === 'roster') {
    applyIdCardDerivation()
    applyHireDerivation()
  }
  editBaseline.value = JSON.stringify(editForm.value)
  detailDirty.value = false
  detailVisible.value = true
}

// 脏检查只标脏，**不自动保存**（第 19 轮）
watch(editForm, () => {
  detailDirty.value = JSON.stringify(editForm.value) !== editBaseline.value
}, { deep: true })

async function saveDetail() {
  // 第 61 轮：宿舍四类档案走通用宿舍记录接口（费用账单由系统生成，不可手改）
  if (activeTab.value === 'hr-dorm') {
    if (dormSubView.value === 'fee') {
      // 第 62 轮：费用明细允许人工改「其它扣款 / 退款 / 备注」，合计由后端重算
      const id = detailRow.value?.id
      if (!id) { detailVisible.value = false; return }
      const d = editForm.value.data || {}
      const numOf = (v: any) => {
        const n = Number(String(v ?? '').replace(/,/g, ''))
        return Number.isFinite(n) ? n : 0
      }
      saving.value = true
      try {
        await hrApi.updateDormCharge(String(id), {
          other: numOf(d['其它扣款(元)']),
          refund: numOf(d['退款(元)']),
          remark: String(d['备注'] || ''),
        })
        MessagePlugin.success('已保存')
        detailVisible.value = false
        await loadList('hr-dorm')
      } catch (e: any) {
        MessagePlugin.error(e?.message || '保存失败')
      } finally {
        saving.value = false
      }
      return
    }
    const d = { ...(editForm.value.data || {}) }
    for (const k of ['可用床位数', '月费用单价(元/月)', '季度租金(元/季)', '在住人数',
      '宿舍状态', '住宿状态', '资产状态', '账单状态', '入住天数', '扣款合计(元)', '补贴合计(元)']) {
      delete d[k]
    }
    // 住宿记录新增：安置未填的 ID / 默认值
    if (dormSubView.value === 'stay') {
      if (!String(d['住宿记录ID'] || '').trim()) d['住宿记录ID'] = 'STAY' + Date.now().toString().slice(-6)
      if (!String(d['住宿状态'] || '').trim()) d['住宿状态'] = '在住'
      if (!String(d['入住日期'] || '').trim()) d['入住日期'] = new Date().toISOString().slice(0, 10)
    }
    saving.value = true
    try {
      const module = hrDormModuleOf(dormSubView.value)
      const id = detailRow.value?.id
      if (id) await hrApi.updateDormRecord(id, d)
      else await hrApi.createDormRecord(module, d)
      MessagePlugin.success('已保存')
      detailVisible.value = false
      await loadList('hr-dorm')
      await reloadStats()
    } catch (e: any) {
      MessagePlugin.error(e?.message || '保存失败')
    } finally {
      saving.value = false
    }
    return
  }
  // 第 56 轮：社保回盘表与月度账单是申报 / 扣款快照，只读；
  // 第 58 轮：正常 / 异常记录允许受控编辑（金额、比例、补退类型），保存走专用接口，
  // 应缴合计由后端按分项重算，绝不把行数据误写到花名册。
  if (activeTab.value === 'hr-social') {
    if (socialSubView.value !== 'normal' && socialSubView.value !== 'abnormal') {
      MessagePlugin.info('社保回盘表与月度账单为只读数据')
      detailVisible.value = false
      return
    }
    const id = detailRow.value?.id
    if (!id) { MessagePlugin.warning('记录不存在'); return }
    const d = editForm.value.data || {}
    saving.value = true
    try {
      await hrApi.updateSocialRecord(String(id), {
        费款所属期: d['费款所属期'],
        缴费基数: d['缴费基数'],
        单位缴费比例: d['单位缴费比例'],
        单位应缴金额: d['单位应缴金额'],
        个人缴费比例: d['个人缴费比例'],
        个人应缴金额: d['个人应缴金额'],
        补退类型: d['补退类型'],
      })
      MessagePlugin.success('已保存')
      detailVisible.value = false
      await Promise.all([loadList('hr-social'), reloadStats()])
    } catch (e: any) {
      MessagePlugin.error(e?.message || '保存失败')
    } finally {
      saving.value = false
    }
    return
  }
  const row = detailRow.value
  const form = hrCleanForm(editForm.value.data || {})
  // 第 52 轮：保存前统一校验，错误直接拦截并给出清晰表单提示。
  const invalid = hrValidateForm(form)
  if (invalid) { MessagePlugin.warning(invalid); return }
  saving.value = true
  try {
    const payload: Record<string, any> = { ...(row || {}), ...form }
    const id = row?.id
    if (activeModule.value && activeTab.value !== 'hr-fund') {
      // 花名册 / 离职管理 / 劳动合同共用 hr_employees：离职与合同字段都是
      // 同一行的 custom_data 投影，抽屉保存也走同一个更新接口。
      if (id) await hrApi.updateRosterEmployee(id, payload)
      else if (activeTab.value === 'hr-roster') await hrApi.createRosterEmployee(payload)
      else { MessagePlugin.warning('请从花名册新增员工'); return }
    } else if (fundSubView.value === 'bills') {
      MessagePlugin.info('月度账单由系统生成，不支持手工编辑')
    } else {
      // 第 54 轮：公积金抽屉按 18 个原生字段编辑 —— 中文键落 custom_data，
      // 同时映射回模型列（账号 / 姓名 / 证件号 / 基数 / 三方金额），
      // 否则「改了缴存基数」只写进 JSON，月度账单仍按旧列汇总。
      const fd: any = { ...(row || {}) }
      const num = (k: string) => Number(String(form[k] ?? '').replace(/[,，\s]/g, '')) || 0
      fd.personal_account = form['职工账号'] ?? fd.personal_account ?? ''
      fd.name = form['职工姓名'] ?? fd.name ?? ''
      fd.identity_number = form['证件号码'] ?? fd.identity_number ?? ''
      fd.status = form['账户状态'] === '封存' ? 'sealed'
        : form['账户状态'] === '提取销户' ? 'extracted' : 'active'
      fd.personal_base = num('缴存基数(元)')
      fd.total_amount = num('月缴存额(元)')
      fd.company_amount = num('单位月缴存额(元)')
      fd.personal_amount = num('个人月缴存额(元)')
      fd.contribution_ratio = num('职工缴存比例(%)') || 5
      fd.custom_data = { ...(row?.custom_data || {}), ...form }
      if (id) await hrApi.updateFundRecord(id, fd)
      else await hrApi.createFundRecord(fd)
    }
    MessagePlugin.success('已保存')
    detailVisible.value = false
    await loadList(activeTab.value)
    // 第 55 轮：合同状态 / 公积金状态改完，顶部 KPI 卡片必须立即重算（免刷新）。
    await reloadStats()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function onDetailDelete() {
  const row = detailRow.value
  if (!row?.id) { detailVisible.value = false; return }
  try {
    // 第 62 轮：宿舍五标签必须走各自的删除接口，严禁落到社保/公积金/车队账单
    if (activeTab.value === 'hr-dorm') {
      if (dormSubView.value === 'fee') await hrApi.removeDormCharge(row.id)
      else await hrApi.removeDormTyped(DORM_DELETE_ROUTE[dormSubView.value], row.id)
    } else if (activeModule.value) await hrApi.removeRosterEmployee(row.id)
    else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
    else await hrApi.removeFundRecord(row.id)
    MessagePlugin.success('已删除')
    detailVisible.value = false
    await loadList(activeTab.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---------------------------------------------------------------------------
// 设置 / 新增 / 打印
// ---------------------------------------------------------------------------
const settingsVisible = ref(false)
/**
 * 设置抽屉保存后的六同步入口（第 50 轮）。
 *
 * 顺序不能反：先 `loadFieldConfig` 把后端最新 subs 拉回来，
 * 再据 `is_default` 重算列显隐，最后覆盖 localStorage。
 * 之前的实现只做了第一步，所以「取消默认表头」保存后列表毫无变化。
 */
async function onSettingsSaved() {
  // 第 62 轮：设置抽屉保存成功后必须立即关闭（用户要求，避免"点了没反应"）
  settingsVisible.value = false
  await loadFieldConfig(activeTab.value)
  const allKeys = businessColumns.value.map((c) => c.colKey)
  const isStrict = ['hr-roster', 'hr-resign', 'hr-contract', 'hr-fund', 'hr-dorm'].includes(activeTab.value)
  if (isStrict) {
    // 第 55 轮：严格按 defaultHeader === true && enabled !== false 重建表头，
    // 保存设置后立即同步列表表头与 [☰ 字段] 筛选器（清掉旧 localStorage 污染）。
    visibleColKeys.value = configKeys.value.defaults.filter((k) => allKeys.includes(k))
  } else {
    visibleColKeys.value = allKeys
  }
  persistColumns()
  await loadList(activeTab.value)
}

function onPrimaryAction() {
  if (activeTab.value === 'hr-social') { void handleAddPeriod(); return }
  // 第 61 轮：宿舍四个档案页的「+ 新增」走同一抽屉（字段契约由当前标签决定）
  if (activeTab.value === 'hr-dorm') { openDetail(null); return }
  // 第 53 轮：新增时**不预生成工号**。部门还没选，任何编码都是猜的；
  // 工号由 #field-部门 的 change 事件在选定部门后生成（用户可再手改）。
  openDetail(null)
}

/**
 * 第 48 轮：新增员工时按「部门 + 职务」自动生成工号建议值。
 * 只填空白工号，用户手改过（非空）绝不覆盖；存量工号不受影响。
 */
/**
 * 按「部门 + 职务」生成工号建议值（第 53 轮重写）。
 *
 * 铁律：**部门为空时绝不生成** —— 工号里的 5 位部门码没有来源，
 * 生成出来必然是错的（旧实现落到 `C020000001` 这种兜底码）。
 * 生成后写 `lastAutoEmployeeId`，供「用户手改过就不覆盖」判定。
 */
async function autoFillEmployeeId() {
  if (activeTab.value !== 'hr-roster') return
  const dept = String(editForm.value?.data?.['部门'] || '').trim()
  if (!dept) {
    // 未选部门：工号保持空（新增场景）；已有员工的历史工号不动
    if (!detailRow.value) {
      editForm.value.data['工号'] = ''
      editForm.value.data['employee_id'] = ''
      lastAutoEmployeeId = ''
    }
    return
  }
  const title = String(editForm.value?.data?.['职务'] || editForm.value?.data?.['岗位'] || '').trim()
  try {
    const res: any = await hrApi.nextEmployeeId({ department: dept, title })
    const d = res?.data || res || {}
    const next = String(d.employee_id || '').trim()
    if (!next) return
    const cur = String(editForm.value?.data?.['工号'] || '').trim()
    // 已有工号且不是上次系统建议值 → 用户手改过，不覆盖
    if (cur && cur !== lastAutoEmployeeId) return
    editForm.value.data['工号'] = next
    editForm.value.data['employee_id'] = next
    lastAutoEmployeeId = next
  } catch { /* 编码建议失败不阻断新增 */ }
}

/** 部门 / 职务变化后重算工号（部门清空时把自动生成的工号一并清空）。 */
async function onDeptOrTitleChange() {
  if (activeTab.value !== 'hr-roster') return
  const dept = String(editForm.value?.data?.['部门'] || '').trim()
  if (!dept) {
    const cur = String(editForm.value?.data?.['工号'] || '').trim()
    if (!cur || cur === lastAutoEmployeeId) {
      editForm.value.data['工号'] = ''
      editForm.value.data['employee_id'] = ''
      lastAutoEmployeeId = ''
    }
    return
  }
  await autoFillEmployeeId()
}
async function handleAddPeriod() {
  const d = new Date()
  const year_month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    await hrApi.createSocialPeriod({ year_month })
    MessagePlugin.success('已新增账期')
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '新增失败') }
}

const printVisible = ref(false)
const printData = ref<Uint8Array | null>(null)
const printUrl = ref('')
const printBusy = ref(false)
/** 第 58 轮：导出 Excel 的 loading 态 —— 首次点击要动态 import xlsx（~490KB），
    没有反馈时用户会连点，重复生成工作簿。 */
const exporting = ref(false)
async function handlePrint() {
  const list = selectedRows.value.length ? selectedRows.value : rows.value
  if (!list.length) { MessagePlugin.warning('当前无数据可打印'); return }
  // 第 59 轮：列表批量栏的「打印清单」**只打印当前所选记录行 + 当前可见列**。
  // 月度账单不再跳去打印内部人员明细 —— 那只能由弹窗内的「打印」触发（真机图 7）。
  printBusy.value = true
  try {
    const cols = visibleCols.value.map((c) => ({ key: c.colKey, label: c.title, value: (r: any) => c.value?.(r) ?? '' }))
    printData.value = await generateCatalogPdf({ title: printTitle.value, columns: cols, rows: list })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([printData.value as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '清单生成失败')
  } finally {
    printBusy.value = false
  }
}
function closePrint() {
  printVisible.value = false
  printData.value = null
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---------------------------------------------------------------------------
// 批量操作（导出 / 封存启封 / 离职恢复 / 删除）
// ---------------------------------------------------------------------------
function downloadBlob(blob: any, name: string) {
  const b = blob instanceof Blob
    ? blob
    : new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  const url = URL.createObjectURL(b)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}
async function handleExportExcel() {
  // 第 56 轮：社保四视图为申报回盘 / 明细投影，不给旧账期导出接口；
  // 统一走「打印清单」或账单抽屉里的「打印预览」生成 PDF 报表。
  if (activeTab.value === 'hr-social') {
    MessagePlugin.info('社保清单请使用「打印清单」或账单抽屉的「打印预览」导出')
    return
  }
  if (exporting.value) return
  exporting.value = true
  try {
    let blob: any
    if (activeModule.value) {
      blob = await hrApi.exportEmployees(selectedRowKeys.value)
    } else {
      blob = await hrApi.exportFund(selectedRowKeys.value)
    }
    downloadBlob(blob, `${printTitle.value}.xlsx`)
    MessagePlugin.success('已导出')
  } catch (e: any) { MessagePlugin.error(e?.message || '导出失败') }
  finally { exporting.value = false }
}

async function handleSeal(seal: boolean) {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    for (const id of ids) {
      if (seal) await hrApi.sealFundRecord(id)
      else await hrApi.unsealFundRecord(id)
    }
    MessagePlugin.success(seal ? '已封存' : '已启封')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

/** 第 46 轮：办理离职改为打开标准弹窗（离职时间/原因/证明/备注），不再静默提交。 */
function handleResign() {
  if (!selectedRowKeys.value.length) { MessagePlugin.warning('请先选择员工'); return }
  resignVisible.value = true
}

/**
 * 公司设置（左下角用户菜单弹窗）保存后：部门可能改名 → 存量员工已被后端级联更新，
 * 必须重拉部门/单位下拉、列表与卡片，否则界面还停在旧部门名上。
 *
 * 第 53 轮：组织机构从「设置抽屉」迁移到全局「公司设置」弹窗，
 * 用 window 事件跨组件通知（弹窗可能在任何页面打开）。
 */
async function onOrgChanged() {
  await loadRosterDepts()
  // 部门被删/改名后当前筛选可能指向不存在的部门 → 复位到全部
  if (rosterDeptFilter.value && !rosterDeptOptions.value.includes(rosterDeptFilter.value)) {
    rosterDeptFilter.value = ''
  }
  await loadFieldConfig(activeTab.value)
  await loadList(activeTab.value)
  await reloadStats()
}

async function onRosterImported() {
  await loadList(activeTab.value)
  await reloadStats()
}

async function onResignDone() {
  clearSelection()
  await loadList(activeTab.value)
  await reloadStats()
}

/**
 * 第 52 轮：离职证明上传（附件与路径都是**独立即时生效**的，不随抽屉保存）。
 * 第 60 轮：上传成功后同步草稿 + 列表行，并只把 `resign_proof_path` 一个字段落库
 * （局部 PUT，不会把抽屉里其它未保存的编辑一起提交）。
 */
const proofInputRef = ref<HTMLInputElement | null>(null)
const proofUploading = ref(false)
/**
 * 第 60 轮：直接同步点开隐藏的 file input。
 *
 * 浏览器的 user activation 只在本轮事件回调里有效 —— 任何 `setTimeout` / `await`
 * 之后再 click() 都会被静默拦截（真机表现：点了确定毫无反应）。因此这个函数
 * 必须由按钮 / t-popconfirm @confirm **同步**调用，内部只做 `.click()`。
 */
function pickProof() { proofInputRef.value?.click() }
async function onProofFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (!/\.(pdf|jpe?g|png)$/i.test(f.name)) {
    MessagePlugin.warning('离职证明仅支持 PDF / JPG / PNG 格式')
    return
  }
  if (f.size > 10 * 1024 * 1024) {
    MessagePlugin.warning('附件不能超过 10MB')
    return
  }
  proofUploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', f)
    const res: any = await hrApi.uploadResignProof(fd)
    const data = res?.data || res || {}
    const path = String(data.path || '')
    if (!path) { MessagePlugin.error('附件上传失败'); return }
    // 1) 抽屉草稿
    editForm.value.data['离职证明'] = path
    editForm.value.data['resign_proof_path'] = path
    // 2) 列表记录（detailRow 与 rows 里是同一个对象引用，原地改即刷新「离职证明」列）
    applyProofPathToRow(detailRow.value, path)
    // 3) 落库：只写离职证明这一个字段（局部 PUT 会合并存量 custom_data，
    //    不会把抽屉里其它未保存的编辑一起提交，仍遵守「手存生效」铁律）
    const id = String(detailRow.value?.id || '')
    if (id) {
      try {
        await hrApi.updateRosterEmployee(id, { resign_proof_path: path, 离职证明: path })
      } catch (err: any) {
        MessagePlugin.warning(err?.message || '附件已上传，但员工记录同步失败，请点保存重试')
        return
      }
    }
    MessagePlugin.success('离职证明已更新')
  } catch (err: any) {
    MessagePlugin.error(err?.message || '附件上传失败')
  } finally {
    proofUploading.value = false
  }
}

/** 把离职证明路径就地写回列表行（主列 + custom_data 双写，各种读法都能命中）。 */
function applyProofPathToRow(row: any, path: string) {
  if (!row || typeof row !== 'object') return
  row['离职证明'] = path
  row.resign_proof_path = path
  const cd = row.custom_data && typeof row.custom_data === 'object' ? row.custom_data : {}
  cd['离职证明'] = path
  cd.resign_proof_path = path
  row.custom_data = cd
}

/** 离职证明路径：主列优先，兼容 custom_data / data 包裹。 */
function proofPathOf(row: any): string {
  const v = row?.resign_proof_path ?? row?.['离职证明']
    ?? row?.custom_data?.['离职证明'] ?? row?.data?.['离职证明']
  return typeof v === 'string' ? v.trim() : ''
}

async function previewProof(row: any) {
  const raw = proofPathOf(row)
  const id = raw.split('/').pop() || ''
  if (!id) { MessagePlugin.warning('附件路径无效'); return }
  try {
    const blob: any = await hrApi.getResignProof(id)
    // 第 55 轮：页内预览器，严禁 window.open / target="_blank"
    releasePreviewUrl()
    const fileBlob = blob instanceof Blob ? blob : new Blob([blob])
    previewUrl = URL.createObjectURL(fileBlob)
    previewSrc.value = previewUrl
    previewName.value = raw.split('/').pop() || '离职证明'
    // 第 61 轮：以后端返回的 Content-Type 为准（历史种子文件会被后端动态生成 PNG，
    // 扩展名与真实字节可能不一致，按扩展名判断会让 pdf.js 去解析 PNG 而报错）。
    const ct = String(fileBlob.type || '')
    previewType.value = ct.includes('pdf') ? 'application/pdf'
      : ct.startsWith('image/') ? 'image'
        : /\.pdf(\?|$)/i.test(raw) ? 'application/pdf' : 'image'
    previewVisible.value = true
  } catch (e: any) { MessagePlugin.error(e?.message || '附件预览失败') }
}

/**
 * 第 58 轮：附件预览 blob URL 生命周期收口。
 * 关闭预览 / 换文件 / 页面卸载三处都 revoke，避免 blob 占内存到会话结束。
 */
function releasePreviewUrl() {
  if (previewUrl) {
    URL.revokeObjectURL(previewUrl)
    previewUrl = ''
  }
  previewSrc.value = ''
}
function onPreviewVisibleChange(v: boolean) {
  previewVisible.value = v
  if (!v) releasePreviewUrl()
}
/** 第 58 轮：账单打印预览关闭时同样释放 blob（原先只在重新生成时 revoke）。 */
function closeBillPrint() {
  billPrintVisible.value = false
  billPrintData.value = null
  if (billPrintUrl.value) { URL.revokeObjectURL(billPrintUrl.value); billPrintUrl.value = '' }
}
/**
 * 第 57 轮：恢复在职（已在模板里二次确认）。
 * 恢复只改 status / resign_date，员工整行档案（含 custom_data 全部字段）原样保留，
 * 转回在职管理列表即可完整看到；随后重算四张卡片（社保/公积金异常人数实时同步）。
 */
async function handleRestore() {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择员工'); return }
  try {
    await hrApi.restoreRosterEmployees(ids)
    MessagePlugin.success('已恢复在职')
    clearSelection()
    await Promise.all([loadList(activeTab.value), reloadStats()])
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

async function handleBatchDelete() {
  const list = selectedRows.value
  if (!list.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    if (activeTab.value === 'hr-dorm') {
      for (const row of list) {
        if (!row?.id) continue
        if (dormSubView.value === 'fee') await hrApi.removeDormCharge(row.id)
        else await hrApi.removeDormTyped(DORM_DELETE_ROUTE[dormSubView.value], row.id)
      }
    } else if (activeModule.value && activeTab.value !== 'hr-fund') {
      await hrApi.batchDeleteRosterEmployees(list.map((r) => r.id).filter(Boolean))
    } else {
      for (const row of list) {
        if (!row?.id) continue
        if (activeTab.value === 'hr-social') {
          if (socialSubView.value === 'bills') await hrApi.removeSocialBill(row.id)
          else { MessagePlugin.warning('申报回盘与扣款记录为只读，仅月度账单可删除'); continue }
        } else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
        else await hrApi.removeFundRecord(row.id)
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---------------------------------------------------------------------------
// 社保业务按钮 / 公积金账单 / 文件导入
// ---------------------------------------------------------------------------
/** 第 55/56 轮：点「生成账单」先选月份（公积金 / 社保共用标准弹窗）。 */
function handleGenerateBill() {
  billMonth.value = activeTab.value === 'hr-dorm' ? (dormMonthFilter.value || '2026-10') : currentMonth()
  billMonthVisible.value = true
}

/**
 * 选好月份后先做前置校验（第 60 轮），再决定发不发写请求：
 *
 *   社保：① 该月「正常记录」必须 > 0（补退记录选传）—— 0 条直接 Toast 拦截，
 *            绝不允许生成一张 0 人 0 元的空账单；
 *         ② 已存在同月账单 → 二次确认覆盖。
 *   公积金：保持原有的「同月已有账单 → 二次确认」。
 *
 * 后端 CreateSIBill 同样校验（defense in depth），前端这层只为把错误翻译成
 * 用户能直接照做的提示，不用等一次 400。
 */
async function confirmGenerateBill() {
  const month = String(billMonth.value || '').trim()
  if (!month) { MessagePlugin.warning('请先选择账单月份'); return }
  const isSocial = activeTab.value === 'hr-social'
  try {
    if (isSocial) {
      const r: any = await hrApi.precheckSocialBill(month)
      const pre = r?.data || r || {}
      if (Number(pre.normal_record_count || 0) <= 0) {
        billMonthVisible.value = false
        MessagePlugin.warning('该月份暂无正常社保记录，请先上传社保系统导出的记录数据')
        return
      }
      if (pre.bill_exists) {
        billOverwriteText.value = `「${month}」月度账单已存在，重新生成将覆盖原有账单数据，是否继续？`
        billMonthVisible.value = false
        billOverwriteVisible.value = true
        return
      }
      await doGenerateBill(false)
      return
    }
    // 第 62 轮：宿舍费用信息（全员扣款明细）—— 已存在则二次确认覆盖
    if (activeTab.value === 'hr-dorm') {
      const res: any = await hrApi.listDormCharges({ month })
      const list = extractItems(res)
      if (list.length) {
        billOverwriteText.value = `「${month}」费用扣款明细已存在，重新生成将覆盖原有明细，是否继续？`
        billMonthVisible.value = false
        billOverwriteVisible.value = true
        return
      }
      await doGenerateBill(false)
      return
    }
    const res: any = await hrApi.listFundBills()
    const list = extractItems(res)
    const dup = list.some((r: any) => String(r.month_label || r.账单周期 || r.月份 || '') === month)
    if (dup) {
      billOverwriteText.value = `「${month}」已存在月度账单记录，每个月度只能生成一条账单，是否覆盖？`
      billMonthVisible.value = false
      billOverwriteVisible.value = true
      return
    }
    await doGenerateBill(false)
  } catch (e: any) { MessagePlugin.error(e?.message || '检查账单失败') }
}

async function doGenerateBill(overwrite: boolean) {
  billOverwriteVisible.value = false
  const isSocial = activeTab.value === 'hr-social'
  try {
    // 第 62 轮：宿舍费用信息 = 跨模块聚合生成全员扣款明细
    if (activeTab.value === 'hr-dorm') {
      const res: any = await hrApi.generateDormCharges({ month: billMonth.value, overwrite })
      const d = res?.data || res || {}
      MessagePlugin.success(overwrite
        ? `已覆盖生成 ${d.rows || 0} 条费用扣款明细`
        : `已生成 ${d.rows || 0} 条费用扣款明细`)
      billMonthVisible.value = false
      dormMonthFilter.value = billMonth.value
      await loadList('hr-dorm')
      return
    }
    if (isSocial) {
      await hrApi.createSocialBill({ month_label: billMonth.value, overwrite })
    } else {
      await hrApi.createFundBill({ month_label: billMonth.value, overwrite })
    }
    MessagePlugin.success(overwrite ? '已覆盖生成月度账单' : '已生成月度账单')
    billMonthVisible.value = false
    if (isSocial) {
      socialSubView.value = 'bills'
      await onSocialSubViewChange()
    } else {
      fundSubView.value = 'bills'
      await onFundSubViewChange()
    }
    await reloadStats()
  } catch (e: any) { MessagePlugin.error(e?.message || '生成失败') }
}

// ---- 月度账单详情抽屉（搜索 / 部门 / 类型 / 缴存主体 / 部门汇总 / 打印） ----
/** 第 57 轮：账单三标签弹窗的公共初始化（社保 / 公积金统一）。 */
function resetBillDialog(kind: 'fund' | 'social', title: string) {
  billDetailKind.value = kind
  billDetailTab.value = kind === 'social' ? 'personal' : 'summary'
  billDetailTitle.value = title
  billDetailVisible.value = true
  billRows.value = []
  billKeyword.value = ''
  billDept.value = ''
  billScheme.value = '全部'
  billDeptKeyword.value = ''
}

async function openBillDetail(row: any) {
  resetBillDialog('fund', `${row?.month_label || row?.账单周期 || ''} 公积金缴存明细`)
  const id = row?.id
  if (!id) return
  try {
    const res: any = await hrApi.getFundBill(id)
    const d = res?.data || res || {}
    const items: any[] = Array.isArray(d.items) ? d.items : []
    billRows.value = items.map((x: any, i: number) => ({
      ...x,
      id: x.id || `pf-${i}`,
      姓名: x.name || x.姓名 || '',
      账号: x.personal_account || x.账号 || '',
      证件号码: x.identity_number || x.证件号码 || '',
      部门: x.department || x.部门 || '',
      缴存基数: Number(x.personal_base ?? x.缴存基数 ?? 0),
      单位缴存: Number(x.company_amount ?? x.单位缴存 ?? 0),
      个人缴存: Number(x.personal_amount ?? x.个人缴存 ?? 0),
      合计: Number(x.total_amount ?? x.合计 ?? 0),
    }))
  } catch (e: any) { MessagePlugin.error(e?.message || '加载账单明细失败') }
}

/** 第 57 轮：社保月度账单 → 扣款明细弹窗（个人明细 / 单位明细 / 汇总数据）。 */
async function openSIBillDetail(row: any) {
  resetBillDialog('social', `${row?.账单周期 || row?.month_label || ''} 社保扣款明细`)
  const id = row?.id
  if (!id) return
  try {
    const res: any = await hrApi.getSocialBill(id)
    const d = res?.data || res || {}
    const items: any[] = Array.isArray(d.items) ? d.items : []
    billRows.value = items.map((x: any, i: number) => ({ ...x, id: x.id || `si-${i}` }))
  } catch (e: any) { MessagePlugin.error(e?.message || '加载社保账单明细失败') }
}

/** 三标签切换：清空两个关键词，部门与险种条件保留（用户通常按部门逐层看）。 */
function onBillDetailTabChange() {
  billKeyword.value = ''
  billDeptKeyword.value = ''
}

const billFiltered = computed(() => {
  const kw = billKeyword.value.trim().toLowerCase()
  const isSocial = billDetailKind.value === 'social'
  return billRows.value.filter((r) => {
    if (billDept.value && String(r.部门 || '') !== billDept.value) return false
    if (isSocial) {
      const keys = SI_BILL_SCHEME_KEYS[String(billScheme.value || '')]
      if (keys && !keys.some((k) => Number(String(r[k] ?? '').replace(/,/g, '')) !== 0)) return false
    }
    if (!kw) return true
    const searchKeys = isSocial
      ? [r.姓名, r.个人编号, r.证件号码, r.部门]
      : [r.姓名, r.账号, r.证件号码, r.部门]
    return searchKeys.some((v) => String(v || '').toLowerCase().includes(kw))
  })
})

// ---------------------------------------------------------------------------
// 第 57 轮：账单三标签弹窗的列表 / 合计 / 部门汇总
// 金额一律按「分」整数累加，空值留白（绝不显示横线 —），合计行恒贴底。
// ---------------------------------------------------------------------------
function siCents(v: any): number {
  return Math.round(Number(String(v ?? '').replace(/[,，\s]/g, '')) * 100) || 0
}
function siFmt(cents: number): string {
  return grouped(cents / 100, { fixed: true, fallback: '0.00' })
}
function siMoney(v: any): string {
  const c = siCents(v)
  return c ? siFmt(c) : ''
}
/** 公积金金额：0 是有效值，固定两位小数（与社保宽表的留白口径不同）。 */
function pfMoney(v: any): string {
  return siFmt(siCents(v))
}

/** 明细表（个人 / 单位）列定义：社保按险种拆列，公积金只有一列缴存额。 */
const BILL_SI_PERSONAL_COLUMNS = [
  { colKey: '序号', title: '序号', width: 76 },
  { colKey: '姓名', title: '姓名', width: 110 },
  { colKey: '部门', title: '部门', width: 120 },
  { colKey: '基数', title: '基数', width: 110 },
  { colKey: '养老个人', title: '养老个人', width: 120 },
  { colKey: '医疗个人', title: '医疗个人', width: 120 },
  { colKey: '大额医疗', title: '大额医疗', width: 120 },
  { colKey: '失业个人', title: '失业个人', width: 120 },
  { colKey: '个人小计', title: '个人小计', width: 130 },
]
const BILL_SI_UNIT_COLUMNS = [
  { colKey: '序号', title: '序号', width: 76 },
  { colKey: '姓名', title: '姓名', width: 110 },
  { colKey: '部门', title: '部门', width: 120 },
  { colKey: '基数', title: '基数', width: 110 },
  { colKey: '养老单位', title: '养老单位', width: 120 },
  { colKey: '医疗+生育单位', title: '医疗+生育单位', width: 150 },
  { colKey: '工伤单位', title: '工伤单位', width: 120 },
  { colKey: '失业单位', title: '失业单位', width: 120 },
  { colKey: '单位小计', title: '单位小计', width: 130 },
]
const BILL_PF_PERSONAL_COLUMNS = [
  { colKey: '序号', title: '序号', width: 76 },
  { colKey: '账号', title: '个人账号', width: 150 },
  { colKey: '姓名', title: '姓名', width: 110 },
  { colKey: '部门', title: '部门', width: 130 },
  { colKey: '缴存基数', title: '缴存基数(元)', width: 140 },
  { colKey: '个人缴存', title: '个人缴存额(元)', width: 150 },
]
const BILL_PF_UNIT_COLUMNS = [
  { colKey: '序号', title: '序号', width: 76 },
  { colKey: '账号', title: '个人账号', width: 150 },
  { colKey: '姓名', title: '姓名', width: 110 },
  { colKey: '部门', title: '部门', width: 130 },
  { colKey: '缴存基数', title: '缴存基数(元)', width: 140 },
  { colKey: '单位缴存', title: '单位缴存额(元)', width: 150 },
]
/** 汇总数据：人数 / 个人 / 单位 / 合计（社保与公积金同构）。 */
const BILL_SUMMARY_COLUMNS = [
  { colKey: '序号', title: '序号', width: 76 },
  { colKey: '部门', title: '部门', width: 180 },
  { colKey: '人数', title: '人数', width: 100 },
  { colKey: '个人金额', title: '个人金额', width: 170 },
  { colKey: '单位金额', title: '单位金额', width: 170 },
  { colKey: '合计', title: '合计', width: 150 },
]
const SI_PERSONAL_MONEY = ['养老个人', '医疗个人', '大额医疗', '失业个人', '个人小计']
const SI_UNIT_MONEY = ['养老单位', '医疗+生育单位', '工伤单位', '失业单位', '单位小计']
const SI_DEPT_ORDER = ['财务部', '行政部', '销售部', '机电部', '库房', '过磅', '质检部',
  '生管', '大拉', '中拉', '小拉', '管退', '束绞', '多头']

const billSubjectColumns = computed(() => {
  if (billDetailKind.value === 'fund') {
    return billDetailTab.value === 'unit' ? BILL_PF_UNIT_COLUMNS : BILL_PF_PERSONAL_COLUMNS
  }
  return billDetailTab.value === 'unit' ? BILL_SI_UNIT_COLUMNS : BILL_SI_PERSONAL_COLUMNS
})
const billSummaryColumns = computed(() => BILL_SUMMARY_COLUMNS)

/** 社保人数口径：补缴行只追加金额，不额外计一个人（恒 75 人）。 */
const siBillPeople = computed(() => siBillPeopleOf(billFiltered.value))
/** 公积金属地人数：按证件号码去重。 */
const billPeopleCount = computed(() => billPeopleCountOf(billFiltered.value))

/** 个人明细 / 单位明细：一人多条记录逐行独立展示（陈刚正常行 + 补缴行 = 2 行）。 */
const billSubjectRows = computed(() => {
  const rows = billFiltered.value
  if (billDetailKind.value === 'fund') {
    const isUnit = billDetailTab.value === 'unit'
    return rows.map((r, i) => ({
      id: r.id || `pf-${i}`,
      序号: String(i + 1),
      账号: r.账号 || '',
      姓名: r.姓名 || '',
      部门: r.部门 || '',
      缴存基数: pfMoney(r.缴存基数),
      个人缴存: pfMoney(r.个人缴存),
      单位缴存: pfMoney(r.单位缴存),
      金额: pfMoney(isUnit ? r.单位缴存 : r.个人缴存),
    }))
  }
  const isUnit = billDetailTab.value === 'unit'
  const money = isUnit ? SI_UNIT_MONEY : SI_PERSONAL_MONEY
  return rows.map((r, i) => {
    const out: Record<string, any> = {
      id: r.id || `si-${i}`,
      序号: r.序号 || String(i + 1),
      姓名: r.姓名 || '',
      部门: r.部门 || '',
      基数: siMoney(r.基数),
    }
    for (const k of money) out[k] = siMoney(r[k])
    return out
  })
})

/** 汇总数据：按当前筛选结果实时聚合（补缴行并入金额但不增加人数）。 */
const billSummaryRows = computed(() => {
  const kw = billDeptKeyword.value.trim().toLowerCase()
  const isSocial = billDetailKind.value === 'social'
  const m = new Map<string, { 部门: string; 人数: number; p: number; u: number }>()
  for (const r of billFiltered.value) {
    const d = String(r.部门 || '未分配部门').trim() || '未分配部门'
    const e = m.get(d) || { 部门: d, 人数: 0, p: 0, u: 0 }
    if (!isAdjustmentRow(r)) e.人数++
    if (isSocial) {
      e.p += siCents(r.个人小计)
      e.u += siCents(r.单位小计)
    } else {
      e.p += siCents(r.个人缴存)
      e.u += siCents(r.单位缴存)
    }
    m.set(d, e)
  }
  const rank = (d: string) => {
    if (!isSocial) return 0
    const i = SI_DEPT_ORDER.indexOf(d)
    return i < 0 ? SI_DEPT_ORDER.length : i
  }
  let keys = Array.from(m.keys()).sort((a, b) => rank(a) - rank(b) || a.localeCompare(b, 'zh-Hans-CN'))
  if (kw) keys = keys.filter((k) => k.toLowerCase().includes(kw))
  return keys.map((k, i) => {
    const e = m.get(k)!
    return {
      id: `dept-${k}`, 序号: String(i + 1), 部门: k, 人数: e.人数,
      个人金额: siFmt(e.p), 单位金额: siFmt(e.u), 合计: siFmt(e.p + e.u),
    }
  })
})

/** 明细合计行：各金额列按分精确累加；人数去重后单独给「合计（N 人 · M 条）」。 */
const billSubjectFoot = computed(() => {
  if (!billSubjectRows.value.length) return []
  const people = billDetailKind.value === 'social' ? siBillPeople.value : billPeopleCount.value
  const label = `合计 ${people} 人`
  if (billDetailKind.value === 'fund') {
    const isUnit = billDetailTab.value === 'unit'
    const key = isUnit ? '单位缴存' : '个人缴存'
    return [{
      id: 'foot', 序号: label, 账号: '', 姓名: '', 部门: '',
      缴存基数: siFmt(billFiltered.value.reduce((n, r) => n + siCents(r.缴存基数), 0)),
      [key]: siFmt(billFiltered.value.reduce((n, r) => n + siCents(r[key]), 0)),
      金额: siFmt(billFiltered.value.reduce((n, r) => n + siCents(r[key]), 0)),
    }]
  }
  const money = billDetailTab.value === 'unit' ? SI_UNIT_MONEY : SI_PERSONAL_MONEY
  const foot: Record<string, any> = { id: 'foot', 序号: label, 姓名: '', 部门: '', 基数: '' }
  for (const k of money) {
    foot[k] = siFmt(billFiltered.value.reduce((n, r) => n + siCents(r[k]), 0))
  }
  return [foot]
})

/** 汇总合计行：人数与三列金额。 */
const billSummaryFoot = computed(() => {
  const rows = billSummaryRows.value
  if (!rows.length) return []
  const p = rows.reduce((n, r) => n + siCents(r.个人金额), 0)
  const u = rows.reduce((n, r) => n + siCents(r.单位金额), 0)
  const people = rows.reduce((n, r) => n + (Number(r.人数) || 0), 0)
  return [{ id: 'foot', 序号: `合计 ${people} 人`, 部门: '', 人数: people,
    个人金额: siFmt(p), 单位金额: siFmt(u), 合计: siFmt(p + u) }]
})

/**
 * 第 57 轮：账单打印 —— 严格只打印**当前激活标签页**的数据表
 * （个人明细 / 单位明细 / 汇总数据），并附带底部合计行。
 * 100% 复用中台 PrintPreviewDialog + generateCatalogPdf，不新造打印组件。
 */
async function printBillDetail() {
  const isSummary = billDetailTab.value === 'summary'
  const activeColumns = isSummary ? billSummaryColumns.value : billSubjectColumns.value
  const activeRows = isSummary ? billSummaryRows.value : billSubjectRows.value
  const footRows = isSummary ? billSummaryFoot.value : billSubjectFoot.value
  if (!activeRows.length) { MessagePlugin.warning('当前无数据可打印'); return }
  printBusy.value = true
  try {
    const columns = activeColumns.map((c) => ({
      key: c.colKey,
      label: c.title,
      value: (r: any) => String((r as any)[c.colKey] ?? ''),
    }))
    const rows = [...activeRows, ...footRows]
    // 第 59 轮：账单打印标题带年月 ——「社保缴费2026年11月账单」/「公积金缴存2026年11月账单」
    const ym = /(\d{4})-(\d{2})/.exec(String(billDetailTitle.value)) || []
    const monthLabel = ym[1] ? `${ym[1]}年${Number(ym[2])}月账单` : '月度账单'
    const billPrefix = billDetailKind.value === 'social' ? '社保缴费' : '公积金缴存'
    const title = `${billPrefix}${monthLabel} · ${isSummary ? '汇总数据' : billDetailTab.value === 'unit' ? '单位明细' : '个人明细'}`
    billPrintData.value = await generateCatalogPdf({ title, columns, rows })
    if (billPrintUrl.value) URL.revokeObjectURL(billPrintUrl.value)
    billPrintUrl.value = URL.createObjectURL(
      new Blob([billPrintData.value as unknown as BlobPart], { type: 'application/pdf' }),
    )
    billPrintTitle.value = title
    billPrintVisible.value = true
  } catch (e: any) { MessagePlugin.error(e?.message || '打印预览生成失败') }
  finally { printBusy.value = false }
}

/**
 * 第 57 轮：导出 Excel —— 只导出当前标签页（与打印同源数据），
 * 动态 import xlsx，避免把 ~490KB 的表格库塞进人事首屏 chunk。
 */
async function exportBillExcel() {
  const isSummary = billDetailTab.value === 'summary'
  const activeColumns = isSummary ? billSummaryColumns.value : billSubjectColumns.value
  const activeRows = isSummary ? billSummaryRows.value : billSubjectRows.value
  const footRows = isSummary ? billSummaryFoot.value : billSubjectFoot.value
  if (!activeRows.length) { MessagePlugin.warning('当前无数据可导出'); return }
  if (exporting.value) return
  exporting.value = true
  try {
    const XLSX = await import('xlsx')
    const header = activeColumns.map((c) => c.title)
    const body = [...activeRows, ...footRows].map((r) =>
      activeColumns.map((c) => String((r as any)[c.colKey] ?? '')))
    const sheet = XLSX.utils.aoa_to_sheet([header, ...body])
    sheet['!cols'] = header.map((h) => ({ wch: Math.max(10, String(h).length * 2 + 4) }))
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, sheet, isSummary ? '汇总数据' : (billDetailTab.value === 'unit' ? '单位明细' : '个人明细'))
    XLSX.writeFile(wb, `${billDetailTitle.value}.xlsx`)
    MessagePlugin.success('已导出 Excel')
  } catch (e: any) { MessagePlugin.error(e?.message || '导出 Excel 失败') }
  finally { exporting.value = false }
}


// ---------------------------------------------------------------------------
// 第 61 轮：宿舍管理交互
// ---------------------------------------------------------------------------
/** 五标签切换：与社保四标签同款「先撤表头再一次性成型」。 */
async function onDormViewChange() {
  headerReady.value = false
  rows.value = []
  selectedKeys.value = new Set()
  filterState.value = { keyword: '' }
  initColumns()
  await Promise.all([loadFieldConfig('hr-dorm'), loadList('hr-dorm')])
  initColumns()
  headerReady.value = true
}

async function downloadDormTemplate(): Promise<Blob> {
  const blob = await hrApi.getDormTemplate(hrDormModuleOf(dormSubView.value))
  return new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
}

async function runDormImport(files: File[], mode = 'merge'): Promise<number> {
  const fd = new FormData()
  fd.append('file', files[0])
  fd.append('mode', mode)
  const res: any = await hrApi.importDormRecords(hrDormModuleOf(dormSubView.value), fd, mode)
  const d = res?.data || res || {}
  await onDormImported()
  return Number(d.imported || 0)
}

async function onDormImported() {
  await Promise.all([loadList('hr-dorm'), reloadStats()])
}

// ---- 费用扣款明细弹窗（第 62 轮：全员 / 员工宿舍水电 / 公租房 / 部门汇总）----
const dormMoney = (v: any) => {
  const n = Number(String(v ?? '').replace(/,/g, ''))
  if (!Number.isFinite(n)) return ''
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
const dormNum = (v: any) => {
  const n = Number(String(v ?? '').replace(/,/g, ''))
  return Number.isFinite(n) ? String(Math.round(n)) : ''
}

const DORM_PERSONAL_COLUMNS = hrDormChargeColumns().filter((c: any) => c.colKey !== 'row-select')
  .map((c: any) => ({ colKey: c.colKey, title: c.title, width: c.width || 110 }))
const DORM_UTIL_COLUMNS = [
  { colKey: '宿舍号', title: '宿舍号', width: 160 },
  { colKey: '姓名', title: '姓名', width: 90 },
  { colKey: '入住天数', title: '入住天数', width: 90, align: 'right' as const },
  { colKey: '分摊比例', title: '分摊比例', width: 90, align: 'right' as const },
  { colKey: '电起度', title: '电起度', width: 90, align: 'right' as const },
  { colKey: '电止度', title: '电止度', width: 90, align: 'right' as const },
  { colKey: '电费', title: '电费(元)', width: 100, align: 'right' as const },
  { colKey: '水起度', title: '水起度', width: 90, align: 'right' as const },
  { colKey: '水止度', title: '水止度', width: 90, align: 'right' as const },
  { colKey: '水费', title: '水费(元)', width: 100, align: 'right' as const },
  { colKey: '合计', title: '水电合计(元)', width: 120, align: 'right' as const },
]
const DORM_PUBLIC_COLUMNS = [
  { colKey: '宿舍号', title: '房屋', width: 190 },
  { colKey: '姓名', title: '姓名', width: 90 },
  { colKey: '公租房租(元)', title: '房租(元)', width: 110, align: 'right' as const },
  { colKey: '水费(元)', title: '水费(元)', width: 100, align: 'right' as const },
  { colKey: '电费(元)', title: '电费(元)', width: 100, align: 'right' as const },
  { colKey: '天然气费(元)', title: '天然气费(元)', width: 120, align: 'right' as const },
  { colKey: '宿舍押金(元)', title: '押金(元)', width: 100, align: 'right' as const },
  { colKey: '扣款合计(元)', title: '合计(元)', width: 120, align: 'right' as const },
]
const DORM_DEPT_COLUMNS = [
  { colKey: 'department', title: '部门', width: 160 },
  { colKey: 'headcount', title: '人数', width: 90, align: 'right' as const },
  { colKey: 'housing', title: '住宿费(元)', width: 130, align: 'right' as const },
  { colKey: 'utility', title: '水电费(元)', width: 130, align: 'right' as const },
  { colKey: 'social', title: '社保(元)', width: 130, align: 'right' as const },
  { colKey: 'fund', title: '公积金(元)', width: 130, align: 'right' as const },
  { colKey: 'other', title: '其它(元)', width: 110, align: 'right' as const },
  { colKey: 'total', title: '合计(元)', width: 130, align: 'right' as const },
]
const dormPersonalColumns = DORM_PERSONAL_COLUMNS
const dormUtilColumns = DORM_UTIL_COLUMNS
const dormPublicColumns = DORM_PUBLIC_COLUMNS
const dormDeptColumns = DORM_DEPT_COLUMNS

const dormPersonalRows = computed(() => dormBillItems.value.map((r: any, i: number) => {
  const out: Record<string, any> = { id: r.id || `c-${i}` }
  for (const f of HR_DORM_FIELDS.fee) {
    const v = r[f.name]
    if (v === undefined || v === null || v === '') { out[f.name] = ''; continue }
    out[f.name] = HR_DORM_INTEGER_FIELDS.has(f.name) ? dormNum(v)
      : (f.type === 'number' ? dormMoney(v) : String(v))
  }
  return out
}))
const dormDormRows = computed(() => dormBillItems.value
  .filter((r: any) => String(r.room_type || '') !== '公租房' && String(r['宿舍号'] || r.room_no || '').trim() !== '')
  .map((r: any, i: number) => ({
    id: `du-${i}`,
    宿舍号: r['宿舍号'] || r.room_no || '',
    姓名: r['姓名'] || r.name || '',
    入住天数: dormNum(r['入住天数']),
    分摊比例: dormMoney(r['分摊比例']),
    电起度: dormNum(r.elec_start ?? 0),
    电止度: dormNum(r.elec_end ?? 0),
    电费: dormMoney(r['电费(元)']),
    水起度: dormNum(r.water_start ?? 0),
    水止度: dormNum(r.water_end ?? 0),
    水费: dormMoney(r['水费(元)']),
    合计: dormMoney(Number(r['电费(元)'] || 0) + Number(r['水费(元)'] || 0)),
  })))
const dormPublicRows = computed(() => dormBillItems.value
  .filter((r: any) => String(r.room_type || '') === '公租房')
  .map((r: any, i: number) => ({
    id: `dp-${i}`,
    宿舍号: r['宿舍号'] || r.room_no || '',
    姓名: r['姓名'] || r.name || '',
    '公租房租(元)': dormMoney(r['公租房租(元)']),
    '水费(元)': dormMoney(r['水费(元)']),
    '电费(元)': dormMoney(r['电费(元)']),
    '天然气费(元)': dormMoney(r['天然气费(元)']),
    '宿舍押金(元)': dormMoney(r['宿舍押金(元)']),
    '扣款合计(元)': dormMoney(r['扣款合计(元)']),
  })))
const dormDeptRows = computed(() => {
  const map = new Map<string, any>()
  for (const r of dormBillItems.value) {
    const d = String(r['所属部门'] || r.department || '未分配部门').trim() || '未分配部门'
    const it = map.get(d) || { department: d, headcount: 0, housing: 0, utility: 0, social: 0, fund: 0, other: 0, total: 0 }
    it.headcount += 1
    it.housing = Math.round((it.housing + Number(r['宿舍押金(元)'] || 0) + Number(r['公租房租(元)'] || 0)) * 100) / 100
    it.utility = Math.round((it.utility + Number(r['水费(元)'] || 0) + Number(r['电费(元)'] || 0) + Number(r['天然气费(元)'] || 0)) * 100) / 100
    it.social = Math.round((it.social + Number(r['社保个人扣款(元)'] || 0)) * 100) / 100
    it.fund = Math.round((it.fund + Number(r['公积金个人扣款(元)'] || 0)) * 100) / 100
    it.other = Math.round((it.other + Number(r['其它扣款(元)'] || 0)) * 100) / 100
    it.total = Math.round((it.total + Number(r['扣款合计(元)'] || 0)) * 100) / 100
    map.set(d, it)
  }
  return [...map.values()]
    .sort((a, b) => a.department.localeCompare(b.department))
    .map((it) => ({
      department: it.department,
      headcount: String(it.headcount),
      housing: dormMoney(it.housing),
      utility: dormMoney(it.utility),
      social: dormMoney(it.social),
      fund: dormMoney(it.fund),
      other: dormMoney(it.other),
      total: dormMoney(it.total),
    }))
})
const dormUtilRows = dormDormRows
const dormBillSum = (rows: any[], key: string) => rows.reduce((n, r) => n + (Number(String(r[key] ?? '').replace(/,/g, '')) || 0), 0)

const dormPersonalFoot = computed(() => (dormPersonalRows.value.length ? [{
  账单月份: `合计 ${dormPersonalRows.value.length} 人`, 员工工号: '', 姓名: '', 所属部门: '', 宿舍号: '',
  入住天数: '', 分摊比例: '',
  '宿舍押金(元)': dormMoney(dormBillSum(dormPersonalRows.value, '宿舍押金(元)')),
  '公租房租(元)': dormMoney(dormBillSum(dormPersonalRows.value, '公租房租(元)')),
  '水费(元)': dormMoney(dormBillSum(dormPersonalRows.value, '水费(元)')),
  '电费(元)': dormMoney(dormBillSum(dormPersonalRows.value, '电费(元)')),
  '天然气费(元)': dormMoney(dormBillSum(dormPersonalRows.value, '天然气费(元)')),
  '社保个人扣款(元)': dormMoney(dormBillSum(dormPersonalRows.value, '社保个人扣款(元)')),
  '公积金个人扣款(元)': dormMoney(dormBillSum(dormPersonalRows.value, '公积金个人扣款(元)')),
  '其它扣款(元)': dormMoney(dormBillSum(dormPersonalRows.value, '其它扣款(元)')),
  '扣款合计(元)': dormMoney(dormBillSum(dormPersonalRows.value, '扣款合计(元)')),
  '退款(元)': dormMoney(dormBillSum(dormPersonalRows.value, '退款(元)')),
  '补贴合计(元)': dormMoney(dormBillSum(dormPersonalRows.value, '补贴合计(元)')),
  备注: '', 账单状态: '',
}] : []))
const dormUtilFoot = computed(() => (dormDormRows.value.length ? [{
  宿舍号: `合计 ${dormDormRows.value.length} 人`, 姓名: '', 入住天数: '', 分摊比例: '',
  电起度: '', 电止度: '', 电费: dormMoney(dormBillSum(dormDormRows.value, '电费')),
  水起度: '', 水止度: '', 水费: dormMoney(dormBillSum(dormDormRows.value, '水费')),
  合计: dormMoney(dormBillSum(dormDormRows.value, '合计')),
}] : []))
const dormPublicFoot = computed(() => (dormPublicRows.value.length ? [{
  宿舍号: `合计 ${dormPublicRows.value.length} 人`, 姓名: '',
  '公租房租(元)': dormMoney(dormBillSum(dormPublicRows.value, '公租房租(元)')),
  '水费(元)': dormMoney(dormBillSum(dormPublicRows.value, '水费(元)')),
  '电费(元)': dormMoney(dormBillSum(dormPublicRows.value, '电费(元)')),
  '天然气费(元)': dormMoney(dormBillSum(dormPublicRows.value, '天然气费(元)')),
  '宿舍押金(元)': dormMoney(dormBillSum(dormPublicRows.value, '宿舍押金(元)')),
  '扣款合计(元)': dormMoney(dormBillSum(dormPublicRows.value, '扣款合计(元)')),
}] : []))
const dormDeptFoot = computed(() => (dormDeptRows.value.length ? [{
  department: `合计 ${dormDeptRows.value.length} 个部门`,
  headcount: String(dormDeptRows.value.reduce((n, r) => n + Number(r.headcount || 0), 0)),
  housing: dormMoney(dormBillSum(dormDeptRows.value, 'housing')),
  utility: dormMoney(dormBillSum(dormDeptRows.value, 'utility')),
  social: dormMoney(dormBillSum(dormDeptRows.value, 'social')),
  fund: dormMoney(dormBillSum(dormDeptRows.value, 'fund')),
  other: dormMoney(dormBillSum(dormDeptRows.value, 'other')),
  total: dormMoney(dormBillSum(dormDeptRows.value, 'total')),
}] : []))

async function openDormBill(row: any) {
  const month = String(row?.账单月份 || row?.month_label || dormMonthFilter.value || '')
  if (!month) return
  dormBillTitle.value = `${month} 费用扣款明细`
  dormBillTab.value = 'personal'
  dormBillItems.value = []
  dormBillVisible.value = true
  try {
    const res: any = await hrApi.listDormCharges({ month })
    const d = res?.data || res || {}
    dormBillItems.value = Array.isArray(d.items) ? d.items : []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载扣款明细失败')
    dormBillVisible.value = false
  }
}

const dormActiveBill = computed(() => {
  const t = dormBillTab.value
  if (t === 'dorm') return { cols: dormUtilColumns, rows: dormUtilRows.value, foot: dormUtilFoot.value, name: '员工宿舍水电分摊' }
  if (t === 'public') return { cols: dormPublicColumns, rows: dormPublicRows.value, foot: dormPublicFoot.value, name: '公租房水电气房租' }
  if (t === 'dept') return { cols: dormDeptColumns, rows: dormDeptRows.value, foot: dormDeptFoot.value, name: '部门汇总' }
  return { cols: dormPersonalColumns, rows: dormPersonalRows.value, foot: dormPersonalFoot.value, name: '全员扣款明细' }
})

async function exportDormBill() {
  const { cols, rows, foot, name } = dormActiveBill.value
  if (!rows.length) { MessagePlugin.warning('当前无数据可导出'); return }
  if (exporting.value) return
  exporting.value = true
  try {
    const XLSX = await import('xlsx')
    const header = cols.map((c: any) => c.title)
    const body = [...rows, ...foot].map((r: any) => cols.map((c: any) => String(r[c.colKey] ?? '')))
    const sheet = XLSX.utils.aoa_to_sheet([header, ...body])
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, sheet, name)
    XLSX.writeFile(wb, `${dormBillTitle.value}.xlsx`)
    MessagePlugin.success('已导出 Excel')
  } catch (e: any) { MessagePlugin.error(e?.message || '导出 Excel 失败') }
  finally { exporting.value = false }
}

async function printDormBill() {
  const { cols, rows, foot, name } = dormActiveBill.value
  if (!rows.length) { MessagePlugin.warning('当前无数据可打印'); return }
  printBusy.value = true
  try {
    billPrintData.value = await generateCatalogPdf({
      title: `${dormBillTitle.value} · ${name}`,
      columns: cols.map((c: any) => ({ key: c.colKey, label: c.title, value: (r: any) => String(r[c.colKey] ?? '') })),
      rows: [...rows, ...foot],
    })
    if (billPrintUrl.value) URL.revokeObjectURL(billPrintUrl.value)
    billPrintUrl.value = URL.createObjectURL(
      new Blob([billPrintData.value as unknown as BlobPart], { type: 'application/pdf' }),
    )
    billPrintTitle.value = `${dormBillTitle.value} · ${name}`
    billPrintVisible.value = true
  } catch (e: any) { MessagePlugin.error(e?.message || '生成打印文件失败') }
  finally { printBusy.value = false }
}

// ---- 住宿联动：员工 / 房源 / 床位 / 费用项目自动带出 ----
const dormEmployeeNameOptions = computed(() => dormEmployeeList.value.map((e) => ({
  label: String(e.name || e['姓名'] || ''), value: String(e.name || e['姓名'] || ''),
})))
const dormUnitOptions = ['重庆星达铜业有限公司', '生产部', '技术质量部', '财务部', '行政部', '销售部']
  .map((v) => ({ label: v, value: v }))
const dormEmployeeOptions = computed(() => dormEmployeeList.value.map((e) => ({
  label: `${e.name || e['姓名']}（${e.employee_id || e['工号'] || ''}）`,
  value: e.name || e['姓名'],
})))
const dormRoomSelectOptions = computed(() => dormRoomList.value.map((r) => ({
  label: r['房屋地址'] || r['宿舍编号'] || r.code || '',
  value: r['房屋地址'] || r['宿舍编号'] || r.code || '',
})))
const dormTransferBedOptions = computed(() => {
  const room = dormRoomList.value.find((r) => (r['房屋地址'] || r['宿舍编号'] || r.code) === dormTransferRoom.value)
  const beds = Number(room?.['床位数'] || 0)
  const used = Number(room?.在住人数 || 0)
  const free = Math.max(0, beds - used)
  return Array.from({ length: free }, (_, i) => ({
    label: `${i + used + 1}号床`, value: `${i + used + 1}号床`,
  }))
})

/** 住宿记录：可选房源（过滤满员 + 性别互斥，夫妻房不限）。 */
const dormAvailableRoomOptions = computed(() => {
  const d = editForm.value?.data || {}
  const name = String(d['员工姓名'] || '').trim()
  const emp = dormEmployeeList.value.find((e) => (e.name || e['姓名']) === name)
  const empGender = String(emp?.gender || emp?.['性别'] || '').trim()
  return dormRoomList.value
    .filter((r) => Number(r['可用床位数'] ?? 1) > 0)
    .filter((r) => {
      if (String(r['房型'] || '') === '夫妻房') return true
      const limit = String(r['性别限制'] || '').trim()
      if (!empGender || !limit || limit === '不限') return true
      return limit === empGender
    })
    .map((r) => ({
      label: `${r['房屋地址'] || r['宿舍编号'] || r.code}（可用 ${r['可用床位数'] ?? 0}）`,
      value: r['房屋地址'] || r['宿舍编号'] || r.code || '',
    }))
})

async function loadDormHelpers() {
  if (dormRoomList.value.length && dormEmployeeList.value.length) return
  try {
    const [d, p, a, roster] = await Promise.all([
      hrApi.listDormRecords({ module: 'dorm' }),
      hrApi.listDormRecords({ module: 'public' }),
      hrApi.listDormRecords({ module: 'asset' }),
      hrApi.listRosterEmployees({ status: '在职', page_size: 500 }),
    ])
    dormRoomList.value = [...extractItems(d), ...extractItems(p)]
    const names = new Set<string>()
    for (const it of extractItems(a)) {
      String(it['资产名称'] || '').split(',').forEach((n: string) => { const v = n.trim(); if (v) names.add(v) })
    }
    dormFacilityOptions.value = [...names]
    dormEmployeeList.value = extractItems(roster)
  } catch { /* 联动失败不阻断抽屉 */ }
}

/** 房型变化 → 床位数自动设置（只读整数）。 */
function onDormRoomTypeChange(v: any) {
  const type = String(v ?? '')
  editForm.value.data['床位数'] = String(hrDormBedsOfType(type))
  if (type === '夫妻房') editForm.value.data['性别限制'] = '不限'
}

/** 选员工 → 带出工号 / 部门 / 电话 / 紧急联系人（只读）。 */
function onStayEmployeeChange(v: any) {
  const name = String(v ?? '').trim()
  const emp = dormEmployeeList.value.find((e) => (e.name || e['姓名']) === name)
  if (!emp) return
  const d = editForm.value.data
  d['员工姓名'] = name
  d['员工工号'] = String(emp.employee_id || emp['工号'] || '')
  d['所属部门'] = String(emp.department || emp['部门'] || '')
  d['联系电话'] = String(emp.phone || emp['手机号码'] || '')
  d['紧急联系人及电话'] = [emp.emergency_contact, emp.emergency_phone].filter(Boolean).join(' / ')
  // 已选房源不满足性别/满员条件时清空
  const still = dormAvailableRoomOptions.value.some((o) => o.value === d['宿舍编号/房屋地址'])
  if (!still) d['宿舍编号/房屋地址'] = ''
}

/** 选房源 → 带出房源类型 / 空闲床位 / 费用项目只读值。 */
function onStayRoomChange(v: any) {
  const code = String(v ?? '').trim()
  const room = dormRoomList.value.find((r) => (r['房屋地址'] || r['宿舍编号'] || r.code) === code)
  const d = editForm.value.data
  d['宿舍编号/房屋地址'] = code
  if (!room) return
  d['房源类型'] = String(room['房屋地址'] ? '公租房' : '公司宿舍')
  const used = Number(room.在住人数 || 0)
  d['床位号'] = `${used + 1}号床`
  if (!String(d['入住日期'] || '').trim()) d['入住日期'] = new Date().toISOString().slice(0, 10)
  if (!String(d['入住类型'] || '').trim()) d['入住类型'] = '新入住'
  d['房屋租金'] = String(room['房屋租金(元/月)'] ?? '')
  d['物业费'] = String(room['物业费(元/月)'] ?? '')
  d['季度租金'] = String(room['季度租金(元/季)'] ?? '')
  d['电费标准'] = String(room['电费标准(元/度)'] ?? '')
  d['水费单价'] = String(room['水费单价(元/m³)'] ?? '')
  d['垃圾处理费'] = String(room['垃圾处理费(元/套)'] ?? '')
  d['二次供水费'] = String(room['二次供水费(元/m³)'] ?? '')
  d['气费标准'] = String(room['气费标准(元/m³)'] ?? '')
  if (!String(d['押金金额(元)'] || '').trim()) d['押金金额(元)'] = String(room['宿舍押金(元)'] ?? '')
}

// 第 62 轮：住宿抽屉里「宿舍编号/房屋地址」用抽屉内置下拉（含斜杠不能开插槽），
// 这里监听草稿值变化，自动带出房源类型 / 空闲床位 / 费用项目。
watch(
  () => editForm.value?.data?.['宿舍编号/房屋地址'],
  (v, old) => {
    if (activeTab.value !== 'hr-dorm' || dormSubView.value !== 'stay') return
    if (!v || v === old) return
    onStayRoomChange(v)
  },
)

// ---- 办理调宿 / 退宿 ----
function openDormTransfer() {
  if (!selectedRowKeys.value.length) { MessagePlugin.warning('请先勾选住宿记录'); return }
  dormTransferRoom.value = ''
  dormTransferBed.value = ''
  dormTransferVisible.value = true
}

async function confirmDormTransfer() {
  if (!dormTransferRoom.value.trim()) { MessagePlugin.warning('请填写目标房源'); return }
  try {
    await hrApi.transferDormStays({ ids: selectedRowKeys.value, room: dormTransferRoom.value.trim(), bed: dormTransferBed.value })
    MessagePlugin.success('已办理调宿')
    dormTransferVisible.value = false
    clearSelection()
    await loadList('hr-dorm')
  } catch (e: any) { MessagePlugin.error(e?.message || '办理调宿失败') }
}

function openDormCheckout() {
  if (!selectedRowKeys.value.length) { MessagePlugin.warning('请先勾选住宿记录'); return }
  dormCheckoutDate.value = new Date().toISOString().slice(0, 10)
  dormCheckoutReason.value = ''
  dormCheckoutVisible.value = true
}

async function confirmDormCheckout() {
  try {
    await hrApi.checkoutDormStays({
      ids: selectedRowKeys.value,
      date: dormCheckoutDate.value,
      reason: dormCheckoutReason.value,
    })
    MessagePlugin.success('已办理退宿')
    dormCheckoutVisible.value = false
    clearSelection()
    await loadList('hr-dorm')
  } catch (e: any) { MessagePlugin.error(e?.message || '办理退宿失败') }
}

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------
onMounted(async () => {
  // 第 53 轮：公司设置弹窗（左下角用户菜单）保存后广播事件，
  // 人事页据此重拉部门/单位下拉、列表与卡片。
  window.addEventListener('hr-org-changed', onOrgChanged)
  void loadRosterDepts()
  initColumns()
  await switchTab('hr-roster')
  await reloadStats()
  filterWatchReady = true
})
onUnmounted(() => {
  window.removeEventListener('hr-org-changed', onOrgChanged)
  // 第 58 轮：页面卸载时释放全部 blob URL（附件预览 / 清单 PDF / 账单 PDF）
  releasePreviewUrl()
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
  if (billPrintUrl.value) { URL.revokeObjectURL(billPrintUrl.value); billPrintUrl.value = '' }
  if (filterTimer) clearTimeout(filterTimer)
})
</script>

<style lang="less" scoped>
/* 中台共享列表样式统一在文末的**非 scoped** 块 @import：
   scoped 会给每条规则加 [data-v-xxx]，而 TDesign 内部渲染的
   .t-table__content / tbody / tfoot 不带该属性，选择器永远命中不到。
   这里只放页面自用样式。 */
.hr-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  flex: 1;
}
.overview-group { flex: 0 0 auto; }
.biz-list-view { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 8px; }

.hr-subview-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 2px 0;
  flex: 0 0 auto;
  /* 第 57 轮：子标签栏与下方搜索工具栏统一留 14px，消除紧贴局促感 */
  margin-bottom: 14px;
}
.hr-unit-config {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
/* 批量按钮外层保留可见盒（真机 data-hr-batch-bar 断言可读性），
   内部按钮随浮条换行；不设固定宽度，避免撑破浮动条。 */
.hr-batch-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.hr-empty-placeholder { flex: 1; min-height: 0; }
.hr-file-input { display: none; }
/* 搜索框正后方的在职状态筛选（第 46 轮） */
.hr-status-select { width: 140px; flex-shrink: 0; }
.hr-month-picker { width: 130px; flex-shrink: 0; }
/* 第 57 轮：离职证明附件控制条 —— 状态标签 + 查看 + 重新上传/上传附件，
   所有按钮强制 nowrap，绝不竖排折行；附件路径不再出现在界面上。 */
.hr-proof-field {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  flex-wrap: nowrap;
  white-space: nowrap;
  min-width: 0;
}
.hr-proof-field .hr-proof-tag { flex: 0 0 auto; white-space: nowrap; }
.hr-proof-field .hr-proof-empty {
  flex: 0 0 auto;
  color: var(--td-text-color-placeholder);
  font-size: var(--td-font-size-body-small);
  white-space: nowrap;
}
.hr-proof-field .hr-proof-btn { flex: 0 0 auto; white-space: nowrap; }

/* 回盘表五险参保状态三色字体（第 57 轮；第 58 轮「暂停缴费」改走设计令牌，
   深色主题下不再是一块看不见的 #1d2129） */
.hr-si-status { white-space: nowrap; font-variant-numeric: tabular-nums; }
.hr-si-status--active { color: #00a870; }
.hr-si-status--ended { color: #ed7b2f; }
.hr-si-status--paused { color: var(--td-text-color-primary, #1d2129); }
/* 补退类型 / 记录类型：正常绿、补缴/退费/调基/异常红（第 58 轮） */
.hr-si-adjust { white-space: nowrap; font-variant-numeric: tabular-nums; }
.hr-si-adjust--ok { color: #00a870; }
.hr-si-adjust--bad { color: #e34d59; }

/* 第 57 轮：账单三标签弹窗 */
.hr-bill-detail { display: flex; flex-direction: column; gap: 12px; min-height: 0; }
.hr-bill-tabs { display: flex; align-items: center; flex: 0 0 auto; margin-bottom: 2px; }
.hr-bill-filters { margin-bottom: 14px; }
.hr-bill-table-wrap { flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; }
.hr-bill-table-wrap > * { flex: 1 1 auto; min-height: 0; }
</style>

<style>
/* 共享列表样式必须在**非 scoped** 块再引入一次：TDesign 内部的
   .t-table__content / .t-table__body / tfoot 等元素拿不到当前组件的 data-v，
   scoped 版本的选择器命中不到它们（发票 / 车队页同款做法）。 */
@import './styles/business-list-shared.css';
</style>

<style>
/* 第 55 轮：月度账单详情抽屉（非 scoped，命中 TDesign 内部节点） */
.hr-bill-detail { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
.hr-bill-filters { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; flex: 0 0 auto; }
.hr-bill-search { width: 240px; }
.hr-bill-select { width: 160px; }
.hr-bill-spacer { flex: 1; }
.hr-bill-count { color: var(--td-text-color-secondary); font-size: 13px; }
.hr-bill-summary-title {
  font-size: 13px; font-weight: 600; color: var(--td-text-color-primary);
  padding-left: 8px; border-left: 3px solid var(--td-success-color); flex: 0 0 auto;
}
.hr-bill-summary-table, .hr-bill-item-table { flex: 0 0 auto; }
.hr-bill-item-table { flex: 1; min-height: 0; }
/* 第 58 轮：滚动条只占右侧 gutter，绝不覆盖表头/合计行的文字；
   表头 sticky（由 sticky-header 提供），合计行粘底。 */
/* 第 61 轮：三段结构自带滚动条槽位，旧的 t-table sticky 规则删除 */
.hr-bill-detail .detail-table-body::-webkit-scrollbar { width: 8px; height: 8px; }
.hr-bill-footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.hr-bill-month-row { display: flex; align-items: center; gap: 12px; padding: 8px 0; }
.hr-bill-month-row > span { white-space: nowrap; min-width: 80px; }
.hr-bill-grow { flex: 1; min-width: 260px; }
.hr-dorm-dialog .t-form__label { white-space: nowrap; min-width: 96px; }
.hr-bill-confirm { padding: 8px 0; line-height: 1.7; color: var(--td-text-color-primary); }
</style>
