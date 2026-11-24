// 第 69 轮：身份证号星号还原 + 全量花名册（含离职）部门回填。
//
// 两个真实问题，根因不同但都要在同一张员工主档上收敛：
//
//	① **星号身份证**：税务/社保网厅导出的原始 Excel 自带
//	   `前6位 + ******** + 后4位` 掩码（2026-09 正常记录 390 条全是这种形态）。
//	   身份证被掩码后，按身份证关联部门/劳务豁免规则全部失效，而员工主档里
//	   明明存着完整 18 位。导入解析与存量数据都必须还原成明文，系统**不脱敏**。
//	② **离职人员部门丢失**：关联花名册时若附加 `status = '在职'`，离职员工的
//	   历史部门/岗位/工号就查不到（他们的档案仍在花名册里，只是状态变了）。
//	   这里统一改查「全量花名册（仅排除物理删除）」，并对存量空部门做幂等回填。
package handler

import (
	"context"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// hrIDMaskPrefixSuffix 拆出打码身份证的「前缀 + 后缀」。
//
//	500225********6884 → ("500225", "6884", true)
//	510232196807023318 → ("", "", false)
//
// 只在含 `*` 时返回 ok=true；前后缀为空（如 `****`）也算打码，但无法用于匹配。
func hrIDMaskPrefixSuffix(id string) (string, string, bool) {
	s := normalizeIDNumber(id)
	if !strings.Contains(s, "*") {
		return "", "", false
	}
	star := strings.Index(s, "*")
	end := star
	for end < len(s) && s[end] == '*' {
		end++
	}
	return s[:star], s[end:], true
}

// hrMaskedID 判断是否为打码态身份证（供列表/导入日志统一口径）。
func hrMaskedID(id string) bool {
	return strings.Contains(normalizeIDNumber(id), "*")
}

// hrIDCardIndex69 明文身份证反查索引：姓名 + 前后缀 / 个人编号 / 唯一姓名。
type hrIDCardIndex69 struct {
	byName         map[string]map[string]bool // 姓名 → 完整身份证集合
	byPrefixSuffix map[string]map[string]bool // 姓名|前缀|后缀 → 完整身份证集合
	byPersonCode   map[string]string          // 个人编号/账号 → 完整身份证
}

func newHRIDCardIndex69() hrIDCardIndex69 {
	return hrIDCardIndex69{
		byName:         map[string]map[string]bool{},
		byPrefixSuffix: map[string]map[string]bool{},
		byPersonCode:   map[string]string{},
	}
}

func (idx *hrIDCardIndex69) add(name, idNumber, personCode string) {
	id := normalizeIDNumber(idNumber)
	name = strings.TrimSpace(name)
	if id == "" || hrMaskedID(id) {
		return
	}
	if name != "" {
		if idx.byName[name] == nil {
			idx.byName[name] = map[string]bool{}
		}
		idx.byName[name][id] = true
		if len(id) >= 10 {
			key := name + "|" + id[:6] + "|" + id[len(id)-4:]
			if idx.byPrefixSuffix[key] == nil {
				idx.byPrefixSuffix[key] = map[string]bool{}
			}
			idx.byPrefixSuffix[key][id] = true
		}
	}
	if pc := strings.TrimSpace(personCode); pc != "" {
		idx.byPersonCode[pc] = id
	}
}

// only 取值集合里唯一的一个；0 个或多个都返回 ""（绝不猜人）。
func only(m map[string]bool) string {
	if len(m) != 1 {
		return ""
	}
	for k := range m {
		return k
	}
	return ""
}

// restore 按「姓名 + 前后缀 → 个人编号 → 唯一姓名」顺序还原。
//
// 三条都是精确命中才返回：重名 / 前后缀冲突一律放弃，宁可留空也不写错身份证
// （写错身份证会把别人的社保、部门、劳务豁免全部串到这个人身上）。
func (idx hrIDCardIndex69) restore(masked, name, personCode string) string {
	prefix, suffix, ok := hrIDMaskPrefixSuffix(masked)
	name = strings.TrimSpace(name)
	if ok && name != "" {
		if id := only(idx.byPrefixSuffix[name+"|"+prefix+"|"+suffix]); id != "" {
			return id
		}
	}
	if pc := strings.TrimSpace(personCode); pc != "" {
		if id := idx.byPersonCode[pc]; id != "" {
			return id
		}
	}
	if name != "" {
		return only(idx.byName[name])
	}
	return ""
}

// hrBuildIDCardIndex69 汇总所有明文身份证来源（全部含离职人员）。
//
// 来源优先级不影响结果正确性（都是"精确命中才用"），但覆盖越全，能还原的越多：
//   - 花名册：员工主档，最权威；
//   - 社保回盘表：网厅全量参保人，含未建档人员；
//   - 公积金职工明细：含 18 位证件号与个人账号；
//   - 社保正常/补退记录：其它月份的明文记录（同一人跨月复用）。
func (h *HRHandler) hrBuildIDCardIndex69(ctx context.Context, tid int64) hrIDCardIndex69 {
	idx := newHRIDCardIndex69()

	var emps []HREmployee
	if err := h.db.WithContext(ctx).
		Select("name, id_number, employee_id, hire_date").
		Where("tenant_id = ?", tid).
		Order("hire_date DESC").Find(&emps).Error; err == nil {
		for _, e := range emps {
			idx.add(e.Name, e.IDNumber, "")
		}
	}

	var receipts []HRSIReceipt
	if err := h.db.WithContext(ctx).
		Select("name, id_number, person_code").
		Where("tenant_id = ?", tid).Find(&receipts).Error; err == nil {
		for _, r := range receipts {
			idx.add(r.Name, r.IDNumber, r.PersonCode)
		}
	}

	var pfs []HRPFRecord
	if err := h.db.WithContext(ctx).
		Select("name, identity_number, personal_account").
		Where("tenant_id = ?", tid).Find(&pfs).Error; err == nil {
		for _, r := range pfs {
			idx.add(r.Name, r.IdentityNumber, r.PersonalAccount)
		}
	}

	var records []HRSIRecord
	if err := h.db.WithContext(ctx).
		Select("name, id_number, person_code").
		Where("tenant_id = ?", tid).Find(&records).Error; err == nil {
		for _, r := range records {
			idx.add(r.Name, r.IDNumber, r.PersonCode)
		}
	}
	return idx
}

// hrDeptIndex69 全量花名册的部门索引（身份证优先，姓名兜底）。
//
// 与 hrSIDeptAuthority 的区别：这里排除物理删除（deleted_at 由 GORM 软删过滤），
// **不**看 status —— 离职员工的历史部门仍必须能关联上。
func (h *HRHandler) hrDeptIndex69(ctx context.Context, tid int64) hrSIDeptAuthority {
	out := hrSIDeptAuthority{byID: map[string]string{}, byName: map[string]string{}}
	var emps []HREmployee
	if err := h.db.WithContext(ctx).
		Select("name, id_number, department, hire_date").
		Where("tenant_id = ?", tid).
		// 同一身份证理论上只有一条（唯一索引），多租户历史脏数据时以最新入职为准。
		Order("hire_date DESC").Find(&emps).Error; err != nil {
		return out
	}
	nameHits := map[string]map[string]bool{}
	for _, e := range emps {
		d := strings.TrimSpace(e.Department)
		if d == "" {
			continue
		}
		if id := normalizeIDNumber(e.IDNumber); id != "" {
			if _, ok := out.byID[id]; !ok {
				out.byID[id] = d
			}
		}
		if n := strings.TrimSpace(e.Name); n != "" {
			if nameHits[n] == nil {
				nameHits[n] = map[string]bool{}
			}
			nameHits[n][d] = true
		}
	}
	// 同名不同部门时不猜（byName 留空），避免把 A 部门的人按姓名串到 B 部门。
	for n, depts := range nameHits {
		if len(depts) == 1 {
			out.byName[n] = only(depts)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 存量刷洗：星号身份证还原 + 部门回填
// ---------------------------------------------------------------------------

// hrRestoreMaskedIdCard69 还原全库打码身份证（幂等，只改含 `*` 的号）。
//
// 覆盖：社保正常/补退记录、回盘表、账单明细与快照、公积金职工明细与账单明细、
// 宿舍档案/扣款（custom_data 里的身份证）。
// 返回「还原条数 / 仍未匹配条数」。
func (h *HRHandler) hrRestoreMaskedIdCard69(ctx context.Context, tid int64) (int, int) {
	idx := h.hrBuildIDCardIndex69(ctx, tid)
	dept := h.hrDeptIndex69(ctx, tid)
	restored, unmatched := 0, 0

	// 1) 社保正常/补退记录
	var records []HRSIRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&records).Error
	for i := range records {
		r := &records[i]
		if !hrMaskedID(r.IDNumber) {
			continue
		}
		full := idx.restore(r.IDNumber, r.Name, r.PersonCode)
		if full == "" {
			unmatched++
			continue
		}
		r.IDNumber = full
		if r.CustomData == nil {
			r.CustomData = map[string]any{}
		}
		r.CustomData["身份证号"] = full
		if strings.TrimSpace(r.Department) == "" {
			if d := dept.deptOf(full, r.Name); d != "" {
				r.Department = d
				r.CustomData["部门"] = d
			}
		}
		/**
		 * 整行 Save：`custom_data` 是 JSONB（serializer:json），
		 * 用 `Update("custom_data", map)` 时 GORM 不会走 serializer，
		 * 驱动会直接报 `unsupported type map[string]interface{}`（SQLite/PG 都一样），
		 * 错误被 `_ =` 吞掉就成了「统计说还原了、库里还是星号」。
		 * 行本身是刚读出来的完整记录，Save 只回写这几列的值，无副作用。
		 */
		// Select + Updates：GORM 只对这些列走 serializer（JSONB 才能正确写入）。
		if err := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("id = ?", r.ID).
			Select("id_number", "department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮还原身份证失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			restored++
		}
	}

	// 2) 社保回盘表
	var receipts []HRSIReceipt
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&receipts).Error
	for i := range receipts {
		r := &receipts[i]
		if !hrMaskedID(r.IDNumber) {
			continue
		}
		full := idx.restore(r.IDNumber, r.Name, r.PersonCode)
		if full == "" {
			unmatched++
			continue
		}
		r.IDNumber = full
		if r.CustomData == nil {
			r.CustomData = map[string]any{}
		}
		r.CustomData["身份证号"] = full
		if strings.TrimSpace(r.Department) == "" {
			if d := dept.deptOf(full, r.Name); d != "" {
				r.Department = d
			}
		}
		// Select + Updates：GORM 只对这些列走 serializer（JSONB 才能正确写入）。
		if err := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("id = ?", r.ID).
			Select("id_number", "department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮还原身份证失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			restored++
		}
	}

	// 3) 社保账单明细（老账单才有行；新账单只读快照）
	var siItems []HRSIBillItem
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&siItems).Error
	for i := range siItems {
		it := &siItems[i]
		if !hrMaskedID(it.IDNumber) {
			continue
		}
		full := idx.restore(it.IDNumber, it.Name, it.PersonCode)
		if full == "" {
			unmatched++
			continue
		}
		it.IDNumber = full
		if strings.TrimSpace(it.Department) == "" {
			it.Department = dept.deptOf(full, it.Name)
		}
		if err := h.db.WithContext(ctx).Model(&HRSIBillItem{}).Where("id = ?", it.ID).
			Updates(map[string]any{"id_number": it.IDNumber, "department": it.Department}).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮账单明细还原失败（%s）: %v", it.ID, err)
		} else {
			restored++
		}
	}

	// 4) 社保账单快照（JSONB 内嵌明细）
	var bills []HRSIBill
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&bills).Error
	for i := range bills {
		b := &bills[i]
		changed := false
		if b.DetailSnapshot != nil {
			if items, ok := b.DetailSnapshot["items"].([]any); ok {
				for j := range items {
					m, ok := items[j].(map[string]any)
					if !ok {
						continue
					}
					idNum := toStringValue(m["证件号码"])
					if !hrMaskedID(idNum) {
						continue
					}
					full := idx.restore(idNum, toStringValue(m["姓名"]), toStringValue(m["个人编号"]))
					if full == "" {
						continue
					}
					m["证件号码"] = full
					if strings.TrimSpace(toStringValue(m["部门"])) == "" {
						if d := dept.deptOf(full, toStringValue(m["姓名"])); d != "" {
							m["部门"] = d
						}
					}
					items[j] = m
					changed = true
					restored++
				}
				b.DetailSnapshot["items"] = items
			}
		}
		if changed {
			// 同上：JSONB 必须走结构体 Save（serializer 才生效）
			if err := h.db.WithContext(ctx).Model(&HRSIBill{}).Where("id = ?", b.ID).
				Select("detail_snapshot").Updates(b).Error; err != nil {
				logger.Warnf(ctx, "第 69 轮账单快照还原失败（%s）: %v", b.ID, err)
			}
		}
	}

	// 5) 公积金职工明细 / 账单明细
	var pfs []HRPFRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&pfs).Error
	for i := range pfs {
		r := &pfs[i]
		if !hrMaskedID(r.IdentityNumber) {
			continue
		}
		full := idx.restore(r.IdentityNumber, r.Name, r.PersonalAccount)
		if full == "" {
			unmatched++
			continue
		}
		r.IdentityNumber = full
		if strings.TrimSpace(r.Department) == "" {
			r.Department = dept.deptOf(full, r.Name)
		}
		if err := h.db.WithContext(ctx).Model(&HRPFRecord{}).Where("id = ?", r.ID).
			Updates(map[string]any{"identity_number": r.IdentityNumber, "department": r.Department}).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮公积金明细还原失败（%s）: %v", r.ID, err)
		} else {
			restored++
		}
	}
	var pfItems []HRPFBillItem
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&pfItems).Error
	for i := range pfItems {
		it := &pfItems[i]
		if !hrMaskedID(it.IdentityNumber) {
			continue
		}
		full := idx.restore(it.IdentityNumber, it.Name, it.PersonalAccount)
		if full == "" {
			unmatched++
			continue
		}
		it.IdentityNumber = full
		if strings.TrimSpace(it.Department) == "" {
			it.Department = dept.deptOf(full, it.Name)
		}
		if err := h.db.WithContext(ctx).Model(&HRPFBillItem{}).Where("id = ?", it.ID).
			Updates(map[string]any{"identity_number": it.IdentityNumber, "department": it.Department}).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮公积金账单明细还原失败（%s）: %v", it.ID, err)
		} else {
			restored++
		}
	}

	// 6) 宿舍档案 / 扣款（身份证存在 custom_data 里）
	var dormRows []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&dormRows).Error
	for i := range dormRows {
		r := &dormRows[i]
		if r.CustomData == nil {
			continue
		}
		raw := hrDormStr(r.CustomData, "身份证号")
		if !hrMaskedID(raw) {
			continue
		}
		full := idx.restore(raw, hrDormStr(r.CustomData, "姓名"), "")
		if full == "" {
			continue
		}
		r.CustomData["身份证号"] = full
		// Select + Updates：GORM 只对这些列走 serializer（JSONB 才能正确写入）。
		if err := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("id = ?", r.ID).
			Select("id_number", "department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮还原身份证失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			restored++
		}
	}
	return restored, unmatched
}

