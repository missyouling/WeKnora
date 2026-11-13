// 人事模块字段契约的 Go 镜像（第 52 轮）。
//
// 前端 frontend/src/views/dailyAffairs/hrFieldSchema.ts 是同一份契约的 TS 版本：
// 花名册 6 大分组 / 离职管理 / 劳动合同 的字段名、类型、枚举、默认表头、引用来源
// 必须两边一致。这里驱动：分类种子（fleet_categories.subs）、三套 Excel 模板
// 表头与枚举下拉、导入解析器的字段归一。
//
// 边界：本文件只新增，不修改任何上游核心代码；所有导出标识符以 hr 前缀收口。
package handler

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 四个可导入模块（同时是模板 module 查询参数）。
const (
	hrModuleRoster   = "roster"
	hrModuleResign   = "resign"
	hrModuleContract = "contract"
	hrModuleSocial   = "social"
	hrModuleFund     = "fund"
)

// hrFieldDef 字段契约：与前端 HrFieldDef 一一对应。
type hrFieldDef struct {
	Name          string
	Type          string // text / date / number / enum / readonly / attachment / system
	EnumKey       string // 枚举字典 key；Type=enum 时必填
	Dynamic       string // department / unit（动态下拉，不进枚举 Sheet）
	DefaultHeader bool
	DefaultValue  string
	Group         string
	Source        *hrFieldSource
	Builtin       bool
}

// hrFieldSource 跨模块引用：值取自花名册 / 组织机构，字段设置里显示来源徽标。
type hrFieldSource struct {
	Category string
	Field    string
}

func hrF(name, typ string, extra ...func(*hrFieldDef)) hrFieldDef {
	d := hrFieldDef{Name: name, Type: typ}
	for _, fn := range extra {
		fn(&d)
	}
	return d
}
func hrEnum(key string) func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.EnumKey = key }
}
func hrDynamic(kind string) func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.Dynamic = kind }
}
func hrHeader() func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.DefaultHeader = true }
}
func hrDefault(v string) func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.DefaultValue = v }
}
func hrGroup(g string) func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.Group = g }
}
func hrRef(cat, field string) func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.Source = &hrFieldSource{Category: cat, Field: field} }
}
func hrBuiltin() func(*hrFieldDef) {
	return func(d *hrFieldDef) { d.Builtin = true }
}

// 花名册 6 大分组名（顺序即字段配置的单一连续列表顺序 / 编辑抽屉分组顺序）。
var hrRosterGroupOrder = []string{
	"在职信息", "基本信息", "联系信息", "紧急联系人情况", "教育情况", "工资社保信息",
}

// hrRosterFieldDefs 精简后的 48 个在职字段。
// 已删除：家庭情况 / 工作经历情况 / 个税申报信息 三组全部字段、在职状态 / 人员状态；
// 已剥离：离职信息 9 字段 → 离职管理，合同签订情况 8 字段 → 劳动合同。
func hrRosterFieldDefs() []hrFieldDef {
	return []hrFieldDef{
		// 1. 在职信息（10）
		hrF("工号", "text", hrGroup("在职信息"), hrHeader()),
		hrF("姓名", "text", hrGroup("在职信息"), hrHeader(), hrBuiltin()),
		hrF("部门", "enum", hrGroup("在职信息"), hrHeader(), hrDynamic("department")),
		hrF("岗位", "text", hrGroup("在职信息"), hrHeader()),
		hrF("合同公司", "enum", hrGroup("在职信息"), hrDynamic("unit")),
		hrF("招聘渠道", "enum", hrGroup("在职信息"), hrEnum("招聘渠道")),
		hrF("入职日期", "date", hrGroup("在职信息"), hrHeader()),
		hrF("试用期", "enum", hrGroup("在职信息"), hrEnum("试用期")),
		hrF("转正日期", "date", hrGroup("在职信息")),
		hrF("司龄", "readonly", hrGroup("在职信息"), hrHeader()),
		// 2. 基本信息（13）
		hrF("证件类型", "enum", hrGroup("基本信息"), hrEnum("证件类型"), hrDefault("身份证")),
		hrF("证件号码", "text", hrGroup("基本信息"), hrHeader()),
		hrF("身份证有效期", "date", hrGroup("基本信息")),
		hrF("性别", "enum", hrGroup("基本信息"), hrHeader(), hrEnum("性别")),
		hrF("出生日期", "date", hrGroup("基本信息"), hrHeader()),
		hrF("年龄", "readonly", hrGroup("基本信息"), hrHeader()),
		hrF("是否已婚", "enum", hrGroup("基本信息"), hrHeader(), hrEnum("是否已婚")),
		hrF("民族", "enum", hrGroup("基本信息"), hrEnum("民族"), hrDefault("汉族")),
		hrF("政治面貌", "enum", hrGroup("基本信息"), hrEnum("政治面貌"), hrDefault("群众")),
		hrF("籍贯", "text", hrGroup("基本信息"), hrHeader()),
		hrF("户口性质", "enum", hrGroup("基本信息"), hrHeader(), hrEnum("户口性质")),
		hrF("户籍地址", "text", hrGroup("基本信息")),
		hrF("服装尺寸", "enum", hrGroup("基本信息"), hrEnum("服装尺寸")),
		// 3. 联系信息（9）
		hrF("手机号码", "text", hrGroup("联系信息"), hrHeader()),
		hrF("现居住地", "text", hrGroup("联系信息")),
		hrF("个人邮箱", "text", hrGroup("联系信息")),
		hrF("工作邮箱", "text", hrGroup("联系信息")),
		hrF("工作电话", "text", hrGroup("联系信息")),
		hrF("分机号", "text", hrGroup("联系信息")),
		hrF("QQ号", "text", hrGroup("联系信息")),
		hrF("微信号", "text", hrGroup("联系信息")),
		hrF("企业微信账号", "text", hrGroup("联系信息")),
		// 4. 紧急联系人情况（4）
		hrF("紧急联系人姓名", "text", hrGroup("紧急联系人情况")),
		hrF("紧急联系人电话", "text", hrGroup("紧急联系人情况")),
		hrF("紧急联系人关系", "enum", hrGroup("紧急联系人情况"), hrEnum("紧急联系人关系")),
		hrF("紧急联系人地址", "text", hrGroup("紧急联系人情况")),
		// 5. 教育情况（4）
		hrF("最高学历", "enum", hrGroup("教育情况"), hrHeader(), hrEnum("最高学历")),
		hrF("毕业院校", "text", hrGroup("教育情况")),
		hrF("毕业专业", "text", hrGroup("教育情况")),
		hrF("毕业时间", "date", hrGroup("教育情况")),
		// 6. 工资社保信息（8）
		hrF("工资卡卡号", "text", hrGroup("工资社保信息")),
		hrF("银行名称", "enum", hrGroup("工资社保信息"), hrEnum("银行名称")),
		hrF("工资卡开户行", "text", hrGroup("工资社保信息")),
		hrF("工资卡开户城市", "text", hrGroup("工资社保信息")),
		// 第 54 轮：这两个字段是**只读跨模块引用**（值来自社保管理 / 公积金管理），
		// 花名册自身不存储、不可导入、不可在抽屉里编辑。
		hrF("个人社保账号", "readonly", hrGroup("工资社保信息"), hrHeader(), hrRef("社保管理", "个人社保账号")),
		hrF("个人公积金账号", "readonly", hrGroup("工资社保信息"), hrHeader(), hrRef("公积金管理", "职工账号")),
		hrF("是否本地首次缴纳社保", "enum", hrGroup("工资社保信息"), hrEnum("是否本地首次缴纳社保")),
		hrF("是否本地首次缴纳公积金", "enum", hrGroup("工资社保信息"), hrEnum("是否本地首次缴纳公积金")),
	}
}

