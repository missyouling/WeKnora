package service

// 能耗（energy）字段目录。
//
// 单独成文件的原因：能耗是「账期台账」口径，与合同/制度/通知的「文档档案」
// 口径形态不同（前者主时间轴是月，后者是日），字段目录的推导规则也不共享。
// 放进 business_field_catalog.go 会让那个文件继续膨胀，且两种口径混在一起
// 容易在后续加字段时互相污染。
//
// 与既有 utility scope 的关系（重要）：
//   - utility/solar 是**账单明细级**口径：一条账单 30~50 行费用明细 + 峰平谷分时，
//     走专用提取管线（knowledge_utility_extract.go），字段字典为空；
//   - energy 是**账期台账级**口径：一条记录 = 一个账期（2026-09）的一张汇总单，
//     核心是「上期读数 / 本期读数 / 倍率 / 实际用量」这条数值连续性主线。
// 两者并存不冲突：energy 负责台账与用量治理，utility 负责明细费用核对。
//
// 键名一律 snake_case，且必须等于落库 json tag —— 否则
// ValidateGenericExtraction 的正则补洞按字段名匹配不到记录字段，规则会静默失效。

// 能耗账期正则：2026-09 / 2026年9月 / 2026 年 09 月。
//
// 账单上的账期写法极不统一（国网写「2026年09月」，水司写「2026-09」，
// 部分写「2026.09」），必须宽松匹配后归一为 YYYY-MM。
const reBillingPeriod = `20\d{2}\s*[-年/.]\s*(?:0?[1-9]|1[0-2])\s*月?`

// 能耗数值：支持千分位与两位小数（读数可能有 3~4 位小数，如 3825.28）。
const reEnergyNumber = `[0-9][0-9,，]*(?:\.[0-9]{1,4})?`

