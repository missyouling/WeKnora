<template>
  <div class="daily-affairs-container">
    <div class="header">
      <div class="header-title">
        <h2>日常事务</h2>
        <p class="header-subtitle">企业日常事务统一管理入口</p>
      </div>
    </div>

    <div class="daily-affairs-main">
      <div class="card-grid">
        <!-- 发票管理（已实现） -->
        <div class="da-card da-card--enabled" @click="goInvoices">
          <div class="da-card-icon da-card-icon--invoice">
            <t-icon name="money" size="24px" />
          </div>
          <div class="da-card-body">
            <div class="da-card-title">
              <span>发票管理</span>
              <t-tag theme="success" variant="light" size="small">已上线</t-tag>
            </div>
            <p class="da-card-desc">上传发票文件，自动识别发票号码、金额、销方购方等字段并归档管理</p>
            <div class="da-card-footer">
              <span class="da-card-link">进入管理</span>
              <t-icon name="chevron-right" size="16px" />
            </div>
          </div>
        </div>

        <!-- 合同管理（已实现） -->
        <div class="da-card da-card--enabled" @click="goContracts">
          <div class="da-card-icon da-card-icon--contract">
            <t-icon name="file-copy" size="24px" />
          </div>
          <div class="da-card-body">
            <div class="da-card-title">
              <span>合同管理</span>
              <t-tag theme="success" variant="light" size="small">已上线</t-tag>
            </div>
            <p class="da-card-desc">上传合同文件，自动识别合同编号、类型、双方主体与金额等字段并归档管理</p>
            <div class="da-card-footer">
              <span class="da-card-link">进入管理</span>
              <t-icon name="chevron-right" size="16px" />
            </div>
          </div>
        </div>

        <!-- 制度管理（已实现） -->
        <div class="da-card da-card--enabled" @click="goRegulations">
          <div class="da-card-icon da-card-icon--policy">
            <t-icon name="file" size="24px" />
          </div>
          <div class="da-card-body">
            <div class="da-card-title">
              <span>制度管理</span>
              <t-tag theme="success" variant="light" size="small">已上线</t-tag>
            </div>
            <p class="da-card-desc">上传制度文件，自动识别制度编号、类型、版本与编制信息并归档管理</p>
            <div class="da-card-footer">
              <span class="da-card-link">进入管理</span>
              <t-icon name="chevron-right" size="16px" />
            </div>
          </div>
        </div>

        <!-- 奖惩管理（已实现） -->
        <div class="da-card da-card--enabled" @click="goAwardPunish">
          <div class="da-card-icon da-card-icon--reward">
            <t-icon name="medal" size="24px" />
          </div>
          <div class="da-card-body">
            <div class="da-card-title">
              <span>奖惩管理</span>
              <t-tag theme="success" variant="light" size="small">已上线</t-tag>
            </div>
            <p class="da-card-desc">上传奖惩通报，自动识别文号、类型、当事人与处罚决定并归档管理</p>
            <div class="da-card-footer">
              <span class="da-card-link">进入管理</span>
              <t-icon name="chevron-right" size="16px" />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'

const router = useRouter()

const goInvoices = () => {
  router.push('/platform/daily-affairs/invoices')
}

const goContracts = () => {
  router.push('/platform/daily-affairs/contracts')
}

const goRegulations = () => {
  router.push('/platform/daily-affairs/regulations')
}

const goAwardPunish = () => {
  router.push('/platform/daily-affairs/award-punish')
}

const onPlaceholder = (name: string) => {
  MessagePlugin.info(`「${name}」功能开发中，敬请期待`)
}
</script>

<style lang="less" scoped>
.daily-affairs-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 24px 32px;
  overflow-y: auto;
}

.header {
  margin-bottom: 20px;

  .header-title {
    h2 {
      font-size: 20px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      margin: 0 0 6px 0;
    }

    .header-subtitle {
      font-size: 14px;
      color: var(--td-text-color-secondary);
      margin: 0;
    }
  }
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.da-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  padding: 20px;
  display: flex;
  gap: 16px;
  cursor: pointer;
  transition: all 0.25s ease;
  min-height: 150px;
  box-sizing: border-box;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 4px 12px rgba(7, 192, 95, 0.12);
    transform: translateY(-2px);
  }

  &.da-card--enabled {
    .da-card-link {
      color: var(--td-brand-color);
    }
  }
}

.da-card-icon {
  flex: 0 0 48px;
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;

  &.da-card-icon--invoice {
    background: linear-gradient(135deg, #07c05f, #0a9d51);
  }

  &.da-card-icon--contract {
    background: linear-gradient(135deg, #4e6b99, #3a5480);
  }

  &.da-card-icon--policy {
    background: linear-gradient(135deg, #f59e0b, #d97706);
  }

  &.da-card-icon--reward {
    background: linear-gradient(135deg, #ef4444, #dc2626);
  }
}

.da-card-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.da-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 8px;
}

.da-card-desc {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
  margin: 0;
  flex: 1;
}

.da-card-footer {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 4px;

  .da-card-link {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}
</style>
