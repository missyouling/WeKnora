// 工号自动编码（第 48 轮）。
//
// 结构：[1位职级] + [5位部门码] + [4位递增序号]，共 10 位，如 C020300005。
//
//	A = 总经理 / 副总经理 / 总裁 / 董事长
//	B = 主管 / 经理 / 主任 / 部长 / 科长 / 组长
//	C = 其余（默认）
//
// **存量工号绝对不改**：本模块只产出「建议值」给新增员工用。
package handler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// 职级前缀
const (
	hrRankA = "A"
	hrRankB = "B"
	hrRankC = "C"
)

var (
	hrRankAKeywords = []string{"总经理", "副总经理", "总裁", "董事长"}
	hrRankBKeywords = []string{"主管", "经理", "主任", "部长", "科长", "组长"}
	// 工号里可提取的部门码：优先 5 位（第 2~6 位），兼容 8 位旧工号末段
	hrEmployeeIDDeptCode = regexp.MustCompile(`^[A-Za-z](\d{5})\d{4}$`)
	hrEmployeeIDTailSeq  = regexp.MustCompile(`(\d{4})$`)
)

// hrRankOfTitle 按职务判定职级前缀。
func hrRankOfTitle(title string) string {
	t := strings.TrimSpace(title)
	for _, kw := range hrRankAKeywords {
		if strings.Contains(t, kw) {
			return hrRankA
		}
	}
	for _, kw := range hrRankBKeywords {
		if strings.Contains(t, kw) {
			return hrRankB
		}
	}
	return hrRankC
}

// hrDeptCodeFromEmployeeID 从存量工号提取 5 位部门码；不匹配返回空。
func hrDeptCodeFromEmployeeID(employeeID string) string {
	if m := hrEmployeeIDDeptCode.FindStringSubmatch(strings.TrimSpace(employeeID)); len(m) == 2 {
		return m[1]
	}
	return ""
}

// hrSeqFromEmployeeID 取工号末尾 4 位序号；不足 4 位或非数字返回 0。
func hrSeqFromEmployeeID(employeeID string) int {
	s := strings.TrimSpace(employeeID)
	if s == "" {
		return 0
	}
	if m := hrEmployeeIDTailSeq.FindStringSubmatch(s); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// hrComposeEmployeeID 拼装 10 位工号。
func hrComposeEmployeeID(rank, deptCode string, seq int) string {
	if len(deptCode) != 5 {
		deptCode = fmt.Sprintf("%05d", 0)
	}
	if seq < 1 {
		seq = 1
	}
	return fmt.Sprintf("%s%s%04d", rank, deptCode, seq)
}

// hrNextEmployeeID 计算下一个工号。
//
// existing 传入全量存量工号即可（函数内按部门码过滤）；deptCode 为空时从
// 存量工号反推该部门最常见的前缀码（用于组织机构尚未维护编码的场景）。
func hrNextEmployeeID(deptCode, title string, existing []string) string {
	rank := hrRankOfTitle(title)
	code := strings.TrimSpace(deptCode)
	if len(code) != 5 {
		code = hrGuessDeptCode(existing)
	}
	maxSeq := 0
	for _, id := range existing {
		if hrDeptCodeFromEmployeeID(id) != code {
			continue
		}
		if seq := hrSeqFromEmployeeID(id); seq > maxSeq {
			maxSeq = seq
		}
	}
	return hrComposeEmployeeID(rank, code, maxSeq+1)
}

// hrGuessDeptCode 无法确定部门码时的兜底：取存量里出现最多的 5 位码。
func hrGuessDeptCode(existing []string) string {
	counts := map[string]int{}
	for _, id := range existing {
		if code := hrDeptCodeFromEmployeeID(id); code != "" {
			counts[code]++
		}
	}
	best, bestN := "", 0
	for code, n := range counts {
		if n > bestN || (n == bestN && code < best) {
			best, bestN = code, n
		}
	}
	if best == "" {
		return "02000"
	}
	return best
}

// hrExistingEmployeeIDs 取该租户全部工号（只查一列，成本可控）。
func (h *HRHandler) hrExistingEmployeeIDs(ctx context.Context, tid int64) []string {
	var ids []string
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Where("tenant_id = ?", tid).Pluck("employee_id", &ids).Error; err != nil {
		return nil
	}
	return ids
}

// hrDeptCodeOf 取组织机构里该部门的 5 位编码；没有则从存量工号反推。
func (h *HRHandler) hrDeptCodeOf(ctx context.Context, tid int64, department string) string {
	dept := strings.TrimSpace(department)
	if dept != "" {
		var row HRDepartment
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND name = ?", tid, dept).First(&row).Error; err == nil {
			if len(strings.TrimSpace(row.Code)) == 5 {
				return strings.TrimSpace(row.Code)
			}
		}
	}
	// 回退：从该部门存量工号里取最常见的 5 位码
	var ids []string
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Where("tenant_id = ? AND department = ?", tid, dept).
		Pluck("employee_id", &ids).Error; err == nil && len(ids) > 0 {
		return hrGuessDeptCode(ids)
	}
	return ""
}

// NextEmployeeID GET /roster/next-employee-id?department=&title=
// 返回建议工号，供「新增员工」抽屉自动填充（用户仍可手改）。
func (h *HRHandler) NextEmployeeID(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	dept := strings.TrimSpace(c.Query("department"))
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		title = strings.TrimSpace(c.Query("position"))
	}
	code := h.hrDeptCodeOf(ctx, tid, dept)
	ids := h.hrExistingEmployeeIDs(ctx, tid)
	next := hrNextEmployeeID(code, title, ids)
	hrOK(c, gin.H{"employee_id": next, "dept_code": code, "rank": hrRankOfTitle(title)})
}
