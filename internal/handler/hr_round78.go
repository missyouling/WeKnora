package handler

// 第 78 轮：维保记录重构（一般维修 / 二级维护）+ 行驶证年检日期。
//
// 三条主线：
//  1. `维保记录` 原 3 个分类（一般维修 / 合格证 / 二级维护）收敛为 2 个业务分类。
//     原「合格证」并入「二级维护」—— 二级维护竣工出厂合格证与质量保证卡本就是
//     同一份单据的两联，拆成两个分类会让用户在两处各填一半。
//     存量「合格证」记录按字段别名映射迁到「二级维护」，旧键全部保留，绝不删数。
//  2. 「一般维修」重建为维修结算清单三段式字段（基本信息 / 费用汇总 / 承修单位），
//     并播种完整提取规则（含 labor_items / part_items / extra_items 三组明细）。
//  3. 「二级维护」重建为合格证 + 质保卡字段（合格证信息 / 车辆与质保 / 承修与质检）。
//  4. 行驶证补「年检日期」字段与提取规则（证照状态改按年检日期判定）。

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ---------------------------------------------------------------------------
// 字段定义
// ---------------------------------------------------------------------------

// hrField78 字段声明（与前端 fleetCategorySeed 一一对应）。
type hrField78 struct {
	name   string
	label  string
	dtype  string
	group  string
	// 列表默认展示
	def bool
	// 筛选栏可见
	filt bool
}

// hrRepairSubs78 「一般维修」= 维修结算清单，3 个胶囊分组共 23 个字段。
func hrRepairSubs78() []types.FleetCategorySub {
	const (
		gBase  = "基本信息"
		gFee   = "费用汇总"
		gRep   = "承修单位"
	)
	fields := []hrField78{
		// 基本信息（10）
		{"结算单号", "结算单号", "text", gBase, true, true},
		{"车牌号", "车牌号", "text", gBase, true, true},
		{"进厂日期", "进厂日期", "date", gBase, false, false},
		{"结算日期", "结算日期", "date", gBase, true, false},
		{"挂账单位", "挂账单位", "text", gBase, true, false},
		{"里程", "里程", "number", gBase, false, false},
		{"车型", "车型", "text", gBase, false, false},
		{"车架号", "车架号", "text", gBase, false, false},
		{"发动机号", "发动机号", "text", gBase, false, false},
		{"维修项目摘要", "维修项目摘要", "text", gBase, true, false},
		// 费用汇总（7）
		{"工时费", "工时费", "amount", gFee, true, false},
		{"材料费", "材料费", "amount", gFee, true, false},
		{"管理费", "管理费", "amount", gFee, false, false},
		{"其它费", "其它费", "amount", gFee, true, false},
		{"实际应收", "实际应收", "amount", gFee, true, false},
		{"结算方式", "结算方式", "text", gFee, false, false},
		{"接车人", "接车人", "text", gFee, false, false},
		// 承修单位（6）
		{"承修单位", "承修单位", "text", gRep, true, false},
		{"承修电话", "承修电话", "text", gRep, false, false},
		{"承修地址", "承修地址", "text", gRep, false, false},
		{"开户行", "开户行", "text", gRep, false, false},
		{"银行账号", "银行账号", "text", gRep, false, false},
		{"操作员", "操作员", "text", gRep, false, false},
	}
	return hrSubsOf78(fields)
}

