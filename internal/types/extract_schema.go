package types

// 通用提取器的 Schema 契约（蓝图 P2-1）。
//
// 目的：把「发票专属」的四个能力（智能字段生成 / 智能规则生成 / 后置正则补洞 /
// 沙箱测试）从结构体硬编码里解出来，变成按 scope 注册的声明式数据。新增业务
// 模块只注册一份 ScopeSchema，四个能力零代码改动即可复用。
//
// 设计红线：
//   - 本文件只放**声明**，不放行为（行为在 service 的注册表与补洞实现里）；
//   - RecordFactory / ItemField / DynamicBucket 等运行期字段不参与 JSON 序列化，
//     它们是 Go 反射协作字段，不是接口数据。

// FieldType 字段的值类型（与前端 FieldMeta.dataType 同一套口径）。
type FieldType string

const (
	FieldTypeText   FieldType = "text"
	FieldTypeNumber FieldType = "number"
	FieldTypeDate   FieldType = "date"
	FieldTypeArray  FieldType = "array"
)

// FieldDef 一个标准契约字段的声明。
type FieldDef struct {
	Key     string    `json:"key"`            // 契约键（= 结构体 json tag / 前端配置名经 canonical 归一后命中）
	Label   string    `json:"label"`          // 中文显示名（喂 AI 字典与界面）
	Type    FieldType `json:"type"`           // text | number | date | array
	Hint    string    `json:"hint,omitempty"` // 喂 LLM 的语义提示
	Aliases []string  `json:"aliases,omitempty"`
	// Regex 内置兜底正则（后置补洞的 builtin 来源；用户配置优先于它）。
	// 仅标准字段使用；自定义字段的正则来自 KbExtractConfig.Fields[].Regex。
	Regex string `json:"regex,omitempty"`
	// SubFields 明细子列定义（Type == array 时；发票 items 用）。
	SubFields []FieldDef `json:"sub_fields,omitempty"`
}

// ScopeSchema 一个业务 scope 的完整提取契约。
type ScopeSchema struct {
	Scope          string     `json:"scope"`          // invoice | contract | regulation | notice | award_punish | utility | solar
	Kind           string     `json:"kind"`           // invoice（票证数组） | document（单记录）
	CategoryLabel  string     `json:"category_label"` // 「发票类型 / 合同类型 / 制度名称」
	StandardFields []FieldDef `json:"standard_fields"`

	// DynamicBucket 记录级扩展字段落点：结构体中承载自定义字段的 map 的
	// **Go 字段名**（发票 = "Fields"，json tag "fields"）。空 = 该 scope
	// 无扩展桶（map 视图沙箱天然就是桶）。
	DynamicBucket string `json:"dynamic_bucket,omitempty"`
	// ItemBucket 明细数组落点：结构体中明细切片的 json tag（发票 = "items"）。
	// 空 = 该 scope 无明细层级。
	ItemBucket string `json:"item_bucket,omitempty"`
	// ItemField 明细行里承载自定义子列的 map 的 **Go 字段名**（发票 = "Extra"，
	// 其 json tag 是 "-"，只能按字段名反射）。
	ItemField string `json:"-"`

	// RegexPatchable 该 scope 是否接入 AI 建议正则护栏与标准字段补洞。
	// false 的 scope 仍享有 KbExtractConfig 级别的 required 补洞（通用线），
	// 只是没有「标准字段全集」可补。
	RegexPatchable bool `json:"regex_patchable"`

	// Deterministic 启用的确定性前置规则标识（"line"|"md_table"|"flat"）。
	// 当前仅发票声明；沙箱与正式提取按同一份清单执行。
	Deterministic []string `json:"deterministic,omitempty"`

	// RecordFactory 生成一条空记录（结构体或 map）。供未来新增 scope 时
	// 做空结果初始化；当前发票注册项用它反射校验标准字段键。
	RecordFactory func() any `json:"-"`
}

// ScopeRegistry scope Schema 注册表契约。
type ScopeRegistry interface {
	Register(s ScopeSchema)
	Get(scope string) (ScopeSchema, bool)
	List() []ScopeSchema
}
