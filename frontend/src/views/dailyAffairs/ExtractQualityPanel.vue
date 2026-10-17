<template>
  <!-- 静默优先：无问题、无补洞时不渲染任何东西（正常文档占多数，常驻徽章会变成告警墙） -->
  <div v-if="!isSilent" class="extract-quality" :class="{ 'eq-compact': compact }">
    <!-- 溯源徽章：蓝底（数据来源提示，不是告警），同一字段只出一枚 -->
    <div v-if="labels.length" class="eq-patches">
      <span class="eq-patches-title">数据来源</span>
      <t-tooltip v-for="l in labels" :key="l.field" :content="patchTip(l)" placement="top">
        <t-tag size="small" theme="primary" variant="light" class="eq-badge">
          正则兜底: {{ l.label }}<template v-if="l.count > 1"> ×{{ l.count }}</template>
        </t-tag>
      </t-tooltip>
    </div>

    <!-- 校验问题：按等级合并成 1~2 条，明细里带 #下标 + 字段，便于定位到具体记录 -->
    <t-alert v-for="g in groups" :key="g.severity" :theme="g.theme" variant="light" class="eq-alert">
      <template #message>
        <span class="eq-alert-title">{{ g.title }}</span>
      </template>
      <template #description>
        <div class="eq-alert-body">
          <div v-for="(line, i) in g.lines" :key="i" class="eq-alert-line">{{ line }}</div>
          <div v-if="g.rest > 0" class="eq-alert-more">…等共 {{ g.total }} 处</div>
        </div>
      </template>
    </t-alert>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  qualityIssueCounts,
  qualityIsEmpty,
  qualityPatchLabels,
  type ExtractQuality,
  type ExtractIssue,
  type QualityPatchLabel,
} from './extractQuality'

const props = withDefaults(
  defineProps<{
    quality?: ExtractQuality | null
    /** 字段 key → 中文名；缺省原样展示 */
    labelOf?: (field: string) => string
    /** 单条 Alert 最多列几行明细，其余折叠成"等共 N 处" */
    maxLines?: number
    /** 紧凑模式（抽屉顶部摘要栏用）：更小的间距与字号 */
    compact?: boolean
  }>(),
  {
    quality: null,
    labelOf: (f: string) => f,
    maxLines: 4,
    compact: false,
  },
)

const isSilent = computed(() => qualityIsEmpty(props.quality))

const labels = computed<QualityPatchLabel[]>(() =>
  qualityPatchLabels(props.quality, (f) => props.labelOf(f)),
)

function patchTip(l: QualityPatchLabel): string {
  const values = l.values.filter(Boolean)
  const head = values.length
    ? `模型未提取到「${l.label}」，由正则从原文回填：${values.slice(0, 3).join('、')}${values.length > 3 ? ' …' : ''}`
    : `「${l.label}」由正则从原文回填`
  return `${head}\n字段键：${l.field}；共 ${l.count} 处`
}

interface AlertGroup {
  severity: 'error' | 'warn'
  theme: 'error' | 'warning'
  title: string
  lines: string[]
  rest: number
  total: number
}

function issueLine(it: ExtractIssue): string {
  const loc = it.index >= 0 ? `#${it.index + 1} ` : ''
  return `${loc}${props.labelOf(it.field)}${it.detail ? `：${it.detail}` : ''}`
}

const groups = computed<AlertGroup[]>(() => {
  const q = props.quality
  if (!q || q.issues.length === 0) return []
  const counts = qualityIssueCounts(q)
  const out: AlertGroup[] = []
  const build = (severity: 'error' | 'warn', theme: 'error' | 'warning', title: string) => {
    const items = q.issues.filter((i) => i.severity === severity)
    if (!items.length) return
    const shown = items.slice(0, props.maxLines)
    out.push({
      severity,
      theme,
      title: `${title}（${counts[severity]} 处）`,
      lines: shown.map(issueLine),
      rest: items.length - shown.length,
      total: items.length,
    })
  }
  build('error', 'error', '字段校验未通过')
  build('warn', 'warning', '字段格式需核对')
  return out
})
</script>

<style lang="less" scoped>
.extract-quality {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;

  &.eq-compact {
    gap: 6px;
    margin-bottom: 8px;
  }
}

.eq-patches {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;

  .eq-patches-title {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    white-space: nowrap;
  }

  .eq-badge {
    cursor: help;
  }
}

.eq-alert {
  :deep(.t-alert__message) {
    font-weight: 500;
  }

  .eq-alert-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 12px;
    line-height: 1.6;
    /* 明细可能很长（金额/日期原文），限宽截断避免把抽屉撑出横向滚动 */
    max-width: 100%;
  }

  .eq-alert-line {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .eq-alert-more {
    color: var(--td-text-color-placeholder);
  }
}
</style>
