<template>
  <!--
    「是否前往提取规则」引导弹窗。
    为什么不用 DialogPlugin.confirm：命令式弹窗拿不到样式（圆角/头部排版/按钮间距），
    与 WeKnora 现有「创建知识库」引导弹窗（BusinessKbWizard）视觉割裂。
    这里复用同一条外壳语言：t-dialog 默认头部 + 自绘 footer（上边框 + spacer + 主次按钮）。
  -->
  <t-dialog
    :visible="visible"
    :header="header"
    :footer="false"
    :close-on-overlay-click="false"
    width="520px"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div class="da-guide">
      <p class="da-guide-lead">
        字段已保存到「{{ certType }}」。是否为它生成提取规则？生成后会带上刚才的样票，
        逐字段校验正则能否唯一命中。
      </p>
      <ul class="da-guide-points">
        <li>规则只写入本分类，不影响其它发票类型</li>
        <li>随时可在「提取规则」里逐条修改或关闭</li>
      </ul>
      <div class="wizard-footer">
        <div class="spacer" />
        <t-button variant="outline" @click="emit('cancel')">稍后再说</t-button>
        <t-button theme="primary" @click="emit('confirm')">前往生成</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ visible: boolean; certType: string; header?: string }>(), {
  header: '字段已保存',
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()
</script>

<style lang="less" scoped>
.da-guide {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.da-guide-lead {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  color: var(--td-text-color-primary);
}

.da-guide-points {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--td-text-color-secondary);
}

/* 与 BusinessKbWizard 的 .wizard-footer 完全一致：上边框 + spacer + 主次按钮 */
.wizard-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 4px;

  .spacer {
    flex: 1;
  }
}
</style>
