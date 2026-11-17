// 第 58 轮：劳动合同 / 劳务合同分流 + 社保公积金对劳务人员豁免。
//
// 业务口径：劳务人员（劳务合同 / 返聘协议 / 实习协议 / 劳务派遣 / 借调）按公司
// 规定不缴社保与公积金。因此本文件提供三件事：
//
//  1. HrContractKindOf：合同类型 → labor / service 的唯一判定口径（前端复用同一函数名）；
//  2. 劳动合同字段配置拆成「劳动合同 / 劳务合同」两个胶囊（公共字段两组各一份）；
//  3. 劳务人员身份识别 + 空占位记录清理：预警不计数、列表不出虚拟行、
//     系统自动生成的空户（无基数无金额）在启动 / 导入 / 改合同类型后清掉。
//
// 身份匹配一律「身份证号为主键、姓名回退」，与 hrSIDeptAuthority 同口径。
package handler

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 合同大类（labor 劳动合同 / service 劳务合同）。
const (
	hrContractKindLabor   = "labor"
	hrContractKindService = "service"
)

// 劳动合同字段配置的两个胶囊分组名（顺序 = 设置抽屉胶囊顺序）。
const (
	hrContractGroupLabor   = "劳动合同"
	hrContractGroupService = "劳务合同"
)

// hrLaborServiceKeywords 劳务类合同类型关键字（包含匹配，容忍前后空格）。
//
// 「劳务」覆盖 劳务合同 / 劳务派遣合同；「返聘」「实习」「借调」各自覆盖对应协议。
// 注意「固定期限劳动合同」里的「劳动」不等于「劳务」，不会被误判。
var hrLaborServiceKeywords = []string{"劳务", "返聘", "实习", "借调"}

// HrContractKindOf 合同类型 → 合同大类（labor 劳动合同 / service 劳务合同）。
//
// 这是后端的唯一判定口径，前端（字段设置 / 合同视图分组 / 社保公积金豁免提示）
// 直接复用同名函数，禁止各写一份关键字清单。空值与无法识别的类型一律 labor。
func HrContractKindOf(contractType string) string {
	s := strings.TrimSpace(contractType)
	if s == "" {
		return hrContractKindLabor
	}
	for _, kw := range hrLaborServiceKeywords {
		if strings.Contains(s, kw) {
			return hrContractKindService
		}
	}
	return hrContractKindLabor
}

// hrContractFieldDefsOfGroup 把合同字段契约整体打上指定胶囊分组。
//
// 公共字段（姓名 / 证件号码 / 工号 / 部门 + 合同编号 / 合同类型 / 三个日期 /
// 合同期限 / 签约公司 / 签订日期等）两组各生成一份，组内字段名与类型完全一致，
// 靠 Group 区分；hrSubsOfDefsByGroup 按 (group,name) 去重，两份都能留下。
func hrContractFieldDefsOfGroup(group string) []hrFieldDef {
	defs := hrContractFieldDefs()
	for i := range defs {
		defs[i].Group = group
	}
	return defs
}

// hrContractSeedSubs58 劳动合同分类 = 劳动合同 17 字段 + 劳务合同 17 字段。
//
// 第 59 轮：每组「合同状态」必须恰好一个（builtin）。两组的公共字段各一份，
// 若将来有人往公共字段里再加一个「合同状态」，这里按 (group, name) 去重兜底。
func hrContractSeedSubs58() []types.FleetCategorySub {
	defs := hrContractFieldDefsOfGroup(hrContractGroupLabor)
	defs = append(defs, hrContractFieldDefsOfGroup(hrContractGroupService)...)
	return hrDedupeContractStatus(hrSubsOfDefsByGroup(defs, true))
}

// hrDedupeContractStatus 同一胶囊内重复的「合同状态」只保留第一条（并锁定为内置）。
//
// 合同状态是系统派生字段（正常 / 未签订 / 即将到期 / 到期 / 已解除），
// 同一胶囊里出现两份会让字段设置与列表表头各显示一次，且保存顺序不确定。
func hrDedupeContractStatus(subs []types.FleetCategorySub) []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, len(subs))
	seen := make(map[string]bool, 2)
	for _, s := range subs {
		if strings.TrimSpace(s.Name) != "合同状态" {
			out = append(out, s)
			continue
		}
		key := strings.TrimSpace(s.Group)
		if seen[key] {
			continue
		}
		seen[key] = true
		s.Builtin = true
		out = append(out, s)
	}
	return out
}