// hrResignFieldDefs 离职管理：5 个花名册引用 + 9 个核心离职字段。
func hrResignFieldDefs() []hrFieldDef {
	return []hrFieldDef{
		hrF("工号", "readonly", hrRef("花名册", "工号"), hrHeader()),
		hrF("姓名", "readonly", hrRef("花名册", "姓名"), hrHeader()),
		hrF("部门", "readonly", hrRef("花名册", "部门"), hrHeader()),
		hrF("岗位", "readonly", hrRef("花名册", "岗位"), hrHeader()),
		hrF("入职日期", "date", hrRef("花名册", "入职日期"), hrHeader()),
		hrF("离职申请日期", "date"),
		hrF("离职日期", "date", hrHeader()),
		hrF("离职类型", "enum", hrHeader(), hrEnum("离职类型")),
		hrF("离职原因", "enum", hrHeader(), hrEnum("离职原因")),
		hrF("离职去向", "text", hrHeader()),
		hrF("是否试用期内离职", "enum", hrHeader(), hrEnum("是否试用期内离职")),
		hrF("薪资结算日", "date", hrHeader()),
		hrF("离职证明", "attachment", hrHeader()),
		// 第 57 轮：说明类字段改为多行文本并固定在最后一行（抽屉里独占整行宽）。
		hrF("离职原因说明", "textarea", hrHeader()),
	}
}

// hrContractFieldDefs 劳动合同：乙方 6 + 甲方 3 + 合同 8。
func hrContractFieldDefs() []hrFieldDef {
	return []hrFieldDef{
		hrF("工号", "readonly", hrRef("花名册", "工号"), hrHeader()),
		hrF("姓名", "readonly", hrRef("花名册", "姓名"), hrHeader()),
		hrF("部门", "readonly", hrRef("花名册", "部门"), hrHeader()),
		hrF("岗位", "readonly", hrRef("花名册", "岗位"), hrHeader()),
		hrF("证件号码", "readonly", hrRef("花名册", "证件号码"), hrHeader()),
		hrF("联系电话", "readonly", hrRef("花名册", "手机号码"), hrHeader()),
		hrF("统一社会信用代码", "readonly", hrRef("组织机构", "统一社会信用代码")),
		hrF("住所", "readonly", hrRef("组织机构", "住所")),
		hrF("法定代表人", "readonly", hrRef("组织机构", "法定代表人")),
		hrF("合同编号", "text"),
		hrF("合同类型", "enum", hrHeader(), hrEnum("合同类型")),
		hrF("合同开始日期", "date", hrHeader()),
		hrF("合同结束日期", "date", hrHeader()),
		hrF("合同期限", "enum", hrEnum("合同期限")),
		hrF("签约公司", "enum", hrDynamic("unit")),
		hrF("签订日期", "date"),
		hrF("合同状态", "system", hrHeader(), hrBuiltin(), hrEnum("合同状态")),
	}
}

