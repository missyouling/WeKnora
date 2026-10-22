package service

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 业务字段目录（合同 / 制度 / 通知公告）。
//
// 这里是「字段定义」的唯一权威源：字段顺序、中文名、描述、默认自然语言规则、
// 默认 RE2 后置正则、以及高级模式 System Prompt 模板全部在此声明，下发给：
//   - ScopeSchema（AI 字段建议字典 + 后置正则补洞面）
//   - 提取规则设置页（未保存配置时现场合成默认底稿，并幂等回填到 DB）
//   - 正式提取（字段口径注入 Prompt）
//
// 键名一律 snake_case，且必须等于结构体 json tag —— 否则 ValidateGenericExtraction
// 的正则补洞按字段名匹配不到记录字段，规则会静默失效（历史遗留：subs 用 camelCase
// 而落库用 snake_case，补洞从来没生效过）。
type BusinessFieldSpec struct {
	Key     string // 契约键（= 结构体 json tag）
	Label   string // 中文显示名
	Desc    string // 字段描述（喂模型 + 设置页展示）
	Rule    string // 默认自然语言提取规则（进 Prompt）
	Regex   string // 默认后置 RE2 正则（尽力而为，可为空）
	Type    string // text | textarea | number | date | array
	Builtin bool   // 系统内置不可删（分类主轴 / extract_status / tags）
	// Derived 派生字段：由其它字段计算，不要求模型提取，编辑抽屉只读展示。
	Derived bool
}

// BusinessFieldCatalog 一个业务 scope 的完整字段目录。
type BusinessFieldCatalog struct {
	Scope          string
	CategoryLabel  string
	Standard       []BusinessFieldSpec
	Extended       []BusinessFieldSpec
	PromptTemplate string
}

// AllFields 返回标准 + 扩展的完整字段序列（顺序即界面顺序）。
func (c BusinessFieldCatalog) AllFields() []BusinessFieldSpec {
	out := make([]BusinessFieldSpec, 0, len(c.Standard)+len(c.Extended))
	out = append(out, c.Standard...)
	out = append(out, c.Extended...)
	return out
}

// 常用正则片段（Go RE2：不支持环视与反向引用，这里只用字符类与量词）。
const (
	reDateLoose = `20\d{2}[-年/.]\d{1,2}[-月/.]\d{1,2}日?`
	reTaxNo     = `[0-9A-HJ-NPQRTUWXY]{18}`
	rePhone     = `(?:\+?86[-\s]?)?(?:0\d{2,3}[-\s]?\d{7,8}|1[3-9]\d{9})`
	reAmount    = `[0-9][0-9,，]*(?:\.[0-9]{1,2})?`
	reTextStop  = `[^\n，,；;]{2,40}`
)

// BusinessFieldCatalogs 三个 document 业务的字段目录。
func BusinessFieldCatalogs() []BusinessFieldCatalog {
	return []BusinessFieldCatalog{contractCatalog(), regulationCatalog(), noticeCatalog()}
}

// BusinessFieldCatalogOf 按 scope 取目录；notice 是 award_punish 的对外别名，
// 两者共用同一份目录（彻底消除双轨分裂）。
func BusinessFieldCatalogOf(scope string) (BusinessFieldCatalog, bool) {
	s := strings.TrimSpace(scope)
	if s == "notice" {
		s = "award_punish"
	}
	for _, c := range BusinessFieldCatalogs() {
		if c.Scope == s {
			return c, true
		}
	}
	return BusinessFieldCatalog{}, false
}

func contractCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "contract",
		CategoryLabel: "合同类型",
		Standard: []BusinessFieldSpec{
			{Key: "contract_no", Label: "合同编号", Type: "text",
				Desc:  "合同编号 / 协议编号",
				Rule:  "提取合同封面或页眉的「合同编号 / 协议编号 / 合同号」；没有编号时留空，不要编造。",
				Regex: `(?:合同编号|协议编号|合同号|编\s*号)\s*[:：]?\s*([A-Za-z0-9][A-Za-z0-9\-_/（）()]{2,39})`},
			{Key: "contract_name", Label: "合同名称", Type: "text",
				Desc:  "合同标题",
				Rule:  "提取合同标题（通常在首页最上方，如「厂房租赁协议书」「设备采购合同」）；标题含书名号时去掉书名号。",
				Regex: `(?:合同名称|协议名称|合同标题|项目名称)\s*[:：]?\s*([^\n]{2,60})`},
			{Key: "contract_type", Label: "合同类型", Type: "text", Builtin: true,
				Desc:  "合同归档类型",
				Rule:  "按合同名称与正文判断：含「采购」→采购合同；含「销售/售」→销售合同；含「租赁/租」→租赁合同；含「服务」→服务合同；含「技术」→技术合同；含「运输/货运」→运输合同；含「借款/贷款」→借款合同；含「保密/NDA」→保密协议；都不含→其它合同。",
				Regex: `(?:合同(?:类)?型|合同类别)\s*[:：]?\s*(采购合同|销售合同|租赁合同|服务合同|技术合同|运输合同|借款合同|保密协议|其它合同)`},
			{Key: "party_a", Label: "甲方", Type: "text",
				Desc:  "甲方（出租方/委托方/采购方）名称",
				Rule:  "提取「甲方」一方的完整企业名称；若合同用「出租方/委托方/采购方」表述，同样归入本字段。",
				Regex: `(?:甲\s*方|出租方|委托方|采购方)\s*[:：]?\s*` + reTextStop},
			{Key: "party_b", Label: "乙方", Type: "text",
				Desc:  "乙方（承租方/受托方/供应方）名称",
				Rule:  "提取「乙方」一方的完整企业名称；若合同用「承租方/受托方/供应方」表述，同样归入本字段。",
				Regex: `(?:乙\s*方|承租方|受托方|供应方)\s*[:：]?\s*` + reTextStop},
			{Key: "sign_date", Label: "签订日期", Type: "date",
				Desc:  "合同签订/签署日期",
				Rule:  "提取合同尾部或首页的「签订日期 / 签署日期」，统一为 YYYY-MM-DD；只有年月时补 01 日。",
				Regex: `(?:签订日期|签署日期|签约日期|签订时间|签署时间)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "expire_date", Label: "到期日期", Type: "date",
				Desc:  "合同到期/终止日期",
				Rule:  "提取「到期日期 / 合同终止日期 / 有效期至」；租赁合同优先取「租赁期限自…至…」的结束日期，统一为 YYYY-MM-DD。",
				Regex: `(?:到期日期|终止日期|有效期至|合同期限至|租赁期限至)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "amount", Label: "合同金额", Type: "number",
				Desc:  "合同总金额（元）",
				Rule:  "提取「合同金额 / 合同总价 / 价款合计」的数字，单位元，去掉千分位与货币符号；大小写金额不一致时以大写为准。",
				Regex: `(?:合同(?:总)?金额|总金额|金额合计|价款合计|合同总价)\s*[:：]?\s*[¥￥]?\s*(` + reAmount + `)`},
			{Key: "extract_status", Label: "提取状态", Type: "text", Builtin: true,
				Desc: "提取状态（系统字段）",
				Rule: "由系统写入，不需要从原文提取。"},
			{Key: "performance_status", Label: "履约状态", Type: "text", Derived: true,
				Desc: "履约状态（由到期日期派生）",
				Rule: "由系统按到期日期计算（履行中 / 即将到期 / 已超期），不需要从原文提取。"},
			{Key: "tags", Label: "标签", Type: "array", Builtin: true,
				Desc: "记录标签（系统字段）",
				Rule: "由用户维护，不需要从原文提取。"},
		},
		Extended: []BusinessFieldSpec{
			{Key: "party_a_contact", Label: "甲方经办人", Type: "text",
				Desc:  "甲方联系人 / 经办人",
				Rule:  "提取甲方一侧的「经办人 / 联系人 / 委托代理人」姓名。",
				Regex: `(?:甲\s*方)?(?:经办人|联系人|委托代理人)\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "party_a_tax_no", Label: "甲方税号", Type: "text",
				Desc:  "甲方统一社会信用代码 / 纳税人识别号",
				Rule:  "提取甲方名称附近或发票信息栏的 18 位统一社会信用代码。",
				Regex: `(?:统一社会信用代码|纳税人识别号|税\s*号)\s*[:：]?\s*(` + reTaxNo + `)`},
			{Key: "party_b_contact", Label: "乙方经办人", Type: "text",
				Desc:  "乙方联系人 / 经办人",
				Rule:  "提取乙方一侧的「经办人 / 联系人 / 委托代理人」姓名。",
				Regex: `(?:乙\s*方)?(?:经办人|联系人|委托代理人)\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "party_b_tax_no", Label: "乙方税号", Type: "text",
				Desc:  "乙方统一社会信用代码 / 纳税人识别号",
				Rule:  "提取乙方名称附近的 18 位统一社会信用代码。",
				Regex: `(?:统一社会信用代码|纳税人识别号|税\s*号)\s*[:：]?\s*(` + reTaxNo + `)`},
			{Key: "party_a_address", Label: "甲方地址", Type: "text",
				Desc:  "甲方住所 / 注册地址",
				Rule:  "提取甲方的「地址 / 住所 / 注册地址」。",
				Regex: `(?:地\s*址|住\s*所|注册地址)\s*[:：]?\s*([^\n，,；;]{4,60})`},
			{Key: "party_a_phone", Label: "甲方电话", Type: "text",
				Desc:  "甲方联系电话",
				Rule:  "提取甲方的「电话 / 联系电话」。",
				Regex: `(?:电\s*话|联系电话|Tel)\s*[:：]?\s*(` + rePhone + `)`},
			{Key: "party_a_bank", Label: "甲方开户行", Type: "text",
				Desc:  "甲方开户银行",
				Rule:  "提取甲方的「开户行 / 开户银行」。",
				Regex: `(?:开户行|开户银行)\s*[:：]?\s*([^\n，,；;]{2,40})`},
			{Key: "party_a_account", Label: "甲方账号", Type: "text",
				Desc:  "甲方银行账号",
				Rule:  "提取甲方的「账号 / 银行账号」。",
				Regex: `(?:账\s*号|银行账号|开户账号)\s*[:：]?\s*(\d{8,32})`},
			{Key: "party_b_address", Label: "乙方地址", Type: "text",
				Desc:  "乙方住所 / 注册地址",
				Rule:  "提取乙方的「地址 / 住所 / 注册地址」。",
				Regex: `(?:地\s*址|住\s*所|注册地址)\s*[:：]?\s*([^\n，,；;]{4,60})`},
			{Key: "party_b_phone", Label: "乙方电话", Type: "text",
				Desc:  "乙方联系电话",
				Rule:  "提取乙方的「电话 / 联系电话」。",
				Regex: `(?:电\s*话|联系电话|Tel)\s*[:：]?\s*(` + rePhone + `)`},
			{Key: "party_b_bank", Label: "乙方开户行", Type: "text",
				Desc:  "乙方开户银行",
				Rule:  "提取乙方的「开户行 / 开户银行」。",
				Regex: `(?:开户行|开户银行)\s*[:：]?\s*([^\n，,；;]{2,40})`},
			{Key: "party_b_account", Label: "乙方账号", Type: "text",
				Desc:  "乙方银行账号",
				Rule:  "提取乙方的「账号 / 银行账号」。",
				Regex: `(?:账\s*号|银行账号|开户账号)\s*[:：]?\s*(\d{8,32})`},
			{Key: "payment_method", Label: "付款方式", Type: "text",
				Desc:  "付款方式",
				Rule:  "从「一次性 / 分期 / 按进度」中选择，与原文最接近的一项；无法判断时留空。",
				Regex: `(?:付款方式|支付方式|结算方式)\s*[:：]?\s*(一次性|分期|按进度|按月|按季)`},
			{Key: "sign_location", Label: "签订地点", Type: "text",
				Desc:  "合同签订地点",
				Rule:  "提取合同尾部的「签订地点 / 签署地点」。",
				Regex: `(?:签订地点|签署地点|签订地)\s*[:：]?\s*` + reTextStop},
			{Key: "summary", Label: "摘要", Type: "textarea",
				Desc:  "合同摘要（复用知识库文档摘要）",
				Rule:  "原文自带「摘要 / 内容摘要」时提取该段；否则留空，系统会用知识库已生成的文档摘要回填。",
				Regex: `(?:摘\s*要|内容摘要)\s*[:：]?\s*([^\n]{5,200})`},
		},
		PromptTemplate: `你是一位合同档案管理员，需要从合同全文中提取结构化字段。

【任务】
阅读 <document> 中的合同文本，输出严格 JSON。一份文件只对应一条合同记录。

【字段与规则】
{{fields_schema}}

【字段口径】
1. 只输出 JSON 中列出的字段；原文没有的字段填空字符串（数字填 0）。
2. 日期一律 YYYY-MM-DD；金额单位为元，去掉千分位与货币符号。
3. 甲方/乙方以合同中「甲方」「乙方」的表述为准；只有单方主体时另一方留空。
4. 签订日期、甲乙方、金额常分散在不同页：签订日期通常在尾页盖章处，必须在全文范围内查找。
5. 合同编号、签订日期等不确定时宁可留空，严禁编造。

【输出】
{"kind": "contract", "contracts": [ { ...字段... } ]}`,
	}
}

func regulationCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "regulation",
		CategoryLabel: "制度类型",
		Standard: []BusinessFieldSpec{
			{Key: "reg_no", Label: "制度编号", Type: "text",
				Desc:  "制度文件编号",
				Rule:  "提取制度封面或页眉的「制度编号 / 文件编号」，如 AD-028；无则留空。",
				Regex: `(?:制度编号|文件编号|编\s*号)\s*[:：]?\s*([A-Za-z]{1,6}[-－]?\d{2,8})`},
			{Key: "reg_name", Label: "制度名称", Type: "text",
				Desc:  "制度标题",
				Rule:  "提取制度标题（首页标题行），去掉书名号与版本后缀。",
				Regex: `(?:制度名称|制度标题|文件名称|文件标题)\s*[:：]?\s*([^\n]{2,60})`},
			{Key: "reg_type", Label: "制度类型", Type: "text", Builtin: true,
				Desc:  "制度归档类型",
				Rule:  "按制度名称判断：含「人事/员工/考勤/招聘」→人事管理；含「财务/费用/报销/资金」→财务管理；含「安全/消防/应急」→安全管理；含「生产/工艺/设备」→生产管理；含「行政/办公/车辆」→行政管理；都不含→其它制度。",
				Regex: `(?:制度(?:类)?型|制度类别|文件类别)\s*[:：]?\s*(人事管理|财务管理|安全管理|生产管理|行政管理|其它制度)`},
			{Key: "dept", Label: "编制部门", Type: "text",
				Desc:  "制度编制/归口部门",
				Rule:  "提取「编制部门 / 归口部门 / 主管部门」。",
				Regex: `(?:编制部门|归口部门|主管部门|起草部门)\s*[:：]?\s*([\p{Han}]{2,20})`},
			{Key: "version", Label: "版本", Type: "text",
				Desc:  "制度版本号",
				Rule:  "提取「版本 / 版本号 / 修订版」，如 A/0、V2.0。",
				Regex: `(?:版\s*本(?:号)?|修订版)\s*[:：]?\s*([A-Za-z]?\d?(?:\.\d+)?(?:/\d+)?)`},
			{Key: "issue_date", Label: "编制日期", Type: "date",
				Desc:  "制度编制/发布/生效日期",
				Rule:  "提取「编制日期 / 发布日期」，统一为 YYYY-MM-DD。",
				Regex: `(?:编制日期|发布日期|颁布日期)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "page_count", Label: "页数", Type: "text",
				Desc:  "制度文件页数",
				Rule:  "提取「共 N 页 / 第 N 页 共 M 页」中的总页数；无法判断时留空。",
				Regex: `共\s*(\d{1,3})\s*页`},
			{Key: "modify_count", Label: "修改次数", Type: "text",
				Desc:  "制度修订次数",
				Rule:  "提取「修改次数 / 修订次数」，如「第 2 次修改」取 2。",
				Regex: `(?:修改|修订)次数\s*[:：]?\s*(\d{1,2})`},
			{Key: "scope", Label: "适用范围", Type: "text",
				Desc:  "制度适用范围",
				Rule:  "提取「适用范围 / 适用对象」原文描述。",
				Regex: `(?:适用范围|适用对象)\s*[:：]?\s*([^\n]{2,60})`},
			{Key: "effective_date", Label: "生效日期", Type: "date",
				Desc:  "制度生效日期",
				Rule:  "提取「生效日期 / 自…起施行」，统一为 YYYY-MM-DD。",
				Regex: `(?:生效日期|施行日期)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "confidentiality", Label: "密级", Type: "text",
				Desc:  "制度密级",
				Rule:  "提取「密级」：公开 / 内部 / 秘密 / 机密；无标注时留空。",
				Regex: `密\s*级\s*[:：]?\s*(公开|内部|秘密|机密|绝密)`},
			{Key: "remark", Label: "备注", Type: "textarea",
				Desc:  "补充说明",
				Rule:  "原文中的其它重要说明，不超过 50 字；无则留空。",
				Regex: `备\s*注\s*[:：]?\s*([^\n]{2,50})`},
			{Key: "compiled_by", Label: "编制人", Type: "text",
				Desc:  "制度编制人",
				Rule:  "提取「编制 / 编制人 / 起草人」姓名。",
				Regex: `(?:编\s*制|起草人)\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "reviewed_by", Label: "审核人", Type: "text",
				Desc:  "制度审核人",
				Rule:  "提取「审核 / 审核人」姓名。",
				Regex: `审\s*核\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "approved_by", Label: "批准人", Type: "text",
				Desc:  "制度批准人",
				Rule:  "提取「批准 / 批准人」姓名。",
				Regex: `批\s*准\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "summary", Label: "摘要", Type: "textarea",
				Desc:  "制度摘要（复用知识库文档摘要）",
				Rule:  "原文自带「摘要」时提取；否则留空，系统会用知识库摘要回填。",
				Regex: `(?:摘\s*要|内容摘要)\s*[:：]?\s*([^\n]{5,200})`},
			{Key: "extract_status", Label: "提取状态", Type: "text", Builtin: true,
				Desc: "提取状态（系统字段）", Rule: "由系统写入，不需要从原文提取。"},
			{Key: "tags", Label: "标签", Type: "array", Builtin: true,
				Desc: "记录标签（系统字段）", Rule: "由用户维护，不需要从原文提取。"},
		},
		Extended: []BusinessFieldSpec{
			{Key: "attachments", Label: "附件", Type: "text",
				Desc:  "制度附件清单",
				Rule:  "提取文末「附件」列出的名称，多个用顿号分隔；无则留空。",
				Regex: `附\s*件\s*[:：]?\s*([^\n]{2,100})`},
		},
		PromptTemplate: `你是一位企业制度档案管理员，需要从制度全文中提取结构化字段。

【任务】
阅读 <document> 中的制度文本，输出严格 JSON。一份文件只对应一条制度记录。

【字段与规则】
{{fields_schema}}

【字段口径】
1. 只输出 JSON 中列出的字段；原文没有的填空字符串（数字填 0）。
2. 日期统一 YYYY-MM-DD。
3. 制度编号、版本、编制/审核/批准人通常在封面或审批栏，可能分处不同页，需全文查找。
4. 不确定的字段宁可留空，严禁编造。

【输出】
{"kind": "regulation", "regulations": [ { ...字段... } ]}`,
	}
}

func noticeCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "award_punish",
		CategoryLabel: "公告类型",
		Standard: []BusinessFieldSpec{
			{Key: "ap_no", Label: "公告编号", Type: "text",
				Desc:  "发文字号 / 公告编号",
				Rule:  "提取红头下方的发文字号，如「星达行政字〔2026〕09号」；多行折断时拼成一行；无则留空。",
				Regex: `([\p{Han}]{2,12}(?:字|发)?\s*[〔\[（(]?\s*20\d{2}\s*[〕\]）)]?\s*\d{1,4}\s*号)`},
			{Key: "ap_title", Label: "公告标题", Type: "text",
				Desc:  "公告/通知/通报标题",
				Rule:  "提取发文字号下方的标题（可能跨两行，需拼成完整一句）。",
				Regex: `(?:标\s*题|事\s*由|公告名称|通知名称|通报名称)\s*[:：]?\s*([^\n]{2,60})`},
			{Key: "ap_type", Label: "公告类型", Type: "text", Builtin: true,
				Desc:  "公告归档类型",
				Rule:  "按标题判断：含「通报」→通报；含「决定」→决定；含「公告」→公告；含「通知」→通知；都不含→其它通知。",
				Regex: `(?:公告(?:类)?型|公告类别|文件类别)\s*[:：]?\s*(通报|决定|公告|通知|其它通知)`},
			{Key: "issue_dept", Label: "发文部门", Type: "text",
				Desc:  "发文/发布单位",
				Rule:  "提取落款单位或红头单位（如「重庆星达铜业有限公司」「行政部」）；有多个时取落款。",
				Regex: `([\p{Han}]{2,20}(?:有限公司|有限责任公司|集团|部|中心|办公室))\s*\n?\s*20\d{2}\s*年`},
			{Key: "sign_date", Label: "发布日期", Type: "date",
				Desc:  "发布日期 / 签发日期",
				Rule:  "提取落款日期（如「2026 年 3 月 20 日」），统一为 YYYY-MM-DD。",
				Regex: `(20\d{2}\s*年\s*\d{1,2}\s*月\s*\d{1,2}\s*日)`},
			{Key: "effective_date", Label: "生效日期", Type: "date",
				Desc:  "生效/施行日期",
				Rule:  "提取「自…起施行 / 自发文之日起执行 / 生效日期」；仅写「自发文之日起」时取发布日期。",
				Regex: `(?:生效日期|施行日期)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "scope", Label: "受文对象", Type: "text",
				Desc:  "受文对象 / 适用范围",
				Rule:  "提取主送对象（如「公司各部门、全体员工」）。",
				Regex: `([^\n]{2,40}(?:各部门|全体员工|各单位)[^\n]{0,20})`},
			{Key: "extract_status", Label: "提取状态", Type: "text", Builtin: true,
				Desc: "提取状态（系统字段）", Rule: "由系统写入，不需要从原文提取。"},
			{Key: "tags", Label: "标签", Type: "array", Builtin: true,
				Desc: "记录标签（系统字段）", Rule: "由用户维护，不需要从原文提取。"},
		},
		Extended: []BusinessFieldSpec{
			{Key: "signer", Label: "签发人", Type: "text",
				Desc:  "文件签发人",
				Rule:  "提取发文字号右侧或落款处的「签发人」姓名。",
				Regex: `签\s*发\s*人\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "contact", Label: "联系人", Type: "text",
				Desc:  "文件联系人",
				Rule:  "提取文末「联系人 / 联系方式」中的姓名。",
				Regex: `联\s*系\s*人\s*[:：]?\s*([\p{Han}]{2,12})`},
			{Key: "contact_phone", Label: "联系电话", Type: "text",
				Desc:  "文件联系电话",
				Rule:  "提取文末联系电话；公司总机与个人手机同时出现时取个人手机。",
				Regex: `(?:联系电话|电\s*话)\s*[:：]?\s*(` + rePhone + `)`},
			{Key: "urgency", Label: "紧急程度", Type: "text",
				Desc:  "紧急程度 / 重要程度",
				Rule:  "提取「特急 / 加急 / 平急 / 重要 / 一般」等标注；无标注时留空。",
				Regex: `(?:紧急程度|重要程度)\s*[:：]?\s*(特急|加急|平急|紧急|重要|一般)`},
			{Key: "valid_until", Label: "有效期限", Type: "date",
				Desc:  "文件有效期截止日期",
				Rule:  "提取「有效期至 / 本通知有效期」；长期有效时留空。",
				Regex: `(?:有效期至|有效期限至)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "attachments", Label: "附件名称", Type: "text",
				Desc:  "附件清单",
				Rule:  "提取文末「附件：」列出的名称，多个用顿号分隔；无则留空。",
				Regex: `附\s*件\s*[:：]?\s*([^\n]{2,100})`},
			{Key: "person", Label: "当事人", Type: "text",
				Desc:  "通报/决定涉及的当事人",
				Rule:  "通报、处理决定类文件中的当事人姓名，多个用顿号分隔。",
				Regex: `(?:当事人|责任人)\s*[:：]?\s*([\p{Han}、，,\s]{2,40})`},
			{Key: "measure", Label: "处理决定", Type: "textarea",
				Desc:  "处理/奖惩决定",
				Rule:  "提取「处理决定 / 处罚决定」原文（如「书面警告」「赔偿 6,798.84 元」）；无则留空。",
				Regex: `(?:处理决定|处罚决定|处理意见|奖惩决定)\s*[:：]?\s*([^\n]{2,80})`},
			{Key: "basis", Label: "依据", Type: "textarea",
				Desc:  "处理依据",
				Rule:  "提取「依据 / 根据」后面的制度或法规名称及条款，原文照录，不超过 80 字。",
				Regex: `(?:依\s*据|根\s*据)\s*[:：]?\s*([^\n]{2,80})`},
			{Key: "summary", Label: "摘要", Type: "textarea",
				Desc:  "公告摘要（复用知识库文档摘要）",
				Rule:  "原文自带「摘要」时提取；否则留空，系统会用知识库摘要回填。",
				Regex: `(?:摘\s*要|内容摘要)\s*[:：]?\s*([^\n]{5,200})`},
		},
		PromptTemplate: `你是一位公文档案管理员，需要从通知/公告/通报全文中提取结构化字段。

【任务】
阅读 <document> 中的公文文本，输出严格 JSON。一份文件只对应一条公告记录。

【字段与规则】
{{fields_schema}}

【字段口径】
1. 只输出 JSON 中列出的字段；原文没有的填空字符串（数字填 0）。
2. 日期统一 YYYY-MM-DD；发文字号可能被排版折断成两行，必须拼接完整。
3. 标题可能跨行，需拼接成完整一句，不要截断。
4. 通报/决定类文件还要提取当事人、处理决定与依据；纯公告类文件这些字段留空。
5. 不确定的字段宁可留空，严禁编造。

【输出】
{"kind": "award_punish", "records": [ { ...字段... } ]}`,
	}
}

