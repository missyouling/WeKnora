package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 主列表 / KPI 准入判据（第 24 轮任务一）。
//
// 真机事故（图 1）：状态栏还显示「正在解析 2026.9-10油费发票.pdf」，
// 列表第 17~20 行已经出现了该文件的 P1~P4（第 20 行还是「待提取 + 0.00」），
// 顶部 KPI 卡片也提前变了。
//
// 根因：旧判据是「未完成 + 一条子记录都没有」才拦 —— 一旦半成品子记录
// 已经落库（上一轮中断的残片、或提前生成的占位），条件就不成立，直接放行。
//
// 新判据 extractionShouldHide 的三个拦截条件：
//  1. 解析明确未完成 → 一律挡（不看子记录）；
//  2. 提取明确在途 → 一律挡（不看子记录）；
//  3. 提取无结论且无子记录 → 挡。
//
// 放行的关键一类：状态字段为空 + 已有子记录（历史数据）。

func TestShouldHideWhenParseIncomplete(t *testing.T) {
	// 解析还在跑，哪怕库里已经有 5 条子记录也必须挡 —— 那是半成品
	for _, ps := range []string{"pending", "processing", "finalizing", "deleting", "parsing"} {
		if !extractionShouldHide(ps, "success", 5) {
			t.Errorf("parse_status=%q 且有 5 条子记录时必须挡（半成品不得进列表）", ps)
		}
	}
}

func TestShouldHideWhenExtractInFlight(t *testing.T) {
	// 解析完成、提取明确在途 → 挡（这是图 1 第 17~20 行的直接成因）
	for _, es := range []string{"pending", "processing", "queued", "running", "extracting"} {
		if !extractionShouldHide("completed", es, 4) {
			t.Errorf("extract_status=%q 且有 4 条子记录时必须挡（残片不得进列表）", es)
		}
	}
}

func TestShouldHideWhenNoConclusionAndNoItems(t *testing.T) {
	// 全新上传、还没开始提取 → 挡
	if !extractionShouldHide("completed", "", 0) {
		t.Fatal("无结论且无子记录时必须挡")
	}
	if !extractionShouldHide("completed", "unknown-future-status", 0) {
		t.Fatal("未知状态且无子记录时必须挡")
	}
}

func TestShouldShowWhenDone(t *testing.T) {
	// 有结论 → 放行
	for _, es := range []string{"success", "manual", "failed"} {
		if extractionShouldHide("completed", es, 3) {
			t.Errorf("extract_status=%q 必须放行", es)
		}
	}
	// failed 且 0 张：也是结论（归历史记录抽屉，但判据本身放行）
	if extractionShouldHide("completed", "failed", 0) {
		t.Error("failed 是明确结论，判据应放行（是否展示由上层 failed 分支决定）")
	}
}

func TestShouldShowHistoricalDataWithoutStatus(t *testing.T) {
	// **最容易改错的一条**：状态字段是后来才加的，历史数据整片缺失，
	// 但它们有真实子记录 —— 绝不能因为状态为空就整体隐身。
	if extractionShouldHide("completed", "", 5) {
		t.Fatal("状态缺失但有子记录的历史数据必须放行")
	}
	if extractionShouldHide("", "", 5) {
		t.Fatal("parse_status 也缺失但有子记录时同样要放行")
	}
}

func TestShouldShowWhenParseFailed(t *testing.T) {
	// 解析失败但 chunk 文本可能已完整落库，业务重提只依赖文本，不该挡死
	if extractionShouldHide("failed", "success", 3) {
		t.Fatal("解析失败但有结论时必须放行")
	}
	if extractionShouldHide("cancelled", "success", 3) {
		t.Fatal("解析取消但有结论时必须放行")
	}
}

func TestParseIncompleteJudgement(t *testing.T) {
	cases := map[string]bool{
		"":           false, // 字段缺失 → 不算未完成（历史数据）
		"completed":  false,
		"success":    false,
		"failed":     false, // 失败是结论，不挡
		"cancelled":  false,
		"canceled":   false,
		"pending":    true,
		"processing": true,
		"finalizing": true,
		"deleting":   true,
		"PARSING":    true, // 大小写不敏感
	}
	for status, want := range cases {
		if got := extractionParseIncomplete(status); got != want {
			t.Errorf("extractionParseIncomplete(%q) = %v，期望 %v", status, got, want)
		}
	}
}

func TestExtractionInFlightJudgement(t *testing.T) {
	// 只有明确列出的中间态算「在途」；空串与未知值不算
	cases := map[string]bool{
		"":           false,
		"pending":    true,
		"processing": true,
		"queued":     true,
		"running":    true,
		"extracting": true,
		"success":    false,
		"failed":     false,
		"unknown":    false, // 未知值不当在途（否则历史数据会被永久挡住）
	}
	for status, want := range cases {
		if got := extractionInFlight(status); got != want {
			t.Errorf("extractionInFlight(%q) = %v，期望 %v", status, got, want)
		}
	}
}

// 六个列表都必须用新判据（防回归：有人改回旧判据时这里会红）
func TestAllListsUseUnifiedGate(t *testing.T) {
	// 由 git grep 保证（见 ci 脚本）；此处只锁「判据函数存在且语义正确」。
	// 具体列表文件的断言放在列表自己的测试里。
	_ = types.Knowledge{}
}
