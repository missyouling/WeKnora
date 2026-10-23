package service

// 市电（utility）与光伏（solar）账单的字段目录。
//
// 这两个 scope 与 energy 的口径差异（见 business_field_catalog_energy.go 的说明）：
//   - energy：账期台账级，一条 = 一个账期的一张汇总单；
//   - utility/solar：账单明细级，一条账单 30~50 行费用明细 + 峰平谷分时。
//
// 为什么现在才建目录：
//   改造前这两个 scope 在 extract_schema_registry 里是**空 schema**
//   （documentScopeSchema，无 StandardFields、RegexPatchable=false），
//   因此字段级正则补洞对它们完全不生效——用户在设置里配的正则形同摆设。
//   建立目录后：① 正则补洞有面可补；② 设置页首次打开能合成完整默认规则底稿；
//   ③ 提取 Prompt 的字段口径有了唯一权威源（不再散落在 1.6 万字常量里）。
//
// 键名铁律：Key 必须逐字等于 types.UtilityBillExtractionItem 的 json tag。
// 否则 ValidateGenericExtraction 按名字匹配不到记录字段，规则静默失效
// ——历史上前端 camelCase / 落库 snake_case 漂移就是这么踩的。

// ---- 通用正则片段 ----
const (
	// 户号：8~20 位数字，可能带横线
	reUtilityAccountNo = `(?:户\s*号|用户编号|客户编号|客户号|用户号|用电户号|发电户号)\s*[:：]?\s*([0-9][0-9\-]{6,24})`
	// 金额：支持千分位与两位小数，可带货币符
	reUtilityAmount = `[¥￥]?\s*(-?[0-9][0-9,，]*(?:\.[0-9]{1,2})?)`
	// 电量：支持千分位，最多 4 位小数（读数口径）
	reUtilityKwh = `(-?[0-9][0-9,，]*(?:\.[0-9]{1,4})?)`
	// 功率因数
	reUtilityPowerFactor = `(?:功率因数|功率因素|cos\s*[φΦ])\s*[:：]?\s*(0?\.\d{2,4}|1\.0{1,2})`
)

// utilityCatalog 市电账单字段目录。
//
// 标准字段严格对齐 types.UtilityBillExtractionItem（固定结构体），
// 用户在设置里新增的扩展字段走 Fields 动态桶，不进本目录。
func utilityCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "utility",
		CategoryLabel: "账单类型",
		Standard:      utilityCatalogFields(),
	}
}

