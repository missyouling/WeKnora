package types

import (
	"database/sql/driver"
	"encoding/json"
)

// RecognitionRule 文档识别"包含判定"规则：命中即认定文件属于该类型（发票/合同）。
// MatchType 支持 keyword（Keywords 关键词，Logic 决定 AND/OR）与 regex（Regex 正则）。
type RecognitionRule struct {
	ID        string   `yaml:"id"        json:"id"`
	Name      string   `yaml:"name"      json:"name"`
	MatchType string   `yaml:"match_type" json:"match_type"` // keyword | regex
	Keywords  []string `yaml:"keywords"  json:"keywords,omitempty"`
	Logic     string   `yaml:"logic"     json:"logic,omitempty"` // AND | OR（多个关键词之间）
	Regex     string   `yaml:"regex"     json:"regex,omitempty"`
	Enabled   bool     `yaml:"enabled"   json:"enabled"`
}

// TypeClassifyRule 发票类型/合同类型归类规则：Pattern（关键词或正则）命中 → 归为 Type。
// IsRegex 为 true 时 Pattern 按正则匹配；否则按包含匹配。Priority 越小越优先。
type TypeClassifyRule struct {
	ID       string `yaml:"id"       json:"id"`
	Pattern  string `yaml:"pattern"  json:"pattern"`
	IsRegex  bool   `yaml:"is_regex" json:"is_regex"`
	Type     string `yaml:"type"     json:"type"`
	Priority int    `yaml:"priority" json:"priority"`
	Enabled  bool   `yaml:"enabled"  json:"enabled"`
}

// RecognitionConfig 知识库级文档识别配置（发票知识库/合同知识库各一份）。
type RecognitionConfig struct {
	// Enabled 总开关：关闭后所有规则不生效（回到纯模型判定）。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// IncludeRules 包含判定规则：任一规则命中（规则内按 AND/OR）→ 认定为该类型文档，
	// 用于"模型判非但规则命中"时的捞回，避免漏入库/误删。
	IncludeRules []RecognitionRule `yaml:"include_rules" json:"include_rules,omitempty"`
	// TypeRules 类型归类规则：按 Priority 顺序匹配，第一个命中生效。
	// 发票默认预置内置关键字（通行费→普通发票 等）；合同默认空。
	TypeRules []TypeClassifyRule `yaml:"type_rules" json:"type_rules,omitempty"`
	// Types 分类列表（发票/合同的类型枚举，支持增删改查）。类型归类规则的目标
	// 从该列表选择；发票默认 5 枚举，合同为空时自动从现有合同类型加载。
	Types []string `yaml:"types" json:"types,omitempty"`
	// Measures 措施库（仅奖惩知识库使用）：抽屉编辑措施字段多选时加载，
	// 支持按奖惩类型分组（处罚/奖励/其它奖惩），可增删改查。
	Measures []MeasureItem `yaml:"measures" json:"measures,omitempty"`
}

// MeasureItem 奖惩措施条目：名称 + 所属奖惩类型。
type MeasureItem struct {
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type,omitempty"`
}

// Value implements driver.Valuer for RecognitionConfig.
func (r RecognitionConfig) Value() (driver.Value, error) { return json.Marshal(r) }

// Scan implements sql.Scanner for RecognitionConfig.
func (r *RecognitionConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, r)
}

// DefaultInvoiceRecognitionConfig returns the built-in invoice recognition
// defaults: a common include-rule (捞回兜底), the keyword type-classification
// rules mirroring NormalizeInvoiceTypeFromTicket, and the 5-type enum.
func DefaultInvoiceRecognitionConfig() *RecognitionConfig {
	return &RecognitionConfig{
		Enabled: true,
		IncludeRules: []RecognitionRule{
			{ID: "inv-include", Name: "含发票关键字", MatchType: "keyword",
				Keywords: []string{"发票", "发票号码", "价税合计"}, Logic: "OR", Enabled: true},
		},
		TypeRules: []TypeClassifyRule{
			{ID: "inv-tongxing", Pattern: "通行费", Type: "普通发票", Priority: 1, Enabled: true},
			{ID: "inv-zhuan", Pattern: "专用", Type: "专用发票", Priority: 2, Enabled: true},
			{ID: "inv-yiliao", Pattern: "医疗", Type: "医疗收据", Priority: 3, Enabled: true},
			{ID: "inv-caizheng", Pattern: "财政", Type: "财政收据", Priority: 4, Enabled: true},
			{ID: "inv-putong", Pattern: "普通", Type: "普通发票", Priority: 5, Enabled: true},
		},
		Types: []string{"专用发票", "普通发票", "医疗收据", "财政收据", "其它票据"},
	}
}