// SnakeKey 把 camelCase / 中文混排的字段名归一到契约键（snake_case）。
//
// 前端 subs 用 camelCase（contractNo）便于 JS 使用，而提取结果与正则补洞按结构体
// json tag（contract_no）匹配。这一步是两者之间**唯一**的转换点：只在喂给提取器
// 的规则配置上做，DB 与界面仍然保存用户看到的 camelCase。
func SnakeKey(name string) string {
	var sb strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				sb.WriteByte('_')
			}
			sb.WriteRune(r + ('a' - 'A'))
			continue
		}
		if r == '-' || r == ' ' {
			sb.WriteByte('_')
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// WithSnakeFieldKeys 返回一份字段名已归一为 snake_case 的规则配置副本。
// 不改原对象（原对象仍要按 camelCase 存回 DB 供界面回显）。
func WithSnakeFieldKeys(cfg *types.KbExtractConfig) *types.KbExtractConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Fields = make([]types.ExtractFieldConfig, len(cfg.Fields))
	for i, f := range cfg.Fields {
		f.Name = SnakeKey(f.Name)
		out.Fields[i] = f
	}
	return &out
}

// DefaultExtractFields 由目录合成某个 scope 的默认提取字段配置。
// certType 仅用于 classif 场景（分类主轴默认值），当前不参与内容。
func DefaultExtractFields(scope, _ string) []types.ExtractFieldConfig {
	cat, ok := BusinessFieldCatalogOf(scope)
	if !ok {
		return nil
	}
	fields := make([]types.ExtractFieldConfig, 0, len(cat.Standard)+len(cat.Extended))
	for _, f := range cat.AllFields() {
		fields = append(fields, types.ExtractFieldConfig{
			Name:    f.Key,
			Label:   f.Label,
			Desc:    f.Desc,
			Type:    f.Type,
			Rule:    f.Rule,
			Regex:   f.Regex,
			Enabled: true,
		})
	}
	return fields
}

