// 宿舍管理字段契约（第 62 轮 v2）。
//
// 五个胶囊 = 页面五个标签，顺序严格 1:1（六同步）：
//
//	宿舍档案 / 公租房档案 / 住宿记录 / 资产档案 / 费用信息
//
// 「费用项目」是编辑抽屉里的第二分组（前端按字段名切分），不是独立胶囊 ——
// 否则设置抽屉会多出 2 个胶囊，破坏五标签六同步。
package handler

import "github.com/Tencent/WeKnora/internal/types"

const (
	hrScopeDorm = "hr-dorm"

	hrDormGroupRoom    = "宿舍档案"
	hrDormGroupPublic  = "公租房档案"
	hrDormGroupStay    = "住宿记录"
	hrDormGroupAsset   = "资产档案"
	hrDormGroupFee     = "费用信息"
	hrDormModuleRoom   = "dorm"
	hrDormModulePublic = "public"
	hrDormModuleStay   = "stay"
	hrDormModuleAsset  = "asset"
	hrDormModuleFee    = "fee"
)

// hrDormStatusGroupFields61 人事胶囊白名单：宿舍五胶囊各自唯一的加锁状态列。
func hrDormStatusGroupFields61() map[string][]string {
	return map[string][]string{
		hrDormGroupRoom:   {"宿舍状态"},
		hrDormGroupPublic: {"宿舍状态"},
		hrDormGroupStay:   {"住宿状态"},
		hrDormGroupAsset:  {"资产状态"},
		hrDormGroupFee:    {"账单状态"},
	}
}

// hrDormEnumOptions 宿舍模块枚举字典（不复用花名册字典，避免串味）。
func hrDormEnumOptions(field string) []string {
	switch field {
	case "房型":
		return []string{"单人间", "双人间", "夫妻房", "四人间", "六人间", "八人间"}
	case "性别限制":
		return []string{"男", "女", "不限"}
	case "宿舍状态":
		return []string{"空闲", "部分入住", "满员", "维修中"}
	case "房源类型":
		// 第 62 轮：对齐 Excel「员工宿舍 / 公租房」口径
		return []string{"公司宿舍", "公租房"}
	case "床位号":
		return []string{"1号床", "2号床", "3号床", "4号床", "5号床", "6号床", "7号床", "8号床"}
	case "入住类型":
		return []string{"新入住", "调宿", "续住"}
	case "住宿状态":
		return []string{"在住", "待入住", "已退宿"}
	case "退宿原因":
		return []string{"离职退宿", "个人外住", "调宿退宿", "工作调动", "其他原因"}
	case "资产分类":
		return []string{"家具", "家电", "办公", "其他"}
	case "计量单位":
		return []string{"张", "个", "台", "把", "套"}
	case "维修状态":
		return []string{"无维修", "待维修", "维修中", "已修复"}
	case "资产状态":
		return []string{"正常", "损坏", "报废"}
	case "账单状态":
		return []string{"已生成", "已作废"}
	case "租赁单位":
		return []string{"重庆星达铜业有限公司"}
	case "所属公司/部门":
		return []string{"重庆星达铜业有限公司", "生产部", "技术质量部", "财务部", "行政部", "销售部"}
	case "关联电表", "关联水表", "关联气表":
		return hrDormMeterOptions()
	}
	return nil
}

// hrDormMeterOptions 关联表计下拉：与能耗管理宿舍水电表命名一致（含总表）。
func hrDormMeterOptions() []string {
	return []string{
		"宿舍总表", "宿舍301", "宿舍302", "宿舍303", "宿舍304", "宿舍305", "宿舍306",
		"宿舍307", "宿舍308", "宿舍309", "宿舍310", "宿舍311", "宿舍312", "宿舍313",
		"宿舍314", "宿舍315", "宿舍316", "宿舍317", "宿舍318", "宿舍319", "宿舍320",
		"宿舍417", "宿舍419",
		"两山秀苑19-23-5", "两山秀苑19-23-6", "两山秀苑19-23-7", "两山秀苑19-23-8",
		"两山秀苑19-23-9", "两山秀苑19-23-10", "两山秀苑19-23-11", "两山秀苑19-23-12",
		"两山秀苑19-23-14",
		"两山景苑5-2-6", "两山景苑5-2-7", "两山景苑5-2-8", "两山景苑5-21-15",
		"两山景苑5-21-16", "两山景苑5-24-3", "两山景苑5-24-6", "两山景苑5-24-7",
		"两山景苑5-24-8", "两山景苑5-24-9",
	}
}

