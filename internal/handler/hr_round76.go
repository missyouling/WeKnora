package handler

// 第 76 轮：租户档案迁入公司设置 + 车队保险单字段重配。
//
// 两条主线：
//  1. 公司设置（hr_org_units）扩展为「单位档案 + 租户档案」二合一，别名成为
//     全局简称唯一真相源；分摊子项仍存在 billing_tenant_items（按
//     billing_tenant_id 关联），迁移只搬档案、不搬金额，保证费用清单零波动。
//  2. 车队「保险单」按交强险 / 商业险 / 险种类别重配字段（3 组 × 30 + 3 组 × 24
//     + 7 项险种）并补齐提取规则与高级 Prompt；存量保单数据按旧字段别名映射
//     补齐新字段，旧键全部保留，绝不删数。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ---------------------------------------------------------------------------
// 一、公司设置：租户档案同步与迁移
// ---------------------------------------------------------------------------

// hrSyncOrgTenants76 把「单位设置」里的租户档案写回 billing_tenants。
//
// 为什么保留 billing_tenants 而不直接读 hr_org_units：
//   - billing_records / billing_tenant_items 都以 billing_tenant_id 为外键，
//     换表主键会让历史费用清单全部失去归属（金额波动的最大风险源）；
//   - 因此公司设置是**档案真相源**，billing_tenants 是**核算标识 + 分摊子项容器**，
//     两者靠 billing_tenant_id 1:1 绑定，保存时单向同步档案字段。
func (h *HRHandler) hrSyncOrgTenants76(ctx context.Context, tid int64) error {
	var units []HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&units).Error; err != nil {
		return err
	}
	var tenants []types.BillingTenant
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&tenants).Error; err != nil {
		return err
	}
	for i := range units {
		u := &units[i]
		if !u.IsTenant {
			continue
		}
		name := strings.TrimSpace(u.Alias)
		if name == "" {
			name = strings.TrimSpace(u.Name)
		}
		if name == "" {
			continue
		}
		var hit *types.BillingTenant
		for j := range tenants {
			if u.BillingTenantID != "" && tenants[j].ID == u.BillingTenantID {
				hit = &tenants[j]
				break
			}
		}
		if hit == nil {
			for j := range tenants {
				if unitNamesMatch(tenants[j].Name, name) || unitNamesMatch(tenants[j].Name, u.Name) {
					hit = &tenants[j]
					break
				}
			}
		}
		if hit == nil {
			created := types.BillingTenant{
				ID:             newID76(),
				TenantID:       tid,
				Name:           name,
				TenantNo:       strings.TrimSpace(u.TenantNo),
				AllocationMode: orDefault76(u.AllocationMode, "按比例分摊"),
				Contact:        strings.TrimSpace(u.Contact),
				Phone:          strings.TrimSpace(u.Phone),
				Remark:         strings.TrimSpace(u.Remark),
			}
			if err := h.db.WithContext(ctx).Create(&created).Error; err != nil {
				return err
			}
			tenants = append(tenants, created)
			hit = &tenants[len(tenants)-1]
		} else {
			updates := map[string]interface{}{
				"name":            name,
				"tenant_no":       strings.TrimSpace(u.TenantNo),
				"allocation_mode": orDefault76(u.AllocationMode, "按比例分摊"),
				"contact":         strings.TrimSpace(u.Contact),
				"phone":           strings.TrimSpace(u.Phone),
				"remark":          strings.TrimSpace(u.Remark),
			}
			if err := h.db.WithContext(ctx).Model(&types.BillingTenant{}).
				Where("id = ?", hit.ID).Updates(updates).Error; err != nil {
				return err
			}
			hit.Name = name
		}
		if u.BillingTenantID != hit.ID {
			if err := h.db.WithContext(ctx).Model(&HROrgUnit{}).
				Where("id = ?", u.ID).Update("billing_tenant_id", hit.ID).Error; err != nil {
				return err
			}
			u.BillingTenantID = hit.ID
		}
	}
	return nil
}

