// 宿舍管理字段契约（第 61 轮）。
//
// 五个胶囊 = 页面五个标签，顺序严格 1:1（六同步）：
//
//	宿舍档案 15 / 公租房档案 17 / 住宿记录 15 / 资产档案 10 / 费用信息 9
//
// 与前端 frontend/src/views/dailyAffairs/hrDorm.ts 的 HR_DORM_GROUPS 同源。
// 状态列（宿舍状态 / 住宿状态 / 资产状态 / 账单状态）是系统内置加锁字段，
// 由 hrGroupStatusFields60 白名单精确绑定到各自胶囊，绝不跨组穿透。
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

// hrDormStatusGroupFields61 人事胶囊白名单补充：宿舍五胶囊各自唯一的加锁状态列。
func hrDormStatusGroupFields61() map[string][]string {
	return map[string][]string{
		hrDormGroupRoom:   {"宿舍状态"},
		hrDormGroupPublic: {"宿舍状态"},
		hrDormGroupStay:   {"住宿状态"},
		hrDormGroupAsset:  {"资产状态"},
		hrDormGroupFee:    {"账单状态"},
	}
}

// hrDormEnumOptions 宿舍模块的枚举字典（不复用花名册字典，避免串味）。
func hrDormEnumOptions(field string) []string {
	switch field {
	case "房型":
		return []string{"单人间", "双人间", "四人间", "六人间", "八人间"}
	case "性别限制":
		return []string{"男", "女", "不限"}
	case "宿舍状态":
		return []string{"空闲", "部分入住", "满员", "维修中"}
	case "房源类型":
		return []string{"厂区宿舍", "公租房"}
	case "床位号":
		return []string{"1号床", "2号床", "3号床", "4号床", "5号床", "6号床", "7号床", "8号床"}
	case "入住类型":
		return []string{"新入住", "调宿", "续住"}
	case "住宿状态":
		return []string{"在住", "待入住", "已退宿"}
	case "资产名称/类型":
		return []string{"床", "衣柜", "空调", "热水器", "书桌", "椅子", "洗衣机", "其他"}
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
		"宿舍总表", "宿舍302", "宿舍304",
		"持睿403", "持睿404", "持睿405", "持睿406", "持睿407", "持睿408",
		"持睿409", "持睿410", "持睿411", "持睿412", "持睿413", "持睿414",
		"持睿415", "持睿416", "持睿417", "持睿418",
		"两山景苑5栋2-6", "两山景苑5栋2-7", "两山景苑5栋2-8",
		"两山景苑5栋21-15", "两山景苑5栋21-16",
		"两山景苑5栋24-3", "两山景苑5栋24-6", "两山景苑5栋24-7",
		"两山景苑5栋24-8", "两山景苑5栋24-9",
	}
}

// hrDormFieldDefs 五个胶囊的全部字段（顺序 = 页面表头顺序）。
func hrDormFieldDefs() []hrFieldDef {
	const (
		roomG   = hrDormGroupRoom
		publicG = hrDormGroupPublic
		stayG   = hrDormGroupStay
		assetG  = hrDormGroupAsset
		feeG    = hrDormGroupFee
	)
	sys := func(name, group string, enum string) hrFieldDef {
		return hrF(name, "system", hrHeader(), hrBuiltin(), hrEnum(enum), hrGroup(group))
	}
	enum := func(name, group string) hrFieldDef {
		return hrF(name, "enum", hrHeader(), hrEnum(name), hrGroup(group))
	}
	defs := []hrFieldDef{
		// ---- 宿舍档案 ----
		hrF("宿舍编号", "text", hrHeader(), hrBuiltin(), hrGroup(roomG)),
		hrF("宿舍楼/区域", "text", hrHeader(), hrGroup(roomG)),
		hrF("房间号", "text", hrHeader(), hrGroup(roomG)),
		enum("房型", roomG),
		hrF("床位数", "number", hrHeader(), hrGroup(roomG)),
		hrF("可用床位数", "number", hrHeader(), hrGroup(roomG)),
		enum("性别限制", roomG),
		hrF("房间面积(㎡)", "number", hrGroup(roomG)),
		hrF("配套设施", "text", hrGroup(roomG)),
		enum("关联电表", roomG),
		enum("关联水表", roomG),
		enum("所属公司/部门", roomG),
		hrF("管理员/舍长", "text", hrGroup(roomG)),
		sys("宿舍状态", roomG, "宿舍状态"),
		hrF("备注", "textarea", hrGroup(roomG)),

		// ---- 公租房档案（图 7《璧山高新区配租明细表》）----
		hrF("房屋编号", "text", hrHeader(), hrBuiltin(), hrGroup(publicG)),
		hrF("房屋地址", "text", hrHeader(), hrGroup(publicG)),
		enum("租赁单位", publicG),
		hrF("建筑面积(㎡)", "number", hrHeader(), hrGroup(publicG)),
		enum("房型", publicG),
		hrF("床位数", "number", hrHeader(), hrGroup(publicG)),
		hrF("可用床位数", "number", hrHeader(), hrGroup(publicG)),
		hrF("租金(元/月)", "number", hrHeader(), hrGroup(publicG)),
		hrF("物业(元/月)", "number", hrHeader(), hrGroup(publicG)),
		hrF("首月费用(元)", "number", hrHeader(), hrGroup(publicG)),
		hrF("季度(元/季)", "number", hrHeader(), hrGroup(publicG)),
		enum("关联电表", publicG),
		enum("关联水表", publicG),
		enum("关联气表", publicG),
		hrF("管理员/舍长", "text", hrGroup(publicG)),
		sys("宿舍状态", publicG, "宿舍状态"),
		hrF("备注", "textarea", hrGroup(publicG)),

		// ---- 住宿记录（宿舍—床位—住宿记录三层模型）----
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
		hrF("押金金额(元)", "number", hrGroup(stayG)),
		hrF("退宿原因", "textarea", hrGroup(stayG)),
		sys("住宿状态", stayG, "住宿状态"),

		// ---- 资产档案 ----
		hrF("资产编号", "text", hrHeader(), hrBuiltin(), hrGroup(assetG)),
		enum("资产名称/类型", assetG),
		hrF("所属宿舍", "text", hrHeader(), hrGroup(assetG)),
		hrF("数量", "number", hrGroup(assetG)),
		hrF("责任人", "text", hrGroup(assetG)),
		hrF("报修日期", "date", hrGroup(assetG)),
		enum("维修状态", assetG),
		hrF("维修费用(元)", "number", hrGroup(assetG)),
		hrF("维修说明", "textarea", hrGroup(assetG)),
		sys("资产状态", assetG, "资产状态"),

		// ---- 费用信息（按入住天数分摊后的月度账单）----
		hrF("账单周期", "text", hrHeader(), hrBuiltin(), hrGroup(feeG)),
		hrF("房源数", "number", hrHeader(), hrGroup(feeG)),
		hrF("入住人数", "number", hrHeader(), hrGroup(feeG)),
		hrF("房租合计", "number", hrHeader(), hrGroup(feeG)),
		hrF("物业合计", "number", hrHeader(), hrGroup(feeG)),
		hrF("水电费合计", "number", hrHeader(), hrGroup(feeG)),
		hrF("费用总计", "number", hrHeader(), hrGroup(feeG)),
		sys("账单状态", feeG, "账单状态"),
		hrF("生成时间", "date", hrHeader(), hrGroup(feeG)),
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