// energyCatalog 能耗台账的字段目录。
//
// 标准字段（10 个，系统级不可删）严格对齐产品清单：
//
//	utility_no / billing_period / utility_type / previous_reading /
//	current_reading / multiplier / usage_amount / unit_price /
//	total_amount / extract_status
//
// 另加 tags（系统内置三件套：分类主轴 + 提取状态 + 标签，与其它 scope 对齐）。
func energyCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "energy",
		CategoryLabel: "能耗类型",
		Standard: []BusinessFieldSpec{
			{Key: "utility_no", Label: "单据编号/户号", Type: "text", Builtin: true,
				Desc:  "水电燃气账单的用户户号 / 客户编号",
				Rule:  "提取账单抬头或「户号 / 用户编号 / 客户号」栏位的编号，如 5001654065328；没有时留空，严禁编造。",
				Regex: `(?:户\s*号|用户编号|客户编号|客户号|用户号)\s*[:：]?\s*([0-9A-Za-z\-]{4,32})`},
			{Key: "billing_period", Label: "账期", Type: "text", Builtin: true,
				Desc:  "账单所属账期（YYYY-MM）",
				Rule:  "提取账单所属账期，统一归一为 YYYY-MM（如 2026-09）。优先取「账期 / 账单周期 / 抄表周期 / 费用所属月份」；没有时用账单周期起止日期的起始月份。",
				Regex: `(?:账\s*期|账单周期|抄表周期|费用所属月份|结算周期|所属月份)\s*[:：]?\s*(` + reBillingPeriod + `)`},
			{Key: "utility_type", Label: "能耗类型", Type: "text", Builtin: true,
				Desc:  "能耗类别（电费 / 水费 / 燃气费 / 供热费）",
				Rule:  "判断账单所属能源种类：含「电」或电力/用电/供电→电费；含「水」或供水/用水→水费；含「气」或燃气/天然气→燃气费；含「热」或供热/蒸汽→供热费；判断不了留空。",
				Regex: `(?:能源种类|费用类型|产品类型|账单类型|项目类型)\s*[:：]?\s*(电费|水费|燃气费|供热费|暖气费)`},
			{Key: "previous_reading", Label: "上期读数", Type: "number",
				Desc:  "上期（抄表）累计读数",
				Rule:  "提取抄表读数表里主计量表（正向有功总表，或「上期示数 / 上期读数 / 上期累计」）的上期读数；只取一个主表值，不要取峰平谷分表的数。",
				Regex: `(?:上期(?:示数|读数|累计|抄见)|期初(?:示数|读数|累计))\s*[:：]?\s*(` + reEnergyNumber + `)`},
			{Key: "current_reading", Label: "本期读数", Type: "number",
				Desc:  "本期（抄表）累计读数",
				Rule:  "提取抄表读数表里主计量表的本期读数（示数 / 读数 / 累计 / 抄见）；只取一个主表值，不要取峰平谷分表的数。",
				Regex: `(?:本期(?:示数|读数|累计|抄见)|期末(?:示数|读数|累计))\s*[:：]?\s*(` + reEnergyNumber + `)`},
			{Key: "multiplier", Label: "倍率", Type: "number",
				Desc:  "计量倍率（CT 变比，单表计为 1）",
				Rule:  "提取主计量表的倍率 / 变比（如 3000）；单表计无倍率时填 1。",
				Regex: `(?:倍\s*率|变\s*比|互感器倍率)\s*[:：]?\s*(` + reEnergyNumber + `)`},
			{Key: "usage_amount", Label: "实际用量", Type: "number",
				Desc:  "本期实际用量（= (本期读数 − 上期读数) × 倍率）",
				Rule:  "提取账单上的本期用量 / 抄见电量 / 用量合计（如 595860）；没有时留空，系统会用读数差值自动反算。",
				Regex: `(?:实际用量|本期用量|用\s*量(?:合计|总计)?|抄见电量|计费电量)\s*[:：]?\s*(` + reEnergyNumber + `)`},
			{Key: "unit_price", Label: "单价", Type: "number",
				Desc:  "综合单价（元/单位）",
				Rule:  "提取账期的综合单价 / 平均单价 / 电价单价（元）；优先取账单上的均价行。",
				Regex: `(?:综合单价|平均单价|单\s*价)\s*[:：]?\s*(` + reEnergyNumber + `)`},
			{Key: "total_amount", Label: "总金额", Type: "number",
				Desc:  "本期应付总金额（元）",
				Rule:  "提取本期应付金额 / 合计金额 / 应缴金额（元），去掉千分位与货币符号；不要取「上次结余 / 预缴余额」。",
				Regex: `(?:应付金额|应缴金额|本期金额|合计金额|费用合计|总金额|金额合计)\s*[:：]?\s*[¥￥]?\s*(` + reEnergyNumber + `)`},
			{Key: "extract_status", Label: "提取状态", Type: "text", Builtin: true,
				Desc: "提取状态（系统字段）",
				Rule: "由系统写入，不需要从原文提取。"},
			{Key: "tags", Label: "标签", Type: "array", Builtin: true,
				Desc: "记录标签（系统字段）",
				Rule: "由用户维护，不需要从原文提取。"},
		},
		Extended: []BusinessFieldSpec{
			{Key: "account_name", Label: "户名", Type: "text",
				Desc:  "用户/户名（用电户、水表户）",
				Rule:  "提取账单上的户名 / 用户名称 / 客户名称。",
				Regex: `(?:户\s*名|用户名称|客户名称|客户名)\s*[:：]?\s*([^\n]{2,60})`},
			{Key: "meter_no", Label: "表号", Type: "text",
				Desc:  "计量表表号",
				Rule:  "提取主计量表的表号 / 电表号 / 水表号。",
				Regex: `(?:表\s*号|电表号|水表号|计量表号)\s*[:：]?\s*([0-9A-Za-z\-]{4,32})`},
			{Key: "address", Label: "用能地址", Type: "text",
				Desc:  "用能地址 / 抄表地址",
				Rule:  "提取账单上的用电地址 / 用水地址 / 抄表地址。",
				Regex: `(?:用电地址|用水地址|抄表地址|用户地址|地\s*址)\s*[:：]?\s*([^\n]{4,80})`},
			{Key: "period_start", Label: "账期起", Type: "date",
				Desc:  "账期起始日期",
				Rule:  "提取账单周期起始日期，统一为 YYYY-MM-DD。",
				Regex: `(?:起始日期|账单周期开始|周期开始|起始)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "period_end", Label: "账期止", Type: "date",
				Desc:  "账期截止日期",
				Rule:  "提取账单周期结束日期，统一为 YYYY-MM-DD。",
				Regex: `(?:截止日期|账单周期结束|周期结束|截止)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "read_date", Label: "抄表日期", Type: "date",
				Desc:  "实际抄表日期",
				Rule:  "提取抄表日期 / 抄见日期 / 计量日期，统一为 YYYY-MM-DD。",
				Regex: `(?:抄表日期|抄见日期|计量日期|抄\s*表\s*日)\s*[:：]?\s*(` + reDateLoose + `)`},
			{Key: "usage_category", Label: "用能类别", Type: "text",
				Desc:  "用电/用水类别（大工业、居民、商业等）",
				Rule:  "提取用电类别 / 用水类别 / 计价类别，如「大工业用电 / 居民生活 / 一般工商业」。",
				Regex: `(?:用电类别|用水类别|计价类别|用户类别)\s*[:：]?\s*([^\n，,；;]{2,20})`},
			{Key: "voltage_level", Label: "电压等级", Type: "text",
				Desc:  "供电电压等级",
				Rule:  "提取电压等级，如「交流10kV / 低压 380V」。",
				Regex: `(?:电压等级|供电电压)\s*[:：]?\s*([^\n，,；;]{2,20})`},
			{Key: "remark", Label: "备注", Type: "textarea",
				Desc:  "账单备注 / 补充说明",
				Rule:  "提取账单上的备注、附加说明；没有则留空。",
				Regex: `备\s*注\s*[:：]?\s*([^\n]{2,120})`},
		},
		PromptTemplate: `你是一位企业能耗台账管理员，需要从水电燃气账单全文中提取账期级结构化字段。

【任务】
阅读 <document> 中的账单文本，输出严格 JSON。一份文件只对应一条能耗台账记录。

【字段与规则】
{{fields_schema}}

【字段口径】
1. 只输出 JSON 中列出的字段；原文没有的字段填空字符串（数字填 0）。
2. **账期统一 YYYY-MM**（如 2026-09），日期统一 YYYY-MM-DD。
3. **读数只取主计量表**：账单常有「正向有功（总）/ 尖峰 / 峰 / 平 / 谷」多行示数，
   上期读数、本期读数、倍率都取**总表**那一行，不要混用分表数值。
4. **用量口径**：实际用量 = (本期读数 − 上期读数) × 倍率；账单已直接给出抄见电量/计费电量时，
   以账单数值为准（系统会做交叉校验）。倍率单表计为 1。
5. 金额单位为元，去掉千分位与货币符号；总金额取本期应付/合计，不要取上次结余或预缴余额。
6. 账期、户号、读数、金额常分散在不同页（读数在「电量明细」表，总金额在「费用明细」合计行），
   必须在全文范围内查找。
7. 抄表记录/汇总单含多个表位时，各表位读数并入 items 明细，仍只输出一条记录
   （一文件一档案，绝不裂变为多条）。
8. 不确定的字段宁可留空，严禁编造。

【输出】
{"kind": "energy", "records": [ { ...字段... } ]}`,
	}
}