// hrBackfillResignedDept69 全量花名册（含离职）回填空部门（幂等，只补空值）。
//
// 覆盖社保记录 / 回盘表 / 账单明细 / 账单快照 / 公积金明细 / 宿舍档案。
// 返回补全条数。
func (h *HRHandler) hrBackfillResignedDept69(ctx context.Context, tid int64) int {
	dept := h.hrDeptIndex69(ctx, tid)
	if len(dept.byID) == 0 && len(dept.byName) == 0 {
		return 0
	}
	filled := 0

	// 必须读完整行：命中后用 Save 回写（含 JSONB custom_data），
	// 只 Select 部分列再 Save 会把未读出的列清成零值。
	var records []HRSIRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&records).Error
	for i := range records {
		r := &records[i]
		if strings.TrimSpace(r.Department) != "" {
			continue
		}
		d := dept.deptOf(r.IDNumber, r.Name)
		if d == "" {
			continue
		}
		if r.CustomData == nil {
			r.CustomData = map[string]any{}
		}
		r.Department = d
		r.CustomData["部门"] = d
		// Model(r) 用主键定位：显式 Where + 结构体 Updates 在部分驱动下会丢条件。
		if err := h.db.WithContext(ctx).Model(r).
			Select("department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮回填部门失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			filled++
		}
	}

	var receipts []HRSIReceipt
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&receipts).Error
	for i := range receipts {
		r := &receipts[i]
		if strings.TrimSpace(r.Department) != "" {
			continue
		}
		d := dept.deptOf(r.IDNumber, r.Name)
		if d == "" {
			continue
		}
		if r.CustomData == nil {
			r.CustomData = map[string]any{}
		}
		r.Department = d
		r.CustomData["部门"] = d
		// 回盘表要写回**自己的表**（HRSIReceipt），不能复用 HRSIRecord 的 Model
		if err := h.db.WithContext(ctx).Model(r).
			Select("department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮回盘表回填部门失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			filled++
		}
	}

	var siItems []HRSIBillItem
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&siItems).Error
	for i := range siItems {
		it := &siItems[i]
		if strings.TrimSpace(it.Department) != "" {
			continue
		}
		d := dept.deptOf(it.IDNumber, it.Name)
		if d == "" {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&HRSIBillItem{}).Where("id = ?", it.ID).
			Update("department", d).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮账单明细部门回填失败（%s）: %v", it.ID, err)
		} else {
			filled++
		}
	}

	// 账单快照里的部门（第 68 轮后明细只读快照，光补记录不会反映到弹窗）
	var bills []HRSIBill
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&bills).Error
	for i := range bills {
		b := &bills[i]
		if b.DetailSnapshot == nil {
			continue
		}
		items, ok := b.DetailSnapshot["items"].([]any)
		if !ok || len(items) == 0 {
			continue
		}
		changed := false
		for j := range items {
			m, ok := items[j].(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(toStringValue(m["部门"])) != "" {
				continue
			}
			d := dept.deptOf(toStringValue(m["证件号码"]), toStringValue(m["姓名"]))
			if d == "" {
				continue
			}
			m["部门"] = d
			items[j] = m
			changed = true
			filled++
		}
		if !changed {
			continue
		}
		b.DetailSnapshot["items"] = items
		if err := h.db.WithContext(ctx).Model(&HRSIBill{}).Where("id = ?", b.ID).
			Select("detail_snapshot").Updates(b).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮账单快照部门回填失败（%s）: %v", b.ID, err)
		}
	}

	var pfs []HRPFRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&pfs).Error
	for i := range pfs {
		r := &pfs[i]
		if strings.TrimSpace(r.Department) != "" {
			continue
		}
		d := dept.deptOf(r.IdentityNumber, r.Name)
		if d == "" {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&HRPFRecord{}).Where("id = ?", r.ID).
			Update("department", d).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮公积金部门回填失败（%s）: %v", r.ID, err)
		} else {
			filled++
		}
	}

	var pfItems []HRPFBillItem
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&pfItems).Error
	for i := range pfItems {
		it := &pfItems[i]
		if strings.TrimSpace(it.Department) != "" {
			continue
		}
		d := dept.deptOf(it.IdentityNumber, it.Name)
		if d == "" {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&HRPFBillItem{}).Where("id = ?", it.ID).
			Update("department", d).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮公积金账单明细部门回填失败（%s）: %v", it.ID, err)
		} else {
			filled++
		}
	}

	var dormRows []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&dormRows).Error
	for i := range dormRows {
		r := &dormRows[i]
		if r.CustomData == nil {
			continue
		}
		if strings.TrimSpace(hrDormStr(r.CustomData, "部门")) != "" {
			continue
		}
		d := dept.deptOf(hrDormStr(r.CustomData, "身份证号"), hrDormStr(r.CustomData, "姓名"))
		if d == "" {
			continue
		}
		r.CustomData["部门"] = d
		if err := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("id = ?", r.ID).
			Select("department", "custom_data").Updates(r).Error; err != nil {
			logger.Warnf(ctx, "第 69 轮回填部门失败（%s/%s）: %v", r.ID, r.Name, err)
		} else {
			filled++
		}
	}
	return filled
}

