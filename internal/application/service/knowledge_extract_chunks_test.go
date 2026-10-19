package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// mkChunk 构造一个测试用 chunk。
func mkChunk(idx int, typ types.ChunkType, content string, pages ...int) *types.Chunk {
	locs := make(types.SourceLocators, 0, len(pages))
	for _, p := range pages {
		locs = append(locs, types.SourceLocator{Type: types.SourceLocatorPDF, Page: p})
	}
	return &types.Chunk{
		ChunkIndex:     idx,
		ChunkType:      typ,
		Content:        content,
		SourceLocators: locs,
	}
}

// TestExtractSourceChunkTypesIncludesMultimodal 锁定「提取必须读多模态 chunk」。
//
// 事故背景：仓储层 ListChunksByKnowledgeID 的 SQL 硬编码 chunk_type='text'，
// 5 个业务提取入口全用它，导致构建函数里对 image_ocr / image_caption 的放行
// 判断成为死代码；纯扫描件（PaddleOCR-VL / Simple+VLM）提取到空文本。
func TestExtractSourceChunkTypesIncludesMultimodal(t *testing.T) {
	got := ExtractSourceChunkTypes()
	want := map[types.ChunkType]bool{
		types.ChunkTypeText:         false,
		types.ChunkTypeImageOCR:     false,
		types.ChunkTypeImageCaption: false,
	}
	for _, ct := range got {
		if _, ok := want[ct]; !ok {
			t.Fatalf("unexpected chunk type %q in extraction source types", ct)
		}
		want[ct] = true
	}
	for ct, seen := range want {
		if !seen {
			t.Fatalf("chunk type %q missing from ExtractSourceChunkTypes()", ct)
		}
	}
}

// TestChunksForExtractionKeepsImageOCR 锁定「只有 image_ocr 也必须能提取」。
func TestChunksForExtractionKeepsImageOCR(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeImageOCR, "发票号码: 12345678 价税合计 ¥100.00", 1),
	}
	got := ChunksForExtraction(chunks)
	if len(got) != 1 {
		t.Fatalf("want 1 chunk kept, got %d", len(got))
	}
	if got[0].ChunkType != types.ChunkTypeImageOCR {
		t.Fatalf("image_ocr chunk must be kept, got %q", got[0].ChunkType)
	}
}

// TestChunksForExtractionDropsSummaryAndEmpty 锁定「summary 等非正文 chunk 不进提取，
// 空内容不占位」。
func TestChunksForExtractionDropsSummaryAndEmpty(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeSummary, "文档摘要", 1),
		mkChunk(1, types.ChunkTypeText, "   ", 1),
		mkChunk(2, types.ChunkTypeText, "正文", 1),
	}
	got := ChunksForExtraction(chunks)
	if len(got) != 1 || got[0].Content != "正文" {
		t.Fatalf("want only the text body chunk, got %d: %+v", len(got), got)
	}
}

// TestChunksForExtractionDedupesAdjacentIdenticalText 锁定「同一图片的 OCR 与
// caption 常输出 identical 文本，相邻完全重复需去重」，避免同一段话喂两遍。
func TestChunksForExtractionDedupesAdjacentIdenticalText(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeImageOCR, "发票号码: 12345678", 1),
		mkChunk(1, types.ChunkTypeImageCaption, "发票号码: 12345678", 1),
		mkChunk(2, types.ChunkTypeText, "其他正文", 1),
	}
	got := ChunksForExtraction(chunks)
	if len(got) != 2 {
		t.Fatalf("adjacent identical text must be deduped, want 2 got %d", len(got))
	}
}

// TestChunkPageReadsSourceLocator 锁定物理页码来源。
func TestChunkPageReadsSourceLocator(t *testing.T) {
	if got := ChunkPage(mkChunk(0, types.ChunkTypeText, "x", 7)); got != 7 {
		t.Fatalf("want page 7, got %d", got)
	}
	// 多个 locator 取第一个非零页
	c := mkChunk(0, types.ChunkTypeText, "x", 0, 9)
	if got := ChunkPage(c); got != 9 {
		t.Fatalf("want page 9 from second locator, got %d", got)
	}
	// 无 locator → 0（调用方回退序号）
	if got := ChunkPage(mkChunk(0, types.ChunkTypeText, "x")); got != 0 {
		t.Fatalf("want 0 when no locators, got %d", got)
	}
}

