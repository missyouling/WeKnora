// 花名册排序与合同状态（第 47 轮）。
package handler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// hrNaturalLess 自然序比较：字母逐字符比，数字段按数值比。
// C020300008 < C02050011 < C02050012（纯字典序会把 C02050011 排在 C020300008 前）。
func hrNaturalLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		da := ca >= '0' && ca <= '9'
		db := cb >= '0' && cb <= '9'
		if da && db {
			// 取连续数字段整体比较
			sa, sb := ia, ib
			for ia < len(a) && a[ia] >= '0' && a[ia] <= '9' {
				ia++
			}
			for ib < len(b) && b[ib] >= '0' && b[ib] <= '9' {
				ib++
			}
			na, _ := strconv.ParseUint(strings.TrimLeft(a[sa:ia], "0"), 10, 64)
			nb, _ := strconv.ParseUint(strings.TrimLeft(b[sb:ib], "0"), 10, 64)
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ia++
		ib++
	}
	return len(a)-ia < len(b)-ib
}

// hrSortEmployeesByEmployeeID 工号自然升序，空工号统一排末尾。
func hrSortEmployeesByEmployeeID(items []HREmployee) {
	sort.SliceStable(items, func(i, j int) bool {
		a := strings.TrimSpace(items[i].EmployeeID)
		b := strings.TrimSpace(items[j].EmployeeID)
		if a == "" && b == "" {
			return items[i].Name < items[j].Name
		}
		if a == "" {
			return false
		}
		if b == "" {
			return true
		}
		return hrNaturalLess(a, b)
	})
}

// 合同状态五态（第 54 轮：新增「未签订」「已解除」）。
const (
	hrContractStatusNormal     = "正常"
	hrContractStatusUnsigned   = "未签订"
	hrContractStatusExpiring   = "即将到期"
	hrContractStatusExpired    = "到期"
	hrContractStatusTerminated = "已解除"
)

// hrContractStatusOf 按「合同结束日期 + 合同类型」判定状态（不含在职/离职判定）。
//
//	· 无固定期限劳动合同 → 正常（没有到期日）
//	· 未填结束日期        → 未签订（在职但合同还没录）
//	· 剩余天数 > 30       → 正常
//	· 0 <= 剩余 <= 30     → 即将到期
//	· 剩余 < 0            → 到期
//
// 「已解除」依赖员工的在职状态，由 hrEmployeeContractStatus 前置判定。
func hrContractStatusOf(endDate, contractType string, now time.Time) string {
	if strings.Contains(strings.TrimSpace(contractType), "无固定期限") {
		return hrContractStatusNormal
	}
	end := strings.TrimSpace(endDate)
	if end == "" {
		return hrContractStatusUnsigned
	}
	t, err := time.Parse("2006-01-02", normalizeHRDateValue(end))
	if err != nil {
		return hrContractStatusUnsigned
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	days := int(t.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return hrContractStatusExpired
	case days <= 30:
		return hrContractStatusExpiring
	default:
		return hrContractStatusNormal
	}
}

// hrEmployeeContractStatus 从员工记录（主列 + custom_data）取合同字段并判定。
// 离职员工一律「已解除」—— 合同不会因为日期没到就还算生效。
func hrEmployeeContractStatus(e HREmployee) string {
	if hrEmployeeIsResigned(e) {
		return hrContractStatusTerminated
	}
	if e.CustomData == nil {
		return hrContractStatusUnsigned
	}
	// 第 52 轮：新字段名优先，兼容存量「现合同…」键。
	endDate := strings.TrimSpace(fmt.Sprint(customValueOrEmpty(e.CustomData, "合同结束日期", "现合同结束日期")))
	contractType := strings.TrimSpace(fmt.Sprint(customValueOrEmpty(e.CustomData, "合同类型", "现合同类型")))
	return hrContractStatusOf(endDate, contractType, time.Now())
}

// hrEmployeeIsResigned 员工是否已离职（兼容中文 / 英文状态值）。
func hrEmployeeIsResigned(e HREmployee) bool {
	return normalizeEmployeeStatus(e.Status) == "resigned"
}

func toStringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(
		strings.TrimSpace(toJSONScalar(v)), "\n", ""), "\r", ""))
}

// toJSONScalar 把任意标量转成文本（避免 fmt 引入多余格式）。
func toJSONScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
