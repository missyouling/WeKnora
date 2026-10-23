/**
 * 账单周期归一化：把账单原文里五花八门的周期写法收敛成 `YYYY-MM`。
 *
 * 与后端 `service.NormalizeBillingPeriod` 同口径（两份实现必须同步，
 * 否则列表显示与落库值会不一致，导出/筛选就打架）。
 *
 * 为什么统一到月粒度：账单的业务语义是「某月账单」，起止日只用于说明这一期
 * 覆盖哪段；把 "2026年8月1日~2026年8月31日" 直接塞进列表列，
 * 列宽必被撑爆并截断成 "2026-09-0..."。
 */
export function normalizeBillingPeriod(raw: unknown): string {
  const s = String(raw ?? '').trim()
  if (!s) return ''

  // 1) 紧凑六位 202608（前后不能再接数字）
  const compact = /(?:^|\D)((?:19|20)\d{2})(0[1-9]|1[0-2])(?:\D|$)/.exec(s)
  if (compact) return joinYearMonth(compact[1], compact[2])

  // 2) 整串就是年月：2026-08 / 2026.8 / 2026/08
  const bare = /^\s*((?:19|20)\d{2})\s*[-./年]\s*(\d{1,2})\s*月?\s*$/.exec(s)
  if (bare) return joinYearMonth(bare[1], bare[2])

  // 3) 通用：年 + 分隔/汉字「年」+ 月（"2026-08-01" / "2026年8月1日"）
  const loose = /((?:19|20)\d{2})\s*(?:年|[-./])\s*(\d{1,2})/.exec(s)
  if (loose) return joinYearMonth(loose[1], loose[2])

  return ''
}

function joinYearMonth(year: string, month: string): string {
  const y = Number(year)
  const m = Number(month)
  if (!Number.isFinite(y) || !Number.isFinite(m)) return ''
  if (y < 1900 || y > 2200 || m < 1 || m > 12) return ''
  return `${y}-${String(m).padStart(2, '0')}`
}
