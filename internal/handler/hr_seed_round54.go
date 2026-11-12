// 第 54 轮初始化数据：3 条测试离职员工。
//
// 目的：让「离职管理」开箱有数据，可以真机验证「花名册 → 办理离职 → 离职管理」
// 与「离职管理 → 恢复在职 → 花名册」的双向流转，以及劳动合同的「已解除」状态。
//
// 幂等：仅当该租户没有任何离职员工时写入；已存在则完全不动。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
)

type hrResignSample struct {
	employeeID string
	name       string
	idNumber   string
	department string
	position   string
	hireDate   string
	applyDate  string
	resignDate string
	resignType string
	reason     string
	where      string
	settleDate string
}

var hrResignSamples = []hrResignSample{
	{
		employeeID: "C020500201", name: "王建国", idNumber: "510232198305164512",
		department: "生产部", position: "操作员", hireDate: "2019-03-12",
		applyDate: "2026-08-20", resignDate: "2026-09-19", resignType: "主动离职",
		reason: "个人发展", where: "重庆万州", settleDate: "2026-09-25",
	},
	{
		employeeID: "C020300202", name: "李思远", idNumber: "500227199107082318",
		department: "销售部", position: "业务员", hireDate: "2021-06-01",
		applyDate: "2026-09-02", resignDate: "2026-09-30", resignType: "主动离职",
		reason: "薪酬福利", where: "成都", settleDate: "2026-10-10",
	},
	{
		employeeID: "C020600203", name: "张雨欣", idNumber: "510722199504236625",
		department: "技术质量部", position: "质检员", hireDate: "2022-11-15",
		applyDate: "2026-09-15", resignDate: "2026-10-14", resignType: "被动离职",
		reason: "合同到期不续签", where: "重庆", settleDate: "2026-10-20",
	},
}

// hrSeedResignSamples 写入 3 条测试离职员工（幂等）。
func (h *HRHandler) hrSeedResignSamples(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, tid := range tids {
		var resigned int64
		if err := h.db.WithContext(ctx).Model(&HREmployee{}).
			Where("tenant_id = ? AND status IN ?", tid,
				[]string{"离职", "resigned"}).
			Count(&resigned).Error; err != nil {
			return err
		}
		if resigned > 0 {
			continue
		}
		for _, s := range hrResignSamples {
			var dup int64
			_ = h.db.WithContext(ctx).Model(&HREmployee{}).
				Where("tenant_id = ? AND id_number = ?", tid, s.idNumber).Count(&dup).Error
			if dup > 0 {
				continue
			}
			emp := HREmployee{
				ID:            uuid.New().String(),
				TenantID:      tid,
				EmployeeID:    s.employeeID,
				Name:          s.name,
				IDNumber:      s.idNumber,
				Gender:        hrGenderOfIDNumber(s.idNumber),
				BirthMonth:    hrBirthDateOfIDNumber(s.idNumber),
				Department:    s.department,
				Position:      s.position,
				HireDate:      s.hireDate,
				Status:        hrRosterStatusResigned,
				ResignDate:    s.resignDate,
				ResignReasons: s.reason,
			}
			emp.CustomData = hrMergeEmployeeCustomData(nil, map[string]string{
				"工号":       s.employeeID,
				"姓名":       s.name,
				"部门":       s.department,
				"岗位":       s.position,
				"证件类型":     "身份证",
				"证件号码":     s.idNumber,
				"性别":       emp.Gender,
				"出生日期":     emp.BirthMonth,
				"入职日期":     s.hireDate,
				"员工状态":     hrRosterStatusResigned,
				"在职状态":     hrRosterStatusResigned,
				"离职申请日期":   s.applyDate,
				"离职日期":     s.resignDate,
				"离职类型":     s.resignType,
				"离职原因":     s.reason,
				"离职去向":     s.where,
				"薪资结算日":    s.settleDate,
				"是否试用期内离职": "否",
				"司龄":       hrSeniorityOf(&emp, now),
				"合同状态":     hrContractStatusTerminated,
			})
			if err := h.db.WithContext(ctx).Create(&emp).Error; err != nil {
				logger.Warnf(ctx, "seed resign sample %s failed: %v", s.name, err)
			}
		}
		logger.Infof(ctx, "离职管理样例已就绪（tenant=%d）", tid)
	}
	return nil
}

// hrGenderOfIDNumber 身份证第 17 位奇男偶女。
func hrGenderOfIDNumber(id string) string {
	s := strings.TrimSpace(id)
	if len(s) != 18 {
		return ""
	}
	if (s[16]-'0')%2 == 1 {
		return "男"
	}
	return "女"
}

// hrBirthDateOfIDNumber 身份证 7~14 位 → YYYY-MM-DD。
func hrBirthDateOfIDNumber(id string) string {
	s := strings.TrimSpace(id)
	if len(s) != 18 {
		return ""
	}
	return s[6:10] + "-" + s[10:12] + "-" + s[12:14]
}

// hrNewHireSamples 2 名新入职员工（尚无合同记录）。
//
// 为什么需要：存量 81 人的「现合同类型」全部已填（无固定期限 / 固定期限），
// 合同状态永远是「正常」，「未签订」这条分支在真机上没有样本可验证。
// 这两条新入职记录让「未签订」成为可见状态，同时演示「入职即自动出现在劳动合同」。
var hrNewHireSamples = []hrResignSample{
	{
		employeeID: "C020500211", name: "周新宇", idNumber: "510232199808152416",
		department: "生产部", position: "操作员", hireDate: "2026-10-08",
	},
	{
		employeeID: "C020300212", name: "罗佳怡", idNumber: "500227199912032826",
		department: "销售部", position: "文员", hireDate: "2026-10-20",
	},
}

// hrSeedNewHireSamples 写入 2 条无合同信息的新员工（幂等）。
func (h *HRHandler) hrSeedNewHireSamples(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	for _, tid := range tids {
		for _, s := range hrNewHireSamples {
			var dup int64
			_ = h.db.WithContext(ctx).Model(&HREmployee{}).
				Where("tenant_id = ? AND id_number = ?", tid, s.idNumber).Count(&dup).Error
			if dup > 0 {
				continue
			}
			emp := HREmployee{
				ID: uuid.New().String(), TenantID: tid,
				EmployeeID: s.employeeID, Name: s.name, IDNumber: s.idNumber,
				Gender: hrGenderOfIDNumber(s.idNumber), BirthMonth: hrBirthDateOfIDNumber(s.idNumber),
				Department: s.department, Position: s.position, HireDate: s.hireDate,
				Status: hrRosterStatusActive,
			}
			emp.CustomData = hrMergeEmployeeCustomData(nil, map[string]string{
				"工号": s.employeeID, "姓名": s.name, "部门": s.department, "岗位": s.position,
				"证件类型": "身份证", "证件号码": s.idNumber, "性别": emp.Gender,
				"出生日期": emp.BirthMonth, "入职日期": s.hireDate,
				"员工状态": hrRosterStatusActive, "在职状态": hrRosterStatusActive,
				"司龄": hrSeniorityOf(&emp, time.Now()),
			})
			if err := h.db.WithContext(ctx).Create(&emp).Error; err != nil {
				logger.Warnf(ctx, "seed new hire %s failed: %v", s.name, err)
			}
		}
	}
	return nil
}
