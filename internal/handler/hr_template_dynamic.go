// 第 58 轮：导入模板表头改为「按当前已保存的字段配置动态生成」。
//
// 用户在第 58 轮之后可以在「设置 → 字段设置」里新增 / 改名 / 停用字段，模板与
// 导入解析必须跟着变，否则新增一列就导入不进来。这里提供三件东西：
//
//   - hrTemplateFieldsFor：读 fleet_categories 的 subs，产出 (契约字段名, 表头文案) 清单；
//   - hrTemplateHeadersFor：上面清单的表头投影（找不到分类时回退静态 hrTemplateHeaders）；
//   - hrTemplateScopeGroup：模板 module → (分类 scope, 胶囊 group) 的映射。
//
// 铁律：静态 hrTemplateHeaders(module) 保持原样（既有单测与老租户兜底都依赖它）。
package handler

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// hrTemplateField 动态模板字段：Name = 契约字段名（custom_data 键），Header = 表头文案。
type hrTemplateField struct {
	Name   string
	Header string
}

// hrFundGroupEmployees 公积金「职工明细」胶囊分组名（与 hrFundFieldDefs 一致）。
const hrFundGroupEmployees = "职工明细"

// hrTemplateScopeGroup 模板 module → (分类 scope, 字段胶囊 group)。
//
// 离职管理没有独立分类：离职字段挂在花名册分类的「离职管理」胶囊下（第 57 轮）。
func hrTemplateScopeGroup(module string) (string, string) {
	switch module {
	case hrModuleResign:
		return hrScopeRoster, hrRosterGroupResigned
	case hrModuleContract:
		return hrScopeContract, hrContractGroupLabor
	case hrModuleFund:
		return hrScopeFund, hrFundGroupEmployees
	case hrModuleSocial:
		return hrScopeSocial, hrSocReceiptGroup
	default:
		return hrScopeRoster, hrRosterGroupActive
	}
}

// hrTemplateModuleOfScope scope+group → 静态模板 module（动态表头的兜底来源）。
func hrTemplateModuleOfScope(scope, group string) string {
	switch scope {
	case hrScopeRoster:
		if strings.TrimSpace(group) == hrRosterGroupResigned {
			return hrModuleResign
		}
		return hrModuleRoster
	case hrScopeContract:
		return hrModuleContract
	case hrScopeFund:
		return hrModuleFund
	case hrScopeSocial:
		return hrModuleSocial
	default:
		return hrModuleRoster
	}
}

// hrTemplateFieldsFor 按已保存的 subs 生成可导入字段清单。
//
// 过滤口径（与「字段设置」可见性一致）：
//   - enabled != false（用户停用的字段不进模板）；
//   - group 匹配（group 为空则不过滤）；
//   - 跨表引用字段（source 非空）不进模板：否则一份表就能把源表数据改坏；
//   - data_type = system / readonly 不可编辑，同样不进模板。
//
// 顺序严格沿用 subs 顺序，表头取 label 优先、name 兜底。
// 第二个返回值 false 表示「没读到分类 / 没有任何可用字段」，调用方回退静态表头。
func hrTemplateFieldsFor(ctx context.Context, db *gorm.DB, tid int64, scope, group string) ([]hrTemplateField, bool) {
	if db == nil {
		return nil, false
	}
	var cats []types.FleetCategory
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND deleted_at IS NULL", tid, scope).
		Order("sort_order ASC, created_at ASC").
		Find(&cats).Error; err != nil || len(cats) == 0 {
		return nil, false
	}
	// 同一 scope 可能有多个分类（用户自建），优先挑包含目标胶囊的那个。
	pick := cats[0]
	if g := strings.TrimSpace(group); g != "" {
	find:
		for _, c := range cats {
			for _, s := range c.Subs {
				if strings.TrimSpace(s.Group) == g {
					pick = c
					break find
				}
			}
		}
	}
	out := make([]hrTemplateField, 0, len(pick.Subs))
	seen := make(map[string]bool, len(pick.Subs))
	for _, s := range pick.Subs {
		if !s.Enabled || s.Source != nil {
			continue
		}
		if s.DataType == "system" || s.DataType == "readonly" {
			continue
		}
		if g := strings.TrimSpace(group); g != "" && strings.TrimSpace(s.Group) != g {
			continue
		}
		name := strings.TrimSpace(s.Name)
		header := strings.TrimSpace(s.Label)
		if header == "" {
			header = name
		}
		if name == "" || header == "" || seen[header] {
			continue
		}
		seen[header] = true
		out = append(out, hrTemplateField{Name: name, Header: header})
	}
	if len(out) == 0 {
		return nil, false
	}
	// 离职 / 合同模板是「按人更新台账」，三个匹配键（工号 / 姓名 / 证件号码）缺一列
	// 整份表都导不进去。这些键在字段契约里常是跨表引用（被上面的过滤剔掉），
	// 因此在这里强制补到最前面；静态模板同样是这三个键打头。
	if module := hrTemplateModuleOfScope(scope, group); module == hrModuleResign || module == hrModuleContract {
		for i := len(hrTemplateMatchKeys) - 1; i >= 0; i-- {
			key := hrTemplateMatchKeys[i]
			if seen[key] {
				continue
			}
			out = append([]hrTemplateField{{Name: key, Header: key}}, out...)
			seen[key] = true
		}
	}
	return out, true
}

// hrTemplateHeadersFor 动态模板表头；找不到分类时回退静态 hrTemplateHeaders(module)。
func hrTemplateHeadersFor(ctx context.Context, db *gorm.DB, tid int64, scope, group string) []string {
	fields, ok := hrTemplateFieldsFor(ctx, db, tid, scope, group)
	if !ok {
		return hrTemplateHeaders(hrTemplateModuleOfScope(scope, group))
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Header)
	}
	return out
}

// hrTemplateHeadersForModule 按模板 module 取动态表头（DownloadRosterTemplate 用）。
func (h *HRHandler) hrTemplateHeadersForModule(ctx context.Context, tid int64, module string) []string {
	scope, group := hrTemplateScopeGroup(module)
	return hrTemplateHeadersFor(ctx, h.db, tid, scope, group)
}