// hrMaintenanceSubs78 「二级维护」= 竣工出厂合格证 + 质量保证卡，3 组 22 个字段
// （第 23 个是派生只读的「附件」计数，由前端注入，不落 subs）。
func hrMaintenanceSubs78() []types.FleetCategorySub {
	const (
		gCert = "合格证信息"
		gVeh  = "车辆与质保"
		gRep  = "承修与质检"
	)
	fields := []hrField78{
		// 合格证信息（10）
		{"合格证编号", "合格证编号", "text", gCert, true, true},
		{"托修方", "托修方", "text", gCert, false, false},
		{"维修类别", "维修类别", "text", gCert, true, true},
		{"合格证类型", "合格证类型", "text", gCert, false, false},
		{"维修合同编号", "维修合同编号", "text", gCert, false, false},
		{"维修档案编号", "维修档案编号", "text", gCert, false, false},
		{"进厂日期", "进厂日期", "date", gCert, false, false},
		{"出厂日期", "出厂日期", "date", gCert, true, false},
		{"接车日期", "接车日期", "date", gCert, false, false},
		{"托修方接车人", "托修方接车人", "text", gCert, false, false},
		// 车辆与质保（8）
		{"车牌号", "车牌号", "text", gVeh, true, true},
		{"厂牌型号", "厂牌型号", "text", gVeh, false, false},
		{"发动机号", "发动机号", "text", gVeh, false, false},
		{"车架号", "车架号", "text", gVeh, false, false},
		{"出厂里程", "出厂里程", "number", gVeh, true, false},
		{"质保里程", "质保里程", "number", gVeh, true, false},
		{"质保天数", "质保天数", "number", gVeh, true, false},
		{"维修发票号", "维修发票号", "text", gVeh, false, false},
		// 承修与质检（5）
		{"承修方", "承修方", "text", gRep, true, false},
		{"承修方许可证号", "承修方许可证号", "text", gRep, false, false},
		{"质量检验员", "质量检验员", "text", gRep, true, false},
		{"承修方电话", "承修方电话", "text", gRep, false, false},
	}
	return hrSubsOf78(fields)
}

