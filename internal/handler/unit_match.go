package handler

import "strings"

/**
 * 单位名称归一匹配（第 75 轮）。
 *
 * 背景：租户档案里是简称（持睿汽车），表计/抄表记录里是工商全称
 * （持睿汽车部件有限公司）。旧实现一律 `use_unit = ?` 精确比较，费用清单
 * 的宿舍电/水与工业水因此全部归零（真机 2026-10）。
 *
 * 口径与前端 `companyScope.sameCompany` 完全一致：
 *   ① 去掉行政区前缀（重庆/重庆市/四川…）与公司后缀（有限公司/股份公司/集团/厂）；
 *   ② 核心词互相包含即视为同一家公司。
 */
func unitCoreName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	for _, p := range []string{"重庆市", "重庆", "四川省", "四川", "广东省", "广东", "北京市", "北京", "上海市", "上海"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			break
		}
	}
	for _, suf := range []string{"股份有限公司", "有限责任公司", "有限公司", "（集团）", "(集团)", "集团", "公司", "厂"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return strings.Join(strings.Fields(s), "")
}

// unitNamesMatch 两个单位名是否指同一家公司（简称 / 全称双向等价）。
func unitNamesMatch(a, b string) bool {
	x := strings.TrimSpace(a)
	y := strings.TrimSpace(b)
	if x == "" || y == "" {
		return false
	}
	if x == y {
		return true
	}
	if strings.Contains(x, y) || strings.Contains(y, x) {
		return true
	}
	cx, cy := unitCoreName(x), unitCoreName(y)
	if cx == "" || cy == "" {
		return false
	}
	return strings.Contains(cx, cy) || strings.Contains(cy, cx)
}
