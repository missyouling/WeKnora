package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 规则张数不足时用 HTML 规则补足（第 24 轮任务二）。
//
// 真机事故链：
//   一页一票的通行费汇总单（22d87e28）→ FlatLayout 命中但只提到 21 张，
//   原文锚点 24 个 → 上层判「rule result shorter than anchors」→ 转 LLM 补提
//   → 撞模型限流 429 → 整份文档 extract_status 卡在 processing，
//   金额/车牌全丢（用户看到 21 张全是 0.00）。
//
// 修法：张数不足时**先**用 HTML 规则补足（纯确定性、零 token），
// 补到更多才替换；补不到才继续走原来的 LLM 补提路径。
//
// 这组测试锁的是**补足判据**（什么时候替换、什么时候保留），
// 而不是两套规则各自的解析细节（那些已有独立测试覆盖）。
// 端到端的实际效果由真机重提验证。

func TestShouldSupplementOnlyWhenShort(t *testing.T) {
	// 真机场景：flat 21 张、锚点 24、HTML 24 张 → 必须替换
	if !shouldSupplementWithHTML(21, 24, 24) {
		t.Fatal("张数不足且 HTML 更多时必须补足")
	}
	// 已经够数 → 不跑（省一次解析）
	if shouldSupplementWithHTML(24, 24, 24) {
		t.Fatal("张数已达标时不应再补足")
	}
	if shouldSupplementWithHTML(25, 24, 30) {
		t.Fatal("超过锚点时更不该替换")
	}
}

func TestShouldSupplementOnlyWhenHTMLActuallyBetter(t *testing.T) {
	// flat 21、锚点 24、HTML 也只 21 → 不能替换（换了没收益，还可能丢字段）
	if shouldSupplementWithHTML(21, 24, 21) {
		t.Fatal("HTML 没有更多时不得替换")
	}
	// HTML 更少 → 更不能替换（会倒退）
	if shouldSupplementWithHTML(21, 24, 18) {
		t.Fatal("HTML 更少时不得替换")
	}
}

func TestShouldSupplementHandlesNoAnchors(t *testing.T) {
	// 没有锚点（无法判断应有多少张）→ 不补足，保持原行为
	if shouldSupplementWithHTML(5, 0, 10) {
		t.Fatal("没有锚点时不得凭空补足")
	}
}

func TestCountInvoiceAnchorsIgnoresCheckCode(t *testing.T) {
	// 真机事故：老版纸质票（8 位票号）的票面上还有 20 位**校验码**，
	// 裸数 20 位数字会把校验码也当票号 → 21 张票被数成 24 个锚点 →
	// 判「提取不全」→ 转 LLM 补提 → 撞限流 429 → 卡在 processing。
	chunks := []*types.Chunk{
		{ChunkType: "text", Content: "发票号码：49471958\n校验码：01154312287907017257\n开票人：刘洋"},
		{ChunkType: "text", Content: "发票号码：53967976\n校验码：00892238182453790376\n开票人：郝敏"},
	}
	if n := CountInvoiceAnchors(chunks); n != 2 {
		t.Fatalf("校验码不得计入锚点，应数到 2 张，实际 %d", n)
	}
}

func TestCountInvoiceAnchorsFallsBackToBareDigits(t *testing.T) {
	// 无「发票号码：」标签的版式（纯号码行）→ 退回数独立 20 位数字
	chunks := []*types.Chunk{
		{ChunkType: "text", Content: "26507906731100002860\n26507903751100004228"},
	}
	if n := CountInvoiceAnchors(chunks); n != 2 {
		t.Fatalf("无标签时应数独立 20 位数字，实际 %d", n)
	}
}

func TestCountInvoiceAnchorsCountsAllPages(t *testing.T) {
	// 锚点计数必须覆盖全部分块（真机 24 个锚点散在 12 个批次里）。
	// 注意：CountInvoiceAnchors 认的是**20 位全电票号**（invoiceNo20Re），
	// 短号（8 位老版纸质票）不计入 —— 这是既有契约，锚点口径与规则口径一致。
	chunks := []*types.Chunk{
		{ChunkType: "text", Content: "发票号码：26507906731100002860\n开票人：张莹月"},
		{ChunkType: "text", Content: "发票号码：26507903751100004228\n开票人：刘洋"},
		{ChunkType: "text", Content: "发票号码：26507904441100005581\n开票人：郝敏"},
	}
	if n := CountInvoiceAnchors(chunks); n != 3 {
		t.Fatalf("应数到 3 个锚点，实际 %d", n)
	}
	// 同一票号在多个分块重复出现时只能算一个（跨页续行的常见形态）
	dup := []*types.Chunk{
		{ChunkType: "text", Content: "发票号码：26507906731100002860"},
		{ChunkType: "text", Content: "发票号码：26507906731100002860\n金额：11.04"},
	}
	if n := CountInvoiceAnchors(dup); n != 1 {
		t.Fatalf("重复票号必须去重，实际 %d", n)
	}
}