// DefaultBusinessPromptTemplate 返回某个 scope 的高级模式 System Prompt 模板。
func DefaultBusinessPromptTemplate(scope string) string {
	cat, ok := BusinessFieldCatalogOf(scope)
	if !ok {
		return ""
	}
	return cat.PromptTemplate
}

// BackfillExtractConfigDefaults 把默认底稿幂等合并进一份用户配置：
//   - 缺失字段补齐（用户已删掉的字段会被"复活"吗？不会：只在字段数少于目录时补，
//     且只在用户配置为空时整体套用；非空配置只补 desc/rule/regex 空值）。
//   - 已有字段只填空的 Desc/Rule/Regex，绝不覆盖用户改过的文案。
//   - PromptTemplate 为空时填默认模板。
//
// 返回是否有改动（调用方据此决定是否落库）。
func BackfillExtractConfigDefaults(cfg *types.KbExtractConfig) bool {
	if cfg == nil {
		return false
	}
	defaults := DefaultExtractFields(cfg.Scope, cfg.CertType)
	if len(defaults) == 0 {
		return false
	}
	changed := false
	if len(cfg.Fields) == 0 {
		cfg.Fields = defaults
		changed = true
	} else {
		byKey := make(map[string]types.ExtractFieldConfig, len(defaults))
		for _, d := range defaults {
			byKey[d.Name] = d
		}
		for i := range cfg.Fields {
			d, ok := byKey[SnakeKey(cfg.Fields[i].Name)]
			if !ok {
				continue
			}
			if strings.TrimSpace(cfg.Fields[i].Desc) == "" && d.Desc != "" {
				cfg.Fields[i].Desc = d.Desc
				changed = true
			}
			if strings.TrimSpace(cfg.Fields[i].Rule) == "" && d.Rule != "" {
				cfg.Fields[i].Rule = d.Rule
				changed = true
			}
			if strings.TrimSpace(cfg.Fields[i].Regex) == "" && d.Regex != "" {
				cfg.Fields[i].Regex = d.Regex
				changed = true
			}
			if strings.TrimSpace(cfg.Fields[i].Type) == "" {
				cfg.Fields[i].Type = d.Type
				changed = true
			}
		}
	}
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		if tpl := DefaultBusinessPromptTemplate(cfg.Scope); tpl != "" {
			cfg.PromptTemplate = tpl
			changed = true
		}
	}
	return changed
}

// ReuseKnowledgeSummary 摘要字段复用知识库已生成的文档摘要：
// 提取结果里没有摘要时，直接取 knowledges.description（与知识库文档摘要同源）。
func ReuseKnowledgeSummary(extracted, description string) string {
	if v := strings.TrimSpace(extracted); v != "" {
		return v
	}
	return strings.TrimSpace(description)
}
