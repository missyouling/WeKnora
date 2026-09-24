<template>
  <div class="extract-rule-panel">
    <div class="extract-toolbar">
      <t-select v-model="certType" class="cert-type-select" :options="certTypeOptions" :placeholder="scope === 'maintain' ? '选择维保类型' : '选择证照类型'" @change="onCertTypeChange" />
      <div class="spacer" />
      <t-switch v-model="advancedEnabled" size="small">
        <template #label>高级模式</template>
      </t-switch>
      <t-button variant="outline" theme="primary" :disabled="!certType" @click="openTest">测试规则</t-button>
    </div>

    <div class="rule-table">
      <div class="rule-table-head">
        <span>字段名称</span>
        <span>字段描述</span>
        <span>数据类型</span>
        <span>状态</span>
        <span></span>
      </div>
      <template v-for="(f, i) in fields" :key="f.name + '-' + i">
        <div class="rule-table-row" :class="{ editing: editingName === f.name }" @click="toggleEdit(f.name)">
          <span class="rtr-name" :class="{ 'rtr-disabled': !f.enabled }">{{ f.name }}</span>
          <span class="rtr-desc" :title="f.desc || f.name">{{ f.desc || '—' }}</span>
          <span class="rtr-type">{{ typeLabel(f.type) }}</span>
          <span class="rtr-status">
            <t-switch v-model="f.enabled" size="small" @click.stop @change="() => {}" />
          </span>
          <span class="rtr-toggle"><t-icon name="chevron-down" size="15px" /></span>
        </div>
        <!-- 行内展开配置（复刻证照配置展开编辑） -->
        <div v-if="editingName === f.name" class="rule-form rule-form--inline">
          <div class="rule-form-title">配置「{{ f.name }}」</div>
          <div class="rule-form-grid">
            <div class="rule-form-item">
              <label>字段描述</label>
              <t-input v-model="f.desc" size="small" placeholder="如：公司实际经营地址" />
            </div>
            <div class="rule-form-item">
              <label>数据类型</label>
              <t-select v-model="f.type" size="small" :options="typeOptions" />
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
          <div class="rule-form-actions">
            <t-button variant="outline" size="small" @click="editingName = ''">收起</t-button>
            <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
          </div>
        </div>
      </template>
      <div v-if="!fields.length" class="rule-empty">暂无字段，请在证照配置中添加字段</div>
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

    <ExtractTestDialog v-model:visible="testVisible" :kb-id="kbId" :scope="currentScope" :cert-type="certTypeName"
      :fields="fields" :advanced-enabled="advancedEnabled" :prompt-template="promptTemplate" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { getExtractConfig, saveExtractConfig, type ExtractFieldConfig } from '@/api/fleet'
import { listFleetCategories, listFleetGroupAliases } from '@/api/fleet'
import { sortCertsByLocalOrder } from './useCertOrder'
import { listKnowledgeBases } from '@/api/knowledge-base'
import ExtractTestDialog from './ExtractTestDialog.vue'

const props = defineProps<{ scope: string }>()

const KB_NAME = '车队管理'
const kbId = ref('')

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
}

// 默认提取规则底稿：每个字段的 desc + rule 预设，加载时自动填充（已保存配置优先）。
// 便于用户直接使用或在此基础上微调。
type RuleDraft = { desc: string; rule: string }
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      状态: { desc: '正常、遗失、补办中、注销', rule: '从状态描述中识别；原文无则留空' },
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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
      备注: { desc: '补充说明', rule: '原文中其它重要信息摘要，不超过 50 字；无则留空' },
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

