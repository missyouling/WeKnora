package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaterPeriodTotals2026_10(t *testing.T) {
	// 真实台账：星达自来水总表 8581→9350 = 769，星达消防总表 3329→3329 = 0，
	// 两表单价均为 5.22 → 本期水费 769×5.22 = 4014.18。
	rows := []waterRowWithPrice{
		{Name: "星达自来水总表", Usage: 769, Price: 5.22, Start: 8581, End: 9350},
		{Name: "星达消防总表", Usage: 0, Price: 5.22, Start: 3329, End: 3329},
		// 分表 / 宿舍表不进总表口径，它们只用于工业用水量扣减
		{Name: "宿舍301", Usage: 4, Fee: 20, Price: 5},
		{Name: "5-2-8", Usage: 12, Fee: 39.84, Price: 3.32},
	}
	usage, fee := waterPeriodTotals(rows)
	assert.Equal(t, float64(769), usage)
	assert.Equal(t, 4014.18, fee)
}

func TestWaterPeriodTotalsFeeFallback(t *testing.T) {
	// 台账只记读数、没记金额时按「用量 × 单价」回算，不能出现 0 元水费
	rows := []waterRowWithPrice{
		{Name: "星达自来水总表", Usage: 100, Price: 5.22, Start: 0, End: 100},
	}
	usage, fee := waterPeriodTotals(rows)
	assert.Equal(t, float64(100), usage)
	assert.Equal(t, 522.0, fee)
}

func TestWaterPeriodTotalsIgnoresUnrelated(t *testing.T) {
	usage, fee := waterPeriodTotals([]waterRowWithPrice{
		{Name: "宿舍301", Usage: 4, Fee: 20},
		{Name: "持睿403", Usage: 3, Fee: 15.66},
	})
	assert.Equal(t, float64(0), usage)
	assert.Equal(t, float64(0), fee)
}

func TestNormalizeCostListWaterSubs(t *testing.T) {
	// 复刻真机里的存量：用户复制字段只改了 label，name 仍是副本名
	subs := []types.FleetCategorySub{
		{Name: "month", Label: "账单周期", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "unit", Label: "使用单位", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "total_kwh", Label: "本期电量", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "total_fee", Label: "本期电费", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "总电费_副本", Label: "本期水量", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "总用水量_副本", Label: "本期水费", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "solar_gen", Label: "光伏发电量", Enabled: true, IsDefault: true, Group: "default"},
	}
	out, changed := normalizeCostListWaterSubs(subs)
	require.True(t, changed)
	require.Len(t, out, len(subs))

	names := make([]string, 0, len(out))
	for _, s := range out {
		names = append(names, s.Name)
	}
	// 严格顺序：本期电费 → 本期水量 → 本期水费 → 光伏发电量
	assert.Equal(t, []string{
		"month", "unit", "total_kwh", "total_fee",
		waterUsageFieldKey, waterFeeFieldKey, "solar_gen",
	}, names)

	byName := map[string]types.FleetCategorySub{}
	for _, s := range out {
		byName[s.Name] = s
	}
	u := byName[waterUsageFieldKey]
	assert.Equal(t, waterUsageFieldLabel, u.Label)
	assert.Equal(t, "number", u.DataType)
	assert.Equal(t, 0, u.Width)
	assert.True(t, u.Enabled && u.IsDefault)

	f := byName[waterFeeFieldKey]
	assert.Equal(t, waterFeeFieldLabel, f.Label)
	assert.Equal(t, "amount", f.DataType)
	assert.True(t, f.Enabled && f.IsDefault)

	// 幂等：再跑一次不再变化
	_, changedAgain := normalizeCostListWaterSubs(out)
	assert.False(t, changedAgain)
}

func TestNormalizeCostListWaterSubsAddsMissing(t *testing.T) {
	subs := []types.FleetCategorySub{
		{Name: "month", Label: "账单周期", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "total_fee", Label: "本期电费", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "solar_gen", Label: "光伏发电量", Enabled: true, IsDefault: true, Group: "default"},
	}
	out, changed := normalizeCostListWaterSubs(subs)
	require.True(t, changed)
	names := make([]string, 0, len(out))
	for _, s := range out {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"month", "total_fee", waterUsageFieldKey, waterFeeFieldKey, "solar_gen"}, names)
}

func TestNormalizeCostListWaterSubsKeepsTenantGroup(t *testing.T) {
	// 租户分组里的同名字段不能被搬到默认分组去
	subs := []types.FleetCategorySub{
		{Name: "total_fee", Label: "本期电费", Enabled: true, IsDefault: true, Group: "default"},
		{Name: "water_fee", Label: "水费", Enabled: true, IsDefault: false, Group: "tenant:abc"},
	}
	out, _ := normalizeCostListWaterSubs(subs)
	for _, s := range out {
		if s.Group == "tenant:abc" {
			assert.Equal(t, "water_fee", s.Name)
			assert.False(t, s.IsDefault, "租户分组字段不应被强制开启")
		}
	}
}