// hrMigrateContractSubs58 把劳动合同分类的 subs 扩展为「劳动合同 / 劳务合同」双胶囊。
//
// 幂等：按 (group,name) 合并，保留用户自定义显示名 / 列宽 / 启停 / 默认勾选，
// 契约外的自定义字段与引用字段原样追加到末尾，绝不删除。
// 存量的单胶囊配置（group 为空或旧分组名）一律归入「劳动合同」。
func (h *HRHandler) hrMigrateContractSubs58(ctx context.Context) error {
	want := hrContractSeedSubs58()
	byKey := make(map[string]types.FleetCategorySub, len(want))
	defaultGroup := make(map[string]string, len(want))
	for _, s := range want {
		byKey[s.Group+"\x00"+s.Name] = s
		if _, ok := defaultGroup[s.Name]; !ok {
			defaultGroup[s.Name] = s.Group
		}
	}
	contractNames := make(map[string]bool, len(defaultGroup))
	for name := range defaultGroup {
		contractNames[name] = true
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", hrScopeContract, "劳动合同").
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		existing := make(map[string]types.FleetCategorySub, len(cats[i].Subs))
		extras := make([]types.FleetCategorySub, 0, 4)
		for _, sub := range cats[i].Subs {
			name := strings.TrimSpace(sub.Name)
			if name == "" {
				continue
			}
			g := strings.TrimSpace(sub.Group)
			if g != hrContractGroupLabor && g != hrContractGroupService {
				// 遗留单胶囊 / 用户自建分组：契约内字段归劳动合同，其余归劳动合同兜底。
				g = defaultGroup[name]
				if g == "" {
					g = hrContractGroupLabor
				}
			}
			key := g + "\x00" + name
			if _, dup := existing[key]; dup {
				extras = append(extras, sub)
				continue
			}
			sub.Group, sub.GroupLabel = g, g
			existing[key] = sub
		}
		next := make([]types.FleetCategorySub, 0, len(want)+len(extras))
		for _, s := range want {
			cur, ok := existing[s.Group+"\x00"+s.Name]
			if !ok {
				next = append(next, s)
				continue
			}
			// 只补契约元数据，用户的显示名 / 宽度 / 启停 / 默认勾选一律保留。
			if cur.DataType == "" {
				cur.DataType = s.DataType
			}
			if s.Builtin && !cur.Builtin {
				cur.Builtin = true
			}
			if s.Source != nil && cur.Source == nil {
				src := *s.Source
				cur.Source = &src
			}
			next = append(next, cur)
		}
		// 契约外的用户自定义字段 / 跨表引用字段：按原顺序追加末尾。
		for _, sub := range extras {
			next = append(next, sub)
		}
		for _, sub := range cats[i].Subs {
			name := strings.TrimSpace(sub.Name)
			if name == "" || contractNames[name] {
				continue
			}
			next = append(next, sub)
		}
		// 第 59 轮：同一胶囊只留一个「合同状态」。
		next = hrDedupeContractStatus(next)
		if reflect.DeepEqual(cats[i].Subs, next) {
			continue
		}
		cats[i].Subs = next
		cats[i].UpdatedAt = time.Now()
		// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "劳动合同字段配置已拆分为劳动/劳务双胶囊（tenant=%d）", cats[i].TenantID)
	}
	return nil
}

// hrEmployeeContractType 员工当前合同类型：新字段名优先，兼容存量「现合同…」键。
func hrEmployeeContractType(e HREmployee) string {
	return strings.TrimSpace(fmt.Sprint(customValueOrEmpty(e.CustomData, "合同类型", "现合同类型", "contract_type")))
}

