package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// awardPunishExtractMaxContentRunes bounds the document text sent to the
// extraction model so the prompt stays inside a normal context window.
const awardPunishExtractMaxContentRunes = 24000

// AwardPunishExtractionResult is the strict-JSON shape the model must return.
type AwardPunishExtractionResult struct {
	Kind         string                          `json:"kind"` // "award_punish" | "not_award_punish"
	Records      []types.AwardPunishExtractionItem `json:"records"`
	ExtractError string                          `json:"extract_error"`
}

// awardPunishExtractionSystemPrompt instructs the model to return strict JSON
// with the notice/announcement fields. 通知公告模块（原奖惩）一份文件一份公告，
// 涉及多人时合并为一条记录（person 用"、"分隔）。
const awardPunishExtractionSystemPromptTemplate = `你是一个专业的公司通知公告文档信息提取助手。你的任务是从文档文本中识别并提取通知公告记录的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 一份文件只输出一条记录（即使涉及多名当事人、多个事项，也合并为一条记录）。person 为全部当事人姓名，多个姓名用"、"分隔（如"汪艳、王郑红"）；dept 为当事人所属部门（多个用"、"分隔）；正文没有当事人维度时（纯通知/公告/制度性文件）person/dept 返回空字符串。
3. 如果文档内容与通知公告无关（如发票、合同、制度正文、清单、报表等），kind 设为 "not_award_punish"，records 返回空数组。
4. ap_no 为文号：提取文档中的文号（如"星达行政字〔2026〕11号"），格式保持原文；文档中确实没有时返回空字符串（不要编造）。
5. ap_title 为标题：通常是文档标题（如"关于加强安全生产管理的通知"）；无法识别时返回空字符串。
6. ap_type 只从以下枚举中选择：通知、公告、通报、决定、其它通知。判断口径：含「通知」「安排」「部署」「要求」「开展」→通知；含「公告」「公示」「公布」「启事」→公告；含「通报」「情况说明」「通报批评」→通报；含「决定」「处分」「处罚」「任命」「表彰决定」→决定；都不明确时用「其它通知」。
7. issue_dept 为发布单位（文件落款单位/发文部门，如"综合管理部""人力资源部"），无法识别时返回空字符串。
8. person 为当事人姓名（多个用"、"分隔）；dept 为当事人所属部门（多个用"、"分隔）；position 为岗位（多个用"、"分隔）；无法识别时返回空字符串。
9. measure 为处理决定（原文明确写明的处理结论，如"经济处罚200元、书面警告""予以通报表扬"），可多项用"、"分隔；原文未提及处理决定时返回空字符串。
10. basis 为依据（如"依据《AD-018奖惩制度》"，可含制度/法规名）；无法识别时返回空字符串。
11. signer 为发布人（落款签发人，如"廖宏"）；sign_date 为发布日期（YYYY-MM-DD 格式）；effective_date 为生效日期（YYYY-MM-DD 格式，通常与发布日期相同或见"自发布之日起生效"）；无法识别时返回空字符串。
12. remark 为备注（附加说明、细节）；summary 为摘要（用 2-4 句话概括文件目的、核心事项与结论）；无法识别时返回空字符串。
13. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"award_punish","records":[{"ap_no":"","ap_title":"","ap_type":"","issue_dept":"","person":"","dept":"","position":"","measure":"","basis":"","signer":"","sign_date":"","effective_date":"","remark":"","summary":""}]}

示例（一份通报处罚两人，合并为一条记录）：
{"kind":"award_punish","records":[
{"ap_no":"星达行政字〔2026〕11号","ap_title":"关于对汪艳、王郑红违规行为的处罚通报","ap_type":"决定","issue_dept":"综合管理部","person":"汪艳、王郑红","dept":"仓库、质管部","position":"仓库主管","measure":"经济处罚、书面警告","basis":"依据《AD-018奖惩制度》","signer":"廖宏","sign_date":"2026-07-02","effective_date":"","remark":"汪艳罚款200元，王郑红罚款300元并取消年度评优资格","summary":"汪艳因重复操作失误被处以经济罚款200元并给予书面警告；王郑红因夜班玩手机、睡觉被处以经济罚款300元、书面警告并取消年度评优资格。"}]}`

