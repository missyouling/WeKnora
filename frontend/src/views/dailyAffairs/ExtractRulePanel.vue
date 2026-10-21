<template>
  <div ref="rootRef" class="extract-rule-panel" :class="{ 'eq-narrow': narrow }">
    <div class="extract-toolbar">
      <t-select v-if="certTypeOptions.length" v-model="certType" class="cert-type-select" :options="certTypeOptions" :placeholder="`选择${getScope(scope)?.uploadTypeLabel || '证照类型'}`" @change="onCertTypeChange" />
      <span v-else class="cert-type-static">整体提取规则</span>
      <div class="spacer" />
      <!-- 高级模式：左侧文字标签 + 右侧开关（不再用 t-switch 内嵌 label，避免文字随开关漂移） -->
      <span class="adv-label">高级模式</span>
      <t-switch v-model="advancedEnabled" size="medium" class="adv-switch" />
      <!-- 图标按钮一律无边框（variant="text"），hover 仅给柔和底色，避免整排描边按钮抢视线。
           批量「智能生成规则」已下线：逐字段生成（展开区「生成该字段规则」）反馈更可控。 -->
      <t-tooltip :content="aiSampleHint" placement="top">
        <!-- 票据图标：缓存里有样票时**常驻绿色**（is-hot），提示"这次生成能校验唯一命中" -->
        <t-button variant="text" shape="square" size="small" class="tb-icon-btn"
          :class="{ 'is-hot': sampleTextAvailable }"
          :disabled="!certType || !sampleTextAvailable" @click="clearCachedSample">
          <template #icon><t-icon name="ticket" /></template>
        </t-button>
      </t-tooltip>
      <t-tooltip content="测试规则（在页面内展开测试区，边改边测）" placement="top">
        <t-button variant="text" shape="square" size="small" class="tb-icon-btn"
          :class="{ 'is-active': testVisible }" :disabled="!certType" @click="toggleTest">
          <template #icon><t-icon name="play-circle" /></template>
        </t-button>
      </t-tooltip>
    </div>

    <!-- 测试区锚点：工具栏正下方、规则表上方（与规则表同屏对照） -->
    <ExtractTestPanel v-model:visible="testVisible" :kb-id="resolvedKbId" :scope="currentScope" :cert-type="certTypeName"
      :fields="fields" :advanced-enabled="advancedEnabled" :prompt-template="promptTemplate"
      @saved="onTestPanelSaved" />

    <div class="rule-table">
      <div class="rule-table-head">
        <span>字段名称</span>
        <span>字段描述</span>
        <span>数据类型</span>
        <span class="col-required">必填</span>
        <span class="col-regex">正则兜底</span>
        <span>状态</span>
      </div>
      <template v-for="(f, i) in fields" :key="f.name + '-' + i">
        <div class="rule-table-row" :class="{ editing: editingName === f.name, 'ai-filled': aiFilled.has(f.name) }" @click="toggleEdit(f.name)">
          <span class="rtr-name" :class="{ 'rtr-disabled': !f.enabled }">
            {{ fieldLabel(f.name) }}
            <t-tag v-if="aiFilled.has(f.name)" size="small" variant="light" theme="primary" class="rtr-ai-badge">AI</t-tag>
          </span>
          <span class="rtr-desc" :title="f.desc || f.name">{{ f.desc || '—' }}</span>
          <span class="rtr-type">{{ typeLabel(f.type || '') }}</span>
          <!-- 必填：控制该字段为空时是否产出 error 级校验告警 -->
          <span class="rtr-required" @click.stop>
            <t-tooltip :content="requiredTip" placement="top">
              <t-switch v-model="f.required" size="small" :disabled="!isStdField(f.name)" />
            </t-tooltip>
          </span>
          <!-- 正则兜底：LLM 漏抽时从原文回填（Go RE2）。行内只做快速填写，完整编辑在展开区 -->
          <span class="rtr-regex" @click.stop>
            <t-tooltip :content="regexTip(f.name)" placement="top">
              <t-input v-model="f.regex" size="small" class="rtr-regex-input" placeholder="未配置"
                :disabled="!regexPatchable(f.name)" @click.stop />
            </t-tooltip>
          </span>
          <span class="rtr-status">
            <t-switch v-model="f.enabled" size="small" @click.stop @change="() => {}" />
          </span>
        </div>
          <!-- 行内展开配置（复刻证照配置展开编辑） -->
        <div v-if="editingName === f.name" class="rule-form rule-form--inline">
          <div class="rule-form-title">
            配置「{{ fieldLabel(f.name) }}」
            <t-button variant="text" size="small" class="rule-form-ai" :loading="aiFieldBusy === f.name"
              @click.stop="generateFieldRule(f)">
              <template #icon><AiSparkIcon size="14px" /></template>生成该字段规则
            </t-button>
          </div>
          <div class="rule-form-grid">
            <div class="rule-form-item">
              <label>字段描述</label>
              <t-input v-model="f.desc" size="small" placeholder="如：公司实际经营地址" />
            </div>
            <div class="rule-form-item">
              <label>数据类型</label>
              <t-select v-model="f.type" size="small" :options="typeOptions" />
            </div>
            <div class="rule-form-item">
              <label>必填校验</label>
              <div class="rule-form-inline">
                <t-switch v-model="f.required" size="small" :disabled="!isStdField(f.name)" />
                <span class="rule-form-hint">开启后该字段为空会产出 error 级告警（不影响提取结果落库）</span>
              </div>
            </div>
            <div class="rule-form-item">
              <label>正则兜底</label>
              <!-- 全字段开放：标准字段 / 扩展字段 / 明细子项都可配；置灰只剩「本模块是否接入」这一条 -->
              <t-input v-model="f.regex" size="small" placeholder="如 \b\d{20}\b"
                :disabled="!regexPatchable(f.name)" />
              <div class="rule-form-hint" :class="{ 'hint-danger': !!regexHintOf(f) }">
                {{ regexHintOf(f) || '模型未提取到该字段时，用此正则从原文唯一命中回填（Go RE2 语法）' }}
              </div>
            </div>
            <div class="rule-form-item rule-form-item--full">
              <label>常用正则</label>
              <div class="rule-quick">
                <span class="rule-quick-title">一键填入</span>
                <t-tooltip v-for="p in REGEX_PRESETS" :key="p.name" :content="p.hint" placement="top">
                  <t-tag size="small" variant="outline" theme="primary" class="rule-quick-tag"
                    @click="applyRegexPreset(f, p)">
                    {{ p.name }}
                  </t-tag>
                </t-tooltip>
                <t-button v-if="f.regex" variant="text" size="small" @click="f.regex = ''">清空</t-button>
              </div>
              <t-alert v-if="!fieldRulesSupported(scope)" theme="info" variant="light" class="rule-form-alert"
                message="本模块暂未接入字段级校验与正则补洞（目前仅发票支持）" />
            </div>
            <div class="rule-form-item rule-form-item--full">
              <label>提取规则</label>
              <t-textarea v-model="f.rule" size="small" :autosize="{ minRows: 2, maxRows: 6 }"
                placeholder="例：出现“注册地址 / 公司地址”等表述时归入本字段；无则留空" />
              <div class="rule-quick">
                <span class="rule-quick-title">常用写法</span>
                <t-tag v-for="(ex, ei) in ruleExamples.slice(0, 4)" :key="ei" size="small" variant="outline"
                  theme="primary" class="rule-quick-tag" @click="applyExample(f, ex.text)">{{ ex.name }}</t-tag>
              </div>
            </div>
          </div>

          <!-- 明细子项规则：点明细行的展开区，逐个配「项目明细」下的子项口径。
               规则/正则随 ExtractFieldConfig.sub_fields 持久化；正则只在
               「单票单明细行」时回填，多行不猜归属（与后端同一条铁律）。 -->
          <div v-if="isItemsField(f)" class="sub-rule-block">
            <div class="sub-rule-head">
              <span class="sub-rule-title">明细子项规则</span>
              <span class="sub-rule-tip">子项正则仅在单票单明细行时回填</span>
            </div>
            <div v-if="!(f.sub_fields || []).length" class="sub-rule-empty">
              该明细字段暂无子项，请先在「字段定义」里添加
            </div>
            <div v-for="sf in f.sub_fields || []" :key="sf.key" class="sub-rule-row" :class="{ 'is-off': sf.enabled === false }">
              <div class="sub-rule-name">
                <span class="sub-rule-label">{{ sf.label || sf.key }}</span>
                <span class="sub-rule-key">{{ sf.key }}</span>
              </div>
              <t-input v-model="sf.rule" size="small" class="sub-rule-input" placeholder="提取口径（可空）" />
              <t-input v-model="sf.regex" size="small" class="sub-rule-regex" placeholder="正则（可空）" />
              <t-switch :model-value="sf.enabled !== false" size="small" class="sub-rule-switch"
                @change="(v: any) => (sf.enabled = !!v)" />
            </div>
            <div class="rule-form-hint">
              子项正则示例：规格型号 <code>\s*规格型号\s*[:：]\s*([A-Za-z0-9]+)</code>
            </div>
          </div>

          <div class="rule-form-actions">
            <t-button variant="outline" size="small" @click="editingName = ''">收起</t-button>
            <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
          </div>
        </div>
      </template>
      <div v-if="!fields.length" class="rule-empty">暂无字段，请在字段定义中添加字段</div>
    </div>

    <template v-if="advancedEnabled">
      <div class="advanced-block">
        <div class="advanced-title">高级 Prompt 模板</div>
        <div class="slot-tags">
          <t-tag v-for="s in slots" :key="s" size="small" variant="outline" theme="primary" class="slot-tag" @click="insertSlot(s)">{{ s }}</t-tag>
          <span class="slot-hint">点击插入变量</span>
          <t-button variant="text" size="small" class="example-toggle" @click="applyTemplateExample">示例模板</t-button>
        </div>
        <t-textarea v-model="promptTemplate" class="prompt-area" :autosize="{ minRows: 6, maxRows: 16 }"
          placeholder="你是一个证件信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。\n需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON，未找到的字段输出 null，不臆造内容。" />
        <t-alert v-if="templateWarning" theme="warning" variant="light" :message="templateWarning" class="advanced-warning" />
        <div class="advanced-actions">
          <t-button variant="outline" size="small" @click="resetPromptTemplate">恢复默认模板</t-button>
          <t-button variant="outline" size="small" @click="advancedEnabled = false">收起</t-button>
          <t-button theme="primary" size="small" :disabled="!certType" :loading="saving" @click="save">保存</t-button>
        </div>
      </div>
    </template>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { getExtractConfig, saveExtractConfig, type ExtractFieldConfig, type ExtractSubFieldConfig, type AiSuggestRulesResult, aiSuggestRules } from '@/api/fleet'