// hrMigrateTenantToOrg76 幂等迁移：原「费用清单 → 租户信息」→ 公司设置。
//
// 数据安全红线：只写档案字段（别名/编码/分摊方式/联系人/电话）与关联键，
// 绝不触碰 billing_tenant_items、billing_records 的任何金额列。
func (h *HRHandler) hrMigrateTenantToOrg76(ctx context.Context, tid int64) (unitsUpdated, tenantsLinked int) {
	var tenants []types.BillingTenant
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&tenants).Error; err != nil {
		return
	}
	var units []HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Order("sort_order ASC").Find(&units).Error; err != nil {
		return
	}
	findUnit := func(name string) *HROrgUnit {
		for i := range units {
			u := &units[i]
			if unitNamesMatch(u.Name, name) || (u.Alias != "" && unitNamesMatch(u.Alias, name)) {
				return u
			}
		}
		return nil
	}
	for _, t := range tenants {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		u := findUnit(name)
		if u == nil {
			// 租户在单位设置里没有对应公司：补一个单位，避免档案搬不过去。
			created := HROrgUnit{
				ID:       newID76(),
				TenantID: tid,
				Name:     name,
				Alias:    name,
				Code:     fmt.Sprintf("%02d", len(units)+1),
				IsTenant: true,
				SortOrder: len(units) + 1,
			}
			if err := h.db.WithContext(ctx).Create(&created).Error; err != nil {
				continue
			}
			units = append(units, created)
			u = &units[len(units)-1]
		}
		updates := map[string]interface{}{
			"is_tenant":         true,
			"billing_tenant_id": t.ID,
		}
		if strings.TrimSpace(u.Alias) == "" {
			updates["alias"] = name
		}
		if strings.TrimSpace(u.TenantNo) == "" && strings.TrimSpace(t.TenantNo) != "" {
			updates["tenant_no"] = strings.TrimSpace(t.TenantNo)
		}
		if strings.TrimSpace(u.AllocationMode) == "" {
			updates["allocation_mode"] = orDefault76(t.AllocationMode, "按比例分摊")
		}
		if strings.TrimSpace(u.Contact) == "" && strings.TrimSpace(t.Contact) != "" {
			updates["contact"] = strings.TrimSpace(t.Contact)
		}
		if strings.TrimSpace(u.Phone) == "" && strings.TrimSpace(t.Phone) != "" {
			updates["phone"] = strings.TrimSpace(t.Phone)
		}
		if err := h.db.WithContext(ctx).Model(&HROrgUnit{}).
			Where("id = ?", u.ID).Updates(updates).Error; err == nil {
			unitsUpdated++
			tenantsLinked++
		}
	}
	// 非租户单位补齐默认别名（重庆星达铜业 → 重庆星达），别名是展示口径，
	// 必须在首次迁移就位，否则核算列表的单位切换标签会显示工商全称。
	for i := range units {
		u := &units[i]
		if strings.TrimSpace(u.Alias) != "" || u.IsTenant {
			continue
		}
		alias := ""
		if unitNamesMatch(u.Name, "重庆星达铜业有限公司") {
			alias = "重庆星达"
		}
		if alias == "" {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&HROrgUnit{}).
			Where("id = ?", u.ID).Update("alias", alias).Error; err == nil {
			unitsUpdated++
		}
	}
	return
}

func orDefault76(v, def string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return def
}

func newID76() string {
	return uuid.New().String()
}

// ---------------------------------------------------------------------------
// 二、车队保险单：交强险 / 商业险 / 险种类别
// ---------------------------------------------------------------------------

// 保险单字段所属标签（写入 subs.group，前端按前缀拆成 3 个配置标签）。
const (
	hrInsTabJq76  = "交强险"
	hrInsTabSy76  = "商业险"
	hrInsTabXz76  = "险种类别"
	hrInsGroupSep = "/"
)

type hrInsField76 struct {
	name    string
	label   string
	dtype   string
	desc    string
	rule    string
	regex   string
	filter  bool
	visible bool // 列表默认列
}

