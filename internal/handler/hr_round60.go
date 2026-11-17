// 第 60 轮：胶囊级加锁状态字段清理 + 社保月度账单生成前置校验。
//
// 两件事：
//
//  1. hrRepair60Once —— 幂等清理字段配置里的「串组加锁字段」。
//     第 60 轮之前，前端把**所有加锁状态字段**无条件渲染进每个胶囊
//     （`group === want || isLockedStatus(f)`），用户点一次保存就把穿透出来的
//     重复 / 跨组字段写进 subs：劳动合同胶囊里两条「合同状态」、正常记录里
//     5 个五险参保状态 + 账单状态。前端已改为按胶囊白名单严格过滤，这里把
//     存量脏数据一次性清掉（只删错放的，绝不新增字段）。
//
//  2. 社保月度账单只允许**手动生成**，且生成前必须校验：
//     · 该月份没有任何「正常记录」→ 直接拒绝（不允许生成空账单）；
//     · 已有同月账单 → 由调用方确认 overwrite 后再覆盖。
//     前端另有 precheck 接口，用于「先提示、再决定发不发写请求」。
//
// 边界：本文件只新增，不改上游核心代码；标识符全部以 hr 前缀收口。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// hrGroupStatusFields60 人事胶囊 → 唯一合法的加锁状态字段（与前端 fieldScope.ts 同源）。
//
// 空切片 = 该胶囊 0 个加锁字段（正常记录 / 补退记录 / 花名册双胶囊）。
// 未列出的胶囊名不参与清理（用户自建胶囊保持原样）。
var hrGroupStatusFields60 = map[string][]string{
	"劳动合同": {"合同状态"},
	"劳务合同": {"合同状态"},
	"回盘表":  {"养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态"},
	"正常记录": {},
	"补退记录": {},
	"月度账单": {"账单状态"},
	"职工明细": {"账户状态"},
	"在职人员": {},
	"离职人员": {},
}

// hrStatusNames60 人事模块所有「系统内置加锁状态字段」的字段名。
//
// 只有命中这份名单的字段才可能被清理 —— 用户自定义的同名字段（例如自己建的
// 「账户状态」业务列）若落在白名单胶囊里会保留，落在非白名单胶囊则视为串组脏数据。
var hrStatusNames60 = []string{
	"在职状态", "账期状态", "参保状态", "缴存状态",
	"合同状态", "账户状态", "账单状态",
	"养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态",
}

// hrSIBillNoRecordMsg 生成账单被前置校验拦截时的统一文案（前端 Toast 同文案）。
const hrSIBillNoRecordMsg = "该月份暂无正常社保记录，请先上传社保系统导出的记录数据"

// hrIsStatusName60 是否人事加锁状态字段名。
func hrIsStatusName60(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return false
	}
	for _, s := range hrStatusNames60 {
		if n == s {
			return true
		}
	}
	return false
}

// hrAllowedStatusOf60 该胶囊允许的状态字段；ok=false 表示胶囊不在白名单表里。
func hrAllowedStatusOf60(group string) (allowed []string, ok bool) {
	allowed, ok = hrGroupStatusFields60[strings.TrimSpace(group)]
	return allowed, ok
}

// hrRepair60Once 幂等清理：
//
//	(a) 去掉不属于该胶囊白名单的加锁状态字段（如正常记录里的五险参保状态、账单状态）；
//	(b) 同一胶囊内重复的加锁状态字段按字段名去重，保留第一条；
//	(c) 命中的加锁字段补 builtin=true（老库里可能缺这个标记）。
func (h *HRHandler) hrRepair60Once(ctx context.Context) error {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?", []string{
			hrScopeRoster, hrScopeResign, hrScopeContract, hrScopeSocial, hrScopeFund,
		}).
		Find(&cats).Error; err != nil {
		return err
	}
	total := 0
	for i := range cats {
		next := make([]types.FleetCategorySub, 0, len(cats[i].Subs))
		seen := map[string]bool{}
		changed := false
		for _, sub := range cats[i].Subs {
			group := strings.TrimSpace(sub.Group)
			name := strings.TrimSpace(sub.Name)
			allowed, inTable := hrAllowedStatusOf60(group)
			if inTable && hrIsStatusName60(name) {
				if !hrContainsString60(allowed, name) {
					changed = true
					continue
				}
				key := group + "\x00" + name
				if seen[key] {
					changed = true
					continue
				}
				seen[key] = true
				if !sub.Builtin {
					sub.Builtin = true
					changed = true
				}
			}
			next = append(next, sub)
		}
		if !changed {
			continue
		}
		cats[i].Subs = next
		cats[i].UpdatedAt = time.Now()
		// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		total++
		logger.Infof(ctx, "HR 胶囊状态字段清理完成（scope=%s tenant=%d）", cats[i].Scope, cats[i].TenantID)
	}
	if total > 0 {
		logger.Infof(ctx, "HR 胶囊状态字段清理：共修复 %d 个分类", total)
	}
	return nil
}

func hrContainsString60(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// hrSIRecordCountsOf 某月份的正常 / 补退记录条数（生成账单的前置校验口径）。
func (h *HRHandler) hrSIRecordCountsOf(ctx context.Context, tid int64, month string) (normal, adjust int64, err error) {
	// 两条独立查询：GORM 的 Where 会就地累积到同一个 Statement，
	// 复用 base 会让第二个 Count 带上第一个的 record_type 条件（永远 0）。
	countOf := func(kind string) (int64, error) {
		var n int64
		e := h.db.WithContext(ctx).Model(&HRSIRecord{}).
			Where("tenant_id = ? AND bill_month = ? AND record_type = ?", tid, month, kind).
			Count(&n).Error
		return n, e
	}
	if normal, err = countOf("normal"); err != nil {
		return 0, 0, err
	}
	if adjust, err = countOf("abnormal"); err != nil {
		return 0, 0, err
	}
	return normal, adjust, nil
}

// PrecheckSIBill GET /hr/social/bills/precheck?month=YYYY-MM
//
// 只读：告诉前端「这个月能不能生成账单」。前端据此先弹 Toast 拦截，
// 避免用户点「确定」后收到一个 400 需要自己翻译成人话。
func (h *HRHandler) PrecheckSIBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	month := strings.TrimSpace(c.Query("month"))
	if len(month) != 7 || month[4] != '-' {
		hrFail(c, "账单月份必须为 YYYY-MM")
		return
	}
	normal, adjust, err := h.hrSIRecordCountsOf(ctx, tid, month)
	if err != nil {
		hrFail(c, "检查社保记录失败: "+err.Error())
		return
	}
	var bills int64
	if err := h.db.WithContext(ctx).Model(&HRSIBill{}).
		Where("tenant_id = ? AND month_label = ?", tid, month).Count(&bills).Error; err != nil {
		hrFail(c, "检查账单失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{
		"month":               month,
		"normal_record_count": normal,
		"adjust_record_count": adjust,
		"bill_exists":         bills > 0,
	})
}
