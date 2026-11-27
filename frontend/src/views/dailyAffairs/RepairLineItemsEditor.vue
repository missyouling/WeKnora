<!--
  维修结算清单「分项与细项」三子表编辑器（第 78 轮）。

  三张表对应票面三段：维修项目（工时）/ 零件材料 / 附加费用。
  组件只负责渲染与增删改，**汇总与钩稽由父级用 lineItems.ts 的纯函数算**
  —— 这样单测能直接覆盖金额口径，不必挂载组件。

  铁律：
    · 金额列右对齐 + 千分位（与全站金额列同口径）；
    · 空值一律空白，不渲染 `—`；
    · 删除行必须二次确认？—— 不：明细行是草稿态（抽屉未保存前不落库），
      误删后刷新即可恢复，加确认反而拖慢录入。
-->
<template>
  <div class="line-items" data-line-items>
    <!-- 维修项目明细 -->
    <div class="li-block">
      <div class="li-head">
        <span class="li-title">维修项目明细</span>
        <span class="li-count">{{ model.labor_items.length }} 项</span>
        <span class="li-sum">工时费合计 {{ money(totals.labor) }}</span>
        <t-button variant="text" size="small" data-li-add="labor" @click="addLabor">+ 添加项目</t-button>
      </div>
      <table class="li-table">
        <thead>
          <tr>
            <th class="c-name">维修项目</th>
            <th class="c-num">工时</th>
            <th class="c-num">工时费</th>
            <th class="c-mid">维修技师</th>
            <th class="c-remark">备注</th>
            <th class="c-act"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(it, i) in model.labor_items" :key="`l${i}`">
            <td><t-input v-model="it.item_name" size="small" placeholder="项目名称" /></td>
            <td><t-input-number v-model="it.hours" size="small" :min="0" :decimal-places="2"
              :allow-input="false" theme="normal" /></td>
            <td><t-input-number v-model="it.labor_fee" size="small" :min="0" :decimal-places="2"
              :allow-input="false" :format="trimMoney" theme="normal" /></td>
            <td><t-input v-model="it.technician" size="small" placeholder="技师" /></td>
            <td><t-input v-model="it.remark" size="small" placeholder="备注" /></td>
            <td class="c-act">
              <button type="button" class="li-del" title="删除该行" @click="removeLabor(i)">
                <t-icon name="delete" size="14px" />
              </button>
            </td>
          </tr>
          <tr v-if="!model.labor_items.length">
            <td colspan="6" class="li-empty"></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 零件材料明细 -->
    <div class="li-block">
      <div class="li-head">
        <span class="li-title">零件材料明细</span>
        <span class="li-count">{{ model.part_items.length }} 件</span>
        <span class="li-sum">材料费合计 {{ money(totals.part) }}</span>
        <t-button variant="text" size="small" data-li-add="part" @click="addPart">+ 添加零件</t-button>
      </div>
      <table class="li-table">
        <thead>
          <tr>
            <th class="c-name">零件名称</th>
            <th class="c-unit">单位</th>
            <th class="c-num">数量</th>
            <th class="c-num">单价</th>
            <th class="c-num">金额</th>
            <th class="c-remark">备注</th>
            <th class="c-act"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(it, i) in model.part_items" :key="`p${i}`">
            <td><t-input v-model="it.part_name" size="small" placeholder="零件名称" /></td>
            <td><t-input v-model="it.unit" size="small" placeholder="个" /></td>
            <td><t-input-number v-model="it.qty" size="small" :min="0" :decimal-places="2"
              :allow-input="false" theme="normal" @change="syncPartAmount(it)" /></td>
            <td><t-input-number v-model="it.unit_price" size="small" :min="0" :decimal-places="2"
              :allow-input="false" :format="trimMoney" theme="normal" @change="syncPartAmount(it)" /></td>
            <td><t-input-number v-model="it.amount" size="small" :min="0" :decimal-places="2"
              :allow-input="false" :format="trimMoney" theme="normal" /></td>
            <td><t-input v-model="it.remark" size="small" placeholder="备注" /></td>
            <td class="c-act">
              <button type="button" class="li-del" title="删除该行" @click="removePart(i)">
                <t-icon name="delete" size="14px" />
              </button>
            </td>
          </tr>
          <tr v-if="!model.part_items.length">
            <td colspan="7" class="li-empty"></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 附加费用明细 -->
    <div class="li-block">
      <div class="li-head">
        <span class="li-title">附加费用明细</span>
        <span class="li-count">{{ model.extra_items.length }} 项</span>
        <span class="li-sum">其它费合计 {{ money(totals.extra) }}</span>
        <t-button variant="text" size="small" data-li-add="extra" @click="addExtra">+ 添加费用</t-button>
      </div>
      <table class="li-table">
        <thead>
          <tr>
            <th class="c-name">费用名称</th>
            <th class="c-num">金额</th>
            <th class="c-act"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(it, i) in model.extra_items" :key="`e${i}`">
            <td><t-input v-model="it.item_name" size="small" placeholder="如 税费" /></td>
            <td><t-input-number v-model="it.amount" size="small" :min="0" :decimal-places="2"
              :allow-input="false" :format="trimMoney" theme="normal" /></td>
            <td class="c-act">
              <button type="button" class="li-del" title="删除该行" @click="removeExtra(i)">
                <t-icon name="delete" size="14px" />
              </button>
            </td>
          </tr>
          <tr v-if="!model.extra_items.length">
            <td colspan="3" class="li-empty"></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { lineItemsTotals, type RepairExtraItem, type RepairLaborItem, type RepairLineItems, type RepairPartItem } from './lineItems'