func hrSubsOf78(fields []hrField78) []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, len(fields))
	for _, f := range fields {
		out = append(out, types.FleetCategorySub{
			Name:       f.name,
			Label:      f.label,
			Enabled:    true,
			IsDefault:  f.def,
			DataType:   f.dtype,
			Width:      0,
			Filterable: f.filt,
			Group:      f.group,
			GroupLabel: f.group,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 一、维保分类收敛（hrReconfigMaintenance78）
// ---------------------------------------------------------------------------

// hrLegacyMaintenanceAlias78 「合格证」旧字段 → 「二级维护」新字段的映射。
//
// 旧版合格证分类的字段名与票面用词更贴近（拖修方 / 车牌号码 / 底盘（车身）号 /
// 出厂里程表示值 / 质检员 / 接车人），新版按业务口径统一（托修方 / 车牌号 /
// 车架号 / 出厂里程 / 质量检验员 / 托修方接车人）。映射只**补新键**，旧键保留。
var hrLegacyMaintenanceAlias78 = map[string]string{
	"拖修方":      "托修方",
	"车牌号码":     "车牌号",
	"底盘（车身）号":  "车架号",
	"底盘(车身)号":  "车架号",
	"出厂里程表示值":  "出厂里程",
	"质检员":      "质量检验员",
	"接车人":      "托修方接车人",
	"发动机型号/编号": "发动机号",
}

// hrReconfigMaintenance78 幂等重配维保记录的两个业务分类。
//
// 返回 (一般维修字段数, 二级维护字段数, 迁移的合格证记录数, 是否播种提取规则)。
//
// 保留策略与第 76 轮保险单一致：已有 subs 里同名字段的 width / label / enabled /
// filterable / is_default 原样保留，只补新增字段与分组 —— 用户手工调过的列宽
// 与启停不能被一次升级抹平。
func (h *HRHandler) hrReconfigMaintenance78(ctx context.Context, tid int64) (repairFields, maintFields, migrated int, cfgSeeded bool) {
	repairFields = h.hrUpsertMaintainCategory78(ctx, tid, "一般维修", hrRepairSubs78(), 1)
	maintFields = h.hrUpsertMaintainCategory78(ctx, tid, "二级维护", hrMaintenanceSubs78(), 2)

	// 「合格证」分类：先把存量记录映射到「二级维护」，再**停用**该分类
	// （不删除 —— 删了 KPI 卡片会消失但记录还在，用户找不到入口；
	// 停用后卡片不显示，记录已迁到二级维护，语义干净且可回滚）。
	migrated = h.hrMigrateMaintenanceCertRecords78(ctx, tid)
	h.hrDisableLegacyCertCategory78(ctx, tid)

	// 提取规则：为两个分类各播种一份（只补空规则，不覆盖用户手写）
	var kbs []types.KnowledgeBase
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&kbs).Error
	for _, kb := range kbs {
		if !strings.Contains(kb.Name, "车队") {
			continue
		}
		if err := h.hrSeedMaintainExtract78(ctx, tid, kb.ID, "一般维修", hrRepairExtractFields78(), hrRepairPrompt78); err != nil {
			logger.Warnf(ctx, "第 78 轮播种一般维修提取规则失败(kb=%s): %v", kb.Name, err)
		} else {
			cfgSeeded = true
		}
		if err := h.hrSeedMaintainExtract78(ctx, tid, kb.ID, "二级维护", hrMaintenanceExtractFields78(), hrMaintenancePrompt78); err != nil {
			logger.Warnf(ctx, "第 78 轮播种二级维护提取规则失败(kb=%s): %v", kb.Name, err)
		} else {
			cfgSeeded = true
		}
	}
	return repairFields, maintFields, migrated, cfgSeeded
}

// hrUpsertMaintainCategory78 创建或重配一个维保分类，返回最终字段数。
func (h *HRHandler) hrUpsertMaintainCategory78(
	ctx context.Context, tid int64, name string, want []types.FleetCategorySub, sortOrder int,
) int {
	var cat types.FleetCategory
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL", tid, "maintain", name).
		Order("sort_order ASC").First(&cat).Error
	if err != nil {
		cat = types.FleetCategory{
			ID: newID78(), TenantID: tid, Scope: "maintain", Name: name,
			Enabled: true, SortOrder: sortOrder,
		}
		if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
			logger.Warnf(ctx, "第 78 轮创建维保分类 %s 失败: %v", name, err)
			return 0
		}
	}
	oldByName := map[string]types.FleetCategorySub{}
	for _, s := range cat.Subs {
		oldByName[strings.TrimSpace(s.Name)] = s
	}
	merged := make([]types.FleetCategorySub, 0, len(want))
	for _, w := range want {
		if old, ok := oldByName[w.Name]; ok {
			if strings.TrimSpace(old.Label) != "" {
				w.Label = old.Label
			}
			w.Width = old.Width
			w.Enabled = old.Enabled
			w.Filterable = w.Filterable || old.Filterable
			// is_default 只在旧值为 true 时继承：新版本按业务口径重新定默认表头，
			// 旧的 false 不该压掉新的 true（否则升级后表头全空）。
			if old.IsDefault {
				w.IsDefault = true
			}
		}
		merged = append(merged, w)
	}
	// 保留用户自建的字段（不在标准清单里）与档案级系统列（group 为空）
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[w.Name] = true
	}
	for _, old := range cat.Subs {
		n := strings.TrimSpace(old.Name)
		if n == "" || wanted[n] {
			continue
		}
		merged = append(merged, old)
	}
	subsJSON, jerr := json.Marshal(merged)
	if jerr != nil {
		logger.Warnf(ctx, "第 78 轮序列化 %s 字段失败: %v", name, jerr)
		return 0
	}
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("id = ?", cat.ID).
		Updates(map[string]interface{}{"subs": string(subsJSON), "enabled": true}).Error; err != nil {
		logger.Warnf(ctx, "第 78 轮写入 %s 字段失败: %v", name, err)
		return 0
	}
	return len(merged)
}

