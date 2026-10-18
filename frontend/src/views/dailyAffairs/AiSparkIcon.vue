<template>
  <!--
    全局唯一的「AI」语义图标。
    为什么统一到一个组件：字段生成入口此前混用五角星（star），与「收藏/评分」语义冲突；
    用户在菜单项、面板标题、规则工具栏看到的应当始终是同一个 AI 符号。
    TDesign 的 ai-1 图标（A + 四角闪星）是官方 AI 语义图标，这里集中封装，
    后续换图标只改这一处。
  -->
  <Ai1Icon class="ai-spark-icon" :size="size" />
</template>

<script setup lang="ts">
import { Ai1Icon } from 'tdesign-icons-vue-next'

withDefaults(defineProps<{ size?: string }>(), { size: '15px' })
</script>

<style scoped>
.ai-spark-icon {
  color: inherit;
  flex: none;
}
</style>
