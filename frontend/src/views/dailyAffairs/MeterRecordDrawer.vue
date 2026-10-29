<template>
  <!--
    抄表记录抽屉（电费/水费/气费核算共用）。

    字段来源只有一个：当前分类的「字段定义（subs）」。
    编辑与新增共用本组件，因此两条路径天然与配置 1:1 —— 顺序、显隐、标签全部照 subs 走，
    不再有写死的表单项（旧版多出 subs 里不存在的「录入日期」，又漏掉已启用的
    抄表方式 / 表号 / 默认单价 / 安装日期 / 用量，真机截图对比确认）。

    差异只有两种，都在字段渲染注册表（meterFieldRenderers.ts）里表达：
      · 分时电表：在「起度」位置整行渲染 尖/峰/平/谷 四段起止度；
      · 派生/档案类字段（电量、金额、倍率、表号、抄表方式、安装日期、默认单价）只读。

    状态契约：`form` 由父级持有并直接传入（对象引用共享，组件写属性即父级生效），
    月份/表计联动仍走父级 watch([form.month, form.meterId])，组件只透传 change 事件。
  -->
  <SettingDrawer :visible="visible" :title="title" width="654px" hide-footer destroy-on-close
    class="meter-record-drawer" :storage-key="`weknora-utility-${category}-record-drawer-width`"
    @update:visible="(v: boolean) => emit('update:visible', v)" @cancel="emit('close')">
    <!-- 「系统内已有数据」提示必须整行、且**在网格之外**：
         旧实现把它塞进「月份」「表计」两个 t-form-item 内部，双列 grid 里它只占 1 格，
         多出来的高度让后续所有表单项整体下移半行 → 左右两列从此错位（真机截图）。
         放在网格上方通栏，出现/消失都不影响网格行高。 -->
    <div v-if="duplicateWarning" class="mr-banner">
      <t-icon name="info-circle-filled" size="16px" class="mr-banner__icon" />
      <span class="mr-banner__text">该月该{{ meterLabel }}已有记录，可直接编辑</span>
    </div>
    <div class="mr-grid">
      <template v-for="f in fields" :key="f.key">
        <!-- 分时电表：四段起止度整行块（替代「起度/止度」两个单项） -->
        <div v-if="f.spec.kind === 'readings'" class="mr-field-wide">
          <template v-if="isTimeMeter">
            <t-form-item v-for="p in timePeriods" :key="p.key" :label="p.label" label-width="96px">
              <div class="mr-period-pair">
                <t-input :model-value="String(form[p.prevKey] ?? '')" type="number" placeholder="起度"
                  :status="readingInvalid ? 'error' : ''"
                  @update:model-value="(v: string) => (form[p.prevKey] = Number(v) || 0)" />
                <span class="mr-period-sep">→</span>
                <t-input :model-value="String(form[p.currKey] ?? '')" type="number" placeholder="止度"
                  :status="readingInvalid ? 'error' : ''"
                  @update:model-value="(v: string) => (form[p.currKey] = Number(v) || 0)" />
              </div>
              <p v-if="p.key === 'valley' && readingInvalid" class="mr-error">各时段止度不得小于起度</p>
            </t-form-item>
          </template>
          <t-form-item v-else :label="f.label" label-width="96px">
            <t-input :model-value="String(form[f.formKey] ?? '')"
              type="number" :placeholder="f.label" :status="readingInvalid ? 'error' : ''"
              @update:model-value="(v: string) => (form[f.formKey] = Number(v) || 0)" />
            <p v-if="readingInvalid" class="mr-error">止度不得小于起度</p>
          </t-form-item>
        </div>

        <!-- 备注等长文本：整行 -->
        <div v-else-if="f.spec.fullWidth" class="mr-field-wide">
          <t-form-item :label="f.label" label-width="96px">
            <t-textarea :model-value="form[f.formKey]" :maxlength="500" placeholder="选填"
              @update:model-value="(v: string) => (form[f.formKey] = v)" />
          </t-form-item>
        </div>

        <t-form-item v-else :label="f.label" label-width="96px">
          <!-- 月份 -->
          <template v-if="f.spec.kind === 'month'">
            <t-date-picker :model-value="form[f.formKey]" mode="month" format="YYYY-MM" value-type="YYYY-MM"
              placeholder="选择月份" @update:model-value="(v: any) => (form[f.formKey] = String(v || ''))" />
          </template>

          <!-- 日期 -->
          <t-date-picker v-else-if="f.spec.kind === 'date'" :model-value="form[f.formKey]" format="YYYY-MM-DD"
            value-type="YYYY-MM-DD" placeholder="选择日期" clearable
            @update:model-value="(v: any) => (form[f.formKey] = String(v || ''))" />

          <!-- 使用单位 -->
          <t-select v-else-if="f.spec.kind === 'use-unit'" :model-value="form[f.formKey]" :options="useUnitOptions"
            filterable :disabled="!!editingItemId"
            @update:model-value="(v: any) => (form[f.formKey] = String(v ?? ''))" @change="emit('use-unit-change')" />

          <!-- 表计 -->
          <template v-else-if="f.spec.kind === 'meter'">
            <t-select :model-value="form[f.formKey]" :placeholder="'选择' + meterLabel" :options="meterOptions" filterable
              @update:model-value="(v: any) => (form[f.formKey] = String(v ?? ''))" @change="emit('meter-change')" />
          </template>

          <!-- 文本 -->
          <t-input v-else-if="f.spec.kind === 'text'" :model-value="String(form[f.formKey] ?? '')" placeholder="选填"
            @update:model-value="(v: string) => (form[f.formKey] = v)" />

          <!-- 数值（单价 / 水费附加费）。分时电表没有单一标准单价：费用按租户分摊核算，
               单价置灰禁用（与「倍率」「表号」同款只读态），禁止输入。 -->
          <t-input v-else-if="f.spec.kind === 'number'" :model-value="unitPriceLocked(f.key) ? '' : String(form[f.formKey] ?? '')"
            type="number" :disabled="unitPriceLocked(f.key)" :placeholder="unitPriceLocked(f.key) ? '分时电表不适用' : f.label"
            @update:model-value="(v: string) => (form[f.formKey] = Number(v) || 0)" />

          <!-- 补差：type=number 会吞负号 → 文本 + 数字键盘，聚焦全选 -->
          <t-input v-else-if="f.spec.kind === 'signed'" :model-value="String(form[f.formKey] ?? '')" type="text"
            inputmode="decimal" placeholder="补差金额，可为负"
            @update:model-value="(v: string) => (form[f.formKey] = v)"
            @focus="(e: any) => { const t = e?.target || e; t?.select?.() }" />

          <!-- 用量：派生只读 -->
          <template v-else-if="f.spec.kind === 'usage'">
            <div class="mr-stack">
              <div class="mr-readonly">{{ fmtNum(formUsage) }} {{ unitLabel }}</div>
              <p class="mr-hint">{{ isTimeMeter ? '四时段电量合计' : '（止度 − 起度）× 倍率' }}</p>
            </div>
          </template>

          <!-- 金额：派生只读 -->
          <template v-else-if="f.spec.kind === 'amount'">
            <div class="mr-stack">
              <div class="mr-calc">{{ isTimeMeter ? '—' : fmtMoney(formAmount) + ' 元' }}</div>
              <p class="mr-hint">{{ amountHint }}</p>
            </div>
          </template>

          <!-- 档案/倍率等只读项 -->
          <template v-else>
            <div class="mr-readonly">{{ readonlyText(f.key) }}</div>
            <p v-if="f.key === 'unit_price' && unitPriceDiff" class="mr-error">与配置默认单价 {{ fmtUnitPrice(defaultUnitPrice) }} 不同</p>
            <p v-if="f.key === 'install_date'" class="mr-hint">表计档案</p>
          </template>
        </t-form-item>
      </template>
    </div>

    <div class="mr-footer">
      <t-button variant="outline" size="small" @click="emit('close')">取消</t-button>
      <t-button theme="primary" size="small" :loading="saving" @click="emit('save')">保存</t-button>
    </div>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import { METER_FALLBACK_FIELDS, meterArchiveValue, meterFieldSpec } from './meterFieldRenderers'