// hrFundFieldDefs 公积金管理：严格对齐《单位职工明细-网厅.xls》的 18 个原生字段。
//
// 职工账号 是跨模块唯一键：导入后按证件号码回写花名册的「个人公积金账号」。
// 18 个字段全部来自网厅导出，不含任何跨表引用（跨表字段只能由用户在字段设置里
// 主动搜索添加，且永远不会进导入模板）。
func hrFundFieldDefs() []hrFieldDef {
	const g = "职工明细"
	return []hrFieldDef{
		hrF("职工账号", "text", hrHeader(), hrBuiltin(), hrGroup(g)),
		hrF("职工姓名", "text", hrHeader(), hrBuiltin(), hrGroup(g)),
		hrF("证件号码", "text", hrHeader(), hrGroup(g)),
		hrF("账户状态", "system", hrHeader(), hrBuiltin(), hrEnum("账户状态"), hrGroup(g)),
		hrF("缴存基数(元)", "number", hrHeader(), hrGroup(g)),
		hrF("月缴存额(元)", "number", hrHeader(), hrGroup(g)),
		hrF("单位月缴存额(元)", "number", hrHeader(), hrGroup(g)),
		hrF("个人月缴存额(元)", "number", hrHeader(), hrGroup(g)),
		hrF("账户余额(元)", "number", hrHeader(), hrGroup(g)),
		hrF("单位缴存比例(%)", "number", hrHeader(), hrDefault("5"), hrGroup(g)),
		hrF("职工缴存比例(%)", "number", hrHeader(), hrDefault("5"), hrGroup(g)),
		hrF("联名卡银行", "enum", hrHeader(), hrEnum("联名卡银行"), hrGroup(g)),
		hrF("联名卡号", "text", hrHeader(), hrGroup(g)),
		hrF("绑定手机", "text", hrHeader(), hrGroup(g)),
		hrF("是否公贷", "enum", hrEnum("是否公贷"), hrGroup(g)),
		hrF("是否冻结", "enum", hrEnum("是否冻结"), hrGroup(g)),
		hrF("是否可在线提取公贷", "enum", hrEnum("是否可在线提取公贷"), hrGroup(g)),
		hrF("是否可在线提取商贷", "enum", hrEnum("是否可在线提取商贷"), hrGroup(g)),
	}
}

// hrFundBillFieldDefs 月度账单 8 字段（第 55 轮，与职工明细分属两个胶囊分组）。
func hrFundBillFieldDefs() []hrFieldDef {
	return []hrFieldDef{
		hrF("账单周期", "text", hrHeader(), hrBuiltin(), hrGroup("月度账单")),
		hrF("汇缴人数", "number", hrHeader(), hrGroup("月度账单")),
		hrF("缴存基数合计(元)", "number", hrHeader(), hrGroup("月度账单")),
		hrF("单位缴存额(元)", "number", hrHeader(), hrGroup("月度账单")),
		hrF("个人缴存额(元)", "number", hrHeader(), hrGroup("月度账单")),
		hrF("月缴存总额(元)", "number", hrHeader(), hrGroup("月度账单")),
		hrF("账单状态", "system", hrHeader(), hrBuiltin(), hrEnum("账单状态"), hrGroup("月度账单")),
		hrF("生成时间", "date", hrHeader(), hrGroup("月度账单")),
	}
}

// ---------------------------------------------------------------------------
// 社保管理四胶囊字段契约（第 56 轮）
//
// 与前端 hrFieldSchema.ts 的 HR_SOCI_GROUPS 一一对应：
// 回盘表 28 字段 / 正常记录 16 / 异常记录 16 / 月度账单 16。
// 正常与异常记录字段名完全相同，靠 Group 区分；hrSocialSeedSubs 用
// (group, name) 复合键去重，避免第二组的 16 个字段被吞掉。
// ---------------------------------------------------------------------------

const (
	hrSocReceiptGroup  = "回盘表"
	hrSocNormalGroup   = "正常记录"
	hrSocAbnormalGroup = "异常记录"
	hrSocBillGroup     = "月度账单"
)