import { listFleetGroupAliases } from '@/api/fleet'
import { useExtractCatalog } from './useExtractCatalog'
import { sortCertsByLocalOrder } from './useCertOrder'
import { toCamelKey, toSnakeKey } from './invoiceFieldLabels'
import {
  REGEX_PRESETS,
  extractFieldLabel,
  fieldRulesSupported,
  isStandardField,
  isRegexPatchable,
  regexDisabledReason,
  regexSyntaxHint,
  type RegexPreset,
} from './extractFieldMeta'
import { listKnowledgeBases } from '@/api/knowledge-base'
import ExtractTestPanel from './ExtractTestPanel.vue'
import AiSparkIcon from './AiSparkIcon.vue'
import { cacheSampleText, dropSampleText, readSampleText } from './sampleTextCache'
import { getScope } from '../../registry'

const props = defineProps<{ scope: string; kbName?: string; kbId?: string }>()

const KB_NAME = computed(() => props.kbName || '日常事务-车队')
const resolvedKbId = ref('')

// 默认字段底稿（无配置时按证照类型生成）：与证照配置内置字段一致
const DEFAULT_FIELDS: Record<string, Record<string, string[]>> = {
  vehicle: {
    车辆登记证书: ['证书编号', '车牌号', 'VIN码', '发证机关', '发证日期', '证书状态', '存放位置', '是否随车', '备注'],
    行驶证: ['编号', '车牌号', 'VIN码/车架号', '发动机号', '品牌型号', '车辆类型', '使用性质', '注册日期', '发证日期', '发证机关', '行驶证编号', '有效期', '状态', '备注'],
    道路运输经营许可证: ['许可证号', '业户名称', '经营地址', '经营范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态', '备注'],
    道路运输证: ['道路运输证号', '车牌号', '经营许可证号', '车辆类型', '吨（座）位', '业户名称', '经营地址', '经营范围', '车辆尺寸', '发证日期', '有效期止', '发证机关', '上次审验日期', '下次审验日期', '技术评定等级', '备注'],
    保险单: ['保单号', '保险公司', '保险类型', '车辆ID', '车牌号', 'VIN码', '被保险人名称', '险种名称', '保额', '保费', '总保费', '起保日期', '终保日期', '保单状态', '缴费状态', '发票号', '备注'],
  },
  driver: {
    驾驶证: ['驾驶证号', '司机姓名', '准驾车型', '初次领证日期', '有效期起', '有效期止', '发证机关', '驾驶证状态', '备注'],
    从业资格证: ['从业资格证号', '司机姓名', '从业资格类别', '准运范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态', '备注'],
  },
  maintain: {
    维修工单: ['工单号', '车牌号', '维修日期', '维修项目', '工时费', '材料费', '总费用'],
    二级维护: ['维护日期', '车牌号', '维护项目', '维护单位', '下次维护日期'],
  },
  invoice: {
    普通发票: ['发票号码', '开票日期', '发票类型', '金额', '税率', '税额', '价税合计', '购买方', '销售方', '开票人', '备注', '状态', '标签', '销售方税号', '购买方税号', '文件名'],
    专用发票: ['发票号码', '开票日期', '发票类型', '金额', '税率', '税额', '价税合计', '购买方', '销售方', '开票人', '备注', '状态', '标签', '销售方税号', '购买方税号', '文件名'],
    其它票据: ['发票号码', '开票日期', '发票类型', '金额', '税率', '税额', '价税合计', '购买方', '销售方', '开票人', '备注', '状态', '标签', '销售方税号', '购买方税号', '文件名'],
    // 水电发票：细分分类 subs 使用英文键名（invoiceNo/invoiceDate/...），字段集与普通发票一致
    水电发票: ['发票号码', '开票日期', '发票类型', '金额', '税率', '税额', '价税合计', '购买方', '销售方', '开票人', '备注', '状态', '标签', '销售方税号', '购买方税号', '文件名'],
  },
  contract: {
    服务合同: ['contract_no', 'contract_name', 'contract_type', 'party_a', 'party_b', 'sign_date', 'expire_date', 'amount', 'extract_status', 'performance_status', 'tags', 'party_a_contact', 'party_a_tax_no', 'party_b_contact', 'party_b_tax_no', 'party_a_address', 'party_a_phone', 'party_a_bank', 'party_a_account', 'party_b_address', 'party_b_phone', 'party_b_bank', 'party_b_account', 'payment_method', 'sign_location', 'summary', 'fileName'],
    租赁合同: ['contract_no', 'contract_name', 'contract_type', 'party_a', 'party_b', 'sign_date', 'expire_date', 'amount', 'extract_status', 'performance_status', 'tags', 'party_a_contact', 'party_a_tax_no', 'party_b_contact', 'party_b_tax_no', 'party_a_address', 'party_a_phone', 'party_a_bank', 'party_a_account', 'party_b_address', 'party_b_phone', 'party_b_bank', 'party_b_account', 'payment_method', 'sign_location', 'summary', 'fileName'],
  },
}

// 默认提取规则底稿：每个字段的 desc + rule 预设，加载时自动填充（已保存配置优先）。
// 便于用户直接使用或在此基础上微调。
type RuleDraft = { desc: string; rule: string }