// 13 个基本信息字段（交强险 / 商业险同构；商业险用 商业险_ 前缀保证 subs 内
// 字段身份唯一 —— 列表列 / 编辑抽屉都按 name 取用，同名会串列）。
var hrInsBase76 = []hrInsField76{
	{"保险单号", "保险单号", "text", "保险单编号", "提取「保险单号 / 保单号码」后的编号", `(?:保险单号|保单号|保单号码)\s*[:：]?\s*([0-9A-Za-z\-]{6,40})`, true, true},
	{"被保险人", "被保险人", "text", "被保险人名称", "提取「被保险人 / 投保人」栏", "", true, true},
	{"信用代码", "信用代码", "text", "被保险人统一社会信用代码", "提取 18 位统一社会信用代码", `[0-9A-HJ-NPQRTUWXY]{18}`, false, true},
	{"地址", "地址", "text", "被保险人地址", "提取「地址 / 住所」栏", "", false, true},
	{"联系人", "联系人", "text", "被保险人联系人", "提取「联系人」栏", "", false, false},
	{"联系电话", "联系电话", "text", "联系电话", "提取「联系电话 / 电话」栏", "", false, false},
	{"保费合计", "保费合计", "amount", "保费合计（元）", "提取「保费合计 / 保险费合计」，只保留数字", `(?:保费合计|保险费合计|保费总计)\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"保险起始日期", "保险起始日期", "date", "保险起期", "提取「保险期间自 / 起保日期」，统一 YYYY-MM-DD", `(?:保险期间自|起保日期|保险起期)\s*[:：]?\s*([0-9]{4}[-年][0-9]{1,2}[-月][0-9]{1,2})`, false, true},
	{"保险截止日期", "保险截止日期", "date", "保险止期", "提取「保险期间至 / 终保日期」，统一 YYYY-MM-DD", `(?:保险期间至|终保日期|保险止期)\s*[:：]?\s*([0-9]{4}[-年][0-9]{1,2}[-月][0-9]{1,2})`, false, true},
	{"车船税", "车船税", "amount", "代收车船税（元）", "提取「车船税」金额；无则留空", `车船税\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"保险公司名称", "保险公司名称", "text", "承保保险公司", "提取「保险人 / 保险公司」名称（含分支机构）", "", true, true},
	{"保险公司地址", "保险公司地址", "text", "保险公司地址", "提取「保险人地址」栏", "", false, false},
	{"签单日期", "签单日期", "date", "出单日期", "提取「签单日期 / 出单日期」，统一 YYYY-MM-DD", `(?:签单日期|出单日期)\s*[:：]?\s*([0-9]{4}[-年][0-9]{1,2}[-月][0-9]{1,2})`, false, true},
}

// 11 个被保险机动车字段。
var hrInsVehicle76 = []hrInsField76{
	{"号牌号码", "号牌号码", "text", "被保险机动车号牌", "提取「号牌号码」栏，如 渝A12345", `(?:号牌号码|车牌号)\s*[:：]?\s*([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][0-9A-Z]{4,6})`, true, true},
	{"机动车种类", "机动车种类", "text", "机动车种类", "提取「机动车种类」栏", "", false, false},
	{"使用性质", "使用性质", "text", "使用性质", "提取「使用性质」栏（营运/非营运等）", "", false, true},
	{"发动机号码", "发动机号码", "text", "发动机号", "提取「发动机号码」栏", "", false, false},
	{"车架号", "车架号", "text", "车辆识别代号 VIN", "提取「车架号 / 车辆识别代号」，17 位", `(?:车架号|车辆识别代号|VIN)\s*[:：]?\s*([A-HJ-NPR-Z0-9]{17})`, false, true},
	{"厂牌型号", "厂牌型号", "text", "厂牌型号", "提取「厂牌型号」栏", "", false, true},
	{"核定载客", "核定载客", "text", "核定载客人数", "提取「核定载客」栏", "", false, false},
	{"核定载质量", "核定载质量", "text", "核定载质量", "提取「核定载质量」栏", "", false, false},
	{"排量", "排量", "text", "排量", "提取「排量」栏", "", false, false},
	{"功率", "功率", "text", "功率", "提取「功率」栏", "", false, false},
	{"登记日期", "登记日期", "date", "机动车登记日期", "提取「登记日期 / 注册日期」，统一 YYYY-MM-DD", `(?:登记日期|注册日期)\s*[:：]?\s*([0-9]{4}[-年][0-9]{1,2}[-月][0-9]{1,2})`, false, false},
}

