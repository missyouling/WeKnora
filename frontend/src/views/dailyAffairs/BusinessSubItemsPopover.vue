<!--
  BusinessSubItemsPopover —— 明细子项浮层中台组件（蓝图 P1-4）。

  来源：InvoiceManagement.vue 税率列的「项目明细」tooltip。合同/制度/奖惩等
  模块同样有「一条记录 N 个条目」的形态，浮层逻辑（宽度自适应、取值兜底、
  数值/税率格式化）完全通用，没必要每个页面抄一份。

  用法：
    <BusinessSubItemsPopover :items="row.items" :sub-fields="enabledSubFieldsOfRow(row)">
      <span class="row-mono">{{ rateTextOf(row) }}</span>
    </BusinessSubItemsPopover>

  契约：
    - subFields 传**全部**子列，组件内部只渲染 `enabled !== false` 的；
    - 取值：标准列键取行属性，自定义子列回退 `row.extra[key]`；
    - 宽度 = 启用列宽之和 + 20px 内距，夹在 [minWidth, maxWidth]。

  样式为什么用 :global：TDesign 的 tooltip 把内容渲染到 body 下，
  scoped 的属性选择器命中不到 popup 容器，只能按 overlay class 全局声明。
-->
<template>
  <t-tooltip placement="top" theme="light" :overlay-class-name="overlayClassName" :show-arrow="true">
    <template #content>
      <div class="biz-sub" :style="{ width: popWidth + 'px' }">
        <table v-if="rows.length" class="biz-sub-table">
          <thead>
            <tr>
              <th v-for="sf in cols" :key="sf.key" :style="colStyle(sf)">{{ sf.label || sf.key }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(it, i) in rows" :key="i">
              <td v-for="sf in cols" :key="sf.key" :class="itemCellClass(sf)" :style="colStyle(sf)">
                {{ formatItemCell(it, sf) }}
              </td>
            </tr>
          </tbody>
          <!--
            合计行（第 24 轮任务三）：票面明细下方本来就有 `合 计 | ¥金额 | ¥税额`，
            浮层不显示合计时用户没法当场核对「明细加起来是不是票面那个数」。
            口径见 sumOfAmountKey / sumOfTaxKey：
              · 首列固定写「合计」；
              · 金额列 / 税额列 = 所有明细行的精确汇总（按「分」整数累加，避免浮点尾巴）；
              · 其余列（规格型号/单位/单价/税率…）留空 —— 它们不可累加，
                写 0 或空串都容易被误读成「合计值为 0」。
          -->
          <tfoot v-if="rows.length && hasSumCols">
            <tr class="biz-sub-total">
              <td v-for="(sf, i) in cols" :key="sf.key" :class="itemCellClass(sf)" :style="colStyle(sf)">
                {{ i === 0 ? '合计' : sumCellOf(sf) }}
              </td>
            </tr>
          </tfoot>
        </table>
        <div v-else class="biz-sub-empty">{{ emptyText }}</div>
      </div>
    </template>
    <slot />
  </t-tooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  enabledSubFields,
  formatItemCell,
  itemCellClass,
  subItemPopoverWidth,
  ITEM_POP_MAX,
  ITEM_POP_MIN,
} from './businessFormat'
import { hasSumColumn, sumTextOfColumn } from './itemTotals'

const props = withDefaults(defineProps<{
  /** 明细行数组（空/未传 → 展示空态） */
  items?: any[]
  /** 子列定义：{ key, label, dataType, width, enabled }；内部按 enabled 过滤 */
  subFields?: any[]
  minWidth?: number
  maxWidth?: number
  emptyText?: string
  /** 浮层容器 class：父组件若已有全局皮肤可覆盖 */
  overlayClassName?: string
}>(), {
  items: () => [],
  subFields: () => [],
  minWidth: ITEM_POP_MIN,
  maxWidth: ITEM_POP_MAX,
  emptyText: '暂无明细',
  overlayClassName: 'biz-sub-popover',
})

const rows = computed<any[]>(() => (Array.isArray(props.items) ? props.items : []))
const cols = computed<any[]>(() => enabledSubFields(props.subFields))
const popWidth = computed(() => subItemPopoverWidth(cols.value, props.minWidth, props.maxWidth))
const colStyle = (sf: any) => (Number(sf?.width) > 0 ? { width: Number(sf.width) + 'px' } : undefined)

// ---- 合计行（第 24 轮任务三）----
// 判据与求和都在 itemTotals：浮层与抽屉**必须完全一致**，
// 各写一份就是「浮窗 51989.49、抽屉 51989.48」这类分叉的源头。
const hasSumCols = computed(() => hasSumColumn(cols.value))
const sumCellOf = (sf: any) => sumTextOfColumn(rows.value, sf)
</script>

<style scoped>
/* popup 容器渲染在 body 下：必须走 :global，且宽度由内层 .biz-sub 行内 width 决定 */
:global(.biz-sub-popover) { max-width: 840px; }
:global(.biz-sub-popover .t-popup__content) {
  max-width: 840px !important;
  width: max-content;
  min-width: 0;
}
:global(.biz-sub-popover .biz-sub-table) {
  border-collapse: collapse;
  font-size: 12px;
  line-height: 1.7;
  width: 100%;
  min-width: 0;
}
:global(.biz-sub-popover .biz-sub-table th),
:global(.biz-sub-popover .biz-sub-table td) {
  padding: 2px 10px;
  text-align: left;
  white-space: nowrap;
  border-bottom: 1px solid var(--td-component-stroke);
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
}
:global(.biz-sub-popover .biz-sub-table th) {
  font-weight: 500;
  color: var(--td-text-color-secondary);
}
:global(.biz-sub-popover .biz-sub-table td) { color: var(--td-text-color-primary); }
:global(.biz-sub-popover .biz-sub-table .item-cell--num) { text-align: right; }
/* 明细浮层里的多行文本子列：折行完整显示（与详情抽屉同一口径） */
:global(.biz-sub-popover .biz-sub-table .item-cell--multiline) {
  white-space: pre-wrap;
  word-break: break-word;
  overflow: visible;
  text-overflow: clip;
  max-width: 240px;
}
:global(.biz-sub-popover .biz-sub-empty) {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  padding: 4px 8px;
}
/* 合计行：加粗 + 上边框 + 略深背景，与明细行明确分层 */
:global(.biz-sub-popover .biz-sub-table .biz-sub-total td) {
  font-weight: 600;
  color: var(--td-text-color-primary);
  border-top: 1px solid var(--td-component-border);
  border-bottom: none;
  background: var(--td-bg-color-secondarycontainer);
}
:global(.biz-sub-popover .biz-sub-table .biz-sub-total td:first-child) {
  text-align: left;
}
</style>
