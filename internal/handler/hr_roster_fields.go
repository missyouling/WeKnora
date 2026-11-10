// 花名册标准模板字段体系（第 46 轮）。
//
// 唯一数据源：.workbuddy/plans/roster-template.json（《导入花名册模板.xlsx》）。
// 本文件由 .workbuddy/plans/gen_round46_data.py 生成，禁止手改常量顺序。
package handler

import (
	"regexp"
	"strings"
)

// hrRosterHeaders 是 `导入数据` Sheet 的 76 个标准表头（精确顺序）。
var hrRosterHeaders = []string{
	"工号",
	"姓名",
	"曾用名",
	"英文名",
	"部门",
	"汇报上级类型",
	"汇报上级姓名",
	"职务",
	"岗位",
	"职级编号",
	"手机号码",
	"证件类型",
	"证件号码",
	"工作性质",
	"员工状态",
	"入职日期",
	"试用期",
	"转正日期",
	"工作城市",
	"工作地点",
	"合同公司",
	"考勤编号",
	"招聘渠道",
	"离职日期",
	"离职原因",
	"档案编号",
	"身份证有效期",
	"性别",
	"出生日期",
	"是否农历生日",
	"是否已婚",
	"是否已育",
	"国家地区",
	"民族",
	"政治面貌",
	"籍贯",
	"户籍城市",
	"户口性质",
	"户籍地址",
	"参加工作时间",
	"血型",
	"服装尺寸",
	"健康证有效期",
	"QQ号",
	"微信号",
	"个人邮箱",
	"工作电话",
	"分机号",
	"工作邮箱",
	"现居住地",
	"工资卡卡号",
	"银行名称",
	"工资卡开户行",
	"工资卡开户城市",
	"个人社保账号",
	"个人公积金账号",
	"紧急联系人姓名",
	"紧急联系人关系",
	"紧急联系人电话",
	"紧急联系人地址",
	"最高学历",
	"毕业院校",
	"毕业专业",
	"教育开始时间",
	"教育结束时间",
	"学习形式",
	"学位类型",
	"上家工作单位",
	"上家工作职务",
	"合同签约公司",
	"现合同编号",
	"现合同类型",
	"现合同开始日期",
	"现合同结束日期",
	"现合同期限",
	"现合同签订日期",
}

// hrRosterDerivedFields 是不在 76 表头内的派生/系统字段。
var hrRosterDerivedFields = []string{
	"年龄",
	"工龄",
	"在职状态",
	"出生日期",
}

// hrRosterExtraResignFields 是办理离职时新增的 2 个扩展字段。
var hrRosterExtraResignFields = []string{
	"离职证明",
	"离职备注",
}

// hrRosterAliases 把旧表头 / 常见别名归一到 76 表头标准名。
var hrRosterAliases = map[string]string{
	"上家工作单位名称": "上家工作单位",
	"上家工作单位职务": "上家工作职务",
	"合同类型":     "现合同类型",
	"合同编号":     "现合同编号",
	"员工状态":     "员工状态",
	"在职状态":     "员工状态",
	"手机号":      "手机号码",
	"联系电话":     "手机号码",
	"身份证号":     "证件号码",
	"身份证号码":    "证件号码",
	"姓名":       "姓名",
	"证件号码":     "证件号码",
}

