// 三套导入模板下载入口（第 46 轮建立，第 52 轮改为按 module 分派）。
//
// GET /api/v1/fleet/hr/roster/template?module=roster|resign|contract
//
//	roster   → 《花名册导入模板.xlsx》：精简后的 48 个在职字段
//	resign   → 《离职管理导入模板.xlsx》：匹配键 + 8 个离职字段
//	contract → 《劳动合同导入模板.xlsx》：匹配键 + 7 个合同字段
//
// 三套模板都含「导入数据 / 说明 / 枚举字段」3 个 Sheet，生成逻辑收口在
// hr_templates.go，表头与枚举全部来自 hr_field_schema.go 的字段契约。
package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"github.com/Tencent/WeKnora/internal/errors"
)

// hrRosterTemplateNotes 保留历史填写说明文案常量（兼容既有单测与文档）。
var hrRosterTemplateNotes = [][]string{
	{"填写说明"},
	{"1. 导入的 Excel 中表头名称建议和「花名册」表头字段的名称一致；合并单元格和复合表头无法识别"},
	{"2. 「姓名」必填且不可删除，已在表头中标红并加粗"},
	{"3. 「选项」类型字段，仅可选择模板支持的选项值，详细规则见「枚举字段」Sheet"},
	{"4. 公式无法识别，请确保单元格没有包含公式"},
	{"5. 日期类字段请填写 YYYY-MM-DD，系统会自动清洗带时间戳的写法"},
	{"6. 部门上下级用「/」隔开，请从一级部门开始填写"},
	{"7. 示例行（工号 000001 / 示例员工）导入时会自动跳过"},
}

// hrRosterTemplateSample 旧版 76 列全量模板的示例行，仅供「旧模板兼容」回归测试
// 构造输入使用；第 52 轮的三套模板示例行由 hrTemplateSample 按表头动态生成。
var hrRosterTemplateSample = []string{
	"000001", "示例员工", "张三", "Tom", "互联网部/产品部/产品一组", "组织负责人", "",
	"产品部经理", "产品经理", "B1", "13012340000", "身份证", "440305199603051319",
	"全职", "正式", "2018-07-01", "2个月", "2018-09-01", "广东/深圳",
	"广东省深圳市南山区", "XXXX有限公司", "11010101", "校招", "", "", "22020202",
	"2024-12-12", "女", "1996-03-05", "否", "未婚", "否", "中国", "汉族", "群众",
	"湖北/武汉", "广东/深圳", "城镇户口", "广东省深圳市南山区XXXX", "2017-07-01",
	"A型", "M", "2017-12-12", "123456", "wx888888", "123456@qq.com",
	"0755-2342343", "8803", "DDD@UBM.com", "广东省深圳市南山区XX小区A单元1203",
	"6216610188888888888", "中国银行", "中国银行高新区支行", "广东/深圳",
	"6217214888888888888", "6217003528888888888", "王小非", "父女",
	"13890780000", "广东省深圳市宝安区A小区C单元709", "本科", "深圳大学",
	"计算机科学与技术", "2013-09-01", "2017-06-30", "全日制", "学士",
	"点米科技", "产品经理", "XXXX有限公司", "201807010003", "固定期限劳动合同",
	"2018-07-01", "2020-06-30", "2年", "2018-07-01",
}

// DownloadRosterTemplate 按 module 返回对应模块的专属 Excel 模板。
// 不传 module 时保持历史行为（花名册），确保旧链接不 404。
func (h *HRHandler) DownloadRosterTemplate(c *gin.Context) {
	module := strings.TrimSpace(c.Query("module"))
	if module == "" {
		module = hrModuleRoster
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := hrWriteTemplate(f, module); err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}
	name := hrTemplateFileName(module)
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s-template.xlsx"; filename*=UTF-8''%s`,
		module, url.PathEscape(name),
	))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// hrRosterEnumOrder 保留历史 20 组顺序常量（枚举 Sheet 顺序稳定用）。
var hrRosterEnumOrder = []string{
	"证件类型", "工作性质", "员工状态", "试用期", "离职原因", "性别",
	"是否农历生日", "是否已婚", "是否已育", "国家地区", "民族", "政治面貌",
	"户口性质", "血型", "最高学历", "学习形式", "学位类型",
	"合同类型", "现合同期限", "汇报上级类型",
}

// hrRosterEnumKeyOf 把旧 76 表头名映射到枚举组名（合同类型等旧名兼容）。
func hrRosterEnumKeyOf(field string) string {
	switch field {
	case "现合同类型":
		return "合同类型"
	case "员工状态", "在职状态":
		return "员工状态"
	case "合同期限":
		return "合同期限"
	case "签约公司":
		return "签约公司"
	default:
		return field
	}
}

func itoa(n int) string {
	if n <= 0 {
		return "1"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
