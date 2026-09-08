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
// with the award/punish fields. A notice may carry several persons — the model
// must emit one record per person.
const awardPunishExtractionSystemPrompt = `你是一个专业的公司奖惩通报文档信息提取助手。你的任务是从文档文本中识别并提取奖惩记录的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 一份奖惩通报只输出一条记录（即使一份通报处罚多人，如"（一）（二）（三）"小节分别叙述每个人的违规事实与处理决定，也合并为一条记录）。person 为全部当事人姓名，多个姓名用"、"分隔（如"汪艳、王郑红"）；dept 为各部门（多个用"、"分隔）；正文没有当事人维度时（纯通报/通知）person/dept 返回空字符串。
3. 如果文档内容与奖惩无关（如发票、合同、制度、清单、报表等），kind 设为 "not_award_punish"，records 返回空数组。
4. ap_no 为文号：提取文档中的文号（如"星达行政字〔2026〕11号"），格式保持原文；文档中确实没有时返回空字符串（不要编造）。
5. ap_title 为标题：通常是文档标题（如"关于对汪艳、王郑红违规行为及质管部手机管理问题的处罚通报"）；无法识别时返回空字符串。
6. ap_type 只从以下枚举中选择：处罚、奖励、通报、其它奖惩。根据处理内容判断：含「处罚」「罚款」「警告」「处分」「批评」「扣款」「记过」→处罚；含「奖励」「表彰」「嘉奖」「表扬」「奖金」「评优」「晋级」→奖励；文档为纯通报/通知性质（无具体奖惩决定）→通报；都不明确时用「其它奖惩」。
7. person 为当事人姓名（多个用"、"分隔，如"汪艳、王郑红"）；dept 为当事人所属部门（多个用"、"分隔）；position 为岗位（多个用"、"分隔）；无法识别时返回空字符串。
8. measure 为措施（如"经济罚款、书面警告""取消年度评优资格"，简要概括所有当事人的处理决定）；basis 为依据（如"依据《AD-018奖惩制度》"，可含制度名）；无法识别时返回空字符串。
9. signer 为签发人（如"廖宏"）；sign_date 为签发日期（YYYY-MM-DD 格式）；effective_date 为生效日期（YYYY-MM-DD 格式，通常与签发日期相同或见"自发布之日起生效"）；无法识别时返回空字符串。
10. remark 为备注（如整改要求、附加说明）；summary 为摘要（用 2-4 句话概括通报目的、违规事实与处理结果）；无法识别时返回空字符串。
11. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"award_punish","records":[{"ap_no":"","ap_title":"","ap_type":"","person":"","dept":"","position":"","measure":"","basis":"","signer":"","sign_date":"","effective_date":"","remark":"","summary":""}]}

示例（一份通报处罚两人，合并为一条记录，person 用"、"分隔多个当事人）：
{"kind":"award_punish","records":[
{"ap_no":"星达行政字〔2026〕11号","ap_title":"关于对汪艳、王郑红违规行为的处罚通报","ap_type":"处罚","person":"汪艳、王郑红","dept":"仓库、质管部","position":"仓库主管","measure":"经济罚款、书面警告；王郑红另取消年度评优资格","basis":"依据《AD-018奖惩制度》","signer":"廖宏","sign_date":"2026-07-02","effective_date":"","remark":"","summary":"汪艳因重复操作失误被处以经济罚款200元并给予书面警告；王郑红因夜班玩手机、睡觉被处以经济罚款300元、书面警告并取消年度评优资格。"}]}`

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
// strict-JSON reply into an AwardPunishExtractionResult.
func ExtractAwardPunishesFromContent(ctx context.Context, model chat.Chat, content string) (*AwardPunishExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &AwardPunishExtractionResult{Kind: "not_award_punish", ExtractError: "no text content to extract"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "award_punish_extract", ""), []chat.Message{
		{Role: "system", Content: awardPunishExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract award/punish fields: %w", err)
	}
	var parsed AwardPunishExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
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
// known kind, drops empty items, normalizes the award/punish type and
// auto-numbers records without an explicit 文号 (JC-YYYYMMDD-NNN).
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
		// 自动编号：无文号时按上传日期自动编号 JC-YYYYMMDD-NNN；
		// 同一文件拆出的多条记录共用同一自动文号（序号相同）。
		if strings.TrimSpace(r.ApNo) == "" {
			r.ApNo = fmt.Sprintf("JC-%s-%03d", now.Format("20060102"), autoSeqStart+1)
		}
		kept = append(kept, r)
	}
	res.Records = kept
}

// MaxAutoAwardPunishSeq returns the highest trailing sequence number among
// records with an auto-generated number JC-YYYYMMDD-NNN for the current date
// inside the knowledge base. Used to keep auto-numbers unique across files.
func (s *knowledgeService) MaxAutoAwardPunishSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "JC-" + time.Now().Format("20060102") + "-"
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
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "award_punish" {
				continue
			}
			for _, r := range meta.Records {
				if !strings.HasPrefix(r.ApNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(r.ApNo, prefix)
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

// NormalizeAwardPunishTypeFromName maps a raw type string onto the fixed enum by
// keyword priority:
//
//	含"处罚"/"罚款"/"警告"/"处分"/"批评"/"扣款"/"记过" → 处罚
//	含"奖励"/"表彰"/"嘉奖"/"表扬"/"奖金"/"评优"/"晋级" → 奖励
//	含"通报" → 通报
//	否则 → 其它奖惩
//
// A blank input stays blank (model could not recognize a type).
func NormalizeAwardPunishTypeFromName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "处罚") || strings.Contains(raw, "罚款") || strings.Contains(raw, "警告") ||
		strings.Contains(raw, "处分") || strings.Contains(raw, "批评") || strings.Contains(raw, "扣款") ||
		strings.Contains(raw, "记过"):
		return "处罚"
	case strings.Contains(raw, "奖励") || strings.Contains(raw, "表彰") || strings.Contains(raw, "嘉奖") ||
		strings.Contains(raw, "表扬") || strings.Contains(raw, "奖金") || strings.Contains(raw, "评优") ||
		strings.Contains(raw, "晋级"):
		return "奖励"
	case strings.Contains(raw, "通报"):
		return "通报"
	default:
		return "其它奖惩"
	}
}

// validAwardPunishTypes is the fixed award/punish type enum surfaced in the UI.
var validAwardPunishTypes = map[string]struct{}{
	"处罚":   {},
	"奖励":   {},
	"通报":   {},
	"其它奖惩": {},
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
