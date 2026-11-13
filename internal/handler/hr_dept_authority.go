// 第 58 轮：部门权威落库。
//
// 第 57 轮只在「读取时」用花名册覆盖部门（身份证优先 / 姓名回退），界面正确，
// 但库里仍是导入文件里的旧部门：直接查库、走导出接口、或在别处 JOIN department
// 都会拿到第二套值。本文件把权威部门**写回**五张落库表，并保证幂等：
// 仅在目标值与现存值不同时才 UPDATE。
//
// 触发点（全部幂等，失败只告警不阻断业务）：
//   - 启动时全租户存量对齐；
//   - 导入社保回盘表 / 正常异常记录、导入公积金职工明细；
//   - 生成社保 / 公积金月度账单；
//   - 花名册改员工部门、组织机构改部门名（SaveOrganization / SaveOrgUnits）。
package handler

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
)

// hrDeptPersistResult 本次写回的行数（按表拆开，便于日志与单测断言）。
type hrDeptPersistResult struct {
	Receipts    int
	Records     int
	BillItems   int
	PFRecords   int
	PFBillItems int
}

// total 五张表合计写回行数。
func (r hrDeptPersistResult) total() int {
	return r.Receipts + r.Records + r.BillItems + r.PFRecords + r.PFBillItems
}

// hrDeptAuthorityRow 各表统一读出的最小列集（identity_number 会先别名成 id_number）。
type hrDeptAuthorityRow struct {
	ID         string
	IDNumber   string
	Name       string
	Department string
}

// hrPersistDeptAuthority 把花名册权威部门写回社保 / 公积金五张表（幂等）。
//
// 口径与读取路径严格一致：
//   - 社保回盘表 / 记录 / 公积金台账：写花名册原名（读时也直接用原名）；
//   - 社保账单明细：写 hrSIDeptLabel 归一后的展示名（读时同样过一次 label），
//     两者幂等（财务部 → 财务部），不会来回抖动。
func (h *HRHandler) hrPersistDeptAuthority(ctx context.Context, tid int64) (hrDeptPersistResult, error) {
	var out hrDeptPersistResult
	auth := h.hrSIDeptAuthority(ctx, tid)
	if len(auth.byID) == 0 && len(auth.byName) == 0 {
		return out, nil
	}
	specs := []struct {
		model  any
		idCol  string
		label  bool
		custom bool
		dest   *int
	}{
		{&HRSIReceipt{}, "id_number", false, true, &out.Receipts},
		{&HRSIRecord{}, "id_number", false, true, &out.Records},
		{&HRSIBillItem{}, "id_number", true, false, &out.BillItems},
		{&HRPFRecord{}, "identity_number", false, true, &out.PFRecords},
		{&HRPFBillItem{}, "identity_number", false, false, &out.PFBillItems},
	}
	for _, sp := range specs {
		n, err := h.hrSyncDeptColumn(ctx, tid, sp.model, sp.idCol, sp.label, sp.custom, auth)
		if err != nil {
			return out, err
		}
		*sp.dest = n
	}
	return out, nil
}

// hrSyncDeptColumn 单表对齐：按目标部门分组批量 UPDATE，避免逐行写放大。
func (h *HRHandler) hrSyncDeptColumn(ctx context.Context, tid int64, model any,
	idCol string, label, withCustom bool, auth hrSIDeptAuthority,
) (int, error) {
	sel := "id, COALESCE(" + idCol + ", '') AS id_number, COALESCE(name, '') AS name, " +
		"COALESCE(department, '') AS department"
	var rows []hrDeptAuthorityRow
	if err := h.db.WithContext(ctx).Model(model).Select(sel).
		Where("tenant_id = ?", tid).Find(&rows).Error; err != nil {
		return 0, err
	}
	byDept := map[string][]string{}
	for _, r := range rows {
		want := auth.deptOf(r.IDNumber, r.Name)
		if want == "" {
			continue
		}
		if label {
			want = hrSIDeptLabel(want)
		}
		if strings.TrimSpace(r.Department) == want {
			continue
		}
		byDept[want] = append(byDept[want], r.ID)
	}
	if len(byDept) == 0 {
		return 0, nil
	}
	updated := 0
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for dept, ids := range byDept {
			updates := map[string]any{"department": dept}
			// custom_data.部门 同步写：导出/离线读取直接读 JSONB 时也是权威值。
			if withCustom && hrIsPostgres(tx) {
				updates["custom_data"] = gorm.Expr(
					`jsonb_set(COALESCE(custom_data, '{}'::jsonb), '{部门}', to_jsonb(?::text), true)`, dept)
			}
			res := tx.Model(model).Where("id IN ?", ids).Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			updated += int(res.RowsAffected)
		}
		return nil
	})
	return updated, err
}

// hrPersistDeptAuthorityAllTenants 启动钩子：对每个已有员工的租户跑一次存量对齐。
func (h *HRHandler) hrPersistDeptAuthorityAllTenants(ctx context.Context) (int, error) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, tid := range tids {
		res, err := h.hrPersistDeptAuthority(ctx, tid)
		if err != nil {
			return total, err
		}
		total += res.total()
	}
	return total, nil
}

// hrPersistDeptAuthorityQuiet 业务路径上的「尽力而为」版本：失败只告警。
//
// 部门是展示维度，写回失败不该让导入 / 生成账单整体失败 —— 读取路径仍会实时覆盖，
// 用户看到的界面不会错，只是库里的存量值晚一轮对齐。
func (h *HRHandler) hrPersistDeptAuthorityQuiet(ctx context.Context, tid int64) {
	res, err := h.hrPersistDeptAuthority(ctx, tid)
	if err != nil {
		logger.Warnf(ctx, "persist HR dept authority failed (tenant=%d): %v", tid, err)
		return
	}
	if res.total() > 0 {
		logger.Infof(ctx, "HR dept authority persisted (tenant=%d): receipts=%d records=%d bill_items=%d pf=%d pf_bill_items=%d",
			tid, res.Receipts, res.Records, res.BillItems, res.PFRecords, res.PFBillItems)
	}
}