func hrSocialReceiptFieldDefs() []hrFieldDef {
	const g = hrSocReceiptGroup
	status := func(name string) hrFieldDef {
		return hrF(name, "system", hrHeader(), hrBuiltin(), hrEnum("参保状态"), hrGroup(g))
	}
	return []hrFieldDef{
		hrF("个人编号", "text", hrHeader(), hrBuiltin(), hrGroup(g)),
		hrF("姓名", "text", hrHeader(), hrBuiltin(), hrGroup(g)),
		hrF("身份证号", "text", hrHeader(), hrGroup(g)),
		// 部门是跨表引用：值来自花名册，导入模板里不存在这一列。
		hrF("部门", "readonly", hrHeader(), hrRef("花名册", "部门"), hrGroup(g)),
		hrF("证件种类", "enum", hrEnum("证件种类"), hrGroup(g)),
		hrF("性别", "enum", hrHeader(), hrEnum("性别"), hrGroup(g)),
		hrF("民族", "enum", hrEnum("民族"), hrGroup(g)),
		hrF("户口性质", "enum", hrHeader(), hrEnum("社保户口性质"), hrGroup(g)),
		hrF("出生日期", "date", hrGroup(g)),
		hrF("工作日期", "date", hrGroup(g)),
		status("养老参保状态"),
		hrF("养老缴费基数", "number", hrHeader(), hrGroup(g)),
		hrF("养老参保时间", "date", hrGroup(g)),
		status("失业参保状态"),
		hrF("失业缴费基数", "number", hrGroup(g)),
		hrF("失业参保时间", "date", hrGroup(g)),
		status("医疗参保状态"),
		hrF("医疗缴费基数", "number", hrGroup(g)),
		hrF("医疗参保时间", "date", hrGroup(g)),
		status("工伤参保状态"),
		hrF("工伤缴费基数", "number", hrGroup(g)),
		hrF("工伤参保时间", "date", hrGroup(g)),
		status("生育参保状态"),
		hrF("生育缴费基数", "number", hrGroup(g)),
		hrF("生育参保时间", "date", hrGroup(g)),
		hrF("工资", "number", hrGroup(g)),
		hrF("个人身份", "enum", hrHeader(), hrEnum("个人身份"), hrGroup(g)),
		hrF("备注", "text", hrGroup(g)),
	}
}

func hrSocialRecordFieldDefs(group string) []hrFieldDef {
	return []hrFieldDef{
		hrF("账单月份", "text", hrHeader(), hrBuiltin(), hrGroup(group)),
		hrF("费款所属期", "text", hrHeader(), hrGroup(group)),
		hrF("个人编号", "text", hrHeader(), hrGroup(group)),
		hrF("姓名", "text", hrHeader(), hrBuiltin(), hrGroup(group)),
		hrF("身份证号", "text", hrHeader(), hrGroup(group)),
		hrF("部门", "readonly", hrHeader(), hrRef("花名册", "部门"), hrGroup(group)),
		hrF("险种类型", "enum", hrHeader(), hrEnum("险种类型"), hrGroup(group)),
		hrF("缴费基数", "number", hrHeader(), hrGroup(group)),
		hrF("单位缴费比例", "text", hrGroup(group)),
		hrF("单位应缴金额", "number", hrHeader(), hrGroup(group)),
		hrF("个人缴费比例", "text", hrGroup(group)),
		hrF("个人应缴金额", "number", hrHeader(), hrGroup(group)),
		hrF("应缴合计", "number", hrHeader(), hrGroup(group)),
		hrF("补退类型", "enum", hrHeader(), hrEnum("补退类型"), hrGroup(group)),
		hrF("记录类型", "text", hrGroup(group)),
		hrF("源文件", "text", hrGroup(group)),
	}
}

func hrSocialBillFieldDefs() []hrFieldDef {
	const g = hrSocBillGroup
	return []hrFieldDef{
		hrF("账单周期", "text", hrHeader(), hrBuiltin(), hrGroup(g)),
		hrF("参保人数", "number", hrHeader(), hrGroup(g)),
		hrF("缴费基数合计", "number", hrHeader(), hrGroup(g)),
		hrF("单位养老", "number", hrHeader(), hrGroup(g)),
		hrF("单位医疗+生育", "number", hrHeader(), hrGroup(g)),
		hrF("单位工伤", "number", hrHeader(), hrGroup(g)),
		hrF("单位失业", "number", hrHeader(), hrGroup(g)),
		hrF("社保单位金额", "number", hrHeader(), hrGroup(g)),
		hrF("个人养老", "number", hrHeader(), hrGroup(g)),
		hrF("个人医疗", "number", hrHeader(), hrGroup(g)),
		hrF("个人大额医疗", "number", hrHeader(), hrGroup(g)),
		hrF("个人失业", "number", hrHeader(), hrGroup(g)),
		hrF("社保个人金额", "number", hrHeader(), hrGroup(g)),
		hrF("应缴总计", "number", hrHeader(), hrGroup(g)),
		hrF("账单状态", "system", hrHeader(), hrBuiltin(), hrEnum("社保账单状态"), hrGroup(g)),
		hrF("生成时间", "date", hrHeader(), hrGroup(g)),
	}
}

func hrModuleFieldDefs(module string) []hrFieldDef {
	switch module {
	case hrModuleResign:
		return hrResignFieldDefs()
	case hrModuleContract:
		return hrContractFieldDefs()
	case hrModuleFund:
		return hrFundFieldDefs()
	case hrModuleSocial:
		out := hrSocialReceiptFieldDefs()
		out = append(out, hrSocialRecordFieldDefs(hrSocNormalGroup)...)
		out = append(out, hrSocialRecordFieldDefs(hrSocAbnormalGroup)...)
		out = append(out, hrSocialBillFieldDefs()...)
		return out
	default:
		return hrRosterFieldDefs()
	}
}

// ---------------------------------------------------------------------------
// 枚举字典（复用 hr_roster_fields.go 的 20 组 + 第 52 轮补齐项）
// ---------------------------------------------------------------------------

var hrYesNo = []string{"是", "否"}

