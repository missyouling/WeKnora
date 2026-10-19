package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// InvoiceRuleKind 规则提取命中的格式类型（仅用于日志与诊断）。
type InvoiceRuleKind string

const (
	InvoiceRuleKindNone          InvoiceRuleKind = ""
	InvoiceRuleKindLine          InvoiceRuleKind = "line"
	InvoiceRuleKindMarkdownTable InvoiceRuleKind = "markdown_table"
	InvoiceRuleKindFlat          InvoiceRuleKind = "flat"
)

// InvoiceRuleOutcome 是三套确定性前置规则的统一产物。
//
// 抽出来的目的（2026-10-19 审查 P0-2）：沙箱测试与正式提取此前有各自的
// 「规则 → LLM」编排，导致沙箱看不到规则短路、内容上限也不同（120000 bytes
// vs 24000 runes），出现「沙箱通过、生产失败」。现在两边都调用本函数。
type InvoiceRuleOutcome struct {
	Result *InvoiceExtractionResult
	Hit    bool
	Kind   InvoiceRuleKind
	// IncompleteCount / Total 用于半残守卫：
	//   全部缺核心字段（IncompleteCount == Total）→ 调用方回退 LLM；
	//   部分缺 → 保留结果并记 warning。
	IncompleteCount int
	Total           int
}

// AllIncomplete 判断命中结果是否「整批都缺核心字段」（应回退 LLM）。
func (o InvoiceRuleOutcome) AllIncomplete() bool {
	return o.Hit && o.Total > 0 && o.IncompleteCount >= o.Total
}

// ExtractInvoiceByRules 依次尝试三套确定性规则：Line → MarkdownTable → FlatLayout，
// 并做半残统计。
//
// 顺序不可调换：Line 覆盖旧纯文本「号码行+金额行」；MarkdownTable 覆盖 invoice2.html
// 模板；FlatLayout 覆盖 docreader 常见的扁平布局。三者都是毫秒级、无模型调用。
// LLM 只在前两套都不命中（或整批半残）时才被调用。
func ExtractInvoiceByRules(chunks []*types.Chunk, docHint string) InvoiceRuleOutcome {
	out := InvoiceRuleOutcome{}
	if res, ok := ExtractInvoicesByLineRulesFromChunks(chunks); ok {
		out.Result, out.Hit, out.Kind = res, true, InvoiceRuleKindLine
	}
	if !out.Hit {
		if res, ok := ExtractInvoicesByMarkdownTableRulesFromChunks(chunks, docHint); ok {
			out.Result, out.Hit, out.Kind = res, true, InvoiceRuleKindMarkdownTable
		}
	}
	if !out.Hit {
		if res, ok := ExtractInvoicesByFlatLayoutFromChunks(chunks); ok {
			out.Result, out.Hit, out.Kind = res, true, InvoiceRuleKindFlat
		}
	}
	if !out.Hit || out.Result == nil {
		return out
	}
	out.Total = len(out.Result.Invoices)
	for _, inv := range out.Result.Invoices {
		if InvoiceRuleResultIncomplete(&InvoiceExtractionResult{
			Kind: "invoice", Invoices: []InvoiceExtractionItem{inv},
		}) {
			out.IncompleteCount++
		}
	}
	return out
}

// InvoiceRuleIncompleteNotice 生成半残提示文案（部分缺字段时写进 extract_error，
// 全部缺时由调用方回退 LLM，不使用该文案）。
func InvoiceRuleIncompleteNotice(incomplete, total int) string {
	if incomplete <= 0 || total <= 0 {
		return ""
	}
	return "规则提取有 " + itoa(incomplete) + "/" + itoa(total) + " 张票字段不完整（可能票面格式变化），建议复核"
}

// InvoiceSamplingNotice 生成「文档超长被采样」的提示文案。
// 采样策略保留头 60% + 中 20% + 尾 20%，中段被跳过的部分可能含发票。
func InvoiceSamplingNotice(maxRunes int) string {
	return "文档超出单批上限（" + itoa(maxRunes) + " 字），已按时序采样后送模型，中段内容可能被跳过、可能遗漏部分票据，建议分段核查"
}

