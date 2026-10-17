package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取器的 golden 回归集（解析层）。
//
// 目的：提取器精度依赖「模型输出 → 结构化结果」这条解析链。模型输出形态会漂移
// （代码围栏、别名键、"null" 字符串、非发票文档、动态字段等），这些漂移是历史上
// 提取失败的主要来源。这里把各种形态固化成样本，任何改动解析/归一化逻辑的行为
// 都会被立刻发现。
//
// 这一层是**确定性的**（不调用模型），因此可以在 CI 里跑。
// 端到端精度（真实文档 → 字段）依赖模型，另以手工/定期脚本验证。

// 分批切分测试：批次大小是"输出长度"与"上下文完整"的平衡点，两侧都踩过坑
// （64 → 16 页合成 1 批输出超长返回空；历史小批 → 发票拆散漏提），必须锁死行为。
// chunk ≠ 页：实测 16 页通行费 PDF 被解析成 49 个 chunk（约每页 3 个）。
// 批次大小必须按 chunk 数换算（8 chunk ≈ 2~3 页，受"输出长度"约束），否则会把文档切成过多小批：
// 曾误按"每批 5 chunk"处理 → 49 chunks 切出 10 批、每批仅 1.6 页，
// 总时长翻倍且因单批内容过少而漏提（16 张只出 6 张）。
func TestExtractBuildInvoiceBatchesDynamicSize(t *testing.T) {
	cases := []struct {
		chunks      int
		wantBatches int
		desc        string
	}{
		{3, 1, "小文档单批"},
		{8, 1, "恰好一批"},
		{16, 2, "16 chunk → 2 批（每批 8 chunk ≈ 2~3 页）"},
		{49, 7, "实测 49 chunk（16 页 PDF）应切 7 批，而不是 10 批"},
		{240, 30, "240 chunk → 30 批（每批 8）"},
	}
	for _, c := range cases {
		chunks := make([]*types.Chunk, 0, c.chunks)
		for i := 0; i < c.chunks; i++ {
			chunks = append(chunks, &types.Chunk{
				ChunkIndex: i,
				ChunkType:  types.ChunkTypeText,
				Content:    fmt.Sprintf("chunk %d", i),
			})
		}
		batches := BuildInvoiceExtractionBatches("doc.pdf", "", chunks, 0)
		if len(batches) != c.wantBatches {
			t.Errorf("%s: %d chunks 应切 %d 批，实际 %d 批", c.desc, c.chunks, c.wantBatches, len(batches))
		}
	}
}

func TestExtractBuildInvoiceBatchesSplitsByChunkCount(t *testing.T) {
	chunks := make([]*types.Chunk, 0, 16)
	for i := 0; i < 16; i++ {
		chunks = append(chunks, &types.Chunk{
			ChunkIndex: i,
			ChunkType:  types.ChunkTypeText,
			Content:    fmt.Sprintf("通行费发票第%d页 内容足够短不会触发rune上限", i),
		})
	}
	batches := BuildInvoiceExtractionBatches("16页通行费.pdf", "汇总单", chunks, 0)
	if len(batches) != 2 {
		t.Fatalf("16 chunks / 每批 8 应切 2 批（8+8），实际 %d 批", len(batches))
	}
	wantStarts := []int{0, 8}
	for i, b := range batches {
		if b.StartIndex != wantStarts[i] {
			t.Errorf("批 %d StartIndex = %d, want %d", i, b.StartIndex, wantStarts[i])
		}
	}
	// 每批内容都应带上文档头（模型需要知道这是发票合集）
	for i, b := range batches {
		if !strings.Contains(b.Content, "16页通行费.pdf") {
			t.Errorf("批 %d 缺少文档名头部", i)
		}
	}
}

func TestExtractBuildInvoiceBatchesSingleBatchForShortDoc(t *testing.T) {
	chunks := make([]*types.Chunk, 0, 3)
	for i := 0; i < 3; i++ {
		chunks = append(chunks, &types.Chunk{
			ChunkIndex: i,
			ChunkType:  types.ChunkTypeText,
			Content:    fmt.Sprintf("单页发票 %d", i),
		})
	}
	batches := BuildInvoiceExtractionBatches("短文档.pdf", "", chunks, 0)
	if len(batches) != 1 {
		t.Fatalf("3 chunks 应为 1 批，实际 %d", len(batches))
	}
	if batches[0].StartIndex != 0 || batches[0].EndIndex != 2 {
		t.Errorf("StartIndex/EndIndex = %d/%d, want 0/2", batches[0].StartIndex, batches[0].EndIndex)
	}
}