// hrRosterEnumGroups 是 `枚举字段` Sheet 的 20 组标准下拉选项。
var hrRosterEnumGroups = map[string][]string{
	"证件类型": []string{
		"身份证",
		"港澳居民来往内地通行证",
		"台湾居民来往内地通行证",
		"外国护照",
		"其他",
		"香港身份证",
		"台湾身份证",
		"澳门身份证",
		"外国人永久居留证",
	},
	"工作性质": []string{
		"全职",
		"实习生",
		"兼职",
		"劳务派遣",
		"劳务",
		"派遣",
		"外包",
		"退休返聘",
		"小时工",
		"临时工",
		"社会兼职",
		"学生兼职",
		"借调",
		"其他",
	},
	"员工状态": []string{
		"正式",
		"试用",
		"待离职",
		"已离职",
	},
	"试用期": []string{
		"无试用期",
		"1个月",
		"2个月",
		"3个月",
		"4个月",
		"5个月",
		"6个月",
		"7个月",
		"8个月",
		"9个月",
		"10个月",
		"11个月",
		"12个月",
	},
	"离职原因": []string{
		"家庭原因",
		"身体原因",
		"薪资原因",
		"交通不便",
		"工作压力",
		"管理问题",
		"无晋升机会",
		"职业规划",
		"合同到期放弃续签",
		"其他个人原因",
		"自动离职",
		"不能胜任工作",
		"回家照顾家人",
		"个人创业",
		"回家乡发展",
		"不认同企业文化",
		"学习深造",
		"回家生小孩",
		"试用期劝退",
		"体检不合格",
		"入职资料有问题",
		"背景调查不通过",
		"自离（不计薪资）",
		"试用期内辞退",
		"违反公司调理",
		"组织调整/裁员",
		"绩效不达标辞退",
		"合同到期不续签",
		"其他原因被动离职",
		"严重违规违纪",
		"造成重大损害",
		"实习期结束",
		"兼职期结束",
		"提供虚假材料",
		"工作能力不胜任",
		"内部调动",
		"法定退休",
		"死亡",
		"坐牢",
		"连续旷工",
		"失联",
		"停薪留职期满不归",
	},
	"性别": []string{
		"男",
		"女",
	},
	"是否农历生日": []string{
		"否",
		"是",
	},
	"是否已婚": []string{
		"未婚",
		"已婚",
		"离异",
		"丧偶",
	},
	"是否已育": []string{
		"否",
		"是",
	},
	"国家地区": []string{
		"中国",
		"中国香港",
		"中国澳门",
		"中国台湾",
		"美国",
		"新加坡",
		"日本",
		"韩国",
		"马来西亚",
		"德国",
		"印度",
		"泰国",
		"澳大利亚",
		"菲律宾",
		"尼日利亚",
		"保加利亚",
		"英国",
		"越南",
		"加拿大",
		"俄罗斯",
		"哈萨克斯坦",
		"荷兰",
		"比利时",
		"法国",
		"瑞典",
		"墨西哥",
		"阿根廷",
		"巴西",
		"意大利",
		"缅甸",
		"乌克兰",
		"老挝",
		"巴基斯坦",
		"也门",
		"孟加拉国",
		"新西兰",
		"南非",
	},
	"民族": []string{
		"汉族",
		"蒙古族",
		"回族",
		"藏族",
		"维吾尔族",
		"苗族",
		"彝族",
		"壮族",
		"布依族",
		"朝鲜族",
		"满族",
		"侗族",
		"瑶族",
		"白族",
		"土家族",
		"哈尼族",
		"哈萨克族",
		"傣族",
		"黎族",
		"傈僳族",
		"佤族",
		"畲族",
		"高山族",
		"拉祜族",
		"水族",
		"东乡族",
		"纳西族",
		"景颇族",
		"柯尔克孜族",
		"土族",
		"达斡尔族",
		"仫佬族",
		"羌族",
		"布朗族",
		"撒拉族",
		"毛南族",
		"仡佬族",
		"锡伯族",
		"阿昌族",
		"普米族",
		"塔吉克族",
		"怒族",
		"乌孜别克族",
		"俄罗斯族",
		"鄂温克族",
		"德昂族",
		"保安族",
		"裕固族",
		"京族",
		"塔塔尔族",
		"独龙族",
		"鄂伦春族",
		"赫哲族",
		"门巴族",
		"珞巴族",
		"基诺族",
		"穿青人",
	},
	"政治面貌": []string{
		"中共党员",
		"共青团员",
		"群众",
		"中共预备党员",
		"民革会员",
		"民盟盟员",
		"民建会员",
		"民进会员",
		"农工党党员",
		"致公党党员",
		"九三学社社员",
		"台盟盟员",
		"无党派民主人士",
	},
	"户口性质": []string{
		"城镇户口",
		"农村户口",
		"居民户口",
	},
	"血型": []string{
		"A型",
		"B型",
		"AB型",
		"O型",
	},
	"最高学历": []string{
		"小学",
		"初中",
		"高中",
		"职业高级中学",
		"中等专业学校",
		"中级技工学校",
		"高级技工学校",
		"中等师范学校",
		"大专",
		"本科",
		"硕士",
		"博士",
		"博士后",
		"其他",
	},
	"学习形式": []string{
		"全日制",
		"成人教育",
		"远程教育",
		"自学考试",
		"其他非全日制",
	},
	"学位类型": []string{
		"学士",
		"硕士",
		"博士",
		"名誉博士",
		"未取得学位",
	},
	"合同类型": []string{
		"固定期限劳动合同",
		"无固定期限劳动合同",
		"以完成一定工作任务为期限的劳动合同",
		"实习协议",
		"劳务合同",
		"返聘协议",
		"劳务派遣合同",
		"借调合同",
		"其他",
	},
	"现合同期限": []string{
		"1年",
		"2年",
		"3年",
		"4年",
		"5年",
		"6年",
		"7年",
		"8年",
		"9年",
		"10年",
	},
	"汇报上级类型": []string{
		"组织负责人",
		"自定义",
	},
}

