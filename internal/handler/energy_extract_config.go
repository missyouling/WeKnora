package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗类账单的配置解析层：把「用户在前端设置里配的那一条」精准解析到提取链路。
//
// ⚠️ 这里是 scope/cert_type 的单一定义点，前端与后端必须逐字对齐：
//
//	前端 StandardSettingDrawer scope="energy"
//	  ├─ 分类「市电账单」 → knowledge_extract_configs (scope=energy, cert_type=市电账单)
//	  └─ 分类「光伏账单」 → knowledge_extract_configs (scope=energy, cert_type=光伏账单)
//	后端 ExtractUtilityBill → resolveEnergyBillExtractConfig(kbID, EnergyBillTypeUtility)
//	后端 ExtractSolarBill   → resolveEnergyBillExtractConfig(kbID, EnergyBillTypeSolar)
//
// 历史上的坑：前端把规则写进 scope='energy', cert_type='市电账单'，后端却去查
// scope='utility'，查了个寂寞——用户在界面上配的规则"保存成功"但永远不生效，
// 且没有任何报错可循。所有读写必须只走本文件的常量。

const (
	// EnergyScope 能耗设置抽屉的统一 scope。
	//
	// 为什么三样东西共用一个 scope：市电账单、光伏账单、能耗台账在业务上是
	// 同一批"水电燃气单"，共用一套设置入口；分类（cert_type）才是区分维度。
	EnergyScope = "energy"

	// EnergyBillTypeUtility 市电账单分类名（= fleet_categories.name）。
	EnergyBillTypeUtility = "市电账单"
	// EnergyBillTypeSolar 光伏账单分类名。
	EnergyBillTypeSolar = "光伏账单"
	// EnergyBillTypeLedger 能耗台账分类名（账期级口径，走中台 document 管线）。
	EnergyBillTypeLedger = "能耗单"
)

// EnergyBillTypeOf 按账单 kind 反解出对应的 cert_type。
//
//	utility_bill → 市电账单
//	solar_bill   → 光伏账单
//	energy       → 能耗单
func EnergyBillTypeOf(kind string) string {
	switch strings.TrimSpace(kind) {
	case "utility_bill":
		return EnergyBillTypeUtility
	case "solar_bill":
		return EnergyBillTypeSolar
	case "energy":
		return EnergyBillTypeLedger
	}
	return ""
}

// EnergyScopeAndCertType 供前端/接口层查询用：给定账单 kind 返回 (scope, cert_type)。
func EnergyScopeAndCertType(kind string) (string, string) {
	return EnergyScope, EnergyBillTypeOf(kind)
}

// resolveEnergyBillExtractConfig 解析某类能耗账单的提取配置。
//
// 查找顺序（与发票的 resolveInvoiceRuleCfg 同构，保证语义一致）：
//  1. 精确命中 (energy, certType)；
//  2. certType 为空或未配置 → 回退 (energy, "") 或 (energy, "__all__") 整体规则；
//  3. 仍未命中 → nil（调用方走内置常量 Prompt，行为与改造前一致）。
//
// 返回的 cfg 已经过 subs 对齐（字段集合/顺序/类型以「字段定义」为权威），
// 调用方无需再自行合并。
func (h *BusinessExtractHandler) resolveEnergyBillExtractConfig(
	ctx context.Context, kbID, certType string,
) *types.KbExtractConfig {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" || h.db == nil {
		return nil
	}
	ct := strings.TrimSpace(certType)
	if ct != "" {
		if cfg := h.getExtractConfig(ctx, kbID, EnergyScope, ct); cfg != nil {
			return h.alignEnergyConfigWithSubs(ctx, cfg, ct)
		}
	}
	// 整体规则兜底（cert_type 为空/__all__）
	if cfg := h.getExtractConfig(ctx, kbID, EnergyScope, ""); cfg != nil {
		return h.alignEnergyConfigWithSubs(ctx, cfg, ct)
	}
	return nil
}