type MeterCategory = 'electricity' | 'water' | 'gas'

const props = defineProps<{
  visible: boolean
  category: MeterCategory
  title: string
  /** 父级持有的表单对象（引用共享：组件写属性 = 父级 form 更新，watch 联动照常触发） */
  form: Record<string, any>
  /** 当前分类的「字段定义」；为空时回落到内置兜底字段集 */
  subs?: Array<{ name: string; label?: string; enabled?: boolean; data_type?: string }>
  /** 当前选中的表计档案（表号 / 抄表方式 / 安装日期 / 默认单价 从这里取） */
  meter?: Record<string, any> | null
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

/**
 * 渲染清单 = subs（启用项，保持配置顺序）。
 *
 * 分时电表下「止度」不再单独出现：它已经被「起度」位置的四段块覆盖，
 * 再渲染一遍会出现一行无意义的普通起止度（旧版就是这种双份）。
 */
const fields = computed(() => {
  const src = (props.subs || []).filter((s: any) => s && s.enabled !== false && String(s.name || '').trim())
  const list = src.length ? src : METER_FALLBACK_FIELDS
  return list
    .map((s: any) => {
      const key = String(s.name || '')
      // 兜底集的「表计」标签按介质自适应（电表/水表/气表）
      const label = key === 'meter' && !src.length ? meterLabel.value : String(s.label || key)
      return {
        key,
        label,
        formKey: formKeyOf(key),
        spec: meterFieldSpec(key, s.data_type),
      }
    })
    .filter((f) => !(props.isTimeMeter && f.key === 'end_reading'))
})

/**
 * subs 的 key 是 snake_case（后端字段目录/列表列名），表单对象是 camelCase。
 * 这层映射是唯一转换点：两侧各自保持既有命名习惯，靠这里对齐
 * （漏一个就会出现「填了但保存不到」的静默丢值）。
 */
const FORM_KEY: Record<string, string> = {
  month: 'month',
  reading_date: 'readingDate',
  use_unit: 'useUnit',
  meter: 'meterId',
  reader: 'reader',
  meter_no: 'meterNo',
  meter_mode: 'meterMode',
  install_date: 'installDate',
  default_unit_price: 'defaultUnitPrice',
  start_reading: 'startReading',
  end_reading: 'endReading',
  rate: 'rate',
  usage: 'usage',
  unit_price: 'unitPrice',
  subsidy: 'subsidy',
  garbage_fee: 'garbageFee',
  secondary_water_fee: 'secondaryWaterFee',
  sewage_fee: 'sewageFee',
  amount: 'amount',
  remark: 'remark',
}
function formKeyOf(key: string): string {
  return FORM_KEY[key] || key
}

const amountHint = computed(() =>
  props.isTimeMeter ? '按租户分摊核算' : `自动计算 · ${fmtNum(props.formUsage)} ${unitLabel.value}`,
)

/** 分时电表的「单价」是禁用项：没有单一标准单价，费用走租户分摊。 */
function unitPriceLocked(key: string): boolean {
  return key === 'unit_price' && props.isTimeMeter
}

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

/** 只读项取值：倍率 → 记录派生；其余 → 表计档案。 */
function readonlyText(key: string): string {
  if (key === 'rate') return `${fmtNum(props.formRate)}×`
  const v = meterArchiveValue(props.meter, key)
  if (key === 'default_unit_price') {
    const s = fmtUnitPrice(v)
    return s === '' ? '—' : `${s} 元`
  }
  return v === '' || v == null ? '—' : String(v)
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

/* 「系统内已有数据」通栏提示：与表单网格完全解耦（不参与 grid 排版）。
   颜色取语义令牌，出现/消失都只改变自身高度，网格行高与列对齐不受影响。 */
.mr-banner {
  display: flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  width: 100%;
  margin-bottom: 12px;
  padding: 8px 12px;
  border-radius: var(--td-radius-default);
  background: var(--td-error-color-1);
  border: 1px solid var(--td-error-color-3);
  color: var(--td-error-color-6);
  font-size: 13px;
  line-height: 20px;

  .mr-banner__icon { flex: none; color: var(--td-error-color-6); }
  .mr-banner__text { flex: 1 1 auto; min-width: 0; }
}

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

.mr-stack { width: 100%; display: flex; flex-direction: column; }
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