// hrRosterFieldGroups 是 `说明` Sheet 的 11 大分组（字段已归一到 76 表头）。
type hrRosterFieldGroup struct {
	Group  string
	Fields []string
}

var hrRosterFieldGroups = []hrRosterFieldGroup{
	{Group: "在职信息", Fields: []string{
		"工号",
		"合同公司",
		"部门",
		"汇报上级类型",
		"汇报上级姓名",
		"职务",
		"岗位",
		"职级编号",
		"工作城市",
		"工作地点",
		"考勤编号",
		"招聘渠道",
	}},
	{Group: "员工状态", Fields: []string{
		"工作性质",
		"员工状态",
		"入职日期",
		"试用期",
		"转正日期",
	}},
	{Group: "离职信息", Fields: []string{
		"离职日期",
		"离职原因",
	}},
	{Group: "档案信息", Fields: []string{
		"档案编号",
	}},
	{Group: "个人基本信息", Fields: []string{
		"姓名",
		"曾用名",
		"英文名",
		"证件类型",
		"证件号码",
		"身份证有效期",
		"性别",
		"出生日期",
		"是否农历生日",
		"是否已婚",
		"是否已育",
		"国家地区",
		"民族",
		"政治面貌",
		"籍贯",
		"户籍城市",
		"户口性质",
		"户籍地址",
		"参加工作时间",
		"血型",
		"服装尺寸",
		"健康证有效期",
	}},
	{Group: "教育经历", Fields: []string{
		"最高学历",
		"毕业院校",
		"毕业专业",
		"教育开始时间",
		"教育结束时间",
		"学习形式",
		"学位类型",
	}},
	{Group: "工作经历", Fields: []string{
		"上家工作单位",
		"上家工作职务",
	}},
	{Group: "联系信息", Fields: []string{
		"手机号码",
		"QQ号",
		"微信号",
		"个人邮箱",
		"工作电话",
		"分机号",
		"工作邮箱",
		"现居住地",
	}},
	{Group: "紧急联系人", Fields: []string{
		"紧急联系人姓名",
		"紧急联系人关系",
		"紧急联系人电话",
		"紧急联系人地址",
	}},
	{Group: "工资社保", Fields: []string{
		"工资卡卡号",
		"银行名称",
		"工资卡开户行",
		"工资卡开户城市",
		"个人社保账号",
		"个人公积金账号",
	}},
	{Group: "合同信息", Fields: []string{
		"现合同编号",
		"现合同类型",
		"合同签约公司",
		"现合同开始日期",
		"现合同结束日期",
		"现合同期限",
		"现合同签订日期",
	}},
}

// hrRosterStatusActive / Probation / Reemployed / Resigned 是四态标准值。
const (
	hrRosterStatusActive     = "在职"
	hrRosterStatusProbation  = "试用期"
	hrRosterStatusReemployed = "退休返聘"
	hrRosterStatusResigned   = "离职"
)

// 支持 2018-07-01 / 2018/7/1 / 2018.7.1 三种字面日期前缀，
// 后面可以跟任意时间部分（空格或 T 分隔），一律只取年月日。
var hrISODatePattern = regexp.MustCompile(`^([0-9]{4})[-/.]([0-9]{1,2})[-/.]([0-9]{1,2})`)

