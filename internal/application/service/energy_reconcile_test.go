package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 核心公式：usage = (本期 − 上期) × 倍率。
// 真机账单口径：3825.28 → 4023.9，倍率 3000，抄见电量 595860。
func TestEnergyReadingReconcileComputesUsage(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 3825.28,
		CurrentReading:  4023.9,
		Multiplier:      3000,
		UsageAmount:     595860,
		HasPrevious:     true,
		HasCurrent:      true,
		HasMultiplier:   true,
		HasUsage:        true,
	})
	assert.True(t, res.Computed, "读数齐时必须能推算用量")
	assert.InDelta(t, 595860, res.ComputedUsage, 0.01, "推算用量应等于账单抄见电量")
	assert.False(t, res.UsagePatched, "账单已给用量时不该标记补齐")
	assert.Empty(t, res.Issues, "推算值与账单一致时不应有告警")
}

// 缺用量但读数齐 → 自动补齐。
func TestEnergyReadingReconcilePatchesMissingUsage(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 100,
		CurrentReading:  150,
		HasPrevious:     true,
		HasCurrent:      true,
	})
	assert.True(t, res.Computed)
	assert.InDelta(t, 50, res.ComputedUsage, 0.001)
	assert.True(t, res.UsagePatched, "缺用量应被补齐")
	assert.True(t, res.Reading.HasUsage)
	assert.InDelta(t, 50, res.Reading.UsageAmount, 0.001)
	assert.Empty(t, res.Issues)
}

// 倍率缺省为 1（单表计场景）。
func TestEnergyReadingReconcileDefaultsMultiplierToOne(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 10,
		CurrentReading:  12.5,
		HasPrevious:     true,
		HasCurrent:      true,
	})
	assert.InDelta(t, 2.5, res.ComputedUsage, 0.0001, "倍率缺省应为 1")
}

// 有用量 + 有读数，但偏差超容差 → 记 mismatch 告警，且**不改账单值**。
func TestEnergyReadingReconcileFlagsMismatch(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 100,
		CurrentReading:  150,
		Multiplier:      2,
		UsageAmount:     500, // 推算值是 100，差 5 倍
		HasPrevious:     true,
		HasCurrent:      true,
		HasMultiplier:   true,
		HasUsage:        true,
	})
	require.Len(t, res.Issues, 1)
	assert.Equal(t, types.EnergyIssueUsageMismatch, res.Issues[0].Code)
	assert.InDelta(t, 100, res.Issues[0].Expected, 0.001)
	assert.InDelta(t, 500, res.Issues[0].Actual, 0.001)
	// 账单值优先：不覆盖
	assert.InDelta(t, 500, res.Reading.UsageAmount, 0.001)
	assert.False(t, res.UsagePatched)
}

// 变损/线损导致的尾差在容差内 → 不告警。
func TestEnergyReadingReconcileToleratesSmallDiff(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 100,
		CurrentReading:  150,
		Multiplier:      1,
		UsageAmount:     50.2, // 推算 50，差 0.4% < 1%
		HasPrevious:     true,
		HasCurrent:      true,
		HasMultiplier:   true,
		HasUsage:        true,
	})
	assert.Empty(t, res.Issues, "容差内的小差不应告警")
}

// 本期读数 < 上期读数（换表/回抄）→ 不补用量，记 regressed 告警。
func TestEnergyReadingReconcileFlagsRegressedReading(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 500,
		CurrentReading:  100,
		HasPrevious:     true,
		HasCurrent:      true,
	})
	require.NotEmpty(t, res.Issues)
	assert.Equal(t, types.EnergyIssueReadingRegressed, res.Issues[0].Code)
	assert.False(t, res.UsagePatched, "读数倒退时绝不能补出负用量")
	assert.False(t, res.Computed)
}

// 有用量但无读数 → 记 usage_missing（无法交叉校验），不补读数。
func TestEnergyReadingReconcileFlagsMissingReading(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		UsageAmount: 120,
		HasUsage:    true,
	})
	require.Len(t, res.Issues, 1)
	assert.Equal(t, types.EnergyIssueUsageMissing, res.Issues[0].Code)
	assert.False(t, res.UsagePatched, "没有读数就不能反算用量")
}

