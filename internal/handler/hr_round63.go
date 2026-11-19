// 第 63 轮：花名册「合同公司」→「签约公司」全量迁移、单位设置补齐、
// 宿舍楼/房型修正、宿舍字段契约 v3（所属公司 / 所属部门拆分）。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	hrRosterCompany63 = "hr_roster_company_v63c"
	hrDormSubsKey63   = "hr_dorm_subs_v63"
	hrDormBuilding63  = "hr_dorm_building_v63"
	// 第 64 轮：费用信息胶囊重建为月度账单汇总（15 列）
	hrDormFeeSubsKey64 = "hr_dorm_fee_subs_v64"
	// 第 64 轮：补齐宿舍/公租房胶囊缺失的「所属部门」
	hrDormDeptSubKey64 = "hr_dorm_dept_sub_v64"
)

// hrEnsureOrgUnits63 单位设置至少包含「星达 + 持睿」两家公司（幂等）。
//
// 为什么需要：花名册签约公司统一为星达后，hrEnsureOrgUnits 从花名册提取单位名
// 只会得到星达一家；新租户会丢失持睿，而宿舍/公租房的所属公司、表计使用单位
// 都要能选到它。这里按名称补齐，不覆盖用户已维护的信用代码等字段。
func (h *HRHandler) hrEnsureOrgUnits63(ctx context.Context, tid int64) error {
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&HROrgUnit{}).Where("tenant_id = ?", tid).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		h.hrEnsureOrgUnits(ctx, tid)
	}
	var units []HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&units).Error; err != nil {
		return err
	}
	seen := map[string]bool{}
	maxSort := 0
	for _, u := range units {
		seen[strings.TrimSpace(u.Name)] = true
		if u.SortOrder > maxSort {
			maxSort = u.SortOrder
		}
	}
	for _, name := range []string{"重庆星达铜业有限公司", "重庆持睿汽车部件有限公司"} {
		if seen[name] {
			continue
		}
		maxSort++
		credit := ""
		if strings.Contains(name, "星达") {
			credit = hrDefaultCreditCode
		}
		if err := h.db.WithContext(ctx).Create(&HROrgUnit{
			ID: uuid.New().String(), TenantID: tid, Name: name,
			Code: strings.TrimSpace(name), CreditCode: credit, SortOrder: maxSort,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// hrFixRosterContractCompany63 花名册签约公司归一 + subs 改名（幂等）。
func (h *HRHandler) hrFixRosterContractCompany63(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrRosterCompany63) {
		return nil
	}
	// 1) 员工数据：签约公司 / 合同公司 双键统一写标准名（整行 Save 走 JSON 序列化）
	var emps []HREmployee
	if err := h.db.WithContext(ctx).Find(&emps).Error; err != nil {
		return err
	}
	fixed := 0
	for i := range emps {
		if emps[i].CustomData == nil {
			emps[i].CustomData = map[string]any{}
		}
		changed := false
		for _, key := range []string{"签约公司", "合同公司"} {
			if strings.TrimSpace(hrDormStr(emps[i].CustomData, key)) == hrContractCompanyName {
				continue
			}
			emps[i].CustomData[key] = hrContractCompanyName
			changed = true
		}
		if !changed {
			continue
		}
		if err := h.db.WithContext(ctx).Save(&emps[i]).Error; err != nil {
			return err
		}
		fixed++
	}

	// 2) 字段设置：合同公司 → 签约公司（保序、去重、保留启用/宽度/默认表头）
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope IN ? AND deleted_at IS NULL", []string{hrScopeRoster, hrScopeResign}).
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		subs := cats[i].Subs
		seen := map[string]bool{}
		out := make([]types.FleetCategorySub, 0, len(subs)+1)
		for _, s := range subs {
			if s.Name == "合同公司" {
				s.Name = "签约公司"
				if s.Label == "" || s.Label == "合同公司" {
					s.Label = "签约公司"
				}
			}
			// 第 63 轮：签约公司进入默认表头（用户可在字段筛选器里取消）
			if s.Name == "签约公司" {
				s.IsDefault = true
			}
			key := s.Group + "\x00" + s.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
		// 离职胶囊原本没有「签约公司」：补一条并紧跟「岗位」，
		// 分组沿用「岗位」所在组（v63b 曾写成 Group="离职人员" 追加到末尾，
		// 会让离职抽屉凭空多出一个空分组，这里一并纠正）。
		if cats[i].Scope == hrScopeResign {
			out = hrEnsureSubAfter(out, "岗位", "签约公司")
		}
		cats[i].Subs = out
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}

	// 3) 单位设置补齐两家公司
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HROrgUnit{}).Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	if len(tids) == 0 {
		_ = h.db.WithContext(ctx).Model(&HREmployee{}).Distinct().Pluck("tenant_id", &tids).Error
	}
	for _, tid := range tids {
		if err := h.hrEnsureOrgUnits63(ctx, tid); err != nil {
			return err
		}
	}

	h.hrMarkSystemFlag(ctx, hrRosterCompany63)
	logger.Infof(ctx, "HR roster contract company normalized: %d employees, %d categories", fixed, len(cats))
	return nil
}

// hrEnsureDormDeptSub64 补齐宿舍/公租房胶囊缺失的「所属部门」（幂等）。
//
// 第 63 轮迁移用 `groupHas` 判断分组是否已有公司/部门字段，命中「所属公司」就
// 跳过整组，导致只有「所属公司」的存量库漏补「所属部门」—— 设置抽屉里根本
// 找不到这个字段，用户也就无法取消它的默认表头。这里按字段逐个补齐。
func (h *HRHandler) hrEnsureDormDeptSub64(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrDormDeptSubKey64) {
		return nil
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeDorm).Find(&cats).Error; err != nil {
		return err
	}
	fixed := 0
	for i := range cats {
		subs := cats[i].Subs
		for _, grp := range []string{hrDormGroupRoom, hrDormGroupPublic} {
			haveDept := false
			anchorPos := -1
			for idx, s := range subs {
				if s.Group != grp {
					continue
				}
				if s.Name == "所属部门" {
					haveDept = true
				}
				if s.Name == "所属公司" {
					anchorPos = idx
				}
			}
			if haveDept {
				continue
			}
			item := types.FleetCategorySub{
				Name: "所属部门", Label: "所属部门", Enabled: true, IsDefault: true,
				DataType: "text", Group: grp, GroupLabel: grp,
			}
			if anchorPos >= 0 {
				subs = append(subs[:anchorPos+1], append([]types.FleetCategorySub{item}, subs[anchorPos+1:]...)...)
			} else {
				subs = append(subs, item)
			}
			fixed++
		}
		cats[i].Subs = subs
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}
	h.hrMarkSystemFlag(ctx, hrDormDeptSubKey64)
	logger.Infof(ctx, "HR dorm 所属部门 subs ensured: %d inserted", fixed)
	return nil
}

