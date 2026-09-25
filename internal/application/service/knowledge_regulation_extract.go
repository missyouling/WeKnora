package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// regulationExtractMaxContentRunes bounds the document text sent to the
// extraction model so the prompt stays inside a normal context window.
const regulationExtractMaxContentRunes = 24000

// RegulationExtractionResult is the strict-JSON shape the model must return.
type RegulationExtractionResult struct {
	Kind         string                        `json:"kind"` // "regulation" | "not_regulation"
	Regulations  []types.RegulationExtractionItem `json:"regulations"`
	ExtractError string                        `json:"extract_error"`
}

// regulationExtractionSystemPrompt instructs the model to return strict JSON
// with the regulation fields found in the document (one regulation per file).
const regulationExtractionSystemPrompt = `你是一个专业的公司制度文档信息提取助手。你的任务是从文档文本中识别并提取制度的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 一份制度文档对应一份制度，regulations 数组只包含一条记录。
3. 制度文档通常包含：制度编号（如"编号：AD-032"）、编制部门、版本、编制日期、制度名称、页码、修改次数等页眉要素，正文包含目的、适用范围、管理原则、条款等。
4. 如果文档内容与制度无关（如发票、合同、清单、报表等），kind 设为 "not_regulation"，regulations 返回空数组。
5. reg_no 为制度编号：提取文档中的编号（如 AD-032、ZD-001），格式保持原文（大写字母-数字）；文档中确实没有编号时返回空字符串（不要编造）。
6. reg_name 为制度名称：通常是文档标题（如"员工转岗管理制度"），去掉"制度"以外的前缀噪音；无法识别时返回空字符串。
7. reg_type 只从以下枚举中选择：人事管理、财务管理、生产管理、行政管理、安全管理、其它制度。根据制度名称或正文关键字判断：含「人事」「员工」「考勤」「绩效」「薪酬」「招聘」「培训」「转岗」「任职」「请假」→人事管理；含「财务」「报销」「预算」「资金」「采购付款」「发票」→财务管理；含「生产」「车间」「工艺」「质量」「设备」「作业」「安全操作」→生产管理；含「行政」「办公」「档案」「会议」「印章」「公文」「接待」→行政管理；含「安全」「消防」「环保」「应急」「危化」→安全管理；都不含时用「其它制度」。
8. dept 为编制部门（如"人力资源和行政管理中心"）；version 为版本号（如"1"）；issue_date 为编制日期（YYYY-MM-DD 格式）；page_count 为页数（如"3"或"共3页"）；modify_count 为修改次数（如"0"）；无法识别时返回空字符串。
9. scope 为适用范围（如"本制度适用于集团公司及各部门及各子公司"，可用首句或概述）；effective_date 为生效日期（YYYY-MM-DD 格式，通常见附则"自发布之日起生效"）；confidentiality 为密级（如"内部""机密""公开"，文档未标注时返回空字符串）；remark 为备注（如修订说明、与旧制度的关系说明）；无法识别时返回空字符串。
10. compiled_by 为编制人（制度末尾签署栏或页眉中的"编制：XXX"）；reviewed_by 为审核人（"审核：XXX"）；approved_by 为批准人（"批准：XXX"）；summary 为制度摘要（用 2-4 句话概括制度目的与主要内容）；文档中未出现时返回空字符串，不要编造。
11. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"regulation","regulations":[{"reg_no":"","reg_name":"","reg_type":"","dept":"","version":"","issue_date":"","page_count":"","modify_count":"","scope":"","effective_date":"","confidentiality":"","remark":"","compiled_by":"","reviewed_by":"","approved_by":"","summary":""}]}`

// BuildRegulationExtractionContent assembles the document text sent to the model.
func BuildRegulationExtractionContent(name, summary string, chunks []*types.Chunk) string {
	parts := make([]string, 0, len(chunks)+2)
	if name = strings.TrimSpace(name); name != "" {
		parts = append(parts, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		parts = append(parts, "Existing summary: "+summary)
	}
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ChunkType != types.ChunkTypeText &&
			chunk.ChunkType != types.ChunkTypeImageOCR &&
			chunk.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		if text := strings.TrimSpace(chunk.Content); text != "" {
			parts = append(parts, text)
		}
	}
	return sampleLongContent(strings.Join(parts, "\n\n"), regulationExtractMaxContentRunes)
}

// ExtractRegulationsFromContent calls the configured chat model and parses its
// strict-JSON reply into a RegulationExtractionResult.
func ExtractRegulationsFromContent(ctx context.Context, model chat.Chat, content string) (*RegulationExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &RegulationExtractionResult{Kind: "not_regulation", ExtractError: "no text content to extract"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "regulation_extract", ""), []chat.Message{
		{Role: "system", Content: regulationExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract regulation fields: %w", err)
	}
	var parsed RegulationExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		return nil, fmt.Errorf("parse regulation extraction response: %w", err)
	}
	return &parsed, nil
}

// RegulationExtractionBatchSize caps how many chunks go into one LLM call.
const RegulationExtractionBatchSize = 10