// buildAwardPunishExtractionSystemPrompt renders the notice extraction prompt.
// measures 仅作为处理决定参考，非强制约束（保留参数以兼容调用链）。
func buildAwardPunishExtractionSystemPrompt(measures []string) string {
	return awardPunishExtractionSystemPromptTemplate
}

// BuildAwardPunishExtractionContent assembles the document text sent to the model.
func BuildAwardPunishExtractionContent(name, summary string, chunks []*types.Chunk) string {
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
	return sampleLongContent(strings.Join(parts, "\n\n"), awardPunishExtractMaxContentRunes)
}

// ExtractAwardPunishesFromContent calls the configured chat model and parses its
// strict-JSON reply into an AwardPunishExtractionResult. measures constrains the
// measure field to choices from the award/punish measure library.
func ExtractAwardPunishesFromContent(ctx context.Context, model chat.Chat, content string, measures []string) (*AwardPunishExtractionResult, error) {
	return extractAwardPunishesWithSystemPrompt(ctx, model, buildAwardPunishExtractionSystemPrompt(measures), content)
}

// extractAwardPunishesWithSystemPrompt 是通知公告提取的唯一实现：走通用执行器
// （4 次尝试 + 跨模型清洗层）。此前是单次非流式调用且 MaxTokens=8192。
func extractAwardPunishesWithSystemPrompt(
	ctx context.Context,
	model chat.Chat,
	systemPrompt string,
	content string,
) (*AwardPunishExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &AwardPunishExtractionResult{Kind: "not_award_punish", ExtractError: "no text content to extract"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	raw, err := RunExtractWithRetry(
		types.WithLLMCallMetadata(ctx, "award_punish_extract", ""),
		model, messages, baseOpts, DefaultExtractAttempts(nil),
		func(raw string) bool {
			_, perr := parseAwardPunishReply(raw)
			return perr == nil
		},
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "award/punish extraction unusable after retries: %v (len=%d)", lastErr, len(lastRaw))
		},
	)
	if err != nil {
		return nil, fmt.Errorf("extract award/punish fields: %w", err)
	}
	parsed, perr := parseAwardPunishReply(raw)
	if perr != nil {
		return nil, fmt.Errorf("extract award/punish fields: %w", perr)
	}
	return parsed, nil
}

// parseAwardPunishReply 解析通知公告提取回复（统一走 ParseLLMJsonResponse 清洗层）。
func parseAwardPunishReply(raw string) (*AwardPunishExtractionResult, error) {
	var parsed AwardPunishExtractionResult
	if err := common.ParseLLMJsonResponse(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse award/punish extraction response: %w", err)
	}
	return &parsed, nil
}

// AwardPunishExtractionBatchSize caps how many chunks go into one LLM call.
const AwardPunishExtractionBatchSize = 10

// BuildAwardPunishExtractionBatches assembles the document text into independent
// batches so long notice documents extract reliably.
func BuildAwardPunishExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = AwardPunishExtractionBatchSize
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
		batches = append(batches, sampleLongContent(strings.Join(parts, "\n\n"), awardPunishExtractMaxContentRunes))
	}
	if len(batches) == 0 {
		batches = []string{""}
	}
	return batches
}

// NormalizeAwardPunishExtractionResult cleans up a model reply: guarantees a
// known kind, drops empty items, normalizes the notice type and
// auto-numbers records without an explicit 文号 (TZ-YYYYMMDD-NNN).
//
// autoSeqStart is the highest auto-number already used in the knowledge base
// for the current date, so generated numbers stay unique across files.
func NormalizeAwardPunishExtractionResult(res *AwardPunishExtractionResult, autoSeqStart int) {
	if res == nil {
		return
	}
	if res.Kind != "not_award_punish" {
		res.Kind = "award_punish"
	}
	if len(res.Records) == 0 {
		return
	}
	now := time.Now()
	kept := res.Records[:0]
	for i, r := range res.Records {
		r.ApType = NormalizeAwardPunishTypeFromName(r.ApType)
		r.Page = i + 1
		// 自动编号：无文号时按上传日期自动编号 TZ-YYYYMMDD-NNN；
		// 同一文件拆出的多条记录共用同一自动文号（序号相同）。
		if strings.TrimSpace(r.ApNo) == "" {
			r.ApNo = fmt.Sprintf("TZ-%s-%03d", now.Format("20060102"), autoSeqStart+1)
		}
		kept = append(kept, r)
	}
	res.Records = kept
}

