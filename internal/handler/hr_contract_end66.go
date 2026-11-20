// 第 66 轮：无固定期限劳动合同的结束日期兜底。
//
// 前端已做联动（选「无固定期限劳动合同」/「合同期限=无固定期限」→ 清空并禁用
// 合同结束日期），但接口必须同口径兜底：脚本或旧客户端仍可能提交一个历史日期，
// 落库后合同状态派生（contractStatusOf）会把无固定期限判成「已到期」。
package handler

import "strings"

// hrEmployeeOpenEndedContract 是否无固定期限合同。
//
// 两侧字段都要认：合同模块写「合同类型 / 合同期限」，
// 花名册模块写「现合同类型 / 现合同期限」（见 hrCore 的别名表）。
func hrEmployeeOpenEndedContract(e *HREmployee) bool {
	if e == nil {
		return false
	}
	d := e.CustomData
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(hrDormStr(d, k)); v != "" {
				return v
			}
		}
		return ""
	}
	if strings.Contains(get("合同类型", "现合同类型"), "无固定期限") {
		return true
	}
	return get("合同期限", "现合同期限") == "无固定期限"
}

// hrNormalizeContractEndDate66 无固定期限时把结束日期清空（就地修改 custom_data）。
func hrNormalizeContractEndDate66(e *HREmployee) {
	if !hrEmployeeOpenEndedContract(e) || e.CustomData == nil {
		return
	}
	for _, k := range []string{"合同结束日期", "现合同结束日期"} {
		if _, ok := e.CustomData[k]; ok {
			e.CustomData[k] = ""
		}
	}
}