var hrExtraEnums = map[string][]string{
	"招聘渠道":        {"社招", "校招", "内部推荐", "猎头", "劳务派遣", "其他"},
	"试用期":         {"无试用期", "1个月", "2个月", "3个月", "4个月", "5个月", "6个月", "7个月", "8个月", "9个月", "10个月", "11个月", "12个月"},
	"证件类型":        {"身份证", "港澳居民来往内地通行证", "台湾居民来往内地通行证", "外国护照", "香港身份证", "台湾身份证", "澳门身份证", "外国人永久居留证", "其他"},
	"性别":          {"男", "女"},
	"是否已婚":        {"未婚", "已婚", "离异", "丧偶"},
	"户口性质":        {"城镇户口", "农村户口", "居民户口"},
	"服装尺寸":        {"S", "M", "L", "XL", "2XL", "3XL", "4XL", "5XL"},
	"紧急联系人关系":     {"父母", "配偶", "子女", "兄弟姐妹", "亲属", "朋友", "其他"},
	"银行名称":        {"中国建设银行", "中国银行", "中国工商银行", "中国农业银行", "农村商业银行", "招商银行", "交通银行", "邮政储蓄银行", "其他"},
	"是否本地首次缴纳社保":  hrYesNo,
	"是否本地首次缴纳公积金": hrYesNo,
	"离职类型":        {"主动离职", "被动离职", "退休", "自动离职", "其他"},
	"是否试用期内离职":    hrYesNo,
	"合同类型":        {"固定期限劳动合同", "无固定期限劳动合同", "以完成一定工作任务为期限的劳动合同", "实习协议", "劳务合同", "返聘协议", "劳务派遣合同", "借调合同", "其他"},
	"合同期限":        {"1年", "2年", "3年", "4年", "5年", "6年", "7年", "8年", "9年", "10年", "无固定期限"},
	// 第 54 轮：合同状态扩到 5 态。已解除 = 员工已离职；未签订 = 在职但没填合同起始/结束日期。
	"合同状态": {"正常", "未签订", "即将到期", "到期", "已解除"},
	"账户状态": {"正常", "封存", "提取销户"},
	"账单状态": {"已汇缴", "待汇缴", "已作废"},
	"联名卡银行": {"中国建设银行", "中国银行", "中国工商银行", "中国农业银行",
		"招商银行", "交通银行", "农村商业银行", "其他"},
	"是否公贷":      hrYesNo,
	"是否冻结":      hrYesNo,
	"是否可在线提取公贷": hrYesNo,
	"是否可在线提取商贷": hrYesNo,
	// 第 56 轮：社保四胶囊枚举（与花名册「户口性质」字典严格隔离）
	"社保户口性质": {"本省农村", "本省城镇", "外省农村", "外省城镇"},
	"个人身份":   {"工人", "农民", "职员", "其他"},
	"证件种类":   {"01-居民身份证", "02-港澳居民来往内地通行证", "03-护照", "99-其他"},
	"参保状态":   {"参保缴费", "暂停缴费", "终止缴费"},
	"险种类型":   {"养老保险", "医疗保险", "生育保险", "大额医疗", "工伤保险", "失业保险"},
	"补退类型":   {"正常", "补缴", "退费"},
	"社保账单状态": {"已申报", "待申报", "已作废"},
}

// hrEnumOptions 字段 → 枚举选项；extra 优先覆盖同名的旧字典。
func hrEnumOptions(field string) []string {
	if opts, ok := hrExtraEnums[field]; ok {
		return opts
	}
	if opts, ok := hrRosterEnumGroups[field]; ok {
		return opts
	}
	// 兼容模板里「合同期限」等新字段名映射到旧字典 key
	if opts, ok := hrRosterEnumGroups[hrRosterEnumKeyOf(field)]; ok {
		return opts
	}
	return nil
}

// ---------------------------------------------------------------------------
// 分类种子 subs
// ---------------------------------------------------------------------------

func hrDataTypeOfDef(d hrFieldDef) string {
	switch d.Type {
	case "date":
		return "date"
	case "number":
		return "number"
	case "textarea":
		// 第 57 轮：说明类字段保留多行语义，前端设置面板与编辑抽屉都按 textarea 渲染。
		return "textarea"
	case "attachment":
		// 附件字段在抽屉里走业务插槽（附件控制条），列定义仍按文本存路径。
		return "attachment"
	default:
		return "text"
	}
}

func hrSubOfDef(d hrFieldDef) types.FleetCategorySub {
	s := types.FleetCategorySub{
		Name:       d.Name,
		Enabled:    true,
		IsDefault:  d.DefaultHeader,
		Width:      0,
		DataType:   hrDataTypeOfDef(d),
		Group:      d.Group,
		GroupLabel: d.Group,
		Builtin:    d.Builtin,
	}
	if d.Source != nil {
		s.Source = &types.FleetCategorySubSource{
			Category: d.Source.Category,
			Field:    d.Source.Field,
			Mode:     "field",
			Label:    d.Source.Field,
		}
	}
	return s
}

func hrSubsOfDefs(defs []hrFieldDef) []types.FleetCategorySub {
	// 社保四胶囊里「正常记录」与「异常记录」字段名完全相同，必须按
	// (group, name) 复合键去重，否则后 16 个字段会被 seen[name] 吞掉。
	return hrSubsOfDefsByGroup(defs, true)
}

func hrSubsOfDefsByGroup(defs []hrFieldDef, byGroup bool) []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, len(defs))
	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if d.Name == "" {
			continue
		}
		key := d.Name
		if byGroup {
			key = d.Group + "\x00" + d.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hrSubOfDef(d))
	}
	return out
}

