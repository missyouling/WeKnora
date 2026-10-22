package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 文本字段细分为单行文本（text）与多行文本（textarea）—— 2026-10-22。
//
// 真机事故：备注/摘要等长文本字段只有一种 `text` 类型，详情抽屉按类型渲染成
// 单行 `<t-input>`，长内容（如「业务流水号:2026102215...」）被截断，
// 用户既看不全也录不全。此前靠硬编码字段名白名单（summary/摘要）兜底，
// 白名单之外的备注、说明一律单行，用户无从选择。
//
// 修复分两半：
//  1. 类型体系新增 `textarea`：字段定义下拉可选「多行文本」，抽屉按 dataType
//     渲染 `<t-textarea>`，列表按列类型切换换行/截断；
//  2. 本文件负责**存量自动迁移**：把已配为 `text` 的已知长文本字段升级为
//     `textarea`（幂等，每次启动执行一次），让存量数据立刻受益，无需用户
//     逐个字段手动改。
//
// 注意：`data_type` 只是渲染/骨架提示，不改变存储格式（长短文本都是 string），
// 因此迁移只改类型声明，不动业务数据。

// longTextFieldNames 已知长文本字段名（key 与中文名双路，比较时统一转小写）。
//
// 收录口径：语义上承载"段落级"内容的字段。短标识（合同编号/金额/日期/…）
// 绝不入表——它们必须是单行。
var longTextFieldNames = map[string]bool{
	// 英文 key
	"remark": true, "remarks": true, "summary": true, "abstract": true,
	"description": true, "desc": true, "comment": true, "comments": true,
	"notes": true, "note": true, "memo": true, "content": true,
	"detail": true, "details": true, "explanation": true, "reason": true,
	// 奖惩/通知类：处理决定与依据在字段目录（business_field_catalog）里已声明为
	// textarea，若这里不收录，存量分类会停在 text —— 同一个「处理决定」字段，
	// 新建分类是多行文本、老分类仍是单行，正是本需求要消灭的不一致。
	"measure": true, "basis": true,
	// 中文名（用户自建字段常直接拿中文名当 Name）
	"备注": true, "摘要": true, "内容摘要": true, "说明": true, "概述": true,
	"评论": true, "注释": true, "详情": true, "详细说明": true, "事由": true,
	"原因": true, "理由": true, "情况说明": true,
	"处理决定": true, "处罚决定": true, "依据": true, "处理依据": true, "处理措施": true,
}

// isLongTextFieldName 该字段名是否属于长文本（大小写不敏感）。
func isLongTextFieldName(name string) bool {
	return longTextFieldNames[strings.ToLower(strings.TrimSpace(name))]
}

// neverListFieldNames 「备注」这类语义上绝不可能是有序列表的长文本字段。
//
// 为什么单列一张表：历史种子把发票的 `remark` 错配成 `data_type=array`（渲染成
// t-tag-input，长备注被切成标签、无法录入段落），而迁移的通用口径是「只升 text、
// 尊重用户显式选过的其它类型」，否则会误伤真正需要数组的字段（如 `tags`、部分
// `summary`）。因此只对这几个「不可能是列表」的名字放开 array → textarea。
var neverListFieldNames = map[string]bool{
	"remark": true, "remarks": true, "备注": true,
}

func isNeverListFieldName(name string) bool {
	return neverListFieldNames[strings.ToLower(strings.TrimSpace(name))]
}

// upgradeLongTextSubs 把 subs 里已知长文本字段的 data_type 从 text 升级为 textarea。
//
// 语义要点：
//   - 只升级 `text`：用户已显式选过其它类型（number/date/array/items）时绝不覆盖
//     （把「摘要」改成数组可能是有意为之）；唯一例外是 isNeverListFieldName
//     （备注类）的 `array`——那是历史种子的错配，见 neverListFieldNames 注释；
//   - 幂等：第二次执行零改动；
//   - 返回新切片与改动条数；无改动时返回原切片（调用方可跳过错写库）。
func upgradeLongTextSubs(subs []types.FleetCategorySub) ([]types.FleetCategorySub, int) {
	if len(subs) == 0 {
		return subs, 0
	}
	changed := 0
	out := make([]types.FleetCategorySub, len(subs))
	copy(out, subs)
	for i := range out {
		if !isLongTextFieldName(out[i].Name) && !isLongTextFieldName(out[i].Label) {
			continue
		}
		if !isTextareaUpgradableType(out[i].DataType, out[i].Name, out[i].Label) {
			continue
		}
		out[i].DataType = "textarea"
		changed++
	}
	if changed == 0 {
		return subs, 0
	}
	return out, changed
}

// isTextareaUpgradableType 该字段的当前类型是否允许被升级为 textarea。
// text（及空值）总是允许；`array` 仅在名字属于「不可能是列表」的备注类时允许。
func isTextareaUpgradableType(dataType, name, label string) bool {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "", "text":
		return true
	case "array":
		return isNeverListFieldName(name) || isNeverListFieldName(label)
	default:
		return false
	}
}

