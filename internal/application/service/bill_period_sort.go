package service

import (
	"strings"
	"time"
)

// 账单周期排序（第 43 轮）。
//
// 真机事故：市电账单列表出现 2026-10、2026-09、2025-06、2025-07… 第 10 行又是
// 2026-03、2026-05、2026-04。根因是列表按 **CreatedAt（上传时间）** 排序，
// 与账单账期完全无关：补传一份 2025-06 的旧账单，它就会插到最前面。
//
// 正确口径：**账单周期 DESC** 为主键，同月再按 CreatedAt DESC 稳定排序。
// 账期必须先归一化成 YYYY-MM，否则 `"2026.10"` / `"2026年7月"` / `"2026-7"`
// 会按字符串字面序排（"2026.10" < "2026-9"，"2026-7" > "2026-10"），照样错乱。

// billPeriodKey 把任意写法的账期归一化为 YYYY-MM；无法识别返回空串。
//
// 空串在排序里**永远排最后**：解析中的文件没有账期，不应占据列表顶部。
func billPeriodKey(raw string) string {
	if v := NormalizeBillingPeriod(raw); v != "" {
		return v
	}
	// NormalizeBillingPeriod 覆盖六位紧凑、YYYY-MM/./、任意文本内取年月；
	// 这里只补它不处理的「已经带日期但月份缺前导零」的边界（如 2026-7-1）。
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if len(s) >= 6 {
		y, m := s[0:4], s[5:6]
		if isDigitStr(y) && m >= "1" && m <= "9" && (len(s) == 6 || s[6] < '0' || s[6] > '9') {
			return y + "-0" + m
		}
	}
	return ""
}

// isDigitStr 判断整串是否全为 ASCII 数字。
func isDigitStr(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// billPeriodLess 账单周期排序比较器：账期 DESC → 创建时间 DESC。
//
// 返回 true 表示 a 应排在 b 前面。账期无法识别的记录排在最后（同为无法识别时
// 按创建时间倒序），保证「解析中/待补录」行不会插到正常账单中间。
func billPeriodLess(aPeriod string, aAt time.Time, bPeriod string, bAt time.Time) bool {
	ka, kb := billPeriodKey(aPeriod), billPeriodKey(bPeriod)
	switch {
	case ka == "" && kb == "":
		return aAt.After(bAt)
	case ka == "":
		return false
	case kb == "":
		return true
	case ka != kb:
		// YYYY-MM 定长字典序等价于时间序
		return ka > kb
	default:
		return aAt.After(bAt)
	}
}