func hrRosterSeedSubs52() []types.FleetCategorySub {
	return hrSubsOfDefs(hrRosterFieldDefs())
}

// 花名册设置抽屉的两个胶囊分组（第 57 轮）。
const (
	hrRosterGroupActive   = "在职管理"
	hrRosterGroupResigned = "离职管理"
)

// hrRosterSeedSubs57 花名册分类 = 在职 48 字段 + 离职 14 字段，两个胶囊分组。
//
// 为什么合并进同一个分类：顶部「离职管理」卡片已并入花名册（在职/离职双标签），
// 离职管理不再有独立设置入口；离职字段必须挂在花名册分类下，用户切到离职标签
// 仍能通过同一个「花名册设置」抽屉维护表头与默认表头。
//
// 分组键用 Group（字段配置胶囊按 group 聚合），花名册自己的 6 个业务分组仍由
// 前端 hrRosterGroups 的静态结构驱动编辑抽屉分块，两者互不冲突。
func hrRosterSeedSubs57() []types.FleetCategorySub {
	active := hrRosterFieldDefs()
	for i := range active {
		active[i].Group = hrRosterGroupActive
	}
	resigned := hrResignFieldDefs()
	for i := range resigned {
		resigned[i].Group = hrRosterGroupResigned
	}
	return hrSubsOfDefsByGroup(append(active, resigned...), true)
}
func hrResignSeedSubs() []types.FleetCategorySub {
	return hrSubsOfDefs(hrResignFieldDefs())
}
func hrContractSeedSubs52() []types.FleetCategorySub {
	return hrSubsOfDefs(hrContractFieldDefs())
}
func hrFundSeedSubs() []types.FleetCategorySub {
	defs := append(append([]hrFieldDef{}, hrFundBillFieldDefs()...), hrFundFieldDefs()...)
	return hrSubsOfDefs(defs)
}

// hrSocialSeedSubs 四胶囊顺序 = 页面四标签顺序（回盘表 / 正常 / 异常 / 月度账单）。
func hrSocialSeedSubs() []types.FleetCategorySub {
	defs := hrSocialReceiptFieldDefs()
	defs = append(defs, hrSocialRecordFieldDefs(hrSocNormalGroup)...)
	defs = append(defs, hrSocialRecordFieldDefs(hrSocAbnormalGroup)...)
	defs = append(defs, hrSocialBillFieldDefs()...)
	return hrSubsOfDefsByGroup(defs, true)
}

// hrRosterGroupDict52 6 大分组 → 字段名（供回填与排序）。
func hrRosterGroupDict52() []struct {
	Key    string
	Fields []string
} {
	out := make([]struct {
		Key    string
		Fields []string
	}, 0, len(hrRosterGroupOrder))
	for _, key := range hrRosterGroupOrder {
		fields := make([]string, 0)
		for _, d := range hrRosterFieldDefs() {
			if d.Group == key {
				fields = append(fields, d.Name)
			}
		}
		out = append(out, struct {
			Key    string
			Fields []string
		}{key, fields})
	}
	return out
}

// hrRosterFieldAllowed 是否属于精简后的花名册字段（存量清理用）。
func hrRosterFieldAllowed(name string) bool {
	for _, d := range hrRosterFieldDefs() {
		if d.Name == name {
			return true
		}
	}
	return false
}

// hrContractAliasToLegacy 新合同字段名 → 存量 custom_data 键（读写双向兼容）。
var hrContractAliasToLegacy = map[string][]string{
	"合同编号":   {"现合同编号"},
	"合同类型":   {"现合同类型"},
	"合同开始日期": {"现合同开始日期"},
	"合同结束日期": {"现合同结束日期"},
	"合同期限":   {"现合同期限"},
	"签约公司":   {"现合同签约公司", "合同签约公司"},
	"签订日期":   {"现合同签订日期"},
	"联系电话":   {"手机号码"},
}

// hrFieldReadKeys 读取候选键：新名优先，兼容存量键。
func hrFieldReadKeys(name string) []string {
	return append([]string{name}, hrContractAliasToLegacy[name]...)
}

