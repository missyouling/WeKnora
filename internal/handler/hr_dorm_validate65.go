// 第 65 轮：宿舍/公租房/资产保存接口的强类型校验。
//
// 前端已经用受控控件拦截，但接口必须同口径兜底 —— 否则脚本或旧客户端仍能写入
// 「打撒'」「饿243」「53q」这类脏值，落库后导出/打印全乱。
package handler

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// 金额/小数：非负，最多 4 位小数（单价允许 4 位，如 0.6185 元/度）
	hrMoneyRe65 = regexp.MustCompile(`^\d+(\.\d{1,4})?$`)
	// 整数：非负整数
	hrIntRe65 = regexp.MustCompile(`^\d+$`)
	// 月份：YYYY-MM
	hrMonthRe65 = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
)

// hrDormMonthFields65 月份型字段（必须 YYYY-MM）。
var hrDormMonthFields65 = map[string]bool{
	"租期起始月份": true,
	"租期截止月份": true,
	"账单月份":   true,
}

// hrDormIntegerFields65 整数型字段（非负整数）。
var hrDormIntegerFields65 = map[string]bool{
	"床位数":   true,
	"可用床位数": true,
	"数量":    true,
	"入住天数":  true,
	"总人数":   true,
	"住宿人数":  true,
}

// hrDormMoneyFields65 金额/小数型字段（非负数值，最多 4 位小数）。
var hrDormMoneyFields65 = map[string]bool{
	"宿舍押金(元)":  true, "电费标准(元/度)": true, "水费单价(元/m³)": true,
	"垃圾处理费(元/套)": true, "二次供水费(元/m³)": true, "污水处理费(元/m³)": true,
	"气费标准(元/m³)": true, "房屋租金(元/月)": true, "物业费(元/月)": true,
	"月费用单价(元/月)": true, "季度租金(元/季)": true, "房间面积(㎡)": true,
	"建筑面积(㎡)": true, "资产价值(元)": true, "押金金额(元)": true,
	"入住时水表度数": true, "入住时电表度数": true, "入住时气表度数": true,
	"分摊比例": true,
}

// hrValidateDormData65 校验一份宿舍类 custom_data；返回中文错误（空串表示通过）。
func hrValidateDormData65(data map[string]any) string {
	for k, v := range data {
		name := strings.TrimSpace(k)
		s := strings.TrimSpace(hrDormStr(data, k))
		if s == "" {
			continue
		}
		_ = v
		switch {
		case hrDormMonthFields65[name]:
			if !hrMonthRe65.MatchString(s) {
				return fmt.Sprintf("「%s」必须为 YYYY-MM 格式的月份", name)
			}
		case hrDormIntegerFields65[name]:
			if !hrIntRe65.MatchString(s) {
				return fmt.Sprintf("「%s」只能填写非负整数", name)
			}
		case hrDormMoneyFields65[name]:
			if !hrMoneyRe65.MatchString(s) {
				return fmt.Sprintf("「%s」只能填写非负数字（最多 4 位小数）", name)
			}
		}
	}
	// 租期起止逻辑校验
	from := strings.TrimSpace(hrDormStr(data, "租期起始月份"))
	to := strings.TrimSpace(hrDormStr(data, "租期截止月份"))
	if from != "" && to != "" && to < from {
		return "「租期截止月份」不能早于「租期起始月份」"
	}
	return ""
}