// upgradeExtractConfigFieldTypes 同步升级提取配置里的字段类型（string/text → textarea）。
//
// 必须与 subs 一起改：否则「字段定义」显示多行文本、「提取规则」仍显示字符串，
// 六同步断裂。
//
// 注意两种方言都要认：subs 用 `text`，而提取配置用 `string`
// （normalizeExtractFields 缺省填 "string"，invoiceRuleTypeFor 也把 text 归一到
// "string"）。只匹配 `text` 会让配置侧一条都改不到——存量库里这个键根本没有
// `text` 这个值。判定口径与 upgradeLongTextSubs 一致：只升纯文本类，绝不覆盖
// 用户显式选过的 number/date/array/items。
func upgradeExtractConfigFieldTypes(fields []types.ExtractFieldConfig) ([]types.ExtractFieldConfig, int) {
	if len(fields) == 0 {
		return fields, 0
	}
	changed := 0
	out := make([]types.ExtractFieldConfig, len(fields))
	copy(out, fields)
	for i := range out {
		if !isPlainTextExtractType(out[i].Type) && !isNeverListArrayType(out[i].Type, out[i].Name, out[i].Label) {
			continue
		}
		if !isLongTextFieldName(out[i].Name) && !isLongTextFieldName(out[i].Label) {
			continue
		}
		out[i].Type = "textarea"
		changed++
	}
	if changed == 0 {
		return fields, 0
	}
	return out, changed
}

// isPlainTextExtractType 该提取配置类型是否「纯文本」方言（空值也按 string 兜底）。
func isPlainTextExtractType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "text", "string":
		return true
	default:
		return false
	}
}

// isNeverListArrayType 提取配置侧与 subs 侧口径一致：备注类字段若被历史种子
// 错配成 array，也一并升回 textarea（否则 subs 已是 textarea、配置仍是 array，
// 六同步再次断裂）。
func isNeverListArrayType(t, name, label string) bool {
	if strings.ToLower(strings.TrimSpace(t)) != "array" {
		return false
	}
	return isNeverListFieldName(name) || isNeverListFieldName(label)
}

// MigrateLongTextFieldsToTextarea 启动幂等迁移：把四个业务模块存量分类 subs 与
// 提取配置里的已知长文本字段从 text 升级为 textarea。
//
// 返回 (分类改动数, 配置改动数)，供日志与验收使用。
// 同步执行（与 CleanupOrphanExtractConfigs 同策略）：它只做 jsonb 局部更新，
// 真机实测毫秒级，不需要放后台。
func (h *BusinessExtractHandler) MigrateLongTextFieldsToTextarea(ctx context.Context) (int, int) {
	if h == nil || h.db == nil {
		return 0, 0
	}
	now := timeNowUTC()
	catChanged := 0

	// 1) fleet_categories.subs：按 scope 逐行读改写（与 countFleetCategoryRefs 同策略，
	//    全部读回 + Go 侧精确判定，避免在 SQL 里做 jsonb 路径更新带来的驱动差异）。
	//
	// 不要用 `CAST(subs AS TEXT) LIKE '%"data_type":"text"%'` 之类探针：Postgres 的
	// jsonb::text 会输出带空格的标准形态（`"data_type": "text"`），紧凑串永远匹配不到，
	// 迁移会静默零命中（真机首跑即踩坑）；sqlite 存的是紧凑 JSON 反而能匹配，
	// 于是单测绿、线上不生效。这类分级列数据量很小（四个 scope 合计几十行），
	// 直接全量读回在 Go 侧判定最稳。
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?",
			[]string{"contract", "regulation", "award_punish", "invoice"}).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "long-text migration: load categories failed: %v", err)
		return 0, 0
	}
	for i := range cats {
		upgraded, n := upgradeLongTextSubs(cats[i].Subs)
		if n == 0 {
			continue
		}
		// 结构体 Save（serializer:json）：与读取路径严格一致，且跨 Postgres/sqlite
		// 都稳定。禁止 gorm.Expr("?::jsonb") —— sqlite 不认该 cast，单测直接报错。
		cats[i].Subs = upgraded
		cats[i].UpdatedAt = now
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			logger.Warnf(ctx, "long-text migration: update category %s failed: %v", cats[i].ID, err)
			continue
		}
		catChanged += n
	}

	// 2) knowledge_extract_configs.fields：同步升级，保证提取规则与字段定义一致。
	//    同上：不做 SQL 侧的 JSON 文本探针（jsonb::text 空格差异会让探针形同虚设）。
	cfgChanged := 0
	var cfgs []types.KbExtractConfig
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?",
			[]string{"contract", "regulation", "award_punish", "invoice"}).
		Find(&cfgs).Error; err != nil {
		logger.Warnf(ctx, "long-text migration: load extract configs failed: %v", err)
		return catChanged, 0
	}
	for i := range cfgs {
		upgraded, n := upgradeExtractConfigFieldTypes(cfgs[i].Fields)
		if n == 0 {
			continue
		}
		// 同 subs：结构体 Save，读写形态一致（见 extract_config.go 保存路径的注释）
		cfgs[i].Fields = upgraded
		cfgs[i].UpdatedAt = now
		if err := h.db.WithContext(ctx).Save(&cfgs[i]).Error; err != nil {
			logger.Warnf(ctx, "long-text migration: update config %s failed: %v", cfgs[i].ID, err)
			continue
		}
		cfgChanged += n
	}

	if catChanged > 0 || cfgChanged > 0 {
		logger.Infof(ctx, "long-text migration done: subs=%d fields=%d", catChanged, cfgChanged)
	}
	return catChanged, cfgChanged
}