// TestBuildExtractSourceTextMarksPages 锁定「拼装文本时记录每个 chunk 的页偏移」。
func TestBuildExtractSourceTextMarksPages(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeText, "AAA", 3),
		mkChunk(1, types.ChunkTypeText, "BBB", 4),
	}
	src := BuildExtractSourceText(chunks)
	if !strings.Contains(src.Content, "AAA") || !strings.Contains(src.Content, "BBB") {
		t.Fatalf("content must contain both chunks, got %q", src.Content)
	}
	if len(src.Markers) != 2 {
		t.Fatalf("want 2 page markers, got %d", len(src.Markers))
	}
	if src.PageAt(0) != 3 {
		t.Fatalf("offset 0 should map to page 3, got %d", src.PageAt(0))
	}
	idx := strings.Index(src.Content, "BBB")
	if got := src.PageAt(idx); got != 4 {
		t.Fatalf("BBB offset should map to page 4, got %d", got)
	}
}

// flatSample 是一张结构完整的扁平布局票（含购销方/金额/税额/价税合计），
// 供页码回填与沙箱同管线测试使用。
func flatSample(invoiceNo string) string {
	return `发票代码: 050002400112

发票号码: ` + invoiceNo + `

开票日期: 2025年02月08日

<table border=1>
<tr><td>购买方</td><td colspan="3">名 称：示例铜业有限公司\n纳税人识别号：91500000MA5U54AJ0A</td></tr>
<tr><td>项目名称</td><td>车牌号</td><td>类型</td><td>通行日期起</td><td>通行日期止</td><td>金 额</td><td>税率</td><td>税 额</td></tr>
<tr><td>经营租赁代收通行费</td><td>渝C12345</td><td>货车</td><td>20250114</td><td>20250119</td><td>73.28</td><td>不征税</td><td>$ ^{{***}} $</td></tr>
<tr><td>价税合计（大写）</td><td colspan="7">柒拾叁元贰角捌分 （小写） ¥73.28</td></tr>
<tr><td>销售方</td><td colspan="3">名 称：示例科技公司\n纳税人识别号：91500000327752319D</td></tr>
</table>`
}

// TestImageOCRChunksFeedAllBusinessBuilders 锁定「只有 image_ocr 的文档，
// 5 个业务提取器的内容拼装都不为空」。
//
// 这是 P0-1 的行为级断言：仓储层曾只返回 text chunk，导致扫描件提取到空文本。
func TestImageOCRChunksFeedAllBusinessBuilders(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeImageOCR, "扫描件正文：发票号码 34013247 价税合计 73.28", 1),
	}
	if got := BuildInvoiceExtractionContent("f.pdf", "", chunks); !strings.Contains(got, "34013247") {
		t.Fatalf("invoice content lost image_ocr text: %q", got)
	}
	if got := BuildContractExtractionContent("f.pdf", "", chunks); !strings.Contains(got, "34013247") {
		t.Fatalf("contract content lost image_ocr text: %q", got)
	}
	if got := BuildRegulationExtractionContent("f.pdf", "", chunks); !strings.Contains(got, "34013247") {
		t.Fatalf("regulation content lost image_ocr text: %q", got)
	}
	if got := BuildAwardPunishExtractionContent("f.pdf", "", chunks); !strings.Contains(got, "34013247") {
		t.Fatalf("award_punish content lost image_ocr text: %q", got)
	}
	batches := BuildInvoiceExtractionBatches("f.pdf", "", chunks, 0)
	if len(batches) == 0 || !strings.Contains(batches[0].Content, "34013247") {
		t.Fatalf("batch builder lost image_ocr text: %+v", batches)
	}
}

// TestSourceLocatorPageFlowsIntoInvoicePageNo 锁定「物理页码透传」：
// chunk 带 SourceLocators.Page=5 时，提取出的发票 page_no 必须是 5。
// 注意 Page 与 PageNo 是两回事：Page 是发票序号（扁平提取器边解析边赋值），
// PageNo 才是物理页码（AssignPhysicalPages 按发票号在原文中的位置反查）。
func TestSourceLocatorPageFlowsIntoInvoicePageNo(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeText, flatSample("34013247"), 5),
	}
	res, ok := ExtractInvoicesByFlatLayoutFromChunks(chunks)
	require.True(t, ok)
	require.Len(t, res.Invoices, 1)
	assert.Equal(t, 5, res.Invoices[0].PageNo, "page_no 必须来自 SourceLocators")
	assert.Equal(t, 1, res.Invoices[0].Page, "Page 是序号，扁平提取器已赋值")
}

// TestPageNoFallsBackToSequenceWithoutLocators 锁定「无页码信息的引擎回退序号」。
func TestPageNoFallsBackToSequenceWithoutLocators(t *testing.T) {
	chunks := []*types.Chunk{
		mkChunk(0, types.ChunkTypeText, flatSample("34013247")),
	}
	res, ok := ExtractInvoicesByFlatLayoutFromChunks(chunks)
	require.True(t, ok)
	NormalizeInvoiceExtractionResult(res)
	require.Len(t, res.Invoices, 1)
	assert.Equal(t, 1, res.Invoices[0].Page, "序号仍为 1")
	assert.Equal(t, 1, res.Invoices[0].PageNo, "无物理页码时 page_no 回退为序号")
}

