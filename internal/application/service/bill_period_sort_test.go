package service

import (
	"sort"
	"testing"
	"time"
)

// 第 43 轮：账单周期必须严格按月份倒序，不能按上传时间。
//
// 真机事故：市电账单列表出现 2026-10、2026-09、2025-06、2025-07… 第 10 行又是
// 2026-03、2026-05、2026-04 —— 因为排序键是 CreatedAt（上传时间），
// 与账单账期完全无关。
func TestBillPeriodKeyNormalizesToYearMonth(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2026-10", "2026-10"},
		{"2026.10", "2026-10"},
		{"2026/10", "2026-10"},
		{"2026年7月", "2026-07"},
		{"2026-7", "2026-07"},
		{"202607", "2026-07"},
		{" 2026-10-01 ", "2026-10"},
		{"", ""},
		{"不是月份", ""},
	} {
		if got := billPeriodKey(tc.in); got != tc.want {
			t.Errorf("billPeriodKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// 混入各种写法的月份 + 空值，排序后必须严格递减且空值排最后。
func TestBillPeriodSortDescStrict(t *testing.T) {
	type rec struct {
		period string
		at     time.Time
	}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	// 故意让上传时间与账期完全相反：2025-06 最新上传、2026-10 最早上传。
	records := []rec{
		{"2025-06", base.Add(10 * time.Hour)},
		{"2026.10", base},
		{"2026年7月", base.Add(5 * time.Hour)},
		{"2026-09", base.Add(2 * time.Hour)},
		{"2026-7", base.Add(6 * time.Hour)}, // 与 2026年7月 同月
		{"", base.Add(11 * time.Hour)},      // 空账期
		{"2025-07", base.Add(9 * time.Hour)},
		{"2026-08", base.Add(3 * time.Hour)},
		{"2026-01", base.Add(4 * time.Hour)},
		{"2025-12", base.Add(7 * time.Hour)},
		{"2025-11", base.Add(8 * time.Hour)},
	}
	sort.SliceStable(records, func(i, j int) bool {
		return billPeriodLess(records[i].period, records[i].at, records[j].period, records[j].at)
	})

	got := make([]string, 0, len(records))
	for _, r := range records {
		got = append(got, billPeriodKey(r.period))
	}
	want := []string{
		"2026-10", "2026-09", "2026-08", "2026-07", "2026-07",
		"2026-01", "2025-12", "2025-11", "2025-07", "2025-06", "",
	}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	// 相邻严格不增
	for i := 1; i < len(got); i++ {
		if got[i-1] == "" {
			continue
		}
		if got[i] != "" && got[i-1] < got[i] {
			t.Fatalf("order violated at %d: %q < %q", i, got[i-1], got[i])
		}
	}
}

// 同月内按上传时间倒序（稳定次键），且不得改变同月同年月的相对顺序。
func TestBillPeriodSortSameMonthByCreatedAtDesc(t *testing.T) {
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	type rec struct {
		period string
		at     time.Time
		name   string
	}
	records := []rec{
		{"2026-07", base, "old"},
		{"2026-07", base.Add(2 * time.Hour), "new"},
	}
	sort.SliceStable(records, func(i, j int) bool {
		return billPeriodLess(records[i].period, records[i].at, records[j].period, records[j].at)
	})
	if records[0].name != "new" || records[1].name != "old" {
		t.Fatalf("same month must order by created_at desc, got %v", records)
	}
}