func TestGoldenParseInvoiceRuleReply(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		cfg   *types.KbExtractConfig
		check func(t *testing.T, got *InvoiceExtractionResult)
	}{
		{
			name: "标准 JSON",
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"12345678","invoice_date":"2026-01-15","total_amount":100.5}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if got.Kind != "invoice" {
					t.Fatalf("kind = %q, want invoice", got.Kind)
				}
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1", len(got.Invoices))
				}
				inv := got.Invoices[0]
				if inv.InvoiceNo != "12345678" {
					t.Errorf("invoice_no = %q, want 12345678", inv.InvoiceNo)
				}
				if inv.InvoiceDate != "2026-01-15" {
					t.Errorf("invoice_date = %q, want 2026-01-15", inv.InvoiceDate)
				}
				if inv.TotalAmount == nil || *inv.TotalAmount != 100.5 {
					t.Errorf("total_amount = %v, want 100.5", inv.TotalAmount)
				}
			},
		},
		{
			name: "Markdown 代码围栏包裹",
			raw:  "```json\n" + `{"kind":"invoice","invoices":[{"invoice_no":"87654321"}]}` + "\n```",
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1 (围栏必须被剥离)", len(got.Invoices))
				}
				if got.Invoices[0].InvoiceNo != "87654321" {
					t.Errorf("invoice_no = %q, want 87654321", got.Invoices[0].InvoiceNo)
				}
			},
		},
		{
			name: `字符串 "null" 应清洗为空`,
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"null","remark":"null"}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1", len(got.Invoices))
				}
				if got.Invoices[0].InvoiceNo != "" {
					t.Errorf(`invoice_no = %q, want "" (字符串 null 必须清洗为空)`, got.Invoices[0].InvoiceNo)
				}
			},
		},
		{
			// 真实数据验证：435 条 knowledges 中 20 条被模型写成 parseStatus，
			// 而分类配置字段名是 extractStatus → 前端按配置键取值为空，该列渲染为空白。
			name: "模型漂移键 parseStatus 归一到配置字段 extractStatus",
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"12345678","parseStatus":"completed"}]}`,
			cfg: &types.KbExtractConfig{
				Scope:  "invoice",
				Fields: []types.ExtractFieldConfig{{Name: "extractStatus", Type: "string", Enabled: true}},
			},
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1", len(got.Invoices))
				}
				fm := got.Invoices[0].Fields
				if fm == nil {
					t.Fatalf("fields 桶为空: %#v", fm)
				}
				if fm["extractStatus"] != "completed" {
					t.Errorf("fields[extractStatus] = %v, want completed", fm["extractStatus"])
				}
				if _, bad := fm["parseStatus"]; bad {
					t.Errorf("fields 仍残留 parseStatus: %v", fm["parseStatus"])
				}
			},
		},
		{
			name: "非发票文档",
			raw:  `{"kind":"not_invoice","invoices":[]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if got.Kind != "not_invoice" {
					t.Errorf("kind = %q, want not_invoice", got.Kind)
				}
				if len(got.Invoices) != 0 {
					t.Errorf("invoices = %d, want 0", len(got.Invoices))
				}
			},
		},
		{
			name: "多张发票",
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"A1"},{"invoice_no":"A2"},{"invoice_no":"A3"}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 3 {
					t.Fatalf("invoices = %d, want 3", len(got.Invoices))
				}
				want := []string{"A1", "A2", "A3"}
				for i, w := range want {
					if got.Invoices[i].InvoiceNo != w {
						t.Errorf("invoices[%d].invoice_no = %q, want %q", i, got.Invoices[i].InvoiceNo, w)
					}
				}
			},
		},
		{
			name: "空发票列表",
			raw:  `{"kind":"invoice","invoices":[]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 0 {
					t.Errorf("invoices = %d, want 0", len(got.Invoices))
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInvoiceRuleReply(tc.raw, tc.cfg)
			if err != nil {
				t.Fatalf("parseInvoiceRuleReply 解析失败: %v\n原始输出: %s", err, tc.raw)
			}
			if got == nil {
				t.Fatal("解析结果为 nil")
			}
			tc.check(t, got)
		})
	}
}

// TestGoldenBuildInvoiceRuleSystemPrompt 锁定提示词拼装行为：
// 提示词一变，模型输出就变，因此它必须像代码一样被回归保护。
func TestGoldenBuildInvoiceRuleSystemPrompt(t *testing.T) {
	cfg := &types.KbExtractConfig{
		CertType: "通行费",
		Fields: []types.ExtractFieldConfig{
			{Name: "plate_no", Desc: "车牌号", Rule: "备注中以「车牌」开头的号码", Enabled: true},
			{Name: "travel_date", Desc: "通行日期", Enabled: true},
			{Name: "disabled_field", Enabled: false},
		},
	}
	p := buildInvoiceRuleSystemPrompt(cfg)

	// 启用字段必须出现在提示词里
	for _, want := range []string{"plate_no", "travel_date", "车牌号", "通行日期"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词缺少 %q", want)
		}
	}
	// 禁用字段不得出现
	if strings.Contains(p, "disabled_field") {
		t.Error("已禁用字段不应出现在提示词里")
	}
	// 分离要求必须存在（车牌号常藏在备注里）
	if !strings.Contains(p, "分离要求") {
		t.Error("缺少「分离要求」指令，备注中的关键数据不会被分离到独立字段")
	}
}

// TestGoldenBuildFieldsSchema 锁定 JSON Schema 生成：字段名/类型/必填。
func TestGoldenBuildFieldsSchema(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "name", Type: "string", Enabled: true},
			{Name: "amount", Type: "number", Enabled: true},
			{Name: "tags", Type: "array", Enabled: true},
			{Name: "date", Type: "date", Enabled: true},
			{Name: "off", Type: "string", Enabled: false},
		},
	}
	s := BuildFieldsSchema(cfg.Fields)
	for _, want := range []string{`"name"`, `"amount"`, `"tags"`, `"date"`, `"number"`, `"array"`, `"format": "date"`} {
		if !strings.Contains(s, want) {
			t.Errorf("schema 缺少 %q", want)
		}
	}
	if strings.Contains(s, `"off"`) {
		t.Error("禁用字段不应出现在 schema 中")
	}
}

// 注：本包已有 contains([]string, string) 辅助函数（knowledge_span_tracker.go），
// 这里统一用标准库 strings.Contains，避免重名冲突。
