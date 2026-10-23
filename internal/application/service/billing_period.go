package service

import (
	"regexp"
	"strconv"
	"strings"
)

// 账单周期归一化：把账单原文里五花八门的周期写法收敛成 YYYY-MM。
//
// 为什么统一到月粒度：
//
//	账单的业务语义是"某月账单"，起止日只用于告诉人这一期覆盖哪段。
//	直接把 "2026年8月1日~2026年8月31日" 塞进列表列，列宽必被撑爆并截断成
//	"2026-09-0..."；而日精度对账期聚合（按月统计、跟上期比）毫无用处。
//
// 同时这也是"两个账单周期列"的治本手段之一：无论上游给的是范围还是单日，
// 主列表的「账单周期」永远只有一种形态。
//
// 解析顺序（先窄后宽，避免误吞）：
//  1. 紧凑六位 202608；
//  2. 已是 YYYY-MM / YYYY.MM / YYYY/MM；
//  3. 通用「四位年 + 分隔 + 1~2 位月」，从字符串里找**第一个**命中（范围就取起始月）；
//  4. 都不匹配返回空串（调用方保留原值或留空，不做臆造）。
func NormalizeBillingPeriod(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// 1) 紧凑六位：202608
	if m := reCompactYearMonth.FindStringSubmatch(s); m != nil {
		return joinYearMonth(m[1], m[2])
	}

	// 2) 已是 YYYY-MM / YYYY.MM / YYYY/MM（整串）
	if m := reBareYearMonth.FindStringSubmatch(s); m != nil {
		return joinYearMonth(m[1], m[2])
	}

	// 3) 通用：从任意文本里取第一个「年 + 月」
	if m := reLooseYearMonth.FindStringSubmatch(s); m != nil {
		return joinYearMonth(m[1], m[2])
	}
	return ""
}

var (
	// 紧凑六位：前后不能再接数字，避免把 20260801 误读成 2026-08（那也确实是 8 月，可接受）
	reCompactYearMonth = regexp.MustCompile(`(?:^|\D)(20\d{2}|19\d{2})(0[1-9]|1[0-2])(?:\D|$)`)
	// 整串就是年月：2026-08 / 2026.8 / 2026/08
	reBareYearMonth = regexp.MustCompile(`^\s*((?:19|20)\d{2})\s*[-./年]\s*(\d{1,2})\s*月?\s*$`)
	// 宽松：年 + 可选分隔 + 月（"2026年8月1日" / "2026-08-01" / "2026.08.01"）
	// 年必须紧跟分隔或汉字「年」，否则 "2026" 后面直接跟数字的组合交给紧凑六位规则处理。
	reLooseYearMonth = regexp.MustCompile(`((?:19|20)\d{2})\s*(?:年|[-./])\s*(\d{1,2})`)
)

// joinYearMonth 组装并校验 YYYY-MM；月份越界返回空串（不猜、不纠正）。
func joinYearMonth(year, month string) string {
	y, err1 := strconv.Atoi(strings.TrimSpace(year))
	m, err2 := strconv.Atoi(strings.TrimSpace(month))
	if err1 != nil || err2 != nil {
		return ""
	}
	if y < 1900 || y > 2200 || m < 1 || m > 12 {
		return ""
	}
	return strconv.Itoa(y) + "-" + pad2(m)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// billingPeriodOfRecord 由起止日期推导账单月份，raw 优先。
//
// raw（原文周期字段）常常更可信：模型可能只给出 start 而 end 为空，
// 或者 start 写的是抄表日而非账期首日。所以先用 raw，raw 解析不出来再退回 start。
func billingPeriodOfRecord(raw, start, end string) string {
	if v := NormalizeBillingPeriod(raw); v != "" {
		return v
	}
	if v := NormalizeBillingPeriod(start); v != "" {
		return v
	}
	return NormalizeBillingPeriod(end)
}