// 6 个交强险责任限额（amount）。
var hrInsLiability76 = []hrInsField76{
	{"死亡伤残赔偿限额", "死亡伤残赔偿限额", "amount", "交强险死亡伤残赔偿限额", "提取「死亡伤残赔偿限额」金额（交强险责任限额，不是商业险保额）", `死亡伤残赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"医疗费用赔偿限额", "医疗费用赔偿限额", "amount", "交强险医疗费用赔偿限额", "提取「医疗费用赔偿限额」金额", `医疗费用赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"财产损失赔偿限额", "财产损失赔偿限额", "amount", "交强险财产损失赔偿限额", "提取「财产损失赔偿限额」金额", `财产损失赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"无责任死亡伤残赔偿限额", "无责任死亡伤残赔偿限额", "amount", "交强险无责任死亡伤残赔偿限额", "提取「无责任死亡伤残赔偿限额」金额", `无责任死亡伤残赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"无责任医疗费用赔偿限额", "无责任医疗费用赔偿限额", "amount", "交强险无责任医疗费用赔偿限额", "提取「无责任医疗费用赔偿限额」金额", `无责任医疗费用赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
	{"无责任财产损失赔偿限额", "无责任财产损失赔偿限额", "amount", "交强险无责任财产损失赔偿限额", "提取「无责任财产损失赔偿限额」金额", `无责任财产损失赔偿限额\s*[:：]?\s*[¥￥]?\s*([0-9,，.]+)`, false, true},
}

// 7 项标准险种（险种类别标签）。
var hrInsTypes76 = []string{
	"机动车损失保险",
	"机动车第三者责任保险",
	"车上人员责任险(司机)",
	"车上人员责任险(乘客)",
	"附加医保外医疗费用责任险（三者）",
	"附加医保外医疗费用责任险（车上人员司机）",
	"附加医保外医疗费用责任险（车上人员乘客）",
}

func hrInsGroup76(tab, group string) string {
	return tab + hrInsGroupSep + group
}

// hrInsuranceSubs76 生成保险单分类的完整 subs（交强险 30 + 商业险 24 + 险种类别 7）。
func hrInsuranceSubs76() []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, 61)
	add := func(tab, group string, f hrInsField76, name, label string) {
		out = append(out, types.FleetCategorySub{
			Name:      name,
			Label:     label,
			Enabled:   true,
			IsDefault: f.visible,
			DataType:  f.dtype,
			Width:     0,
			Filterable: f.filter,
			Group:      hrInsGroup76(tab, group),
			GroupLabel: tab + " · " + group,
		})
	}
	for _, f := range hrInsBase76 {
		add(hrInsTabJq76, "基本信息", f, f.name, f.label)
	}
	for _, f := range hrInsVehicle76 {
		add(hrInsTabJq76, "被保险机动车", f, f.name, f.label)
	}
	for _, f := range hrInsLiability76 {
		add(hrInsTabJq76, "责任限额", f, f.name, f.label)
	}
	// 商业险：字段身份加前缀，展示名与交强险完全一致。
	for _, f := range hrInsBase76 {
		add(hrInsTabSy76, "基本信息", f, "商业险_"+f.name, f.label)
	}
	for _, f := range hrInsVehicle76 {
		add(hrInsTabSy76, "被保险机动车", f, "商业险_"+f.name, f.label)
	}
	for _, t := range hrInsTypes76 {
		out = append(out, types.FleetCategorySub{
			Name:      "投保险别_" + t,
			Label:     t,
			Enabled:   true,
			IsDefault: true,
			DataType:  "amount",
			Width:     0,
			Group:      hrInsGroup76(hrInsTabSy76, "投保险别"),
			GroupLabel: "商业险 · 投保险别",
		})
	}
	for _, t := range hrInsTypes76 {
		out = append(out, types.FleetCategorySub{
			Name:      t,
			Label:     t,
			Enabled:   true,
			IsDefault: false, // 险种字典不是保单列
			DataType:  "text",
			Width:     0,
			Group:      hrInsTabXz76,
			GroupLabel: hrInsTabXz76,
		})
	}
	return out
}

// hrInsuranceExtractFields76 生成提取规则字段清单（与 subs 同源、同序）。
func hrInsuranceExtractFields76(subs []types.FleetCategorySub) []types.ExtractFieldConfig {
	ruleOf := func(name string) (desc, rule, regex string) {
		base := strings.TrimPrefix(name, "商业险_")
		base = strings.TrimPrefix(base, "投保险别_")
		for _, arr := range [][]hrInsField76{hrInsBase76, hrInsVehicle76, hrInsLiability76} {
			for _, f := range arr {
				if f.name == base {
					return f.desc, f.rule, f.regex
				}
			}
		}
		return "险种分项金额/保额", "按投保险别分别提取对应金额，无则留空", ""
	}
	out := make([]types.ExtractFieldConfig, 0, len(subs))
	for _, s := range subs {
		d, r, rg := ruleOf(s.Name)
		out = append(out, types.ExtractFieldConfig{
			Name:    s.Name,
			Label:   s.Label,
			Desc:    d,
			Type:    s.DataType,
			Rule:    r,
			Regex:   rg,
			Enabled: true,
		})
	}
	return out
}

const hrInsurancePrompt76 = `你是保险合同信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。

需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON，未找到的字段输出 null，不臆造内容。

区分要点：
1. 交强险的「死亡伤残赔偿限额 / 医疗费用赔偿限额 / 财产损失赔偿限额」及其「无责任」三项属于责任限额，只从交强险保单的责任限额表取值，绝不能用商业险的保额填充；
2. 商业险的投保险别（机动车损失保险、第三者责任保险、车上人员责任险等）按险种逐项提取对应的保额/金额，一项一行，不要把多个险种金额合并成一个数；
3. 保单号、被保险人、号牌号码、车架号等基础信息按原文逐字提取，不要改写；
4. 金额只保留数字与小数点，日期统一为 YYYY-MM-DD。`

// hrReconfigFleetInsurance76 幂等重配保险单字段与提取规则。
//
// 保留策略：已有 subs 里同名字段的 width / label / enabled / filterable 原样保留
// （用户手工调过的列宽与启停不会被覆盖），只补新增字段与分组。
func (h *HRHandler) hrReconfigFleetInsurance76(ctx context.Context, tid int64) (fields int, cfgSeeded bool) {
	want := hrInsuranceSubs76()
	var cat types.FleetCategory
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL", tid, "vehicle", "保险单").
		Order("sort_order ASC").First(&cat).Error
	if err != nil {
		cat = types.FleetCategory{
			ID: newID76(), TenantID: tid, Scope: "vehicle", Name: "保险单",
			Enabled: true, SortOrder: 5,
		}
		if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
			logger.Warnf(ctx, "第 76 轮创建保险单分类失败: %v", err)
			return 0, false
		}
	}
	// 旧 subs 与旧字段名映射（存量保单数据迁移用）
	oldByName := map[string]types.FleetCategorySub{}
	for _, s := range cat.Subs {
		oldByName[strings.TrimSpace(s.Name)] = s
	}
	// 旧→新字段别名：旧版保险单字段（保单号/保险公司/车牌号…）的列宽与启停
	// 必须平滑迁到新标准字段上，否则用户调过的列宽在升级后全部回到默认。
	legacyNameOf := map[string]string{
		"保险单号":   "保单号",
		"保险公司名称": "保险公司",
		"被保险人":   "被保险人名称",
		"号牌号码":   "车牌号",
		"车架号":    "VIN码",
		"保险起始日期": "起保日期",
		"保险截止日期": "终保日期",
		"保费合计":   "保费",
		"车船税":    "车船税合计",
	}
	merged := make([]types.FleetCategorySub, 0, len(want))
	for _, w := range want {
		old, ok := oldByName[w.Name]
		if !ok {
			if legacy, has := legacyNameOf[w.Name]; has {
				old, ok = oldByName[legacy]
			}
		}
		if ok {
			if strings.TrimSpace(old.Label) != "" {
				w.Label = old.Label
			}
			w.Width = old.Width
			w.Enabled = old.Enabled
			w.Filterable = w.Filterable || old.Filterable
			w.IsDefault = old.IsDefault
		}
		merged = append(merged, w)
		fields++
	}
	// 保留前端在保存时补入的档案级系统列（证照状态 / 提取状态，group 为空）。
	// 这些列不属于保险业务字段，但列表状态与提取状态依赖它们；重建 subs 时
	// 丢掉会让用户每次重启后看到状态列消失。
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[w.Name] = true
	}
	// 旧字段名（保单号/保险公司/车牌号…）已映射到新字段，不能当"系统列"留下
	for _, legacy := range legacyNameOf {
		wanted[legacy] = true
	}
	extra := make([]types.FleetCategorySub, 0, 2)
	for _, old := range cat.Subs {
		name := strings.TrimSpace(old.Name)
		if name == "" || wanted[name] || strings.TrimSpace(old.Group) != "" {
			continue
		}
		extra = append(extra, old)
	}
	merged = append(merged, extra...)
	// JSONB 列必须传序列化后的 JSON 字符串：GORM 的 map Updates 不会对
	// []FleetCategorySub 做 serializer（真机报 unsupported type），
	// 与 UpdateFleetCategory 的 dataJSON 口径保持一致。
	subsJSON, jerr := json.Marshal(merged)
	if jerr != nil {
		logger.Warnf(ctx, "第 76 轮序列化保险单字段失败: %v", jerr)
		return 0, false
	}
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("id = ?", cat.ID).
		Updates(map[string]interface{}{"subs": string(subsJSON), "enabled": true}).Error; err != nil {
		logger.Warnf(ctx, "第 76 轮写入保险单字段失败: %v", err)
		return 0, false
	}
	// 存量保单：旧字段名 → 新字段名补齐（旧键保留，绝不删数）
	h.hrMapLegacyInsuranceRecords76(ctx, tid)
	// 提取规则：按知识库 × scope=vehicle × cert_type=保险单 播种（已存在则只补空字段）
	var kbs []types.KnowledgeBase
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&kbs).Error
	for _, kb := range kbs {
		if !strings.Contains(kb.Name, "车队") {
			continue
		}
		if err := h.hrSeedInsuranceExtract76(ctx, tid, kb.ID, merged); err != nil {
			logger.Warnf(ctx, "第 76 轮播种保险单提取规则失败(kb=%s): %v", kb.Name, err)
		} else {
			cfgSeeded = true
		}
	}
	return fields, cfgSeeded
}

// hrMapLegacyInsuranceRecords76 存量保单按旧字段名映射补齐新字段。
func (h *HRHandler) hrMapLegacyInsuranceRecords76(ctx context.Context, tid int64) int {
	var rows []types.FleetRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL AND (doc_type = ? OR sub_category = ?)", tid, "保险单", "保险单").
		Find(&rows).Error; err != nil {
		return 0
	}
	alias := map[string]string{
		"保单号":    "保险单号",
		"保险公司":   "保险公司名称",
		"被保险人名称": "被保险人",
		"车牌号":    "号牌号码",
		"VIN码":    "车架号",
		"起保日期":   "保险起始日期",
		"终保日期":   "保险截止日期",
		"保费":     "保费合计",
		"总保费":    "保费合计",
		"车船税合计":  "车船税",
	}
	changed := 0
	for i := range rows {
		row := &rows[i]
		if row.Data == nil {
			continue
		}
		dirty := false
		for oldKey, newKey := range alias {
			v, ok := row.Data[oldKey]
			if !ok {
				continue
			}
			if cur, exists := row.Data[newKey]; exists && strings.TrimSpace(toStringValue(cur)) != "" {
				continue
			}
			row.Data[newKey] = v
			dirty = true
		}
		if !dirty {
			continue
		}
		// JSONB 必须走 serializer（map 直写会被 database/sql 拒绝）
		if err := h.db.WithContext(ctx).Model(row).Select("data").Updates(row).Error; err == nil {
			changed++
		}
	}
	return changed
}

// hrSeedInsuranceExtract76 播种保险单提取规则（幂等：已存在配置只补空值）。
func (h *HRHandler) hrSeedInsuranceExtract76(ctx context.Context, tid int64, kbID string, subs []types.FleetCategorySub) error {
	fields := hrInsuranceExtractFields76(subs)
	var cfg types.KbExtractConfig
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ? AND scope = ? AND cert_type = ? AND deleted_at IS NULL",
			tid, kbID, "vehicle", "保险单").
		First(&cfg).Error
	if err != nil {
		cfg = types.KbExtractConfig{
			ID:              newID76(),
			TenantID:        tid,
			KnowledgeBaseID: kbID,
			Scope:           "vehicle",
			CertType:        "保险单",
			Fields:          fields,
			AdvancedEnabled: true,
			PromptTemplate:  hrInsurancePrompt76,
			Version:         1,
			Enabled:         true,
		}
		return h.db.WithContext(ctx).Create(&cfg).Error
	}
	// 已存在：按 name 合并，保留用户手写规则；只补缺失字段与空模板。
	byName := map[string]int{}
	for i, f := range cfg.Fields {
		byName[f.Name] = i
	}
	for _, f := range fields {
		if idx, ok := byName[f.Name]; ok {
			if strings.TrimSpace(cfg.Fields[idx].Rule) == "" {
				cfg.Fields[idx] = f
			}
			continue
		}
		cfg.Fields = append(cfg.Fields, f)
	}
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		cfg.PromptTemplate = hrInsurancePrompt76
		cfg.AdvancedEnabled = true
	}
	fieldsJSON, ferr := json.Marshal(cfg.Fields)
	if ferr != nil {
		return ferr
	}
	return h.db.WithContext(ctx).Model(&types.KbExtractConfig{}).
		Where("id = ?", cfg.ID).
		Updates(map[string]interface{}{
			"fields":           string(fieldsJSON),
			"prompt_template":  cfg.PromptTemplate,
			"advanced_enabled": cfg.AdvancedEnabled,
			"enabled":          true,
		}).Error
}

// hrRepair76 第 76 轮启动迁移总入口（幂等）。
//
// 与第 75 轮同范式：启动链路没有 HTTP 上下文，租户 id 从花名册去重取；
// 一个租户都没有时什么都不做（空库首启由 seed 链路负责建档）。
func (h *HRHandler) hrRepair76(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 76 轮读取租户失败: %v", err)
		return
	}
	for _, tid := range tids {
		if tid <= 0 {
			continue
		}
		h.hrEnsureOrgDefaults(ctx, tid)
		h.hrEnsureOrgUnits(ctx, tid)
		units, linked := h.hrMigrateTenantToOrg76(ctx, tid)
		if err := h.hrSyncOrgTenants76(ctx, tid); err != nil {
			logger.Warnf(ctx, "第 76 轮同步租户档案失败: %v", err)
		}
		fields, cfg := h.hrReconfigFleetInsurance76(ctx, tid)
		logger.Infof(ctx,
			"第 76 轮数据修复（tenant=%d）：单位档案 %d 条、租户关联 %d 条、保险单字段 %d 个、提取规则 %v",
			tid, units, linked, fields, cfg)
	}
}