// TestBatchInjectsPageAnchorForLLM 锁定「批次文本带 [Page N] 锚点」，
// 且无 locator 的引擎不注入锚点。
func TestBatchInjectsPageAnchorForLLM(t *testing.T) {
	withPage := []*types.Chunk{mkChunk(0, types.ChunkTypeText, flatSample("34013247"), 7)}
	b := BuildInvoiceExtractionBatches("f.pdf", "", withPage, 0)
	require.NotEmpty(t, b)
	assert.Contains(t, b[0].Content, "[Page 7]", "有物理页码时必须注入锚点")

	noPage := []*types.Chunk{mkChunk(0, types.ChunkTypeText, flatSample("34013247"))}
	b2 := BuildInvoiceExtractionBatches("f.pdf", "", noPage, 0)
	require.NotEmpty(t, b2)
	assert.NotContains(t, b2[0].Content, "[Page ", "无物理页码时不得注入锚点")
}

// TestBatchReportFlagsSampling 锁定「超长采样必须显式上报」。
func TestBatchReportFlagsSampling(t *testing.T) {
	huge := strings.Repeat("发票内容", invoiceExtractMaxContentRunes) // 远超单批上限
	chunks := []*types.Chunk{mkChunk(0, types.ChunkTypeText, huge, 1)}
	report := BuildInvoiceExtractionBatchesWithReport("f.pdf", "", chunks, 0)
	assert.True(t, report.Sampled, "超过单批上限必须标记 Sampled")
	assert.Equal(t, invoiceExtractMaxContentRunes, report.MaxRunes)
}

// TestSandboxInvoicePipelineMatchesRules 锁定「沙箱与正式提取同管线」：
// 规则可完整覆盖的票面，沙箱（model=nil）产出与规则结果一致，且带上校验问题。
func TestSandboxInvoicePipelineMatchesRules(t *testing.T) {
	chunks := []*types.Chunk{mkChunk(0, types.ChunkTypeText, flatSample("34013247"), 3)}
	cfg := &types.KbExtractConfig{Scope: "invoice", CertType: "通行费发票"}

	rule := ExtractInvoiceByRules(chunks, "f")
	require.True(t, rule.Hit, "fixture 必须命中规则")
	require.False(t, rule.AllIncomplete(), "fixture 必须是完整票面")

	got, err := ExtractInvoiceForSandbox(context.Background(), nil, chunks, cfg, "f")
	require.NoError(t, err)
	require.Len(t, got.Invoices, len(rule.Result.Invoices))
	assert.Equal(t, rule.Result.Invoices[0].InvoiceNo, got.Invoices[0].InvoiceNo)
	assert.Equal(t, 3, got.Invoices[0].PageNo, "沙箱也要回填物理页码")
	assert.Equal(t, "invoice", got.Kind)
}

// TestSandboxFieldsLandInConfigKeySpace 锁定「沙箱回显键名必须落在配置空间」：
// 配置字段名是 camelCase（invoiceNo），而发票结构体的 JSON tag 是 snake_case
// （invoice_no）—— 若直接下发 marshal 结果，missing 会把全部字段误报为未提取到。
func TestSandboxFieldsLandInConfigKeySpace(t *testing.T) {
	chunks := []*types.Chunk{mkChunk(0, types.ChunkTypeText, flatSample("34013247"), 3)}
	cfg := &types.KbExtractConfig{
		Scope: "invoice",
		Fields: []types.ExtractFieldConfig{
			{Name: "invoiceNo", Enabled: true},
			{Name: "invoiceCode", Enabled: true},
			{Name: "buyerName", Enabled: true},
			{Name: "totalAmount", Enabled: true},
		},
	}
	got, err := ExtractInvoiceForSandbox(context.Background(), nil, chunks, cfg, "f")
	require.NoError(t, err)
	require.Len(t, got.Invoices, 1)

	fields := InvoiceSandboxFields(got.Invoices[0], cfg.Fields)
	// 每个配置字段名都要能取到值，且键名与配置一字不差（camelCase）。
	require.Len(t, fields, len(cfg.Fields), "配置字段必须全部命中: %+v", fields)
	for name, v := range fields {
		assert.NotEmpty(t, v, "字段 %s 有键无值", name)
	}
	assert.Equal(t, "34013247", fields["invoiceNo"])
	assert.Contains(t, fields, "invoiceCode")
	// snake_case tag 不得出现在回显键里。
	for k := range fields {
		assert.NotContains(t, k, "_", "回显键必须落在配置空间: %q", k)
	}
}