// hrMigrateDormFeeSubs64 费用信息胶囊重建为「月度账单汇总」15 列（幂等）。
//
// 第 64 轮：主列表从「一人一行明细」改成「每月 1 行汇总」，列集合随之更换。
// 不重建的话，落库 subs 仍是旧的 19 个明细字段，前端按 subs 派生列会与新的
// 汇总列几乎零交集（真机只剩「账单月份 / 账单状态」两列）。
func (h *HRHandler) hrMigrateDormFeeSubs64(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrDormFeeSubsKey64) {
		return nil
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeDorm).Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		subs := cats[i].Subs
		// 只替换「费用信息」分组，其余四胶囊原样保留
		out := make([]types.FleetCategorySub, 0, len(subs)+16)
		inserted := false
		for _, s := range subs {
			if s.Group != hrDormGroupFee {
				out = append(out, s)
				continue
			}
			if !inserted {
				out = append(out, hrDormFeeSubs64()...)
				inserted = true
			}
		}
		if !inserted {
			out = append(out, hrDormFeeSubs64()...)
		}
		cats[i].Subs = out
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}
	h.hrMarkSystemFlag(ctx, hrDormFeeSubsKey64)
	logger.Infof(ctx, "HR dorm fee subs migrated to v64 (monthly bill): %d categories", len(cats))
	return nil
}

// hrDormFeeSubs64 费用信息 15 列（与 hrDormFieldDefs 的 feeG 段同源）。
func hrDormFeeSubs64() []types.FleetCategorySub {
	names := []string{
		"账单月份", "总人数", "住宿人数", "押金合计", "房租合计",
		"水费合计", "电费合计", "气费合计", "社保个人合计", "公积金个人合计",
		"其它扣款合计", "扣款总计", "退款合计", "补贴合计", "账单状态",
	}
	out := make([]types.FleetCategorySub, 0, len(names))
	for _, n := range names {
		dataType := "number"
		switch n {
		case "账单月份":
			dataType = "text"
		case "账单状态":
			dataType = "system"
		}
		out = append(out, types.FleetCategorySub{
			Name: n, Label: n, Enabled: true, IsDefault: true,
			DataType: dataType, Group: hrDormGroupFee, GroupLabel: hrDormGroupFee,
		})
	}
	return out
}

// hrEnsureSubAfter 保证 subs 里有且只有一个 name，且紧跟在 anchor 之后。
// 已存在时先摘除（无论原先落在哪个分组），再按 anchor 的分组重建，保证顺序与分组正确。
func hrEnsureSubAfter(subs []types.FleetCategorySub, anchor, name string) []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, len(subs)+1)
	pos := -1
	for _, s := range subs {
		if s.Name == name {
			continue
		}
		out = append(out, s)
		if s.Name == anchor {
			pos = len(out) - 1
		}
	}
	item := types.FleetCategorySub{
		Name: name, Label: name, Enabled: true, IsDefault: true, DataType: "text",
	}
	if pos >= 0 {
		item.Group, item.GroupLabel = out[pos].Group, out[pos].GroupLabel
	}
	if pos < 0 {
		return append(out, item)
	}
	return append(out[:pos+1], append([]types.FleetCategorySub{item}, out[pos+1:]...)...)
}