// NewExtractSamplingIssue 生成「文档超长被采样」的 warn 级质量问题条目。
//
// Index = -1 表示结果级问题（不针对某一张票），与现有 issues 语义一致；
// 前端 ExtractQualityPanel 按 severity 渲染为黄色告警。
func NewExtractSamplingIssue(maxRunes int) ExtractFieldIssue {
	return ExtractFieldIssue{
		Index:    -1,
		Field:    "",
		Kind:     "truncated",
		Detail:   InvoiceSamplingNotice(maxRunes),
		Severity: issueSeverityWarn,
	}
}

// ExtractInvoiceForSandbox 以**与正式提取完全一致的管线**跑一次发票提取：
//
//	规则（Line → MarkdownTable → FlatLayout，含半残守卫）
//	→ 未命中/整批半残才回退 LLM（同一分批器、同一引导语）
//	→ Normalize + 确定性覆盖 + 物理页码
//	→ FinalizeInvoiceRuleResult（日期归一 + 必填校验 + 正则补洞）
//
// 沙箱此前走的是 `ExtractFleetDocumentFromContent`（map 视图、120000 bytes、
// 无前置规则、无分类锁定），与生产管线差异极大 —— "沙箱通过"并不能预测
// 正式提取的表现。改为调用本函数后，两边共用同一份实现。
//
// model 为 nil 时只跑规则（沙箱可据此验证"规则能否覆盖该票面"）。
func ExtractInvoiceForSandbox(
	ctx context.Context,
	model chat.Chat,
	chunks []*types.Chunk,
	cfg *types.KbExtractConfig,
	docHint string,
) (*InvoiceExtractionResult, error) {
	outcome := ExtractInvoiceByRules(chunks, docHint)
	var res *InvoiceExtractionResult
	sampledIssue := (*ExtractFieldIssue)(nil)
	switch {
	case outcome.Hit && !outcome.AllIncomplete():
		res = outcome.Result
		res.ExtractError = InvoiceRuleIncompleteNotice(outcome.IncompleteCount, outcome.Total)
	case model == nil:
		res = &InvoiceExtractionResult{Kind: "invoice"}
	default:
		report := BuildInvoiceExtractionBatchesWithReport("", "", chunks, 0)
		parts := make([]*InvoiceExtractionResult, 0, len(report.Batches))
		for i, batch := range report.Batches {
			content := strings.TrimSpace(batch.Content)
			if content == "" {
				continue
			}
			guide := "重要提示：以下文本是文档的第 " + itoa(i+1) + "/" + itoa(len(report.Batches)) +
				" 批内容，请逐张提取本批中出现的全部发票，一张都不可遗漏。"
			part, err := ExtractInvoicesFromContentWithRulesGuided(ctx, model, content, cfg, guide)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		res = MergeInvoiceResults(parts)
		if report.Sampled {
			issue := NewExtractSamplingIssue(report.MaxRunes)
			sampledIssue = &issue
		}
	}
	if res == nil {
		res = &InvoiceExtractionResult{Kind: "invoice"}
	}

	src := BuildExtractSourceText(chunks)
	NormalizeInvoiceExtractionResult(res)
	OverlayDeterministicInvoiceFields(res, chunks)
	// 与规则/LLM 收尾同源：日期归一 + 必填 + 配置级正则补洞。
	// Finalize 会用重新校验的结果**覆盖** res.ExtractIssues，因此采样告警
	// 必须在它之后追加，否则会被吞掉。
	FinalizeInvoiceRuleResult(ctx, res, src.Content, cfg)
	if sampledIssue != nil {
		res.ExtractIssues = append(res.ExtractIssues, *sampledIssue)
	}
	return res, nil
}

// MergeInvoiceResults 把多批结果合并成一份（按发票号去重，保留首次出现）。
//
// 与 handler 内的合并逻辑同源：并发批次写入预分配切片后，必须由主 goroutine
// 顺序合并，保证发票顺序与文档顺序一致（历史事故：并发 append 导致页码错位）。
func MergeInvoiceResults(parts []*InvoiceExtractionResult) *InvoiceExtractionResult {
	merged := &InvoiceExtractionResult{Kind: "invoice"}
	seen := make(map[string]struct{})
	for _, part := range parts {
		if part == nil {
			continue
		}
		if part.ExtractError != "" && merged.ExtractError == "" {
			merged.ExtractError = part.ExtractError
		}
		for _, inv := range part.Invoices {
			no := strings.TrimSpace(inv.InvoiceNo)
			if no != "" {
				if _, dup := seen[no]; dup {
					continue
				}
				seen[no] = struct{}{}
			}
			merged.Invoices = append(merged.Invoices, inv)
		}
	}
	return merged
}