// 全缺 → reading_missing。
func TestEnergyReadingReconcileFlagsAllMissing(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{})
	require.Len(t, res.Issues, 1)
	assert.Equal(t, types.EnergyIssueReadingMissing, res.Issues[0].Code)
}

// 倍率为 0（脏数据）→ 按 1 参与推算并告警。
func TestEnergyReadingReconcileHandlesInvalidMultiplier(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 100,
		CurrentReading:  120,
		Multiplier:      0,
		HasPrevious:     true,
		HasCurrent:      true,
		HasMultiplier:   true,
	})
	assert.InDelta(t, 20, res.ComputedUsage, 0.001, "脏倍率按 1 处理")
	require.NotEmpty(t, res.Issues)
}

// 数值是 0 时也必须视为「有值」（0 是合法读数，不能与「没提取到」混淆）。
func TestEnergyReadingZeroIsAValidValue(t *testing.T) {
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: 0,
		CurrentReading:  0,
		HasPrevious:     true,
		HasCurrent:      true,
	})
	assert.True(t, res.Computed)
	assert.InDelta(t, 0, res.ComputedUsage, 0.001)
	assert.Empty(t, res.Issues, "0/0 的读数对是合法的（本期无用量），不应告警")
}

// 从模型输出的 map 构造记录：数值可能是字符串形态，宽松解析。
func TestEnergyRecordFromMapParsesStringNumbers(t *testing.T) {
	m := map[string]any{
		"utility_no":       "5001654065328",
		"billing_period":   "2026-09",
		"utility_type":     "电费",
		"previous_reading": "3825.28",
		"current_reading":  "4023.9",
		"multiplier":       "3000",
		"usage_amount":     nil,
		"total_amount":     "433,433.07",
	}
	rec := energyRecordFromMap(m)
	assert.Equal(t, "5001654065328", rec.UtilityNo)
	assert.Equal(t, "2026-09", rec.BillingPeriod)
	require.NotNil(t, rec.PreviousReading)
	require.NotNil(t, rec.CurrentReading)
	require.NotNil(t, rec.Multiplier)
	// 语义：UsageAmount 非 nil = 当前有可用值；UsagePatched = 该值由读数反算推算而来
	require.NotNil(t, rec.UsageAmount, "读数齐 → 补齐后用量有值")
	assert.True(t, rec.UsagePatched, "补齐值必须标记为系统推算")
	assert.InDelta(t, 595860, *rec.UsageAmount, 0.01)
	require.NotNil(t, rec.TotalAmount)
	assert.InDelta(t, 433433.07, *rec.TotalAmount, 0.001)
}

// 多表位：items[] 逐行反算，主表上提到记录级，一文件一档案不裂变。
func TestEnergyRecordFromMapMergesMultipleMeters(t *testing.T) {
	m := map[string]any{
		"utility_no":       "5001654065328",
		"billing_period":   "2026-09",
		"previous_reading": 3825.28,
		"current_reading":  4023.9,
		"multiplier":       3000,
		"items": []any{
			map[string]any{
				"meter_no": "M1", "meter_type": "正向有功（总）",
				"previous_reading": 3825.28, "current_reading": 4023.9,
				"multiplier": 3000,
			},
			map[string]any{
				"meter_no": "M2", "meter_type": "正向有功（尖峰）",
				"previous_reading": 229.66, "current_reading": 238.31,
				"multiplier": 3000,
			},
		},
	}
	rec := energyRecordFromMap(m)
	require.Len(t, rec.Items, 2, "两个表位都进明细，不裂变成两条记录")
	assert.Equal(t, "M1", rec.Items[0].MeterNo)
	require.NotNil(t, rec.Items[0].ComputedUsage)
	assert.InDelta(t, 595860, *rec.Items[0].ComputedUsage, 0.01)
	require.NotNil(t, rec.Items[1].ComputedUsage)
	assert.InDelta(t, 25950, *rec.Items[1].ComputedUsage, 0.01)
	require.NotNil(t, rec.Items[0].UsageAmount, "明细行也应补齐用量")
	// 记录级主表口径
	require.NotNil(t, rec.UsageAmount)
	assert.InDelta(t, 595860, *rec.UsageAmount, 0.01)
}