// normalizeHRDateValue 把 Excel / ISO / 中文日期统一成 YYYY-MM-DD。
// 典型脏值：2018-07-01 00:00:00、2018/7/1 0:00、2018-07-01T00:00:00+08:00。
func normalizeHRDateValue(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return ""
	}
	// 已有 YYYY-M-D 前缀时直接按字面取年月日，避免 new Date 的时区偏移。
	if m := hrISODatePattern.FindStringSubmatch(s); len(m) == 4 {
		month, day := m[2], m[3]
		if len(month) == 1 {
			month = "0" + month
		}
		if len(day) == 1 {
			day = "0" + day
		}
		return m[1] + "-" + month + "-" + day
	}
	return normalizeDateValue(s)
}

// normalizeHREmployeeStatus 把模板值 / 旧值归一到四态。
// 工作性质=退休返聘 优先，其余按员工状态映射；空值默认在职。
func normalizeHREmployeeStatus(value, workNature string) string {
	if strings.TrimSpace(workNature) == hrRosterStatusReemployed {
		return hrRosterStatusReemployed
	}
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "", "正式", "在职", "active", "employed":
		return hrRosterStatusActive
	case "试用", "试用期", "probation":
		return hrRosterStatusProbation
	case "已离职", "待离职", "离职", "resigned":
		return hrRosterStatusResigned
	case "退休返聘":
		return hrRosterStatusReemployed
	default:
		return s
	}
}

// hrRosterHeaderKey 把任意中文表头归一到 76 表头标准名；未识别时返回原串。
func hrRosterHeaderKey(header string) string {
	h := normalizeEmployeeHeader(header)
	if h == "" {
		return ""
	}
	if key, ok := hrRosterAliasNormalized[h]; ok {
		return key
	}
	for _, std := range hrRosterHeaders {
		if normalizeEmployeeHeader(std) == h {
			return std
		}
	}
	return ""
}

// hrRosterAliasNormalized 预归一别名表（避免每行重复 normalize）。
var hrRosterAliasNormalized = func() map[string]string {
	m := make(map[string]string, len(hrRosterAliases))
	for k, v := range hrRosterAliases {
		m[normalizeEmployeeHeader(k)] = v
	}
	// 76 个标准表头自身也要能命中
	for _, std := range hrRosterHeaders {
		m[normalizeEmployeeHeader(std)] = std
	}
	return m
}()

// hrIsRosterDateField 判断该表头是否为日期/时间/有效期类。
func hrIsRosterDateField(name string) bool {
	return strings.Contains(name, "日期") || strings.Contains(name, "时间") || strings.Contains(name, "有效期")
}

// ---------------------------------------------------------------------------
// 手写逻辑段（不参与 gen_round46_data.py 生成）
// ---------------------------------------------------------------------------

// hrStatusDBValues 把前端四态筛选值展开成数据库里可能出现的全部历史写法，
// 兼容第 45 轮存量数据（active / resigned）与模板导入值（正式 / 试用 / 已离职）。
func hrStatusDBValues(status string) []string {
	switch strings.TrimSpace(status) {
	case hrRosterStatusActive:
		// 第 48 轮：前端在职状态只留「全部 / 在职 / 离职」三项，
		// 「在职」需把试用期与退休返聘一并纳入（任务 §3.2）。
		return []string{
			hrRosterStatusActive, hrRosterStatusProbation, hrRosterStatusReemployed,
			"active", "正式", "试用", "probation",
		}
	case hrRosterStatusProbation:
		return []string{hrRosterStatusProbation, "试用", "probation"}
	case hrRosterStatusReemployed:
		return []string{hrRosterStatusReemployed}
	case hrRosterStatusResigned:
		return []string{hrRosterStatusResigned, "resigned", "已离职", "待离职"}
	default:
		return []string{strings.TrimSpace(status)}
	}
}

// hrMergeEmployeeCustomData 把一组中文键值合并进 custom_data（nil 时先建 map）。
func hrMergeEmployeeCustomData(existing map[string]any, pairs map[string]string) map[string]any {
	out := existing
	if out == nil {
		out = make(map[string]any, len(pairs))
	}
	for k, v := range pairs {
		out[k] = v
	}
	return out
}