/**
 * 直接用 `defineModel` 双向绑定整个 line_items 对象：
 * 父级（FleetRecordList）持有 `editForm.line_items`，保存时整体提交。
 * 子表行内的 v-model 直接改数组元素（对象引用），Vue 的深度响应能感知。
 */
const model = defineModel<RepairLineItems>({ required: true })

const totals = computed(() => lineItemsTotals(model.value))

const money = (n: number): string => (Number(n) || 0).toFixed(2)
/** 数字输入框去尾零显示（与抽屉金额字段同一口径）。 */
const trimMoney = (v: any) => {
  const s = String(v ?? '')
  if (!s) return ''
  return s.includes('.') ? s.replace(/0+$/, '').replace(/\.$/, '') : s
}

function addLabor() {
  const row: RepairLaborItem = { item_name: '', hours: 0, labor_fee: 0, technician: '', remark: '' }
  model.value = { ...model.value, labor_items: [...model.value.labor_items, row] }
}
function removeLabor(i: number) {
  model.value = { ...model.value, labor_items: model.value.labor_items.filter((_, k) => k !== i) }
}
function addPart() {
  const row: RepairPartItem = { part_name: '', unit: '', qty: 0, unit_price: 0, amount: 0, remark: '' }
  model.value = { ...model.value, part_items: [...model.value.part_items, row] }
}
function removePart(i: number) {
  model.value = { ...model.value, part_items: model.value.part_items.filter((_, k) => k !== i) }
}
function addExtra() {
  const row: RepairExtraItem = { item_name: '', amount: 0 }
  model.value = { ...model.value, extra_items: [...model.value.extra_items, row] }
}
function removeExtra(i: number) {
  model.value = { ...model.value, extra_items: model.value.extra_items.filter((_, k) => k !== i) }
}

/**
 * 数量或单价改动时自动补金额（qty × unit_price）。
 *
 * 只在金额为空或恰好等于旧乘积时才覆盖 —— 票面常有「让价/折扣」让
 * 金额 ≠ 数量×单价，用户手工填过的金额不能被自动改掉。
 */
function syncPartAmount(it: RepairPartItem) {
  const qty = Number(it.qty) || 0
  const price = Number(it.unit_price) || 0
  const calc = Math.round(qty * price * 100) / 100
  const cur = Number(it.amount) || 0
  if (cur === 0 || Math.abs(cur - calc) <= 0.01) it.amount = calc
}
</script>

<style lang="less" scoped>
.line-items { display: flex; flex-direction: column; gap: 12px; }

.li-block {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
}
.li-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: 13px;
}
.li-title { font-weight: 600; color: var(--td-text-color-primary); }
.li-count { color: var(--td-text-color-placeholder); font-size: 12px; }
.li-sum { margin-left: auto; color: var(--td-text-color-secondary); font-size: 12px; }

.li-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;

  th, td {
    padding: 4px 6px;
    font-size: 12px;
    text-align: left;
    border-bottom: 1px solid var(--td-component-stroke);
  }
  th { color: var(--td-text-color-secondary); font-weight: 500; background: var(--td-bg-color-container); }
  tbody tr:last-child td { border-bottom: none; }
  .c-name { width: 26%; }
  .c-num { width: 12%; }
  .c-mid { width: 14%; }
  .c-unit { width: 8%; }
  .c-remark { width: 20%; }
  .c-act { width: 34px; text-align: center; }
}
.li-empty { height: 26px; color: var(--td-text-color-placeholder); text-align: center; }

.li-del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: 3px;
  background: none;
  color: var(--td-text-color-placeholder);
  cursor: pointer;

  &:hover { background: var(--td-error-color-1); color: var(--td-error-color); }
}
</style>