// DefaultContractRecognitionConfig returns the built-in contract recognition
// defaults: a common include-rule (捞回兜底). Type rules and the type list are
// empty — the contract type list auto-loads from existing contracts.
func DefaultContractRecognitionConfig() *RecognitionConfig {
	return &RecognitionConfig{
		Enabled: true,
		IncludeRules: []RecognitionRule{
			{ID: "ctr-include", Name: "含合同关键字", MatchType: "keyword",
				Keywords: []string{"合同", "协议", "甲方", "乙方"}, Logic: "OR", Enabled: true},
		},
		TypeRules: nil,
		Types:     nil,
	}
}

// DefaultRegulationRecognitionConfig returns the built-in regulation
// recognition defaults: a common include-rule (捞回兜底), keyword type
// classification rules and the 6-type enum.
func DefaultRegulationRecognitionConfig() *RecognitionConfig {
	return &RecognitionConfig{
		Enabled: true,
		IncludeRules: []RecognitionRule{
			{ID: "reg-include", Name: "含制度关键字", MatchType: "keyword",
				Keywords: []string{"制度", "管理办法", "管理规定", "第一条", "本制度"}, Logic: "OR", Enabled: true},
		},
		TypeRules: []TypeClassifyRule{
			{ID: "reg-renshi", Pattern: "人事|员工|考勤|绩效|薪酬|招聘|培训|转岗|任职|请假|人力", Type: "人事管理", Priority: 1, Enabled: true},
			{ID: "reg-caiwu", Pattern: "财务|报销|预算|资金|付款", Type: "财务管理", Priority: 2, Enabled: true},
			{ID: "reg-shengchan", Pattern: "生产|车间|工艺|质量|设备|作业", Type: "生产管理", Priority: 3, Enabled: true},
			{ID: "reg-xingzheng", Pattern: "行政|办公|档案|会议|印章|公文|接待", Type: "行政管理", Priority: 4, Enabled: true},
			{ID: "reg-anquan", Pattern: "安全|消防|环保|应急|危化", Type: "安全管理", Priority: 5, Enabled: true},
		},
		Types: []string{"人事管理", "财务管理", "生产管理", "行政管理", "安全管理", "其它制度"},
	}
}

// DefaultAwardPunishRecognitionConfig returns the built-in recognition rules for
// an award/punish knowledge base: one include rule plus keyword type rules that
// map notice content onto the 4-type enum (处罚/奖励/通报/其它奖惩).
func DefaultAwardPunishRecognitionConfig() *RecognitionConfig {
	return &RecognitionConfig{
		Enabled: true,
		IncludeRules: []RecognitionRule{
			{ID: "ap-include", Name: "含奖惩关键字", MatchType: "keyword",
				Keywords: []string{"奖惩", "处罚", "奖励", "通报", "罚款", "警告", "表彰", "嘉奖", "处分"}, Logic: "OR", Enabled: true},
		},
		TypeRules: []TypeClassifyRule{
			{ID: "ap-chufa", Pattern: "处罚|罚款|警告|处分|批评|扣款|记过|通报批评", Type: "处罚", Priority: 1, Enabled: true},
			{ID: "ap-jiangli", Pattern: "奖励|表彰|嘉奖|表扬|奖金|评优|晋级", Type: "奖励", Priority: 2, Enabled: true},
			{ID: "ap-tongbao", Pattern: "通报", Type: "通报", Priority: 3, Enabled: true},
		},
		Types: []string{"处罚", "奖励", "通报", "其它奖惩"},
		Measures: []MeasureItem{
			{Name: "书面警告", Type: "处罚"}, {Name: "经济处罚", Type: "处罚"},
			{Name: "通报批评", Type: "处罚"}, {Name: "记过", Type: "处罚"},
			{Name: "扣款", Type: "处罚"}, {Name: "取消评优资格", Type: "处罚"},
			{Name: "降职", Type: "处罚"}, {Name: "停职检查", Type: "处罚"},
			{Name: "通报表扬", Type: "奖励"}, {Name: "嘉奖", Type: "奖励"},
			{Name: "记功", Type: "奖励"}, {Name: "奖金", Type: "奖励"},
			{Name: "晋升", Type: "奖励"}, {Name: "评优评先", Type: "奖励"},
		},
	}
}