// ---------------------------------------------------------------------------
// 导入路径：解析完成后、落库之前就地还原
// ---------------------------------------------------------------------------

// hrRestoreImportIDs69 还原本次导入里的星号身份证，并用全量花名册补部门。
//
// 必须在 `hrSIApplyImport66` 之前调用：合并键是 `(身份证, 险种, 月份, 类型)`，
// 带着星号进去会与上月明文行对不上 —— 同一个人在两套键上各存一份，
// 之后按身份证关联部门/劳务豁免全部失效（这正是 2026-09 的真机现象）。
//
// 返回（还原条数, 仍未匹配条数）。
func (h *HRHandler) hrRestoreImportIDs69(ctx context.Context, tid int64, items []HRSIRecord) (int, int) {
	hasMasked := false
	for i := range items {
		if hrMaskedID(items[i].IDNumber) {
			hasMasked = true
			break
		}
	}
	if !hasMasked {
		// 没有星号也要补部门：源文件常常只有姓名，部门列是空的。
		dept := h.hrDeptIndex69(ctx, tid)
		for i := range items {
			h.fillImportDept69(&items[i], dept)
		}
		return 0, 0
	}
	idx := h.hrBuildIDCardIndex69(ctx, tid)
	dept := h.hrDeptIndex69(ctx, tid)
	restored, unmatched := 0, 0
	for i := range items {
		it := &items[i]
		if hrMaskedID(it.IDNumber) {
			full := idx.restore(it.IDNumber, it.Name, it.PersonCode)
			if full == "" {
				unmatched++
			} else {
				it.IDNumber = full
				if it.CustomData == nil {
					it.CustomData = map[string]any{}
				}
				it.CustomData["身份证号"] = full
				restored++
			}
		}
		h.fillImportDept69(it, dept)
	}
	return restored, unmatched
}

