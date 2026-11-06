package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestExtractCanonicalizeHTMLTables(t *testing.T) {
	raw := `<p>账单</p><table><tr><th>项目</th><th>数值</th></tr>` +
		`<tr><td>发电量</td><td>194,489 kWh</td></tr>` +
		`<tr><td>上网电量</td><td>27,480 千瓦时</td></tr></table>`
	got := CanonicalizeParsedDocument(raw)
	for _, want := range []string{
		"| 项目 | 数值 |",
		"| 发电量 | 194489 |",
		"项目: 发电量",
		"数值: 194489",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("canonical text missing %q\n--- got ---\n%s", want, got)
		}
	}
	if strings.Contains(got, "<table") || strings.Contains(got, "<td") {
		t.Fatalf("HTML tags must be gone:\n%s", got)
	}
	// 幂等
	if again := CanonicalizeParsedDocument(got); again != got {
		t.Fatalf("canonicalize is not idempotent:\n%s\n--- vs ---\n%s", got, again)
	}
}

func TestExtractCanonicalizeOCRSpacingAndFullWidth(t *testing.T) {
	raw := "账 单 周 期 ：２０２６－１０\n发 电 量：１９４，４８９ 千瓦时\n结 算 金 额：¥8,409.37元\n上 网 电 量 27,480 kWh"
	got := CanonicalizeParsedDocument(raw)
	for _, want := range []string{"账单周期", "发电量", "上网电量", "结算金额", "2026", "194489", "8409.37", "27480"} {
		if !strings.Contains(got, want) {
			t.Fatalf("canonical text missing %q\n--- got ---\n%s", want, got)
		}
	}
	for _, banned := range []string{"发 电 量", "结 算 金 额", "8,409.37", "¥", "kWh", "千瓦时"} {
		if strings.Contains(got, banned) {
			t.Fatalf("canonical text still contains %q\n--- got ---\n%s", banned, got)
		}
	}
}

func TestExtractAggregateAllChunks(t *testing.T) {
	chunks := []*types.Chunk{
		{ChunkIndex: 0, ChunkType: types.ChunkTypeText, Content: "<table><tr><th>项目</th><th>数值</th></tr><tr><td>发电量</td><td>194489</td></tr></table>"},
		{ChunkIndex: 1, ChunkType: types.ChunkTypeImageOCR, Content: "上 网 电 量：27,480 千瓦时"},
		{ChunkIndex: 2, ChunkType: types.ChunkTypeImageCaption, Content: "结算金额：¥8,409.37元"},
		{ChunkIndex: 3, ChunkType: types.ChunkTypeTableSummary, Content: "不该进提取正文"},
	}
	got := AggregateChunksForExtraction(chunks)
	for _, want := range []string{"发电量", "194489", "上网电量", "27480", "结算金额", "8409.37"} {
		if !strings.Contains(got, want) {
			t.Fatalf("aggregate lost chunk content %q\n--- got ---\n%s", want, got)
		}
	}
	if strings.Contains(got, "不该进提取正文") {
		t.Fatalf("non-text chunk must be excluded:\n%s", got)
	}
}

func TestExtractFastPathUserFieldsAndRegexPatch(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "bill_period", Label: "账单周期", Regex: `账单周期[：:]\s*(\d{4}-\d{2})`, Required: true, Enabled: true},
		{Name: "generation_kwh", Label: "发电量", Regex: `发电量[：:]\s*([0-9.]+)`, Required: true, Enabled: true},
		{Name: "remark", Label: "备注", Regex: `备注[：:]\s*(.+)`, Required: false, Enabled: true},
	}}
	text := "账单周期: 2026-10\n发电量: 194489\n备注: 已核对"
	values, complete := FastPathUserFields(cfg, text)
	if !complete {
		t.Fatalf("required fields should be complete: %#v", values)
	}
	if values["bill_period"] != "2026-10" || values["generation_kwh"] != "194489" {
		t.Fatalf("unexpected values: %#v", values)
	}
	// 缺字段用用户正则补洞
	partial := map[string]string{"bill_period": "2026-10"}
	patched := RegexPatchUserFields(cfg, partial, text)
	if partial["generation_kwh"] != "194489" || partial["remark"] != "已核对" {
		t.Fatalf("regex patch failed: %#v patched=%v", partial, patched)
	}
	// required 未命中 → 不能走快筛
	_, complete = FastPathUserFields(cfg, "账单周期: 2026-10")
	if complete {
		t.Fatal("missing required field must disable fast-path")
	}
}