// MaxAutoAwardPunishSeq returns the highest trailing sequence number among
// records with an auto-generated number JC-YYYYMMDD-NNN for the current date
// inside the knowledge base. Used to keep auto-numbers unique across files.

// NormalizeAwardPunishTypeFromName maps a raw type string onto the fixed notice
// enum by keyword priority:
//
//	含"决定"/"处分"/"处罚"/"任命"/"表彰决定" → 决定
//	含"通知"/"安排"/"部署"/"要求"/"开展" → 通知
//	含"公告"/"公示"/"公布"/"启事" → 公告
//	含"通报" → 通报
//	否则 → 其它通知
//
// A blank input stays blank (model could not recognize a type).
func NormalizeAwardPunishTypeFromName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "决定") || strings.Contains(raw, "处分") || strings.Contains(raw, "处罚") ||
		strings.Contains(raw, "任命"):
		return "决定"
	case strings.Contains(raw, "通知") || strings.Contains(raw, "安排") || strings.Contains(raw, "部署") ||
		strings.Contains(raw, "要求") || strings.Contains(raw, "开展"):
		return "通知"
	case strings.Contains(raw, "公告") || strings.Contains(raw, "公示") || strings.Contains(raw, "公布") ||
		strings.Contains(raw, "启事"):
		return "公告"
	case strings.Contains(raw, "通报"):
		return "通报"
	default:
		return "其它通知"
	}
}

// validAwardPunishTypes is the fixed notice type enum surfaced in the UI.
var validAwardPunishTypes = map[string]struct{}{
	"通知":   {},
	"公告":   {},
	"通报":   {},
	"决定":   {},
	"其它通知": {},
}

// AwardPunishRecordsAllEmpty reports whether every record in the result is
// effectively blank. Used to guard against persisting a garbage/truncated
// model reply as a successful extraction.
func AwardPunishRecordsAllEmpty(recs []types.AwardPunishExtractionItem) bool {
	for _, r := range recs {
		if r.ApNo != "" || r.ApTitle != "" || r.ApType != "" || r.Person != "" || r.Dept != "" {
			return false
		}
	}
	return true
}

// ============ 规则提取（用户可配置字段口径） ============

// buildAwardPunishRuleSystemPrompt 在默认通知公告提示词基础上注入用户配置的提取规则
// （字段口径、高级模板）。规则为空时行为与默认版一致。
func buildAwardPunishRuleSystemPrompt(cfg *types.KbExtractConfig, measures []string) string {
	base := buildAwardPunishExtractionSystemPrompt(measures)
	if cfg == nil {
		return base
	}
	hasTemplate := strings.TrimSpace(cfg.PromptTemplate) != ""
	hasFields := len(cfg.Fields) > 0
	if !hasTemplate && !hasFields {
		return base
	}
	var sb strings.Builder
	if hasTemplate {
		r := strings.NewReplacer("{{fields_schema}}", BuildFieldsSchema(cfg.Fields))
		sb.WriteString(strings.TrimSpace(r.Replace(cfg.PromptTemplate)))
		sb.WriteString("\n\n")
	}
	if hasFields {
		sb.WriteString("提取要点（用户配置的字段口径，请按这些要点识别通知公告内容；输出结构仍严格按下方规则执行）:\n")
		for _, f := range cfg.Fields {
			n := strings.TrimSpace(f.Name)
			if n == "" || !f.Enabled {
				continue
			}
			parts := make([]string, 0, 2)
			if d := strings.TrimSpace(f.Desc); d != "" {
				parts = append(parts, "描述："+d)
			}
			if r := strings.TrimSpace(f.Rule); r != "" {
				parts = append(parts, "规则："+r)
			}
			line := "· " + n
			if len(parts) > 0 {
				line += "（" + strings.Join(parts, "；") + "）"
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(base)
	return sb.String()
}

// ExtractAwardPunishesFromContentWithRules 与 ExtractAwardPunishesFromContent 相同，但支持注入
// 用户配置的提取规则（字段口径、高级模板），规则为空时行为与默认版一致。
func ExtractAwardPunishesFromContentWithRules(ctx context.Context, model chat.Chat, content string, measures []string, cfg *types.KbExtractConfig) (*AwardPunishExtractionResult, error) {
	return extractAwardPunishesWithSystemPrompt(ctx, model, buildAwardPunishRuleSystemPrompt(cfg, measures), content)
}