// hrDormFieldDefs 五个胶囊的全部字段（顺序 = 表头顺序；费用项目字段排在各档案末尾）。
func hrDormFieldDefs() []hrFieldDef {
	const (
		roomG   = hrDormGroupRoom
		publicG = hrDormGroupPublic
		stayG   = hrDormGroupStay
		assetG  = hrDormGroupAsset
		feeG    = hrDormGroupFee
	)
	sys := func(name, group, enum string) hrFieldDef {
		return hrF(name, "system", hrHeader(), hrBuiltin(), hrEnum(enum), hrGroup(group))
	}
	enum := func(name, group string) hrFieldDef {
		return hrF(name, "enum", hrHeader(), hrEnum(name), hrGroup(group))
	}
	num := func(name, group string) hrFieldDef {
		return hrF(name, "number", hrGroup(group))
	}

	defs := []hrFieldDef{
		// ---- 宿舍档案 · 分组一 ----
		hrF("宿舍编号", "text", hrHeader(), hrBuiltin(), hrGroup(roomG)),
		hrF("宿舍楼/区域", "text", hrHeader(), hrDefault("1号楼"), hrGroup(roomG)),
		hrF("房间号", "text", hrHeader(), hrGroup(roomG)),
		hrF("房型", "enum", hrHeader(), hrEnum("房型"), hrDefault("双人间"), hrGroup(roomG)),
		hrF("床位数", "number", hrHeader(), hrGroup(roomG)),
		hrF("可用床位数", "number", hrHeader(), hrGroup(roomG)),
		enum("性别限制", roomG),
		hrF("房间面积(㎡)", "number", hrHeader(), hrGroup(roomG)),
		hrF("配套设施", "text", hrHeader(), hrGroup(roomG)),
		hrF("所属公司", "enum", hrHeader(), hrDynamic("unit"), hrGroup(roomG)),
		hrF("所属部门", "enum", hrHeader(), hrDynamic("department"), hrGroup(roomG)),
		hrF("管理员/舍长", "text", hrGroup(roomG)),
		sys("宿舍状态", roomG, "宿舍状态"),
		hrF("备注", "textarea", hrGroup(roomG)),
		// ---- 宿舍档案 · 分组二：费用项目 ----
		enum("关联电表", roomG),
		enum("关联水表", roomG),
		enum("关联气表", roomG),
		num("宿舍押金(元)", roomG),
		num("电费标准(元/度)", roomG),
		num("水费单价(元/m³)", roomG),
		num("垃圾处理费(元/套)", roomG),
		num("二次供水费(元/m³)", roomG),
		num("污水处理费(元/m³)", roomG),
		num("气费标准(元/m³)", roomG),
		num("房屋租金(元/月)", roomG),
		num("物业费(元/月)", roomG),

		// ---- 公租房档案 · 分组一 ----
		hrF("房屋编号", "text", hrHeader(), hrBuiltin(), hrGroup(publicG)),
		hrF("房屋地址", "text", hrHeader(), hrGroup(publicG)),
		enum("租赁单位", publicG),
		hrF("所属公司", "enum", hrHeader(), hrDynamic("unit"), hrGroup(publicG)),
		hrF("所属部门", "enum", hrHeader(), hrDynamic("department"), hrGroup(publicG)),
		hrF("建筑面积(㎡)", "number", hrHeader(), hrGroup(publicG)),
		enum("房型", publicG),
		hrF("床位数", "number", hrHeader(), hrGroup(publicG)),
		hrF("可用床位数", "number", hrHeader(), hrGroup(publicG)),
		enum("性别限制", publicG),
		hrF("配套设施", "text", hrGroup(publicG)),
		hrF("管理员/舍长", "text", hrGroup(publicG)),
		sys("宿舍状态", publicG, "宿舍状态"),
		hrF("备注", "textarea", hrGroup(publicG)),
		// ---- 公租房档案 · 分组二：费用项目 ----
		enum("关联电表", publicG),
		enum("关联水表", publicG),
		enum("关联气表", publicG),
		num("宿舍押金(元)", publicG),
		num("电费标准(元/度)", publicG),
		num("水费单价(元/m³)", publicG),
		num("垃圾处理费(元/套)", publicG),
		num("二次供水费(元/m³)", publicG),
		num("污水处理费(元/m³)", publicG),
		num("气费标准(元/m³)", publicG),
		num("房屋租金(元/月)", publicG),
		num("物业费(元/月)", publicG),
		num("月费用单价(元/月)", publicG),
		num("季度租金(元/季)", publicG),
		hrF("租期起始月份", "text", hrGroup(publicG)),
		hrF("租期截止月份", "text", hrGroup(publicG)),

		// ---- 住宿记录 · 分组一 ----
		hrF("住宿记录ID", "text", hrGroup(stayG)),
		hrF("员工工号", "readonly", hrHeader(), hrRef("花名册", "工号"), hrGroup(stayG)),
		hrF("员工姓名", "text", hrHeader(), hrBuiltin(), hrGroup(stayG)),
		hrF("所属部门", "readonly", hrHeader(), hrRef("花名册", "部门"), hrGroup(stayG)),
		hrF("联系电话", "readonly", hrRef("花名册", "手机号码"), hrGroup(stayG)),
		hrF("紧急联系人及电话", "readonly", hrRef("花名册", "紧急联系人姓名"), hrGroup(stayG)),
		enum("房源类型", stayG),
		hrF("宿舍编号/房屋地址", "text", hrHeader(), hrGroup(stayG)),
		enum("床位号", stayG),
		enum("入住类型", stayG),
		hrF("入住日期", "date", hrHeader(), hrGroup(stayG)),
		hrF("退宿日期", "date", hrGroup(stayG)),
		num("押金金额(元)", stayG),
		enum("退宿原因", stayG),
		sys("住宿状态", stayG, "住宿状态"),
		// ---- 住宿记录 · 分组二：费用项目 ----
		num("入住时水表度数", stayG),
		num("入住时电表度数", stayG),
		num("入住时气表度数", stayG),
		num("分摊比例", stayG),
		num("房屋租金", stayG),
		num("物业费", stayG),
		num("季度租金", stayG),
		num("电费标准", stayG),
		num("水费单价", stayG),
		num("垃圾处理费", stayG),
		num("二次供水费", stayG),
		num("气费标准", stayG),

		// ---- 资产档案 ----
		hrF("资产编号", "text", hrHeader(), hrBuiltin(), hrGroup(assetG)),
		hrF("资产名称", "text", hrHeader(), hrGroup(assetG)),
		enum("资产分类", assetG),
		hrF("规格型号", "text", hrGroup(assetG)),
		hrF("所属宿舍", "text", hrHeader(), hrGroup(assetG)),
		num("数量", assetG),
		enum("计量单位", assetG),
		num("资产价值(元)", assetG),
		hrF("购置日期", "date", hrGroup(assetG)),
		hrF("责任人", "text", hrGroup(assetG)),
		hrF("报修日期", "date", hrGroup(assetG)),
		enum("维修状态", assetG),
		num("维修费用(元)", assetG),
		hrF("维修说明", "textarea", hrGroup(assetG)),
		hrF("备注", "textarea", hrGroup(assetG)),
		sys("资产状态", assetG, "资产状态"),

		// ---- 费用信息（全员月度扣款明细，19 列）----
		hrF("账单月份", "text", hrHeader(), hrBuiltin(), hrGroup(feeG)),
		hrF("员工工号", "text", hrHeader(), hrGroup(feeG)),
		hrF("姓名", "text", hrHeader(), hrBuiltin(), hrGroup(feeG)),
		hrF("所属部门", "text", hrHeader(), hrGroup(feeG)),
		hrF("宿舍号", "text", hrHeader(), hrGroup(feeG)),
		// 金额列默认进表头：扣款明细的合计行要能直接看到各项累加（其余列可在字段筛选器开启）
		hrF("入住天数", "number", hrHeader(), hrGroup(feeG)),
		num("分摊比例", feeG),
		num("宿舍押金(元)", feeG),
		hrF("公租房租(元)", "number", hrHeader(), hrGroup(feeG)),
		hrF("水费(元)", "number", hrHeader(), hrGroup(feeG)),
		hrF("电费(元)", "number", hrHeader(), hrGroup(feeG)),
		hrF("天然气费(元)", "number", hrHeader(), hrGroup(feeG)),
		hrF("社保个人扣款(元)", "number", hrHeader(), hrGroup(feeG)),
		hrF("公积金个人扣款(元)", "number", hrHeader(), hrGroup(feeG)),
		num("其它扣款(元)", feeG),
		hrF("扣款合计(元)", "number", hrHeader(), hrGroup(feeG)),
		num("退款(元)", feeG),
		num("补贴合计(元)", feeG),
		hrF("备注", "textarea", hrGroup(feeG)),
		sys("账单状态", feeG, "账单状态"),
	}
	return defs
}

// hrDormSeedSubs 五胶囊字段（同名字段按 (group,name) 保留）。
func hrDormSeedSubs() []types.FleetCategorySub {
	return hrSubsOfDefsByGroup(hrDormFieldDefs(), true)
}

// hrDormFieldsOf 某胶囊的字段定义（导入模板 / 校验复用）。
func hrDormFieldsOf(group string) []hrFieldDef {
	out := make([]hrFieldDef, 0)
	for _, d := range hrDormFieldDefs() {
		if d.Group == group {
			out = append(out, d)
		}
	}
	return out
}
