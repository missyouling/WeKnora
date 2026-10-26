<template>
  <!--
    抄表记录抽屉（电费/水费/气费核算共用）。

    为什么抽组件：三张核算表此前各自用自写 <t-drawer> + 手写 .rec-field 布局，
    与发票/合同的中台详情抽屉（BusinessSplitDetailDrawer：SettingDrawer 底座 +
    t-form-item 双列 + 统一底栏）风格割裂。表单结构 100% 一致（只有介质文案、
    水费附加费、分时四段三种差异），所以共用一份实现：
      · 底座换成 SettingDrawer（自带拖拽调宽/宽度持久化，与发票同手柄）；
      · 字段用 t-form-item 双列网格（.bsd-grid 同款栅格与控件右缘对齐）；
      · 分时/普通两种形态与校验提示全保留。

    状态契约：`form` 由父级持有并直接传入（对象引用共享，组件写属性即父级生效），
    月份/表计联动仍走父级 watch([form.month, form.meterId])，组件只透传 change 事件。
  -->
  <SettingDrawer :visible="visible" :title="title" width="654px" hide-footer destroy-on-close
    class="meter-record-drawer" :storage-key="`weknora-utility-${category}-record-drawer-width`"
    @update:visible="(v: boolean) => emit('update:visible', v)" @cancel="emit('close')">
    <div class="mr-grid">
      <t-form-item label="月份" label-width="96px">
        <t-date-picker :model-value="form.month" mode="month" format="YYYY-MM" value-type="YYYY-MM"
          placeholder="选择月份" @update:model-value="(v: any) => (form.month = String(v || ''))" />
        <p v-if="duplicateWarning" class="mr-error">该月该{{ meterLabel }}已有记录，可直接编辑</p>
      </t-form-item>
      <t-form-item label="抄表日期" label-width="96px">
        <t-date-picker :model-value="form.readingDate" format="YYYY-MM-DD" value-type="YYYY-MM-DD"
          placeholder="选择日期" clearable @update:model-value="(v: any) => (form.readingDate = String(v || ''))" />
      </t-form-item>
      <t-form-item label="使用单位" label-width="96px">
        <t-select :model-value="form.useUnit" :options="useUnitOptions" filterable :disabled="!!editingItemId"
          @update:model-value="(v: any) => (form.useUnit = String(v ?? ''))" @change="emit('use-unit-change')" />
      </t-form-item>
      <t-form-item :label="meterLabel" label-width="96px">
        <t-select :model-value="form.meterId" :placeholder="'选择' + meterLabel" :options="meterOptions" filterable
          @update:model-value="(v: any) => (form.meterId = String(v ?? ''))" @change="emit('meter-change')" />
        <p v-if="duplicateWarning" class="mr-error">该月该{{ meterLabel }}已有记录，可直接编辑</p>
      </t-form-item>

      <t-form-item label="抄表人" label-width="96px">
        <t-input :model-value="form.reader" placeholder="默认取表计管理人员"
          @update:model-value="(v: string) => (form.reader = v)" />
      </t-form-item>
      <t-form-item label="录入日期" label-width="96px">
        <div class="mr-readonly">{{ form.recordDate || today }}</div>
      </t-form-item>

      <template v-if="isTimeMeter">
        <div v-for="p in timePeriods" :key="p.key" class="mr-field-wide">
          <t-form-item :label="p.label" label-width="96px">
            <div class="mr-period-pair">
              <t-input :model-value="String(form[p.prevKey] ?? '')" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''"
                @update:model-value="(v: string) => (form[p.prevKey] = Number(v) || 0)" />
              <span class="mr-period-sep">→</span>
              <t-input :model-value="String(form[p.currKey] ?? '')" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''"
                @update:model-value="(v: string) => (form[p.currKey] = Number(v) || 0)" />
            </div>
            <p v-if="p.key === 'valley' && readingInvalid" class="mr-error">各时段止度不得小于起度</p>
          </t-form-item>
        </div>
      </template>
      <template v-else>
        <t-form-item label="起度" label-width="96px">
          <t-input :model-value="String(form.startReading ?? '')" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''"
            @update:model-value="(v: string) => (form.startReading = Number(v) || 0)" />
          <p v-if="readingInvalid" class="mr-error">止度不得小于起度</p>
        </t-form-item>
        <t-form-item label="止度" label-width="96px">
          <t-input :model-value="String(form.endReading ?? '')" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''"
            @update:model-value="(v: string) => (form.endReading = Number(v) || 0)" />
          <p v-if="readingInvalid" class="mr-error">止度不得小于起度</p>
        </t-form-item>
      </template>

      <t-form-item label="倍率" label-width="96px">
        <div class="mr-readonly">{{ fmtNum(formRate) }}×</div>
      </t-form-item>
      <t-form-item label="单价" label-width="96px">
        <t-input :model-value="String(form.unitPrice ?? '')" type="number" placeholder="单价" :status="unitPriceDiff ? 'error' : ''"
          @update:model-value="(v: string) => (form.unitPrice = Number(v) || 0)" />
        <p v-if="unitPriceDiff" class="mr-error">与配置默认单价 {{ fmtUnitPrice(defaultUnitPrice) }} 不同</p>
      </t-form-item>
      <template v-if="category === 'water'">
        <t-form-item label="垃圾处置费" label-width="96px">
          <t-input :model-value="String(form.garbageFee ?? '')" type="number" placeholder="默认 13 元/套"
            @update:model-value="(v: string) => (form.garbageFee = Number(v) || 0)" />
        </t-form-item>
        <t-form-item label="二次供水费" label-width="96px">
          <t-input :model-value="String(form.secondaryWaterFee ?? '')" type="number" placeholder="选填"
            @update:model-value="(v: string) => (form.secondaryWaterFee = Number(v) || 0)" />
        </t-form-item>
        <t-form-item label="污水处理费" label-width="96px">
          <t-input :model-value="String(form.sewageFee ?? '')" type="number" placeholder="选填"
            @update:model-value="(v: string) => (form.sewageFee = Number(v) || 0)" />
        </t-form-item>
      </template>
      <t-form-item label="补差" label-width="96px">
        <!-- type=number 会吞负号：文本输入 + 数字键盘，聚焦全选便于覆盖 -->
        <t-input :model-value="String(form.subsidy ?? '')" type="text" inputmode="decimal" placeholder="补差金额，可为负"
          @update:model-value="(v: string) => (form.subsidy = v)"
          @focus="(e: any) => { const t = e?.target || e; t?.select?.() }" />
      </t-form-item>
      <t-form-item v-if="!isTimeMeter" :label="categoryLabel" label-width="96px">
        <div class="mr-calc">{{ fmtMoney(formAmount) }} 元</div>
        <p class="mr-hint">自动计算 · {{ usageLabel }} {{ fmtNum(formUsage) }} {{ unitLabel }}</p>
      </t-form-item>
      <t-form-item v-else label="电费" label-width="96px">
        <div class="mr-readonly">按租户分摊核算</div>
        <p class="mr-hint">{{ usageLabel }} {{ fmtNum(formUsage) }} {{ unitLabel }}</p>
      </t-form-item>

      <div class="mr-field-wide">
        <t-form-item label="备注" label-width="96px">
          <t-textarea :model-value="form.remark" :maxlength="500" placeholder="选填"
            @update:model-value="(v: string) => (form.remark = v)" />
        </t-form-item>
      </div>
    </div>

    <div class="mr-footer">
      <slot name="footer-left" />
      <t-button variant="outline" size="small" @click="emit('close')">取消</t-button>
      <t-button theme="primary" size="small" :loading="saving" @click="emit('save')">保存</t-button>
    </div>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'

