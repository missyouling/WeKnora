<template>
  <t-dialog
    :visible="visible"
    :header="title"
    :footer="false"
    :close-on-overlay-click="false"
    :close-btn="step > 1 ? true : false"
    width="640px"
    @update:visible="onUpdateVisible"
    @close="handleClose"
  >
    <div class="invoice-kb-wizard">
      <!-- 步骤条 -->
      <div class="wizard-steps">
        <div v-for="(s, i) in steps" :key="s.key" class="wizard-step" :class="{ active: i + 1 === step, done: i + 1 < step }">
          <div class="step-index">{{ i + 1 < step ? '✓' : i + 1 }}</div>
          <span class="step-label">{{ s.label }}</span>
        </div>
      </div>

      <!-- 第 1 步：基础信息 -->
      <div v-if="step === 1" class="wizard-body">
        <div class="setting-row">
          <div class="setting-info">
            <label>知识库名称 <span class="required">*</span></label>
            <p class="desc">发票管理固定使用专用知识库，名称不可修改</p>
          </div>
          <div class="setting-control">
            <t-input :value="kbName" disabled />
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-info">
            <label>描述</label>
            <p class="desc">用于说明该知识库的用途</p>
          </div>
          <div class="setting-control">
            <t-textarea
              v-model="description"
              :maxlength="200"
              placeholder="用于存放并解析发票文件，自动提取发票字段"
            />
          </div>
        </div>
      </div>

      <!-- 第 2 步：索引策略 -->
      <div v-if="step === 2" class="wizard-body">
        <div class="strategy-card">
          <div class="strategy-item">
            <t-checkbox :checked="true" disabled />
            <div class="strategy-info">
              <div class="strategy-title">向量检索</div>
              <p class="strategy-desc">启用向量索引，支持语义检索发票内容</p>
            </div>
          </div>
          <div class="strategy-item">
            <t-checkbox :checked="true" disabled />
            <div class="strategy-info">
              <div class="strategy-title">关键词检索</div>
              <p class="strategy-desc">启用关键词索引，支持精确匹配发票号码等信息</p>
            </div>
          </div>
          <div class="strategy-item">
            <t-checkbox :checked="false" disabled />
            <div class="strategy-info">
              <div class="strategy-title">Wiki 知识库</div>
              <p class="strategy-desc">发票管理不需要 Wiki 模式</p>
            </div>
          </div>
          <div class="strategy-item">
            <t-checkbox :checked="false" disabled />
            <div class="strategy-info">
              <div class="strategy-title">知识图谱</div>
              <p class="strategy-desc">发票管理不需要知识图谱</p>
            </div>
          </div>
        </div>
      </div>

      <!-- 第 3 步：模型选择 -->
      <div v-if="step === 3" class="wizard-body">
        <div class="model-tip">
          <t-alert theme="info" :message="'提取模型将复用下方「对话模型」，用于解析发票字段。若列表为空，请先在系统设置中添加模型。'" />
        </div>
        <KBModelConfig
          :config="modelConfig"
          :has-files="false"
          :rag-enabled="true"
          :all-models="allModels"
          @update:config="onModelConfigChange"
        />
      </div>

      <!-- 底部按钮 -->
      <div class="wizard-footer">
        <t-button v-if="step > 1" variant="outline" @click="step--">上一步</t-button>
        <div class="spacer" />
        <t-button v-if="step < 3" theme="primary" @click="step++">下一步</t-button>
        <t-button v-if="step === 3" theme="primary" :loading="submitting" @click="handleSubmit">创建知识库</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { createKnowledgeBase } from '@/api/knowledge-base'
import { useChatResourcesStore } from '@/stores/chatResources'
import { selectInitialModelId } from '@/utils/modelDefaults'
import KBModelConfig from '@/views/knowledge/settings/KBModelConfig.vue'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  created: [kb: any]
}>()

