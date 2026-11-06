package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 第 43 轮：电量（定比）的**唯一权威来源**是市电账单 fee_items 里
// `(5)目录电费` 的计费数量（qty），不是 residential_readings 的 bill_kwh。
//
// 真机事故：费用清单「电量（定比）」在 2026-07 显示 147.32（= 9821 × 0.015，
// 被乘了两次定比），2026-06 / 2026-04 / 2025-10 显示 0，其余月份出现
// 149.79 / 208.95 / 82.65 / 209.52 / 213 等「乘过一次 0.015」的值。
// 而 fee_items 的 `(5)目录电费` qty 在全部 17 个月都是正确的居民计费电量。
func TestResidentBillingKwhFromCatalogFee(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []types.UtilityBillFeeItem
		want  float64
	}{
		{
			name:  "单行目录电费",
			items: []types.UtilityBillFeeItem{{Category: "(5)目录电费", Qty: 9821, Fee: 4679.86}},
			want:  9821,
		},
		{
			name: "2025-07 两行目录电费（电能表 + 定比）必须合计",
			items: []types.UtilityBillFeeItem{
				{Category: "(5)目录电费", Qty: 4501},
				{Category: "(5)目录电费", Qty: 11953},
			},
			want: 16454,
		},
		{
			name: "目录电费缺失时退回 0（由调用方决定是否兜底）",
			items: []types.UtilityBillFeeItem{
				{Category: "(1)市场化购电费", Qty: 196797},
			},
			want: 0,
		},
		{
			name: "忽略 0 数量的目录电费行，但保留有效行",
			items: []types.UtilityBillFeeItem{
				{Category: "(5)目录电费", Qty: 0},
				{Category: "(5)目录电费", Qty: 9683},
			},
			want: 9683,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := &types.UtilityBillExtractionItem{FeeItems: tc.items}
			if got := ResidentBillingKwh(item); got != tc.want {
				t.Fatalf("ResidentBillingKwh = %v, want %v", got, tc.want)
			}
		})
	}
}

// 修复器把 residential_readings 主行的 bill_kwh 对齐到权威值。
func TestRepairResidentReadingBillKwh(t *testing.T) {
	for _, tc := range []struct {
		name       string
		readings   []types.UtilityResidentialReading
		feeItems   []types.UtilityBillFeeItem
		wantBill   float64
		wantChange bool
	}{
		{
			name:       "147.32（乘两次定比）→ 9821",
			readings:   []types.UtilityResidentialReading{{Project: "定比0.015", Kwh: 655058, Ratio: 0.015, BillKwh: 147.32}},
			feeItems:   []types.UtilityBillFeeItem{{Category: "(5)目录电费", Qty: 9821}},
			wantBill:   9821,
			wantChange: true,
		},
		{
			name:       "0（缺值）→ 9683",
			readings:   []types.UtilityResidentialReading{{Project: "定比0.015", Kwh: 0, Ratio: 0.015, BillKwh: 0}},
			feeItems:   []types.UtilityBillFeeItem{{Category: "(5)目录电费", Qty: 9683}},
			wantBill:   9683,
			wantChange: true,
		},
		{
			name:       "已经正确（9354）不产生变更",
			readings:   []types.UtilityResidentialReading{{Project: "定比0.015", Kwh: 623932, Ratio: 0.015, BillKwh: 9354}},
			feeItems:   []types.UtilityBillFeeItem{{Category: "(5)目录电费", Qty: 9354}},
			wantBill:   9354,
			wantChange: false,
		},
		{
			name:       "没有目录电费时不动（缺权威值不猜）",
			readings:   []types.UtilityResidentialReading{{Project: "定比0.015", Kwh: 1000, Ratio: 0.015, BillKwh: 15}},
			feeItems:   nil,
			wantBill:   15,
			wantChange: false,
		},
		{
			name: "2025-07 两行居民读数时不动（多表计行不能覆盖）",
			readings: []types.UtilityResidentialReading{
				{Project: "居民目录电量（电能表5030001221300008576267）", Kwh: 4501, Ratio: 1, BillKwh: 4501},
				{Project: "居民目录电量（定比0.015）", Kwh: 11953, Ratio: 1, BillKwh: 11953},
			},
			feeItems:   []types.UtilityBillFeeItem{{Category: "(5)目录电费", Qty: 4501}, {Category: "(5)目录电费", Qty: 11953}},
			wantBill:   4501, // 主行保持原值
			wantChange: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := &types.UtilityBillExtractionItem{
				ResidentialReadings: tc.readings,
				FeeItems:            tc.feeItems,
			}
			got := RepairResidentReadingBillKwh(item)
			if got != tc.wantChange {
				t.Fatalf("changed = %v, want %v", got, tc.wantChange)
			}
			// 校验主行
			if len(tc.readings) > 0 {
				var main *types.UtilityResidentialReading
				for i := range item.ResidentialReadings {
					r := &item.ResidentialReadings[i]
					if (r.Ratio > 0 && r.Ratio < 0.05) || main == nil {
						main = r
					}
				}
				if main == nil {
					t.Fatal("no main reading")
				}
				if main.BillKwh != tc.wantBill {
					t.Fatalf("main bill_kwh = %v, want %v", main.BillKwh, tc.wantBill)
				}
			}
		})
	}
}

// 用电量（工业）必须用修正后的居民电量核减：
// Q_工业 = Q_总 − Q_定比 − Q_持睿工业 − Q_宿舍
func TestIndustrialKwhUsesCorrectedResidentKwh(t *testing.T) {
	// 2026-07 真机数据：总电量 655058，持睿工业分摊 314227.57，宿舍 4273
	total, resident, tenantInd, dorm := 655058.0, 9821.0, 314227.57, 4273.0
	want := round2(total - resident - tenantInd - dorm)
	if want < 0 {
		t.Fatalf("fixture wrong")
	}
	// 脏值 147.32 会得到 326736.11，与正确值差 9673.68
	bad := round2(total - 147.32 - tenantInd - dorm)
	if want == bad {
		t.Fatal("fixture must distinguish correct vs dirty resident kwh")
	}
	if d := round2(bad - want); d != round2(9821-147.32) {
		t.Fatalf("delta = %v, want %v", d, round2(9821-147.32))
	}
}