// 分组别名：与设置抽屉 FleetCertPanel 同 key（`${scope}:${builtinGroupKey}`），builtin group_key = company/driver/maintain
const BUILTIN_GROUP_KEY: Record<string, string> = { vehicle: 'company', driver: 'driver', maintain: 'maintain' }
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
  return TYPE_GROUP_META
    .filter((g) => g.scope === props.scope)
    .map(({ scope, fallback }) => {
    const title = groupTitle(scope, fallback)
    const names: string[] = []
    const seen = new Set<string>()
    // 顺序：按 builtinOrder（内置定义顺序）遍历 categories 匹配项，
    // 内置未入库的按同位置插入，自定义分类追加末尾；localStorage 拖拽顺序优先。
    const builtinKeys = Object.keys(DEFAULT_FIELDS[scope] || {})
    const sortedCats = sortCertsByLocalOrder(scope, (categories.value[scope] || []) as any[], builtinKeys) as any[]
    const pushedNames = new Set<string>()
    builtinKeys.forEach((bKey) => {
      const cat = sortedCats.find((c: any) => c.builtin_key === bKey)
      if (cat) {
        if (cat.enabled === false) return
        if (!seen.has(cat.name)) { seen.add(cat.name); names.push(cat.name); pushedNames.add(cat.name) }
      } else {
        // 内置未入库：直接用内置名
        if (!seen.has(bKey)) { seen.add(bKey); names.push(bKey) }
      }
    })
    sortedCats.forEach((c: any) => {
      if (c && c.name && c.enabled !== false && !seen.has(c.name)) {
        seen.add(c.name); names.push(c.name)
      }
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

function typeLabel(t: string) {
  return typeOptions.find((o) => o.value === t)?.label || '字符串'
}

function applyExample(f: ExtractFieldConfig, text: string) {
  f.rule = f.rule ? `${f.rule}；${text}` : text
}

function makeField(name: string): ExtractFieldConfig {
  return { name, desc: '', type: 'string', rule: '', enabled: true }
}

// 字段名权威 = 证照配置（fleet_categories.subs，启用字段优先），未入库类型回退内置底稿。
// 与证照配置面板共用同一来源规则，保证两处字段永远一致。
function authorityFields(): { name: string; enabled: boolean; dataType: string }[] {
  const scope = currentScope.value
  const c = (categories.value[scope] || []).find((x: any) => x.name === certTypeName.value)
  if (c && Array.isArray(c.subs) && c.subs.length) {
    const seen = new Set<string>()
    return c.subs
      .map((s: any) => ({ name: String(s.name || '').trim(), enabled: s.enabled !== false, dataType: s.data_type || 'text' }))
      .filter((s: any) => s.name && !seen.has(s.name) && seen.add(s.name))
  }
  const defaults = (DEFAULT_FIELDS[scope] || {})[certTypeName.value] || []
  const seen = new Set<string>()
  return defaults
    .map((n) => ({ name: n, enabled: true, dataType: 'text' }))
    .filter((s: any) => s.name && !seen.has(s.name) && seen.add(s.name))
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
  if (kbId.value) return kbId.value
  try {
    const res: any = await listKnowledgeBases({ page: 1, page_size: 100 })
    const list = Array.isArray(res) ? res : res?.data || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    if (found) kbId.value = found.id
  } catch (e) {
    console.error('load kb failed', e)
  }
  return kbId.value
}

async function loadConfig() {
  if (!certType.value) {
    fields.value = []
    advancedEnabled.value = false
    promptTemplate.value = ''
    return
  }
  await ensureKb()
  if (!kbId.value) return
  loadingCfg.value = true
  try {
    const res: any = await getExtractConfig(kbId.value, currentScope.value, certTypeName.value)
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
      const d = draft[a.name] || {}
      return {
        name: a.name,
        desc: c?.desc || d.desc || '',
        type: c?.type || ({ text: 'string', number: 'number', date: 'date', array: 'array' } as Record<string,string>)[a.dataType] || 'string',
        rule: c?.rule || d.rule || '',
        enabled: c && c.enabled !== undefined ? !!c.enabled : a.enabled,
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
  await loadConfig()
}

async function save() {
  if (!certType.value) return
  await ensureKb()
  if (!kbId.value) {
    MessagePlugin.error('未找到车队管理知识库')
    return
  }
  const payload = {
    scope: currentScope.value,
    cert_type: certTypeName.value,
    fields: fields.value.map((f) => ({ ...f })),
    advanced_enabled: advancedEnabled.value,
    prompt_template: promptTemplate.value,
  }
  saving.value = true
  try {
    await saveExtractConfig(kbId.value, payload)
    MessagePlugin.success('提取规则已保存')
  } catch (e: any) {
    console.error('save extract config failed', e)
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function openTest() {
  if (!certType.value) return
  testVisible.value = true
}

// 重新拉取证照配置（公司证照 vehicle + 司机证照 driver，含自定义分组类型）
async function refresh() {
  try {
    const scopes = ['vehicle', 'driver', 'maintain']
    const [results] = await Promise.all([
      Promise.all(scopes.map((s) => listFleetCategories({ scope: s }))),
      loadGroupAliases(),
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
defineExpose({ refresh })

watch(
  () => props.scope,
  () => {
    certType.value = ''
    fields.value = []
  },
)

onMounted(async () => {
  await ensureKb()
  await refresh()
  const groups = certTypeOptions.value
  const first = groups[0]?.children?.[0]
  if (first) {
    certType.value = first.value
    await loadConfig()
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
  .spacer {
    flex: 1;
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
    grid-template-columns: minmax(110px, 1.2fr) minmax(120px, 2fr) 0.7fr 0.6fr 22px;
    align-items: center;
    gap: 10px;
    padding: 7px 12px;
    font-size: 12px;
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
    .rtr-status {
      display: flex;
      align-items: center;
      .rtr-on { font-size: 12px; color: var(--td-success-color); }
    }
    .rtr-toggle {
      display: flex;
      align-items: center;
      justify-content: center;
      color: var(--td-text-color-placeholder);
      transition: transform 0.15s;
    }
    &.editing .rtr-toggle { transform: rotate(180deg); }
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

  .rule-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }
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
