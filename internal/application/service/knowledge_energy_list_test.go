package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Tencent/WeKnora/internal/types"
)

// energyRecordForTest 构造一条测试用能耗记录。
func energyRecordForTest(prev, curr, mult, usage float64) types.EnergyRecord {
	rec := types.EnergyRecord{
		UtilityNo:     "5001654065328",
		BillingPeriod: "2026-09",
		UtilityType:   "电费",
	}
	if prev != 0 {
		rec.PreviousReading = &prev
	}
	if curr != 0 {
		rec.CurrentReading = &curr
	}
	if mult != 0 {
		rec.Multiplier = &mult
	}
	if usage != 0 {
		rec.UsageAmount = &usage
	}
	rec.Items = []types.EnergyMeterItem{{MeterNo: "M1", MeterType: "正向有功（总）"}}
	return rec
}

// 能耗台账的主时间轴是**账期（月）**，不是日期（日）。
// 账期区间比较必须按 YYYYMM 归一后做字典序比较，且容忍端点给到日。
func TestEnergyPeriodInRange(t *testing.T) {
	cases := []struct {
		period string
		from   string
		to     string
		want   bool
	}{
		{"2026-09", "", "", true},
		{"2026-09", "2026-01", "2026-12", true},
		{"2026-09", "2026-09", "2026-09", true},
		{"2026-09", "2026-10", "2026-12", false},
		{"2026-09", "2026-01", "2026-08", false},
		// 端点给到日也必须正确截断到月
		{"2026-09", "2026-09-15", "", true},
		{"2026-09", "", "2026-09-15", true},
		{"2026-09", "2026-10-01", "", false},
		// 其它写法归一
		{"202609", "2026-01", "2026-12", true},
		{"2026年09月", "2026-01", "2026-12", true},
		// 账期为空：有区间要求时不命中
		{"", "2026-01", "2026-12", false},
		// 无区间时永远命中
		{"", "", "", true},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, energyPeriodInRange(c.period, c.from, c.to),
			"energyPeriodInRange(%q, %q, %q)", c.period, c.from, c.to)
	}
}

// 抄表日期区间是日粒度（与账期不同轴）。
func TestEnergyDateInRange(t *testing.T) {
	assert.True(t, energyDateInRange("2026-09-15", "", ""))
	assert.True(t, energyDateInRange("2026-09-15", "2026-09-01", "2026-09-30"))
	assert.False(t, energyDateInRange("2026-09-15", "2026-10-01", ""))
	assert.False(t, energyDateInRange("2026-09-15", "", "2026-08-31"))
	assert.False(t, energyDateInRange("", "2026-09-01", "2026-09-30"), "空日期不命中有区间的筛选")
}

// 全字段搜索必须穿透读数类数值字段与明细行表号。
func TestEnergyRecordMatchKeyword(t *testing.T) {
	prev := 3825.28
	curr := 4023.9
	mult := 3000.0
	usage := 595860.0
	rec := energyRecordForTest(prev, curr, mult, usage)
	assert.True(t, energyRecordMatchKeyword(rec, "5001654065328"), "户号")
	assert.True(t, energyRecordMatchKeyword(rec, "2026-09"), "账期")
	assert.True(t, energyRecordMatchKeyword(rec, "电费"), "能耗类型")
	assert.True(t, energyRecordMatchKeyword(rec, "595860"), "实际用量数值")
	assert.True(t, energyRecordMatchKeyword(rec, "M1"), "明细行表号")
	assert.False(t, energyRecordMatchKeyword(rec, "不存在的关键字"))
}

// 空提取判定：无任何关键字段的记录不进主列表。
func TestEnergyRecordBlank(t *testing.T) {
	// 真正全空的记录（连户号/账期都没有）
	empty := types.EnergyRecord{}
	assert.True(t, energyRecordBlank(empty))
	// 构造器带的户号/账期/类型本身就不是空提取
	assert.False(t, energyRecordBlank(energyRecordForTest(0, 0, 0, 0)),
		"带户号与账期的记录不是空提取")
	rec := energyRecordForTest(3825.28, 4023.9, 3000, 595860)
	assert.False(t, energyRecordBlank(rec), "有读数的记录不是空提取")
}
