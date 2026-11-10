// 组织机构改名 → 存量员工记录级联更新（第 50 轮）。
//
// SaveOrgUnits 是「删表重建」的整表替换语义，重建后旧名字就查不到了；
// 因此必须在事务内**先**把旧单位/部门读出来，重建后按 id（或 code）比对，
// 把改名映射落回 hr_employees。
//
// 三个约束：
//  1. 只在同一 id / code 的新旧名称不同时才算改名，新增/删除不触发级联；
//  2. 值没变时不做任何 UPDATE（幂等，重复保存不产生写放大）；
//  3. JSONB 更新用 jsonb_set（PostgreSQL）；其它方言只改主列，避免测试库报错。
package handler

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

// hrRenamePair 一组「旧名 → 新名」映射。
type hrRenamePair struct {
	Old string
	New string
}

// hrDeptRenamePairs 比对旧/新部门列表，产出改名映射。
//
// 匹配优先级：先 id，再 code。两者都缺（新加的行）不参与比对。
func hrDeptRenamePairs(oldRows, newRows []HRDepartment) []hrRenamePair {
	byID := make(map[string]string, len(oldRows))
	byCode := make(map[string]string, len(oldRows))
	for _, d := range oldRows {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		if id := strings.TrimSpace(d.ID); id != "" {
			byID[id] = name
		}
		if code := strings.TrimSpace(d.Code); code != "" {
			byCode[code] = name
		}
	}
	out := make([]hrRenamePair, 0, 4)
	seen := map[string]bool{}
	for _, d := range newRows {
		newName := strings.TrimSpace(d.Name)
		if newName == "" {
			continue
		}
		oldName := ""
		if id := strings.TrimSpace(d.ID); id != "" {
			oldName = byID[id]
		}
		if oldName == "" {
			if code := strings.TrimSpace(d.Code); code != "" {
				oldName = byCode[code]
			}
		}
		if oldName == "" || oldName == newName {
			continue
		}
		key := oldName + "\x00" + newName
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hrRenamePair{Old: oldName, New: newName})
	}
	return out
}

// hrUnitRenamePairs 比对旧/新单位列表，产出改名映射。
func hrUnitRenamePairs(oldRows, newRows []HROrgUnit) []hrRenamePair {
	byID := make(map[string]string, len(oldRows))
	byCode := make(map[string]string, len(oldRows))
	for _, u := range oldRows {
		name := strings.TrimSpace(u.Name)
		if name == "" {
			continue
		}
		if id := strings.TrimSpace(u.ID); id != "" {
			byID[id] = name
		}
		if code := strings.TrimSpace(u.Code); code != "" {
			byCode[code] = name
		}
	}
	out := make([]hrRenamePair, 0, 2)
	seen := map[string]bool{}
	for _, u := range newRows {
		newName := strings.TrimSpace(u.Name)
		if newName == "" {
			continue
		}
		oldName := ""
		if id := strings.TrimSpace(u.ID); id != "" {
			oldName = byID[id]
		}
		if oldName == "" {
			if code := strings.TrimSpace(u.Code); code != "" {
				oldName = byCode[code]
			}
		}
		if oldName == "" || oldName == newName {
			continue
		}
		key := oldName + "\x00" + newName
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hrRenamePair{Old: oldName, New: newName})
	}
	return out
}

// hrIsPostgres 判断当前连接是不是 PostgreSQL（JSONB 语法只在它上面可用）。
func hrIsPostgres(tx *gorm.DB) bool {
	return tx != nil && tx.Dialector != nil && tx.Dialector.Name() == "postgres"
}

// hrCascadeDepartmentRename 把一个旧部门名下的员工全部改到新部门名。
//
// 主列 department 与 custom_data 里的「部门」键同时更新，
// 否则列表按主列过滤、抽屉按 custom_data 展示会出现两套值。
func hrCascadeDepartmentRename(tx *gorm.DB, tid int64, oldName, newName string) (int64, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return 0, nil
	}
	q := tx.Model(&HREmployee{}).
		Where("tenant_id = ? AND btrim(department) = ?", tid, oldName)
	if hrIsPostgres(tx) {
		res := q.Updates(map[string]any{
			"department": newName,
			"custom_data": gorm.Expr(
				`jsonb_set(COALESCE(custom_data, '{}'::jsonb), '{部门}', to_jsonb(?::text), true)`,
				newName,
			),
		})
		return res.RowsAffected, res.Error
	}
	res := q.Update("department", newName)
	return res.RowsAffected, res.Error
}