// 公共底稿（发票的「发票代码 / 项目明细 / 车牌号」等跨分类通用键）已迁入
// BusinessScopeRegistry：`registry/invoice.ts` 的 commonRuleDrafts。
// 本组件只按当前 scope 取用，不再内嵌任何 scope 专属字典。
const DEFAULT_RULES: Record<string, Record<string, Record<string, RuleDraft>>> = {
  vehicle: {
    车辆登记证书: {
      证书编号: { desc: '机动车登记证书编号', rule: '提取登记证书“证书编号 / 登记证书编号”栏，如“渝D202300001”' },
      车牌号: { desc: '车辆号牌号码', rule: '提取“号牌号码”栏，格式如“渝A12345”；多个号牌只取第一个' },
      VIN码: { desc: '车辆识别代号', rule: '提取“车辆识别代号 / VIN”栏，17 位字母数字组合' },
      发证机关: { desc: '登记证书发证机关', rule: '提取“发证机关”栏，如“重庆市公安局交通管理局车辆管理所”' },
      发证日期: { desc: '登记证书核发日期', rule: '提取“发证日期 / 核发日期”栏，统一为 YYYY-MM-DD' },
      证书状态: { desc: '证书当前状态', rule: '从正常、遗失、补办中、注销中识别；原文无则留空' },
      存放位置: { desc: '证书实物存放地点', rule: '如“财务室档案柜”；原文无则留空' },
      是否随车: { desc: '证书是否随车携带', rule: '识别“是 / 否”；原文无则留空' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
    行驶证: {
      编号: { desc: '证件编号', rule: '提取行驶证“证件编号”栏' },
      车牌号: { desc: '号牌号码', rule: '提取“号牌号码”栏，格式如“渝A12345”' },
      'VIN码/车架号': { desc: '车辆识别代号', rule: '提取“车辆识别代号”栏，17 位字母数字组合' },
      发动机号: { desc: '发动机编号', rule: '提取“发动机号码”栏' },
      品牌型号: { desc: '品牌及型号', rule: '提取“品牌型号”栏，如“东风牌DFH4250D4”' },
      车辆类型: { desc: '客车、货车、专项作业车等', rule: '提取“车辆类型”栏；原文无则留空' },
      使用性质: { desc: '营运、非营运、租赁、危化品等', rule: '提取“使用性质”栏' },
      注册日期: { desc: '初次登记日期', rule: '提取“注册日期”栏，统一为 YYYY-MM-DD' },
      发证日期: { desc: '行驶证核发日期', rule: '提取“发证日期”栏，统一为 YYYY-MM-DD' },
      发证机关: { desc: '车管所名称', rule: '提取“发证机关”栏' },
      行驶证编号: { desc: '证芯编号', rule: '提取“行驶证编号 / 证芯编号”栏' },
      有效期: { desc: '检验有效期', rule: '提取“检验有效期”栏，统一为 YYYY-MM-DD' },
      extractStatus: { desc: '正常、遗失、补办中、注销', rule: '从状态描述中识别；原文无则留空' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
    道路运输经营许可证: {
      许可证号: { desc: '道路运输经营许可证编号', rule: '提取“许可证号”栏，如“渝交运管许可渝字5001202023001号”' },
      业户名称: { desc: '经营业户名称', rule: '提取“业户名称”栏，如“重庆均和汽车运输有限公司”' },
      经营地址: { desc: '公司实际经营地址', rule: '文档中出现公司地址、注册地址、单位地址等表述时，统一归入本字段' },
      经营范围: { desc: '许可经营范围', rule: '提取“经营范围”栏，如“普通货运、货物专用运输”' },
      发证机关: { desc: '发证机关名称', rule: '提取“发证机关”栏' },
      发证日期: { desc: '许可证核发日期', rule: '提取“发证日期”栏，统一为 YYYY-MM-DD' },
      有效期起: { desc: '有效期开始日期', rule: '提取“有效期起”栏，统一为 YYYY-MM-DD' },
      有效期止: { desc: '有效期截止日期', rule: '提取“有效期止 / 有效期至”栏，统一为 YYYY-MM-DD' },
      证件状态: { desc: '证件当前状态', rule: '根据有效期自动判断：已过期 / 有效 / 即将到期；原文有明确状态时优先取原文' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
    道路运输证: {
      道路运输证号: { desc: '道路运输证编号', rule: '提取“道路运输证号”栏' },
      车牌号: { desc: '号牌号码', rule: '提取“车牌号”栏，格式如“渝A12345”' },
      经营许可证号: { desc: '关联的经营许可证编号', rule: '提取“经营许可证号”栏' },
      车辆类型: { desc: '客车、货车、专项作业车等', rule: '提取“车辆类型”栏' },
      '吨（座）位': { desc: '核定吨位 / 座位数', rule: '提取“吨（座）位”栏，如“15吨”' },
      业户名称: { desc: '经营业户名称', rule: '提取“业户名称”栏' },
      经营地址: { desc: '公司实际经营地址', rule: '文档中出现公司地址、注册地址、单位地址等表述时，统一归入本字段' },
      经营范围: { desc: '许可经营范围', rule: '提取“经营范围”栏' },
      车辆尺寸: { desc: '外廓尺寸', rule: '提取“车辆尺寸”栏，如“11800×2550×3800mm”' },
      发证日期: { desc: '证件核发日期', rule: '提取“发证日期”栏，统一为 YYYY-MM-DD' },
      有效期止: { desc: '有效期截止日期', rule: '提取“有效期止 / 有效期至”栏，统一为 YYYY-MM-DD' },
      发证机关: { desc: '发证机关名称', rule: '提取“发证机关”栏' },
      上次审验日期: { desc: '上次年度审验日期', rule: '提取“上次审验日期”栏，统一为 YYYY-MM-DD' },
      下次审验日期: { desc: '下次年度审验日期', rule: '提取“下次审验日期”栏，统一为 YYYY-MM-DD' },
      技术评定等级: { desc: '车辆技术等级', rule: '提取“技术评定等级”栏，如“一级”' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
    保险单: {
      保单号: { desc: '保险单编号', rule: '提取“保单号”栏' },
      保险公司: { desc: '承保保险公司', rule: '提取“保险公司 / 保险人”栏' },
      保险类型: { desc: '交强险 / 商业险等', rule: '提取“保险类型”栏，如“机动车交通事故责任强制保险”' },
      车辆ID: { desc: '被保险车辆内部编号', rule: '保单未列明时留空，不猜测' },
      车牌号: { desc: '被保险车辆号牌', rule: '提取“车牌号”栏，格式如“渝A12345”' },
      VIN码: { desc: '车辆识别代号', rule: '提取“车架号 / VIN”栏，17 位字母数字组合' },
      被保险人名称: { desc: '被保险人名称', rule: '提取“被保险人”栏' },
      险种名称: { desc: '具体险种', rule: '提取“险种名称”栏，如“车辆损失险、第三者责任险”' },
      保额: { desc: '单险种保险金额', rule: '提取“保险金额”栏，保留数字与单位' },
      保费: { desc: '单险种保费', rule: '提取“保费”栏，保留数字与单位' },
      总保费: { desc: '保单合计保费', rule: '提取“总保费 / 合计保费”栏' },
      起保日期: { desc: '保险责任开始日期', rule: '提取“起保日期”栏，统一为 YYYY-MM-DD' },
      终保日期: { desc: '保险责任截止日期', rule: '提取“终保日期 / 到期日”栏，统一为 YYYY-MM-DD' },
      保单状态: { desc: '正常、退保、已到期等', rule: '根据有效期与原文识别；原文无则留空' },
      缴费状态: { desc: '已缴 / 未缴', rule: '识别“已缴 / 未缴 / 已支付”等表述' },
      发票号: { desc: '保费发票号码', rule: '提取“发票号”栏；原文无则留空' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
  },
  driver: {
    驾驶证: {
      驾驶证号: { desc: '驾驶证编号', rule: '提取“驾驶证号”栏' },
      司机姓名: { desc: '驾驶人姓名', rule: '提取“姓名”栏' },
      准驾车型: { desc: '准驾车型代号', rule: '提取“准驾车型”栏，如“A2”' },
      初次领证日期: { desc: '初次领证日期', rule: '提取“初次领证日期”栏，统一为 YYYY-MM-DD' },
      有效期起: { desc: '有效期开始日期', rule: '提取“有效期限”起始日期，统一为 YYYY-MM-DD' },
      有效期止: { desc: '有效期截止日期', rule: '提取“有效期限”截止日期，统一为 YYYY-MM-DD' },
      发证机关: { desc: '发证机关名称', rule: '提取“发证机关”栏' },
      驾驶证状态: { desc: '正常、注销、暂扣等', rule: '从状态描述中识别；原文无则留空' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
    从业资格证: {
      从业资格证号: { desc: '从业资格证编号', rule: '提取“从业资格证号”栏' },
      司机姓名: { desc: '持证人姓名', rule: '提取“姓名”栏' },
      从业资格类别: { desc: '资格类别', rule: '提取“从业资格类别”栏，如“道路旅客运输”' },
      准运范围: { desc: '准运范围', rule: '提取“准运范围”栏' },
      发证机关: { desc: '发证机关名称', rule: '提取“发证机关”栏' },
      发证日期: { desc: '证件核发日期', rule: '提取“发证日期”栏，统一为 YYYY-MM-DD' },
      有效期起: { desc: '有效期开始日期', rule: '提取“有效期起”栏，统一为 YYYY-MM-DD' },
      有效期止: { desc: '有效期截止日期', rule: '提取“有效期止”栏，统一为 YYYY-MM-DD' },
      证件状态: { desc: '证件当前状态', rule: '根据有效期判断：有效 / 已过期 / 即将到期；原文有明确状态时优先取原文' },
      remark: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
    },
  },
  maintain: {
    维修工单: {
      工单号: { desc: '维修工单编号', rule: '提取“工单号”栏' },
      车牌号: { desc: '维修车辆号牌', rule: '提取“车牌号”栏' },
      维修日期: { desc: '维修完成日期', rule: '提取“维修日期”栏，统一为 YYYY-MM-DD' },
      维修项目: { desc: '维修项目清单', rule: '提取全部维修项目，多个用“、”分隔' },
      工时费: { desc: '工时费用', rule: '提取“工时费”栏，保留数字与单位' },
      材料费: { desc: '材料费用', rule: '提取“材料费”栏，保留数字与单位' },
      总费用: { desc: '工单合计金额', rule: '提取“总费用 / 合计”栏' },
    },
    二级维护: {
      维护日期: { desc: '维护完成日期', rule: '提取“维护日期”栏，统一为 YYYY-MM-DD' },
      车牌号: { desc: '维护车辆号牌', rule: '提取“车牌号”栏' },
      维护项目: { desc: '维护项目清单', rule: '提取全部维护项目，多个用“、”分隔' },
      维护单位: { desc: '维护单位名称', rule: '提取“维护单位”栏' },
      下次维护日期: { desc: '下次维护日期', rule: '提取“下次维护日期”栏，统一为 YYYY-MM-DD' },
    },

  },
  // 发票档案：普通发票 / 专用发票共用一套字段口径（对齐 fleet_categories invoice 内置字段）
  invoice: {
    普通发票: {
      invoiceNo: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，纯数字或数字字母组合，如“26507000000266963455”' },
      invoiceDate: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      invoiceType: { desc: '发票类型', rule: '从 专用发票、普通发票、医疗收据、财政收据、其它票据 中识别；含通行费或电子发票归为普通发票' },
      amount: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数；多张发票分别取值' },
      taxRate: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%；多档税率以“、”分隔' },
      tax: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      totalAmount: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      buyerName: { desc: '购买方（受票方）名称', rule: '提取“购买方名称”栏' },
      sellerName: { desc: '销售方（开票方）名称', rule: '提取“销售方名称”栏' },
      issuer: { desc: '开票人姓名', rule: '提取“开票人”栏；原文无则留空' },
      remark: { desc: '备注', rule: '原文备注栏内容；如含车牌号、通行日期等运输信息以“车牌号：XX；通行日期起：XX；通行日期止：XX”格式拼接；无则留空' },
      extractStatus: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      tags: { desc: '发票标签', rule: '根据货物/劳务名称或销方类型归纳业务标签，如 差旅、办公用品、加油；多个标签以数组返回' },
      sellerTaxNo: { desc: '销售方纳税人识别号', rule: '提取“销售方纳税人识别号”栏' },
      buyerTaxNo: { desc: '购买方纳税人识别号', rule: '提取“购买方纳税人识别号”栏' },
      fileName: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
    专用发票: {
      invoiceNo: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，纯数字或数字字母组合，如“26507000000266963455”' },
      invoiceDate: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      invoiceType: { desc: '发票类型', rule: '从 专用发票、普通发票、医疗收据、财政收据、其它票据 中识别；含通行费或电子发票归为普通发票' },
      amount: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数；多张发票分别取值' },
      taxRate: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%；多档税率以“、”分隔' },
      tax: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      totalAmount: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      buyerName: { desc: '购买方（受票方）名称', rule: '提取“购买方名称”栏' },
      sellerName: { desc: '销售方（开票方）名称', rule: '提取“销售方名称”栏' },
      issuer: { desc: '开票人姓名', rule: '提取“开票人”栏；原文无则留空' },
      remark: { desc: '备注', rule: '原文备注栏内容；如含车牌号、通行日期等运输信息以“车牌号：XX；通行日期起：XX；通行日期止：XX”格式拼接；无则留空' },
      extractStatus: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      tags: { desc: '发票标签', rule: '根据货物/劳务名称或销方类型归纳业务标签，如 差旅、办公用品、加油；多个标签以数组返回' },
      sellerTaxNo: { desc: '销售方纳税人识别号', rule: '提取“销售方纳税人识别号”栏' },
      buyerTaxNo: { desc: '购买方纳税人识别号', rule: '提取“购买方纳税人识别号”栏' },
      fileName: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
    // 其它票据：系统内置兜底分类的泛用规则底稿（对齐 seed subs：invoice-misc）。
    // 规则回退链（分类→普通发票→整体规则）保证即使未保存本分类规则也能提取。
    其它票据: {
      invoiceNo: { desc: '票据号码', rule: '提取“票据号码 / 发票号码 / 编号”栏，数字或数字字母组合；原文无则留空' },
      invoiceDate: { desc: '开票/出票日期', rule: '提取“开票日期 / 出票日期 / 日期”栏，统一为 YYYY-MM-DD' },
      invoiceType: { desc: '票据类型', rule: '按票面标题识别：专用发票、普通发票、通行费发票、电费发票等；无法识别时保持为空' },
      amount: { desc: '不含税金额', rule: '提取“金额 / 不含税金额 / 合计金额（不含税）”栏，保留两位小数，不要货币符号；多张分别取值' },
      taxRate: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%、1%；多档税率以英文逗号拼接；原文无则留空' },
      tax: { desc: '税额', rule: '提取“税额”栏，保留两位小数；无则留空' },
      totalAmount: { desc: '价税合计 / 总金额', rule: '提取“价税合计 / 合计 / 总金额”栏，保留两位小数' },
      buyerName: { desc: '购买方（付款方）名称', rule: '提取“购买方 / 付款方 / 户名”栏；原文无则留空' },
      buyerTaxNo: { desc: '购买方纳税人识别号', rule: '提取“购买方纳税人识别号 / 统一社会信用代码”栏；原文无则留空' },
      sellerName: { desc: '销售方（收款方）名称', rule: '提取“销售方 / 收款方”栏；原文无则留空' },
      sellerTaxNo: { desc: '销售方纳税人识别号', rule: '提取“销售方纳税人识别号”栏；原文无则留空' },
      issuer: { desc: '开票人/经办人', rule: '提取“开票人 / 收款人 / 经办人”栏；原文无则留空' },
      remark: { desc: '备注', rule: '原文备注栏内容摘要；无法归入常规字段的有效业务信息（事由、经办人等）整合于此；无则留空' },
      extractStatus: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      tags: { desc: '票据标签', rule: '根据票面用途归纳业务标签，如 差旅、办公用品、加油；多个标签以数组返回' },
      items: { desc: '项目明细行', rule: '提取票面货物/服务明细：名称、数量、单价、税率；多行明细逐行输出；原文无则留空' },
      fileName: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
    // 水电发票：水电类细分分类底稿（subs 使用英文键名 invoiceNo/invoiceDate/...，
    // 与普通发票同字段集）。购买方为用电/用水企业，销售方为电网/供水公司，
    // 明细行用量信息格式化汇总至备注。
    水电发票: {
      invoiceNo: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，数字或数字字母组合；原文无则留空' },
      invoiceDate: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      invoiceType: { desc: '发票类型', rule: '识别为“水电发票（电费/水费）”；票面含“电费/水费”或供电/供水公司抬头即可判定' },
      amount: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数，不要货币符号' },
      taxRate: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%' },
      tax: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      totalAmount: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      buyerName: { desc: '购买方（受票方）名称', rule: '购买方通常为用电/用水的企业名称（户号对应单位），从“购买方/户名”栏识别提取' },
      buyerTaxNo: { desc: '购买方纳税人识别号', rule: '提取“购买方纳税人识别号 / 统一社会信用代码”栏' },
      sellerName: { desc: '销售方（开票方）名称', rule: '销售方通常为国家电网、南方电网或当地自来水/燃气公司，从“销售方”栏识别提取' },
      sellerTaxNo: { desc: '销售方纳税人识别号', rule: '提取“销售方纳税人识别号”栏' },
      issuer: { desc: '开票人', rule: '提取“开票人”栏；原文无则留空' },
      fileName: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
      extractStatus: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      tags: { desc: '发票标签', rule: '归为“水电费”类业务标签；多个标签以数组返回' },
      remark: { desc: '用电/用水明细汇总', rule: '请从票面的明细行中，尝试提取总用电量/用水量、单价及计费起止日期，并将这些信息格式化后汇总至备注字段中返回' },
      items: { desc: '项目明细行', rule: '提取票面货物/服务明细：名称、数量、单价、税率；多行明细逐行输出' },
    },
    // 电费发票：水电类细分分类底稿（subs 多为中文名，如 购买方税号/销售方名称）。
    // 购买方为用电企业、销售方为电网/供水公司，明细行信息格式化汇总至备注。
    电费发票: {
      发票号码: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，数字或数字字母组合；原文无则留空' },
      开票日期: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      发票类型: { desc: '发票类型', rule: '识别为“电费发票”；票面含“电费”或供电公司抬头即可判定' },
      金额: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数，不要货币符号' },
      税率: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%' },
      税额: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      价税合计: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      价税合计金额: { desc: '价税合计金额', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      购买方名称: { desc: '购买方（受票方）名称', rule: '购买方通常为用电/用水的企业名称，从“购买方/户名”栏识别提取' },
      购买方税号: { desc: '购买方纳税人识别号', rule: '提取“购买方纳税人识别号 / 统一社会信用代码”栏' },
      销售方名称: { desc: '销售方（开票方）名称', rule: '销售方通常为国家电网、南方电网或当地自来水/燃气公司，从“销售方”栏识别提取' },
      销售方税号: { desc: '销售方纳税人识别号', rule: '提取“销售方纳税人识别号”栏' },
      开票人: { desc: '开票人姓名', rule: '提取“开票人”栏；原文无则留空' },
      备注: { desc: '用电/用水明细汇总', rule: '请从票面的明细行中，尝试提取总用电量/用水量、单价及计费起止日期，并将这些信息格式化后汇总至备注字段中返回' },
      提取状态: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      标签: { desc: '发票标签', rule: '归为“水电费”类业务标签；多个标签以数组返回' },
      源文件名: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
    // 通行费发票：细分分类底稿。车牌号无独立表头，藏在备注栏文字中，
    // 必须从备注分离提取到独立字段（动态字段扩展桶随记录持久化）。
    通行费发票: {
      发票号码: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，数字或数字字母组合' },
      invoiceNo: { desc: '发票号码', rule: '提取“发票号码 / 发票号”栏，数字或数字字母组合' },
      开票日期: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      invoiceDate: { desc: '开票日期', rule: '提取“开票日期”栏，统一为 YYYY-MM-DD' },
      发票类型: { desc: '发票类型', rule: '识别为“通行费发票”；票面含“通行费”或高速/桥梁收费抬头即可判定' },
      invoiceType: { desc: '发票类型', rule: '识别为“通行费发票”；票面含“通行费”或高速/桥梁收费抬头即可判定' },
      车牌号: { desc: '通行车辆号牌号码', rule: '格式通常为省份简称加字母和数字（如：渝C87567）。如果票面上没有独立的‘车牌号’表头，请务必仔细阅读‘备注’栏文字，从中分离并提取出车牌号，直接输出车牌号本身，不要带有‘车牌号：’等前缀' },
      金额: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数，不要货币符号' },
      amount: { desc: '不含税金额', rule: '提取“金额 / 不含税金额”栏，保留两位小数，不要货币符号' },
      税率: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%' },
      taxRate: { desc: '适用税率', rule: '提取“税率”栏，如 13%、9%、6%、3%' },
      税额: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      tax: { desc: '税额', rule: '提取“税额”栏，保留两位小数' },
      价税合计: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      totalAmount: { desc: '价税合计', rule: '提取“价税合计 / 合计”栏，保留两位小数' },
      购买方名称: { desc: '购买方（受票方）名称', rule: '提取“购买方名称”栏' },
      buyerName: { desc: '购买方（受票方）名称', rule: '提取“购买方名称”栏' },
      销售方名称: { desc: '销售方（开票方）名称', rule: '提取“销售方名称”栏' },
      sellerName: { desc: '销售方（开票方）名称', rule: '提取“销售方名称”栏' },
      开票人: { desc: '开票人姓名', rule: '提取“开票人”栏；原文无则留空' },
      issuer: { desc: '开票人姓名', rule: '提取“开票人”栏；原文无则留空' },
      备注: { desc: '备注', rule: '保留票面备注原文；若备注中包含车牌号、通行日期等关键信息，同时按字段口径输出到对应独立字段' },
      remark: { desc: '备注', rule: '保留票面备注原文；若备注中包含车牌号、通行日期等关键信息，同时按字段口径输出到对应独立字段' },
      提取状态: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      extractStatus: { desc: '提取状态', rule: '系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写' },
      标签: { desc: '发票标签', rule: '归为“通行费”类业务标签；多个标签以数组返回' },
      tags: { desc: '发票标签', rule: '归为“通行费”类业务标签；多个标签以数组返回' },
      源文件名: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
      fileName: { desc: '源文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
  },
  // 合同档案：字段口径对齐 fleet_categories contract 内置字段（35 项，与编辑抽屉表单同源）
  contract: {
    服务合同: {
      contract_no: { desc: '合同编号', rule: '提取合同封面或页眉的「合同编号 / 协议编号 / 合同号」；没有编号时留空，不要编造。' },
      contract_name: { desc: '合同标题', rule: '提取合同标题（通常在首页最上方，如「厂房租赁协议书」「设备采购合同」）；标题含书名号时去掉书名号。' },
      contract_type: { desc: '合同归档类型', rule: '按合同名称与正文判断：含「采购」→采购合同；含「销售/售」→销售合同；含「租赁/租」→租赁合同；含「服务」→服务合同；都不含→其它合同。' },
      party_a: { desc: '甲方（出租方/委托方/采购方）名称', rule: '提取「甲方」一方的完整企业名称；若合同用「出租方/委托方/采购方」表述，同样归入本字段。' },
      party_b: { desc: '乙方（承租方/受托方/供应方）名称', rule: '提取「乙方」一方的完整企业名称；若合同用「承租方/受托方/供应方」表述，同样归入本字段。' },
      sign_date: { desc: '合同签订/签署日期', rule: '提取合同尾部或首页的「签订日期 / 签署日期」，统一为 YYYY-MM-DD；只有年月时补 01 日。' },
      expire_date: { desc: '合同到期/终止日期', rule: '提取「到期日期 / 合同终止日期 / 有效期至」；租赁合同优先取「租赁期限自…至…」的结束日期，统一为 YYYY-MM-DD。' },
      amount: { desc: '合同总金额（元）', rule: '提取「合同金额 / 合同总价 / 价款合计」的数字，单位元，去掉千分位与货币符号；大小写金额不一致时以大写为准。' },
      extract_status: { desc: '提取状态', rule: '由系统写入，不需要从原文提取。' },
      performance_status: { desc: '履约状态（由到期日期派生）', rule: '由系统按到期日期计算（履行中 / 即将到期 / 已超期），不需要从原文提取。' },
      tags: { desc: '记录标签（系统字段）', rule: '由用户维护，不需要从原文提取。' },
      party_a_contact: { desc: '甲方联系人 / 经办人', rule: '提取甲方一侧的「经办人 / 联系人 / 委托代理人」姓名。' },
      party_a_tax_no: { desc: '甲方统一社会信用代码 / 纳税人识别号', rule: '提取甲方名称附近或发票信息栏的 18 位统一社会信用代码。' },
      party_b_contact: { desc: '乙方联系人 / 经办人', rule: '提取乙方一侧的「经办人 / 联系人 / 委托代理人」姓名。' },
      party_b_tax_no: { desc: '乙方统一社会信用代码 / 纳税人识别号', rule: '提取乙方名称附近的 18 位统一社会信用代码。' },
      party_a_address: { desc: '甲方住所 / 注册地址', rule: '提取甲方的「地址 / 住所 / 注册地址」。' },
      party_a_phone: { desc: '甲方联系电话', rule: '提取甲方的「电话 / 联系电话」。' },
      party_a_bank: { desc: '甲方开户银行', rule: '提取甲方的「开户行 / 开户银行」。' },
      party_a_account: { desc: '甲方银行账号', rule: '提取甲方的「账号 / 银行账号」。' },
      party_b_address: { desc: '乙方住所 / 注册地址', rule: '提取乙方的「地址 / 住所 / 注册地址」。' },
      party_b_phone: { desc: '乙方联系电话', rule: '提取乙方的「电话 / 联系电话」。' },
      party_b_bank: { desc: '乙方开户银行', rule: '提取乙方的「开户行 / 开户银行」。' },
      party_b_account: { desc: '乙方银行账号', rule: '提取乙方的「账号 / 银行账号」。' },
      payment_method: { desc: '付款方式', rule: '从「一次性 / 分期 / 按进度」中选择，与原文最接近的一项；无法判断时留空。' },
      sign_location: { desc: '合同签订地点', rule: '提取合同尾部的「签订地点 / 签署地点」。' },
      summary: { desc: '合同摘要（复用知识库文档摘要）', rule: '原文自带「摘要 / 内容摘要」时提取该段；否则留空，系统会用知识库已生成的文档摘要回填。' },
      fileName: { desc: '文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
    租赁合同: {
      contract_no: { desc: '合同编号', rule: '提取合同封面或页眉的「合同编号 / 协议编号 / 合同号」；没有编号时留空，不要编造。' },
      contract_name: { desc: '合同标题', rule: '提取合同标题（通常在首页最上方，如「厂房租赁协议书」「设备租赁合同」）；标题含书名号时去掉书名号。' },
      contract_type: { desc: '合同归档类型', rule: '按合同名称与正文判断：含「租赁/租」→租赁合同；含「采购」→采购合同；含「服务」→服务合同；都不含→其它合同。' },
      party_a: { desc: '甲方（出租方）名称', rule: '提取「甲方 / 出租方」一方的完整企业名称。' },
      party_b: { desc: '乙方（承租方）名称', rule: '提取「乙方 / 承租方」一方的完整企业名称。' },
      sign_date: { desc: '签订日期', rule: '提取「签订日期 / 起租日期」栏，统一为 YYYY-MM-DD；只有年月时补 01 日。' },
      expire_date: { desc: '到期日期', rule: '提取「租赁期限自…至…」「到期日期 / 合同终止日期」的结束日期，统一为 YYYY-MM-DD。' },
      amount: { desc: '合同总金额（元）', rule: '提取「租金总额 / 合同金额 / 价款合计」的数字，单位元，去掉千分位与货币符号。' },
      extract_status: { desc: '提取状态', rule: '由系统写入，不需要从原文提取。' },
      performance_status: { desc: '履约状态（由到期日期派生）', rule: '由系统按到期日期计算（履行中 / 即将到期 / 已超期），不需要从原文提取。' },
      tags: { desc: '记录标签（系统字段）', rule: '由用户维护，不需要从原文提取。' },
      party_a_contact: { desc: '甲方联系人 / 经办人', rule: '提取甲方（出租方）一侧的「经办人 / 联系人」姓名。' },
      party_a_tax_no: { desc: '甲方统一社会信用代码 / 纳税人识别号', rule: '提取甲方（出租方）名称附近的 18 位统一社会信用代码。' },
      party_b_contact: { desc: '乙方联系人 / 经办人', rule: '提取乙方（承租方）一侧的「经办人 / 联系人」姓名。' },
      party_b_tax_no: { desc: '乙方统一社会信用代码 / 纳税人识别号', rule: '提取乙方（承租方）名称附近的 18 位统一社会信用代码。' },
      party_a_address: { desc: '甲方住所 / 注册地址', rule: '提取甲方（出租方）的「地址 / 住所 / 注册地址」。' },
      party_a_phone: { desc: '甲方联系电话', rule: '提取甲方（出租方）的「电话 / 联系电话」。' },
      party_a_bank: { desc: '甲方开户银行', rule: '提取甲方（出租方）的「开户行 / 开户银行」。' },
      party_a_account: { desc: '甲方银行账号', rule: '提取甲方（出租方）的「账号 / 银行账号」。' },
      party_b_address: { desc: '乙方住所 / 注册地址', rule: '提取乙方（承租方）的「地址 / 住所 / 注册地址」。' },
      party_b_phone: { desc: '乙方联系电话', rule: '提取乙方（承租方）的「电话 / 联系电话」。' },
      party_b_bank: { desc: '乙方开户银行', rule: '提取乙方（承租方）的「开户行 / 开户银行」。' },
      party_b_account: { desc: '乙方银行账号', rule: '提取乙方（承租方）的「账号 / 银行账号」。' },
      payment_method: { desc: '付款方式', rule: '从「一次性 / 分期 / 按进度」中选择，与原文最接近的一项；无法判断时留空。' },
      sign_location: { desc: '合同签订地点', rule: '提取合同尾部的「签订地点 / 签署地点」。' },
      summary: { desc: '合同摘要（复用知识库文档摘要）', rule: '原文自带「摘要 / 内容摘要」时提取该段；否则留空，系统会用知识库已生成的文档摘要回填。' },
      fileName: { desc: '文件名', rule: '取上传文件的原始文件名，无需识别' },
    },
  },
}

// 常用提取规则写法示例（展开“更多”展示）
const ruleExamples = [
  { name: '归并提取', text: '文档中出现多个同义表述（如公司地址、注册地址、单位地址）时，统一归入同一字段' },
  { name: '格式统一', text: '日期统一为 YYYY-MM-DD；金额保留两位小数；去除多余空白与标点' },
  { name: '缺省留空', text: '原文未出现该字段信息时输出空，不猜测、不编造' },
  { name: '多值提取', text: '同一字段存在多个值时，全部提取并用“、”分隔' },
  { name: '长文摘要', text: '备注等长文本仅保留关键片段（建议不超过 50 字）' },
]

const typeOptions = [
  { label: '字符串', value: 'string' },
  { label: '数组', value: 'array' },
  { label: '日期', value: 'date' },
  { label: '数字', value: 'number' },
]
const slots = ['{{fields_schema}}', '{{document_text}}']

const categories = ref<Record<string, any[]>>({ vehicle: [], driver: [], maintain: [] })
// 分类字典缓存（30s TTL + 并发去重）：切 Tab/切 scope 会反复 refresh。
const catalog = useExtractCatalog()

// 分组别名：与设置抽屉 FleetCertPanel 同 key（`${scope}:${builtinGroupKey}`），builtin group_key = company/driver/maintain
const BUILTIN_GROUP_KEY: Record<string, string> = { vehicle: 'company', driver: 'driver', maintain: 'maintain' }
// 非车队 scope（如 invoice）：类型 = categories[scope] 分类列表（含新增自定义类型），规则按类型存取
const isFleetScope = computed(() => !!BUILTIN_GROUP_KEY[props.scope])
const groupAliases = ref<Record<string, string>>({})
async function loadGroupAliases() {
  try {
    const res: any = await listFleetGroupAliases()
    const arr = res?.data || res || []
    const m: Record<string, string> = {}
    for (const it of arr) m[`${it.scope}:${it.group_key}`] = it.name
    groupAliases.value = m
  } catch { groupAliases.value = {} }
}

// 车辆档案统一管理「公司证照(vehicle)」与「司机证照(driver)」两组类型；
// 下拉按组展示，每个类型携带其真实 scope，读写规则均按真实 scope 走。
const TYPE_GROUP_META = [
  { scope: 'vehicle', fallback: '公司证照' },
  { scope: 'driver', fallback: '司机证照' },
  { scope: 'maintain', fallback: '维保文件' },
] as const
function groupTitle(scope: string, fallback: string): string {
  return groupAliases.value[`${scope}:${BUILTIN_GROUP_KEY[scope] || scope}`] || fallback
}

// v-model 存复合值 `${scope}__${name}`，避免公司/司机同名类型在下拉中冲突
function typeComposite(scope: string, name: string) {
  return `${scope}__${name}`
}

const certTypeOptions = computed(() => {
  if (!isFleetScope.value) {
    // 非车队 scope：类型 = 分类列表（含新增自定义类型），平铺无分组
    const arr = categories.value[props.scope] || []
    const seen = new Set<string>()
    const children: { label: string; value: string }[] = []
    arr.forEach((c: any) => {
      if (c && c.name && c.enabled !== false && !seen.has(c.name)) {
        seen.add(c.name)
        children.push({ label: c.name, value: typeComposite(props.scope, c.name) })
      }
    })
    return children
  }
  return TYPE_GROUP_META
    .filter((g) => g.scope === props.scope)
    .map(({ scope, fallback }) => {
    const title = groupTitle(scope, fallback)
    const names: string[] = []
    const seen = new Set<string>()
    // 顺序唯一权威：categories 后端顺序（sort_order，用户拖动后持久化）
    const builtinKeys = Object.keys(DEFAULT_FIELDS[scope] || {})
    const sortedCats = sortCertsByLocalOrder(scope, (categories.value[scope] || []) as any[], builtinKeys) as any[]
    sortedCats.forEach((c: any) => {
      if (c && c.name && c.enabled !== false && !seen.has(c.name)) {
        seen.add(c.name); names.push(c.name)
      }
    })
    // 内置未入库兜底：按内置顺序追加
    builtinKeys.forEach((bKey) => {
      const exists = sortedCats.some((c: any) => c && (c.builtin_key === bKey || c.name === bKey))
      if (!exists && !seen.has(bKey)) { seen.add(bKey); names.push(bKey) }
    })
    return { group: title, children: names.map((n) => ({ label: n, value: typeComposite(scope, n) })) }
  }).filter((g) => (g.children || []).length > 0)
})

// 当前选中类型的真实 scope 与纯类型名（复合值解析）
const currentScope = computed(() => {
  const s = certType.value.split('__', 1)[0]
  return s && s !== certType.value ? s : props.scope
})
const certTypeName = computed(() => {
  const parts = certType.value.split('__')
  return parts.length > 1 ? parts.slice(1).join('__') : certType.value
})
// 跨分类公共规则底稿（scope 级兜底）；未声明的 scope 为空对象
const commonDrafts = computed<Record<string, RuleDraft>>(
  () => (getScope(currentScope.value)?.commonRuleDrafts || {}) as Record<string, RuleDraft>,
)

const certType = ref('')
const fields = ref<ExtractFieldConfig[]>([])
const advancedEnabled = ref(false)
const promptTemplate = ref('')
const saving = ref(false)
const testVisible = ref(false)
const loadingCfg = ref(false)
const editingName = ref('')

// 高级模板防呆：非空但未引用字段 schema 插槽时给出警告（字段规则以后端拼装的 schema 为准）
const templateWarning = computed(() => {
  const t = (promptTemplate.value || '').trim()
  if (!t) return ''
  if (!t.includes('{{fields_schema}}')) {
    return '模板未包含 {{fields_schema}} 插槽，字段规则将无法注入；建议保留该插槽，避免模型字段缺失。'
  }
  return ''
})

function resetPromptTemplate() {
  promptTemplate.value = ''
  MessagePlugin.success('已恢复默认模板，将使用系统默认字段规则拼装')
}
function toggleEdit(name: string) {
  editingName.value = editingName.value === name ? '' : name
}

// 当前选中分类的 subs（字段名/中文名/子项的唯一权威源）
function currentCategory(): any {
  const scope = currentScope.value
  const list = categories.value[scope] || []
  let c = list.find((x: any) => x.name === certTypeName.value)
  if (!isFleetScope.value && !c) {
    c = list.find((x: any) => Array.isArray(x.subs)) || list[0]
  }
  return c
}
/** 分类里给该字段配置的中文表头（无则空串） */
function categoryLabelOf(n: string): string {
  const subs: any[] = currentCategory()?.subs || []
  const want = String(n || '').trim().toLowerCase()
  const hit = subs.find((x: any) => String(x?.name || '').trim().toLowerCase() === want)
  return String(hit?.label || '').trim()
}
// 字段中文名：**已加载行上的 label 优先**（由 subs.label 派生，含用户改名「摘要2」），
// 其次分类自定义 label，再次契约字典（camelCase / snake_case 都认）。
// 之所以先查 fields.value：改名后静态字典仍会把「摘要2」查回「摘要」，
// 只有行上的 label 才是用户在「字段定义」里保存的那个值。
function fieldLabel(n: string): string {
  const row = fields.value.find((f) => f.name === n)
  const rowLabel = String(row?.label || '').trim()
  if (rowLabel) return rowLabel
  return categoryLabelOf(n) || extractFieldLabel(props.scope, n)
}

/** subs 的 data_type → 提取配置的 type 方言（text↔string 归一）。 */
function subsTypeToExtractType(dataType: string): string {
  switch (String(dataType || '').trim()) {
    case 'number': return 'number'
    case 'date': return 'date'
    case 'array': return 'array'
    case 'items': return 'items'
    case 'text': return 'string'
    default: return ''
  }
}

// 字段级规则（必填/正则）是否可用。该模块已接入通用校验 + 字段确实是标准契约键。
// 自定义扩展中文 key 只进 Prompt，后端补洞器装不下它 —— UI 必须置灰而不是装作能用。
const isStdField = (name: string) => fieldRulesSupported(props.scope) && isStandardField(props.scope, name)

// 正则可配判定**独立于**必填判定：必填走通用校验器认的全部 22 个标准键，
// 正则走后端补洞器实际支持的 8 个 invoiceFieldOrder 键（extract_regex_fallback.go）。
// 此前二者共用 isStdField，导致用户给 totalAmount 配了正则却不执行（静默失效）。
const regexPatchable = (name: string) => fieldRulesSupported(props.scope) && isRegexPatchable(props.scope, name)

const requiredTip = computed(() =>
  fieldRulesSupported(props.scope)
    ? '开启后该字段为空会产出 error 级校验告警'
    : '本模块暂未接入字段级校验（仅发票/合同/制度/奖惩生效）',
)

function regexTip(name: string): string {
  if (regexPatchable(name)) return '模型未提取到该字段时，用此正则从原文唯一命中回填（Go RE2 语法）'
  return regexDisabledReason(props.scope, name)
}

function regexHintOf(f: ExtractFieldConfig): string {
  if (!regexPatchable(f.name)) return ''
  return regexSyntaxHint(f.regex || '')
}

function applyRegexPreset(f: ExtractFieldConfig, p: RegexPreset) {
  if (!regexPatchable(f.name)) {
    MessagePlugin.warning(regexDisabledReason(props.scope, f.name) || '该字段不支持正则兜底')
    return
  }
  f.regex = p.value
}

function typeLabel(t: string) {
  return typeOptions.find((o) => o.value === t)?.label || '字符串'
}

function applyExample(f: ExtractFieldConfig, text: string) {
  f.rule = f.rule ? `${f.rule}；${text}` : text
}

function makeField(name: string): ExtractFieldConfig {
  return { name, desc: '', type: 'string', rule: '', enabled: true, required: false, regex: '', sub_fields: [] }
}

/** 该行是否明细字段（决定展开区是否显示「明细子项规则」）。 */
function isItemsField(f: ExtractFieldConfig): boolean {
  return f.type === 'items' || f.name === 'items'
}

/** 把证照配置里的 sub_fields 原样搬成编辑态（结构以证照配置为权威）。 */
function subFieldsFromSubs(raw: unknown): ExtractSubFieldConfig[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter((s: any) => s && typeof s === 'object' && String(s.key || '').trim())
    .map((s: any) => {
      const sf: ExtractSubFieldConfig = {
        key: String(s.key).trim(),
        label: String(s.label || s.key).trim(),
        data_type: s.data_type || s.dataType || 'text',
        width: Number(s.width) || 0,
        rule: String(s.rule || '').trim(),
        regex: String(s.regex || '').trim(),
      }
      if (s.enabled === false) sf.enabled = false
      return sf
    })
}

/**
 * 子项口径合并：结构/文案以证照配置为准，口径（rule/regex/enabled）以提取配置为准。
 * 与后端 handler.injectInvoiceItemSubFields 的同名合并方向一致，避免两边互相覆盖。
 */
function mergeSubFieldRules(base: ExtractSubFieldConfig[], saved?: ExtractSubFieldConfig[]): ExtractSubFieldConfig[] {
  if (!saved?.length) return base
  const byKey = new Map(saved.map((s) => [String(s.key || ''), s]))
  return base.map((b) => {
    const s = byKey.get(b.key)
    if (!s) return b
    const out: ExtractSubFieldConfig = { ...b }
    if (s.rule) out.rule = String(s.rule)
    if (s.regex) out.regex = String(s.regex)
    if (s.desc) out.desc = String(s.desc)
    if (s.enabled === false) out.enabled = false
    return out
  })
}

// 字段名权威 = 证照配置（fleet_categories.subs，启用字段优先），未入库类型回退内置底稿。
// 与证照配置面板共用同一来源规则，保证两处字段永远一致。
//
// 六同步（2026-10-21）：**subs 的 label 与 data_type 都是权威**。
//   · label：用户在「字段定义」把「摘要」改成「摘要2」，规则面板必须显示摘要2；
//   · data_type：用户在字段定义把类型改成 array，规则面板的类型必须跟着变——
//     此前这里让 extract_config.type 优先，改类型后规则面板仍显示「字符串」。
function authorityFields(): { name: string; label: string; enabled: boolean; dataType: string; subFields: ExtractSubFieldConfig[] }[] {
  const scope = currentScope.value
  // 非车队 scope：按选中类型取 subs，未选中/无匹配回退第一个分类
  const c = currentCategory()
  if (c && Array.isArray(c.subs) && c.subs.length) {
    const seen = new Set<string>()
    return c.subs
      .filter((s: any) => {
        const n = String(s?.name || '').trim()
        return n && !seen.has(n) && (seen.add(n), true)
      })
      .map((s: any) => ({
        name: String(s.name || '').trim(),
        label: String(s.label || '').trim(),
        enabled: s.enabled !== false,
        dataType: s.data_type || 'text',
        subFields: subFieldsFromSubs(s.sub_fields),
      }))
  }
  if (isFleetScope.value) {
    const defaults = (DEFAULT_FIELDS[scope] || {})[certTypeName.value] || []
    const seen = new Set<string>()
    return defaults
      .map((n) => ({ name: n, label: '', enabled: true, dataType: 'text', subFields: [] as ExtractSubFieldConfig[] }))
      .filter((s: any) => s.name && !seen.has(s.name) && seen.add(s.name))
  }
  return []
}

function insertSlot(slot: string) {
  promptTemplate.value += slot
}

const TEMPLATE_EXAMPLE = `你是一个证件信息提取助手。请从以下文档中提取结构化信息：{{document_text}}

需要提取的字段如下（{{fields_schema}}）：
- 严格按字段名输出 JSON 对象
- 未找到的字段输出 null，不要臆造内容
- 日期统一为 YYYY-MM-DD
- 金额与数字保留原文精度`

function applyTemplateExample() {
  promptTemplate.value = TEMPLATE_EXAMPLE
}

async function ensureKb() {
  if (resolvedKbId.value) return resolvedKbId.value
  // 父组件直接传入 kbId（发票等业务模块与列表同库），免去按名称查找
  if (props.kbId) {
    resolvedKbId.value = props.kbId
    return resolvedKbId.value
  }
  try {
    const res: any = await listKnowledgeBases({ page: 1, page_size: 100 } as any)
    const list = Array.isArray(res) ? res : res?.data || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME.value) : null
    if (found) resolvedKbId.value = found.id
  } catch (e) {
    console.error('load kb failed', e)
  }
  return resolvedKbId.value
}

async function loadConfig() {
  if (!certType.value) {
    fields.value = []
    advancedEnabled.value = false
    promptTemplate.value = ''
    return
  }
  await ensureKb()
  if (!resolvedKbId.value) return
  loadingCfg.value = true
  try {
    const res: any = await getExtractConfig(resolvedKbId.value, currentScope.value, certTypeName.value)
    const data = res?.data
    const cfgMap = new Map<string, any>()
    if (data && Array.isArray(data.fields)) {
      for (const f of data.fields) {
        if (f?.name) cfgMap.set(String(f.name), f)
      }
    }
    // 字段名以证照配置为准：subs 或内置底稿；提取配置只提供 desc/type/rule 增强
    const auth = authorityFields()
    const draft = (DEFAULT_RULES[currentScope.value] || {})[certTypeName.value] || {}
    fields.value = auth.map((a) => {
      const c = cfgMap.get(a.name)
      // 底稿别名回退：自定义分类 subs 可能为中文（车牌号）或英文（invoiceNo），
      // 精确 miss 时按最长包含命中（与后端 invoiceFieldAliases 同思路），仅影响 desc/rule 展示
      const d = draftByKeyForms(draft, a.name)
        || draftByContains(draft, a.name)
        || draftByKeyForms(commonDrafts.value, a.name)
        || {}
      return {
        name: a.name,
        // 字段中文名 = 分类 label（用户改名后的）→ 契约字典 → 原名。
        // 规则面板表头直接用它，不再二次查字典（那会把「摘要2」盖回「摘要」）。
        label: a.label || categoryLabelOf(a.name) || extractFieldLabel(currentScope.value, a.name),
        desc: c?.desc || d.desc || '',
        // 类型以 subs.data_type 为权威（六同步）：用户在「字段定义」把 text 改成 array，
        // 规则面板必须随即显示「数组」。extract_config.type 只在 subs 没有该字段时兜底。
        type: subsTypeToExtractType(a.dataType) || c?.type || 'string',
        rule: c?.rule || d.rule || '',
        enabled: c && c.enabled !== undefined ? !!c.enabled : a.enabled,
        // 必填/正则兜底：配置里有就沿用（历史配置没有这两个键 → 关闭/空），
        // 不写进 DEFAULT_RULES 底稿：默认开必填会让存量文档一上线就满屏 error。
        regex: c?.regex || '',
        required: c && c.required !== undefined ? !!c.required : false,
        // 明细子项：结构来自证照配置，规则/正则来自提取配置（两端都为空就是空数组）
        sub_fields: mergeSubFieldRules(a.subFields, c?.sub_fields),
      }
    })
    if (!fields.value.length) fields.value.push(makeField(''))
    advancedEnabled.value = false
    promptTemplate.value = data?.prompt_template || ''
  } catch (e) {
    console.error('load extract config failed', e)
    MessagePlugin.error('加载提取规则失败')
  } finally {
    loadingCfg.value = false
  }
}

async function onCertTypeChange() {
  // 切换分类时清掉上一分类的 AI 填充高亮与展开态：它们属于旧分类的字段清单，
  // 留着会让用户误以为这些行刚被新分类的 AI 改过。
  aiFilled.value = new Set()
  editingName.value = ''
  testVisible.value = false
  await loadConfig()
}

/**
 * 底稿按 key 直查，三种写法都试（原名 → camelCase → snake_case）。
 *
 * 为什么需要：分类 subs 历史上混用两种写法（invoiceNo / invoice_code），
 * 而 DEFAULT_RULES 的键只写了一种。只查一种，另一种就渲染成「描述 —」的裸英文行。
 */
function draftByKeyForms(draft: Record<string, any>, name: string): any {
  const k = String(name || '').trim()
  if (!k) return undefined
  if (draft[k]) return draft[k]
  const camel = toCamelKey(k)
  if (draft[camel]) return draft[camel]
  const snake = toSnakeKey(k)
  if (draft[snake]) return draft[snake]
  return undefined
}

// 底稿别名回退：subs 名（车牌号/invoiceNo）与底稿键存在包含关系时，
// 按最长包含命中返回对应底稿条目；无命中返回 null（与后端 invoiceFieldAliases 同思路）。
function draftByContains(draft: Record<string, any>, name: string): any {
  let best: any = null
  let bestLen = 0
  for (const [k, v] of Object.entries(draft)) {
    if (k !== name && (k.includes(name) || name.includes(k))) {
      const len = Math.min(k.length, name.length)
      if (len > bestLen) {
        best = v
        bestLen = len
      }
    }
  }
  // 关键：无命中必须返回 null。此前返回 {}，而空对象是**真值**，
  // 让 `draftByKeyForms(...) || draftByContains(...) || 公共底稿` 这条链在第二步就短路，
  // 公共底稿永远不会被查到（发票代码一行的描述因此一直是「—」）。
  return best || null
}

async function save() {
  if (!certType.value) return
  await ensureKb()
  if (!resolvedKbId.value) {
    MessagePlugin.error(`未找到${KB_NAME.value}知识库`)
    return
  }
  // 保存前按「当前启用字段」清洗：剔除已禁用/已删除的旧字段（如被停用的 invoiceNo），
  // 避免提交不在证照配置中的字段触发后端 400 校验（证照配置是字段名的唯一权威源）。
  // 分类未入库（authorityFields 为空）时不过滤，保持旧行为整体提交。
  const activeNames = new Set(
    authorityFields()
      .filter((a) => a.enabled)
      .map((a) => a.name),
  )
  const cleanFields = fields.value
    .map((f) => ({ ...f }))
    .filter((f) => activeNames.size === 0 || activeNames.has(f.name))
  const payload = {
    scope: currentScope.value,
    cert_type: certTypeName.value,
    fields: cleanFields,
    advanced_enabled: advancedEnabled.value,
    prompt_template: promptTemplate.value,
  }
  saving.value = true
  try {
    await saveExtractConfig(resolvedKbId.value, payload)
    aiFilled.value = new Set()
    MessagePlugin.success('提取规则已保存')
  } catch (e: any) {
    console.error('save extract config failed', e)
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function toggleTest() {
  if (!certType.value) return
  testVisible.value = !testVisible.value
}

/** 测试面板里「确认规则并保存」成功后同步刷新（规则可能被测试面板写过）。 */
function onTestPanelSaved() {
  void loadConfig()
}

// ---- 智能配置规则（直接回填：只填表，不落库；落库仍由用户点「保存」触发） ----
//
// 动线为什么这么改：原先"生成 → 中间绿色预览框勾选 → 应用 → 再回表格"绕了两跳，
// 用户看不到建议落到哪一行。现在生成即填进表格对应行（空值才填，保护手写内容），
// 更新行加 AI 徽章与高亮、自动展开第一行，核对后直接保存。
/** 本行正在单字段生成（行级 loading，不阻塞其它行）。 */
const aiFieldBusy = ref('')
/** 本次被 AI 填充过的字段名（用于徽章/高亮，保存或重新生成时清空）。 */
const aiFilled = ref<Set<string>>(new Set())
const sampleTextAvailable = computed(() => !!readSampleText(certTypeName.value))
const aiSampleHint = computed(() =>
  sampleTextAvailable.value
    ? '已缓存该分类的样票文本：生成的正则会做唯一命中校验。点击可清除缓存。'
    : '尚未缓存样票文本。可先在「字段定义」Tab 用「智能生成字段」上传样票，或直接生成（此时正则无法验证唯一命中，会被全部丢弃）。',
)
/** 取当前分类的样票文本（缓存优先，其次调用方传入）。 */
function resolveSampleText(payload?: { sampleText?: string }) {
  const sample = String(payload?.sampleText || readSampleText(certTypeName.value) || '')
  if (payload?.sampleText && certTypeName.value) cacheSampleText(certTypeName.value, sample)
  return sample
}

/**
 * 批量智能生成规则：结果直接回填 fields（overwrite=false 只填空值）。
 * 返回实际填充的字段名列表。
 */
async function fillRulesFromAI(
  targets: Array<{ name: string }>,
  overwrite: boolean,
  payload?: { sampleText?: string },
): Promise<{ filled: string[]; total: number }> {
  await ensureKb()
  if (!resolvedKbId.value) throw new Error(`未找到${KB_NAME.value}知识库`)
  const res: any = await aiSuggestRules(resolvedKbId.value, {
    scope: currentScope.value,
    cert_type: certTypeName.value,
    fields: targets.map((f) => {
      const row = fields.value.find((x) => x.name === f.name)
      return { name: f.name, data_type: row?.type || 'string' }
    }),
    sample_text: resolveSampleText(payload),
    overwrite,
  })
  const data = (res?.data || res) as AiSuggestRulesResult
  if (!data || !Array.isArray(data.fields)) throw new Error('建议结果格式异常')

  const wanted = new Set(targets.map((t) => t.name))
  const byName = new Map(data.fields.filter((f) => !!f.name && wanted.has(f.name)).map((f) => [f.name, f]))
  const filled: string[] = []
  fields.value = fields.value.map((f) => {
    const sug = byName.get(f.name)
    if (!sug) return f
    const next: ExtractFieldConfig = { ...f }
    let changed = false
    // overwrite=true（单字段）时直接覆盖；批量只补空值，避免冲掉手写配置
    if (sug.desc && (overwrite || !next.desc?.trim())) { next.desc = sug.desc; changed = true }
    if (sug.rule && (overwrite || !next.rule?.trim())) { next.rule = sug.rule; changed = true }
    if (sug.regex && sug.regex_status === 'accepted' && (overwrite || !next.regex?.trim())) {
      next.regex = sug.regex
      changed = true
    }
    if (sug.required && !next.required) { next.required = true; changed = true }
    if (changed) filled.push(f.name)
    return next
  })

  if (data.prompt_template && data.prompt_template.trim()) {
    // 只填空的模板不覆盖用户已保存的高级模板
    if (!promptTemplate.value.trim()) promptTemplate.value = data.prompt_template.trim()
  }
  return { filled, total: data.fields.length }
}

/**
 * 单字段生成：只传这一行 + overwrite=true，覆盖该字段的 desc/rule/regex。
 * 为什么允许覆盖：用户是明确针对这一行点的 ✨，语义就是"重新给我写一遍"。
 */
async function generateFieldRule(f: ExtractFieldConfig) {
  if (!f?.name || aiFieldBusy.value) return
  aiFieldBusy.value = f.name
  try {
    const { filled } = await fillRulesFromAI([{ name: f.name }], true)
    if (!filled.length) {
      MessagePlugin.warning('该字段未生成可用规则（可能是模型未返回或正则未通过校验）')
      return
    }
    aiFilled.value = new Set([...aiFilled.value, f.name])
    editingName.value = f.name
    MessagePlugin.success(`已生成「${fieldLabel(f.name)}」的规则，核对后保存生效`)
  } catch (e: any) {
    console.error('ai suggest single field failed', e)
    MessagePlugin.error(e?.message || '生成该字段规则失败')
  } finally {
    aiFieldBusy.value = ''
  }
}

function clearCachedSample() {
  if (!certTypeName.value) return
  dropSampleText(certTypeName.value)
  MessagePlugin.success('已清除该分类的样票缓存')
}

/**
 * 被外部（抽屉）调用：切到指定分类。
 *
 * 批量智能生成已下线，这里只负责"选中分类 + 载入规则"；
 * 需要生成时由用户在展开区点「生成该字段规则」（逐字段，反馈可控）。
 * sampleText 仍会先落缓存，供那次单字段生成做正则唯一命中校验。
 */
async function focusCertType(p?: { certType: string; sampleText?: string; autoSuggest?: boolean }) {
  const target = String(p?.certType || '').trim()
  if (!target) return
  if (p?.sampleText) cacheSampleText(target, p.sampleText)
  // 先确保分类列表已加载，否则 certTypeOptions 里还没有该分类
  await refresh()
  const composite = typeComposite(props.scope, target)
  certType.value = composite
  await loadConfig()
}

// 重新拉取证照配置（公司证照 vehicle + 司机证照 driver，含自定义分组类型）
async function refresh() {
  try {
    const scopes = isFleetScope.value ? ['vehicle', 'driver', 'maintain'] : [props.scope]
    // 走分类缓存：切 Tab 时 refresh 会被反复触发，此前每次都真发 1~3 个请求。
    const [results] = await Promise.all([
      Promise.all(scopes.map((s) => catalog.list(s))),
      isFleetScope.value ? loadGroupAliases() : Promise.resolve(),
    ])
    scopes.forEach((s, i) => {
      const r: any = results[i]
      categories.value[s] = Array.isArray(r) ? r : r?.data || []
    })
  } catch (e) {
    console.error('load categories failed', e)
  }
  await loadConfig()
}
defineExpose({ refresh, focusCertType })

watch(
  () => props.scope,
  () => {
    certType.value = ''
    fields.value = []
    refresh()
  },
)

// 防御：父组件 kbId 异步注入（如发票页 loadKb 晚于面板挂载）时，注入后自动加载整体规则
watch(
  () => props.kbId,
  (v) => {
    if (!v || isFleetScope.value) return
    if (resolvedKbId.value === v) return
    resolvedKbId.value = v
    refresh()
  },
)

// ---- 紧凑模式：抽屉可拖宽（560~1100），列多了必然挤压 ----
// 监听自身宽度而不是 viewport：同一个面板既在 760px 设置抽屉里，也在可拖宽的车队抽屉里，
// 用 media query 会按窗口宽度误判。窄于 640px 时收起"字段描述"列，把宽度让给正则输入。
const NARROW_WIDTH = 640
const rootRef = ref<HTMLElement | null>(null)
const narrow = ref(false)
let ro: ResizeObserver | null = null

onMounted(async () => {
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
  if (typeof ResizeObserver !== 'undefined' && rootRef.value) {
    ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect?.width || 0
      narrow.value = w > 0 && w < NARROW_WIDTH
    })
    ro.observe(rootRef.value)
  }
  await ensureKb()
  await refresh()
  const first = pickFirstOption()
  if (first) {
    certType.value = first
    await loadConfig()
  }
})

// 六同步：字段定义保存后派发 fleet-categories-changed，这里跟着重载
//（否则用户切回本 Tab 前看到的还是旧字段清单）。
function onCategoriesChanged() {
  void refresh()
}

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})

// 兜底：categories 异步加载完成后，若仍未选中类型则自动选中第一个（默认加载第一个分类）
// 车队分组式选项取首个 children；非车队平铺选项直接取首项
function pickFirstOption(): string {
  const opts: any = certTypeOptions.value
  if (!opts || !opts.length) return ''
  if (!isFleetScope.value) return opts[0]?.value || ''
  return opts[0]?.children?.[0]?.value || ''
}

watch(certTypeOptions, (groups) => {
  if (certType.value || !groups.length) return
  const first = pickFirstOption()
  if (first) {
    certType.value = first
    loadConfig()
  }
})
</script>

<style lang="less" scoped>
.extract-rule-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  min-height: 0;
}
.extract-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  .cert-type-select {
    width: 220px;
  }
  .cert-type-static {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-secondary);
  }
  .spacer {
    flex: 1;
  }
  /* 高级模式：左文字（13px 次要色）+ 右开关，与无边框图标按钮垂直居中 */
  .adv-label {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    white-space: nowrap;
  }
  .adv-switch {
    margin-right: 4px;
  }
  /* 无边框图标按钮：hover 图标平滑变主题绿 + 柔和底色，不画描边 */
  .tb-icon-btn {
    color: var(--td-text-color-secondary);
    border: none;
    transition: color 0.2s ease, background-color 0.2s ease;

    &:hover:not(:disabled) {
      color: var(--td-brand-color);
      background: var(--td-brand-color-1);
    }

    /* 样票已缓存：常驻绿色高亮 */
    &.is-hot {
      color: var(--td-brand-color);
    }
  }
}
.rule-example-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 6px 10px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  .example-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--td-brand-color);
    white-space: nowrap;
  }
  .example-item {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
  .example-toggle {
    margin-left: auto;
  }
}
.rule-example-detail {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  .example-line {
    display: flex;
    gap: 10px;
    font-size: 12px;
    .ex-name {
      flex-shrink: 0;
      width: 64px;
      color: var(--td-brand-color);
      font-weight: 500;
    }
    .ex-text {
      color: var(--td-text-color-secondary);
    }
  }
}
/* 字段列表（复刻证照配置表格：紧凑行 + 点击展开） */
.rule-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);

  .rule-table-head,
  .rule-table-row {
    display: grid;
    /* 字段名 / 描述 / 类型 / 必填 / 正则兜底 / 状态
       行尾的展开箭头与 AI 图标都已下线：整行非控件区即展开热区，
       少两列把宽度让给正则输入。 */
    grid-template-columns: minmax(96px, 1fr) minmax(110px, 1.4fr) 58px 46px minmax(120px, 1.1fr) 46px;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    font-size: 12px;
  }

  /* 窄态：收起字段描述列，把宽度让给正则输入（避免抽屉内水平挤压/错位） */
  .extract-rule-panel.eq-narrow & {
    grid-template-columns: minmax(88px, 1fr) 52px 42px minmax(104px, 1.2fr) 42px;

    /* 只隐藏"字段描述"列（行内第 2 格 / 表头第 2 格），必填与正则列必须保留 */
    .rtr-desc,
    .rule-table-head > span:nth-child(2) {
      display: none;
    }
  }

  .rule-table-head {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
    font-weight: 500;
  }

  .rule-table-row {
    border-bottom: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    cursor: pointer;
    transition: background 0.15s;

    &:hover { background: var(--td-bg-color-secondarycontainer); }
    &.editing { background: var(--td-brand-color-light); }
    &:last-of-type { border-bottom: none; }

    .rtr-name {
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      &.rtr-disabled { color: var(--td-text-color-placeholder); font-weight: 400; }
    }
    .rtr-desc {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--td-text-color-secondary);
    }
    .rtr-type { font-variant-numeric: tabular-nums; }
    .rtr-required {
      display: flex;
      align-items: center;
      justify-content: center;
      :deep(.t-switch) { margin: 0 auto; }
    }
    .rtr-regex {
      display: flex;
      align-items: center;
      min-width: 0;
      .rtr-regex-input {
        width: 100%;
        min-width: 0;
        :deep(input) {
          font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
          font-size: 11px;
        }
      }
    }
    .rtr-status {
      display: flex;
      align-items: center;
      .rtr-on { font-size: 12px; color: var(--td-success-color); }
    }
  }
}

.rule-empty {
  padding: 22px 0;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}

/* 行内展开配置（复刻证照配置 meter-form--inline） */
.rule-form {
  border-top: 1px dashed var(--td-component-stroke);
  padding: 12px 14px;
  background: var(--td-bg-color-secondarycontainer);

  .rule-form-title {
    font-size: 13px;
    font-weight: 600;
    margin-bottom: 10px;
    color: var(--td-text-color-primary);
  }

  .rule-form-grid {
    display: grid;
    grid-template-columns: 1.2fr 0.8fr;
    gap: 10px 16px;

    .rule-form-item {
      label {
        display: block;
        font-size: 12px;
        color: var(--td-text-color-secondary);
        margin-bottom: 4px;
      }
      &--full { grid-column: 1 / -1; }
      :deep(.t-input__wrap),
      :deep(.t-select__wrap),
      :deep(.t-textarea) {
        width: 100%;
        min-width: 0;
      }
    }
  }

  .rule-quick {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    margin-top: 6px;
    .rule-quick-title {
      font-size: 12px;
      color: var(--td-text-color-placeholder);
      white-space: nowrap;
    }
    .rule-quick-tag { cursor: pointer; }
  }

  .rule-form-inline {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 24px;
  }

  /* 明细子项规则：一行一个子项（名称/口径/正则/启用） */
  .sub-rule-block {
    margin-top: 12px;
    padding: 10px 12px;
    border: 1px dashed var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-container);
  }
  .sub-rule-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 8px;

    .sub-rule-title {
      font-size: 12px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }
    .sub-rule-tip {
      font-size: 11px;
      color: var(--td-text-color-placeholder);
    }
  }
  .sub-rule-empty {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    padding: 4px 0;
  }
  .sub-rule-row {
    display: grid;
    grid-template-columns: 128px minmax(120px, 1fr) minmax(140px, 1.2fr) 44px;
    align-items: center;
    gap: 8px;
    padding: 3px 0;

    &.is-off { opacity: 0.55; }

    .sub-rule-name {
      display: flex;
      flex-direction: column;
      min-width: 0;
      line-height: 1.25;
    }
    .sub-rule-label {
      font-size: 12px;
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .sub-rule-key {
      font-size: 10px;
      font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
      color: var(--td-text-color-placeholder);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .sub-rule-switch {
      justify-self: center;
    }
  }

  .rule-form-hint {
    font-size: 11px;
    line-height: 1.5;
    margin-top: 4px;
    color: var(--td-text-color-placeholder);

    &.hint-danger { color: var(--td-error-color); }
    code {
      font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
      font-size: 10px;
      padding: 0 4px;
      border-radius: 3px;
      background: var(--td-bg-color-secondarycontainer);
    }
  }

  .rule-form-alert {
    margin-top: 8px;
  }

  .rule-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }
}
/* AI 直接回填后的行标记：徽章 + 淡高亮（保存或重新生成后清除） */
.rtr-ai-badge {
  margin-left: 4px;
  transform: scale(0.86);
  transform-origin: left center;
}
.rule-table-row.ai-filled {
  background: var(--td-brand-color-1);
}
.rule-form-title {
  display: flex;
  align-items: center;
  gap: 8px;
  .rule-form-ai {
    margin-left: auto;
    color: var(--td-brand-color);
  }
}
/* 工具栏图标按钮的激活态（测试区已展开） */
.extract-toolbar .is-active {
  background: var(--td-brand-color-1);
}

.advanced-block {
  margin-top: 8px;
  border-top: 1px solid var(--td-component-stroke);
  padding-top: 10px;
  .advanced-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }
  .advanced-title {
    font-size: 13px;
    font-weight: 500;
    margin-bottom: 8px;
  }
  .slot-tags {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    margin-bottom: 8px;
    .slot-tag {
      cursor: pointer;
    }
    .slot-hint {
      font-size: 12px;
      color: var(--td-text-color-placeholder);
    }
  }
  .prompt-area {
    font-family: Consolas, Monaco, 'Courier New', monospace;
    font-size: 12px;
  }
}
</style>