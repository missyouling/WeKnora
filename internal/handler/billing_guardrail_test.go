package handler

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 写入侧护栏（第 22 轮 P2-2 / P2-5）。
//
// 这两条都不是「算法对不对」，而是「脏数据能不能进来」：
// 字段定义一旦写进空名/重名/非法类型，六处消费端会在**同一个 subs** 上
// 解释出不同结果，而且没有任何报错 —— 只能靠人眼在截图里发现两列表头同名。

func TestSanitizeFleetSubs(t *testing.T) {
	in := []types.FleetCategorySub{
		{Name: "month", Label: " 账单周期 ", DataType: "text", Width: 120},
		{Name: "   "}, // 空名 → 丢
		{Name: "month", Label: "重复的账单周期"},                       // 重名 → 丢
		{Name: "total_kwh", Label: "总电量", DataType: "NUMBER"},   // 大写 → number
		{Name: "items_field", Label: "明细", DataType: "array[]"}, // 非法 → text
		{Name: "remark", Label: "备注", DataType: "", Width: -5},  // 空类型 → text；负宽 → 0
		{Name: "wide", Label: "超宽", DataType: "text", Width: 9999},
		{Name: "long_label", Label: "一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二"},
	}
	out, dropped := sanitizeFleetSubs(in)

	if dropped != 2 {
		t.Fatalf("应丢弃 2 条（空名 + 重名），实际 %d", dropped)
	}
	if len(out) != 6 {
		t.Fatalf("保留 6 条，实际 %d", len(out))
	}
	// 顺序保持（拖拽顺序即表头顺序，不得重排）
	if out[0].Name != "month" || out[1].Name != "total_kwh" {
		t.Fatalf("顺序被改动：%v", []string{out[0].Name, out[1].Name})
	}
	if out[0].Label != "账单周期" {
		t.Fatalf("label 未 trim：%q", out[0].Label)
	}
	if out[1].DataType != "number" {
		t.Fatalf("大写类型未归一：%q", out[1].DataType)
	}
	if out[2].DataType != "text" {
		t.Fatalf("非法类型必须回落 text，实际 %q", out[2].DataType)
	}
	if out[3].DataType != "text" {
		t.Fatalf("空类型必须回落 text，实际 %q", out[3].DataType)
	}
	if out[3].Width != 0 {
		t.Fatalf("负宽度必须归 0，实际 %v", out[3].Width)
	}
	if out[4].Width != 0 {
		t.Fatalf("超宽（9999）必须归 0 交给自适应，实际 %v", out[4].Width)
	}
	// 超长中文名按 rune 截断到 40，不得切出半个字（bytes 截断会产生乱码）
	if n := len([]rune(out[5].Label)); n != 40 {
		t.Fatalf("超长 label 必须截到 40 个字符，实际 %d", n)
	}
}

func TestSanitizeFleetSubsKeepsEmptyInput(t *testing.T) {
	out, dropped := sanitizeFleetSubs(nil)
	if dropped != 0 || len(out) != 0 {
		t.Fatalf("空输入应原样返回，实际 %d/%d", len(out), dropped)
	}
}

// 健康检查只告警不阻断：异常账单也要能算出来。
func TestCheckBillMeterHealthDoesNotPanic(t *testing.T) {
	// logger 需要真实 ctx（nil 会 panic），生产调用链永远带 ctx
	ctx := context.Background()
	bill := &types.UtilityBillExtractionItem{TotalKwh: 596168}
	checkBillMeterHealth(ctx, "t1", "2026-09", nil, nil, nil)
	checkBillMeterHealth(ctx, "t1", "2026-09", &types.UtilityBillExtractionItem{}, nil, nil)
	// 正常量级（损耗 0.05% / 加减 1.5%）不应触发任何分支
	checkBillMeterHealth(ctx, "t1", "2026-09", bill,
		map[string]float64{"deep": 2.76, "peak": 12.53, "flat": 20.39, "valley": 29.73},
		map[string]float64{"deep": -284.77, "peak": -664.25, "flat": -569.33, "valley": -379.69})
	// 明显失真（损耗 5%）只告警，函数仍正常返回
	checkBillMeterHealth(ctx, "t1", "2026-09", bill,
		map[string]float64{"deep": 8000, "peak": 9000, "flat": 9000, "valley": 9000},
		map[string]float64{})
}
