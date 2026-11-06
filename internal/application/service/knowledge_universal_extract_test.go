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

// TestExtractFastPathSolarBillFullFields 第 43 轮回归：快筛命中后必须把
// 表头字段 / 年累计 / 环比 / 关口明细 / 顶层示数镜像一并产出。
//
// 事故：2026.10光伏账单.pdf 只剩「账单周期/发电量/上网电量/结算金额」有值，
// 发电户号、户名、纳税人类型、并网电压、消纳方式、发电方式、服务单位、
// 发电地址、年累计、上期/本期示数、倍率、抄见/计费电量全是空或 0。
// 根因是快筛只产出 4 个概览字段就短路，且 handler 在快筛路径跳过正则补洞。
func TestExtractFastPathSolarBillFullFields(t *testing.T) {
	raw := "|||国|网|重|庆 市|电 力 公|司||\n" +
		"|---|---|---|---|---|---|---|---|---|\n" +
		"||账单周期 2026-10-01 至 2026-10-31||发电户号 户名 纳税人类型 并网电压等级|电 交流380V|" +
		"费 账 5000303216586 重庆星达铜业有限公司 一般纳税人|单|服务单位 发电地址 消纳方式 发电方式|" +
		"璧城供电服务中心 重庆市璧山区青杠街道莲花社区居民委员会106号新立路106号 自发自用余电上网 光伏发电|\n\n" +
		"发 电 量 66026 千瓦时 上 网 电 量 2970 千瓦时 结 算 金 额 1125.14元\n\n" +
		"本期您的上网结算电量为2970,较上期 上网关口 (电能表编号:5030001221300052554525)重庆星达铜业有限公司增长-83.08%。\n" +
		"本期电量明细截止本期,您的年累计上网电量为117960 。\n" +
		"|示数类型|上期示数|本期示数|倍率|抄见电量|计费电量||\n|---|---|---|---|---|---|---|\n" +
		"|反向有功(总)|38.36|39.35|3000|2970|2970||\n\n" +
		"|类别|电量|电价|电费|\n|---|---|---|---|\n|分布式上网电费|2970|0.378834|1125.14|\n\n" +
		"* 发电补助电费明细发电关口 (电能表编号:5030001221300051528459) 本期电量明细\n" +
		"|示数类型|上期示数|本期示数|倍率|抄见电量|计费电量|\n|---|---|---|---|---|---|\n" +
		"|反向有功(总)|3295.58|3461.05|120|19856|0|\n"

	item, ok := FastPathSolarBill(raw, "2026.10光伏账单.pdf", nil)
	if !ok || item == nil {
		t.Fatalf("complete bill should hit fast-path, canonical:/n%s", CanonicalizeParsedDocument(raw))
	}

	// 概览四件套（原有契约，不得回退）
	if item.BillPeriod != "2026-10" || item.GenerationKwh != 66026 ||
		item.GridKwh != 2970 || item.SettlementAmount != 1125.14 {
		t.Fatalf("overview fields regressed: %+v", item)
	}

	// 表头字段（本次事故的全部缺失项）
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"account_no", item.AccountNo, "5000303216586"},
		{"account_name", item.AccountName, "重庆星达铜业有限公司"},
		{"taxpayer_type", item.TaxpayerType, "一般纳税人"},
		{"voltage_level", item.VoltageLevel, "交流380V"},
		{"consumption_mode", item.ConsumptionMode, "自发自用余电上网"},
		{"generation_mode", item.GenerationMode, "光伏发电"},
		{"supply_unit", item.SupplyUnit, "璧城供电服务中心"},
	} {
		if tc.got == "" {
			t.Errorf("%s missing (want %q), fields=%#v", tc.name, tc.want, item.Fields)
			continue
		}
		if !strings.Contains(tc.got, strings.TrimSpace(tc.want)) && tc.name != "address" {
			t.Errorf("%s = %q, want to contain %q", tc.name, tc.got, tc.want)
		}
	}
	if item.Address == "" || !strings.Contains(item.Address, "重庆市璧山区") {
		t.Errorf("address missing or wrong: %q", item.Address)
	}

	// 年累计 / 环比
	if item.CumulativeKwh != 117960 {
		t.Errorf("cumulative_kwh = %v, want 117960", item.CumulativeKwh)
	}
	if item.MomChange == "" || !strings.Contains(item.MomChange, "83.08") {
		t.Errorf("mom_change = %q, want contains 83.08", item.MomChange)
	}

	// 关口明细 + 顶层示数镜像（旧版全 0，抽屉一屏「请输入」）
	if len(item.Gateways) == 0 {
		t.Fatal("gateways missing: 上网/发电关口明细必须结构化")
	}
	up := item.Gateways[0]
	if up.GatewayType != "上网关口" {
		t.Errorf("first gateway = %q, want 上网关口", up.GatewayType)
	}
	if up.MeterNo != "5030001221300052554525" {
		t.Errorf("meter_no = %q", up.MeterNo)
	}
	if len(up.Readings) != 1 {
		t.Fatalf("upstream gateway readings = %d, want 1", len(up.Readings))
	}
	rd := up.Readings[0]
	if rd.Prev != 38.36 || rd.Curr != 39.35 || rd.Multiplier != 3000 ||
		rd.ReadingKwh != 2970 || rd.BillKwh != 2970 {
		t.Errorf("reading wrong: %+v", rd)
	}
	if len(up.Fees) != 1 || up.Fees[0].Category != "分布式上网电费" ||
		up.Fees[0].Fee != 1125.14 || up.Fees[0].Rate != 0.378834 {
		t.Errorf("upstream fees wrong: %+v", up.Fees)
	}

	if item.MeterPrev != 38.36 || item.MeterCurr != 39.35 || item.MeterRatio != 3000 ||
		item.MeterReading != 2970 || item.MeterBill != 2970 {
		t.Errorf("top-level meter mirror wrong: prev=%v curr=%v ratio=%v reading=%v bill=%v",
			item.MeterPrev, item.MeterCurr, item.MeterRatio, item.MeterReading, item.MeterBill)
	}

	// 发电关口也必须结构化（N 个关口）
	if len(item.Gateways) < 2 {
		t.Errorf("generation gateways missing: got %d", len(item.Gateways))
	}
	if g := item.Gateways[1]; g.GatewayType != "发电关口" || g.MeterNo != "5030001221300051528459" {
		t.Errorf("second gateway = %+v", g)
	}
}

// TestExtractFastPathSolarBillUserFieldOnFastPath 快筛路径同样要应用用户配置正则。
//
// 事故：handler 用 `if !usedFastPath` 包住 ApplySolarRegexPatch，于是快筛命中
// 的文件永远不会补用户新增字段（配置改了也没反应）。
func TestExtractFastPathSolarBillUserFieldOnFastPath(t *testing.T) {
	text := "账单周期 2026-10-01 至 2026-10-31\n发电量 66026 上网电量 2970 结算金额 1125.14\n" +
		"发电户号 5000303216586 户名 重庆星达铜业有限公司\n" +
		"自定义标记 光伏A类项目\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{{
		Name: "project_tag", Label: "项目标记",
		Regex: `自定义标记[：:\s]*([^\n]+)`, Enabled: true,
	}}}
	item, ok := FastPathSolarBill(text, "2026.10光伏账单.pdf", cfg)
	if !ok || item == nil {
		t.Fatalf("fast-path should hit:/n%s", CanonicalizeParsedDocument(text))
	}
	if v, _ := item.Fields["project_tag"].(string); strings.TrimSpace(v) == "" {
		t.Fatalf("user-configured field must be patched on fast-path too: %#v", item.Fields)
	}
}
