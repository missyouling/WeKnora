// 第 56 轮回归：真实附件数据量、部门汇总金额、四胶囊字段契约、文件名/日期解析。
package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound56RealSeedRowCounts(t *testing.T) {
	require.Len(t, hrSIReceiptSeedHeader, 28)
	require.Len(t, hrSIReceiptSeedRows, 242, "《人员增减申请回盘表.xlsx》第 3~244 行")
	require.Len(t, hrSIRecordSeedRows, 378, "375 条正常 + 3 条异常")
	require.Len(t, hrSIBillItemSeedRows, 76, "75 名参保员工 + 陈刚补缴行")

	normal, abnormal := 0, 0
	for _, r := range hrSIRecordSeedRows {
		if r.Kind == "异常" {
			abnormal++
		} else {
			normal++
		}
	}
	require.Equal(t, 375, normal)
	require.Equal(t, 3, abnormal)
	require.Equal(t, "2026-11", hrSIBillSeedValue.MonthLabel)
	require.Equal(t, 75, hrSIBillSeedValue.Headcount)
	require.InDelta(t, 48254.78, hrSIBillSeedValue.PersonalTotal, 0.001)
	require.InDelta(t, 128084.72, hrSIBillSeedValue.UnitTotal, 0.001)
	require.InDelta(t, 176339.50, hrSIBillSeedValue.GrandTotal, 0.001)
}

func TestRound56ReceiptKeyRowsAndAccountBinding(t *testing.T) {
	byName := map[string][]string{}
	for _, row := range hrSIReceiptSeedRows {
		if _, ok := byName[row[3]]; !ok {
			byName[row[3]] = row
		}
	}
	for name, code := range map[string]string{
		"刘正均": "2015097775",
		"李永娇": "2001114865",
		"陈刚":  "2011209240",
		"黄润州": "2033433498",
	} {
		row, ok := byName[name]
		require.True(t, ok, "回盘表应包含 %s", name)
		require.Equal(t, code, row[1], "%s 的个人编号", name)
	}
	// 个人编号 / 身份证号都是纯数字串，不含千分位逗号
	for _, row := range hrSIReceiptSeedRows {
		require.NotContains(t, row[1], ",")
		require.NotContains(t, row[2], ",")
	}
	// 同身份证重复行（刘正均序号 1 / 168）：主记录必须取「参保缴费」那条
	var dup []HRSIReceipt
	for _, row := range hrSIReceiptSeedRows {
		if row[2] == "510232197011157112" {
			dup = append(dup, hrSIReceiptRowToModel(0, hrSIReceiptSeedHeader, row, nil))
		}
	}
	require.Len(t, dup, 2)
	require.Equal(t, "2015097775", hrSIReceiptByID(dup)["510232197011157112"].PersonCode)
}

func TestRound56DeptSummaryMatchesFigure2(t *testing.T) {
	items := make([]HRSIBillItem, 0, len(hrSIBillItemSeedRows))
	for _, d := range hrSIBillItemSeedRows {
		items = append(items, hrSIBillItemRowToModel(0, "bill-1", d))
	}
	summary := hrSIDeptSummaryOf(items)
	by := map[string]map[string]any{}
	for _, s := range summary {
		by[s["部门"].(string)] = s
	}
	require.Len(t, summary, 14)
	// 图 2：财务部 1 人 636.58 / 1693.24；库房 7 人含陈刚补缴 4967.34 / 12944.41
	require.Equal(t, 1, by["财务部"]["人数"])
	require.InDelta(t, 636.58, by["财务部"]["社保个人金额"], 0.001)
	require.InDelta(t, 1693.24, by["财务部"]["社保单位金额"], 0.001)
	require.Equal(t, 7, by["库房"]["人数"])
	require.InDelta(t, 4967.34, by["库房"]["社保个人金额"], 0.001)
	require.InDelta(t, 12944.41, by["库房"]["社保单位金额"], 0.001)

	var people int
	var personal, unit, total float64
	for _, s := range summary {
		people += s["人数"].(int)
		personal += s["社保个人金额"].(float64)
		unit += s["社保单位金额"].(float64)
		total += s["合计"].(float64)
	}
	require.Equal(t, 75, people, "补缴行不增加人数")
	require.InDelta(t, 48254.78, personal, 0.001)
	require.InDelta(t, 128084.72, unit, 0.001)
	require.InDelta(t, 176339.50, total, 0.001)
}

func TestRound56SocialFourGroupSubs(t *testing.T) {
	subs := hrSocialSeedSubs()
	require.Len(t, subs, 76, "28 + 16 + 16 + 16")
	want := []string{"回盘表", "正常记录", "异常记录", "月度账单"}
	got := []string{}
	for _, s := range subs {
		if len(got) == 0 || got[len(got)-1] != s.GroupLabel {
			got = append(got, s.GroupLabel)
		}
		require.Equal(t, s.Group, s.GroupLabel, "胶囊标题必须等于 group")
	}
	require.Equal(t, want, got)
	// 同名字段在两个分组里都要存在（复合键去重生效）
	count := 0
	for _, s := range subs {
		if s.Name == "账单月份" {
			count++
		}
	}
	require.Equal(t, 2, count)
	// 五险参保状态系统内置加锁
	for _, name := range []string{"养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态"} {
		found := false
		for _, s := range subs {
			if s.Name == name {
				found = true
				require.True(t, s.Builtin, name)
				require.True(t, s.IsDefault, name)
			}
		}
		require.True(t, found, name)
	}
}

func TestRound56FileSchemeAndDateNormalize(t *testing.T) {
	require.Equal(t, "大额医疗", hrSISchemeOfFile("职工大额医疗互助保险(个人缴纳)_2026-11至2026-11_未申报信息明细.xlsx"))
	require.Equal(t, "养老保险", hrSISchemeOfFile("职工基本养老保险(单位缴纳)_2026-11至2026-11_未申报信息明细.xlsx"))
	require.Equal(t, "医疗保险", hrSISchemeOfFile("职工基本医疗保险(个人缴纳)_2026-11至2026-11_未申报信息明细.xlsx"))
	require.Equal(t, "失业保险", hrSISchemeOfFile("失业保险(单位缴纳)_2026-11至2026-11_未申报信息明细.xlsx"))
	require.Equal(t, "工伤保险", hrSISchemeOfFile("工伤保险_2026-11至2026-11_未申报信息明细.xlsx"))
	require.Equal(t, "1970-11-15", hrSINormalizeDate("19701115"))
	require.Equal(t, "2017-12-01", hrSINormalizeDate("20171201"))
	require.Equal(t, "", hrSINormalizeDate(""))
	require.Equal(t, "1970-11-15", hrSINormalizeDate("1970-11-15"))
}

func TestRound56SITemplateHeaders(t *testing.T) {
	h := hrTemplateHeaders(hrModuleSocial)
	require.Len(t, h, 27)
	require.Equal(t, "个人编号", h[0])
	require.NotContains(t, h, "部门", "跨表引用字段不进导入模板")
	spec := hrTemplateSpecOf(hrModuleSocial)
	require.Equal(t, hrModuleSocial, spec.Module)
	require.Equal(t, "社保回盘表导入模板.xlsx", spec.Name)
}