// fillImportDept69 空部门按全量花名册补（已有值不动，绝不覆盖源文件里的部门）。
func (h *HRHandler) fillImportDept69(it *HRSIRecord, dept hrSIDeptAuthority) {
	if strings.TrimSpace(it.Department) != "" {
		if it.CustomData == nil {
			it.CustomData = map[string]any{}
		}
		it.CustomData["部门"] = it.Department
		return
	}
	d := dept.deptOf(it.IDNumber, it.Name)
	if d == "" {
		return
	}
	it.Department = d
	if it.CustomData == nil {
		it.CustomData = map[string]any{}
	}
	it.CustomData["部门"] = d
}

// hrEnsureSocialRecordType69 社保「记录类型」字段自愈（幂等，只补缺）。
//
// 为什么需要：它是第 56 轮四胶囊契约里的字段（正常记录/补退记录各一份），
// 但用户在设置面板里误删后没有任何自愈路径 —— 导入落库与列表都靠它区分
// 「正常 / 补退」，缺了之后该列从表头、筛选、模板一起消失（六同步整体缺一列）。
// 只补缺不覆盖：已有该字段（用户改过 label/宽度/顺序）一律不动。
func (h *HRHandler) hrEnsureSocialRecordType69(ctx context.Context) {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeSocial).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "读取社保字段配置失败: %v", err)
		return
	}
	for i := range cats {
		changed := false
		for _, group := range []string{"正常记录", "补退记录"} {
			has := false
			for _, sub := range cats[i].Subs {
				if strings.TrimSpace(sub.Group) == group && strings.TrimSpace(sub.Name) == "记录类型" {
					has = true
					break
				}
			}
			if has {
				continue
			}
			cats[i].Subs = append(cats[i].Subs, types.FleetCategorySub{
				Name: "记录类型", Group: group, GroupLabel: group,
				Enabled: true, IsDefault: false, DataType: "text", Width: 0,
			})
			changed = true
		}
		if !changed {
			continue
		}
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			logger.Warnf(ctx, "补齐社保「记录类型」字段失败（tenant=%d）: %v", cats[i].TenantID, err)
			continue
		}
		logger.Infof(ctx, "已补齐社保「记录类型」字段（tenant=%d）", cats[i].TenantID)
	}
}

