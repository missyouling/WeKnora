// 全模块「状态字段」的**唯一**视觉映射（第 70 轮）。
//
// 为什么必须收敛到一个文件：
//   · 列表用 TDesign Tag（浅底 + 同色字），抽屉用纯带色文字（无边框，第 64 轮口径），
//     两处各写一套 `if/else` 时语义必然漂移 —— 真机上表现为
//     「列表里 `到期` 是红的、抽屉里是黑的」「回盘表五险状态在抽屉变成输入框」；
//   · 状态判定同时依赖**字段名**与**值**：`正常` 在合同里是绿、在公积金封存语义里
//     不能一律绿；只按值判断无法区分。
//
// 口径（与需求逐条对齐）：
//   合同状态     正常=ok   即将到期=warn   到期/已终止/解除=danger
//   五险参保状态 参保缴费=ok  暂停参保/暂停缴费=warn  终止参保/终止缴费/未参保=off
//   记录/账单状态 已申报/已生成/已缴=ok  待生成/待申报=warn  作废=off
//   公积金状态   正常=ok   封存/启封中=off
//   宿舍状态     空闲=ok   部分入住=warn   满员=danger   维修中=off
//   住宿状态     在住/待入住=ok  已退宿=off
//   资产状态     正常=ok   损坏=warn   报废=danger
//   房源类型     公司宿舍=ok  公租房=warn
/** 状态语义色（与 CSS 类 `.is-*` 一一对应）。 */
export type StatusTone = 'ok' | 'warn' | 'danger' | 'off' | 'muted'

const TONE_CLASS: Record<StatusTone, string> = {
  ok: 'is-ok',
  warn: 'is-warn',
  danger: 'is-danger',
  off: 'is-off',
  muted: 'is-muted',
}

const TAG_THEME: Record<StatusTone, 'success' | 'warning' | 'danger' | 'default'> = {
  ok: 'success',
  warn: 'warning',
  danger: 'danger',
  off: 'default',
  muted: 'default',
}

/**
 * 只读状态字段白名单：命中才在抽屉里渲染成**纯带色文字**。
 *
 * 注意「资产状态」「房源类型」是**可编辑 enum**（下拉），必须排除 ——
 * 第 65 轮真机踩过：把它们当只读状态后，抽屉里改不了资产状态。
 * 列表侧仍会给它们上色（那里本来就是只读展示，用的是 statusTagThemeOf）。
 */
export const STATUS_TEXT_FIELDS = new Set([
  // 劳动合同 / 劳务合同
  '合同状态',
  // 社保
  '养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态',
  '参保状态', '养老保险参保状态', '医疗保险参保状态',
  '记录类型', '账单状态', '账单月份状态',
  // 公积金
  '账户状态', '缴存状态', '缴存状态',
  // 宿舍（资产状态/房源类型是可编辑 enum，不在这里）
  '宿舍状态', '住宿状态',
  // 通用
  '在职状态', '证件状态', '证照状态', '提取状态', '账期状态', '记录性质',
])

/** 字段名精确映射：先按字段名分组，再按值判定。 */
const BY_FIELD: Record<string, Array<[RegExp, StatusTone]>> = {
  合同状态: [
    [/^(正常|生效中|履约中)$/, 'ok'],
    [/即将到期|临期/, 'warn'],
    [/^(到期|已到期|已终止|终止)$/, 'danger'],
    // 已解除是「历史归档」语义（第 54 轮契约：灰），不是异常
    [/已解除|解除/, 'off'],
    // 未签订 / 待签 / 续签中：都属于「还没落定」，橙色提示而不是灰色
    [/未签订|未签|待签|续签|待续/, 'warn'],
  ],
  资产状态: [
    [/^正常$/, 'ok'],
    [/损坏|维修/, 'warn'],
    [/报废|变卖|丢失/, 'danger'],
  ],
  宿舍状态: [
    [/^空闲$/, 'ok'],
    [/部分入住|待入住/, 'warn'],
    [/^满员$/, 'danger'],
    [/维修中|停用/, 'off'],
  ],
  住宿状态: [
    [/^(在住|待入住)$/, 'ok'],
    [/已退宿|退宿/, 'off'],
  ],
  房源类型: [
    [/公司宿舍|员工宿舍/, 'ok'],
    [/公租房|租赁房/, 'warn'],
  ],
  账户状态: [
    [/^正常$/, 'ok'],
    [/封存|启封|暂停/, 'off'],
  ],
  缴存状态: [
    [/^正常$/, 'ok'],
    [/封存|停缴|暂停/, 'off'],
  ],
  账单状态: [
    [/已申报|已生成|已汇缴|已缴|已结算|已出账/, 'ok'],
    [/待生成|待申报|待缴|草稿|部分/, 'warn'],
    [/作废|已删除|红冲/, 'off'],
  ],
  记录类型: [
    [/^正常$/, 'ok'],
    [/补退|补缴|异常/, 'warn'],
  ],
}

/** 五险参保状态共用一套规则（字段名不同、值域相同）。 */
const INSURANCE: Array<[RegExp, StatusTone]> = [
  [/参保缴费|正常参保|^正常$/, 'ok'],
  [/暂停参保|暂停缴费|中断|欠费/, 'warn'],
  [/终止参保|终止缴费|未参保|已转出|转出/, 'off'],
]

/** 通用兜底：字段名不在白名单或值未命中时，按关键词保守判定。 */
const FALLBACK: Array<[RegExp, StatusTone]> = [
  [/^(正常|在住|在岗|在职|空闲|已生成|已申报|已缴|完成|已修复|公司宿舍)$/, 'ok'],
  [/部分入住|待入住|待生成|待申报|待维修|即将到期|损坏|补退|异常|公租房/, 'warn'],
  [/^(满员|到期|已到期|已终止|报废|作废|解除)$/, 'danger'],
  [/已退宿|退宿|封存|停用|终止|未参保|维修中|已删除/, 'off'],
]

/** 五险状态字段（含裸「参保状态」与「养老参保状态」等），共用同一套值域规则。 */
function isInsuranceField(field: string): boolean {
  return /参保状态$/.test(field)
}

/**
 * 解析状态语义色。
 *
 * @param field 字段名（`合同状态` / `宿舍状态` / `养老参保状态` …）
 * @param value 单元格或表单里的原始值（允许带空格）
 */
export function resolveStatusTone(field: string, value: unknown): StatusTone {
  const s = String(value ?? '').trim()
  if (!s) return 'muted'
  const name = String(field || '').trim()

  const rules = BY_FIELD[name]
  if (rules) {
    for (const [re, tone] of rules) if (re.test(s)) return tone
  }
  if (isInsuranceField(name)) {
    for (const [re, tone] of INSURANCE) if (re.test(s)) return tone
  }
  for (const [re, tone] of FALLBACK) if (re.test(s)) return tone
  return 'muted'
}

/** 抽屉 / 列表纯文字用的 class（`is-ok` / `is-warn` / `is-danger` / `is-off` / `is-muted`）。 */
export function statusTextClassOf(field: string, value: unknown): string {
  return TONE_CLASS[resolveStatusTone(field, value)]
}

/** 列表 TDesign Tag 的 theme。 */
export function statusTagThemeOf(field: string, value: unknown): 'success' | 'warning' | 'danger' | 'default' {
  return TAG_THEME[resolveStatusTone(field, value)]
}