// hrMigrateMaintenanceCertRecords78 存量「合格证」记录 → 「二级维护」。
//
// 只改 sub_category / doc_type 与补齐新字段别名；**旧字段全部保留**（data 里
// 既有的「拖修方」不会因为补了「托修方」而被删），保证任何历史单据都能原样回看。
func (h *HRHandler) hrMigrateMaintenanceCertRecords78(ctx context.Context, tid int64) int {
	var rows []types.FleetRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL AND record_type = ? AND (sub_category = ? OR doc_type = ?)",
			tid, "maintain-archive", "合格证", "合格证").
		Find(&rows).Error; err != nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		data := r.Data
		if data == nil {
			data = map[string]any{}
		}
		for legacy, cur := range hrLegacyMaintenanceAlias78 {
			v, has := data[legacy]
			if !has {
				continue
			}
			if _, exists := data[cur]; exists {
				continue
			}
			data[cur] = v
		}
		dataJSON, jerr := json.Marshal(data)
		if jerr != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).Where("id = ?", r.ID).
			Updates(map[string]interface{}{
				"data":         string(dataJSON),
				"doc_type":     "二级维护",
				"sub_category": "二级维护",
			}).Error; err != nil {
			logger.Warnf(ctx, "第 78 轮迁移合格证记录 %s 失败: %v", r.ID, err)
			continue
		}
		n++
	}
	return n
}