// hrFieldPersistKeys 保存时要写的键（新名 + 存量别名都写，保证两种读法都命中）。
func hrFieldPersistKeys(name string) []string {
	keys := hrFieldReadKeys(name)
	seen := map[string]bool{}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------------------
// 三套 Excel 导入模板
// ---------------------------------------------------------------------------

var hrTemplateMatchKeys = []string{"工号", "姓名", "证件号码"}

// hrTemplateSpec 一个模块的标准模板定义。
type hrTemplateSpec struct {
	Module  string
	Name    string
	Headers []string
	Groups  []struct {
		Key    string
		Fields []string
	}
	EnumGroups []string
	Notes      []string
}

func hrTemplateHeaders(module string) []string {
	switch module {
	case hrModuleResign:
		return append(append([]string{}, hrTemplateMatchKeys...),
			"离职申请日期", "离职日期", "离职类型", "离职原因", "离职原因说明",
			"离职去向", "是否试用期内离职", "薪资结算日")
	case hrModuleContract:
		return append(append([]string{}, hrTemplateMatchKeys...),
			"合同编号", "合同类型", "合同开始日期", "合同结束日期", "合同期限", "签约公司", "签订日期")
	case hrModuleFund:
		defs := hrFundFieldDefs()
		out := make([]string, 0, len(defs))
		for _, d := range defs {
			out = append(out, d.Name)
		}
		return out
	case hrModuleSocial:
		// 回盘表模板 = 27 个源表原生字段（部门是跨表引用，不进模板）
		defs := hrSocialReceiptFieldDefs()
		out := make([]string, 0, len(defs))
		for _, d := range defs {
			if d.Source != nil {
				continue
			}
			out = append(out, d.Name)
		}
		return out
	default:
		// 第 54 轮铁律：跨模块引用字段（个人社保账号 / 个人公积金账号）只读联动，
		// 绝不进导入模板、也不参与导入解析 —— 否则用户一份表就能把源表数据改坏。
		defs := hrRosterFieldDefs()
		out := make([]string, 0, len(defs))
		for _, d := range defs {
			if d.Source != nil {
				continue
			}
			out = append(out, d.Name)
		}
		return out
	}
}

func hrTemplateNotes(module string) []string {
	base := []string{
		"填写说明",
		"1. 表头名称须与本模板一致；合并单元格、复合表头无法识别",
		"2. 「姓名」必填且不可删除，已在表头中标红加粗",
		"3. 「工号 / 证件号码」用于匹配员工，至少填写其一；证件类型默认为身份证",
		"4. 带下拉的枚举列仅可选择模板支持的值，完整清单见「枚举字段」Sheet",
		"5. 日期类字段填写 YYYY-MM-DD；公式无法识别",
		"6. 证件号码 / 手机号码 / 工资卡卡号中的空格与横杠会被自动清除",
		"7. 示例行导入时会自动跳过",
	}
	if module == hrModuleResign {
		base = append(base, "8. 导入后自动按证件号码 / 工号匹配员工，更新为离职状态并落库离职字段")
	}
	if module == hrModuleContract {
		base = append(base, "8. 导入后自动按证件号码 / 工号匹配员工，更新劳动合同台账并重算合同状态")
	}
	if module == hrModuleFund {
		base = append(base,
			"8. 支持直接上传公积金网厅导出的《单位职工明细》：自动跳过标题行与末行验证码",
			"9. 职工账号为纯文本，请勿加千分位逗号；导入后自动回显到花名册「个人公积金账号」",
		)
	}
	return base
}

func hrTemplateSpecOf(module string) hrTemplateSpec {
	if module != hrModuleResign && module != hrModuleContract && module != hrModuleFund && module != hrModuleSocial {
		module = hrModuleRoster
	}
	headers := hrTemplateHeaders(module)
	spec := hrTemplateSpec{
		Module:  module,
		Name:    hrTemplateFileName(module),
		Headers: headers,
		Notes:   hrTemplateNotes(module),
	}
	// 分组清单（「说明」Sheet）
	if module == hrModuleRoster {
		for _, g := range hrRosterGroupDict52() {
			fields := make([]string, 0, len(g.Fields))
			for _, name := range g.Fields {
				// 与表头一致：引用字段不进模板说明
				if hrFieldIsRef("roster", name) {
					continue
				}
				fields = append(fields, name)
			}
			spec.Groups = append(spec.Groups, struct {
				Key    string
				Fields []string
			}{g.Key, fields})
		}
	} else {
		groupName := "离职信息"
		if module == hrModuleContract {
			groupName = "合同信息"
		}
		if module == hrModuleFund {
			groupName = "公积金信息"
		}
		if module == hrModuleSocial {
			groupName = "社保回盘信息"
		}
		spec.Groups = append(spec.Groups, struct {
			Key    string
			Fields []string
		}{groupName, headers})
	}
	// 枚举列（只列出该模板表头里真实存在的枚举字段）
	inHeader := map[string]bool{}
	for _, h := range headers {
		inHeader[h] = true
	}
	order := []string{
		"招聘渠道", "试用期", "证件类型", "性别", "是否已婚", "民族", "政治面貌",
		"户口性质", "服装尺寸", "紧急联系人关系", "最高学历", "银行名称",
		"是否本地首次缴纳社保", "是否本地首次缴纳公积金",
		"离职类型", "离职原因", "是否试用期内离职", "合同类型", "合同期限", "合同状态",
		"账户状态", "联名卡银行", "是否公贷", "是否冻结",
		"是否可在线提取公贷", "是否可在线提取商贷",
	}
	for _, key := range order {
		if inHeader[key] && len(hrEnumOptions(key)) > 0 {
			spec.EnumGroups = append(spec.EnumGroups, key)
		}
	}
	return spec
}

// hrFieldIsRef 字段是否跨模块引用（引用字段不进导入模板）。
func hrFieldIsRef(module, name string) bool {
	for _, d := range hrModuleFieldDefs(module) {
		if d.Name == name {
			return d.Source != nil
		}
	}
	return false
}

func hrTemplateFileName(module string) string {
	switch module {
	case hrModuleResign:
		return "离职管理导入模板.xlsx"
	case hrModuleContract:
		return "劳动合同导入模板.xlsx"
	case hrModuleFund:
		return "公积金管理导入模板.xlsx"
	case hrModuleSocial:
		return "社保回盘表导入模板.xlsx"
	default:
		return "花名册导入模板.xlsx"
	}
}

// hrTemplateSample 模板第 2 行示例（按表头顺序生成，导入器自动跳过）。
func hrTemplateSample(module string, headers []string) []string {
	sample := map[string]string{
		"工号": "000001", "姓名": "示例员工", "证件号码": "51072319850217371X",
		"证件类型": "身份证", "部门": "技术质量部", "岗位": "工程师", "入职日期": "2018-07-01",
		"手机号码": "13800138000", "性别": "男", "出生日期": "1985-02-17", "年龄": "41.7",
		"最高学历": "本科", "民族": "汉族", "政治面貌": "群众", "是否已婚": "已婚",
		"户口性质": "城镇户口", "招聘渠道": "社招", "试用期": "3个月", "转正日期": "2018-10-01",
		"离职日期": "2026-10-31", "离职类型": "主动离职", "离职原因": "个人发展",
		"是否试用期内离职": "否", "薪资结算日": "2026-11-10",
		"合同类型": "固定期限劳动合同", "合同开始日期": "2024-01-01",
		"合同结束日期": "2027-12-31", "合同期限": "3年", "签订日期": "2023-12-20",
		"职工账号": "801504241502", "职工姓名": "示例员工", "账户状态": "正常",
		"缴存基数(元)": "6000", "月缴存额(元)": "600", "单位月缴存额(元)": "300",
		"个人月缴存额(元)": "300", "账户余额(元)": "18600",
		"单位缴存比例(%)": "5", "职工缴存比例(%)": "5",
		"联名卡银行": "中国建设银行", "联名卡号": "6217003760153123456",
		"绑定手机": "13800138000", "是否公贷": "否", "是否冻结": "否",
		"是否可在线提取公贷": "是", "是否可在线提取商贷": "否",
	}
	out := make([]string, 0, len(headers))
	for _, h := range headers {
		out = append(out, sample[h])
	}
	return out
}

// hrLegacyRosterFields 第 52 轮从花名册彻底移除的存量内置字段。
// 启动回填时只剔除这批已知废弃字段，用户在设置里自行新增的自定义字段一律保留。
var hrLegacyRosterFields = map[string]bool{}
var hrLegacyRosterFieldNames = []string{
	// 在职信息组（已精简为 10 项）
	"职务", "员工状态", "在职状态", "人员状态", "汇报上级类型", "汇报上级姓名", "备注",
	"多级部门名称", "岗位类别", "汇报上级", "职等", "职级编号", "职级名称",
	"工作城市", "工作地点", "考勤编号", "工时制度", "工作性质",
	"档案编号", "档案备注", "OA编码",
	// 基本信息组（已精简为 13 项）
	"是否已育", "参加工作时间", "血型", "健康状况", "健康证有效期",
	"曾用名", "英文名", "是否农历生日", "国家地区", "籍贯(省)", "籍贯(市)",
	"户籍城市", "户籍城市(省)", "户籍城市(市)",
	// 离职信息 → 独立「离职管理」模块
	"离职日期", "离职申请日期", "薪资结算日", "离职类型", "离职原因",
	"离职原因说明", "离职备注", "离职去向", "是否试用期内离职", "离职证明",
	// 家庭情况（整组删除）
	"婚姻状况", "结婚时间", "子女数", "配偶姓名", "配偶户籍城市", "配偶文化程度",
	// 教育情况精简
	"教育开始时间", "教育结束时间", "学习形式", "学位类型",
	// 工作经历情况（整组删除）
	"上家工作单位", "上家工作职务",
	// 合同签订情况 → 独立「劳动合同」模块
	"现合同编号", "现合同类型", "现合同开始日期", "现合同结束日期", "现合同期限",
	"现合同签约公司", "合同签约公司", "现合同签订日期", "合同签订次数", "合同状态",
	// 个税申报信息（整组删除）
	"是否烈属", "烈属证号", "是否孤老", "是否特定行业", "是否股东/投资者", "个人股本(投资)额",
	// 历史重复 / 合并下线列
	"生日", "工龄", "司龄开始日期", "司龄(天数)", "身份证号码", "出生月份", "联系电话",
}

// hrLegacyContractFields 劳动合同视图里被第 52 轮新字段名替换的旧列。
var hrLegacyContractFields = map[string]bool{}
var hrLegacyContractFieldNames = []string{
	"职务", "入职日期", "合同签约公司",
	"现合同编号", "现合同类型", "现合同开始日期", "现合同结束日期",
	"现合同期限", "现合同签订日期",
}

func init() {
	for _, n := range hrLegacyRosterFieldNames {
		hrLegacyRosterFields[n] = true
	}
	for _, n := range hrLegacyContractFieldNames {
		hrLegacyContractFields[n] = true
	}
}

// hrCleanImportValue 导入时的字段清洗（与前端 hrCleanValue 同口径）。
func hrCleanImportValue(name, raw string) string {
	s := strings.TrimSpace(stripBOM(raw))
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff', '\u00a0', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
	switch name {
	case "证件号码", "手机号码", "工资卡卡号", "紧急联系人电话", "工作电话":
		s = strings.NewReplacer(" ", "", "-", "").Replace(s)
	}
	if name == "证件号码" && strings.HasSuffix(s, "x") {
		s = s[:len(s)-1] + "X"
	}
	return strings.TrimSpace(s)
}