// BuildRegulationExtractionBatches assembles the document text into independent
// batches so long regulation documents extract reliably.
func BuildRegulationExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = RegulationExtractionBatchSize
	}
	usable := make([]*types.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ChunkType != types.ChunkTypeText &&
			chunk.ChunkType != types.ChunkTypeImageOCR &&
			chunk.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		if strings.TrimSpace(chunk.Content) != "" {
			usable = append(usable, chunk)
		}
	}
	header := make([]string, 0, 2)
	if name = strings.TrimSpace(name); name != "" {
		header = append(header, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		header = append(header, "Existing summary: "+summary)
	}
	headStr := strings.Join(header, "\n\n")

	var batches []string
	for i := 0; i < len(usable); i += batchSize {
		end := i + batchSize
		if end > len(usable) {
			end = len(usable)
		}
		parts := make([]string, 0, end-i+1)
		if headStr != "" {
			parts = append(parts, headStr)
		}
		for _, c := range usable[i:end] {
			parts = append(parts, strings.TrimSpace(c.Content))
		}
		batches = append(batches, sampleLongContent(strings.Join(parts, "\n\n"), regulationExtractMaxContentRunes))
	}
	if len(batches) == 0 {
		batches = []string{""}
	}
	return batches
}

// NormalizeRegulationExtractionResult cleans up a model reply: guarantees a
// known kind, drops empty items, normalizes the regulation type and
// auto-numbers regulations without an explicit number (ZD-YYYYMMDD-NNN).
//
// autoSeqStart is the highest auto-number already used in the knowledge base
// for the current date, so generated numbers stay unique across files.
func NormalizeRegulationExtractionResult(res *RegulationExtractionResult, autoSeqStart int) {
	if res == nil {
		return
	}
	if res.Kind != "not_regulation" {
		res.Kind = "regulation"
	}
	if len(res.Regulations) == 0 {
		return
	}
	now := time.Now()
	kept := res.Regulations[:0]
	for i, r := range res.Regulations {
		r.RegType = NormalizeRegulationTypeFromName(r.RegType)
		// 制度一份文件对应一份制度，不按页展开：Page 恒为 1。
		r.Page = 1
		// 自动编号：无制度编号时按上传日期自动编号 ZD-YYYYMMDD-NNN。
		if strings.TrimSpace(r.RegNo) == "" {
			r.RegNo = fmt.Sprintf("ZD-%s-%03d", now.Format("20060102"), autoSeqStart+i+1)
		}
		kept = append(kept, r)
	}
	res.Regulations = kept
}

// MaxAutoRegulationSeq returns the highest trailing sequence number among
// regulations with an auto-generated number ZD-YYYYMMDD-NNN for the current
// date inside the knowledge base. Used to keep auto-numbers unique across files.
func (s *knowledgeService) MaxAutoRegulationSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "ZD-" + time.Now().Format("20060102") + "-"
	max := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return max, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta regulationMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "regulation" {
				continue
			}
			for _, r := range meta.Regulations {
				if !strings.HasPrefix(r.RegNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(r.RegNo, prefix)
				if v, aerr := strconv.Atoi(n); aerr == nil && v > max {
					max = v
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return max, nil
}

// NormalizeRegulationTypeFromName maps a raw type string onto the fixed enum by
// keyword priority:
//
//	含"人事"/"员工"/"考勤"/"绩效"/"薪酬"/"招聘"/"培训"/"转岗"/"任职"/"请假" → 人事管理
//	含"财务"/"报销"/"预算"/"资金" → 财务管理
//	含"生产"/"车间"/"工艺"/"质量"/"设备"/"作业" → 生产管理
//	含"行政"/"办公"/"档案"/"会议"/"印章"/"公文"/"接待" → 行政管理
//	含"安全"/"消防"/"环保"/"应急"/"危化" → 安全管理
//	否则 → 其它制度
//
// A blank input stays blank (model could not recognize a type).
func NormalizeRegulationTypeFromName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "人事") || strings.Contains(raw, "员工") || strings.Contains(raw, "考勤") ||
		strings.Contains(raw, "绩效") || strings.Contains(raw, "薪酬") || strings.Contains(raw, "招聘") ||
		strings.Contains(raw, "培训") || strings.Contains(raw, "转岗") || strings.Contains(raw, "任职") ||
		strings.Contains(raw, "请假") || strings.Contains(raw, "人力"):
		return "人事管理"
	case strings.Contains(raw, "财务") || strings.Contains(raw, "报销") || strings.Contains(raw, "预算") ||
		strings.Contains(raw, "资金") || strings.Contains(raw, "付款"):
		return "财务管理"
	case strings.Contains(raw, "生产") || strings.Contains(raw, "车间") || strings.Contains(raw, "工艺") ||
		strings.Contains(raw, "质量") || strings.Contains(raw, "设备") || strings.Contains(raw, "作业"):
		return "生产管理"
	case strings.Contains(raw, "行政") || strings.Contains(raw, "办公") || strings.Contains(raw, "档案") ||
		strings.Contains(raw, "会议") || strings.Contains(raw, "印章") || strings.Contains(raw, "公文") ||
		strings.Contains(raw, "接待"):
		return "行政管理"
	case strings.Contains(raw, "安全") || strings.Contains(raw, "消防") || strings.Contains(raw, "环保") ||
		strings.Contains(raw, "应急") || strings.Contains(raw, "危化"):
		return "安全管理"
	default:
		return "其它制度"
	}
}

// validRegulationTypes is the fixed regulation type enum surfaced in the UI.
var validRegulationTypes = map[string]struct{}{
	"人事管理": {},
	"财务管理": {},
	"生产管理": {},
	"行政管理": {},
	"安全管理": {},
	"其它制度": {},
}

// RegulationsAllEmpty reports whether every regulation in the result is
// effectively blank. Used to guard against persisting a garbage/truncated
// model reply as a successful extraction.
func RegulationsAllEmpty(regs []types.RegulationExtractionItem) bool {
	for _, r := range regs {
		if r.RegNo != "" || r.RegName != "" || r.RegType != "" || r.Dept != "" || r.Version != "" {
			return false
		}
	}
	return true
}