func TestExtractAdaptivePromptCompact(t *testing.T) {
	cfg := &types.KbExtractConfig{
		PromptTemplate: "只提取票面字段",
		Fields: []types.ExtractFieldConfig{
			{Name: "generation_kwh", Type: "number", Desc: "光伏发电量", Rule: "取本期数值", Required: true, Enabled: true},
			{Name: "disabled", Type: "string", Enabled: false},
		},
	}
	prompt := BuildAdaptiveExtractPrompt(cfg, "发电量: 194489\n\n\n上网电量: 27480")
	for _, want := range []string{"generation_kwh", "number", "光伏发电量", "取本期数值", "只提取票面字段", "<document>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "disabled") || strings.Contains(prompt, "\n\n\n") {
		t.Fatalf("prompt must be compact and skip disabled fields:\n%s", prompt)
	}
}

func TestExtractFastPathSolarBill(t *testing.T) {
	text := "账单周期: 2026-10\n" +
		"| 项目 | 数值 |\n| --- | --- |\n" +
		"| 光伏发电量 | 194,489 千瓦时 |\n" +
		"| 上网电量 | 27,480 kWh |\n" +
		"| 自发自用电量 | 167,009 |\n" +
		"| 结算单价 | 0.306018 |\n" +
		"| 结算金额 | 8,409.37 元 |\n"
	item, ok := FastPathSolarBill(text, "2026.10光伏账单.pdf", nil)
	if !ok || item == nil {
		t.Fatalf("complete bill should hit fast-path:\n%s", CanonicalizeParsedDocument(text))
	}
	if item.BillPeriod != "2026-10" {
		t.Fatalf("bill period = %q, want 2026-10", item.BillPeriod)
	}
	if item.GenerationKwh != 194489 || item.GridKwh != 27480 || item.SettlementAmount != 8409.37 {
		t.Fatalf("values wrong: gen=%v grid=%v amount=%v",
			item.GenerationKwh, item.GridKwh, item.SettlementAmount)
	}
	if item.Fields["self_use_kwh"] != float64(167009) {
		t.Fatalf("self-use not collected: %#v", item.Fields)
	}
	// 用户自定义正则实时覆盖内置规则
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{{
		Name: "grid_kwh", Label: "上网电量", Regex: `上网电量[：:]\s*([0-9.]+)`, Enabled: true,
	}}}
	if _, ok := FastPathSolarBill("账单周期: 2026-10\n发电量: 100\n上网电量: 40\n结算金额: 12", "x.pdf", cfg); !ok {
		t.Fatal("user regex should still allow a complete fast-path")
	}
	// 字段不全 / 数值明显不合理 → 必须回退 LLM
	for _, bad := range []string{
		"账单周期: 2026-10\n发电量: 100",
		"账单周期: 2026-10\n发电量: 10\n上网电量: 400\n结算金额: 12",
		"发电量: 100\n上网电量: 40\n结算金额: 12",
	} {
		if _, ok := FastPathSolarBill(bad, "x.pdf", nil); ok {
			t.Fatalf("incomplete/inconsistent bill must not hit fast-path:\n%s", bad)
		}
	}
}

// TestExtractFastPathSolarBillRealOCR 用真机 2026.10光伏账单.pdf 的解析原文
// （MinerU 风格 Markdown 表格 + OCR 字间空格）锁定字段不全事故不再复现。
func TestExtractFastPathSolarBillRealOCR(t *testing.T) {
	raw := "||||国|网|重|庆 市|电 力 公|司||\n" +
		"|---|---|---|---|---|---|---|---|---|\n" +
		"||账单周期 2026-10-01 至 2026-10-31||发电户号 户名 纳税人类型 并网电压等级|电 交流380V|费 账 5000303216586 重庆星达铜业有限公司 一般纳税人|单|服务单位 发电地址 消纳方式 发电方式|璧城供电服务中心 自发自用余电上网 光伏发电|\n\n" +
		"发 电 量 66026 千瓦时 上 网 电 量 2970 千瓦时 结 算 金 额 1125.14元\n\n" +
		"|类别|电量|电价|电费|\n|---|---|---|---|\n|分布式上网电费|2970|0.378834|1125.14|\n"
	item, ok := FastPathSolarBill(raw, "2026.10光伏账单.pdf", nil)
	if !ok || item == nil {
		t.Fatalf("real OCR bill must hit fast-path, canonical:\n%s", CanonicalizeParsedDocument(raw))
	}
	if item.BillPeriod != "2026-10" {
		t.Fatalf("bill period = %q, want 2026-10", item.BillPeriod)
	}
	if item.GenerationKwh != 66026 || item.GridKwh != 2970 || item.SettlementAmount != 1125.14 {
		t.Fatalf("real OCR values wrong: gen=%v grid=%v amount=%v",
			item.GenerationKwh, item.GridKwh, item.SettlementAmount)
	}
}

func TestExtractAdaptiveTimeout(t *testing.T) {
	ctx, cancel := WithAdaptiveExtractTimeout(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("adaptive LLM call must have a deadline")
	}
	if d := time.Until(deadline); d <= 0 || d > AdaptiveExtractLLMTimeout+time.Second {
		t.Fatalf("unexpected deadline: %v", d)
	}
}