// hrRepair69 启动幂等修复：先把星号身份证还原成明文，再回填空部门。
//
// 顺序不能反：部门回填依赖完整的 18 位身份证做精确关联，
// 先还原再回填，一次启动就能把「打码 + 空部门」两条问题一起收敛。
func (h *HRHandler) hrRepair69(ctx context.Context) {
	// 字段自愈先跑：后面按 subs 派生的列表/模板都依赖它存在
	h.hrEnsureSocialRecordType69(ctx)
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 69 轮修复：读取租户失败: %v", err)
		return
	}
	// 社保/公积金里可能有未建档员工（花名册无档案），这些租户也要刷一遍。
	var extra []int64
	_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).Distinct().Pluck("tenant_id", &extra).Error
	tids = append(tids, extra...)
	sort.Slice(tids, func(i, j int) bool { return tids[i] < tids[j] })
	seen := map[int64]bool{}
	for _, tid := range tids {
		if tid == 0 || seen[tid] {
			continue
		}
		seen[tid] = true
		restored, unmatched := h.hrRestoreMaskedIdCard69(ctx, tid)
		filled := h.hrBackfillResignedDept69(ctx, tid)
		if restored > 0 || filled > 0 || unmatched > 0 {
			logger.Infof(ctx, "第 69 轮数据修复（tenant=%d）：星号身份证还原 %d 条，未匹配 %d 条，部门回填 %d 条",
				tid, restored, unmatched, filled)
		}
	}
}