func utilityCatalogFields() []BusinessFieldSpec {
	return []BusinessFieldSpec{
		// ---- 账单概况：严格 20 个业务字段（产品定稿，顺序即列表列顺序）----
		//
		// 这 20 项与 `electricityDefaultGroupFields("overview")` 必须逐项一致：
		// 前者喂提取规则/正则补洞，后者喂字段定义 subs，两处漂移会让「六同步」断裂。
		//
		// 为什么砍掉原来的 27 项：起止日/打印日/分时电量/容需量挤进主列表后，
		// 一是「账单周期」出现两列（start 与派生 period 同名），二是核心信息要横向
		// 滚动才可见。这些维度仍在提取结果里，详情抽屉照常展示，只是不进主表配置面。
		{Key: "bill_period", Label: "账单周期", Type: "text",
			Desc: "账单月份（YYYY-MM，由账单周期起止统一归一）",
			Rule: "不需要单独提取：由账单周期起止归一为 YYYY-MM（如 2026-08）。",
			Regex: `(?:账单周期|用电周期|结算周期|抄表周期)\s*[:：]?\s*(20\d{2}\s*[-/.年]\s*\d{1,2})`},
		{Key: "account_no", Label: "户号", Type: "text",
			Desc: "用电户号（供电单位客户编号）",
			Rule: "提取账单抬头或「户号 / 用户编号 / 客户号 / 用电户号」栏位的编号；没有时留空，严禁编造。",
			Regex: reUtilityAccountNo},
		{Key: "account_name", Label: "户名", Type: "text",
			Desc: "用电户名（客户名称）",
			Rule: "提取「户名 / 客户名称 / 用电人」栏位的名称，去掉多余空格。"},
		{Key: "usage_category", Label: "用电类别", Type: "text",
			Desc: "用电类别（大工业用电 / 一般工商业 / 居民生活等）",
			Rule: "提取「用电类别 / 电价类别」栏位，如「大工业用电」「一般工商业及其他用电」。"},
		{Key: "voltage_level", Label: "电压等级", Type: "text",
			Desc: "受电电压等级（如交流 10kV）",
			Rule: "提取「电压等级 / 受电电压」栏位，保留「交流 10kV」这类原文写法。"},
		{Key: "market_attr", Label: "市场化属性", Type: "text",
			Desc: "是否参与市场化交易及类型",
			Rule: "提取「市场化属性 / 交易类型」栏位（如「市场化交易」「代购电」）。"},
		{Key: "supply_unit", Label: "供电服务单位", Type: "text",
			Desc: "供电服务单位（如国网XX供电公司）",
			Rule: "提取账单开票方或「供电单位 / 服务单位」栏位。"},
		{Key: "address", Label: "用电地址", Type: "text",
			Desc: "用电地址",
			Rule: "提取「用电地址 / 用电地点 / 受电地址」栏位。"},
		{Key: "meter_no", Label: "电能表编号", Type: "text",
			Desc: "主计量表的电能表编号",
			Rule: "提取「电能表编号 / 表号 / 计量点编号」，只取主表，不要取分表。",
			Regex: `(?:电能表编号|电表编号|表\s*号|计量点编号)\s*[:：]?\s*([0-9A-Za-z\-]{4,32})`},
		{Key: "total_kwh", Label: "本期电量", Type: "number",
			Desc: "本期用电量（千瓦时）",
			Rule: "提取「本期电量 / 用电量合计 / 抄见电量合计」，去掉千分位。",
			Regex: `(?:本期电量|用电量合计|抄见电量合计|总用电量)\s*[:：]?\s*` + reUtilityKwh},
		{Key: "total_amount", Label: "本期电费", Type: "number",
			Desc: "本期电费总额（元）",
			Rule: "提取「本期电费 / 电费合计 / 应付金额」，去掉千分位与货币符，不要取结余或预缴。",
			Regex: `(?:本期电费|电费合计|应付金额|应缴金额)\s*[:：]?\s*` + reUtilityAmount},
		{Key: "prev_kwh", Label: "上期电量", Type: "number",
			Desc: "上期用电量（千瓦时）",
			Rule: "提取「上期电量 / 上期用电量」，没有则留空。"},
		{Key: "mom_change", Label: "环比", Type: "text",
			Desc: "本期电量环比变化（如 -15.18%）",
			Rule: "提取「环比 / 同比增长」栏位，保留百分号写法。"},
		{Key: "avg_price", Label: "平均电价", Type: "number",
			Desc: "综合平均电价（元/千瓦时）",
			Rule: "提取「平均电价 / 综合电价 / 均价」，保留 4 位小数。"},
		{Key: "power_factor", Label: "功率因素", Type: "number",
			Desc: "功率因素考核值",
			Rule: "提取「功率因数 / 功率因素」考核值，通常 0.90 上下。",
			Regex: reUtilityPowerFactor},
		{Key: "due_date", Label: "交费截止", Type: "date",
			Desc: "电费缴纳截止日",
			Rule: "提取「交费截止日期 / 缴费期限」，归一为 YYYY-MM-DD。"},
		{Key: "industrial_amount", Label: "工商业电费", Type: "number",
			Desc: "工商业用电电费小计",
			Rule: "提取工商业类别电费小计，没有则留空。"},
		{Key: "residential_amount", Label: "居民电费", Type: "number",
			Desc: "居民用电电费小计",
			Rule: "提取居民类别电费小计，没有则留空。"},
		{Key: "pf_adjust_amount", Label: "力调电费", Type: "number",
			Desc: "功率因数考核产生的增减电费（可为负）",
			Rule: "提取「功率因数调整电费 / 力调电费」，可为负数（奖励为负、惩罚为正按账单原文）。"},
		{Key: "grand_total", Label: "合计", Type: "number",
			Desc: "电费合计（含容需量与力调）",
			Rule: "提取「合计 / 总计 / 电费总计」，含基本电费与功率因数调整。"},
		// ---- 明细表（结构体数组，不进正则补洞面）----
		{Key: "meter_readings", Label: "电量明细", Type: "array",
			Desc: "电量明细表逐行（示数类型/上期/本期/倍率/抄见/变损/线损/加减/计费电量）",
			Rule: "逐行提取账单「电量明细」表，保留原始示数类型与全部数值列，不要合并行。"},
		{Key: "fee_items", Label: "费用明细", Type: "array",
			Desc: "费用明细逐行（类别/费用组成/时段/计费电量/计费标准/电费/账单电费）",
			Rule: "逐行提取全部费用明细，category 保留「(1)市场化购电费」这类原始编号前缀。"},
		{Key: "residential_readings", Label: "居民电量明细", Type: "array",
			Desc: "居民电量明细表（项目/本期电量/比例/加减/计费电量）",
			Rule: "逐行提取居民电量明细表，没有该表时留空数组。"},
		{Key: "capacity_detail", Label: "容需量明细", Type: "object",
			Desc: "输配容（需）量电费明细",
			Rule: "提取容需量明细对象，没有则留空。"},
		{Key: "pf_detail", Label: "力调明细", Type: "object",
			Desc: "功率因素调整电费明细",
			Rule: "提取功率因素调整明细对象，没有则留空。"},
		{Key: "remark", Label: "备注", Type: "textarea",
			Desc: "账单备注信息",
			Rule: "提取账单备注栏，没有则留空。"},
	}
}