// hrCascadeUnitRename 把一个旧单位名下的员工合同甲方改到新单位名。
//
// 花名册里单位名有两个键（现合同签约公司 / 合同公司），两个都要改，
// 否则劳动合同列表会按旧键匹配不到组织机构里的新单位。
func hrCascadeUnitRename(tx *gorm.DB, tid int64, oldName, newName string) (int64, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return 0, nil
	}
	var affected int64
	for _, key := range []string{"现合同签约公司", "合同公司"} {
		q := tx.Model(&HREmployee{}).
			Where("tenant_id = ? AND btrim(custom_data->>?) = ?", tid, key, oldName)
		if hrIsPostgres(tx) {
			res := q.Update("custom_data", gorm.Expr(
				`jsonb_set(COALESCE(custom_data, '{}'::jsonb), ARRAY[?]::text[], to_jsonb(?::text), true)`,
				key, newName,
			))
			if res.Error != nil {
				return affected, res.Error
			}
			affected += res.RowsAffected
			continue
		}
		// 非 PG 方言没有 JSONB：退回全表读改写（仅测试库会走到）
		var rows []HREmployee
		if err := tx.Where("tenant_id = ?", tid).Find(&rows).Error; err != nil {
			return affected, err
		}
		for i := range rows {
			if rows[i].CustomData == nil {
				continue
			}
			if strings.TrimSpace(toStringValue(rows[i].CustomData[key])) != oldName {
				continue
			}
			rows[i].CustomData[key] = newName
			if err := tx.Model(&HREmployee{}).Where("id = ?", rows[i].ID).
				Update("custom_data", rows[i].CustomData).Error; err != nil {
				return affected, err
			}
			affected++
		}
	}
	return affected, nil
}

// hrNormalizeEmployeeDepartments 把不在组织机构里的存量部门名归并到最接近的正式名。
//
// 典型场景：组织机构是「技术质量部」，存量里有一批员工写着「质量部」，
// 两边对不上 → 工具栏部门下拉会多出一个孤立选项，按部门筛也筛不全。
//
// 匹配规则：精确命中直接用；否则取「互相包含」且名字最长的那个组织部门
// （`质量部` ⊂ `技术质量部` → 技术质量部）。找不到唯一匹配就不动，
// 宁可留脏数据也不猜。
func hrNormalizeEmployeeDepartments(tx *gorm.DB, tid int64, deptNames []string) (int, error) {
	official := make([]string, 0, len(deptNames))
	seen := map[string]bool{}
	for _, d := range deptNames {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		official = append(official, d)
	}
	if len(official) == 0 {
		return 0, nil
	}
	var stored []string
	if err := tx.Model(&HREmployee{}).
		Where("tenant_id = ? AND btrim(department) <> ''", tid).
		Distinct().Pluck("btrim(department)", &stored).Error; err != nil {
		return 0, err
	}
	fixed := 0
	for _, cur := range stored {
		cur = strings.TrimSpace(cur)
		if cur == "" || seen[cur] {
			continue
		}
		best := hrBestDeptMatch(cur, official)
		if best == "" {
			continue
		}
		n, err := hrCascadeDepartmentRename(tx, tid, cur, best)
		if err != nil {
			return fixed, err
		}
		fixed += int(n)
	}
	return fixed, nil
}

// hrBestDeptMatch 给一个非官方部门名找最接近的官方名（找不到返回空串）。
func hrBestDeptMatch(name string, official []string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	best := ""
	for _, o := range official {
		if o == name {
			return ""
		}
		// 只接受互相包含：质量部 ↔ 技术质量部
		if !strings.Contains(o, name) && !strings.Contains(name, o) {
			continue
		}
		// 取最长（最具体）的那个，避免「部」这种泛匹配把人都并到一起
		if len(o) > len(best) {
			best = o
		}
	}
	return best
}

// hrRunOrgCascade 事务内执行改名级联（供 SaveOrgUnits 调用）。
func hrRunOrgCascade(tx *gorm.DB, tid int64,
	oldUnits, newUnits []HROrgUnit,
	oldDepts, newDepts []HRDepartment,
) (int64, error) {
	var affected int64
	for _, p := range hrUnitRenamePairs(oldUnits, newUnits) {
		n, err := hrCascadeUnitRename(tx, tid, p.Old, p.New)
		if err != nil {
			return affected, err
		}
		affected += n
	}
	for _, p := range hrDeptRenamePairs(oldDepts, newDepts) {
		n, err := hrCascadeDepartmentRename(tx, tid, p.Old, p.New)
		if err != nil {
			return affected, err
		}
		affected += n
	}
	return affected, nil
}

// hrRefreshOrgDepartments 保存后把存量部门名归并到最新组织机构（幂等）。
//
// 每次组织机构保存后跑一次：用户新增/改名部门后，存量员工里那些
// 「沾边但不完全一致」的旧名会被并过来，保证工具栏下拉与组织机构 1:1。
func (h *HRHandler) hrRefreshOrgDepartments(ctx context.Context, tid int64) (int, error) {
	units := h.hrOrgUnitsFor(ctx, tid)
	names := make([]string, 0, len(units)*4)
	for _, u := range units {
		for _, d := range u.Departments {
			names = append(names, d.Name)
		}
	}
	var fixed int
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := hrNormalizeEmployeeDepartments(tx, tid, names)
		fixed = n
		return err
	})
	return fixed, err
}