// alignEnergyConfigWithSubs 把配置的字段集合对齐到该分类当前的 subs。
//
// 口径与发票/合同一致：**subs 是字段集合/顺序/名称/类型的唯一权威源**，
// 配置只提供 desc/rule/regex/required。用户在「字段定义」里加了字段但
// 还没来得及在「提取规则」里配规则时，这里也要让新字段出现在提取面里
// （desc/rule 为空 = 由内置 Prompt 兜底），否则"加了字段提不出来"依旧成立。
func (h *BusinessExtractHandler) alignEnergyConfigWithSubs(
	ctx context.Context, cfg *types.KbExtractConfig, certType string,
) *types.KbExtractConfig {
	if cfg == nil || certType == "" || h.db == nil {
		return cfg
	}
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", EnergyScope, certType).
		Order("sort_order ASC").First(&cat).Error; err != nil {
		return cfg
	}
	subs := cat.Subs
	if len(subs) == 0 {
		return cfg
	}
	// 先按目录回填空白的 desc/rule/regex（不覆盖用户改过的文案）。
	//
	// 为什么必须回填：迁移出来的 subs 只带 label/type/enabled，
	// 而用户之前可能已经存过一份「字段集合对得上、但 desc 全空」的旧配置。
	// 不回填的话，「提取规则」页会有一屏「—」的空描述，看着像功能没生效。
	service.BackfillExtractConfigDefaults(cfg)

	// 目录兜底：配置里**根本没有**的字段（存量配置由旧版目录生成，缺少后加的字段）
	// 也要能显示 desc/rule，否则规则表还是会出现空描述。
	// 这里不往 cfg 里写（那是保存时才做的事），只在本次渲染的对齐副本上取值。
	defaults := make(map[string]types.ExtractFieldConfig)
	for _, d := range service.DefaultExtractFields(cfg.Scope, certType) {
		defaults[strings.TrimSpace(d.Name)] = d
	}
	// 已保存配置按 name 建索引（保留用户配的 desc/rule/regex）
	byName := make(map[string]types.ExtractFieldConfig, len(cfg.Fields))
	for _, f := range cfg.Fields {
		byName[strings.TrimSpace(f.Name)] = f
	}
	aligned := make([]types.ExtractFieldConfig, 0, len(subs))
	seen := make(map[string]bool, len(subs))
	for _, s := range subs {
		name := strings.TrimSpace(s.Name)
		if name == "" || seen[name] {
			continue
		}
		// 只有主表字段参与提取规则。
		//
		// 市电账单的其余 20 个分组是**表格结构**：11 个费用分项定义子表的列
		// （name/period/qty/rate/fee，7 组同名列），9 个 -rows 分组定义行标题。
		// 它们的数据来自 fee_items / meter_readings 这类数组字段，由模型整表提取、
		// 前端逐行渲染，不存在「单独提取某个列」。放进规则面只会得到多个同名
		// 字段抢一条配置，且永远配不出值。
		if g := strings.TrimSpace(s.Group); g != "" && g != "overview" {
			continue
		}
		seen[name] = true
		f := byName[name]
		f.Name = name
		// 配置里没有这个字段 → 用目录底稿补齐 desc/rule/regex/type，
		// 避免「字段定义新增了、提取规则显示一片 —」。
		if d, ok := defaults[name]; ok {
			if strings.TrimSpace(f.Desc) == "" {
				f.Desc = d.Desc
			}
			if strings.TrimSpace(f.Rule) == "" {
				f.Rule = d.Rule
			}
			if strings.TrimSpace(f.Regex) == "" {
				f.Regex = d.Regex
			}
			if strings.TrimSpace(f.Type) == "" {
				f.Type = d.Type
			}
		}
		if strings.TrimSpace(f.Label) == "" {
			f.Label = strings.TrimSpace(s.Label)
		}
		if strings.TrimSpace(f.Type) == "" {
			f.Type = extractTypeOfSubDataType(s.DataType)
		}
		f.Enabled = s.Enabled
		aligned = append(aligned, f)
	}
	// 配置独有字段（只在「提取规则」里加过、没在「字段定义」里加）也要保留。
	//
	// 两种入口都应可用：
	//   · 字段定义加字段 → 这里由 subs 驱动，自动出现在规则面；
	//   · 提取规则直接加字段 → subs 里没有，但用户就是想让它被提取。
	// 若这里丢弃，后者会"配了但从不生效"，且没有任何提示——最难查的一类问题。
	for _, f := range cfg.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		aligned = append(aligned, f)
	}
	out := *cfg
	out.Fields = aligned
	return &out
}