// solarCatalog 光伏账单字段目录（对齐 types.SolarBillExtractionItem）。
func solarCatalog() BusinessFieldCatalog {
	return BusinessFieldCatalog{
		Scope:         "solar",
		CategoryLabel: "账单类型",
		Standard:      solarCatalogFields(),
	}
}

func solarCatalogFields() []BusinessFieldSpec {
	return []BusinessFieldSpec{
		{Key: "bill_period_start", Label: "账单周期起", Type: "date",
			Desc: "结算周期起始日",
			Rule: "提取光伏结算周期起止，归一为 YYYY-MM-DD。",
			Regex: `(?:结算周期|账单周期|发电周期)\s*[:：]?\s*(20\d{2}[-/.年]\d{1,2}[-/.月]\d{1,2})`},
		{Key: "bill_period_end", Label: "账单周期止", Type: "date",
			Desc: "结算周期截止日",
			Rule: "提取结算周期止日，归一为 YYYY-MM-DD。"},
		{Key: "account_no", Label: "发电户号", Type: "text",
			Desc: "光伏发电户号",
			Rule: "提取「发电户号 / 户号 / 客户编号」栏位。",
			Regex: reUtilityAccountNo},
		{Key: "account_name", Label: "户名", Type: "text",
			Desc: "发电户名",
			Rule: "提取「户名 / 客户名称」栏位。"},
		{Key: "supply_unit", Label: "服务单位", Type: "text",
			Desc: "供电服务单位",
			Rule: "提取「服务单位 / 供电单位」栏位。"},
		{Key: "address", Label: "发电地址", Type: "text",
			Desc: "发电项目地址",
			Rule: "提取「发电地址 / 项目地址」栏位。"},
		{Key: "taxpayer_type", Label: "纳税人类型", Type: "text",
			Desc: "纳税人类型（一般纳税人/小规模纳税人）",
			Rule: "提取「纳税人类型」栏位，没有则留空。"},
		{Key: "consumption_mode", Label: "消纳方式", Type: "text",
			Desc: "电量消纳方式（自发自用余电上网/全额上网）",
			Rule: "提取「消纳方式 / 上网方式」栏位。"},
		{Key: "voltage_level", Label: "并网电压等级", Type: "text",
			Desc: "并网电压等级（如交流 380V）",
			Rule: "提取「并网电压等级 / 电压等级」栏位。"},
		{Key: "generation_mode", Label: "发电方式", Type: "text",
			Desc: "发电方式（光伏发电）",
			Rule: "提取「发电方式 / 能源类型」栏位。"},
		{Key: "generation_kwh", Label: "发电量", Type: "number",
			Desc: "本期发电量（千瓦时）",
			Rule: "提取「发电量 / 本期发电量」，去掉千分位。",
			Regex: `(?:发电量|本期发电量)\s*[:：]?\s*` + reUtilityKwh},
		{Key: "grid_kwh", Label: "上网电量", Type: "number",
			Desc: "本期上网电量（千瓦时）",
			Rule: "提取「上网电量 / 余电上网电量」，去掉千分位。",
			Regex: `(?:上网电量|余电上网电量)\s*[:：]?\s*` + reUtilityKwh},
		{Key: "settlement_amount", Label: "结算金额", Type: "number",
			Desc: "本期结算金额（元）",
			Rule: "提取「结算金额 / 应付金额 / 合计金额」，去掉千分位与货币符。",
			Regex: `(?:结算金额|应付金额|合计金额|发电收入)\s*[:：]?\s*` + reUtilityAmount},
		{Key: "mom_change", Label: "环比", Type: "text",
			Desc: "本期上网电量环比",
			Rule: "提取「环比」栏位，保留百分号写法。"},
		{Key: "cumulative_kwh", Label: "年累计上网电量", Type: "number",
			Desc: "当年累计上网电量（千瓦时）",
			Rule: "提取「年累计 / 累计上网电量」。"},
		{Key: "tax_rate", Label: "税率", Type: "text",
			Desc: "税率（如 13%）",
			Rule: "提取「税率」栏位，保留百分号。"},
		{Key: "tax_amount", Label: "税额", Type: "number",
			Desc: "税额（元）",
			Rule: "提取「税额 / 增值税额」。"},
		{Key: "gateways", Label: "关口明细", Type: "array",
			Desc: "上网关口与发电关口明细（含电量明细与电费明细）",
			Rule: "按关口逐组提取：上网关口在前、发电关口在后，每组含电能表编号、项目名称、电量明细与电费明细。"},
		{Key: "remark", Label: "备注", Type: "textarea",
			Desc: "账单备注",
			Rule: "提取备注栏，没有则留空。"},
	}
}