type MeterCategory = 'electricity' | 'water' | 'gas'
const props = defineProps<{
  visible: boolean
  category: MeterCategory
  title: string
  /** 父级持有的表单对象（引用共享：组件写属性 = 父级 form 更新，watch 联动照常触发） */
  form: Record<string, any>
  /** 当前编辑行的 item id（有值 = 编辑态，使用单位锁定） */
  editingItemId?: string
  meterOptions: Array<{ label: string; value: string; disabled?: boolean }>
  useUnitOptions: Array<{ label: string; value: string }>
  /** 单价差异提示的比对基准（当前表计配置默认单价） */
  defaultUnitPrice?: number
  isTimeMeter: boolean
  readingInvalid?: boolean
  unitPriceDiff?: boolean
  duplicateWarning?: boolean
  formRate: number
  formUsage: number
  formAmount: number
  saving?: boolean
  today: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'close'): void
  (e: 'save'): void
  (e: 'meter-change'): void
  (e: 'use-unit-change'): void
}>()

const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))
const categoryLabel = computed(() => (props.category === 'water' ? '水费' : props.category === 'electricity' ? '电费' : '气费'))
const usageLabel = computed(() => (props.category === 'water' ? '用水量' : props.category === 'electricity' ? '用电量' : '用气量'))

const timePeriods = [
  { key: 'deep', label: '尖', prevKey: 'deepPrev', currKey: 'deepCurr' },
  { key: 'peak', label: '峰', prevKey: 'peakPrev', currKey: 'peakCurr' },
  { key: 'flat', label: '平', prevKey: 'flatPrev', currKey: 'flatCurr' },
  { key: 'valley', label: '谷', prevKey: 'valleyPrev', currKey: 'valleyCurr' },
]

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtUnitPrice = (v: any) => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return String(Math.round(n * 1000) / 1000)
}
</script>

<style lang="less" scoped>
.mr-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px 16px;

  :deep(.t-form__item) { margin-bottom: 14px; }
  // 标签单行不换行不溢出（曾出现「电费（自动计算）」压到控件上）
  :deep(.t-form__label) { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  // 控件撑满列宽、右缘对齐（BSD 规范：所有输入控件 width:100%）
  :deep(.t-date-picker), :deep(.t-select), :deep(.t-input), :deep(.t-textarea) { width: 100%; }
}
.mr-field-wide { grid-column: 1 / -1; }

.mr-readonly {
  width: 100%;
  height: 32px;
  line-height: 32px;
  padding: 0 10px;
  border-radius: var(--td-radius-default);
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  box-sizing: border-box;
}

.mr-period-pair {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;

  .t-input { flex: 1; min-width: 0; }
}
.mr-period-sep { color: var(--td-text-color-placeholder); font-size: 13px; flex-shrink: 0; }

.mr-calc {
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.mr-hint { margin: 2px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }
.mr-error { margin: 2px 0 0; font-size: 12px; color: var(--td-error-color); }

.mr-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
  padding-top: 14px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 6px;
}
</style>

<style lang="less">
// SettingDrawer 底座在 meter-record 场景的收敛：body 不再叠 extra padding（栅格自带节奏）
.meter-record-drawer .t-drawer__body { padding: 18px 20px 14px; }
</style>