const kbName = '日常事务-发票'
const steps = [
  { key: 'basic', label: '基础信息' },
  { key: 'strategy', label: '索引策略' },
  { key: 'models', label: '模型选择' },
]
const title = computed(() => `创建「${kbName}」知识库`)

const step = ref(1)
const description = ref('用于存放并解析发票文件，自动提取发票字段')
const submitting = ref(false)

const chatResources = useChatResourcesStore()
const allModels = computed(() => chatResources.allModels || [])

const modelConfig = ref<{
  llmModelId?: string
  embeddingModelId?: string
}>({
  llmModelId: undefined,
  embeddingModelId: undefined,
})

watch(
  () => props.visible,
  async (v) => {
    if (v) {
      step.value = 1
      try {
        await chatResources.ensureModels()
      } catch {
        /* 模型列表拉取失败时保留空列表 */
      }
      modelConfig.value = {
        llmModelId: selectInitialModelId(allModels.value, 'KnowledgeQA') || undefined,
        embeddingModelId: selectInitialModelId(allModels.value, 'Embedding') || undefined,
      }
    }
  },
)

const onModelConfigChange = (config: any) => {
  modelConfig.value = {
    llmModelId: config.llmModelId,
    embeddingModelId: config.embeddingModelId,
  }
}

const handleSubmit = async () => {
  if (!modelConfig.value.llmModelId) {
    MessagePlugin.warning('请选择对话模型（提取模型）')
    step.value = 3
    return
  }
  if (!modelConfig.value.embeddingModelId) {
    MessagePlugin.warning('请选择向量模型')
    step.value = 3
    return
  }

  submitting.value = true
  try {
    const payload: any = {
      name: kbName,
      description: description.value || '',
      type: 'document',
      chunking_config: {
        chunk_size: 512,
        chunk_overlap: 80,
        separators: ['\n\n', '\n', '。', '；', '，', ';', '、'],
        enable_parent_child: true,
        strategy: '',
        token_limit: 0,
        languages: [],
        table_metadata_instructions: '',
      },
      embedding_model_id: modelConfig.value.embeddingModelId,
      summary_model_id: modelConfig.value.llmModelId,
      indexing_strategy: {
        vector_enabled: true,
        keyword_enabled: true,
        wiki_enabled: false,
        graph_enabled: false,
      },
    }
    const res: any = await createKnowledgeBase(payload)
    const kb = res?.data || res
    MessagePlugin.success('知识库创建成功')
    emit('created', kb)
    handleClose()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '知识库创建失败')
  } finally {
    submitting.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
}

const onUpdateVisible = (v: boolean) => {
  emit('update:visible', v)
}
</script>

<style lang="less" scoped>
.invoice-kb-wizard {
  padding: 8px 0 0;
}

.wizard-steps {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 0 20px;
  border-bottom: 1px solid var(--td-component-stroke);
  margin-bottom: 20px;
}

.wizard-step {
  display: flex;
  align-items: center;
  gap: 8px;

  &.active {
    .step-index {
      background: var(--td-brand-color);
      border-color: var(--td-brand-color);
      color: #fff;
    }

    .step-label {
      color: var(--td-text-color-primary);
      font-weight: 600;
    }
  }

  &.done {
    .step-index {
      background: var(--td-brand-color-2);
      border-color: var(--td-brand-color-2);
      color: var(--td-brand-color);
    }
  }
}

.step-index {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 1px solid var(--td-component-stroke);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
}

.step-label {
  font-size: 14px;
  color: var(--td-text-color-secondary);
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
  flex: 0 0 38%;
  max-width: 38%;
  padding-right: 20px;

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

.strategy-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.strategy-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
}

.strategy-info {
  .strategy-title {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }

  .strategy-desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    margin: 2px 0 0;
  }
}

.model-tip {
  margin-bottom: 12px;
}

.wizard-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 16px;

  .spacer {
    flex: 1;
  }
}
</style>
