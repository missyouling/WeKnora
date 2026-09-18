package types

import "time"

// ExtractFieldConfig 可配置提取字段：名称（输出 JSON key）、描述、数据类型、提取规则。
type ExtractFieldConfig struct {
	Name    string `json:"name"`    // 输出 JSON 的字段 key，如“经营地址”
	Label   string `json:"label"`   // 界面显示名（可为空，缺省与 name 相同）
	Desc    string `json:"desc"`    // 字段描述，如“公司实际经营地址”
	Type    string `json:"type"`    // string | array | date | number
	Rule    string `json:"rule"`    // 提取规则/特殊说明
	Enabled bool   `json:"enabled"` // 是否参与提取
}

// KbExtractConfig 用户可配置的字段提取规则（知识库 × scope × 证照类型维度）。
type KbExtractConfig struct {
	ID              string               `gorm:"primaryKey" json:"id"`
	TenantID        int64                `gorm:"index" json:"tenant_id"`
	KnowledgeBaseID string               `gorm:"index" json:"knowledge_base_id"`
	Scope           string               `json:"scope"` // vehicle | driver | maintain
	CertType        string               `json:"cert_type"`
	Fields          []ExtractFieldConfig `gorm:"type:jsonb;serializer:json" json:"fields"`
	AdvancedEnabled bool                 `json:"advanced_enabled"`
	PromptTemplate  string               `gorm:"type:text" json:"prompt_template"`
	Version         int                  `json:"version"`
	Enabled         bool                 `json:"enabled"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	DeletedAt       *time.Time           `gorm:"index" json:"deleted_at"`
}

func (KbExtractConfig) TableName() string { return "knowledge_extract_configs" }

// KbExtractConfigVersion 配置版本快照（每次保存 version+1 落一版，保留最近 N 版）。
type KbExtractConfigVersion struct {
	ID              string               `gorm:"primaryKey" json:"id"`
	ConfigID        string               `gorm:"index" json:"config_id"`
	Version         int                  `json:"version"`
	Fields          []ExtractFieldConfig `gorm:"type:jsonb;serializer:json" json:"fields"`
	AdvancedEnabled bool                 `json:"advanced_enabled"`
	PromptTemplate  string               `gorm:"type:text" json:"prompt_template"`
	Remark          string               `gorm:"type:text" json:"remark"`
	CreatedAt       time.Time            `json:"created_at"`
}

func (KbExtractConfigVersion) TableName() string { return "knowledge_extract_config_versions" }
