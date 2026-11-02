// 明细行合计（第 24 轮任务三）。
//
// 列表悬停浮层（BusinessSubItemsPopover）与详情抽屉（BusinessSplitDetailDrawer）
// 的「项目明细」表都要在底部渲染一行 `合计`，且**两处数字必须完全一致** ——
// 各写一份是最典型的「浮窗 51989.49、抽屉 51989.48」分叉来源。
//
// 所以判据与求和都放在这里，两个组件只负责渲染。
//
// 口径要点：
//   · 只有「金额」与「税额」两列参与累加。数量（qty）与单价（price）也是数值，
//     但合计没有业务意义（单价合计更是荒谬），绝不能因为 dataType=number 就累加；
//   · 判据用「列键 + 标签」双轨：分类字段配置（subs）里金额列的 dataType
//     历史上有标成 number 的，只看 dataType 会漏；
//   · 求和按「分」整数累加再还原：明细里有负数折扣行（-513.35），
//     裸浮点相加会留下 51989.480000000003 这种尾巴，直接上屏就是脏值。

/** 金额列键（含历史别名）。 */
const AMOUNT_KEY_RE = /^(amount|amt|金额|价税合计)$/i
/** 税额列键。 */
const TAX_KEY_RE = /^(tax_amount|taxamt|tax|税额)$/i
/** 金额列标签。 */
const AMOUNT_LABEL_RE = /金额|价税合计/
/** 税额列标签。 */
const TAX_LABEL_RE = /税额/

/** 该列是否参与合计（金额 / 税额）。 */
export function isSumColumn(sf: any): boolean {
  const key = String(sf?.key || '')
  const label = String(sf?.label || '')
  if (AMOUNT_KEY_RE.test(key) || AMOUNT_LABEL_RE.test(label)) return true
  if (TAX_KEY_RE.test(key) || TAX_LABEL_RE.test(label)) return true
  return false
}

/** 取明细行某列的数值（标准键优先，回退 extra；非数值给 0）。 */
export function numOfItem(it: any, sf: any): number {
  const key = String(sf?.key || '')
  const raw = it?.[key] !== undefined ? it?.[key] : it?.extra?.[key]
  const n = Number(raw)
  return Number.isFinite(n) ? n : 0
}

/**
 * 某列的合计文本。
 *
 * 非合计列返回空串（不是 '0'）：规格型号/单位/单价/税率写 0 会被误读成
 * 「合计值为 0」，空串才是「这一列不适用合计」。
 */
export function sumTextOfColumn(items: any[], sf: any): string {
  if (!isSumColumn(sf)) return ''
  const cents = (items || []).reduce((s, it) => s + Math.round(numOfItem(it, sf) * 100), 0)
  const yuan = cents / 100
  return yuan.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/** 至少有一列可累加（否则不渲染合计行，避免只有名称/规格的表多出一条空行）。 */
export function hasSumColumn(cols: any[]): boolean {
  return (cols || []).some((sf) => isSumColumn(sf))
}

/**
 * 构造一整行的合计单元格文本（首列固定「合计」，其余按列判定）。
 *
 * 两处渲染共用本函数 → 单元格个数与内容天然对齐。
 */
export function totalRowTexts(items: any[], cols: any[]): string[] {
  return (cols || []).map((sf, i) => (i === 0 ? '合计' : sumTextOfColumn(items, sf)))
}