// hrMigrateDormSubs63 宿舍字段契约 v3：所属公司/部门 拆成两个字段。
func (h *HRHandler) hrMigrateDormSubs63(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrDormSubsKey63) {
		return nil
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeDorm).Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		subs := cats[i].Subs
		// 先按原顺序搬一遍，合并「所属公司/部门」为「所属公司」+「所属部门」
		out := make([]types.FleetCategorySub, 0, len(subs)+2)
		seen := map[string]bool{}
		for _, s := range subs {
			grp := s.Group
			key := grp + "\x00" + s.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			if s.Name == "所属公司/部门" {
				comp := s
				comp.Name, comp.Label, comp.DataType = "所属公司", "所属公司", "text"
				out = append(out, comp)
				dept := s
				dept.Name, dept.Label, dept.DataType = "所属部门", "所属部门", "text"
				out = append(out, dept)
				seen[grp+"\x00所属公司"] = true
				seen[grp+"\x00所属部门"] = true
				continue
			}
			out = append(out, s)
		}
		// v62 的公租房分组没有「所属公司/部门」：在「租赁单位」/「配套设施」之后补两条
		// 第 64 轮：必须**逐字段**判断 —— 旧实现只看 groupHas（命中「所属公司」即视为齐），
		// 存量库只有「所属公司」时会漏补「所属部门」，真机上设置抽屉里就找不到该字段。
		for _, grp := range []string{hrDormGroupRoom, hrDormGroupPublic} {
			haveComp, haveDept := false, false
			for _, s := range out {
				if s.Group != grp {
					continue
				}
				if s.Name == "所属公司" {
					haveComp = true
				}
				if s.Name == "所属部门" {
					haveDept = true
				}
			}
			if haveComp && haveDept {
				continue
			}
			anchor := "配套设施"
			if grp == hrDormGroupPublic {
				anchor = "租赁单位"
			}
			if haveComp && !haveDept {
				anchor = "所属公司"
			}
			pos := -1
			for idx := range out {
				if out[idx].Group == grp && out[idx].Name == anchor {
					pos = idx
					break
				}
			}
			base := types.FleetCategorySub{
				Enabled: true, DataType: "text", Group: grp, GroupLabel: grp,
			}
			comp := base
			comp.Name, comp.Label = "所属公司", "所属公司"
			dept := base
			dept.Name, dept.Label = "所属部门", "所属部门"
			add := make([]types.FleetCategorySub, 0, 2)
			if !haveComp {
				add = append(add, comp)
			}
			if !haveDept {
				add = append(add, dept)
			}
			if pos < 0 {
				out = append(out, add...)
			} else {
				out = append(out[:pos+1], append(add, out[pos+1:]...)...)
			}
		}
		cats[i].Subs = out
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}

	// 存量记录：所属公司/部门 → 所属公司（部门留空，不造假）
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("module IN ?", []string{hrDormModuleRoom, hrDormModulePublic}).
		Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		d := rows[i].CustomData
		if d == nil {
			continue
		}
		old := strings.TrimSpace(hrDormStr(d, "所属公司/部门"))
		if old != "" && strings.TrimSpace(hrDormStr(d, "所属公司")) == "" {
			d["所属公司"] = old
			if _, ok := d["所属部门"]; !ok {
				d["所属部门"] = ""
			}
			rows[i].CustomData = d
			if err := h.db.WithContext(ctx).Save(&rows[i]).Error; err != nil {
				return err
			}
		}
	}

	h.hrMarkSystemFlag(ctx, hrDormSubsKey63)
	logger.Infof(ctx, "HR dorm subs migrated to v63: %d categories, %d records", len(cats), len(rows))
	return nil
}

// hrFixDormBuildingAndType63 22 间员工宿舍统一「1号楼 / 双人间 / 2 床」。
func (h *HRHandler) hrFixDormBuildingAndType63(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrDormBuilding63) {
		return nil
	}
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("module = ?", hrDormModuleRoom).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		d := rows[i].CustomData
		if d == nil {
			d = map[string]any{}
		}
		d["宿舍楼/区域"] = "1号楼"
		d["房型"] = "双人间"
		d["床位数"] = 2
		rows[i].CustomData = d
		if err := h.db.WithContext(ctx).Save(&rows[i]).Error; err != nil {
			return err
		}
	}
	h.hrMarkSystemFlag(ctx, hrDormBuilding63)
	logger.Infof(ctx, "HR dorm building/type normalized: %d rooms", len(rows))
	return nil
}