// hrDisableLegacyCertCategory78 停用「合格证」分类（记录已迁走，不再展示卡片）。
func (h *HRHandler) hrDisableLegacyCertCategory78(ctx context.Context, tid int64) {
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = ? AND scope = ? AND name = ?", tid, "maintain", "合格证").
		Update("enabled", false).Error; err != nil {
		logger.Warnf(ctx, "第 78 轮停用合格证分类失败: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 二、提取规则
// ---------------------------------------------------------------------------

// hrSeedMaintainExtract78 播种某维保分类的提取规则（已存在则只补空规则与空模板）。
func (h *HRHandler) hrSeedMaintainExtract78(
	ctx context.Context, tid int64, kbID, certType string,
	fields []types.ExtractFieldConfig, prompt string,
) error {
	var cfg types.KbExtractConfig
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ? AND scope = ? AND cert_type = ? AND deleted_at IS NULL",
			tid, kbID, "maintain", certType).
		First(&cfg).Error
	if err != nil {
		cfg = types.KbExtractConfig{
			ID:              newID78(),
			TenantID:        tid,
			KnowledgeBaseID: kbID,
			Scope:           "maintain",
			CertType:        certType,
			Fields:          fields,
			AdvancedEnabled: true,
			PromptTemplate:  prompt,
			Version:         1,
			Enabled:         true,
		}
		return h.db.WithContext(ctx).Create(&cfg).Error
	}
	byName := map[string]int{}
	for i, f := range cfg.Fields {
		byName[f.Name] = i
	}
	for _, f := range fields {
		if idx, ok := byName[f.Name]; ok {
			// 已有规则且非空 → 用户手写过，绝不覆盖；只补类型与正则空位
			if strings.TrimSpace(cfg.Fields[idx].Regex) == "" {
				cfg.Fields[idx].Regex = f.Regex
			}
			if strings.TrimSpace(cfg.Fields[idx].Rule) == "" {
				cfg.Fields[idx].Rule = f.Rule
			}
			continue
		}
		cfg.Fields = append(cfg.Fields, f)
	}
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		cfg.PromptTemplate = prompt
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

// hrRepairExtractFields78 「一般维修」提取规则：主表 23 字段 + 3 组明细数组说明。
func hrRepairExtractFields78() []types.ExtractFieldConfig {
	type rule struct{ name, dtype, desc, r, regex string }
	list := []rule{
		{"结算单号", "text", "结算单右上角 No. 后的编号", "取 No. 之后的完整编号，保留前缀字母与连字符", `No[.．:：]?\s*([A-Za-z0-9\-]{4,32})`},
		{"车牌号", "text", "维修车辆号牌", "逐字提取，含省份简称与字母", `([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][A-HJ-NP-Z0-9]{5,6})`},
		{"进厂日期", "date", "车辆进厂日期", "统一为 YYYY-MM-DD", `进厂日期[^\d]{0,8}(\d{4}[-/年]\d{1,2}[-/月]\d{1,2})`},
		{"结算日期", "date", "结算单开具日期", "统一为 YYYY-MM-DD", `结算日期[^\d]{0,8}(\d{4}[-/年]\d{1,2}[-/月]\d{1,2})`},
		{"挂账单位", "text", "挂账/结算对象单位名称", "逐字提取单位全称", ""},
		{"里程", "number", "进厂里程数（km）", "只保留数字", `里程[^\d]{0,6}(\d{1,7})`},
		{"车型", "text", "车辆品牌型号", "逐字提取", ""},
		{"车架号", "text", "17 位 VIN", "严格 17 位大写字母与数字，不含 I/O/Q", `([A-HJ-NPR-Z0-9]{17})`},
		{"发动机号", "text", "发动机编号", "逐字提取，保留字母与数字", ""},
		{"维修项目摘要", "text", "维修内容摘要", "按明细归纳为「项目A、项目B（N项/M件）」", ""},
		{"工时费", "amount", "工时费合计", "只保留数字与小数点", `工时费[^\d]{0,8}([\d,]+\.?\d{0,2})`},
		{"材料费", "amount", "材料费合计", "只保留数字与小数点", `材料费[^\d]{0,8}([\d,]+\.?\d{0,2})`},
		{"管理费", "amount", "管理费", "只保留数字与小数点，无则留空", `管理费[^\d]{0,8}([\d,]+\.?\d{0,2})`},
		{"其它费", "amount", "其它费用（税费等）", "只保留数字与小数点", `其[它他]费[^\d]{0,8}([\d,]+\.?\d{0,2})`},
		{"实际应收", "amount", "实际应收合计", "只保留数字与小数点", `(?:实际应收|应收[合总]计|总计)[^\d]{0,8}([\d,]+\.?\d{0,2})`},
		{"结算方式", "text", "现金/挂账/转账等", "逐字提取", ""},
		{"接车人", "text", "接车人姓名", "逐字提取", ""},
		{"承修单位", "text", "承修方单位全称", "逐字提取", ""},
		{"承修电话", "text", "承修方联系电话", "保留区号与连字符", `(?:电话|联系电话)[^\d]{0,6}([\d\-]{7,20})`},
		{"承修地址", "text", "承修方地址", "逐字提取", ""},
		{"开户行", "text", "承修方开户银行", "逐字提取", ""},
		{"银行账号", "text", "承修方银行账号", "只保留数字", ""},
		{"操作员", "text", "制单/操作员", "逐字提取", ""},
		// 三组明细：以 items 类型声明，模型需输出数组
		{"labor_items", "items", "维修项目明细数组", "每行 {item_name, hours, labor_fee, technician, remark}", ""},
		{"part_items", "items", "零件材料明细数组", "每行 {part_name, unit, qty, unit_price, amount, remark}", ""},
		{"extra_items", "items", "附加费用明细数组", "每行 {item_name, amount}", ""},
	}
	out := make([]types.ExtractFieldConfig, 0, len(list))
	for _, x := range list {
		out = append(out, types.ExtractFieldConfig{
			Name: x.name, Label: x.name, Desc: x.desc, Type: x.dtype,
			Rule: x.r, Regex: x.regex, Enabled: true,
		})
	}
	return out
}

// hrMaintenanceExtractFields78 「二级维护」提取规则（双联票：合格证 + 质保卡）。
func hrMaintenanceExtractFields78() []types.ExtractFieldConfig {
	type rule struct{ name, dtype, desc, r, regex string }
	list := []rule{
		{"合格证编号", "text", "右上/左上角 No. 后的编号（括号内年份不属于编号）", "只取 No. 后的数字串，忽略括号里的年份", `No[.．:：]?\s*(\d{4,12})`},
		{"托修方", "text", "托修单位名称（旧版写「拖修方」）", "逐字提取单位全称，印章叠字以正文为准", ""},
		{"维修类别", "text", "如「二级维护」", "逐字提取", `维修类别[^\u4e00-\u9fa5]{0,4}([\u4e00-\u9fa5]{2,10})`},
		{"合格证类型", "text", "竣工出厂合格证", "逐字提取", ""},
		{"维修合同编号", "text", "维修合同编号", "逐字提取", ""},
		{"维修档案编号", "text", "维修档案编号", "逐字提取", ""},
		{"进厂日期", "date", "进厂日期", "统一为 YYYY-MM-DD", `进厂日期[^\d]{0,8}(\d{4}[-/年]\d{1,2}[-/月]\d{1,2})`},
		{"出厂日期", "date", "出厂日期", "统一为 YYYY-MM-DD", `出厂日期[^\d]{0,8}(\d{4}[-/年]\d{1,2}[-/月]\d{1,2})`},
		{"接车日期", "date", "接车日期", "统一为 YYYY-MM-DD", `接车日期[^\d]{0,8}(\d{4}[-/年]\d{1,2}[-/月]\d{1,2})`},
		{"托修方接车人", "text", "托修方接车人姓名", "逐字提取", ""},
		{"车牌号", "text", "车牌号码（旧版写「车牌号码」）", "逐字提取，含省份简称", `([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][A-HJ-NP-Z0-9]{5,6})`},
		{"厂牌型号", "text", "厂牌型号", "逐字提取", ""},
		{"发动机号", "text", "发动机号（旧版写「发动机型号/编号」）", "逐字提取", ""},
		{"车架号", "text", "底盘(车身)号 = 17 位 VIN", "取底盘(车身)号栏，严格 17 位", `([A-HJ-NPR-Z0-9]{17})`},
		{"出厂里程", "number", "出厂里程表示值（km）", "只保留数字（旧版字段名「出厂里程表示值」）", `出厂里程[^\d]{0,8}(\d{1,7})`},
		{"质保里程", "number", "质量保证卡质保里程（万公里 → 折算为公里数值）", "如「0.5万公里」取 5000；仅数字", `质保里程[^\d]{0,8}([\d.]+)`},
		{"质保天数", "number", "质量保证卡质保天数（日）", "只保留数字", `质保(?:天数|期)[^\d]{0,8}(\d{1,4})`},
		{"维修发票号", "text", "维修发票号", "逐字提取", ""},
		{"承修方", "text", "承修单位全称", "逐字提取，印章叠字以正文为准", ""},
		{"承修方许可证号", "text", "承修方许可证号", "逐字提取", ""},
		{"质量检验员", "text", "质量检验员（旧版写「质检员」）", "逐字提取", ""},
		{"承修方电话", "text", "承修方联系电话", "保留区号与连字符", `(?:电话|联系电话)[^\d]{0,6}([\d\-]{7,20})`},
	}
	out := make([]types.ExtractFieldConfig, 0, len(list))
	for _, x := range list {
		out = append(out, types.ExtractFieldConfig{
			Name: x.name, Label: x.name, Desc: x.desc, Type: x.dtype,
			Rule: x.r, Regex: x.regex, Enabled: true,
		})
	}
	return out
}

const hrRepairPrompt78 = `你是汽车维修结算清单信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。

需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON，未找到的字段输出 null，不臆造内容。

三组明细数组（必须同时输出，缺一不可）：
1. labor_items —— 维修项目明细，每行 {"item_name": "项目名称", "hours": 工时数, "labor_fee": 工时费金额, "technician": "维修技师", "remark": "备注"}；
2. part_items —— 零件材料明细，每行 {"part_name": "零件名称", "unit": "单位", "qty": 数量, "unit_price": 单价, "amount": 金额, "remark": "备注"}；
3. extra_items —— 附加费用明细，每行 {"item_name": "费用名称", "amount": 金额}。

校验要点：
- 三组明细的金额合计必须与主表「工时费」「材料费」「其它费」一致；不一致时以票面明细为准，并在对应主表字段给出明细合计值；
- part_items 每行必须满足 qty × unit_price = amount；
- 金额只保留数字与小数点，不要带「¥」「元」「,」；
- 日期统一为 YYYY-MM-DD；
- 车牌号、车架号（17 位 VIN）、发动机号逐字提取，不要改写。`

const hrMaintenancePrompt78 = `你是二级维护竣工出厂合格证与质量保证卡信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。

需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON，未找到的字段输出 null，不臆造内容。

票面特征与处理要点：
1. 该单据为**双联排版**：左侧/上部是《竣工出厂合格证（车属单位保管）》，右侧/下部是《质量保证卡》。同一栏位在两联可能重复，取字迹清晰的一联；
2. 右上或左上角的「No. 4926591 (2020)」中，括号内年份**不属于**合格证编号，编号只取 No. 后的数字串；
3. 印章叠字会压住栏位文字，以未被遮挡的正文为准，不要因为印章红字改写数值；
4. 旧版栏位名与新字段的对应：拖修方→托修方、车牌号码→车牌号、底盘(车身)号→车架号、出厂里程表示值→出厂里程、质检员→质量检验员、接车人→托修方接车人；
5. 质保里程常见写法「0.5 万公里」，折算为 5000（公里）；质保天数常见「30 日」，只取 30；
6. 承修方许可证号、承修方电话、维修发票号只在《质量保证卡》一侧出现；
7. 日期统一为 YYYY-MM-DD。`

// ---------------------------------------------------------------------------
// 三、行驶证「年检日期」
// ---------------------------------------------------------------------------

// hrDrivingLicenseInspection78 行驶证年检日期的字段契约。
type hrDrivingLicenseInspection78 struct {
	name  string
	label string
	group string
}

var hrDrivingLicenseInspection = hrDrivingLicenseInspection78{
	name: "年检日期", label: "年检日期", group: "",
}

// hrAddDrivingLicenseInspection78 给「行驶证」分类补「年检日期」字段与提取规则。
//
// 幂等：字段已存在则只保证 enabled=true / is_default=true（不覆盖用户改过的宽度与顺序）；
// 插入位置紧跟「强制报废日期」之后 —— 语义上两个都是「合法性边界」，
// 放在一起用户扫一眼就能看到；找不到锚点时追加到末尾。
func (h *HRHandler) hrAddDrivingLicenseInspection78(ctx context.Context, tid int64) (added int, cfgSeeded bool) {
	var cat types.FleetCategory
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL", tid, "vehicle", "行驶证").
		Order("sort_order ASC").First(&cat).Error
	if err != nil {
		// 行驶证分类还不存在（空库首启）：交给种子链路，这里什么都不做
		return 0, false
	}
	exists := false
	for i := range cat.Subs {
		if strings.TrimSpace(cat.Subs[i].Name) == hrDrivingLicenseInspection.name {
			exists = true
			cat.Subs[i].Enabled = true
			cat.Subs[i].IsDefault = true
			if cat.Subs[i].DataType == "" {
				cat.Subs[i].DataType = "date"
			}
			break
		}
	}
	if !exists {
		fresh := types.FleetCategorySub{
			Name:       hrDrivingLicenseInspection.name,
			Label:      hrDrivingLicenseInspection.label,
			Enabled:    true,
			IsDefault:  true,
			DataType:   "date",
			Width:      0,
			Filterable: false,
		}
		at := -1
		for i, s := range cat.Subs {
			if strings.TrimSpace(s.Name) == "强制报废日期" {
				at = i + 1
				break
			}
		}
		if at < 0 {
			cat.Subs = append(cat.Subs, fresh)
		} else {
			next := make([]types.FleetCategorySub, 0, len(cat.Subs)+1)
			next = append(next, cat.Subs[:at]...)
			next = append(next, fresh)
			next = append(next, cat.Subs[at:]...)
			cat.Subs = next
		}
		added = 1
	}
	subsJSON, jerr := json.Marshal(cat.Subs)
	if jerr != nil {
		logger.Warnf(ctx, "第 78 轮序列化行驶证字段失败: %v", jerr)
		return 0, false
	}
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("id = ?", cat.ID).
		Updates(map[string]interface{}{"subs": string(subsJSON)}).Error; err != nil {
		logger.Warnf(ctx, "第 78 轮写入行驶证字段失败: %v", err)
		return 0, false
	}
	// 提取规则：按知识库 × scope=vehicle × cert_type=行驶证 播种（只补空规则）
	var kbs []types.KnowledgeBase
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&kbs).Error
	for _, kb := range kbs {
		if !strings.Contains(kb.Name, "车队") {
			continue
		}
		if err := h.hrSeedDrivingLicenseInspection78(ctx, tid, kb.ID); err != nil {
			logger.Warnf(ctx, "第 78 轮播种年检日期规则失败(kb=%s): %v", kb.Name, err)
		} else {
			cfgSeeded = true
		}
	}
	return added, cfgSeeded
}

// hrInspectionFieldRule78 年检日期的提取规则（副页「检验有效期至 YYYY年MM月」）。
var hrInspectionFieldRule78 = types.ExtractFieldConfig{
	Name:  "年检日期",
	Label: "年检日期",
	Desc:  "行驶证副页「检验有效期至」的年月，归一到该月最后一天",
	Type:  "date",
	Rule:  "取「检验有效期至」的年月，归一为该月最后一天 YYYY-MM-DD；已是标准日期则原样保留",
	Regex: `检验有效期至[^\d]{0,6}(\d{4}\s*年\s*\d{1,2}\s*月|\d{4}[-/.]\d{1,2}(?:[-/.]\d{1,2})?)`,
	Enabled: true,
}

// hrSeedDrivingLicenseInspection78 把年检日期规则并入行驶证的提取配置。
func (h *HRHandler) hrSeedDrivingLicenseInspection78(ctx context.Context, tid int64, kbID string) error {
	var cfg types.KbExtractConfig
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ? AND scope = ? AND cert_type = ? AND deleted_at IS NULL",
			tid, kbID, "vehicle", "行驶证").
		First(&cfg).Error
	if err != nil {
		// 还没配行驶证规则：不新建一份只有年检日期的残缺配置（会让其余字段丢失），
		// 交给用户在设置里保存时整体播种。
		return nil
	}
	for i := range cfg.Fields {
		if strings.TrimSpace(cfg.Fields[i].Name) != hrInspectionFieldRule78.Name {
			continue
		}
		changed := false
		if strings.TrimSpace(cfg.Fields[i].Regex) == "" {
			cfg.Fields[i].Regex = hrInspectionFieldRule78.Regex
			changed = true
		}
		if strings.TrimSpace(cfg.Fields[i].Rule) == "" {
			cfg.Fields[i].Rule = hrInspectionFieldRule78.Rule
			changed = true
		}
		if !changed {
			return nil
		}
		fieldsJSON, jerr := json.Marshal(cfg.Fields)
		if jerr != nil {
			return jerr
		}
		return h.db.WithContext(ctx).Model(&types.KbExtractConfig{}).
			Where("id = ?", cfg.ID).Update("fields", string(fieldsJSON)).Error
	}
	cfg.Fields = append(cfg.Fields, hrInspectionFieldRule78)
	fieldsJSON, jerr := json.Marshal(cfg.Fields)
	if jerr != nil {
		return jerr
	}
	return h.db.WithContext(ctx).Model(&types.KbExtractConfig{}).
		Where("id = ?", cfg.ID).Update("fields", string(fieldsJSON)).Error
}

// ---------------------------------------------------------------------------
// 四、启动迁移入口
// ---------------------------------------------------------------------------

func newID78() string { return newID76() }

// hrRepair78 第 78 轮启动迁移入口（幂等）。
//
// 与第 76/77 轮同范式：启动链路没有 HTTP 上下文，租户 id 从花名册去重取。
func (h *HRHandler) hrRepair78(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		return
	}
	for _, tid := range tids {
		if tid <= 0 {
			continue
		}
		repair, maint, migrated, cfg1 := h.hrReconfigMaintenance78(ctx, tid)
		inspection, cfg2 := h.hrAddDrivingLicenseInspection78(ctx, tid)
		if repair+maint+migrated+inspection > 0 || cfg1 || cfg2 {
			logger.Infof(ctx,
				"第 78 轮数据修复（tenant=%d）：一般维修 %d 字段、二级维护 %d 字段、合格证迁移 %d 条、行驶证年检日期 %d、提取规则 %v/%v",
				tid, repair, maint, migrated, inspection, cfg1, cfg2)
		}
	}
}
