<!--
  BusinessEmptyState —— 中台统一空状态（第 26 轮 O-6）。

  背景：发票/合同/制度/奖惩四页各写一份「未建库」空状态，而列表无数据时用
  TDesign 表格默认占位（居中灰字），两种空态视觉权重完全不同 —— 用户在
  「没有知识库」和「这个月没数据」之间看不出区别。

  两种尺寸：
    - 默认（整页/整块）：图标 64px + 大间距，用于「未建库」这类引导场景；
    - compact（表格内）：图标 32px + 紧凑间距，用于 t-table 的 #empty slot。
      表格里用 64px 图标会把表头下方撑出大片空白，和分页器挤在一起。

  契约：
    - 只接收文案与图标名，不认识任何业务 scope（与其它中台组件一致）；
    - 有 actionText 才渲染按钮，事件名固定 `action`。
-->
<template>
  <div class="biz-empty" :class="{ 'biz-empty--compact': compact }">
    <!-- 自绘空状态，不用 t-empty（第 31 轮）。
         TDesign Empty 的 title 默认值就是「暂无数据」，`:title="''"` 会被当成空值
         回退到默认文案 —— 于是 title 与 description 各渲染一次「暂无数据」，
         真机上就是上下两行重复（图 4/图 5）。自绘后文案完全由 props 决定，
         也顺手统一了各业务页的图标、间距与字色层级。 -->
    <t-icon :name="icon" :size="compact ? '32px' : '64px'" class="biz-empty__icon" />
    <p class="biz-empty__desc">{{ description }}</p>
    <p v-if="hint" class="biz-empty__hint">{{ hint }}</p>
    <t-button v-if="actionText" theme="primary" size="medium" @click="emit('action')">
      {{ actionText }}
    </t-button>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  /** 主文案（如「尚未创建「日常事务-发票」知识库」） */
  description: string
  /** TDesign 图标名，缺省 file */
  icon?: string
  /** 有值才渲染操作按钮 */
  actionText?: string
  /** 表格内使用：缩小图标与间距 */
  compact?: boolean
  /** 副文案（可选）：一句操作指引，弱化为次要灰字 */
  hint?: string
}>(), {
  icon: 'file',
  actionText: '',
  compact: false,
  hint: '',
})

const emit = defineEmits<{ (e: 'action'): void }>()
</script>

<style scoped>
.biz-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--td-comp-margin-l, 16px);
  min-height: 300px;
  color: var(--td-text-color-secondary);
}

/* 表格内：不抢高度，只做「这里没数据」的轻提示 */
.biz-empty--compact {
  min-height: 0;
  padding: var(--td-comp-paddingTB-l, 16px) 0;
  gap: var(--td-comp-margin-s, 8px);
}
</style>

<style scoped>
.biz-empty__icon {
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}
.biz-empty__desc {
  margin: 0;
  font-size: 14px;
  line-height: 22px;
  color: var(--td-text-color-secondary);
  text-align: center;
}
.biz-empty__hint {
  margin: 0;
  font-size: 12px;
  line-height: 20px;
  color: var(--td-text-color-placeholder);
  text-align: center;
}
</style>
