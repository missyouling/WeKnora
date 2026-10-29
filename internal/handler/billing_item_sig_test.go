package handler

import "testing"

// 同一费用项的多种写法必须折叠成同一签名，否则每次生成账单都会补一条重复子项，
// 最终表现为表头多出「字段定义里没有」的同名列。
func TestBillingItemSignatureFoldsSynonyms(t *testing.T) {
	pf := [][2]string{
		{"功率因素调整电费", "功率因素调整电费"},
		{"功率因数调整电费", "功率因数调整电费"},
		{"功率因数调整电费", "功率因数调整"},
	}
	pfSig := billingItemSignature(pf[0][0], pf[0][1])
	for _, p := range pf[1:] {
		if got := billingItemSignature(p[0], p[1]); got != pfSig {
			t.Errorf("力调电费签名不一致：%v => %q，期望 %q", p, got, pfSig)
		}
	}
	res := [][2]string{
		{"(6)政府性基金及附加-工商业", "小型水库移民后期扶持资金（地方）"},
		{"(6)政府性基金及附加-工商业", "小型水库移民后期扶持资金(地方)"},
	}
	if billingItemSignature(res[0][0], res[0][1]) != billingItemSignature(res[1][0], res[1][1]) {
		t.Error("全角/半角括号的小型水库移民基金未折叠")
	}
	// 工商业 / 居民 是同名但**不同**的分组，签名必须区分
	if billingItemSignature("(6)政府性基金及附加-工商业", "农网还贷") ==
		billingItemSignature("(6)政府性基金及附加-居民", "农网还贷") {
		t.Error("工商业与居民的农网还贷被折叠成同一签名")
	}
	// 「其它 / 其他」也是同一项
	if billingItemSignature("(4)系统运行费", "其它系统运行费用") !=
		billingItemSignature("(4)系统运行费", "其他系统运行费用") {
		t.Error("其它/其他 未归一")
	}
}

// 精确键优先：居民行的开关不能被裸名（工商业那条）翻成 true。
func TestBillingItemEnabledPrefersExactKey(t *testing.T) {
	enabled := map[string]bool{
		"(6)政府性基金及附加-工商业|农网还贷": true,
		"(6)政府性基金及附加-居民|农网还贷":  false,
		"农网还贷":                  true, // 裸名兼容键（先到先得，落在工商业这条上）
	}
	if !billingItemEnabled(enabled, "(6)政府性基金及附加-工商业", "农网还贷") {
		t.Error("工商业农网还贷应为启用")
	}
	if billingItemEnabled(enabled, "(6)政府性基金及附加-居民", "农网还贷") {
		t.Error("居民农网还贷应为关闭：精确键优先，不得被裸名覆盖")
	}
	// 没有分组键的老数据 → 回落到裸名
	if !billingItemEnabled(map[string]bool{"农网还贷": true}, "(6)政府性基金及附加-工商业", "农网还贷") {
		t.Error("老数据（只有裸名）应继续生效")
	}
	// 什么都没有 → 关闭
	if billingItemEnabled(map[string]bool{}, "x", "y") {
		t.Error("未知子项必须默认关闭")
	}
}