// hrLaborServiceIdentities 劳务人员身份表（身份证号主键、姓名回退）。
//
// 调用方把空表理解为「没有劳务人员」，因此查库失败时也返回空表 + error，
// 由调用方决定是告警还是忽略（预警统计一律忽略）。
func (h *HRHandler) hrLaborServiceIdentities(ctx context.Context, tid int64) (map[string]bool, map[string]bool, error) {
	byID := map[string]bool{}
	byName := map[string]bool{}
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&employees).Error; err != nil {
		return byID, byName, err
	}
	for _, e := range employees {
		if HrContractKindOf(hrEmployeeContractType(e)) != hrContractKindService {
			continue
		}
		if id := normalizeIDNumber(e.IDNumber); id != "" {
			byID[id] = true
		}
		if n := strings.TrimSpace(e.Name); n != "" {
			byName[n] = true
		}
	}
	return byID, byName, nil
}

// hrLaborServiceHit 身份证命中优先；没有身份证或命中不到时回退姓名。
func hrLaborServiceHit(byID, byName map[string]bool, idNumber, name string) bool {
	if len(byID) == 0 && len(byName) == 0 {
		return false
	}
	if id := normalizeIDNumber(idNumber); id != "" && byID[id] {
		return true
	}
	n := strings.TrimSpace(name)
	return n != "" && byName[n]
}

// hrPurgeLaborServicePlaceholders 删除劳务人员在社保 / 公积金里的系统空占位记录。
//
// 判据（任何一条不满足都保留，绝不碰有金额或有人工备注的行）：
//
//	公积金 hr_pf_records：status=active 且 基数 <= 0 且 个人/单位/合计金额全为 0
//	                     且 notes 为空（notes 非空 = 用户导入或人工备注）。
//	                     该表没有 source 列，按约定忽略。
//	社保   hr_si_records ：source='import' 且 base=0 且 单位/个人/合计金额全为 0。
func (h *HRHandler) hrPurgeLaborServicePlaceholders(ctx context.Context, tid int64) (int, error) {
	byID, byName, err := h.hrLaborServiceIdentities(ctx, tid)
	if err != nil {
		return 0, err
	}
	if len(byID) == 0 && len(byName) == 0 {
		return 0, nil
	}
	removed := 0
	var pf []HRPFRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tid, hrFundStatusActive).
		Find(&pf).Error; err != nil {
		return removed, err
	}
	pfIDs := make([]string, 0, len(pf))
	for _, r := range pf {
		if !hrLaborServiceHit(byID, byName, r.IdentityNumber, r.Name) {
			continue
		}
		if r.PersonalBase > 0 || r.PersonalAmount != 0 || r.CompanyAmount != 0 || r.TotalAmount != 0 {
			continue
		}
		if strings.TrimSpace(r.Notes) != "" {
			continue
		}
		pfIDs = append(pfIDs, r.ID)
	}
	if len(pfIDs) > 0 {
		if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, pfIDs).
			Delete(&HRPFRecord{}).Error; err != nil {
			return removed, err
		}
		removed += len(pfIDs)
	}
	var si []HRSIRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND source = ? AND base <= 0 AND unit_amount = 0 AND personal_amount = 0 AND total_amount = 0",
			tid, "import").Find(&si).Error; err != nil {
		return removed, err
	}
	siIDs := make([]string, 0, len(si))
	for _, r := range si {
		if !hrLaborServiceHit(byID, byName, r.IDNumber, r.Name) {
			continue
		}
		siIDs = append(siIDs, r.ID)
	}
	if len(siIDs) > 0 {
		if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, siIDs).
			Delete(&HRSIRecord{}).Error; err != nil {
			return removed, err
		}
		removed += len(siIDs)
	}
	return removed, nil
}

// hrPurgeLaborServicePlaceholdersAllTenants 启动钩子：对每个已有员工的租户跑一次清理。
func (h *HRHandler) hrPurgeLaborServicePlaceholdersAllTenants(ctx context.Context) (int, error) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, tid := range tids {
		n, err := h.hrPurgeLaborServicePlaceholders(ctx, tid)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// hrPurgeLaborServicePlaceholdersQuiet 业务路径上的尽力而为版本：失败只告警。
func (h *HRHandler) hrPurgeLaborServicePlaceholdersQuiet(ctx context.Context, tid int64) {
	n, err := h.hrPurgeLaborServicePlaceholders(ctx, tid)
	if err != nil {
		logger.Warnf(ctx, "purge labor-service placeholders failed (tenant=%d): %v", tid, err)
		return
	}
	if n > 0 {
		logger.Infof(ctx, "purged %d labor-service placeholder rows (tenant=%d)", n, tid)
	}
}
